package ironic

import (
	"context"
	"fmt"
	"os"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/apiversions"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/noauth"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Meta stores the client connection information for Ironic.
type Meta struct {
	Client *gophercloud.ServiceClient
}

// Shared descriptions for provider attributes to ensure consistency.
var frameworkDescriptions = map[string]string{
	"url":                "The authentication endpoint for Ironic",
	"inspector":          "The endpoint for Ironic inspector",
	"microversion":       "The microversion to use for Ironic",
	"timeout":            "Wait at least the specified number of seconds for the API to become available",
	"auth_strategy":      "Determine the strategy to use for authentication with Ironic services, Possible values: noauth, http_basic. Defaults to noauth.",
	"ironic_username":    "Username to be used by Ironic when using `http_basic` authentication",
	"ironic_password":    "Password to be used by Ironic when using `http_basic` authentication",
	"inspector_username": "Username to be used by Ironic Inspector when using `http_basic` authentication",
	"inspector_password": "Password to be used by Ironic Inspector when using `http_basic` authentication",
}

// IronicProvider is a type that implements the terraform-plugin-framework
// provider.Provider interface. Someday, this will probably encompass the entire
// behavior of the ironic provider. Today, it is a small but growing subset.
type IronicProvider struct{}

var (
	_ provider.Provider                       = &IronicProvider{}
	_ provider.ProviderWithEphemeralResources = &IronicProvider{}
)

// IronicProviderModel is a helper type for extracting the provider
// configuration from the provider block.
type IronicProviderModel struct {
	Url               types.String `tfsdk:"url"`
	Inspector         types.String `tfsdk:"inspector"`
	Microversion      types.String `tfsdk:"microversion"`
	Timeout           types.Int64  `tfsdk:"timeout"`
	AuthStrategy      types.String `tfsdk:"auth_strategy"`
	IronicUsername    types.String `tfsdk:"ironic_username"`
	IronicPassword    types.String `tfsdk:"ironic_password"`
	InspectorUsername types.String `tfsdk:"inspector_username"`
	InspectorPassword types.String `tfsdk:"inspector_password"`
}

// New is a helper function for initializing the portion of
// the ironic provider implemented via the terraform-plugin-framework.
func New() func() provider.Provider {
	return func() provider.Provider {
		return &IronicProvider{}
	}
}

// Metadata (a Provider interface function) lets the provider identify itself.
// Resources and data sources can access this information from their request
// objects.
func (p *IronicProvider) Metadata(
	_ context.Context,
	_ provider.MetadataRequest,
	res *provider.MetadataResponse,
) {
	res.TypeName = "ironic"
}

// Schema (a Provider interface function) returns the schema for the Terraform
// block that configures the provider itself.
func (p *IronicProvider) Schema(
	_ context.Context,
	_ provider.SchemaRequest,
	res *provider.SchemaResponse,
) {
	res.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Required:    true,
				Description: frameworkDescriptions["url"],
			},
			"inspector": schema.StringAttribute{
				Optional:    true,
				Description: frameworkDescriptions["inspector"],
			},
			"microversion": schema.StringAttribute{
				Required:    true,
				Description: frameworkDescriptions["microversion"],
			},
			"timeout": schema.Int64Attribute{
				Optional:    true,
				Description: frameworkDescriptions["timeout"],
			},
			"auth_strategy": schema.StringAttribute{
				Optional:    true,
				Description: frameworkDescriptions["auth_strategy"],
			},
			"ironic_username": schema.StringAttribute{
				Optional:    true,
				Description: frameworkDescriptions["ironic_username"],
			},
			"ironic_password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: frameworkDescriptions["ironic_password"],
			},
			"inspector_username": schema.StringAttribute{
				Optional:    true,
				Description: frameworkDescriptions["inspector_username"],
			},
			"inspector_password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: frameworkDescriptions["inspector_password"],
			},
		},
	}
}

