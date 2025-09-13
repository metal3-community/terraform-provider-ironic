package ironic

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/metal3-community/terraform-provider-ironic/ironic/util"
)

const (
	DefaultAvailable   = true
	DefaultManage      = true
	DefaultInspect     = true
	DefaultClean       = false
	DefaultMaintenance = false
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &NodeResource{}
	_ resource.ResourceWithConfigure   = &NodeResource{}
	_ resource.ResourceWithImportState = &NodeResource{}
)

// NodeResource defines the resource implementation.
type NodeResource struct {
	meta *Meta
}

// NodeResourceModel describes the resource data model.
type NodeResourceModel struct {
	ID                   types.String      `tfsdk:"id"`
	Name                 types.String      `tfsdk:"name"`
	Namespace            types.String      `tfsdk:"namespace"`
	FullName             types.String      `tfsdk:"full_name"`
	AllocationUUID       types.String      `tfsdk:"allocation_uuid"`
	Automated            types.Bool        `tfsdk:"automated_clean"`
	Available            types.Bool        `tfsdk:"available"`
	BIOSInterface        types.String      `tfsdk:"bios_interface"`
	BootInterface        types.String      `tfsdk:"boot_interface"`
	Chassis              types.String      `tfsdk:"chassis_uuid"`
	Clean                types.Bool        `tfsdk:"clean"`
	CleanStep            types.Dynamic     `tfsdk:"clean_step"`
	Conductor            types.String      `tfsdk:"conductor"`
	ConductorGroup       types.String      `tfsdk:"conductor_group"`
	ConsoleInterface     types.String      `tfsdk:"console_interface"`
	DeployInterface      types.String      `tfsdk:"deploy_interface"`
	DeployStep           types.Dynamic     `tfsdk:"deploy_step"`
	Driver               types.String      `tfsdk:"driver"`
	DriverInfo           types.Dynamic     `tfsdk:"driver_info"`
	ExtraData            types.Dynamic     `tfsdk:"extra"`
	Fault                types.String      `tfsdk:"fault"`
	FirmwareInterface    types.String      `tfsdk:"firmware_interface"`
	Inspect              types.Bool        `tfsdk:"inspect"`
	InspectInterface     types.String      `tfsdk:"inspect_interface"`
	InstanceInfo         types.Dynamic     `tfsdk:"instance_info"`
	InstanceUUID         types.String      `tfsdk:"instance_uuid"`
	LastError            types.String      `tfsdk:"last_error"`
	Lessee               types.String      `tfsdk:"lessee"`
	Maintenance          types.Bool        `tfsdk:"maintenance"`
	MaintenanceReason    types.String      `tfsdk:"maintenance_reason"`
	Manage               types.Bool        `tfsdk:"manage"`
	ManagementInterface  types.String      `tfsdk:"management_interface"`
	NetworkInterface     types.String      `tfsdk:"network_interface"`
	Owner                types.String      `tfsdk:"owner"`
	Ports                []NodePortModel   `tfsdk:"ports"`
	PowerInterface       types.String      `tfsdk:"power_interface"`
	PowerState           types.String      `tfsdk:"power_state"`
	Properties           types.Dynamic     `tfsdk:"properties"`
	Protected            types.Bool        `tfsdk:"protected"`
	ProvisionState       types.String      `tfsdk:"provision_state"`
	RAIDInterface        types.String      `tfsdk:"raid_interface"`
	RescueInterface      types.String      `tfsdk:"rescue_interface"`
	ResourceClass        types.String      `tfsdk:"resource_class"`
	StorageInterface     types.String      `tfsdk:"storage_interface"`
	TargetPowerState     types.String      `tfsdk:"target_power_state"`
	TargetProvisionState types.String      `tfsdk:"target_provision_state"`
	VendorInterface      types.String      `tfsdk:"vendor_interface"`
	Updated              timetypes.RFC3339 `tfsdk:"updated_at"`
	Created              timetypes.RFC3339 `tfsdk:"created_at"`
	InspectionStarted    timetypes.RFC3339 `tfsdk:"inspection_started_at"`
	InspectionFinished   timetypes.RFC3339 `tfsdk:"inspection_finished_at"`
	ProvisionUpdated     timetypes.RFC3339 `tfsdk:"provision_updated_at"`
}

