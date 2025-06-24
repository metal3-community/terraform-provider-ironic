package ironic

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &introspectionDataSource{}
	_ datasource.DataSourceWithConfigure = &introspectionDataSource{}
)

// introspectionDataSource defines the data source implementation.
type introspectionDataSource struct {
	clients *Clients
}

// introspectionDataSourceModel describes the data source data model.
type introspectionDataSourceModel struct {
	ID        types.String                 `tfsdk:"id"`
	UUID      types.String                 `tfsdk:"uuid"`
	Inventory *introspectionInventoryModel `tfsdk:"inventory"`
	CPU       *introspectionCPUModel       `tfsdk:"cpu"`
	Memory    *introspectionMemoryModel    `tfsdk:"memory"`
	Disks     []introspectionDiskModel     `tfsdk:"disks"`
	NICs      []introspectionNICModel      `tfsdk:"nics"`
	System    *introspectionSystemModel    `tfsdk:"system"`
}

type introspectionInventoryModel struct {
	BmcAddress    types.String `tfsdk:"bmc_address"`
	BmcV6Address  types.String `tfsdk:"bmc_v6address"`
	BootInterface types.String `tfsdk:"boot_interface"`
}

type introspectionCPUModel struct {
	Architecture types.String `tfsdk:"architecture"`
	Count        types.Int64  `tfsdk:"count"`
	Frequency    types.String `tfsdk:"frequency"`
	Flags        types.List   `tfsdk:"flags"`
	ModelName    types.String `tfsdk:"model_name"`
}

type introspectionMemoryModel struct {
	PhysicalMb types.Int64  `tfsdk:"physical_mb"`
	Total      types.String `tfsdk:"total"`
}

type introspectionDiskModel struct {
	Name         types.String `tfsdk:"name"`
	Model        types.String `tfsdk:"model"`
	Size         types.Int64  `tfsdk:"size"`
	Rotational   types.Bool   `tfsdk:"rotational"`
	WWN          types.String `tfsdk:"wwn"`
	WWNWithExt   types.String `tfsdk:"wwn_with_extension"`
	WWNVendorExt types.String `tfsdk:"wwn_vendor_extension"`
	Serial       types.String `tfsdk:"serial"`
}

type introspectionNICModel struct {
	Name          types.String `tfsdk:"name"`
	MAC           types.String `tfsdk:"mac"`
	IPV4          types.String `tfsdk:"ipv4"`
	IPV6          types.String `tfsdk:"ipv6"`
	HasCarrier    types.Bool   `tfsdk:"has_carrier"`
	LLDPProcessed types.Bool   `tfsdk:"lldp_processed"`
	Product       types.String `tfsdk:"product"`
	Vendor        types.String `tfsdk:"vendor"`
	SpeedMbps     types.Int64  `tfsdk:"speed_mbps"`
}

type introspectionSystemModel struct {
	Product      types.String `tfsdk:"product"`
	Family       types.String `tfsdk:"family"`
	Version      types.String `tfsdk:"version"`
	SKU          types.String `tfsdk:"sku"`
	Serial       types.String `tfsdk:"serial"`
	UUID         types.String `tfsdk:"uuid"`
	Manufacturer types.String `tfsdk:"manufacturer"`
}

func NewIntrospectionDataSource() datasource.DataSource {
	return &introspectionDataSource{}
}

func (d *introspectionDataSource) Metadata(
	ctx context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_introspection"
}

