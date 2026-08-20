// Package shiitake is a thin gRPC client for shiitake-server's ScheduleService
// — the slice registry the orchestrator schedules from (DATA-382).
//
// Deliberately thin: it owns the connection, the auth header and error
// translation, and nothing else. Resource semantics live in internal/provider.
package shiitake

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	shiitakev1 "github.com/understory-io/terraform-provider-shiitake/gen/shiitake/v1"
)

// Client wraps a ScheduleService connection.
type Client struct {
	conn     *grpc.ClientConn
	schedule shiitakev1.ScheduleServiceClient
	apiKey   string
}

// Slice is the provider-facing view of a registry entry. Mirrors
// shiitake.v1.SliceSpec, so the field-for-field mapping stays obvious.
type Slice struct {
	Name     string
	Project  string
	Domain   string
	Image    string
	Schedule string // cron expression; empty means one-shot
	CPU      string
	Mem      string
	Arch     string
	Env      map[string]string
}

// New dials server and returns a client.
//
// `server` is a URL — `https://host:port` gets TLS, `http://host:port` does not.
// A bare `host:port` is treated as https, because the only deployment that is
// not TLS is a local dev server and that is the case worth making explicit.
func New(server, apiKey string) (*Client, error) {
	target, useTLS, err := parseServer(server)
	if err != nil {
		return nil, err
	}

	creds := insecure.NewCredentials()
	if useTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial shiitake-server at %s: %w", server, err)
	}

	return &Client{
		conn:     conn,
		schedule: shiitakev1.NewScheduleServiceClient(conn),
		apiKey:   apiKey,
	}, nil
}

// Close releases the connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// parseServer splits a server URL into a gRPC target and a TLS decision.
func parseServer(server string) (target string, useTLS bool, err error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", false, fmt.Errorf("empty shiitake-server address")
	}
	if !strings.Contains(server, "://") {
		// Bare host:port. Assume TLS — see New's doc comment.
		return server, true, nil
	}
	u, err := url.Parse(server)
	if err != nil {
		return "", false, fmt.Errorf("parse shiitake-server address %q: %w", server, err)
	}
	host := u.Host
	if host == "" {
		return "", false, fmt.Errorf("shiitake-server address %q has no host", server)
	}
	switch u.Scheme {
	case "https":
		return host, true, nil
	case "http":
		return host, false, nil
	default:
		return "", false, fmt.Errorf("shiitake-server address %q: unsupported scheme %q (want http or https)", server, u.Scheme)
	}
}

// auth attaches the bearer token the server expects. Matches the shiitake
// CLI's interceptor: `authorization: Bearer <key>`.
func (c *Client) auth(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	if c.apiKey == "" {
		return ctx, cancel
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.apiKey), cancel
}

func toSpec(s Slice) *shiitakev1.SliceSpec {
	return &shiitakev1.SliceSpec{
		Project:  s.Project,
		Domain:   s.Domain,
		Name:     s.Name,
		Image:    s.Image,
		Schedule: s.Schedule,
		Cpu:      s.CPU,
		Mem:      s.Mem,
		Arch:     s.Arch,
		Env:      s.Env,
	}
}

func fromSpec(sp *shiitakev1.SliceSpec) Slice {
	if sp == nil {
		return Slice{}
	}
	return Slice{
		Name:     sp.GetName(),
		Project:  sp.GetProject(),
		Domain:   sp.GetDomain(),
		Image:    sp.GetImage(),
		Schedule: sp.GetSchedule(),
		CPU:      sp.GetCpu(),
		Mem:      sp.GetMem(),
		Arch:     sp.GetArch(),
		Env:      sp.GetEnv(),
	}
}

// RegisterSlice upserts a slice.
//
// NOTE this is not a passive registry write: for a one-shot slice the server
// provisions the ECS task definition AND LAUNCHES A RUN (DATA-382). So an apply
// that changes a slice runs it, which is usually what you want from "the image
// moved" — but it is a side effect worth knowing about before you `apply` in
// anger. Documented on the resource too.
func (c *Client) RegisterSlice(ctx context.Context, s Slice) error {
	ctx, cancel := c.auth(ctx)
	defer cancel()
	_, err := c.schedule.RegisterSlice(ctx, &shiitakev1.RegisterSliceRequest{Spec: toSpec(s)})
	return wrap("register slice "+s.Name, err)
}

// GetSlice reads one slice. found=false means the registry has no such entry,
// which the provider treats as "removed outside Terraform".
func (c *Client) GetSlice(ctx context.Context, name string) (sl Slice, found bool, err error) {
	ctx, cancel := c.auth(ctx)
	defer cancel()
	resp, err := c.schedule.GetSlice(ctx, &shiitakev1.GetSliceRequest{Name: name})
	if err != nil {
		return Slice{}, false, wrap("get slice "+name, err)
	}
	if !resp.GetFound() {
		return Slice{}, false, nil
	}
	st := resp.GetState()
	// A pruned entry still exists in the registry but is retired. Treat it as
	// absent so Terraform recreates rather than reporting drift forever.
	if st.GetPruned() {
		return Slice{}, false, nil
	}
	return fromSpec(st.GetSpec()), true, nil
}

// PruneSlice retires a slice: stops scheduling it and marks it for ECS
// teardown. Idempotent, per the service contract.
func (c *Client) PruneSlice(ctx context.Context, name string) error {
	ctx, cancel := c.auth(ctx)
	defer cancel()
	_, err := c.schedule.PruneSlice(ctx, &shiitakev1.PruneSliceRequest{Name: name})
	return wrap("prune slice "+name, err)
}

// ListSlices returns every non-pruned entry. Backs the data source.
func (c *Client) ListSlices(ctx context.Context) ([]Slice, error) {
	ctx, cancel := c.auth(ctx)
	defer cancel()
	resp, err := c.schedule.ListSlices(ctx, &shiitakev1.ListSlicesRequest{})
	if err != nil {
		return nil, wrap("list slices", err)
	}
	out := make([]Slice, 0, len(resp.GetSlices()))
	for _, st := range resp.GetSlices() {
		if st.GetPruned() {
			continue
		}
		out = append(out, fromSpec(st.GetSpec()))
	}
	return out, nil
}

// wrap turns a gRPC status into an error whose message names the operation and
// carries the server's own text — an "invalid api key" or "not found" should
// read as itself, not as "rpc error: code = ...".
func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok {
		return fmt.Errorf("%s: %s (%s)", op, st.Message(), st.Code())
	}
	return fmt.Errorf("%s: %w", op, err)
}