// Configure (a Provider interface function) sets up the Ironic client per the
// specified provider configuration block and env vars.
func (p *IronicProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	res *provider.ConfigureResponse,
) {
	var meta Meta
	var data IronicProviderModel
	diags := req.Config.Get(ctx, &data)

	res.Diagnostics.Append(diags...)
	if res.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Configuring Ironic provider")

	// Get URL with environment variable fallback
	url := data.Url.ValueString()
	if url == "" {
		if v := os.Getenv("IRONIC_ENDPOINT"); v != "" {
			url = v
		}
	}
	if url == "" {
		res.Diagnostics.AddError(
			"Missing Ironic URL",
			"The 'url' field is required for the Ironic provider.",
		)
		return
	}
	tflog.Debug(ctx, "Setting up ironic endpoint", map[string]any{"url": url})

	// Get microversion with environment variable fallback and default
	microversion := data.Microversion.ValueString()
	if microversion == "" {
		if v := os.Getenv("IRONIC_MICROVERSION"); v != "" {
			microversion = v
		} else {
			microversion = "1.99"
		}
	}

	// Get auth strategy with environment variable fallback and default
	authStrategy := data.AuthStrategy.ValueString()
	if authStrategy == "" {
		if v := os.Getenv("IRONIC_AUTH_STRATEGY"); v != "" {
			authStrategy = v
		} else {
			authStrategy = "noauth"
		}
	}

	if authStrategy == "http_basic" {
		tflog.Debug(ctx, "Using http_basic auth_strategy")

		// Get Ironic credentials with environment variable fallback
		ironicUser := data.IronicUsername.ValueString()
		if ironicUser == "" {
			if v := os.Getenv("IRONIC_HTTP_BASIC_USERNAME"); v != "" {
				ironicUser = v
			}
		}

		ironicPassword := data.IronicPassword.ValueString()
		if ironicPassword == "" {
			if v := os.Getenv("IRONIC_HTTP_BASIC_PASSWORD"); v != "" {
				ironicPassword = v
			}
		}

		ironic, err := httpbasic.NewBareMetalHTTPBasic(httpbasic.EndpointOpts{
			IronicEndpoint:     url,
			IronicUser:         ironicUser,
			IronicUserPassword: ironicPassword,
		})
		if err != nil {
			res.Diagnostics.AddError(
				"Could not configure Ironic endpoint",
				fmt.Sprintf("Error: %s", err.Error()),
			)
			return
		}

		ironic.Microversion = microversion
		meta.Client = ironic

	} else {
		tflog.Debug(ctx, "Using noauth auth_strategy")
		ironic, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
			IronicEndpoint: url,
		})
		if err != nil {
			res.Diagnostics.AddError(
				"Could not configure Ironic endpoint",
				fmt.Sprintf("Error: %s", err.Error()),
			)
			return
		}
		ironic.Microversion = microversion
		meta.Client = ironic
	}

	if err := healthCheck(ctx, meta.Client); err != nil {
		res.Diagnostics.AddError(
			"Ironic API health check failed",
			fmt.Sprintf("Error: %s", err.Error()),
		)
		return
	}

	res.DataSourceData = &meta
	res.ResourceData = &meta
	res.EphemeralResourceData = &meta
}

func (p *IronicProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNodeInventoryDataSource,
	}
}

func (p *IronicProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNodeResource,
		NewPortGroupResource,
		NewPortV1Resource,
		NewAllocationV1Resource,
		NewDeploymentResource,
	}
}

func (p *IronicProvider) EphemeralResources(
	ctx context.Context,
) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{}
}

func healthCheck(ctx context.Context, client *gophercloud.ServiceClient) error {
	apiversionListResp, err := apiversions.List(ctx, client).Extract()
	if err != nil {
		return fmt.Errorf("failed to extract conductors from Ironic API response: %w",
			err)
	}
	for _, apiversion := range apiversionListResp.Versions {
		if apiversion.ID == "v1" && apiversion.Status != "CURRENT" {
			tflog.Error(ctx, "v1 API version is not current", map[string]any{
				"version": apiversion.Version,
				"status":  apiversion.Status,
			})
			return fmt.Errorf("ironic API health check failed: API version %s is not current",
				apiversion.Version)
		}
	}
	// If we reach here, the API is considered healthy.
	tflog.Info(ctx, "Ironic API is healthy", map[string]any{
		"client": client.Endpoint,
	})
	return nil
}
