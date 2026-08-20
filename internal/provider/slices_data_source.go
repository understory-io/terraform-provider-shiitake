package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/understory-io/terraform-provider-shiitake/internal/shiitake"
)

// NewSlicesDataSource lists what is actually registered.
//
// Worth having beyond curiosity: the registry was hand-maintained before this
// provider, so the first job is reconciling what Terraform thinks exists with
// what does. This is the read side of that.
func NewSlicesDataSource() datasource.DataSource { return &slicesDataSource{} }

type slicesDataSource struct {
	client *shiitake.Client
}

type sliceModel struct {
	Name     types.String `tfsdk:"name"`
	Project  types.String `tfsdk:"project"`
	Domain   types.String `tfsdk:"domain"`
	Image    types.String `tfsdk:"image"`
	Schedule types.String `tfsdk:"schedule"`
	CPU      types.String `tfsdk:"cpu"`
	Mem      types.String `tfsdk:"mem"`
	Arch     types.String `tfsdk:"arch"`
}

type slicesDataSourceModel struct {
	Slices []sliceModel `tfsdk:"slices"`
}

func (d *slicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slices"
}

func (d *slicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Every non-pruned slice in the shiitake registry.",
		Attributes: map[string]schema.Attribute{
			"slices": schema.ListNestedAttribute{
				MarkdownDescription: "Registered slices.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":     schema.StringAttribute{Computed: true},
						"project":  schema.StringAttribute{Computed: true},
						"domain":   schema.StringAttribute{Computed: true},
						"image":    schema.StringAttribute{Computed: true},
						"schedule": schema.StringAttribute{Computed: true, MarkdownDescription: "Empty for a one-shot slice."},
						"cpu":      schema.StringAttribute{Computed: true},
						"mem":      schema.StringAttribute{Computed: true},
						"arch":     schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *slicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*shiitake.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *shiitake.Client, got %T. This is a provider bug.", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *slicesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	slices, err := d.client.ListSlices(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list slices", err.Error())
		return
	}

	state := slicesDataSourceModel{Slices: make([]sliceModel, 0, len(slices))}
	for _, s := range slices {
		state.Slices = append(state.Slices, sliceModel{
			Name:     types.StringValue(s.Name),
			Project:  types.StringValue(s.Project),
			Domain:   types.StringValue(s.Domain),
			Image:    types.StringValue(s.Image),
			Schedule: types.StringValue(s.Schedule),
			CPU:      types.StringValue(s.CPU),
			Mem:      types.StringValue(s.Mem),
			Arch:     types.StringValue(s.Arch),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

var (
	_ datasource.DataSource              = &slicesDataSource{}
	_ datasource.DataSourceWithConfigure = &slicesDataSource{}
)
