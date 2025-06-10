package ironic

import (
	"context"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/ports"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourcePortV1() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourcePortV1Create,
		ReadContext:   resourcePortV1Read,
		UpdateContext: resourcePortV1Update,
		DeleteContext: resourcePortV1Delete,

		Schema: map[string]*schema.Schema{
			"node_uuid": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"address": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"port_group_uuid": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"local_link_connection": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"pxe_enabled": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"physical_network": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"extra": {
				Type:     schema.TypeMap,
				Optional: true,
			},
			"is_smart_nic": {
				Type:     schema.TypeBool,
				Optional: true,
			},
		},
	}
}

func resourcePortV1Create(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	opts := portSchemaToCreateOpts(ctx, d)
	result, err := ports.Create(ctx, client, opts).Extract()
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(result.UUID)

	return resourcePortV1Read(ctx, d, meta)
}

func resourcePortV1Read(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	port, err := ports.Get(ctx, client, d.Id()).Extract()
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("address", port.Address)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("node_uuid", port.NodeUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("port_group_uuid", port.PortGroupUUID)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("local_link_connection", port.LocalLinkConnection)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("pxe_enabled", port.PXEEnabled)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("physical_network", port.PhysicalNetwork)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("extra", port.Extra)
	if err != nil {
		return diag.FromErr(err)
	}
	return diag.FromErr(d.Set("is_smart_nic", port.IsSmartNIC))
}

func resourcePortV1Update(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	updateOpts := ports.UpdateOpts{}

	if d.HasChange("address") {
		old, newState := d.GetChange("address")
		if old == nil {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.AddOp,
				Path:  "/address",
				Value: newState.(string),
			})
		} else {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/address",
				Value: newState.(string),
			})
		}
	}

	if d.HasChange("physical_network") {
		old, newState := d.GetChange("physical_network")
		if (old == nil || old == "") && (newState != nil && newState != "") {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.AddOp,
				Path:  "/physical_network",
				Value: newState.(string),
			})
		} else if (newState == nil || newState == "") && (old != nil && old != "") {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:   ports.RemoveOp,
				Path: "/physical_network",
			})
		} else if old != newState {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/physical_network",
				Value: newState.(string),
			})
		}
	}

	if d.HasChange("node_uuid") {
		old, newState := d.GetChange("node_uuid")
		if old == nil {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.AddOp,
				Path:  "/node_uuid",
				Value: newState.(string),
			})
		} else {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/node_uuid",
				Value: newState.(string),
			})
		}
	}

	if d.HasChange("port_group_uuid") {
		old, newState := d.GetChange("port_group_uuid")
		if old == nil {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.AddOp,
				Path:  "/port_group_uuid",
				Value: newState.(string),
			})
		} else {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/port_group_uuid",
				Value: newState.(string),
			})
		}
	}

	if d.HasChange("local_link_connection") {
		old, newState := d.GetChange("local_link_connection")
		if old == nil {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.AddOp,
				Path:  "/local_link_connection",
				Value: newState.(map[string]any),
			})
		} else {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/local_link_connection",
				Value: newState.(map[string]any),
			})
		}
	}

	if d.HasChange("pxe_enabled") {
		old, newState := d.GetChange("pxe_enabled")
		if old == nil {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.AddOp,
				Path:  "/pxe_enabled",
				Value: newState.(bool),
			})
		} else {
			updateOpts = append(updateOpts, ports.UpdateOperation{
				Op:    ports.ReplaceOp,
				Path:  "/pxe_enabled",
				Value: newState.(bool),
			})
		}
	}

	ports.Update(ctx, client, d.Id(), updateOpts)

	return nil
}

func resourcePortV1Delete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	ports.Delete(ctx, client, d.Id())

	return nil
}

func portSchemaToCreateOpts(_ context.Context, d *schema.ResourceData) *ports.CreateOpts {
	pxeEnabled := d.Get("pxe_enabled").(bool)
	isSmartNic := d.Get("is_smart_nic").(bool)

	opts := ports.CreateOpts{
		NodeUUID:            d.Get("node_uuid").(string),
		Address:             d.Get("address").(string),
		PortGroupUUID:       d.Get("port_group_uuid").(string),
		LocalLinkConnection: d.Get("local_link_connection").(map[string]any),
		PXEEnabled:          &pxeEnabled,
		PhysicalNetwork:     d.Get("physical_network").(string),
		Extra:               d.Get("extra").(map[string]any),
		IsSmartNIC:          &isSmartNic,
	}

	return &opts
}
