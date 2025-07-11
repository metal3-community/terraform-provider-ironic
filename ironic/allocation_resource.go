package ironic

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/allocations"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/dynamicplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/metal3-community/terraform-provider-ironic/ironic/util"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &allocationV1Resource{}
	_ resource.ResourceWithConfigure   = &allocationV1Resource{}
	_ resource.ResourceWithImportState = &allocationV1Resource{}
)

// allocationV1Resource defines the resource implementation.
type allocationV1Resource struct {
	meta *Meta
}

// allocationV1ResourceModel describes the resource data model.
type allocationV1ResourceModel struct {
	ID             types.String  `tfsdk:"id"`
	Name           types.String  `tfsdk:"name"`
	ResourceClass  types.String  `tfsdk:"resource_class"`
	CandidateNodes types.List    `tfsdk:"candidate_nodes"`
	Node           types.String  `tfsdk:"node"`
	Traits         types.List    `tfsdk:"traits"`
	Extra          types.Dynamic `tfsdk:"extra"`
	NodeUUID       types.String  `tfsdk:"node_uuid"`
	State          types.String  `tfsdk:"state"`
	LastError      types.String  `tfsdk:"last_error"`
}

func NewAllocationV1Resource() resource.Resource {
	return &allocationV1Resource{}
}

func (r *allocationV1Resource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_allocation"
}

func (r *allocationV1Resource) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Ironic allocation resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The UUID of the allocation.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the allocation.",
				Optional:            true,
			},
			"resource_class": schema.StringAttribute{
				MarkdownDescription: "The resource class required for this allocation.",
				Required:            true,
			},
			"candidate_nodes": schema.ListAttribute{
				MarkdownDescription: "List of candidate node UUIDs for this allocation.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.MatchRoot("node")),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"node": schema.StringAttribute{
				MarkdownDescription: `Backfill this allocation from the provided node that has already been deployed. 
				Bypasses the normal allocation process. Conflicts with "candidate_nodes".`,
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("candidate_nodes")),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"traits": schema.ListAttribute{
				MarkdownDescription: "List of required traits for this allocation.",
				ElementType:         types.StringType,
				Optional:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"extra": schema.DynamicAttribute{
				MarkdownDescription: "Extra metadata for the allocation.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.RequiresReplace(),
				},
			},
			"node_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the node allocated to this allocation.",
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "The current state of the allocation.",
				Computed:            true,
			},
			"last_error": schema.StringAttribute{
				MarkdownDescription: "The last error message for the allocation.",
				Computed:            true,
			},
		},
	}
}

func (r *allocationV1Resource) Configure(
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

func (r *allocationV1Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan allocationV1ResourceModel

	// Get the plan
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prepare create options
	createOpts := allocations.CreateOpts{}

	// Set optional fields
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		createOpts.Name = plan.Name.ValueString()
	}

	createOpts.ResourceClass = plan.ResourceClass.ValueString()

	// Handle candidate nodes list
	if !plan.CandidateNodes.IsNull() && !plan.CandidateNodes.IsUnknown() {
		var candidateNodes []string
		diags = plan.CandidateNodes.ElementsAs(ctx, &candidateNodes, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.CandidateNodes = candidateNodes
	}

	if !plan.Node.IsNull() && !plan.Node.IsUnknown() {
		// If a node is specified, it must be a single UUID
		if plan.CandidateNodes.IsNull() || plan.CandidateNodes.IsUnknown() ||
			len(plan.CandidateNodes.Elements()) == 0 {
			backfillOpts := struct {
				*allocations.CreateOpts
				Node string `json:"node"`
			}{
				&createOpts,
				plan.Node.ValueString(),
			}
			reqBody, err := gophercloud.BuildRequestBody(backfillOpts, "")
			if err != nil {
				resp.Diagnostics.AddAttributeError(
					path.Root("node"),
					"Error Building Request Body",
					fmt.Sprintf("Could not build request body for node backfill: %s", err),
				)
				return
			}
			res, err := r.meta.Client.Post(
				ctx,
				r.meta.Client.ServiceURL("allocations"),
				reqBody,
				&reqBody,
				nil,
			)
			_, _, err = gophercloud.ParseResponse(res, err)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error creating allocation with node backfill",
					fmt.Sprintf("Could not create allocation with node backfill: %s", err),
				)
				return
			}
		} else {
			resp.Diagnostics.AddAttributeError(
				path.Root("node"),
				"Node Conflicts with Candidate Nodes",
				"Cannot specify both 'node' and 'candidate_nodes'. Please choose one.",
			)
			return
		}
	} else if !plan.CandidateNodes.IsNull() && !plan.CandidateNodes.IsUnknown() {
		// If candidate nodes are specified, ensure they are not empty
		if len(plan.CandidateNodes.Elements()) == 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root("candidate_nodes"),
				"Empty Candidate Nodes List",
				"Candidate nodes list cannot be empty. Please provide at least one candidate node UUID.",
			)
			return
		}
		// If candidate nodes are provided, we don't set NodeUUID
	} else {
		// If neither candidate nodes nor node is specified, we cannot create the allocation
		resp.Diagnostics.AddAttributeError(
			path.Root("candidate_nodes"),
			"Missing Candidate Nodes or Node",
			"At least one of 'candidate_nodes' or 'node' must be specified to create an allocation.",
		)
		return
	}

	// Handle traits list
	if !plan.Traits.IsNull() && !plan.Traits.IsUnknown() {
		var traits []string
		diags = plan.Traits.ElementsAs(ctx, &traits, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.Traits = traits
	}

	// Handle extra data
	if !plan.Extra.IsNull() && !plan.Extra.IsUnknown() {
		if extra, err := util.DynamicToStringMap(ctx, plan.Extra); err != nil {
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

	// Create the allocation
	allocation, err := allocations.Create(ctx, r.meta.Client, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating allocation",
			fmt.Sprintf("Could not create allocation: %s", err),
		)
		return
	}

	// Update plan with computed values
	plan.ID = types.StringValue(allocation.UUID)

	// Wait for allocation to complete
	if err := r.waitForAllocationComplete(ctx, allocation.UUID, &plan, &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for allocation completion",
			fmt.Sprintf("Could not wait for allocation to complete: %s", err),
		)
		// Clean up the allocation if it failed
		_ = allocations.Delete(ctx, r.meta.Client, allocation.UUID).ExtractErr()
		return
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *allocationV1Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state allocationV1ResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read the allocation from the API
	r.readAllocationData(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *allocationV1Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	// Allocations do not support updates - they are immutable
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Allocations are immutable and cannot be updated. Any changes require replacement.",
	)
}

func (r *allocationV1Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state allocationV1ResourceModel

	// Get current state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if allocation exists before trying to delete
	_, err := allocations.Get(ctx, r.meta.Client, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Already deleted
			return
		}
		resp.Diagnostics.AddError(
			"Error checking allocation",
			fmt.Sprintf("Could not check allocation %s: %s", state.ID.ValueString(), err),
		)
		return
	}

	// Delete the allocation
	err = allocations.Delete(ctx, r.meta.Client, state.ID.ValueString()).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting allocation",
			fmt.Sprintf("Could not delete allocation %s: %s", state.ID.ValueString(), err),
		)
		return
	}
}

