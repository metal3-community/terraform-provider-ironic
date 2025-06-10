package ironic

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
)

const maxRetryNumber = 3

// provisionStateWorkflow is used to track state through the process of updating's it's provision state.
type provisionStateWorkflow struct {
	ctx         context.Context
	client      *gophercloud.ServiceClient
	node        nodes.Node
	uuid        string
	target      nodes.TargetProvisionState
	wait        time.Duration
	retryNumber int

	configDrive any
	deploySteps []nodes.DeployStep
	cleanSteps  []nodes.CleanStep

	operationStarted bool
}

// ChangeProvisionStateToTarget drives Ironic's state machine through the process to reach our desired end state. This requires multiple
// possibly long-running steps.  If required, we'll build a config drive ISO for deployment.
func ChangeProvisionStateToTarget(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	uuid string,
	target nodes.TargetProvisionState,
	configDrive any,
	deploySteps []nodes.DeployStep,
	cleanSteps []nodes.CleanStep,
) error {
	// Run the provisionStateWorkflow - this could take a while
	wf := provisionStateWorkflow{
		target:      target,
		client:      client,
		wait:        5 * time.Second,
		uuid:        uuid,
		configDrive: configDrive,
		deploySteps: deploySteps,
		cleanSteps:  cleanSteps,
		retryNumber: maxRetryNumber,
	}

	return wf.run()
}

// Keep driving the state machine forward.
func (workflow *provisionStateWorkflow) run() error {
	log.Printf(
		"[INFO] Beginning provisioning workflow, will try to change node to state '%s'",
		workflow.target,
	)

	for {
		log.Printf("[DEBUG] Node is in state '%s'", workflow.node.ProvisionState)

		done, err := workflow.next()
		if err != nil {
			_ = workflow.reloadNode() // to get the lastError
			return fmt.Errorf("%w , last error was '%s'", err, workflow.node.LastError)
		}
		if done {
			return nil
		}

		time.Sleep(workflow.wait)
	}
}

// Do the next thing to get us to our target state.
func (workflow *provisionStateWorkflow) next() (bool, error) {
	// Refresh the node on each run
	if err := workflow.reloadNode(); err != nil {
		return true, err
	}

	log.Printf(
		"[DEBUG] Node current state is '%s', target is %s",
		workflow.node.ProvisionState,
		workflow.target,
	)

	switch workflow.target {
	case nodes.TargetManage:
		return workflow.toManageable()
	case nodes.TargetProvide:
		return workflow.toAvailable()
	case nodes.TargetActive:
		return workflow.toActive()
	case nodes.TargetDeleted:
		return workflow.toDeleted()
	case nodes.TargetClean:
		return workflow.toClean()
	case nodes.TargetInspect:
		return workflow.toInspect()
	default:
		return true, fmt.Errorf("unknown target state '%s'", workflow.target)
	}
}

func (workflow *provisionStateWorkflow) maybeRetry() (bool, error) {
	state := workflow.node.ProvisionState
	if workflow.retryNumber == 0 {
		return true, errors.New(state)
	}

	workflow.retryNumber--
	workflow.operationStarted = false
	log.Printf("[DEBUG] Node %s is '%s', going to retry", workflow.uuid, state)

	if state == "deploy failed" {
		return workflow.changeProvisionState(nodes.TargetDeleted)
	}
	return workflow.changeProvisionState(nodes.TargetManage)
}

// Change a node to "manageable" stable.
func (workflow *provisionStateWorkflow) toManageable() (bool, error) {
	switch nodes.ProvisionState(workflow.node.ProvisionState) {
	case nodes.Manageable:
		// We're done!
		return true, nil
	case nodes.Enroll, nodes.Available:
		return workflow.changeProvisionState(nodes.TargetManage)
	case nodes.AdoptFail,
		nodes.CleanFail,
		nodes.InspectFail:
		return workflow.maybeRetry()
	case nodes.Verifying:
		// Not done, no error - Ironic is working
		return false, nil

	default:
		return true, fmt.Errorf(
			"cannot go from state '%s' to state 'manageable'",
			workflow.node.ProvisionState,
		)
	}
}

