package ironic

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	utilgc "github.com/gophercloud/utils/openstack/baremetal/v1/nodes"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = &deploymentResource{}
	_ resource.ResourceWithConfigure   = &deploymentResource{}
	_ resource.ResourceWithImportState = &deploymentResource{}
)

// deploymentResource defines the resource implementation.
type deploymentResource struct {
	meta *Meta
}

type fixedIpModel struct {
	IPAddress types.String `tfsdk:"ip_address"`
}

type deployStepModel struct {
	Interface types.String `tfsdk:"interface"`
	Step      types.String `tfsdk:"step"`
	Args      types.Map    `tfsdk:"args"`
	Priority  types.Int64  `tfsdk:"priority"`
	Tes       []nodes.DeployStep
}

// deploymentResourceModel describes the resource data model.
type deploymentResourceModel struct {
	ID                 types.String  `tfsdk:"id"`
	Name               types.String  `tfsdk:"name"`
	NodeUUID           types.String  `tfsdk:"node_uuid"`
	InstanceInfo       types.Dynamic `tfsdk:"instance_info"`
	DeploySteps        types.List    `tfsdk:"deploy_steps"`
	UserData           types.Dynamic `tfsdk:"user_data"`
	UserDataURL        types.String  `tfsdk:"user_data_url"`
	UserDataURLCaCert  types.String  `tfsdk:"user_data_url_ca_cert"`
	UserDataURLHeaders types.Dynamic `tfsdk:"user_data_url_headers"`
	NetworkData        types.Dynamic `tfsdk:"network_data"`
	Metadata           types.Dynamic `tfsdk:"metadata"`
	FixedIPs           types.List    `tfsdk:"fixed_ips"`
	ProvisionState     types.String  `tfsdk:"provision_state"`
	LastError          types.String  `tfsdk:"last_error"`
}

func NewDeploymentResource() resource.Resource {
	return &deploymentResource{}
}

func (r *deploymentResource) Metadata(
	ctx context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (r *deploymentResource) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Ironic deployment resource. This drives the node through the provisioning state machine to deploy an operating system.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The UUID of the deployment (same as node_uuid).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the deployment.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"node_uuid": schema.StringAttribute{
				MarkdownDescription: "The UUID of the node to deploy.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"instance_info": schema.DynamicAttribute{
				MarkdownDescription: "Instance information for the deployment.",
				Required:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.RequiresReplace(),
				},
			},
			"deploy_steps": schema.ListNestedAttribute{
				MarkdownDescription: "JSON string of deploy steps for the deployment.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"interface": schema.StringAttribute{
							MarkdownDescription: "The interface to use for the deploy step.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									string(nodes.InterfaceBIOS),
									string(nodes.InterfaceDeploy),
									string(nodes.InterfaceFirmware),
									string(nodes.InterfaceManagement),
									string(nodes.InterfacePower),
									string(nodes.InterfaceRAID),
								),
							},
						},
						"step": schema.StringAttribute{
							MarkdownDescription: "The name of the deploy step.",
							Required:            true,
						},
						"args": schema.MapAttribute{
							MarkdownDescription: "Arguments for the deploy step.",
							Optional:            true,
							ElementType:         types.StringType,
						},
						"priority": schema.Int64Attribute{
							MarkdownDescription: "The priority of the deploy step.",
							Optional:            true,
							Computed:            true,
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"user_data": schema.DynamicAttribute{
				MarkdownDescription: "User data for the deployment.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.RequiresReplace(),
				},
			},
			"user_data_url": schema.StringAttribute{
				MarkdownDescription: "URL to fetch user data from.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_data_url_ca_cert": schema.StringAttribute{
				MarkdownDescription: "CA certificate for user data URL verification.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_data_url_headers": schema.DynamicAttribute{
				MarkdownDescription: "Headers to send when fetching user data URL.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.RequiresReplace(),
				},
			},
			"network_data": schema.DynamicAttribute{
				MarkdownDescription: "Network data for the deployment.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.RequiresReplace(),
				},
			},
			"metadata": schema.DynamicAttribute{
				MarkdownDescription: "Metadata for the deployment.",
				Optional:            true,
				PlanModifiers: []planmodifier.Dynamic{
					dynamicplanmodifier.RequiresReplace(),
				},
			},
			"fixed_ips": schema.ListNestedAttribute{
				MarkdownDescription: "Fixed IP addresses for the deployment.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip_address": schema.StringAttribute{
							MarkdownDescription: "The fixed IP address.",
							Required:            true,
						},
					},
				},
				Optional: true,
			},
			"provision_state": schema.StringAttribute{
				MarkdownDescription: "The current provision state of the node.",
				Computed:            true,
			},
			"last_error": schema.StringAttribute{
				MarkdownDescription: "The last error message from the node.",
				Computed:            true,
			},
		},
	}
}