func (r *allocationV1Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	// Set the id attribute to the import identifier
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper function to wait for allocation completion.
func (r *allocationV1Resource) waitForAllocationComplete(
	ctx context.Context,
	allocationID string,
	model *allocationV1ResourceModel,
	diagnostics *diag.Diagnostics,
) error {
	timeout := 1 * time.Minute
	checkInterval := 2 * time.Second

	for {
		// Read current allocation state
		r.readAllocationData(ctx, model, diagnostics)
		if diagnostics.HasError() {
			return fmt.Errorf("error reading allocation during wait")
		}

		state := model.State.ValueString()
		tflog.Debug(ctx, "Requested allocation; current state", map[string]any{
			"allocation_id": allocationID,
			"state":         state,
		})

		switch state {
		case "allocating":
			time.Sleep(checkInterval)
			checkInterval += 2 * time.Second
			timeout -= checkInterval
			if timeout < 0 {
				return fmt.Errorf("timed out waiting for allocation")
			}
		case "error":
			errorMsg := model.LastError.ValueString()
			return fmt.Errorf("error creating allocation: %s", errorMsg)
		default:
			// Allocation completed (active, etc.)
			return nil
		}
	}
}

// Helper function to read allocation data from the API and populate the model.
func (r *allocationV1Resource) readAllocationData(
	ctx context.Context,
	model *allocationV1ResourceModel,
	diagnostics *diag.Diagnostics,
) {
	allocation, err := allocations.Get(ctx, r.meta.Client, model.ID.ValueString()).Extract()
	if err != nil {
		diagnostics.AddError(
			"Error reading allocation",
			fmt.Sprintf("Could not read allocation %s: %s", model.ID.ValueString(), err),
		)
		return
	}

	// Map the API response to the model
	model.ID = types.StringValue(allocation.UUID)
	model.Name = types.StringValue(allocation.Name)
	model.ResourceClass = types.StringValue(allocation.ResourceClass)
	model.NodeUUID = types.StringValue(allocation.NodeUUID)
	model.State = types.StringValue(allocation.State)

	// Handle last_error - set to null if empty string
	if allocation.LastError == "" {
		model.LastError = types.StringNull()
	} else {
		model.LastError = types.StringValue(allocation.LastError)
	}

	// Handle node field - set to the allocated node UUID if allocation is complete
	if allocation.NodeUUID != "" {
		model.Node = types.StringValue(allocation.NodeUUID)
	} else {
		model.Node = types.StringNull()
	}

	// Handle candidate nodes list
	if len(allocation.CandidateNodes) > 0 {
		var candidateNodesValues []attr.Value
		for _, node := range allocation.CandidateNodes {
			candidateNodesValues = append(candidateNodesValues, types.StringValue(node))
		}
		candidateNodesList, diags := types.ListValue(types.StringType, candidateNodesValues)
		diagnostics.Append(diags...)
		if diagnostics.HasError() {
			return
		}
		model.CandidateNodes = candidateNodesList
	} else {
		model.CandidateNodes = types.ListNull(types.StringType)
	}

	// Handle traits list
	if len(allocation.Traits) > 0 {
		traitsValues := make([]attr.Value, len(allocation.Traits))
		for i, trait := range allocation.Traits {
			traitsValues[i] = types.StringValue(trait)
		}
		traitsList, diags := types.ListValue(types.StringType, traitsValues)
		diagnostics.Append(diags...)
		if diagnostics.HasError() {
			return
		}
		model.Traits = traitsList
	} else {
		model.Traits = types.ListNull(types.StringType)
	}

	// Handle extra data
	if len(allocation.Extra) > 0 {
		extra, err := util.StringMapToDynamic(ctx, allocation.Extra)
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
