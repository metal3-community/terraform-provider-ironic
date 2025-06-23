package ironic

import (
	"context"
	"fmt"

	"github.com/appkins-org/terraform-provider-ironic/ironic/util"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
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

const (
	DefaultAvailable = true
	DefaultManage    = true
	DefaultInspect   = true
	DefaultClean     = false
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &nodeV1Resource{}
	_ resource.ResourceWithConfigure   = &nodeV1Resource{}
	_ resource.ResourceWithImportState = &nodeV1Resource{}
)

// nodeV1Resource defines the resource implementation.
type nodeV1Resource struct {
	clients *Clients
}

// nodeV1ResourceModel describes the resource data model.
type nodeV1ResourceModel struct {
	ID                   types.String      `tfsdk:"id"`
	Name                 types.String      `tfsdk:"name"`
	NetworkInterface     types.String      `tfsdk:"network_interface"`
	Driver               types.String      `tfsdk:"driver"`
	BootInterface        types.String      `tfsdk:"boot_interface"`
	ConsoleInterface     types.String      `tfsdk:"console_interface"`
	DeployInterface      types.String      `tfsdk:"deploy_interface"`
	InspectInterface     types.String      `tfsdk:"inspect_interface"`
	ManagementInterface  types.String      `tfsdk:"management_interface"`
	PowerInterface       types.String      `tfsdk:"power_interface"`
	RAIDInterface        types.String      `tfsdk:"raid_interface"`
	RescueInterface      types.String      `tfsdk:"rescue_interface"`
	StorageInterface     types.String      `tfsdk:"storage_interface"`
	VendorInterface      types.String      `tfsdk:"vendor_interface"`
	Automated            types.Bool        `tfsdk:"automated_clean"`
	Protected            types.Bool        `tfsdk:"protected"`
	Maintenance          types.Bool        `tfsdk:"maintenance"`
	MaintenanceReason    types.String      `tfsdk:"maintenance_reason"`
	Clean                types.Bool        `tfsdk:"clean"`
	Inspect              types.Bool        `tfsdk:"inspect"`
	Available            types.Bool        `tfsdk:"available"`
	Manage               types.Bool        `tfsdk:"manage"`
	Properties           types.Dynamic     `tfsdk:"properties"`
	DriverInfo           types.Dynamic     `tfsdk:"driver_info"`
	InstanceInfo         types.Dynamic     `tfsdk:"instance_info"`
	InstanceUUID         types.String      `tfsdk:"instance_uuid"`
	ResourceClass        types.String      `tfsdk:"resource_class"`
	Ports                []nodeV1PortModel `tfsdk:"ports"`
	ProvisionState       types.String      `tfsdk:"provision_state"`
	PowerState           types.String      `tfsdk:"power_state"`
	TargetProvisionState types.String      `tfsdk:"target_provision_state"`
	TargetPowerState     types.String      `tfsdk:"target_power_state"`
	LastError            types.String      `tfsdk:"last_error"`
	ExtraData            types.Dynamic     `tfsdk:"extra"`
	Owner                types.String      `tfsdk:"owner"`
	Lessee               types.String      `tfsdk:"lessee"`
	Conductor            types.String      `tfsdk:"conductor"`
	ConductorGroup       types.String      `tfsdk:"conductor_group"`
	AllocationUUID       types.String      `tfsdk:"allocation_uuid"`
	Chassis              types.String      `tfsdk:"chassis_uuid"`
	Created              types.String      `tfsdk:"created_at"`
	Updated              types.String      `tfsdk:"updated_at"`
	CleanStep            types.Dynamic     `tfsdk:"clean_step"`
	DeployStep           types.Dynamic     `tfsdk:"deploy_step"`
	Fault                types.String      `tfsdk:"fault"`
	BIOSInterface        types.String      `tfsdk:"bios_interface"`
	FirmwareInterface    types.String      `tfsdk:"firmware_interface"`
}

// nodeV1PortModel describes the port data model within the node.
type nodeV1PortModel struct {
	UUID       types.String `tfsdk:"uuid"`
	MACAddress types.String `tfsdk:"mac_address"`
}

func NewNodeV1Resource() resource.Resource {
	return &nodeV1Resource{}
}

func (r *nodeV1Resource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_node_v1"
}

func (r *nodeV1Resource) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Node v1 resource represents a single node in Ironic.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier of the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the node.",
				Optional:            true,
				Computed:            true,
			},
			"network_interface": schema.StringAttribute{
				MarkdownDescription: "The network interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"driver": schema.StringAttribute{
				MarkdownDescription: "The driver associated with the node.",
				Required:            true,
			},
			"boot_interface": schema.StringAttribute{
				MarkdownDescription: "The boot interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"console_interface": schema.StringAttribute{
				MarkdownDescription: "The console interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"deploy_interface": schema.StringAttribute{
				MarkdownDescription: "The deploy interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"inspect_interface": schema.StringAttribute{
				MarkdownDescription: "The inspect interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"management_interface": schema.StringAttribute{
				MarkdownDescription: "The management interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"power_interface": schema.StringAttribute{
				MarkdownDescription: "The power interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"raid_interface": schema.StringAttribute{
				MarkdownDescription: "The RAID interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"rescue_interface": schema.StringAttribute{
				MarkdownDescription: "The rescue interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"storage_interface": schema.StringAttribute{
				MarkdownDescription: "The storage interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vendor_interface": schema.StringAttribute{
				MarkdownDescription: "The vendor interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"automated_clean": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the node should be cleaned automatically.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"protected": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the node is protected.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"maintenance": schema.BoolAttribute{
				MarkdownDescription: "Indicates whether the node is in maintenance mode.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"maintenance_reason": schema.StringAttribute{
				MarkdownDescription: "The reason for putting the node in maintenance mode.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"properties": schema.DynamicAttribute{
				MarkdownDescription: "The properties of the node.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"driver_info": schema.DynamicAttribute{
				MarkdownDescription: "The driver info of the node.",
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_info": schema.DynamicAttribute{
				MarkdownDescription: "The instance info of the node.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the instance associated with the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"resource_class": schema.StringAttribute{
				MarkdownDescription: "The resource class of the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provision_state": schema.StringAttribute{
				MarkdownDescription: "The current provision state of the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"power_state": schema.StringAttribute{
				MarkdownDescription: "The current power state of the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_provision_state": schema.StringAttribute{
				MarkdownDescription: "The target provision state of the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_power_state": schema.StringAttribute{
				MarkdownDescription: "The target power state of the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"last_error": schema.StringAttribute{
				MarkdownDescription: "The last error message for the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"extra": schema.DynamicAttribute{
				MarkdownDescription: "Extra metadata for the node.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"owner": schema.StringAttribute{
				MarkdownDescription: "The owner of the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"lessee": schema.StringAttribute{
				MarkdownDescription: "The lessee of the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"conductor": schema.StringAttribute{
				MarkdownDescription: "The conductor managing the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"conductor_group": schema.StringAttribute{
				MarkdownDescription: "The conductor group for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allocation_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the allocation associated with the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"chassis_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the chassis associated with the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node was created.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node was last updated.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"clean_step": schema.DynamicAttribute{
				MarkdownDescription: "The current clean step for the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"deploy_step": schema.DynamicAttribute{
				MarkdownDescription: "The current deploy step for the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.UseStateForUnknown(),
				},
			},
			"fault": schema.StringAttribute{
				MarkdownDescription: "The fault status of the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"bios_interface": schema.StringAttribute{
				MarkdownDescription: "The BIOS interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"firmware_interface": schema.StringAttribute{
				MarkdownDescription: "The firmware interface for the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"clean": schema.BoolAttribute{
				MarkdownDescription: "Trigger node cleaning. When set to true, the node will be moved to the cleaning state.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(DefaultClean),
			},
			"inspect": schema.BoolAttribute{
				MarkdownDescription: "Trigger node inspection. When set to true, the node will be moved to the inspection state.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(DefaultInspect),
			},
			"available": schema.BoolAttribute{
				MarkdownDescription: "Make node available. When set to true, the node will be moved to the available state.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(DefaultAvailable),
			},
			"manage": schema.BoolAttribute{
				MarkdownDescription: "Manage node. When set to true, the node will be moved to the manageable state.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(DefaultManage),
			},
		},
		Blocks: map[string]schema.Block{
			"ports": schema.ListNestedBlock{
				MarkdownDescription: "Ports associated with the node.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"uuid": schema.StringAttribute{
							MarkdownDescription: "The UUID of the port.",
							Computed:            true,
						},
						"mac_address": schema.StringAttribute{
							MarkdownDescription: "The MAC address of the port.",
							Required:            true,
						},
					},
				},
			},
		},
	}
}