func (r *deploymentResource) Configure(
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

func (r *deploymentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var model deploymentResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nodeUUID := model.NodeUUID.ValueString()
	model.ID = model.NodeUUID

	tflog.Info(ctx, "Creating deployment", map[string]any{
		"node_uuid": nodeUUID,
	})

	node, err := nodes.Get(ctx, r.meta.Client, nodeUUID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.Diagnostics.AddError(
				"Node not found",
				fmt.Sprintf("Node with UUID %s was not found. Please ensure the node exists.",
					nodeUUID,
				),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error getting node",
			fmt.Sprintf("Could not get node %s: %s", nodeUUID, err),
		)
		return
	}

	// Prepare update options
	updateOpts := nodes.UpdateOpts{}

	// Handle deploy_steps if present
	var deploySteps []nodes.DeployStep
	if !model.DeploySteps.IsNull() && !model.DeploySteps.IsUnknown() {
		var dSteps []deployStepModel
		resp.Diagnostics.Append(model.DeploySteps.ElementsAs(ctx, &dSteps, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(buildDeploySteps(ctx, dSteps, &deploySteps)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(deploySteps) > 0 {
			updateOpts = append(updateOpts, nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  "/deploy_steps",
				Value: deploySteps,
			})
		}
	}

	util.AddDynamicUpdateOptForFieldWithMap(
		ctx,
		&updateOpts,
		&resp.Diagnostics,
		model.InstanceInfo,
		node.InstanceInfo,
		"instance_info",
	)

	// Handle fixed_ips if present
	if !model.FixedIPs.IsNull() && !model.FixedIPs.IsUnknown() {
		var fixedIPs []fixedIpModel
		resp.Diagnostics.Append(model.FixedIPs.ElementsAs(ctx, &fixedIPs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if len(fixedIPs) > 0 {
			// Convert to interface{} slice for the update operation
			fixedIPsInterface := make([]any, len(fixedIPs))
			for i, ip := range fixedIPs {
				ipMap := make(map[string]any)
				ipMap["ip_address"] = ip.IPAddress.ValueString()
				fixedIPsInterface[i] = ipMap
			}

			updateOpts = append(updateOpts, nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  "/instance_info/fixed_ips",
				Value: fixedIPsInterface,
			})

			_, err = UpdateNode(ctx, r.meta.Client, nodeUUID, updateOpts)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error updating fixed_ips",
					fmt.Sprintf("Could not update fixed_ips: %s", err),
				)
				return
			}
		}
	}

	// Handle user data
	userDataURL := model.UserDataURL.ValueString()
	userDataCaCert := model.UserDataURLCaCert.ValueString()

	// Build config drive
	var userDataMap map[string]any
	if !model.UserData.IsNull() && !model.UserData.IsUnknown() {
		var err error
		userDataMap, err = util.DynamicToMap(ctx, model.UserData)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("user_data"),
				"Error Converting Network Data",
				fmt.Sprintf("Could not convert user_data to map: %s", err),
			)
			return
		}
	}

	var userDataHeaders map[string]any
	if !model.UserDataURLHeaders.IsNull() && !model.UserDataURLHeaders.IsUnknown() {
		userDataHeadersMap, err := util.DynamicToMap(ctx, model.UserDataURLHeaders)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("user_data_url_headers"),
				"Error Converting Headers",
				fmt.Sprintf("Could not convert user_data_url_headers to map: %s", err),
			)
			return
		}
		if userDataHeadersMap != nil {
			userDataHeaders = userDataHeadersMap
		}
	}

	// If user_data_url is specified in addition to user_data, use the former
	ignitionData, err := fetchFullIgnition(userDataURL, userDataCaCert, userDataHeaders)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching user data from URL",
			fmt.Sprintf("Could not fetch data from user_data_url: %s", err),
		)
		return
	}
	if ignitionData != nil {
		userDataMap = ignitionData
	}

	// Build config drive
	var networkDataMap map[string]any
	if !model.NetworkData.IsNull() && !model.NetworkData.IsUnknown() {
		var err error
		networkDataMap, err = util.DynamicToMap(ctx, model.NetworkData)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("network_data"),
				"Error Converting Network Data",
				fmt.Sprintf("Could not convert network_data to map: %s", err),
			)
			return
		}
	}

	var metaDataMap map[string]any
	if !model.Metadata.IsNull() && !model.Metadata.IsUnknown() {
		var err error
		metaDataMap, err = util.DynamicToMap(ctx, model.Metadata)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("metadata"),
				"Error Converting Metadata",
				fmt.Sprintf("Could not convert metadata to map: %s", err),
			)
			return
		}
	}

	configDrive, err := buildConfigDrive(
		r.meta.Client.Microversion,
		userDataMap,
		networkDataMap,
		metaDataMap,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error building config drive",
			fmt.Sprintf("Could not build config drive: %s", err),
		)
		return
	}

	// Deploy the node - drive Ironic state machine until node is 'active'
	tflog.Info(ctx, "Starting deployment", map[string]any{
		"node_uuid": nodeUUID,
		"target":    "active",
	})

	if err := ChangeProvisionStateToTarget(
		ctx,
		r.meta.Client,
		nodeUUID,
		nodes.TargetActive,
		configDrive,
		deploySteps,
		nil,
	); err != nil {
		resp.Diagnostics.AddError(
			"Error deploying node",
			fmt.Sprintf("Could not deploy node: %s", err),
		)
		return
	}

	// Read the final state
	r.read(ctx, &model, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *deploymentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var model deploymentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &model, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *deploymentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *deploymentResource) read(
	ctx context.Context,
	model *deploymentResourceModel,
	diagnostics *diag.Diagnostics,
) {
	nodeUUID := model.NodeUUID.ValueString()
	if nodeUUID == "" {
		nodeUUID = model.ID.ValueString()
	}

	tflog.Debug(ctx, "Reading deployment", map[string]any{
		"node_uuid": nodeUUID,
	})

	result, err := nodes.Get(ctx, r.meta.Client, nodeUUID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			diagnostics.AddWarning(
				"Deployment not found",
				fmt.Sprintf(
					"Deployment with node UUID %s was not found. Removing from state.",
					nodeUUID,
				),
			)
			return
		}
		diagnostics.AddError(
			"Error reading deployment",
			fmt.Sprintf("Could not read deployment %s: %s", nodeUUID, err),
		)
		return
	}

	model.ProvisionState = types.StringValue(result.ProvisionState)
	model.LastError = types.StringValue(result.LastError)
}

