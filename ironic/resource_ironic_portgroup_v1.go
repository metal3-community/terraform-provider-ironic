package ironic

import (
	"context"
	"encoding/json"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/portgroups"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourcePortGroupV1() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourcePortGroupV1Create,
		ReadContext:   resourcePortGroupV1Read,
		DeleteContext: resourcePortGroupV1Delete,
		Importer: &schema.ResourceImporter{
			StateContext: resourcePortGroupV1Import,
		},

		Schema: map[string]*schema.Schema{
			"uuid": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "The UUID of the port group. If not specified, a new UUID will be generated.",
			},
			"node_uuid": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"address": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},
			"mode": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "active-backup",
			},
			"extra": {
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: true,
			},
		},
	}
}

func resourcePortGroupV1Create(
	ctx context.Context,
	d *schema.ResourceData,
	meta any,
) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	opts := portGroupSchemaToCreateOpts(ctx, d)
	result, err := portgroups.Create(ctx, client, opts).Extract()
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(result.UUID)

	return resourcePortGroupV1Read(ctx, d, meta)
}

func resourcePortGroupV1Read(
	ctx context.Context,
	d *schema.ResourceData,
	meta any,
) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	port, err := portgroups.Get(ctx, client, d.Id()).Extract()
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
	err = d.Set("mode", port.Mode)
	if err != nil {
		return diag.FromErr(err)
	}
	err = d.Set("name", port.Name)
	if err != nil {
		return diag.FromErr(err)
	}

	var extra map[string]string

	if ex, ok := d.Get("extra").(map[string]string); ok {
		extra = ex
	} else {
		extra = make(map[string]string)
	}

	for k, v := range port.Extra {
		if vs, ok := v.(string); ok {
			extra[k] = vs
		}
	}

	return diag.FromErr(d.Set("extra", extra))
}

func resourcePortGroupV1Import(
	ctx context.Context,
	d *schema.ResourceData,
	meta any,
) ([]*schema.ResourceData, error) {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return nil, err
	}

	port, err := portgroups.Get(ctx, client, d.Id()).Extract()
	if err != nil {
		return nil, err
	}

	err = d.Set("address", port.Address)
	if err != nil {
		return nil, err
	}
	err = d.Set("node_uuid", port.NodeUUID)
	if err != nil {
		return nil, err
	}
	err = d.Set("mode", port.Mode)
	if err != nil {
		return nil, err
	}
	err = d.Set("name", port.Name)
	if err != nil {
		return nil, err
	}

	var extra map[string]string

	if ex, ok := d.Get("extra").(map[string]string); ok {
		extra = ex
	} else {
		extra = make(map[string]string)
	}

	for k, v := range port.Extra {
		if vs, ok := v.(string); ok {
			extra[k] = vs
		}
	}

	if err := d.Set("extra", extra); err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}

func resourcePortGroupV1Delete(
	ctx context.Context,
	d *schema.ResourceData,
	meta any,
) diag.Diagnostics {
	client, err := GetIronicClient(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	portgroups.Delete(ctx, client, d.Id())

	return nil
}

func portGroupSchemaToCreateOpts(
	ctx context.Context,
	d *schema.ResourceData,
) *portgroups.CreateOpts {
	extra := make(map[string]string)
	if v, ok := d.Get("extra").(map[string]any); ok {
		for k, val := range v {
			if sval, ok := val.(string); ok {
				extra[k] = sval
			} else {
				sb, err := json.Marshal(val)
				if err != nil {
					continue // Skip if we can't marshal the value
				}
				extra[k] = string(sb)
			}
		}
	}

	opts := portgroups.CreateOpts{
		NodeUUID: d.Get("node_uuid").(string),
		Address:  d.Get("address").(string),
		Name:     d.Get("name").(string),
		Mode:     d.Get("mode").(string),
		UUID:     d.Get("uuid").(string),
		Extra:    extra,
	}

	return &opts
}