// Clean a node.
func (workflow *provisionStateWorkflow) toClean() (bool, error) {
	if !workflow.operationStarted {
		// Node must be manageable first
		if workflow.node.ProvisionState != string(nodes.Manageable) {
			if err := ChangeProvisionStateToTarget(workflow.ctx, workflow.client, workflow.uuid, nodes.TargetManage, nil, nil, nil); err != nil {
				return true, err
			}
		}

		// Set target to clean
		_, err := workflow.changeProvisionState(nodes.TargetClean)
		if err != nil {
			return true, err
		}

		// Marking that we should not return to manageable any more
		workflow.operationStarted = true
		return false, nil
	}

	switch nodes.ProvisionState(workflow.node.ProvisionState) {
	case nodes.Manageable:
		return true, nil
	case nodes.Cleaning, nodes.CleanWait:
		// Not done, no error - Ironic is working
		return false, nil
	case nodes.CleanFail:
		return workflow.maybeRetry()
	default:
		return true, fmt.Errorf(
			"could not clean node, node is currently '%s'",
			workflow.node.ProvisionState,
		)
	}
}

// Inspect a node.
func (workflow *provisionStateWorkflow) toInspect() (bool, error) {
	if !workflow.operationStarted {
		// Node must be manageable first
		if workflow.node.ProvisionState != string(nodes.Manageable) {
			if err := ChangeProvisionStateToTarget(workflow.ctx, workflow.client, workflow.uuid, nodes.TargetManage, nil, nil, nil); err != nil {
				return true, err
			}
		}

		// Set target to inspect
		_, err := workflow.changeProvisionState(nodes.TargetInspect)
		if err != nil {
			return true, err
		}

		// Marking that we should not return to manageable any more
		workflow.operationStarted = true
		return false, nil
	}

	switch nodes.ProvisionState(workflow.node.ProvisionState) {
	case nodes.Manageable:
		return true, nil
	case nodes.Inspecting, nodes.InspectWait:
		// Not done, no error - Ironic is working
		return false, nil
	case nodes.InspectFail:
		return workflow.maybeRetry()
	default:
		return true, fmt.Errorf(
			"could not inspect node, node is currently '%s'",
			workflow.node.ProvisionState,
		)
	}
}