func (r *nodeV1Resource) Configure(
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

func (r *nodeV1Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan nodeV1ResourceModel

	// Get the plan
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prepare create options
	createOpts := nodes.CreateOpts{
		Driver: plan.Driver.ValueString(),
	}

	// Set optional fields
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		createOpts.Name = plan.Name.ValueString()
	}

	if !plan.NetworkInterface.IsNull() && !plan.NetworkInterface.IsUnknown() {
		createOpts.NetworkInterface = plan.NetworkInterface.ValueString()
	}

	if !plan.BootInterface.IsNull() && !plan.BootInterface.IsUnknown() {
		createOpts.BootInterface = plan.BootInterface.ValueString()
	}

	if !plan.ConsoleInterface.IsNull() && !plan.ConsoleInterface.IsUnknown() {
		createOpts.ConsoleInterface = plan.ConsoleInterface.ValueString()
	}

	if !plan.DeployInterface.IsNull() && !plan.DeployInterface.IsUnknown() {
		createOpts.DeployInterface = plan.DeployInterface.ValueString()
	}

	if !plan.InspectInterface.IsNull() && !plan.InspectInterface.IsUnknown() {
		createOpts.InspectInterface = plan.InspectInterface.ValueString()
	}

	if !plan.ManagementInterface.IsNull() && !plan.ManagementInterface.IsUnknown() {
		createOpts.ManagementInterface = plan.ManagementInterface.ValueString()
	}

	if !plan.PowerInterface.IsNull() && !plan.PowerInterface.IsUnknown() {
		createOpts.PowerInterface = plan.PowerInterface.ValueString()
	}

	if !plan.RAIDInterface.IsNull() && !plan.RAIDInterface.IsUnknown() {
		createOpts.RAIDInterface = plan.RAIDInterface.ValueString()
	}

	if !plan.RescueInterface.IsNull() && !plan.RescueInterface.IsUnknown() {
		createOpts.RescueInterface = plan.RescueInterface.ValueString()
	}

	if !plan.StorageInterface.IsNull() && !plan.StorageInterface.IsUnknown() {
		createOpts.StorageInterface = plan.StorageInterface.ValueString()
	}

	if !plan.VendorInterface.IsNull() && !plan.VendorInterface.IsUnknown() {
		createOpts.VendorInterface = plan.VendorInterface.ValueString()
	}

	if !plan.BIOSInterface.IsNull() && !plan.BIOSInterface.IsUnknown() {
		createOpts.BIOSInterface = plan.BIOSInterface.ValueString()
	}

	if !plan.FirmwareInterface.IsNull() && !plan.FirmwareInterface.IsUnknown() {
		createOpts.FirmwareInterface = plan.FirmwareInterface.ValueString()
	}

	// Handle maps
	if !plan.Properties.IsNull() && !plan.Properties.IsUnknown() {
		if properties, err := util.DynamicToMap(ctx, plan.Properties); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("driver_info"),
				"Error Converting Driver Info",
				fmt.Sprintf("Could not convert driver_info to map: %s", err),
			)
		} else {
			createOpts.Properties = properties
		}
	}

	if !plan.DriverInfo.IsNull() && !plan.DriverInfo.IsUnknown() {
		if driverInfo, err := util.DynamicToMap(ctx, plan.DriverInfo); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("driver_info"),
				"Error Converting Driver Info",
				fmt.Sprintf("Could not convert driver_info to map: %s", err),
			)
		} else {
			createOpts.DriverInfo = driverInfo
		}
	}

	if !plan.ExtraData.IsNull() && !plan.ExtraData.IsUnknown() {
		if extra, err := util.DynamicToMap(ctx, plan.Properties); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("extra"),
				"Error Converting Extra Data",
				fmt.Sprintf("Could not convert extra to map: %s", err),
			)
		} else {
			createOpts.Extra = extra
		}
	}

	// Handle other optional fields supported by CreateOpts
	if !plan.ResourceClass.IsNull() && !plan.ResourceClass.IsUnknown() {
		createOpts.ResourceClass = plan.ResourceClass.ValueString()
	}

	if !plan.Owner.IsNull() && !plan.Owner.IsUnknown() {
		createOpts.Owner = plan.Owner.ValueString()
	}

	if !plan.ConductorGroup.IsNull() && !plan.ConductorGroup.IsUnknown() {
		createOpts.ConductorGroup = plan.ConductorGroup.ValueString()
	}

	// Handle boolean fields
	if !plan.Automated.IsNull() && !plan.Automated.IsUnknown() {
		automated := plan.Automated.ValueBool()
		createOpts.AutomatedClean = &automated
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

	// Create the node
	node, err := nodes.Create(ctx, client, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating node",
			fmt.Sprintf("Could not create node: %s", err),
		)
		return
	}

	// Update plan with computed values
	plan.ID = types.StringValue(node.UUID)

	// Read the created node to get all computed fields
	r.readNodeData(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Handle action attributes
	r.handleActionAttributes(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *nodeV1Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state nodeV1ResourceModel

	// Get current state
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Available.IsNull() || state.Available.IsUnknown() {
		state.Available = types.BoolValue(DefaultAvailable)
	}

	if state.Manage.IsNull() || state.Manage.IsUnknown() {
		state.Manage = types.BoolValue(DefaultManage)
	}

	if state.Inspect.IsNull() || state.Inspect.IsUnknown() {
		state.Inspect = types.BoolValue(DefaultInspect)
	}

	if state.Clean.IsNull() || state.Clean.IsUnknown() {
		state.Clean = types.BoolValue(DefaultClean)
	}

	// Read the node from the API
	r.readNodeData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *nodeV1Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan nodeV1ResourceModel
	var state nodeV1ResourceModel

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

	// Prepare update options
	updateOpts := nodes.UpdateOpts{}

	// Check for changes and build update operations
	if !plan.Name.Equal(state.Name) {
		updateOpts = append(updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/name",
			Value: plan.Name.ValueString(),
		})
	}

	// Add other update operations as needed...
	// This is a simplified version - you would need to handle all updatable fields

	if len(updateOpts) > 0 {
		// Get the ironic client
		client, err := r.clients.GetIronicClient()
		if err != nil {
			resp.Diagnostics.AddError(
				"Error getting Ironic client",
				fmt.Sprintf("Could not get Ironic client: %s", err),
			)
			return
		}

		// Perform the update
		_, err = nodes.Update(ctx, client, state.ID.ValueString(), updateOpts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating node",
				fmt.Sprintf("Could not update node %s: %s", state.ID.ValueString(), err),
			)
			return
		}
	}

	// Read the updated node
	r.readNodeData(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Handle action attributes
	r.handleActionAttributes(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set updated state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *nodeV1Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state nodeV1ResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete the node
	client, err := r.clients.GetIronicClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting Ironic client",
			fmt.Sprintf("Could not get Ironic client: %s", err),
		)
		return
	}

	err = nodes.Delete(ctx, client, state.ID.ValueString()).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting node",
			fmt.Sprintf("Could not delete node %s: %s", state.ID.ValueString(), err),
		)
		return
	}
}

