package ironic

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// StateTransition represents a valid state transition.
type StateTransition struct {
	From   nodes.ProvisionState
	Target nodes.TargetProvisionState
	To     nodes.ProvisionState
}

// stateTransitions defines the valid state transitions based on the state diagram.
var stateTransitions = []StateTransition{
	// From enroll
	{nodes.Enroll, nodes.TargetManage, nodes.Manageable},

	// From error
	{nodes.Error, nodes.TargetDeleted, nodes.Deploying},

	// From manageable
	{nodes.Manageable, nodes.TargetManage, nodes.Enroll},
	{nodes.Manageable, nodes.TargetInspect, nodes.Inspecting},
	{nodes.Manageable, nodes.TargetClean, nodes.Cleaning},
	{nodes.Manageable, nodes.TargetAdopt, nodes.Adopting},

	// From inspecting
	{nodes.Inspecting, nodes.TargetManage, nodes.Manageable}, // success

	// From cleaning
	{nodes.Cleaning, nodes.TargetProvide, nodes.Available}, // success
	{nodes.Cleaning, nodes.TargetAbort, nodes.Manageable},  // abort
	{nodes.Cleaning, nodes.TargetClean, nodes.Cleaning},    // clean again

	// From available
	{nodes.Available, nodes.TargetActive, nodes.Deploying},

	// From deploying
	{nodes.Deploying, nodes.TargetActive, nodes.Active},     // success
	{nodes.DeployFail, nodes.TargetDeleted, nodes.Deleting}, // failure
	{nodes.Deploying, nodes.TargetAbort, nodes.DeployFail},  // abort

	// From active
	{nodes.Active, nodes.TargetDeleted, nodes.Deleting},
	{nodes.Active, nodes.TargetRebuild, nodes.Deploying},
	{nodes.Active, nodes.TargetRescue, nodes.Rescuing},

	// From deleting
	{nodes.Deleting, nodes.TargetProvide, nodes.Available}, // success

	// From rescuing
	{nodes.Rescuing, nodes.TargetRescue, nodes.Rescue}, // success

	// From rescued
	{nodes.Rescue, nodes.TargetUnrescue, nodes.Unrescuing},

	// From unrescuing
	{nodes.Unrescuing, nodes.TargetActive, nodes.Active}, // success

	// From adopting
	{nodes.Adopting, nodes.TargetManage, nodes.Manageable}, // success
}

// getValidTransitions returns the valid transitions from a given state.
func getValidTransitions(from nodes.ProvisionState) []StateTransition {
	var transitions []StateTransition
	for _, transition := range stateTransitions {
		if transition.From == from {
			transitions = append(transitions, transition)
		}
	}
	return transitions
}

// isValidTransition checks if a transition from one state to a target is valid.
func isValidTransition(from nodes.ProvisionState, target nodes.TargetProvisionState) bool {
	for _, transition := range stateTransitions {
		if transition.From == from && transition.Target == target {
			return true
		}
	}
	return false
}

// getExpectedState returns the expected state after a successful transition.
func getExpectedState(
	from nodes.ProvisionState,
	target nodes.TargetProvisionState,
) (nodes.ProvisionState, bool) {
	for _, transition := range stateTransitions {
		if transition.From == from && transition.Target == target {
			return transition.To, true
		}
	}
	return "", false
}

// isTerminalFailureState checks if a state represents a terminal failure.
func isTerminalFailureState(state nodes.ProvisionState) bool {
	terminalStates := []nodes.ProvisionState{
		nodes.InspectFail,
		nodes.CleanFail,
		nodes.DeployFail,
		nodes.Error,
		nodes.RescueFail,
		nodes.AdoptFail,
	}

	return slices.Contains(terminalStates, state)
}

// isTransientState checks if a state is transient (will change automatically).
func isTransientState(state nodes.ProvisionState) bool {
	transientStates := []nodes.ProvisionState{
		nodes.Inspecting,
		nodes.Cleaning,
		nodes.Deploying,
		nodes.Deleting,
		nodes.Rescuing,
		nodes.Unrescuing,
		nodes.Adopting,
		nodes.CleanWait,
	}

	return slices.Contains(transientStates, state)
}

