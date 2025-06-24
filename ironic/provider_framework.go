package ironic

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/noauth"
	httpbasicintrospection "github.com/gophercloud/gophercloud/v2/openstack/baremetalintrospection/httpbasic"
	noauthintrospection "github.com/gophercloud/gophercloud/v2/openstack/baremetalintrospection/noauth"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

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

// frameworkProvider is a type that implements the terraform-plugin-framework
// provider.Provider interface. Someday, this will probably encompass the entire
// behavior of the ironic provider. Today, it is a small but growing subset.
type frameworkProvider struct{}

var (
	_ provider.Provider                       = &frameworkProvider{}
	_ provider.ProviderWithEphemeralResources = &frameworkProvider{}
)

// FrameworkProviderConfig is a helper type for extracting the provider
// configuration from the provider block.
type FrameworkProviderConfig struct {
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

// NewFrameworkProvider is a helper function for initializing the portion of
// the ironic provider implemented via the terraform-plugin-framework.
func NewFrameworkProvider() provider.Provider {
	return &frameworkProvider{}
}

// Metadata (a Provider interface function) lets the provider identify itself.
// Resources and data sources can access this information from their request
// objects.
func (p *frameworkProvider) Metadata(
	_ context.Context,
	_ provider.MetadataRequest,
	res *provider.MetadataResponse,
) {
	res.TypeName = "ironic"
}

// Schema (a Provider interface function) returns the schema for the Terraform
// block that configures the provider itself.
func (p *frameworkProvider) Schema(
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
func (p *frameworkProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	res *provider.ConfigureResponse,
) {
	var clients Clients
	var data FrameworkProviderConfig
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
	log.Printf("[DEBUG] Ironic endpoint is %s", url)

	// Get microversion with environment variable fallback and default
	microversion := data.Microversion.ValueString()
	if microversion == "" {
		if v := os.Getenv("IRONIC_MICROVERSION"); v != "" {
			microversion = v
		} else {
			microversion = "1.52"
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
		log.Printf("[DEBUG] Using http_basic auth_strategy")

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
		clients.ironic = ironic

		// Handle inspector endpoint if provided
		inspectorURL := data.Inspector.ValueString()
		if inspectorURL == "" {
			if v := os.Getenv("IRONIC_INSPECTOR_ENDPOINT"); v != "" {
				inspectorURL = v
			}
		}

		if inspectorURL != "" {
			inspectorUser := data.InspectorUsername.ValueString()
			if inspectorUser == "" {
				if v := os.Getenv("INSPECTOR_HTTP_BASIC_USERNAME"); v != "" {
					inspectorUser = v
				}
			}

			inspectorPassword := data.InspectorPassword.ValueString()
			if inspectorPassword == "" {
				if v := os.Getenv("INSPECTOR_HTTP_BASIC_PASSWORD"); v != "" {
					inspectorPassword = v
				}
			}

			log.Printf("[DEBUG] Inspector endpoint is %s", inspectorURL)

			inspector, err := httpbasicintrospection.NewBareMetalIntrospectionHTTPBasic(
				httpbasicintrospection.EndpointOpts{
					IronicInspectorEndpoint:     inspectorURL,
					IronicInspectorUser:         inspectorUser,
					IronicInspectorUserPassword: inspectorPassword,
				},
			)
			if err != nil {
				res.Diagnostics.AddError(
					"Could not configure inspector endpoint",
					fmt.Sprintf("Error: %s", err.Error()),
				)
				return
			}
			clients.inspector = inspector
		}

	} else {
		log.Printf("[DEBUG] Using noauth auth_strategy")
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
		clients.ironic = ironic

		// Handle inspector endpoint if provided
		inspectorURL := data.Inspector.ValueString()
		if inspectorURL == "" {
			if v := os.Getenv("IRONIC_INSPECTOR_ENDPOINT"); v != "" {
				inspectorURL = v
			}
		}

		if inspectorURL != "" {
			log.Printf("[DEBUG] Inspector endpoint is %s", inspectorURL)
			inspector, err := noauthintrospection.NewBareMetalIntrospectionNoAuth(noauthintrospection.EndpointOpts{
				IronicInspectorEndpoint: inspectorURL,
			})
			if err != nil {
				res.Diagnostics.AddError(
					"Could not configure inspector endpoint",
					fmt.Sprintf("Error: %s", err.Error()),
				)
				return
			}
			clients.inspector = inspector
		}
	}

	// Set timeout with default value of 0
	timeout := int(data.Timeout.ValueInt64())
	clients.timeout = timeout

	res.DataSourceData = &clients
	res.ResourceData = &clients
	res.EphemeralResourceData = &clients
}

func (p *frameworkProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *frameworkProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNodeV1Resource,
		NewPortGroupV1Resource,
		NewPortV1Resource,
		NewAllocationV1Resource,
	}
}

func (p *frameworkProvider) EphemeralResources(
	ctx context.Context,
) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{}
}
