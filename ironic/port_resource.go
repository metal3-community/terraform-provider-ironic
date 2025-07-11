package ironic

import (
	"context"
	"fmt"

	"github.com/appkins-org/terraform-provider-ironic/ironic/util"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &portV1Resource{}
	_ resource.ResourceWithConfigure   = &portV1Resource{}
	_ resource.ResourceWithImportState = &portV1Resource{}
)

// portV1Resource defines the resource implementation.
type portV1Resource struct {
	clients *Clients
}

// portV1ResourceModel describes the resource data model.
type portV1ResourceModel struct {
	ID                  types.String  `tfsdk:"id"`
	NodeUUID            types.String  `tfsdk:"node_uuid"`
	Address             types.String  `tfsdk:"address"`
	PortGroupUUID       types.String  `tfsdk:"port_group_uuid"`
	LocalLinkConnection types.Dynamic `tfsdk:"local_link_connection"`
	PXEEnabled          types.Bool    `tfsdk:"pxe_enabled"`
	PhysicalNetwork     types.String  `tfsdk:"physical_network"`
	Extra               types.Dynamic `tfsdk:"extra"`
	IsSmartNIC          types.Bool    `tfsdk:"is_smart_nic"`
}

func NewPortV1Resource() resource.Resource {
	return &portV1Resource{}
}

func (r *portV1Resource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_port_v1"
}

func (r *portV1Resource) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Ironic port resource. Ports represent network interfaces on baremetal nodes and can be used to configure network connectivity, including PXE boot settings and switch connectivity information.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The UUID of the port.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"node_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the node this port belongs to.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"address": schema.StringAttribute{
				MarkdownDescription: "The MAC address of the port.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"port_group_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the port group this port belongs to.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"local_link_connection": schema.DynamicAttribute{
				MarkdownDescription: "The local link connection information for the port.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"pxe_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether PXE is enabled for this port.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"physical_network": schema.StringAttribute{
				MarkdownDescription: "The physical network name for the port.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"extra": schema.DynamicAttribute{
				MarkdownDescription: "Extra metadata for the port.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"is_smart_nic": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a Smart NIC port.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *portV1Resource) Configure(
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

func (r *portV1Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan portV1ResourceModel

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
	createOpts := ports.CreateOpts{}

	// Set optional fields
	if !plan.NodeUUID.IsNull() && !plan.NodeUUID.IsUnknown() {
		createOpts.NodeUUID = plan.NodeUUID.ValueString()
	}

	if !plan.Address.IsNull() && !plan.Address.IsUnknown() {
		createOpts.Address = plan.Address.ValueString()
	}

	if !plan.PortGroupUUID.IsNull() && !plan.PortGroupUUID.IsUnknown() {
		createOpts.PortGroupUUID = plan.PortGroupUUID.ValueString()
	}

	if !plan.PhysicalNetwork.IsNull() && !plan.PhysicalNetwork.IsUnknown() {
		createOpts.PhysicalNetwork = plan.PhysicalNetwork.ValueString()
	}

	// Handle boolean fields
	if !plan.PXEEnabled.IsNull() && !plan.PXEEnabled.IsUnknown() {
		pxeEnabled := plan.PXEEnabled.ValueBool()
		createOpts.PXEEnabled = &pxeEnabled
	}

	if !plan.IsSmartNIC.IsNull() && !plan.IsSmartNIC.IsUnknown() {
		isSmartNIC := plan.IsSmartNIC.ValueBool()
		createOpts.IsSmartNIC = &isSmartNIC
	}

	// Handle dynamic fields
	if !plan.LocalLinkConnection.IsNull() && !plan.LocalLinkConnection.IsUnknown() {
		if localLink, err := util.DynamicToMap(ctx, plan.LocalLinkConnection); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("local_link_connection"),
				"Error Converting Local Link Connection",
				fmt.Sprintf("Could not convert local_link_connection to map: %s", err),
			)
			return
		} else {
			createOpts.LocalLinkConnection = localLink
		}
	}

	if !plan.Extra.IsNull() && !plan.Extra.IsUnknown() {
		if extra, err := util.DynamicToMap(ctx, plan.Extra); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("extra"),
				"Error Converting Extra Data",
				fmt.Sprintf("Could not convert extra to map: %s", err),
			)
			return
		} else {
			createOpts.Extra = extra
		}
	}

	// Create the port
	port, err := ports.Create(ctx, client, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating port",
			fmt.Sprintf("Could not create port: %s", err),
		)
		return
	}

	// Update plan with computed values
	plan.ID = types.StringValue(port.UUID)

	// Read the created port to get all computed fields
	r.readPortData(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *portV1Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state portV1ResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read the port from the API
	r.readPortData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *portV1Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan portV1ResourceModel
	var state portV1ResourceModel

	// Get plan and current state
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = req.State.Get(ctx, &state)
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

	// Prepare update options
	updateOpts := ports.UpdateOpts{}

	// Check for changes and build update operations
	if !plan.Address.Equal(state.Address) {
		if !plan.Address.IsNull() && !plan.Address.IsUnknown() {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/address",
				Value: plan.Address.ValueString(),
			})
		}
	}

	if !plan.NodeUUID.Equal(state.NodeUUID) {
		if !plan.NodeUUID.IsNull() && !plan.NodeUUID.IsUnknown() {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/node_uuid",
				Value: plan.NodeUUID.ValueString(),
			})
		}
	}

	if !plan.PortGroupUUID.Equal(state.PortGroupUUID) {
		if !plan.PortGroupUUID.IsNull() && !plan.PortGroupUUID.IsUnknown() {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/portgroup_uuid",
				Value: plan.PortGroupUUID.ValueString(),
			})
		}
	}

	if !plan.PhysicalNetwork.Equal(state.PhysicalNetwork) {
		if plan.PhysicalNetwork.IsNull() || plan.PhysicalNetwork.ValueString() == "" {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:   ports.RemoveOp,
				Path: "/physical_network",
			})
		} else {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/physical_network",
				Value: plan.PhysicalNetwork.ValueString(),
			})
		}
	}

	if !plan.PXEEnabled.Equal(state.PXEEnabled) {
		if !plan.PXEEnabled.IsNull() && !plan.PXEEnabled.IsUnknown() {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/pxe_enabled",
				Value: plan.PXEEnabled.ValueBool(),
			})
		}
	}

	if !plan.LocalLinkConnection.Equal(state.LocalLinkConnection) {
		if !plan.LocalLinkConnection.IsNull() && !plan.LocalLinkConnection.IsUnknown() {
			if localLink, err := util.DynamicToMap(ctx, plan.LocalLinkConnection); err != nil {
				resp.Diagnostics.AddAttributeError(
					path.Root("local_link_connection"),
					"Error Converting Local Link Connection",
					fmt.Sprintf("Could not convert local_link_connection to map: %s", err),
				)
				return
			} else {
				updateOpts = append(updateOpts, ports.UpdateOperation{
					Op:    ports.ReplaceOp,
					Path:  "/local_link_connection",
					Value: localLink,
				})
			}
		}
	}

	// Perform the update if there are changes
	if len(updateOpts) > 0 {
		_, err = ports.Update(ctx, client, state.ID.ValueString(), updateOpts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating port",
				fmt.Sprintf("Could not update port %s: %s", state.ID.ValueString(), err),
			)
			return
		}
	}

	// Read the updated port
	r.readPortData(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set updated state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *portV1Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state portV1ResourceModel

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

	// Delete the port
	err = ports.Delete(ctx, client, state.ID.ValueString()).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting port",
			fmt.Sprintf("Could not delete port %s: %s", state.ID.ValueString(), err),
		)
		return
	}
}