// ChangeProvisionStateToTarget triggers a provision state change on a node.
func ChangeProvisionStateToTarget(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	nodeID string,
	target nodes.TargetProvisionState,
	configDrive any,
	deploySteps []nodes.DeployStep,
	cleanSteps []nodes.CleanStep,
) error {
	// Get current node state
	node, err := nodes.Get(ctx, client, nodeID).Extract()
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeID, err)
	}

	currentState := nodes.ProvisionState(node.ProvisionState)
	tflog.Info(ctx, "Starting provision state change", map[string]any{
		"node_id":       nodeID,
		"current_state": string(currentState),
		"target":        string(target),
	})

	// Check if we're already in the desired final state
	if expectedState, ok := getExpectedState(currentState, target); ok {
		if currentState == expectedState {
			tflog.Info(ctx, "Node already in target state", map[string]any{
				"node_id": nodeID,
				"state":   string(currentState),
			})
			return nil
		}
	}

	// Perform the state change workflow
	workflow := &provisionWorkflow{
		ctx:         ctx,
		client:      client,
		nodeID:      nodeID,
		target:      target,
		configDrive: configDrive,
		deploySteps: deploySteps,
		cleanSteps:  cleanSteps,
	}

	return workflow.execute()
}

// provisionWorkflow manages the state machine execution.
type provisionWorkflow struct {
	ctx         context.Context
	client      *gophercloud.ServiceClient
	nodeID      string
	target      nodes.TargetProvisionState
	configDrive any
	deploySteps []nodes.DeployStep
	cleanSteps  []nodes.CleanStep
}

// execute runs the provision workflow.
func (w *provisionWorkflow) execute() error {
	const (
		maxAttempts  = 1000
		pollInterval = 15 * time.Second
		maxTimeout   = 30 * time.Minute
	)

	timeout := time.After(maxTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-w.ctx.Done():
			return fmt.Errorf("context cancelled: %w", w.ctx.Err())

		case <-timeout:
			return fmt.Errorf("timeout waiting for provision state change after %v", maxTimeout)

		case <-ticker.C:
			// Get current node state
			node, err := nodes.Get(w.ctx, w.client, w.nodeID).Extract()
			if err != nil {
				tflog.Warn(w.ctx, "Failed to get node during workflow", map[string]any{
					"node_id": w.nodeID,
					"error":   err.Error(),
				})
				continue
			}

			currentState := nodes.ProvisionState(node.ProvisionState)
			tflog.Debug(w.ctx, "Checking workflow progress", map[string]any{
				"node_id":       w.nodeID,
				"current_state": string(currentState),
				"target":        string(w.target),
				"attempt":       attempt,
			})

			// Check if we've reached the final desired state
			if done, err := w.checkCompletion(currentState); done {
				return err
			}

			// Determine next action
			if err := w.takeNextAction(currentState); err != nil {
				if isTerminalFailureState(currentState) {
					return fmt.Errorf(
						"workflow failed in terminal state '%s': %w",
						currentState,
						err,
					)
				}
				tflog.Warn(w.ctx, "Action failed, will retry", map[string]any{
					"node_id": w.nodeID,
					"error":   err.Error(),
				})
			}
		}
	}

	return fmt.Errorf("workflow failed after %d attempts", maxAttempts)
}