func (r *nodeV1Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper function to read node data from the API and populate the model.
func (r *nodeV1Resource) readNodeData(
	ctx context.Context,
	model *nodeV1ResourceModel,
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

	node, err := nodes.Get(ctx, client, model.ID.ValueString()).Extract()
	if err != nil {
		diagnostics.AddError(
			"Error reading node",
			fmt.Sprintf("Could not read node %s: %s", model.ID.ValueString(), err),
		)
		return
	}

	// Map the API response to the model
	model.ID = types.StringValue(node.UUID)
	model.Name = types.StringValue(node.Name)
	model.Driver = types.StringValue(node.Driver)
	model.NetworkInterface = types.StringValue(node.NetworkInterface)
	model.BootInterface = types.StringValue(node.BootInterface)
	model.ConsoleInterface = types.StringValue(node.ConsoleInterface)
	model.DeployInterface = types.StringValue(node.DeployInterface)
	model.InspectInterface = types.StringValue(node.InspectInterface)
	model.ManagementInterface = types.StringValue(node.ManagementInterface)
	model.PowerInterface = types.StringValue(node.PowerInterface)
	model.RAIDInterface = types.StringValue(node.RAIDInterface)
	model.RescueInterface = types.StringValue(node.RescueInterface)
	model.StorageInterface = types.StringValue(node.StorageInterface)
	model.VendorInterface = types.StringValue(node.VendorInterface)
	model.BIOSInterface = types.StringValue(node.BIOSInterface)
	model.FirmwareInterface = types.StringValue(node.FirmwareInterface)

	model.Automated = types.BoolValue(*node.AutomatedClean)
	model.Protected = types.BoolValue(node.Protected)
	model.Maintenance = types.BoolValue(node.Maintenance)
	model.MaintenanceReason = types.StringValue(node.MaintenanceReason)

	model.InstanceUUID = types.StringValue(node.InstanceUUID)
	model.ResourceClass = types.StringValue(node.ResourceClass)
	model.ProvisionState = types.StringValue(node.ProvisionState)
	model.PowerState = types.StringValue(node.PowerState)
	model.TargetProvisionState = types.StringValue(node.TargetProvisionState)
	model.TargetPowerState = types.StringValue(node.TargetPowerState)
	model.LastError = types.StringValue(node.LastError)
	model.Owner = types.StringValue(node.Owner)
	model.Lessee = types.StringValue(node.Lessee)
	model.Conductor = types.StringValue(node.Conductor)
	model.ConductorGroup = types.StringValue(node.ConductorGroup)
	model.AllocationUUID = types.StringValue(node.AllocationUUID)
	model.Chassis = types.StringValue(node.ChassisUUID)
	model.Fault = types.StringValue(node.Fault)

	// Handle time fields
	if !node.CreatedAt.IsZero() {
		model.Created = types.StringValue(node.CreatedAt.Format("2006-01-02T15:04:05Z"))
	}
	if !node.UpdatedAt.IsZero() {
		model.Updated = types.StringValue(node.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	}

	// Handle map fields - this is simplified, you may need more complex handling
	if node.Properties != nil {
		if properties, err := util.MapToDynamic(ctx, node.Properties); err != nil {
			diagnostics.AddAttributeError(
				path.Root("properties"),
				"Error Converting Properties",
				fmt.Sprintf("Could not convert properties to dynamic: %s", err),
			)
		} else {
			model.Properties = properties
		}
	}

	if node.DriverInfo != nil {
		if driverInfo, err := util.MapToDynamic(ctx, node.DriverInfo); err != nil {
			diagnostics.AddAttributeError(
				path.Root("driver_info"),
				"Error Converting Driver Info",
				fmt.Sprintf("Could not convert driver_info to dynamic: %s", err),
			)
		} else {
			model.DriverInfo = driverInfo
		}
	}

	if node.InstanceInfo != nil {
		instanceInfo, err := util.MapToDynamic(ctx, node.InstanceInfo)
		if err != nil {
			diagnostics.AddAttributeError(
				path.Root("instance_info"),
				"Error Converting Instance Info",
				fmt.Sprintf("Could not convert instance_info to dynamic: %s", err),
			)
		} else {
			model.InstanceInfo = instanceInfo
		}
	}

	if len(node.Extra) > 0 {
		if extra, err := util.MapToDynamic(ctx, node.Extra); err != nil {
			diagnostics.AddAttributeError(
				path.Root("extra"),
				"Error Converting Extra Data",
				fmt.Sprintf("Could not convert extra to dynamic: %s", err),
			)
		} else {
			model.ExtraData = extra
		}
	}

	if len(node.CleanStep) > 0 {
		if cleanStep, err := util.MapToDynamic(ctx, node.CleanStep); err != nil {
			diagnostics.AddAttributeError(
				path.Root("clean_step"),
				"Error Converting Clean Step",
				fmt.Sprintf("Could not convert clean_step to dynamic: %s", err),
			)
		} else {
			model.CleanStep = cleanStep
		}
	}

	// Handle other complex fields as needed...
}

// handleActionAttributes handles the action attributes (clean, inspect, available, manage).
// These attributes trigger state changes but are not persisted in the API.
func (r *nodeV1Resource) handleActionAttributes(
	ctx context.Context,
	model *nodeV1ResourceModel,
	diagnostics *diag.Diagnostics,
) {
	nodeUUID := model.ID.ValueString()

	// Get the ironic client
	client, err := r.clients.GetIronicClient()
	if err != nil {
		diagnostics.AddError(
			"Error getting Ironic client",
			fmt.Sprintf("Could not get Ironic client: %s", err),
		)
		return
	}

	// Handle clean action
	if !model.Clean.IsNull() && model.Clean.ValueBool() {
		err := ChangeProvisionStateToTarget(
			ctx,
			client,
			nodeUUID,
			nodes.TargetClean,
			nil,
			nil,
			nil,
		)
		if err != nil {
			diagnostics.AddError(
				"Error cleaning node",
				fmt.Sprintf("Could not clean node %s: %s", nodeUUID, err),
			)
			return
		}
		// Reset the action attribute to null after triggering
		model.Clean = types.BoolNull()
	}

	// Handle inspect action
	if !model.Inspect.IsNull() && model.Inspect.ValueBool() {
		err := ChangeProvisionStateToTarget(
			ctx,
			client,
			nodeUUID,
			nodes.TargetInspect,
			nil,
			nil,
			nil,
		)
		if err != nil {
			diagnostics.AddError(
				"Error inspecting node",
				fmt.Sprintf("Could not inspect node %s: %s", nodeUUID, err),
			)
			return
		}
		// Reset the action attribute to null after triggering
		model.Inspect = types.BoolNull()
	}

	// Handle available action
	if !model.Available.IsNull() && model.Available.ValueBool() {
		err := ChangeProvisionStateToTarget(
			ctx,
			client,
			nodeUUID,
			nodes.TargetProvide,
			nil,
			nil,
			nil,
		)
		if err != nil {
			diagnostics.AddError(
				"Error making node available",
				fmt.Sprintf("Could not make node %s available: %s", nodeUUID, err),
			)
			return
		}
		// Reset the action attribute to null after triggering
		model.Available = types.BoolNull()
	}

	// Handle manage action
	if !model.Manage.IsNull() && model.Manage.ValueBool() {
		err := ChangeProvisionStateToTarget(
			ctx,
			client,
			nodeUUID,
			nodes.TargetManage,
			nil,
			nil,
			nil,
		)
		if err != nil {
			diagnostics.AddError(
				"Error managing node",
				fmt.Sprintf("Could not manage node %s: %s", nodeUUID, err),
			)
			return
		}
		// Reset the action attribute to null after triggering
		model.Manage = types.BoolNull()
	}
}
