package shiitake

import "testing"

// parseServer decides whether a connection gets TLS. Getting that wrong fails
// in opposite directions — a silently-plaintext call to prod, or a TLS
// handshake against a local dev server — so each branch is pinned here.
func TestParseServer(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantTarget string
		wantTLS    bool
		wantErr    bool
	}{
		{
			name:       "https keeps host:port and enables TLS",
			in:         "https://shiitake-server.data.api.understory.io:4000",
			wantTarget: "shiitake-server.data.api.understory.io:4000",
			wantTLS:    true,
		},
		{
			name:       "http disables TLS",
			in:         "http://localhost:4040",
			wantTarget: "localhost:4040",
			wantTLS:    false,
		},
		{
			// A bare host:port defaults to TLS on purpose: the only deployment
			// that is not TLS is a local dev server, and that is the case worth
			// spelling out rather than the one worth defaulting to.
			name:       "bare host:port assumes TLS",
			in:         "shiitake-server.data.api.understory.io:4000",
			wantTarget: "shiitake-server.data.api.understory.io:4000",
			wantTLS:    true,
		},
		{
			name:       "surrounding whitespace is tolerated",
			in:         "  https://example.test:4000  ",
			wantTarget: "example.test:4000",
			wantTLS:    true,
		},
		{
			name:    "empty is an error, not a default",
			in:      "",
			wantErr: true,
		},
		{
			name:    "unsupported scheme is rejected",
			in:      "grpc://example.test:4000",
			wantErr: true,
		},
		{
			name:    "scheme with no host is rejected",
			in:      "https://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, useTLS, err := parseServer(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseServer(%q) = (%q, %v, nil), want an error", tt.in, target, useTLS)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseServer(%q) returned unexpected error: %v", tt.in, err)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
			if useTLS != tt.wantTLS {
				t.Errorf("useTLS = %v, want %v", useTLS, tt.wantTLS)
			}
		})
	}
}

// The Slice <-> SliceSpec mapping is field-for-field by hand, which is exactly
// where a typo goes unnoticed: a swapped cpu/mem or a dropped schedule would
// still compile and still register a slice, just the wrong one.
func TestSpecRoundTrip(t *testing.T) {
	in := Slice{
		Name:     "google-ads-batch",
		Project:  "canopy-models",
		Domain:   "marketing",
		Image:    "ghcr.io/understory-io/canopy-models/marketing/google-ads/batch:e5d9e63",
		Schedule: "0 */6 * * *",
		CPU:      "1024",
		Mem:      "2048",
		Arch:     "x86_64",
		Env:      map[string]string{"SOME_FLAG": "1"},
	}

	got := fromSpec(toSpec(in))

	if got.Name != in.Name {
		t.Errorf("Name = %q, want %q", got.Name, in.Name)
	}
	if got.Project != in.Project {
		t.Errorf("Project = %q, want %q", got.Project, in.Project)
	}
	if got.Domain != in.Domain {
		t.Errorf("Domain = %q, want %q", got.Domain, in.Domain)
	}
	if got.Image != in.Image {
		t.Errorf("Image = %q, want %q", got.Image, in.Image)
	}
	if got.Schedule != in.Schedule {
		t.Errorf("Schedule = %q, want %q", got.Schedule, in.Schedule)
	}
	if got.CPU != in.CPU {
		t.Errorf("CPU = %q, want %q", got.CPU, in.CPU)
	}
	if got.Mem != in.Mem {
		t.Errorf("Mem = %q, want %q", got.Mem, in.Mem)
	}
	if got.Arch != in.Arch {
		t.Errorf("Arch = %q, want %q", got.Arch, in.Arch)
	}
	if got.Env["SOME_FLAG"] != "1" {
		t.Errorf("Env = %v, want SOME_FLAG=1", got.Env)
	}
}

// An empty schedule is how a one-shot slice is expressed, so it has to survive
// the round trip as empty rather than becoming a zero-ish cron.
func TestOneShotScheduleSurvivesRoundTrip(t *testing.T) {
	got := fromSpec(toSpec(Slice{Name: "google-ads-stream", Schedule: ""}))
	if got.Schedule != "" {
		t.Errorf("Schedule = %q, want empty (one-shot)", got.Schedule)
	}
}

// fromSpec is called on server responses, which can legitimately be nil for a
// not-found slice. It must not panic there.
func TestFromSpecNil(t *testing.T) {
	if got := fromSpec(nil); got.Name != "" {
		t.Errorf("fromSpec(nil) = %+v, want zero Slice", got)
	}
}

func TestWrapNilStaysNil(t *testing.T) {
	if err := wrap("register slice x", nil); err != nil {
		t.Errorf("wrap(op, nil) = %v, want nil", err)
	}
}