// checkCompletion determines if the workflow is complete.
func (w *provisionWorkflow) checkCompletion(currentState nodes.ProvisionState) (bool, error) {
	// Check if we're in a terminal failure state
	if isTerminalFailureState(currentState) {
		return true, fmt.Errorf("node %s in terminal failure state: %s", w.nodeID, currentState)
	}

	// Check if we've reached the final desired state for the target
	switch w.target {
	case nodes.TargetManage:
		return currentState == nodes.Manageable, nil
	case nodes.TargetInspect:
		return currentState == nodes.Manageable, nil // Inspection ends in manageable
	case nodes.TargetClean:
		return currentState == nodes.Available, nil // Cleaning ends in available
	case nodes.TargetProvide:
		return currentState == nodes.Available, nil
	case nodes.TargetActive:
		return currentState == nodes.Active, nil
	case nodes.TargetDeleted:
		return currentState == nodes.Available || currentState == nodes.Manageable, nil
	case nodes.TargetRescue:
		return currentState == nodes.Rescue, nil
	case nodes.TargetUnrescue:
		return currentState == nodes.Active, nil
	case nodes.TargetAdopt:
		return currentState == nodes.Manageable, nil
	default:
		return true, fmt.Errorf("unknown target: %s", w.target)
	}
}

// takeNextAction determines and executes the next action needed.
func (w *provisionWorkflow) takeNextAction(currentState nodes.ProvisionState) error {
	// If we're in a transient state, just wait
	if isTransientState(currentState) {
		tflog.Debug(w.ctx, "Waiting for transient state to complete", map[string]any{
			"node_id": w.nodeID,
			"state":   string(currentState),
		})
		return nil
	}

	// Determine what action to take based on current state and target
	nextTarget := w.determineNextTarget(currentState)
	if nextTarget == "" {
		return fmt.Errorf(
			"no valid transition from state '%s' for target '%s'",
			currentState,
			w.target,
		)
	}

	// Execute the state change
	return w.changeProvisionState(nextTarget)
}

// determineNextTarget figures out the next target based on current state and desired end goal.
func (w *provisionWorkflow) determineNextTarget(
	currentState nodes.ProvisionState,
) nodes.TargetProvisionState {
	// Direct transitions first
	if isValidTransition(currentState, w.target) {
		return w.target
	}

	// Multi-step workflows - determine intermediate steps
	switch w.target {
	case nodes.TargetActive:
		if currentState == nodes.Enroll {
			return nodes.TargetManage
		}
		if currentState == nodes.Manageable {
			return nodes.TargetProvide
		}
		if currentState == nodes.Available {
			return nodes.TargetActive
		}

	case nodes.TargetProvide:
		if currentState == nodes.Enroll {
			return nodes.TargetManage
		}
		if currentState == nodes.Manageable {
			return nodes.TargetProvide
		}

	case nodes.TargetDeleted:
		if currentState == nodes.Active {
			return nodes.TargetDeleted
		}

	case nodes.TargetInspect:
		if currentState == nodes.Enroll {
			return nodes.TargetManage
		}
		if currentState == nodes.Manageable {
			return nodes.TargetInspect
		}
		if currentState == nodes.Available {
			return nodes.TargetManage
		}

	case nodes.TargetClean:
		if currentState == nodes.Enroll {
			return nodes.TargetManage
		}
		if currentState == nodes.Manageable {
			return nodes.TargetClean
		}
	}

	return ""
}

// changeProvisionState executes a provision state change.
func (w *provisionWorkflow) changeProvisionState(target nodes.TargetProvisionState) error {
	opts := nodes.ProvisionStateOpts{
		Target: target,
	}

	// Add additional options based on target
	switch target {
	case nodes.TargetActive:
		opts.ConfigDrive = w.configDrive
		if w.deploySteps != nil {
			opts.DeploySteps = w.deploySteps
		}
	case nodes.TargetClean:
		if w.cleanSteps != nil {
			opts.CleanSteps = w.cleanSteps
		} else {
			opts.CleanSteps = []nodes.CleanStep{}
		}
	}

	tflog.Info(w.ctx, "Executing provision state change", map[string]any{
		"node_id": w.nodeID,
		"target":  string(target),
	})

	err := nodes.ChangeProvisionState(w.ctx, w.client, w.nodeID, opts).ExtractErr()
	if err != nil {
		return fmt.Errorf("failed to change provision state to '%s': %w", target, err)
	}

	return nil
}

