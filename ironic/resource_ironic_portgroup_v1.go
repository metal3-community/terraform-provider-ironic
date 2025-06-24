package ironic

import (
	"context"
	"fmt"

	"github.com/appkins-org/terraform-provider-ironic/ironic/util"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/portgroups"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &portgroupV1Resource{}
	_ resource.ResourceWithConfigure   = &portgroupV1Resource{}
	_ resource.ResourceWithImportState = &portgroupV1Resource{}
)

// portgroupV1Resource defines the resource implementation.
type portgroupV1Resource struct {
	clients *Clients
}

// portgroupV1ResourceModel describes the resource data model.
type portgroupV1ResourceModel struct {
	ID       types.String  `tfsdk:"id"`
	UUID     types.String  `tfsdk:"uuid"`
	NodeUUID types.String  `tfsdk:"node_uuid"`
	Name     types.String  `tfsdk:"name"`
	Address  types.String  `tfsdk:"address"`
	Mode     types.String  `tfsdk:"mode"`
	Extra    types.Dynamic `tfsdk:"extra"`
}

func NewPortgroupV1Resource() resource.Resource {
	return &portgroupV1Resource{}
}

func (r *portgroupV1Resource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_portgroup_v1"
}

func (r *portgroupV1Resource) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Ironic port group resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The UUID of the port group.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the port group. If not specified, a new UUID will be generated.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"node_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the node this port group belongs to.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the port group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"address": schema.StringAttribute{
				MarkdownDescription: "The MAC address of the port group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "The bonding mode of the port group. Defaults to 'active-backup'.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("active-backup"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"extra": schema.DynamicAttribute{
				MarkdownDescription: "Extra metadata for the port group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *portgroupV1Resource) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	clients, ok := req.ProviderData.(*Clients)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected *Clients, got: %T. Please report this issue to the provider developers.",
				req.ProviderData,
			),
		)
		return
	}

	r.clients = clients
}

func (r *portgroupV1Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan portgroupV1ResourceModel

	// Get the plan
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the ironic client
	client, err := r.clients.GetIronicClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting Ironic client",
			fmt.Sprintf("Could not get Ironic client: %s", err),
		)
		return
	}

	// Prepare create options
	createOpts := portgroups.CreateOpts{}

	// Set optional fields
	if !plan.UUID.IsNull() && !plan.UUID.IsUnknown() {
		createOpts.UUID = plan.UUID.ValueString()
	}

	if !plan.NodeUUID.IsNull() && !plan.NodeUUID.IsUnknown() {
		createOpts.NodeUUID = plan.NodeUUID.ValueString()
	}

	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		createOpts.Name = plan.Name.ValueString()
	}

	if !plan.Address.IsNull() && !plan.Address.IsUnknown() {
		createOpts.Address = plan.Address.ValueString()
	}

	if !plan.Mode.IsNull() && !plan.Mode.IsUnknown() {
		createOpts.Mode = plan.Mode.ValueString()
	}

	// Handle extra data
	if !plan.Extra.IsNull() && !plan.Extra.IsUnknown() {
		if extra, err := util.DynamicToMap(ctx, plan.Extra); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("extra"),
				"Error Converting Extra Data",
				fmt.Sprintf("Could not convert extra to map: %s", err),
			)
		} else {
			// Convert map[string]any to map[string]string
			extraStr := make(map[string]string)
			for k, v := range extra {
				if str, ok := v.(string); ok {
					extraStr[k] = str
				} else {
					extraStr[k] = fmt.Sprintf("%v", v)
				}
			}
			createOpts.Extra = extraStr
		}
	}

	// Create the portgroup
	portgroup, err := portgroups.Create(ctx, client, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating portgroup",
			fmt.Sprintf("Could not create portgroup: %s", err),
		)
		return
	}

	// Update plan with computed values
	plan.ID = types.StringValue(portgroup.UUID)

	// Read the created portgroup to get all computed fields
	r.readPortgroupData(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *portgroupV1Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state portgroupV1ResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read the portgroup from the API
	r.readPortgroupData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *portgroupV1Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	// Portgroups don't support updates in the original implementation
	// All attributes are ForceNew, so this should not be called
	resp.Diagnostics.AddError(
		"Update not supported",
		"Portgroup resources do not support updates. All changes require replacement.",
	)
}

func (r *portgroupV1Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state portgroupV1ResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the ironic client
	client, err := r.clients.GetIronicClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting Ironic client",
			fmt.Sprintf("Could not get Ironic client: %s", err),
		)
		return
	}

	// Delete the portgroup
	err = portgroups.Delete(ctx, client, state.ID.ValueString()).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting portgroup",
			fmt.Sprintf("Could not delete portgroup %s: %s", state.ID.ValueString(), err),
		)
		return
	}
}

func (r *portgroupV1Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	// Set the id attribute to the import identifier
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	// Read the portgroup data
	var state portgroupV1ResourceModel
	state.ID = types.StringValue(req.ID)

	r.readPortgroupData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state
	diags := resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Helper function to read portgroup data from the API and populate the model.
func (r *portgroupV1Resource) readPortgroupData(
	ctx context.Context,
	model *portgroupV1ResourceModel,
	diagnostics *diag.Diagnostics,
) {
	client, err := r.clients.GetIronicClient()
	if err != nil {
		diagnostics.AddError(
			"Error getting Ironic client",
			fmt.Sprintf("Could not get Ironic client: %s", err),
		)
		return
	}

	portgroup, err := portgroups.Get(ctx, client, model.ID.ValueString()).Extract()
	if err != nil {
		diagnostics.AddError(
			"Error reading portgroup",
			fmt.Sprintf("Could not read portgroup %s: %s", model.ID.ValueString(), err),
		)
		return
	}

	// Map the API response to the model
	model.ID = types.StringValue(portgroup.UUID)
	model.UUID = types.StringValue(portgroup.UUID)
	model.NodeUUID = types.StringValue(portgroup.NodeUUID)
	model.Name = types.StringValue(portgroup.Name)
	model.Address = types.StringValue(portgroup.Address)
	model.Mode = types.StringValue(portgroup.Mode)

	// Handle extra data
	if portgroup.Extra != nil {
		extra, err := util.MapToDynamic(ctx, portgroup.Extra)
		if err != nil {
			diagnostics.AddError(
				"Error converting extra data",
				fmt.Sprintf("Could not convert extra data to dynamic: %s", err),
			)
			return
		}
		model.Extra = extra
	} else {
		model.Extra = types.DynamicNull()
	}
}