func (d *introspectionDataSource) Schema(
	ctx context.Context,
	req datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves introspection data for an Ironic node using the node's inventory data.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Data source identifier.",
				Computed:            true,
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "UUID of the node to get introspection data for.",
				Required:            true,
			},
		},
		Blocks: map[string]schema.Block{
			"inventory": schema.SingleNestedBlock{
				MarkdownDescription: "Basic inventory information.",
				Attributes: map[string]schema.Attribute{
					"bmc_address": schema.StringAttribute{
						MarkdownDescription: "BMC IP address.",
						Computed:            true,
					},
					"bmc_v6address": schema.StringAttribute{
						MarkdownDescription: "BMC IPv6 address.",
						Computed:            true,
					},
					"boot_interface": schema.StringAttribute{
						MarkdownDescription: "Boot interface name.",
						Computed:            true,
					},
				},
			},
			"cpu": schema.SingleNestedBlock{
				MarkdownDescription: "CPU information.",
				Attributes: map[string]schema.Attribute{
					"architecture": schema.StringAttribute{
						MarkdownDescription: "CPU architecture (e.g., x86_64).",
						Computed:            true,
					},
					"count": schema.Int64Attribute{
						MarkdownDescription: "Number of CPU cores.",
						Computed:            true,
					},
					"frequency": schema.StringAttribute{
						MarkdownDescription: "CPU frequency.",
						Computed:            true,
					},
					"flags": schema.ListAttribute{
						MarkdownDescription: "List of CPU flags.",
						ElementType:         types.StringType,
						Computed:            true,
					},
					"model_name": schema.StringAttribute{
						MarkdownDescription: "CPU model name.",
						Computed:            true,
					},
				},
			},
			"memory": schema.SingleNestedBlock{
				MarkdownDescription: "Memory information.",
				Attributes: map[string]schema.Attribute{
					"physical_mb": schema.Int64Attribute{
						MarkdownDescription: "Physical memory in MB.",
						Computed:            true,
					},
					"total": schema.StringAttribute{
						MarkdownDescription: "Total memory as string.",
						Computed:            true,
					},
				},
			},
			"disks": schema.ListNestedBlock{
				MarkdownDescription: "List of discovered disks.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Disk device name.",
							Computed:            true,
						},
						"model": schema.StringAttribute{
							MarkdownDescription: "Disk model.",
							Computed:            true,
						},
						"size": schema.Int64Attribute{
							MarkdownDescription: "Disk size in bytes.",
							Computed:            true,
						},
						"rotational": schema.BoolAttribute{
							MarkdownDescription: "Whether the disk is rotational (HDD) or not (SSD).",
							Computed:            true,
						},
						"wwn": schema.StringAttribute{
							MarkdownDescription: "World Wide Name of the disk.",
							Computed:            true,
						},
						"wwn_with_extension": schema.StringAttribute{
							MarkdownDescription: "WWN with extension.",
							Computed:            true,
						},
						"wwn_vendor_extension": schema.StringAttribute{
							MarkdownDescription: "WWN vendor extension.",
							Computed:            true,
						},
						"serial": schema.StringAttribute{
							MarkdownDescription: "Disk serial number.",
							Computed:            true,
						},
					},
				},
			},
			"nics": schema.ListNestedBlock{
				MarkdownDescription: "List of discovered network interfaces.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Interface name.",
							Computed:            true,
						},
						"mac": schema.StringAttribute{
							MarkdownDescription: "MAC address.",
							Computed:            true,
						},
						"ipv4": schema.StringAttribute{
							MarkdownDescription: "IPv4 address.",
							Computed:            true,
						},
						"ipv6": schema.StringAttribute{
							MarkdownDescription: "IPv6 address.",
							Computed:            true,
						},
						"has_carrier": schema.BoolAttribute{
							MarkdownDescription: "Whether the interface has carrier signal.",
							Computed:            true,
						},
						"lldp_processed": schema.BoolAttribute{
							MarkdownDescription: "Whether LLDP data was processed for this interface.",
							Computed:            true,
						},
						"product": schema.StringAttribute{
							MarkdownDescription: "Network interface product name.",
							Computed:            true,
						},
						"vendor": schema.StringAttribute{
							MarkdownDescription: "Network interface vendor.",
							Computed:            true,
						},
						"speed_mbps": schema.Int64Attribute{
							MarkdownDescription: "Interface speed in Mbps.",
							Computed:            true,
						},
					},
				},
			},
			"system": schema.SingleNestedBlock{
				MarkdownDescription: "System information.",
				Attributes: map[string]schema.Attribute{
					"product": schema.StringAttribute{
						MarkdownDescription: "System product name.",
						Computed:            true,
					},
					"family": schema.StringAttribute{
						MarkdownDescription: "System family.",
						Computed:            true,
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "System version.",
						Computed:            true,
					},
					"sku": schema.StringAttribute{
						MarkdownDescription: "System SKU.",
						Computed:            true,
					},
					"serial": schema.StringAttribute{
						MarkdownDescription: "System serial number.",
						Computed:            true,
					},
					"uuid": schema.StringAttribute{
						MarkdownDescription: "System UUID.",
						Computed:            true,
					},
					"manufacturer": schema.StringAttribute{
						MarkdownDescription: "System manufacturer.",
						Computed:            true,
					},
				},
			},
		},
	}
}

func (d *introspectionDataSource) Configure(
	ctx context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	clients, ok := req.ProviderData.(*Clients)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected *Clients, got: %T. Please report this issue to the provider developers.",
				req.ProviderData,
			),
		)
		return
	}

	d.clients = clients
}