// WaitForTargetProvisionState waits for a node to reach a specific state.
func WaitForTargetProvisionState(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	nodeID string,
	targetState nodes.ProvisionState,
) error {
	const (
		pollInterval = 10 * time.Second
		maxTimeout   = 30 * time.Minute
	)

	timeout := time.After(maxTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	tflog.Debug(ctx, "Waiting for provision state", map[string]any{
		"node_id":       nodeID,
		"target_state":  string(targetState),
		"poll_interval": pollInterval.String(),
		"max_timeout":   maxTimeout.String(),
	})

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for provision state: %w", ctx.Err())

		case <-timeout:
			return fmt.Errorf("timeout waiting for node %s to reach state '%s' after %v",
				nodeID, targetState, maxTimeout)

		case <-ticker.C:
			node, err := nodes.Get(ctx, client, nodeID).Extract()
			if err != nil {
				tflog.Warn(ctx, "Failed to get node during state wait", map[string]any{
					"node_id": nodeID,
					"error":   err.Error(),
				})
				continue
			}

			currentState := nodes.ProvisionState(node.ProvisionState)
			tflog.Debug(ctx, "Checking provision state", map[string]any{
				"node_id":       nodeID,
				"current_state": string(currentState),
				"target_state":  string(targetState),
				"last_error":    node.LastError,
			})

			// Check if we've reached the target state
			if currentState == targetState {
				return nil
			}

			// Check if we're in a terminal failure state
			if isTerminalFailureState(currentState) {
				errorMsg := "unknown error"
				if node.LastError != "" {
					errorMsg = node.LastError
				}
				return fmt.Errorf("node %s entered terminal failure state '%s': %s",
					nodeID, currentState, errorMsg)
			}
		}
	}
}

// GetNodeProvisionState returns the current provision state of a node.
func GetNodeProvisionState(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	nodeID string,
) (nodes.ProvisionState, error) {
	node, err := nodes.Get(ctx, client, nodeID).Extract()
	if err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeID, err)
	}

	return nodes.ProvisionState(node.ProvisionState), nil
}

// ValidateProvisionState checks if a provision state string is valid.
func ValidateProvisionState(state string) error {
	validStates := []nodes.ProvisionState{
		nodes.Enroll, nodes.Manageable, nodes.Inspecting, nodes.InspectFail,
		nodes.Cleaning, nodes.CleanFail, nodes.CleanWait, nodes.Available,
		nodes.Active, nodes.Deploying, nodes.DeployFail, nodes.Deleting,
		nodes.Error, nodes.Rescuing, nodes.Rescue, nodes.RescueFail,
		nodes.Unrescuing, nodes.Adopting, nodes.AdoptFail,
	}

	if slices.Contains(validStates, nodes.ProvisionState(state)) {
		return nil
	}

	return fmt.Errorf("invalid provision state: %s", state)
}

// GetValidTargetsFromState returns the valid provision targets from a given state.
func GetValidTargetsFromState(state nodes.ProvisionState) []nodes.TargetProvisionState {
	var targets []nodes.TargetProvisionState
	transitions := getValidTransitions(state)

	for _, transition := range transitions {
		targets = append(targets, transition.Target)
	}

	return targets
}

// ProvisionStateError represents an error with provision state context.
type ProvisionStateError struct {
	NodeID       string
	CurrentState nodes.ProvisionState
	TargetState  nodes.TargetProvisionState
	Err          error
}

func (e *ProvisionStateError) Error() string {
	return fmt.Sprintf("provision state error for node %s (current: %s, target: %s): %v",
		e.NodeID, e.CurrentState, e.TargetState, e.Err)
}

func (e *ProvisionStateError) Unwrap() error {
	return e.Err
}

// AddProvisionStateError is a helper to add provision state errors to diagnostics.
func AddProvisionStateError(
	diags *diag.Diagnostics,
	nodeID string,
	currentState nodes.ProvisionState,
	targetState nodes.TargetProvisionState,
	err error,
) {
	provisionErr := &ProvisionStateError{
		NodeID:       nodeID,
		CurrentState: currentState,
		TargetState:  targetState,
		Err:          err,
	}

	diags.AddError(
		"Provision State Change Failed",
		provisionErr.Error(),
	)
}