func (r *deploymentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	// Deployment updates are not supported - all attributes require replacement
	resp.Diagnostics.AddError(
		"Update not supported",
		"Updates to deployment resources are not supported. All changes require replacement.",
	)
}

func (r *deploymentResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var model deploymentResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nodeUUID := model.ID.ValueString()

	tflog.Info(ctx, "Deleting deployment", map[string]any{
		"node_uuid": nodeUUID,
	})

	if err := ChangeProvisionStateToTarget(
		ctx,
		r.meta.Client,
		nodeUUID,
		nodes.TargetDeleted,
		nil,
		nil,
		nil,
	); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting deployment",
			fmt.Sprintf("Could not delete deployment: %s", err),
		)
		return
	}

	tflog.Info(ctx, "Deployment deleted successfully", map[string]any{
		"node_uuid": nodeUUID,
	})
}

// fetchFullIgnition gets full ignition from the URL and cert passed to it and returns userdata as a string.
func fetchFullIgnition(
	userDataURL string,
	userDataCaCert string,
	userDataHeaders map[string]any,
) (map[string]any, error) {
	// Send full ignition, if the URL is specified
	if userDataURL != "" {
		caCertPool := x509.NewCertPool()
		transport := &http.Transport{}

		if userDataCaCert != "" {
			caCert, err := base64.StdEncoding.DecodeString(userDataCaCert)
			if err != nil {
				tflog.Error(
					context.Background(),
					"could not decode user_data_url_ca_cert",
					map[string]any{
						"error": err.Error(),
					},
				)
				return nil, err
			}
			caCertPool.AppendCertsFromPEM(caCert)
			// disable "G402 (CWE-295): TLS MinVersion too low. (Confidence: HIGH, Severity: HIGH)"
			// #nosec G402
			transport.TLSClientConfig = &tls.Config{RootCAs: caCertPool}
		} else {
			// Disable certificate verification
			// disable "G402 (CWE-295): TLS MinVersion too low. (Confidence: HIGH, Severity: HIGH)"
			// #nosec G402
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}

		client := retryablehttp.NewClient()
		client.HTTPClient.Transport = transport

		// Get the data
		req, err := retryablehttp.NewRequest("GET", userDataURL, nil)
		if err != nil {
			tflog.Error(
				context.Background(),
				"could not create request for user_data_url",
				map[string]any{
					"url":   userDataURL,
					"error": err.Error(),
				},
			)
			return nil, err
		}
		for k, v := range userDataHeaders {
			if strVal, ok := v.(string); ok {
				req.Header.Add(k, strVal)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			tflog.Error(context.Background(), "could not fetch user_data_url", map[string]any{
				"url":   userDataURL,
				"error": err.Error(),
			})
			return nil, err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				tflog.Error(
					context.Background(),
					"could not close user_data_url response body",
					map[string]any{
						"error": err.Error(),
					},
				)
			}
		}()
		var userData map[string]any
		err = json.NewDecoder(resp.Body).Decode(&userData)
		if err != nil {
			tflog.Error(
				context.Background(),
				"could not read user_data_url response",
				map[string]any{
					"error": err.Error(),
				},
			)
			return nil, err
		}
		return userData, nil
	}
	return nil, nil
}

