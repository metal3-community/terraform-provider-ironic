package ironic

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ProvisionState represents the various states a node can be in.
type ProvisionState string

const (
	// Initial states.
	StateEnroll ProvisionState = "enroll"

	// Management states.
	StateManageable ProvisionState = "manageable"

	// Inspection states.
	StateInspecting  ProvisionState = "inspecting"
	StateInspectFail ProvisionState = "inspect failed"

	// Cleaning states.
	StateCleaning  ProvisionState = "cleaning"
	StateCleanFail ProvisionState = "clean failed"
	StateCleanWait ProvisionState = "clean wait"

	// Available and deployment states.
	StateAvailable  ProvisionState = "available"
	StateActive     ProvisionState = "active"
	StateDeploying  ProvisionState = "deploying"
	StateDeployFail ProvisionState = "deploy failed"
	StateDeleting   ProvisionState = "deleting"
	StateDeleteFail ProvisionState = "delete failed"

	// Rescue states.
	StateRescuing   ProvisionState = "rescuing"
	StateRescued    ProvisionState = "rescued"
	StateRescueFail ProvisionState = "rescue failed"
	StateUnrescuing ProvisionState = "unrescuing"

	// Adoption states.
	StateAdopting  ProvisionState = "adopting"
	StateAdoptFail ProvisionState = "adopt failed"
)

// ProvisionTarget represents the valid targets for state transitions.
type ProvisionTarget = nodes.TargetProvisionState

const (
	TargetManage    ProvisionTarget = nodes.TargetManage
	TargetUnmanage  ProvisionTarget = nodes.TargetManage // Use manage to go back to manageable from enroll
	TargetInspect   ProvisionTarget = nodes.TargetInspect
	TargetClean     ProvisionTarget = nodes.TargetClean
	TargetAvailable ProvisionTarget = nodes.TargetProvide
	TargetActive    ProvisionTarget = nodes.TargetActive
	TargetDeleted   ProvisionTarget = nodes.TargetDeleted
	TargetRescue    ProvisionTarget = nodes.TargetRescue
	TargetUnrescue  ProvisionTarget = nodes.TargetUnrescue
	TargetAdopt     ProvisionTarget = nodes.TargetAdopt
	TargetAbort     ProvisionTarget = nodes.TargetAbort
	TargetRebuild   ProvisionTarget = nodes.TargetRebuild
)

// StateTransition represents a valid state transition.
type StateTransition struct {
	From   ProvisionState
	Target ProvisionTarget
	To     ProvisionState
}

// stateTransitions defines the valid state transitions based on the state diagram.
var stateTransitions = []StateTransition{
	// From enroll
	{StateEnroll, TargetManage, StateManageable},

	// From manageable
	{StateManageable, TargetUnmanage, StateEnroll},
	{StateManageable, TargetInspect, StateInspecting},
	{StateManageable, TargetClean, StateCleaning},
	{StateManageable, TargetAdopt, StateAdopting},

	// From inspecting
	{StateInspecting, TargetManage, StateManageable}, // success

	// From cleaning
	{StateCleaning, TargetAvailable, StateAvailable}, // success
	{StateCleaning, TargetAbort, StateManageable},    // abort
	{StateCleaning, TargetClean, StateCleaning},      // clean again

	// From available
	{StateAvailable, TargetActive, StateDeploying},

	// From deploying
	{StateDeploying, TargetActive, StateActive},     // success
	{StateDeployFail, TargetDeleted, StateDeleting}, // failure
	{StateDeploying, TargetAbort, StateDeployFail},  // abort

	// From active
	{StateActive, TargetDeleted, StateDeleting},
	{StateActive, TargetRebuild, StateDeploying},
	{StateActive, TargetRescue, StateRescuing},

	// From deleting
	{StateDeleting, TargetAvailable, StateAvailable}, // success

	// From rescuing
	{StateRescuing, TargetRescue, StateRescued}, // success

	// From rescued
	{StateRescued, TargetUnrescue, StateUnrescuing},

	// From unrescuing
	{StateUnrescuing, TargetActive, StateActive}, // success

	// From adopting
	{StateAdopting, TargetManage, StateManageable}, // success
}

// getValidTransitions returns the valid transitions from a given state.
func getValidTransitions(from ProvisionState) []StateTransition {
	var transitions []StateTransition
	for _, transition := range stateTransitions {
		if transition.From == from {
			transitions = append(transitions, transition)
		}
	}
	return transitions
}

// isValidTransition checks if a transition from one state to a target is valid.
func isValidTransition(from ProvisionState, target ProvisionTarget) bool {
	for _, transition := range stateTransitions {
		if transition.From == from && transition.Target == target {
			return true
		}
	}
	return false
}

// getExpectedState returns the expected state after a successful transition.
func getExpectedState(from ProvisionState, target ProvisionTarget) (ProvisionState, bool) {
	for _, transition := range stateTransitions {
		if transition.From == from && transition.Target == target {
			return transition.To, true
		}
	}
	return "", false
}

// isTerminalFailureState checks if a state represents a terminal failure.
func isTerminalFailureState(state ProvisionState) bool {
	terminalStates := []ProvisionState{
		StateInspectFail,
		StateCleanFail,
		StateDeployFail,
		StateDeleteFail,
		StateRescueFail,
		StateAdoptFail,
	}

	for _, terminalState := range terminalStates {
		if state == terminalState {
			return true
		}
	}
	return false
}

