package ironic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/hashicorp/terraform-plugin-log/tfsdklog"
)

// Clients stores the client connection information for Ironic and Inspector.
type Clients struct {
	ironic    *gophercloud.ServiceClient
	inspector *gophercloud.ServiceClient

	// Boolean that determines if Ironic API was previously determined to be available, we don't need to try every time.
	ironicUp bool

	// Boolean that determines we've already waited and the API never came up, we don't need to wait again.
	ironicFailed bool

	// Mutex so that only one resource being created by terraform checks at a time. There's no reason to have multiple
	// resources calling out to the API.
	ironicMux sync.Mutex

	// Boolean that determines if Inspector API was previously determined to be available, we don't need to try every time.
	inspectorUp bool

	// Boolean that determines that we've already waited, and inspector API did not come up.
	inspectorFailed bool

	// Mutex so that only one resource being created by terraform checks at a time. There's no reason to have multiple
	// resources calling out to the API.
	inspectorMux sync.Mutex

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
		tfsdklog.Info(ctx, "Waiting for Ironic API to become available")
		waitForAPI(ctx, c.ironic)
		tfsdklog.Info(ctx, "Ironic API is up, waiting for conductor to be available")
		waitForConductor(ctx, c.ironic)
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

// GetInspectorClient returns the API client for Ironic, optionally retrying to reach the API if timeout is set.
func (c *Clients) GetInspectorClient() (*gophercloud.ServiceClient, error) {
	// Terraform concurrently creates some resources which means multiple callers can request an Inspector client. We
	// only need to check if the API is available once, so we use a mux to restrict one caller to polling the API.
	// When the mux is released, the other callers will fall through to the check for inspectorUp.
	c.inspectorMux.Lock()
	defer c.inspectorMux.Unlock()

	if c.inspector == nil {
		return nil, fmt.Errorf("no inspector endpoint was specified")
	} else if c.inspectorUp || c.timeout == 0 {
		return c.inspector, nil
	} else if c.inspectorFailed {
		return nil, fmt.Errorf("could not contact Inspector API: timeout reached")
	}

	// Let's poll the API until it's up, or times out.
	duration := time.Duration(c.timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	done := make(chan struct{})
	go func() {
		tfsdklog.Info(ctx, "Waiting for Inspector API to become available")
		waitForAPI(ctx, c.inspector)
		close(done)
	}()

	// Wait for done or time out
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			c.ironicFailed = true
			return nil, err
		}
	case <-done:
	}

	if err := ctx.Err(); err != nil {
		c.inspectorFailed = true
		return nil, err
	}

	c.inspectorUp = true
	return c.inspector, ctx.Err()
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

	// Ensure the API is available before returning the client.
	waitForAPI(ctx, ironicClient)

	return ironicClient, nil
}
