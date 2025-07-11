package ironic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/conductors"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Clients stores the client connection information for Ironic.
type Clients struct {
	ironic *gophercloud.ServiceClient
	// inspector *gophercloud.ServiceClient // No longer used - migrated to Ironic nodes API

	// Boolean that determines if Ironic API was previously determined to be available, we don't need to try every time.
	ironicUp bool

	// Boolean that determines we've already waited and the API never came up, we don't need to wait again.
	ironicFailed bool

	// Mutex so that only one resource being created by terraform checks at a time. There's no reason to have multiple
	// resources calling out to the API.
	ironicMux sync.Mutex

	// Boolean that determines if Inspector API was previously determined to be available, we don't need to try every time.
	// inspectorUp bool // No longer used - migrated to Ironic nodes API

	// Boolean that determines that we've already waited, and inspector API did not come up.
	// inspectorFailed bool // No longer used - migrated to Ironic nodes API

	// Mutex so that only one resource being created by terraform checks at a time. There's no reason to have multiple
	// resources calling out to the API.
	// inspectorMux sync.Mutex // No longer used - migrated to Ironic nodes API

	timeout int
}

// GetIronicClient returns the API client for Ironic, optionally retrying to reach the API if timeout is set.
func (c *Clients) GetIronicClient() (*gophercloud.ServiceClient, error) {
	// Terraform concurrently creates some resources which means multiple callers can request an Ironic client. We
	// only need to check if the API is available once, so we use a mux to restrict one caller to polling the API.
	// When the mux is released, the other callers will fall through to the check for ironicUp.
	c.ironicMux.Lock()
	defer c.ironicMux.Unlock()

	// Ironic is UP, or user didn't ask us to check
	if c.ironicUp || c.timeout == 0 {
		return c.ironic, nil
	}

	// We previously tried and it failed.
	if c.ironicFailed {
		return nil, fmt.Errorf("could not contact Ironic API: timeout reached")
	}

	// Let's poll the API until it's up, or times out.
	duration := time.Duration(c.timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	done := make(chan struct{})
	go func() {
		tflog.Info(ctx, "Waiting for Ironic API to become available")
		_ = healthCheck(ctx, c.ironic)
		close(done)
	}()

	// Wait for done or time out
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			c.ironicFailed = true
			return nil, fmt.Errorf("could not contact Ironic API: %w", err)
		}
	case <-done:
	}

	c.ironicUp = true
	return c.ironic, ctx.Err()
}

func GetIronicClient(ctx context.Context, meta any) (*gophercloud.ServiceClient, error) {
	client, ok := meta.(*Clients)
	if !ok {
		return nil, fmt.Errorf("expected meta to be of type *Clients, got %T", meta)
	}

	ironicClient, err := client.GetIronicClient()
	if err != nil {
		return nil, fmt.Errorf("could not get Ironic client: %w", err)
	}

	return ironicClient, nil
}

func healthCheck(ctx context.Context, client *gophercloud.ServiceClient) error {
	// Perform a simple health check by making a request to the API.
	// This is a placeholder for actual health check logic.
	pages, err := conductors.List(client, conductors.ListOpts{}).AllPages(ctx)
	if err != nil {
		return fmt.Errorf("ironic API health check failed: %w", err)
	}

	conductorsList, err := conductors.ExtractConductors(pages)
	if err != nil {
		return fmt.Errorf("failed to extract conductors from Ironic API response: %w",
			err)
	}
	for _, conductor := range conductorsList {
		if !conductor.Alive {
			tflog.Error(ctx, "Conductor is not alive", map[string]any{
				"hostname": conductor.Hostname,
				"alive":    conductor.Alive,
				"drivers":  conductor.Drivers,
			})
			return fmt.Errorf("ironic API health check failed: conductor %s is not alive",
				conductor.Hostname)
		}
		if len(conductor.Drivers) == 0 {
			tflog.Error(ctx, "Conductor has no drivers", map[string]any{
				"hostname": conductor.Hostname,
				"drivers":  conductor.Drivers,
			})
			return fmt.Errorf(
				"ironic API health check failed: conductor %s has no drivers",
				conductor.Hostname,
			)
		}
		tflog.Info(ctx, "Conductor is alive", map[string]any{
			"hostname":   conductor.Hostname,
			"drivers":    conductor.Drivers,
			"alive":      conductor.Alive,
			"group":      conductor.ConductorGroup,
			"created_at": conductor.CreatedAt,
			"updated_at": conductor.UpdatedAt,
		})
	}
	// If we reach here, the API is considered healthy.
	tflog.Info(ctx, "Ironic API is healthy", map[string]any{
		"client": client.Endpoint,
	})
	return nil
}
