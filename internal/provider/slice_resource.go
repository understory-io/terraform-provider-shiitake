package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/understory-io/terraform-provider-shiitake/internal/shiitake"
)

// NewSliceResource is the resource factory used by the provider.
func NewSliceResource() resource.Resource { return &sliceResource{} }

type sliceResource struct {
	client *shiitake.Client
}

type sliceResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Project  types.String `tfsdk:"project"`
	Domain   types.String `tfsdk:"domain"`
	Image    types.String `tfsdk:"image"`
	Schedule types.String `tfsdk:"schedule"`
	CPU      types.String `tfsdk:"cpu"`
	Mem      types.String `tfsdk:"mem"`
	Arch     types.String `tfsdk:"arch"`
	Env      types.Map    `tfsdk:"env"`
}

func (r *sliceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_slice"
}

func (r *sliceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Registers a data-mesh slice with shiitake-server, which provisions its ECS task " +
			"definition and schedules it.\n\n" +
			"~> **An apply runs the slice.** Registering a one-shot slice does not merely record it: the server " +
			"provisions the task definition and launches a run (DATA-382). That is the intended behaviour — moving " +
			"`image` to a new build and applying is how a slice picks it up — but it means `terraform apply` on this " +
			"resource executes warehouse work, not just registry bookkeeping. A no-change plan launches nothing.\n\n" +
			"Destroying the resource prunes the slice: scheduling stops and the task definition is marked for teardown.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Same as `name` — the registry is keyed by slice name.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique slice id, e.g. `google-ads-batch`. Must match the slice's " +
					"`shiitake.yaml` `metadata.name`, which is what the running container reports itself as. Immutable.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"project": schema.StringAttribute{
				MarkdownDescription: "Owning workspace, e.g. `canopy-models`. Slices are pruned per-project, so " +
					"this is what scopes a reconcile. Immutable.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "Data-mesh domain, e.g. `marketing`. Should match the slice's `fungus.yaml` " +
					"`spec.domain`.",
				Required: true,
			},
			"image": schema.StringAttribute{
				MarkdownDescription: "Container image, e.g. " +
					"`ghcr.io/understory-io/canopy-models/marketing/google-ads/batch:main`. Changing this is how a " +
					"slice takes a new build — and, for a one-shot, what makes it run.",
				Required: true,
			},
			"schedule": schema.StringAttribute{
				MarkdownDescription: "Cron expression, e.g. `*/5 * * * *`. Leave empty (the default) for a " +
					"one-shot slice that runs on registration rather than on a timer.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
			},
			"cpu": schema.StringAttribute{
				MarkdownDescription: "Fargate CPU units. Defaults to `256`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("256"),
			},
			"mem": schema.StringAttribute{
				MarkdownDescription: "Fargate task memory in MiB. Defaults to `512`. Raise it for slices that " +
					"backfill a large history — the exchange-rates batch slice unnests ~283k rows on first run.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("512"),
			},
			"arch": schema.StringAttribute{
				MarkdownDescription: "Task CPU architecture: `x86_64` or `arm64` (Graviton). Defaults to `x86_64`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("x86_64"),
				Validators: []validator.String{
					stringvalidator.OneOf("x86_64", "arm64"),
				},
			},
			"env": schema.MapAttribute{
				MarkdownDescription: "Extra container environment variables. Credentials do NOT belong here — the " +
					"slice fetches those from shiitake-server at runtime, which is why slice tasks run with no task role.",
				ElementType: types.StringType,
				Optional:    true,
			},
		},
	}
}

func (r *sliceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.client = client
}

// toSlice maps the Terraform model onto the client type. Returns diagnostics
// rather than panicking on a bad env map.
func (m sliceResourceModel) toSlice(ctx context.Context) (shiitake.Slice, error) {
	env := map[string]string{}
	if !m.Env.IsNull() && !m.Env.IsUnknown() {
		if diags := m.Env.ElementsAs(ctx, &env, false); diags.HasError() {
			return shiitake.Slice{}, fmt.Errorf("env is not a map of strings")
		}
	}
	return shiitake.Slice{
		Name:     m.Name.ValueString(),
		Project:  m.Project.ValueString(),
		Domain:   m.Domain.ValueString(),
		Image:    m.Image.ValueString(),
		Schedule: m.Schedule.ValueString(),
		CPU:      m.CPU.ValueString(),
		Mem:      m.Mem.ValueString(),
		Arch:     m.Arch.ValueString(),
		Env:      env,
	}, nil
}

func (r *sliceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sliceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sl, err := plan.toSlice(ctx)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("env"), "Invalid env map", err.Error())
		return
	}
	if err := r.client.RegisterSlice(ctx, sl); err != nil {
		resp.Diagnostics.AddError("Unable to register slice", err.Error())
		return
	}

	plan.ID = types.StringValue(sl.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sliceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sliceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sl, found, err := r.client.GetSlice(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read slice", err.Error())
		return
	}
	if !found {
		// Gone (or pruned) outside Terraform — drop it from state so the next
		// plan recreates it rather than reporting drift on every field.
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(sl.Name)
	state.Project = types.StringValue(sl.Project)
	state.Domain = types.StringValue(sl.Domain)
	state.Image = types.StringValue(sl.Image)
	state.Schedule = types.StringValue(sl.Schedule)
	state.CPU = types.StringValue(sl.CPU)
	state.Mem = types.StringValue(sl.Mem)
	state.Arch = types.StringValue(sl.Arch)

	// Only refresh `env` when the server actually returned one. Writing an
	// empty map over a null would show a permanent diff for every slice that
	// sets no env, which is most of them.
	if len(sl.Env) > 0 {
		env, diags := types.MapValueFrom(ctx, types.StringType, sl.Env)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Env = env
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sliceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sliceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sl, err := plan.toSlice(ctx)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("env"), "Invalid env map", err.Error())
		return
	}
	// RegisterSlice is an upsert, so update is the same call as create.
	if err := r.client.RegisterSlice(ctx, sl); err != nil {
		resp.Diagnostics.AddError("Unable to update slice", err.Error())
		return
	}

	plan.ID = types.StringValue(sl.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sliceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sliceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PruneSlice(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to prune slice", err.Error())
	}
}

// ImportState adopts an already-registered slice by name — the path out of the
// hand-registered world without re-running every slice.
func (r *sliceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

var (
	_ resource.Resource                = &sliceResource{}
	_ resource.ResourceWithConfigure   = &sliceResource{}
	_ resource.ResourceWithImportState = &sliceResource{}
)