func (r *portV1Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	// Set the id attribute to the import identifier
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	// Read the port data
	var state portV1ResourceModel
	state.ID = types.StringValue(req.ID)

	r.readPortData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state
	diags := resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Helper function to read port data from the API and populate the model.
func (r *portV1Resource) readPortData(
	ctx context.Context,
	model *portV1ResourceModel,
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

	port, err := ports.Get(ctx, client, model.ID.ValueString()).Extract()
	if err != nil {
		diagnostics.AddError(
			"Error reading port",
			fmt.Sprintf("Could not read port %s: %s", model.ID.ValueString(), err),
		)
		return
	}

	// Map the API response to the model
	model.ID = types.StringValue(port.UUID)
	model.NodeUUID = types.StringValue(port.NodeUUID)
	model.Address = types.StringValue(port.Address)
	model.PortGroupUUID = types.StringValue(port.PortGroupUUID)
	model.PhysicalNetwork = types.StringValue(port.PhysicalNetwork)

	// Handle boolean fields
	model.PXEEnabled = types.BoolValue(port.PXEEnabled)
	model.IsSmartNIC = types.BoolValue(port.IsSmartNIC)

	// Handle local link connection
	if port.LocalLinkConnection != nil {
		localLink, err := util.MapToDynamic(ctx, port.LocalLinkConnection)
		if err != nil {
			diagnostics.AddError(
				"Error converting local link connection",
				fmt.Sprintf("Could not convert local link connection to dynamic: %s", err),
			)
			return
		}
		model.LocalLinkConnection = localLink
	} else {
		model.LocalLinkConnection = types.DynamicNull()
	}

	// Handle extra data
	if port.Extra != nil {
		extra, err := util.MapToDynamic(ctx, port.Extra)
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