// NodePortModel describes the port data model within the node.
type NodePortModel struct {
	UUID       types.String `tfsdk:"uuid"`
	MACAddress types.String `tfsdk:"mac_address"`
}

func NewNodeResource() resource.Resource {
	return &NodeResource{}
}

func (r *NodeResource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (r *NodeResource) Schema(
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
			"namespace": schema.StringAttribute{
				MarkdownDescription: "The namespace of the node.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"full_name": schema.StringAttribute{
				MarkdownDescription: "The full name of the node, combining name and namespace.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
				Computed:            true,
			},
			"maintenance_reason": schema.StringAttribute{
				MarkdownDescription: "The reason for putting the node in maintenance mode.",
				Computed:            true,
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
				CustomType:          timetypes.RFC3339Type{},
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node was last updated.",
				CustomType:          timetypes.RFC3339Type{},
				Computed:            true,
			},
			"inspection_started_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node inspection started.",
				CustomType:          timetypes.RFC3339Type{},
				Computed:            true,
			},
			"inspection_finished_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node inspection finished.",
				CustomType:          timetypes.RFC3339Type{},
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"provision_updated_at": schema.StringAttribute{
				MarkdownDescription: "The timestamp when the node provision was last updated.",
				CustomType:          timetypes.RFC3339Type{},
				Computed:            true,
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

func (r *NodeResource) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	clients, ok := req.ProviderData.(*Meta)
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

	r.meta = clients
}

func (r *NodeResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan NodeResourceModel

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
		// Build the node name: combine namespace and name if namespace is provided
		nodeName := plan.Name.ValueString()
		if !plan.Namespace.IsNull() && !plan.Namespace.IsUnknown() &&
			plan.Namespace.ValueString() != "" {
			nodeName = plan.Namespace.ValueString() + "~" + plan.Name.ValueString()
		}
		createOpts.Name = nodeName
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
				path.Root("properties"),
				"Error Converting Properties",
				fmt.Sprintf("Could not convert properties to map: %s", err),
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

	// Create the node
	node, err := nodes.Create(ctx, r.meta.Client, createOpts).Extract()
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

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	// Handle action attributes
	r.handleActionAttributes(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *NodeResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state NodeResourceModel

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

	if state.Maintenance.IsNull() || state.Maintenance.IsUnknown() {
		state.Maintenance = types.BoolValue(false)
	}

	if state.Inspect.IsNull() || state.Inspect.IsUnknown() {
		state.Inspect = types.BoolValue(DefaultInspect)
	}

	if state.Clean.IsNull() || state.Clean.IsUnknown() {
		state.Clean = types.BoolValue(DefaultClean)
	}

	if state.CleanStep.IsNull() || state.CleanStep.IsUnknown() {
		state.CleanStep = types.DynamicNull()
	}

	if state.DeployStep.IsNull() || state.DeployStep.IsUnknown() {
		state.DeployStep = types.DynamicNull()
	}

	if state.ResourceClass.IsNull() || state.ResourceClass.IsUnknown() {
		state.ResourceClass = types.StringNull()
	}

	// Read the node from the API
	r.readNodeData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *NodeResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan NodeResourceModel
	var state NodeResourceModel

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
	// Handle name changes (including namespace changes)
	nameChanged := !plan.Name.Equal(state.Name) || !plan.Namespace.Equal(state.Namespace)
	if nameChanged {
		// Build the node name: combine namespace and name if namespace is provided
		fullName := plan.Name.ValueString()
		if !plan.Namespace.IsNull() &&
			!plan.Namespace.IsUnknown() &&
			plan.Namespace.ValueString() != "" {
			fullName = plan.Namespace.ValueString() + "~" + plan.Name.ValueString()
		}
		updateOpts = append(updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/name",
			Value: fullName,
		})
		state.FullName = types.StringValue(fullName)
		state.Name = plan.Name
		state.Namespace = plan.Namespace
	}

	// Handle driver changes
	if !plan.Driver.Equal(state.Driver) {
		updateOpts = append(updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/driver",
			Value: plan.Driver.ValueString(),
		})
	}

	// Handle interface changes
	r.addInterfaceUpdateOps(&updateOpts, &plan, &state)

	// Handle dynamic type changes
	r.addDynamicUpdateOps(ctx, &updateOpts, &plan, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Handle string field changes
	r.addStringUpdateOps(&updateOpts, &plan, &state)

	// Handle boolean field changes
	r.addBooleanUpdateOps(&updateOpts, &plan, &state)

	if len(updateOpts) > 0 {

		// Perform the update
		_, err := nodes.Update(ctx, r.meta.Client, state.ID.ValueString(), updateOpts).Extract()
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

func (r *NodeResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state NodeResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := nodes.Delete(ctx, r.meta.Client, state.ID.ValueString()).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting node",
			fmt.Sprintf("Could not delete node %s: %s", state.ID.ValueString(), err),
		)
		return
	}
}

func (r *NodeResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper function to read node data from the API and populate the model.
func (r *NodeResource) readNodeData(
	ctx context.Context,
	model *NodeResourceModel,
	diagnostics *diag.Diagnostics,
) {
	node, err := nodes.Get(ctx, r.meta.Client, model.ID.ValueString()).Extract()
	if err != nil {
		diagnostics.AddError(
			"Error reading node",
			fmt.Sprintf("Could not read node %s: %s", model.ID.ValueString(), err),
		)
		return
	}

	// Map the API response to the model
	model.AllocationUUID = types.StringValue(node.AllocationUUID)
	model.Automated = types.BoolValue(*node.AutomatedClean)
	model.BIOSInterface = types.StringValue(node.BIOSInterface)
	model.BootInterface = types.StringValue(node.BootInterface)
	model.Chassis = types.StringValue(node.ChassisUUID)
	model.Conductor = types.StringValue(node.Conductor)
	model.ConductorGroup = types.StringValue(node.ConductorGroup)
	model.ConsoleInterface = types.StringValue(node.ConsoleInterface)
	model.DeployInterface = types.StringValue(node.DeployInterface)
	model.Driver = types.StringValue(node.Driver)
	model.Fault = types.StringValue(node.Fault)
	model.FirmwareInterface = types.StringValue(node.FirmwareInterface)
	model.ID = types.StringValue(node.UUID)
	model.InspectInterface = types.StringValue(node.InspectInterface)
	model.InstanceUUID = types.StringValue(node.InstanceUUID)
	model.LastError = types.StringValue(node.LastError)
	model.Lessee = types.StringValue(node.Lessee)
	model.Maintenance = types.BoolValue(node.Maintenance)
	model.MaintenanceReason = types.StringValue(node.MaintenanceReason)
	model.ManagementInterface = types.StringValue(node.ManagementInterface)

	// Handle name parsing for Metal3 namespace pattern
	// Set full_name from API response
	model.FullName = types.StringValue(node.Name)

	// Parse namespace and name from full_name if it contains the ~ delimiter
	if strings.Contains(node.Name, "~") {
		parts := strings.SplitN(node.Name, "~", 2)
		if len(parts) == 2 {
			model.Namespace = types.StringValue(parts[0])
			model.Name = types.StringValue(parts[1])
			// Don't overwrite the user-configured name, keep the parsed name separate
			// The Name field should remain as the user configured it
		}
	} else {
		// No namespace delimiter, clear namespace and use the full name
		model.Name = types.StringValue(node.Name)
		model.Namespace = types.StringNull()
	}

	model.NetworkInterface = types.StringValue(node.NetworkInterface)
	model.Owner = types.StringValue(node.Owner)
	model.PowerInterface = types.StringValue(node.PowerInterface)
	model.PowerState = types.StringValue(node.PowerState)
	model.Protected = types.BoolValue(node.Protected)
	model.ProvisionState = types.StringValue(node.ProvisionState)
	model.RAIDInterface = types.StringValue(node.RAIDInterface)
	model.RescueInterface = types.StringValue(node.RescueInterface)
	model.ResourceClass = types.StringValue(node.ResourceClass)
	model.StorageInterface = types.StringValue(node.StorageInterface)
	model.TargetPowerState = types.StringValue(node.TargetPowerState)
	model.TargetProvisionState = types.StringValue(node.TargetProvisionState)
	model.VendorInterface = types.StringValue(node.VendorInterface)

	model.Created = timeTypeOrNull(node.CreatedAt)
	model.Updated = timeTypeOrNull(node.UpdatedAt)
	model.ProvisionUpdated = timeTypeOrNull(node.ProvisionUpdatedAt)
	model.InspectionStarted = timetypes.NewRFC3339TimePointerValue(node.InspectionStartedAt)
	model.InspectionFinished = timetypes.NewRFC3339TimePointerValue(node.InspectionFinishedAt)

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
	} else {
		model.Properties = types.DynamicNull()
	}

	if node.DeployStep != nil {
		if deployStep, err := util.MapToDynamic(ctx, node.DeployStep); err != nil {
			diagnostics.AddAttributeError(
				path.Root("deployStep"),
				"Error Converting DeployStep",
				fmt.Sprintf("Could not convert deployStep to dynamic: %s", err),
			)
		} else {
			model.DeployStep = deployStep
		}
	} else {
		model.DeployStep = types.DynamicNull()
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
	} else {
		model.DriverInfo = types.DynamicNull()
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
	} else {
		model.InstanceInfo = types.DynamicNull()
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
	} else {
		model.ExtraData = types.DynamicNull()
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
	} else {
		model.CleanStep = types.DynamicNull()
	}

	// Handle other complex fields as needed...
}

func timeTypeOrNull(v time.Time) timetypes.RFC3339 {
	if !v.IsZero() {
		return timetypes.NewRFC3339TimeValue(v)
	} else {
		return timetypes.NewRFC3339Null()
	}
}

// addDynamicUpdateOps handles changes for dynamic attributes.
func (r *NodeResource) addDynamicUpdateOps(
	ctx context.Context,
	updateOpts *nodes.UpdateOpts,
	plan *NodeResourceModel,
	state *NodeResourceModel,
	diagnostics *diag.Diagnostics,
) {
	r.addDynamicUpdateOpsForField(
		ctx,
		updateOpts,
		diagnostics,
		plan.Properties,
		state.Properties,
		"properties",
	)
	r.addDynamicUpdateOpsForField(
		ctx,
		updateOpts,
		diagnostics,
		plan.DriverInfo,
		state.DriverInfo,
		"driver_info",
	)
	r.addDynamicUpdateOpsForField(
		ctx,
		updateOpts,
		diagnostics,
		plan.ExtraData,
		state.ExtraData,
		"extra",
	)
	r.addDynamicUpdateOpsForField(
		ctx,
		updateOpts,
		diagnostics,
		plan.InstanceInfo,
		state.InstanceInfo,
		"instance_info",
	)
}

// addDynamicUpdateOpsForField handles changes for a single dynamic attribute.
func (r *NodeResource) addDynamicUpdateOpsForField(
	ctx context.Context,
	updateOpts *nodes.UpdateOpts,
	diagnostics *diag.Diagnostics,
	planValue, stateValue types.Dynamic,
	fieldName string,
) {
	if planValue.Equal(stateValue) {
		return
	}

	basePath := "/" + fieldName

	planMap, err := util.DynamicToMap(ctx, planValue)
	if err != nil {
		diagnostics.AddAttributeError(
			path.Root(fieldName),
			"Error Converting Plan Value",
			fmt.Sprintf("Could not convert %s to map: %s", fieldName, err),
		)
		return
	}

	stateMap, err := util.DynamicToMap(ctx, stateValue)
	if err != nil {
		diagnostics.AddAttributeError(
			path.Root(fieldName),
			"Error Converting State Value",
			fmt.Sprintf("Could not convert %s to map: %s", fieldName, err),
		)
		return
	}

	// If one of the maps is nil (e.g. attribute is null or empty),
	// we replace the whole object.
	if planMap == nil || stateMap == nil {
		*updateOpts = append(*updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  basePath,
			Value: planMap, // if planMap is nil, this will set the field to null.
		})
		return
	}

	// Compare maps and generate update operations
	for key, planVal := range planMap {
		stateVal, ok := stateMap[key]
		if !ok {
			// Add operation
			*updateOpts = append(*updateOpts, nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  fmt.Sprintf("%s/%s", basePath, key),
				Value: planVal,
			})
		} else if !reflect.DeepEqual(planVal, stateVal) {
			// Replace operation
			*updateOpts = append(*updateOpts, nodes.UpdateOperation{
				Op:    nodes.ReplaceOp,
				Path:  fmt.Sprintf("%s/%s", basePath, key),
				Value: planVal,
			})
		}
	}

	for key := range stateMap {
		if _, ok := planMap[key]; !ok {
			// Remove operation
			*updateOpts = append(*updateOpts, nodes.UpdateOperation{
				Op:   nodes.RemoveOp,
				Path: fmt.Sprintf("%s/%s", basePath, key),
			})
		}
	}
}