// Change a node to "available" state.
func (workflow *provisionStateWorkflow) toAvailable() (bool, error) {
	switch nodes.ProvisionState(workflow.node.ProvisionState) {
	case nodes.Available:
		// We're done!
		return true, nil
	case nodes.Cleaning, nodes.CleanWait:
		// Not done, no error - Ironic is working
		log.Printf(
			"[DEBUG] Node %s is '%s', waiting for Ironic to finish.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		return false, nil
	case nodes.Manageable:
		// From manageable, we can go to provide
		log.Printf(
			"[DEBUG] Node %s is '%s', going to change to 'available'",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		return workflow.changeProvisionState(nodes.TargetProvide)
	case nodes.DeployFail:
		return workflow.maybeRetry()
	default:
		// Otherwise we have to get into manageable state first
		log.Printf(
			"[DEBUG] Node %s is '%s', going to change to 'manageable'.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		_, err := workflow.toManageable()
		if err != nil {
			return true, err
		}
		return false, nil
	}
}

// Change a node to "active" state.
func (workflow *provisionStateWorkflow) toActive() (bool, error) {
	switch nodes.ProvisionState(workflow.node.ProvisionState) {
	case nodes.Active:
		// We're done!
		log.Printf("[DEBUG] Node %s is 'active', we are done.", workflow.uuid)
		return true, nil
	case nodes.Deploying, nodes.DeployWait:
		// Not done, no error - Ironic is working
		log.Printf(
			"[DEBUG] Node %s is '%s', waiting for Ironic to finish.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		return false, nil
	case nodes.Available:
		// From available, we can go to active
		log.Printf("[DEBUG] Node %s is 'available', going to change to 'active'.", workflow.uuid)
		workflow.wait = 30 * time.Second // Deployment takes a while
		return workflow.changeProvisionState(nodes.TargetActive)
	default:
		// Otherwise we have to get into available state first
		log.Printf(
			"[DEBUG] Node %s is '%s', going to change to 'available'.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		_, err := workflow.toAvailable()
		if err != nil {
			return true, err
		}
		return false, nil
	}
}

// Change a node to be "deleted," and remove the object from Ironic.
func (workflow *provisionStateWorkflow) toDeleted() (bool, error) {
	switch nodes.ProvisionState(workflow.node.ProvisionState) {
	case nodes.Manageable, nodes.Available, nodes.Enroll:
		// We're done deleting the node
		return true, nil
	case nodes.Cleaning, nodes.Deleting:
		// Not done, no error - Ironic is working
		log.Printf(
			"[DEBUG] Node %s is '%s', waiting for Ironic to finish.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		return false, nil
	case nodes.Active, nodes.DeployWait, nodes.Error:
		log.Printf(
			"[DEBUG] Node %s is '%s', going to change to 'deleted'.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		return workflow.changeProvisionState(nodes.TargetDeleted)
	case nodes.DeployFail:
		return workflow.maybeRetry()
	case nodes.InspectFail, nodes.CleanFail:
		// We have to get into manageable state first
		log.Printf(
			"[DEBUG] Node %s is '%s', going to change to 'manageable'.",
			workflow.uuid,
			workflow.node.ProvisionState,
		)
		_, err := workflow.toManageable()
		if err != nil {
			return true, err
		}
		return false, nil
	default:
		return true, fmt.Errorf("cannot delete node in state '%s'", workflow.node.ProvisionState)
	}
}

// Builds the ProvisionStateOpts to send to Ironic -- including config drive.
func (workflow *provisionStateWorkflow) buildProvisionStateOpts(
	target nodes.TargetProvisionState,
) *nodes.ProvisionStateOpts {
	opts := nodes.ProvisionStateOpts{
		Target: target,
	}

	// If we're deploying, then build a config drive to send to Ironic
	if target == nodes.TargetActive {
		opts.ConfigDrive = workflow.configDrive

		if workflow.deploySteps != nil {
			opts.DeploySteps = workflow.deploySteps
		}
	}
	if target == nodes.TargetClean {
		if workflow.cleanSteps != nil {
			opts.CleanSteps = workflow.cleanSteps
		} else {
			opts.CleanSteps = []nodes.CleanStep{}
		}
	}

	return &opts
}

// Call Ironic's API and issue the change provision state request.
func (workflow *provisionStateWorkflow) changeProvisionState(
	target nodes.TargetProvisionState,
) (bool, error) {
	opts := workflow.buildProvisionStateOpts(target)

	if target == nodes.TargetClean && len(opts.CleanSteps) == 0 {
		return true, nil
	}

	interval := 5 * time.Second
	for range 5 {
		err := nodes.ChangeProvisionState(workflow.ctx, workflow.client, workflow.uuid, *opts).
			ExtractErr()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusConflict) {
				log.Printf(
					"[DEBUG] Failed to change provision state: ironic is busy, will retry in %s.",
					interval.String(),
				)
				time.Sleep(interval)
				interval *= 2
				continue
			}
		} else {
			return true, nil
		}
	}

	return false, fmt.Errorf(
		"failed to change provision state for node %s to %s",
		workflow.uuid,
		target,
	)
}

// Call Ironic's API and reload the node's current state.
func (workflow *provisionStateWorkflow) reloadNode() error {
	return nodes.Get(workflow.ctx, workflow.client, workflow.uuid).
		ExtractInto(&workflow.node)
}
