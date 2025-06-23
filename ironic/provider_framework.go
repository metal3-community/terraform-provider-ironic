package ironic

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// frameworkProvider is a type that implements the terraform-plugin-framework
// provider.Provider interface. Someday, this will probably encompass the entire
// behavior of the ironic provider. Today, it is a small but growing subset.
type frameworkProvider struct {
	defaultSiteName *string
}

var (
	_ provider.Provider                       = &frameworkProvider{}
	_ provider.ProviderWithEphemeralResources = &frameworkProvider{}
)

// FrameworkProviderConfig is a helper type for extracting the provider
// configuration from the provider block.
type FrameworkProviderConfig struct {
	Url               types.String `tfsdk:"url"`
	Inspector         types.String `tfsdk:"inspector"`
	MicroVersion      types.String `tfsdk:"microversion"`
	Timeout           types.Int64  `tfsdk:"timeout"`
	AuthStrategy      types.String `tfsdk:"auth_strategy"`
	User              types.String `tfsdk:"ironic_username"`
	Password          types.String `tfsdk:"ironic_password"`
	InspectorUser     types.String `tfsdk:"inspector_username"`
	InspectorPassword types.String `tfsdk:"inspector_password"`
}

// NewFrameworkProvider is a helper function for initializing the portion of
// the ironic provider implemented via the terraform-plugin-framework.
func NewFrameworkProvider() provider.Provider {
	return &frameworkProvider{}
}

// NewFrameworkProviderWithDefaultOrg is a helper function for
// initializing a framework provider with a default site name.
func NewFrameworkProviderWithDefaultSite(defaultSiteName string) provider.Provider {
	return &frameworkProvider{defaultSiteName: &defaultSiteName}
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
				MarkdownDescription: descriptions["url"],
				Required:            true,
			},
			"inspector": schema.StringAttribute{
				MarkdownDescription: descriptions["inspector"],
				Optional:            true,
			},
			"microversion": schema.StringAttribute{
				MarkdownDescription: descriptions["microversion"],
				Optional:            true,
			},
			"timeout": schema.Int64Attribute{
				MarkdownDescription: descriptions["timeout"],
				Optional:            true,
			},
			"auth_strategy": schema.StringAttribute{
				MarkdownDescription: descriptions["auth_strategy"],
				Optional:            true,
			},
			"ironic_username": schema.StringAttribute{
				MarkdownDescription: descriptions["ironic_username"],
				Optional:            true,
			},
			"ironic_password": schema.StringAttribute{
				MarkdownDescription: descriptions["ironic_password"],
				Optional:            true,
				Sensitive:           true,
			},
			"inspector_username": schema.StringAttribute{
				MarkdownDescription: descriptions["inspector_username"],
				Optional:            true,
			},
			"inspector_password": schema.StringAttribute{
				MarkdownDescription: descriptions["inspector_password"],
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

// Configure (a Provider interface function) sets up the HCP Terraform client per the
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

	tflog.Info(ctx, "Configuring Synology provider")

	res.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if res.Diagnostics.HasError() {
		return
	}
	endpoint := data.Url.ValueString()
	if endpoint == "" {
		if v := os.Getenv("IRONIC_ENDPOINT"); v != "" {
			endpoint = v
		}
	}
	inspector := data.Inspector.ValueString()
	if inspector == "" {
		if v := os.Getenv("IRONIC_INSPECTOR_ENDPOINT"); v != "" {
			inspector = v
		}
	}
	user := data.User.ValueString()
	if user == "" {
		if v := os.Getenv("IRONIC_HTTP_BASIC_USERNAME"); v != "" {
			user = v
		}
	}
	pass := data.Password.ValueString()
	if pass == "" {
		if v := os.Getenv("IRONIC_HTTP_BASIC_PASSWORD"); v != "" {
			pass = v
		}
	}
	authStrategy := data.AuthStrategy.ValueString()
	if !authStrategy {
		if v := os.Getenv("IRONIC_INSECURE"); v != "" {
			authStrategy == "http_basic" {

			}
		}
	}
	site := data.Site.ValueString()
	if site == "" {
		if v := os.Getenv("IRONIC_SITE"); v != "" {
			site = v
		}
	}
	if site == "" {
		if p.defaultSiteName != nil {
			site = *p.defaultSiteName
		} else {
			site = "default"
		}
	}

	res.DataSourceData = clients
	res.ResourceData = clients
	res.EphemeralResourceData = clients
}

func (p *frameworkProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *frameworkProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (p *frameworkProvider) EphemeralResources(
	ctx context.Context,
) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{}
}