// addInterfaceUpdateOps handles changes for interface attributes.
func (r *NodeResource) addInterfaceUpdateOps(
	updateOpts *nodes.UpdateOpts,
	plan *NodeResourceModel,
	state *NodeResourceModel,
) {
	// TODO: Implement interface update logic
}

// addStringUpdateOps handles changes for string attributes.
func (r *NodeResource) addStringUpdateOps(
	updateOpts *nodes.UpdateOpts,
	plan *NodeResourceModel,
	state *NodeResourceModel,
) {
	if !plan.BIOSInterface.Equal(state.BIOSInterface) {
		*updateOpts = append(*updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/bios_interface",
			Value: plan.BIOSInterface.ValueString(),
		})
	}
	if !plan.ResourceClass.Equal(state.ResourceClass) {
		*updateOpts = append(*updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/resource_class",
			Value: plan.ResourceClass.ValueString(),
		})
	}
}

// addBooleanUpdateOps handles changes for boolean attributes.
func (r *NodeResource) addBooleanUpdateOps(
	updateOpts *nodes.UpdateOpts,
	plan *NodeResourceModel,
	state *NodeResourceModel,
) {
	// TODO: Implement boolean update logic
}

// handleActionAttributes handles the action attributes (clean, inspect, available, manage).
// These attributes trigger state changes but are not persisted in the API.
func (r *NodeResource) handleActionAttributes(
	ctx context.Context,
	model *NodeResourceModel,
	diagnostics *diag.Diagnostics,
) {
	nodeUUID := model.ID.ValueString()

	nodeInfo, err := nodes.Get(ctx, r.meta.Client, nodeUUID).Extract()
	if err != nil {
		diagnostics.AddError(
			"Error getting node information",
			fmt.Sprintf("Could not get node %s: %s", nodeUUID, err),
		)
		return
	}

	// Handle clean action
	if !model.Clean.IsNull() && model.Clean.ValueBool() {
		err := ChangeProvisionStateToTarget(
			ctx,
			r.meta.Client,
			nodeUUID,
			nodes.TargetClean,
			nil,
			nil,
			nil,
			nil, // serviceSteps
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
		if model.InspectionFinished.IsNull() || model.InspectionFinished.IsUnknown() {
			if nodeInfo.ProvisionState == string(nodes.Manageable) {
				err := ChangeProvisionStateToTarget(
					ctx,
					r.meta.Client,
					nodeUUID,
					nodes.TargetInspect,
					nil,
					nil,
					nil,
					nil, // serviceSteps
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
		}
	}

	// Handle available action
	if !model.Available.IsNull() && model.Available.ValueBool() {
		err := ChangeProvisionStateToTarget(
			ctx,
			r.meta.Client,
			nodeUUID,
			nodes.TargetProvide,
			nil,
			nil,
			nil,
			nil, // serviceSteps
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
			r.meta.Client,
			nodeUUID,
			nodes.TargetManage,
			nil,
			nil,
			nil,
			nil, // serviceSteps
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
