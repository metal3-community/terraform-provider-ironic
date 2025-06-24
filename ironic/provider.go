package ironic

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/noauth"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/drivers"
	httpbasicintrospection "github.com/gophercloud/gophercloud/v2/openstack/baremetalintrospection/httpbasic"
	noauthintrospection "github.com/gophercloud/gophercloud/v2/openstack/baremetalintrospection/noauth"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// Provider Ironic.
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"url": {
				Type:        schema.TypeString,
				Required:    true,
				Description: descriptions["url"],
			},
			"inspector": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: descriptions["inspector"],
			},
			"microversion": {
				Type:        schema.TypeString,
				Required:    true,
				Description: descriptions["microversion"],
			},
			"timeout": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: descriptions["timeout"],
				Default:     0,
			},
			"auth_strategy": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: descriptions["auth_strategy"],
				ValidateFunc: validation.StringInSlice([]string{
					"noauth", "http_basic",
				}, false),
			},
			"ironic_username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: descriptions["ironic_username"],
			},
			"ironic_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: descriptions["ironic_password"],
			},
			"inspector_username": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: descriptions["inspector_username"],
			},
			"inspector_password": {
				Type:        schema.TypeString,
				Optional:    true,
				Sensitive:   true,
				Description: descriptions["inspector_password"],
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			// "ironic_node_v1":       resourceNodeV1(),
			// "ironic_port_v1":       resourcePortV1(),
			// "ironic_portgroup_v1":  resourcePortGroupV1(),
			// "ironic_allocation_v1": resourceAllocationV1(),
			"ironic_deployment": resourceDeployment(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"ironic_introspection": dataSourceIronicIntrospection(),
		},
		ConfigureContextFunc: configureProvider,
	}
}

var descriptions map[string]string

func init() {
	descriptions = map[string]string{
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
}

// Creates a noauth Ironic client.
func configureProvider(ctx context.Context, schema *schema.ResourceData) (any, diag.Diagnostics) {
	var clients Clients
	var diags diag.Diagnostics

	// Get URL with environment variable fallback
	url := schema.Get("url").(string)
	if url == "" {
		if v := os.Getenv("IRONIC_ENDPOINT"); v != "" {
			url = v
		}
	}
	if url == "" {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Missing Ironic URL",
			Detail:   "The 'url' field is required for the Ironic provider.",
		})
		// Return nil to indicate that the provider configuration failed.
		return nil, diags
	}
	log.Printf("[DEBUG] Ironic endpoint is %s", url)

	// Get microversion with environment variable fallback and default
	microversion := schema.Get("microversion").(string)
	if microversion == "" {
		if v := os.Getenv("IRONIC_MICROVERSION"); v != "" {
			microversion = v
		} else {
			microversion = "1.52"
		}
	}

	// Get auth strategy with environment variable fallback and default
	authStrategy := schema.Get("auth_strategy").(string)
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
		ironicUser := schema.Get("ironic_username").(string)
		if ironicUser == "" {
			if v := os.Getenv("IRONIC_HTTP_BASIC_USERNAME"); v != "" {
				ironicUser = v
			}
		}

		ironicPassword := schema.Get("ironic_password").(string)
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
			return nil, diag.Errorf("could not configure Ironic endpoint: %s", err.Error())
		}

		ironic.Microversion = microversion
		clients.ironic = ironic

		// Get inspector URL with environment variable fallback
		inspectorURL := schema.Get("inspector").(string)
		if inspectorURL == "" {
			if v := os.Getenv("IRONIC_INSPECTOR_ENDPOINT"); v != "" {
				inspectorURL = v
			}
		}

		if inspectorURL != "" {
			// Get inspector credentials with environment variable fallback
			inspectorUser := schema.Get("inspector_username").(string)
			if inspectorUser == "" {
				if v := os.Getenv("INSPECTOR_HTTP_BASIC_USERNAME"); v != "" {
					inspectorUser = v
				}
			}

			inspectorPassword := schema.Get("inspector_password").(string)
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
				return nil, diag.FromErr(err)
			}
			clients.inspector = inspector
		}

	} else {
		log.Printf("[DEBUG] Using noauth auth_strategy")
		ironic, err := noauth.NewBareMetalNoAuth(noauth.EndpointOpts{
			IronicEndpoint: url,
		})
		if err != nil {
			return nil, diag.FromErr(err)
		}
		ironic.Microversion = microversion
		clients.ironic = ironic

		// Get inspector URL with environment variable fallback
		inspectorURL := schema.Get("inspector").(string)
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
				return nil, diag.Errorf("could not configure inspector endpoint: %s", err.Error())
			}
			clients.inspector = inspector
		}

	}

	clients.timeout = schema.Get("timeout").(int)

	return &clients, nil
}

// Retries an API forever until it responds.
func waitForAPI(ctx context.Context, client *gophercloud.ServiceClient) {
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// NOTE: Some versions of Ironic inspector returns 404 for /v1/ but 200 for /v1,
	// which seems to be the default behavior for Flask. Remove the trailing slash
	// from the client endpoint.
	endpoint := strings.TrimSuffix(client.Endpoint, "/")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("[DEBUG] Waiting for API to become available...")

			r, err := httpClient.Get(endpoint)
			if err == nil {
				statusCode := r.StatusCode
				r.Body.Close()
				if statusCode == http.StatusOK {
					return
				}
			}

			time.Sleep(5 * time.Second)
		}
	}
}

// Ironic conductor can be considered up when the driver count returns non-zero.
func waitForConductor(ctx context.Context, client *gophercloud.ServiceClient) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("[DEBUG] Waiting for conductor API to become available...")
			driverCount := 0

			err := drivers.ListDrivers(client, drivers.ListDriversOpts{
				Detail: false,
			}).EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
				actual, err := drivers.ExtractDrivers(page)
				if err != nil {
					return false, err
				}
				driverCount += len(actual)
				return true, nil
			})
			// If we have any drivers, conductor is up.
			if err == nil && driverCount > 0 {
				return
			}

			time.Sleep(5 * time.Second)
		}
	}
}