func (d *introspectionDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var config introspectionDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get Ironic client
	client, err := d.clients.GetIronicClient()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Get Ironic Client",
			fmt.Sprintf("Unable to get Ironic client: %s", err),
		)
		return
	}

	nodeUUID := config.UUID.ValueString()
	tflog.Debug(ctx, "Getting inventory data for node", map[string]any{"uuid": nodeUUID})

	// Get inventory data using nodes.GetInventory
	inventoryData, err := nodes.GetInventory(ctx, client, nodeUUID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Get Node Inventory",
			fmt.Sprintf("Unable to get inventory data for node %s: %s", nodeUUID, err),
		)
		return
	}

	// Map inventory data to the model
	config.ID = types.StringValue(time.Now().UTC().String())

	// Basic inventory information
	if inventoryData != nil {
		config.Inventory = &introspectionInventoryModel{
			BmcAddress:    types.StringValue(inventoryData.Inventory.BmcAddress),
			BmcV6Address:  types.StringValue(""), // Not available in InventoryType
			BootInterface: types.StringValue(inventoryData.Inventory.Boot.PXEInterface),
		}

		// CPU information
		config.CPU = &introspectionCPUModel{
			Architecture: types.StringValue(inventoryData.Inventory.CPU.Architecture),
			Count:        types.Int64Value(int64(inventoryData.Inventory.CPU.Count)),
			Frequency:    types.StringValue(inventoryData.Inventory.CPU.Frequency),
			ModelName:    types.StringValue(inventoryData.Inventory.CPU.ModelName),
		}

		// Convert CPU flags
		cpuFlags, diags := types.ListValueFrom(
			ctx,
			types.StringType,
			inventoryData.Inventory.CPU.Flags,
		)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		config.CPU.Flags = cpuFlags

		// Memory information
		config.Memory = &introspectionMemoryModel{
			PhysicalMb: types.Int64Value(int64(inventoryData.Inventory.Memory.PhysicalMb)),
			Total: types.StringValue(
				fmt.Sprintf("%d", inventoryData.Inventory.Memory.Total),
			),
		}

		// Disk information
		if len(inventoryData.Inventory.Disks) > 0 {
			disks := make([]introspectionDiskModel, len(inventoryData.Inventory.Disks))
			for i, disk := range inventoryData.Inventory.Disks {
				disks[i] = introspectionDiskModel{
					Name:         types.StringValue(disk.Name),
					Model:        types.StringValue(disk.Model),
					Size:         types.Int64Value(disk.Size),
					Rotational:   types.BoolValue(disk.Rotational),
					WWN:          types.StringValue(disk.Wwn),
					WWNWithExt:   types.StringValue(disk.WwnWithExtension),
					WWNVendorExt: types.StringValue(disk.WwnVendorExtension),
					Serial:       types.StringValue(disk.Serial),
				}
			}
			config.Disks = disks
		}

		// NIC information
		if len(inventoryData.Inventory.Interfaces) > 0 {
			nics := make([]introspectionNICModel, len(inventoryData.Inventory.Interfaces))
			for i, nic := range inventoryData.Inventory.Interfaces {
				nics[i] = introspectionNICModel{
					Name:          types.StringValue(nic.Name),
					MAC:           types.StringValue(nic.MACAddress),
					IPV4:          types.StringValue(nic.IPV4Address),
					IPV6:          types.StringValue(nic.IPV6Address),
					HasCarrier:    types.BoolValue(nic.HasCarrier),
					LLDPProcessed: types.BoolValue(false), // Not available in InventoryType
					Product:       types.StringValue(nic.Product),
					Vendor:        types.StringValue(nic.Vendor),
					SpeedMbps:     types.Int64Value(int64(nic.SpeedMbps)),
				}
			}
			config.NICs = nics
		}

		// System information
		config.System = &introspectionSystemModel{
			Product: types.StringValue(
				inventoryData.Inventory.SystemVendor.ProductName,
			),
			Family: types.StringValue(""), // Not available in InventoryType
			Version: types.StringValue(
				inventoryData.Inventory.SystemVendor.Firmware.Version,
			),
			SKU: types.StringValue(""), // Not available in InventoryType
			Serial: types.StringValue(
				inventoryData.Inventory.SystemVendor.SerialNumber,
			),
			UUID: types.StringValue(""), // Not available in InventoryType
			Manufacturer: types.StringValue(
				inventoryData.Inventory.SystemVendor.Manufacturer,
			),
		}
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