// buildDeploySteps handles customized deploy steps.
func buildDeploySteps(
	ctx context.Context,
	dSteps []deployStepModel,
	deploySteps *[]nodes.DeployStep,
) (diags diag.Diagnostics) {
	// Convert deploy steps to nodes.DeployStep
	if len(dSteps) == 0 {
		diags.AddWarning(
			"Empty deploy_steps",
			"Deploy steps are empty. No deploy steps will be applied.",
		)
		return diags
	}
	// Build deploy steps from the model
	dStepsN := make([]nodes.DeployStep, len(dSteps))
	for i, step := range dSteps {
		deployStep := nodes.DeployStep{
			Interface: nodes.StepInterface(step.Interface.ValueString()),
			Step:      step.Step.ValueString(),
			Priority:  int(step.Priority.ValueInt64()),
		}
		diags.Append(step.Args.ElementsAs(ctx, &deployStep.Args, false)...)
		if diags.HasError() {
			return diags
		}
		dStepsN[i] = deployStep
	}
	*deploySteps = dStepsN
	return diags
}

func convertNetworkData(networkData map[string]any) (map[string]any, error) {
	networkDataU := map[string]any{}
	for k, v := range networkData {
		if vs, ok := v.(string); ok {
			firstChar := vs[0]
			switch firstChar {
			case '[':
				vd := []any{}
				if err := json.Unmarshal([]byte(vs), &vd); err != nil {
					return nil, fmt.Errorf(
						"error unmarshalling network data for key %s: %v",
						k,
						err,
					)
				}
				networkDataU[k] = vd
			case '{':
				// If the value is a JSON string, unmarshal it into an interface
				vd := map[string]any{}
				if err := json.Unmarshal([]byte(vs), &vd); err != nil {
					return nil, fmt.Errorf(
						"error unmarshalling network data for key %s: %v",
						k,
						err,
					)
				}
				networkDataU[k] = vd
			default:
				// If the value is a string, just use it as is
				networkDataU[k] = vs
			}
		} else {
			networkDataU[k] = v
		}
	}
	return networkDataU, nil
}

// buildConfigDrive handles building a config drive appropriate for the Ironic version we are using.  Newer versions
// support sending the user data directly, otherwise we need to build an ISO image.
func buildConfigDrive(
	apiVersion string,
	userDataMap map[string]any,
	networkData, metaData map[string]any,
) (any, error) {
	var networkDataU map[string]any
	var err error
	var userData any

	if len(userDataMap) == 1 && userDataMap["value"] != nil {
		// If user_data is a single value, convert it to a string
		if userDataStr, ok := userDataMap["value"].(string); !ok {
			return nil, fmt.Errorf(
				"user_data must be a string when only one key is present, got %T",
				userDataMap["value"],
			)
		} else {
			userData = userDataStr
		}
	} else {
		userData = userDataMap
	}

	if networkData != nil {
		networkDataU, err = convertNetworkData(networkData)
		if err != nil {
			return nil, fmt.Errorf("error converting network data: %v", err)
		}
	}

	actual, err := version.NewVersion(apiVersion)
	if err != nil {
		return nil, err
	}
	minimum, err := version.NewVersion("1.56")
	if err != nil {
		return nil, err
	}

	if minimum.GreaterThan(actual) {
		userDataStr, ok := userData.(string)
		if !ok {
			return nil, fmt.Errorf(
				"user_data must be a string for Ironic versions < 1.56, got %T",
				userData,
			)
		}
		// Create config drive ISO directly with gophercloud/utils
		configDriveData := utilgc.ConfigDrive{
			UserData:    utilgc.UserDataString(userDataStr),
			NetworkData: networkDataU,
			MetaData:    metaData,
		}
		configDriveISO, err := configDriveData.ToConfigDrive()
		if err != nil {
			return nil, err
		}
		return &configDriveISO, nil
	}
	// Let Ironic handle creating the config drive
	return &nodes.ConfigDrive{
		UserData:    userData,
		NetworkData: networkDataU,
		MetaData:    metaData,
	}, nil
}

// UpdateNode wraps gophercloud's update function, so we are able to retry on 409 when Ironic is busy.
func UpdateNode(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	uuid string,
	opts nodes.UpdateOpts,
) (node *nodes.Node, err error) {
	interval := 5 * time.Second
	for range 5 {
		node, err = nodes.Update(ctx, client, uuid, opts).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
				tflog.Debug(
					ctx,
					"Failed to update node: ironic is busy, will retry",
					map[string]any{
						"uuid":     uuid,
						"interval": interval.String(),
					},
				)
				time.Sleep(interval)
				interval *= 2
				continue
			}
		} else {
			break
		}
	}

	return
}