// isTransientState checks if a state is transient (will change automatically).
func isTransientState(state ProvisionState) bool {
	transientStates := []ProvisionState{
		StateInspecting,
		StateCleaning,
		StateDeploying,
		StateDeleting,
		StateRescuing,
		StateUnrescuing,
		StateAdopting,
		StateCleanWait,
	}

	for _, transientState := range transientStates {
		if state == transientState {
			return true
		}
	}
	return false
}

// ChangeProvisionStateToTarget triggers a provision state change on a node.
func ChangeProvisionStateToTarget(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	nodeID string,
	target ProvisionTarget,
	configDrive any,
	deploySteps []nodes.DeployStep,
	cleanSteps []nodes.CleanStep,
) error {
	// Get current node state
	node, err := nodes.Get(ctx, client, nodeID).Extract()
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeID, err)
	}

	currentState := ProvisionState(node.ProvisionState)
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
	target      ProvisionTarget
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

			currentState := ProvisionState(node.ProvisionState)
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
func (w *provisionWorkflow) checkCompletion(currentState ProvisionState) (bool, error) {
	// Check if we're in a terminal failure state
	if isTerminalFailureState(currentState) {
		return true, fmt.Errorf("node %s in terminal failure state: %s", w.nodeID, currentState)
	}

	// Check if we've reached the final desired state for the target
	switch w.target {
	case TargetManage:
		return currentState == StateManageable, nil
	case TargetInspect:
		return currentState == StateManageable, nil // Inspection ends in manageable
	case TargetClean:
		return currentState == StateAvailable, nil // Cleaning ends in available
	case TargetAvailable:
		return currentState == StateAvailable, nil
	case TargetActive:
		return currentState == StateActive, nil
	case TargetDeleted:
		return currentState == StateAvailable || currentState == StateManageable, nil
	case TargetRescue:
		return currentState == StateRescued, nil
	case TargetUnrescue:
		return currentState == StateActive, nil
	case TargetAdopt:
		return currentState == StateManageable, nil
	default:
		return true, fmt.Errorf("unknown target: %s", w.target)
	}
}

// takeNextAction determines and executes the next action needed.
func (w *provisionWorkflow) takeNextAction(currentState ProvisionState) error {
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
func (w *provisionWorkflow) determineNextTarget(currentState ProvisionState) ProvisionTarget {
	// Direct transitions first
	if isValidTransition(currentState, w.target) {
		return w.target
	}

	// Multi-step workflows - determine intermediate steps
	switch w.target {
	case TargetActive:
		if currentState == StateEnroll {
			return TargetManage
		}
		if currentState == StateManageable {
			return TargetAvailable
		}
		if currentState == StateAvailable {
			return TargetActive
		}

	case TargetAvailable:
		if currentState == StateEnroll {
			return TargetManage
		}
		if currentState == StateManageable {
			return TargetAvailable
		}

	case TargetDeleted:
		if currentState == StateActive {
			return TargetDeleted
		}

	case TargetInspect:
		if currentState == StateEnroll {
			return TargetManage
		}
		if currentState == StateManageable {
			return TargetInspect
		}
		if currentState == StateAvailable {
			return TargetManage
		}

	case TargetClean:
		if currentState == StateEnroll {
			return TargetManage
		}
		if currentState == StateManageable {
			return TargetClean
		}
	}

	return ""
}

// changeProvisionState executes a provision state change.
func (w *provisionWorkflow) changeProvisionState(target ProvisionTarget) error {
	opts := nodes.ProvisionStateOpts{
		Target: target,
	}

	// Add additional options based on target
	switch target {
	case TargetActive:
		opts.ConfigDrive = w.configDrive
		if w.deploySteps != nil {
			opts.DeploySteps = w.deploySteps
		}
	case TargetClean:
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
	targetState ProvisionState,
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

			currentState := ProvisionState(node.ProvisionState)
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
) (ProvisionState, error) {
	node, err := nodes.Get(ctx, client, nodeID).Extract()
	if err != nil {
		return "", fmt.Errorf("failed to get node %s: %w", nodeID, err)
	}

	return ProvisionState(node.ProvisionState), nil
}

// ValidateProvisionState checks if a provision state string is valid.
func ValidateProvisionState(state string) error {
	validStates := []ProvisionState{
		StateEnroll, StateManageable, StateInspecting, StateInspectFail,
		StateCleaning, StateCleanFail, StateCleanWait, StateAvailable,
		StateActive, StateDeploying, StateDeployFail, StateDeleting,
		StateDeleteFail, StateRescuing, StateRescued, StateRescueFail,
		StateUnrescuing, StateAdopting, StateAdoptFail,
	}

	for _, validState := range validStates {
		if ProvisionState(state) == validState {
			return nil
		}
	}

	return fmt.Errorf("invalid provision state: %s", state)
}

// GetValidTargetsFromState returns the valid provision targets from a given state.
func GetValidTargetsFromState(state ProvisionState) []ProvisionTarget {
	var targets []ProvisionTarget
	transitions := getValidTransitions(state)

	for _, transition := range transitions {
		targets = append(targets, transition.Target)
	}

	return targets
}

// ProvisionStateError represents an error with provision state context.
type ProvisionStateError struct {
	NodeID       string
	CurrentState ProvisionState
	TargetState  ProvisionTarget
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
	currentState ProvisionState,
	targetState ProvisionTarget,
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
