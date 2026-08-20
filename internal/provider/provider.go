// Package provider implements the terraform-provider-shiitake Terraform
// provider. It manages entries in shiitake-server's slice registry — the thing
// the orchestrator schedules data-mesh slices from (DATA-382).
//
// Why a provider rather than a CI script: slice registration is desired STATE,
// not an event. Before this, `shiitake schedule register` was run by hand, so a
// slice existed in the registry because somebody remembered — which is how
// external/exchange-rates shipped its images, its contracts and its ClickHouse
// grants and still never ran (DATA-590). Terraform makes the set of slices a
// reviewable file, and removing one from that file prunes it.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/understory-io/terraform-provider-shiitake/internal/shiitake"
)

// New returns a provider factory bound to the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &shiitakeProvider{version: version} }
}

type shiitakeProvider struct {
	version string
}

type providerModel struct {
	Server types.String `tfsdk:"server"`
	APIKey types.String `tfsdk:"api_key"`
}

func (p *shiitakeProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "shiitake"
	resp.Version = p.version
}

func (p *shiitakeProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the shiitake slice registry — which data-mesh slices the orchestrator " +
			"provisions and schedules.",
		Attributes: map[string]schema.Attribute{
			"server": schema.StringAttribute{
				MarkdownDescription: "shiitake-server gRPC address, e.g. `https://shiitake-server.<your-env>:4000`. " +
					"Falls back to the `SHIITAKE_SERVER` environment variable. " +
					"An `http://` scheme disables TLS; a bare `host:port` assumes TLS.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Service-account API key, sent as `authorization: Bearer <key>`. " +
					"Falls back to the `SHIITAKE_SERVICE_ACCOUNT_API_KEY` environment variable. " +
					"Terraform generates this key, so prefer reading it from the secret rather than " +
					"pasting it into a provider block.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}

func (p *shiitakeProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	server := data.Server.ValueString()
	if server == "" {
		server = os.Getenv("SHIITAKE_SERVER")
	}
	if server == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("server"),
			"Missing shiitake-server address",
			"Provide `server` in the provider block or set the SHIITAKE_SERVER environment variable.",
		)
		return
	}

	apiKey := data.APIKey.ValueString()
	if apiKey == "" {
		apiKey = os.Getenv("SHIITAKE_SERVICE_ACCOUNT_API_KEY")
	}
	if apiKey == "" {
		// Not fatal: the server rejects unauthenticated calls itself, and its
		// error names the problem better than a guess here would. But warn,
		// because "invalid api key" on every resource is a confusing way to
		// learn the variable was unset.
		resp.Diagnostics.AddAttributeWarning(
			path.Root("api_key"),
			"No shiitake API key configured",
			"Neither `api_key` nor SHIITAKE_SERVICE_ACCOUNT_API_KEY is set. Calls will be rejected as unauthenticated.",
		)
	}

	client, err := shiitake.New(server, apiKey)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create shiitake client", err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *shiitakeProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSliceResource,
	}
}

func (p *shiitakeProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSlicesDataSource,
	}
}
