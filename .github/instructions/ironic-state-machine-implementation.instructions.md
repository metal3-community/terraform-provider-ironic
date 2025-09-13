# Ironic Bare Metal State Machine Implementation Guide

## Executive Summary

Analysis of the current `workflow.go` implementation reveals significant gaps compared to the [official Ironic state machine documentation](https://docs.openstack.org/ironic/latest/user/states.html). The implementation is missing:

- **9 provision states** (verifying, inspect wait, deploy wait, rescue wait, rescue failed, unrescue failed, servicing, service wait, service failed)
- **2 target actions** (service, unhold) 
- **~25 state transitions** including critical failure recovery paths
- **Service operation support** for node maintenance while active
- **Hold/unhold functionality** for pausing operations

**Good news**: All required constants exist in gophercloud v2.0.1, so implementation can proceed immediately.

**Impact**: Current implementation cannot handle many real-world scenarios including service operations, proper failure recovery, and complete deployment workflows with callback waiting.

## Overview

This document provides detailed instructions for correctly implementing the Ironic Bare Metal State Machine in the terraform-provider-ironic's `workflow.go` file. The current implementation has several gaps and inconsistencies compared to the [official Ironic state machine documentation](https://docs.openstack.org/ironic/latest/user/states.html).

## State Machine Analysis

### Official State Machine Summary

According to the Ironic documentation, the bare metal state machine includes the following states and key behaviors:

#### Stable States (thick border in diagram)
- **enroll**: Initial state for all nodes (API version 1.11+)
- **manageable**: Node can be managed by Ironic, ready for configuration
- **available**: Node is cleaned and ready for deployment
- **active**: Node has a workload running
- **rescue**: Node is in rescue mode
- **error**: Terminal failure during deletion

#### Transient States (automatic transitions)
- **verifying**: Validating driver/interface credentials (missing from current implementation)
- **inspecting**: Hardware introspection in progress
- **inspect wait**: Asynchronous inspection in progress (missing from current implementation)
- **cleaning**: Scrubbing and reconfiguring node
- **clean wait**: Waiting for in-band clean steps
- **deploying**: Deploying workload to node
- **wait call-back**: Waiting for deployment ramdisk (missing from current implementation)
- **deleting**: Tearing down active workload
- **rescuing**: Preparing rescue environment
- **rescue wait**: Waiting for rescue ramdisk (missing from current implementation)
- **unrescuing**: Transitioning from rescue back to active
- **adopting**: Taking over management of existing workload
- **servicing**: Performing service operations (missing from current implementation)
- **service wait**: Waiting for service operations (missing from current implementation)

#### Failure States
- **inspect failed**: Hardware inspection failed
- **clean failed**: Cleaning operation failed
- **deploy failed**: Deployment failed
- **rescue failed**: Rescue operation failed (missing from current implementation)
- **unrescue failed**: Unrescue operation failed (missing from current implementation)
- **adopt failed**: Adoption failed
- **service failed**: Service operation failed (missing from current implementation)

### Current Implementation Gaps

#### Missing States
1. **verifying** - Credential validation state after `enroll`
2. **inspect wait** - Asynchronous inspection state
3. **wait call-back** - Deployment callback waiting state
4. **rescue wait** - Rescue ramdisk waiting state
5. **rescue failed** - Rescue operation failure state
6. **unrescue failed** - Unrescue operation failure state
7. **servicing** - Service operation state
8. **service wait** - Service operation waiting state
9. **service failed** - Service operation failure state

#### Missing Target Actions
1. **abort** - For interrupting operations in certain states (✅ exists, but underutilized)
2. **service** - For servicing operations (✅ exists as `nodes.TargetService`)
3. **unhold** - For resuming paused operations (✅ exists as `nodes.TargetUnhold`)

#### Critical Issues in `determineNextTarget()` Function

The `determineNextTarget()` function (line 304 in user's selection) has several critical gaps:

1. **Missing enroll → verifying transition**: Should go through `verifying` state, not directly to `manageable`
2. **No service workflow support**: Missing `TargetService` handling
3. **Incomplete failure recovery**: No paths from failure states back to operational states
4. **Missing abort handling**: No logic for `TargetAbort` workflows
5. **No hold/unhold support**: Missing `TargetUnhold` logic

**Example of missing logic in determineNextTarget()**:
```go
// MISSING: Service operations
case nodes.TargetService:
    if currentState == nodes.Active {
        return nodes.TargetService  // Direct service from active
    }

// MISSING: Recovery from failure states  
case nodes.TargetRescue:
    if currentState == nodes.RescueFail {
        return nodes.TargetRescue   // Retry rescue from failure
    }
    if currentState == nodes.ServiceFail {
        return nodes.TargetRescue   // Rescue from service failure
    }
```

#### Missing State Transitions

Based on the official documentation, the following transitions are missing or incorrect:

1. **From enroll**:
   - `enroll` → `manage` → `verifying` (should go to verifying, not directly to manageable)

2. **From verifying**:
   - `verifying` → automatic → `manageable` (success)
   - `verifying` → automatic → `enroll` (failure - credential validation failed)

3. **From manageable**:
   - `manageable` → `provide` → `cleaning` (when automatic cleaning is enabled)

4. **From inspect wait**:
   - `inspect wait` → automatic → `manageable` (success)
   - `inspect wait` → automatic → `inspect failed` (failure)

5. **From wait call-back**:
   - `wait call-back` → automatic → `active` (success)
   - `wait call-back` → automatic → `deploy failed` (failure)
   - `wait call-back` → `deleted`/`undeploy` → `deleting` (interrupt deployment)

6. **From rescue wait**:
   - `rescue wait` → automatic → `rescue` (success)
   - `rescue wait` → automatic → `rescue failed` (failure)
   - `rescue wait` → `abort` → `rescue failed` (abort operation)

7. **From active**:
   - `active` → `service` → `servicing` (service operations)

8. **From servicing**:
   - `servicing` → automatic → `service wait` (in-band operations)
   - `servicing` → automatic → `active` (success)
   - `servicing` → automatic → `service failed` (failure)

9. **From service wait**:
   - `service wait` → automatic → `active` (success)
   - `service wait` → automatic → `service failed` (failure)
   - `service wait` → `abort` → `service failed` (abort operation)

10. **From failure states to recovery**:
    - `rescue failed` → `rescue` → `rescuing`
    - `rescue failed` → `unrescue` → `unrescuing`
    - `rescue failed` → `deleted` → `deleting`
    - `unrescue failed` → `rescue` → `rescuing`
    - `unrescue failed` → `unrescue` → `unrescuing`
    - `unrescue failed` → `deleted` → `deleting`
    - `service failed` → `service` → `servicing`
    - `service failed` → `rescue` → `rescuing`
    - `service failed` → `abort` → `active`

#### Incorrect Transitions

1. **Error state behavior**:
   - Current: `error` → `deleted` → `deploying` (incorrect)
   - Should be: `error` → `deleted`/`undeploy` → `deleting`

2. **Manageable to Enroll**:
   - Current: `manageable` → `manage` → `enroll`
   - This transition doesn't exist in the official documentation

3. **Available transitions**:
   - Missing: `available` → `manage` → `manageable`

## Implementation Checklist

### Phase 1: Add Missing States and Constants

**Good News**: All required constants already exist in gophercloud v2.0.1! The following states and targets are available:

- [ ] **1.1** Missing state constants that exist in gophercloud:
  ```go
  // These constants are available in gophercloud nodes package:
  nodes.Verifying        // "verifying"
  nodes.InspectWait      // "inspect wait"  
  nodes.DeployWait       // "wait call-back" (this is the correct name, not WaitCallBack)
  nodes.RescueWait       // "rescue wait"
  nodes.RescueFail       // "rescue failed" (already exists)
  nodes.UnrescueFail     // "unrescue failed"
  nodes.Servicing        // "servicing"
  nodes.ServiceWait      // "service wait"
  nodes.ServiceFail      // "service failed"
  nodes.ServiceHold      // "service hold"
  nodes.CleanHold        // "clean hold"
  nodes.Rebuild          // "rebuild" (state, not just target)
  ```

- [ ] **1.2** Missing target constants that exist in gophercloud:
  ```go
  // These target constants are available in gophercloud nodes package:
  nodes.TargetService    // "service"
  nodes.TargetUnhold     // "unhold"
  // TargetAbort already exists and is used
  ```

### Phase 2: Update State Classifications

- [ ] **2.1** Update `isTerminalFailureState()` to include missing failure states:
  ```go
  terminalStates := []nodes.ProvisionState{
      nodes.InspectFail,
      nodes.CleanFail,
      nodes.DeployFail,
      nodes.Error,
      nodes.RescueFail,      // already exists in gophercloud
      nodes.UnrescueFail,    // exists in gophercloud
      nodes.AdoptFail,
      nodes.ServiceFail,     // exists in gophercloud
  }
  ```

- [ ] **2.2** Update `isTransientState()` to include missing transient states:
  ```go
  transientStates := []nodes.ProvisionState{
      nodes.Verifying,       // exists in gophercloud
      nodes.Inspecting,
      nodes.InspectWait,     // exists in gophercloud
      nodes.Cleaning,
      nodes.CleanWait,
      nodes.Deploying,
      nodes.DeployWait,      // exists in gophercloud (this is "wait call-back")
      nodes.Deleting,
      nodes.Rescuing,
      nodes.RescueWait,      // exists in gophercloud
      nodes.Unrescuing,
      nodes.Adopting,
      nodes.Servicing,       // exists in gophercloud
      nodes.ServiceWait,     // exists in gophercloud
  }
  ```

- [ ] **2.3** Update `ValidateProvisionState()` to include all valid states:
  ```go
  validStates := []nodes.ProvisionState{
      nodes.Enroll, nodes.Verifying, nodes.Manageable,
      nodes.Inspecting, nodes.InspectWait, nodes.InspectFail,
      nodes.Cleaning, nodes.CleanFail, nodes.CleanWait, nodes.CleanHold,
      nodes.Available,
      nodes.Active, nodes.Deploying, nodes.DeployWait, nodes.DeployFail,
      nodes.Deleting, nodes.Error, nodes.Rebuild,
      nodes.Rescuing, nodes.RescueWait, nodes.Rescue, nodes.RescueFail,
      nodes.Unrescuing, nodes.UnrescueFail,
      nodes.Adopting, nodes.AdoptFail,
      nodes.Servicing, nodes.ServiceWait, nodes.ServiceFail, nodes.ServiceHold,
  }
  ```

### Phase 3: Fix State Transitions

- [ ] **3.1** Remove incorrect transitions:
  ```go
  // Remove these from stateTransitions:
  {nodes.Error, nodes.TargetDeleted, nodes.Deploying},        // INCORRECT
  {nodes.Manageable, nodes.TargetManage, nodes.Enroll},       // INCORRECT
  ```

- [ ] **3.2** Add missing core transitions:
  ```go
  // Add these to stateTransitions:
  
  // From enroll 
  {nodes.Enroll, nodes.TargetManage, nodes.Verifying},
  
  // From verifying 
  {nodes.Verifying, nodes.TargetManage, nodes.Manageable},     // success
  // Note: verifying to enroll on failure is automatic, not API-driven
  
  // From manageable
  {nodes.Manageable, nodes.TargetProvide, nodes.Cleaning},     // when auto-clean enabled
  
  // From available  
  {nodes.Available, nodes.TargetManage, nodes.Manageable},
  
  // From inspect wait 
  {nodes.InspectWait, nodes.TargetManage, nodes.Manageable},   // success path
  
  // From deploy wait (wait call-back)
  {nodes.DeployWait, nodes.TargetActive, nodes.Active},        // success
  {nodes.DeployWait, nodes.TargetDeleted, nodes.Deleting},     // abort deployment
  
  // From rescue wait 
  {nodes.RescueWait, nodes.TargetRescue, nodes.Rescue},        // success
  {nodes.RescueWait, nodes.TargetAbort, nodes.RescueFail},     // abort
  
  // From error
  {nodes.Error, nodes.TargetDeleted, nodes.Deleting},          // FIX: should go to deleting
  
  // From rescue failed 
  {nodes.RescueFail, nodes.TargetRescue, nodes.Rescuing},
  {nodes.RescueFail, nodes.TargetUnrescue, nodes.Unrescuing},
  {nodes.RescueFail, nodes.TargetDeleted, nodes.Deleting},
  
  // From unrescue failed  
  {nodes.UnrescueFail, nodes.TargetRescue, nodes.Rescuing},
  {nodes.UnrescueFail, nodes.TargetUnrescue, nodes.Unrescuing},
  {nodes.UnrescueFail, nodes.TargetDeleted, nodes.Deleting},
  
  // From active (servicing)
  {nodes.Active, nodes.TargetService, nodes.Servicing},        
  
  // From servicing 
  {nodes.Servicing, nodes.TargetService, nodes.Active},        // success
  {nodes.Servicing, nodes.TargetAbort, nodes.ServiceFail},     // abort
  
  // From service wait 
  {nodes.ServiceWait, nodes.TargetService, nodes.Active},      // success
  {nodes.ServiceWait, nodes.TargetAbort, nodes.ServiceFail},   // abort
  
  // From service failed 
  {nodes.ServiceFail, nodes.TargetService, nodes.Servicing},
  {nodes.ServiceFail, nodes.TargetRescue, nodes.Rescuing},
  {nodes.ServiceFail, nodes.TargetAbort, nodes.Active},
  
  // From clean hold and service hold (if hold/unhold is needed)
  {nodes.CleanHold, nodes.TargetUnhold, nodes.Cleaning},
  {nodes.ServiceHold, nodes.TargetUnhold, nodes.Servicing},
  ```

### Phase 4: Update Workflow Logic

- [ ] **4.1** Update `checkCompletion()` to handle new states and targets:
  ```go
  switch w.target {
  // ... existing cases ...
  case nodes.TargetService:  
      return currentState == nodes.Active, nil
  case nodes.TargetUnhold:
      // Handle unhold completion based on previous state
      // This requires tracking what state we came from
  case nodes.TargetAbort:
      // Handle abort completion based on context
      // This is complex and needs state-specific logic
  }
  ```

- [ ] **4.2** Update `determineNextTarget()` to handle complex multi-step workflows:
  ```go
  // Add cases for new targets and improve existing logic
  // Consider automatic cleaning when going from manageable to available
  // Handle servicing workflows
  // Handle abort scenarios
  // Add service workflow support:
  case nodes.TargetService:
      if currentState == nodes.Active {
          return nodes.TargetService
      }
  ```

- [ ] **4.3** Update `changeProvisionState()` to handle new targets:
  ```go
  switch target {
  // ... existing cases ...
  case nodes.TargetService:  
      if w.serviceSteps != nil {
          opts.ServiceSteps = w.serviceSteps
      } else {
          opts.ServiceSteps = []nodes.ServiceStep{}
      }
  case nodes.TargetAbort:
      // No additional options typically needed
  case nodes.TargetUnhold:
      // No additional options needed
  }
  ```

### Phase 5: Implement Advanced Features

- [ ] **5.1** Add support for automatic vs manual cleaning detection:
  ```go
  // Need to detect if automatic cleaning is enabled
  // This affects manageable → provide transitions
  ```

- [ ] **5.2** Add proper abort handling:
  ```go
  // Implement logic to handle abort operations
  // Different states have different abort behaviors
  ```

- [ ] **5.3** Add service operation support:
  ```go
  // Add service steps support similar to deploy/clean steps
  type provisionWorkflow struct {
      // ... existing fields ...
      serviceSteps []nodes.ServiceStep  // this type exists in gophercloud
  }
  
  // Update ChangeProvisionStateToTarget function signature to accept serviceSteps
  func ChangeProvisionStateToTarget(
      ctx context.Context,
      client *gophercloud.ServiceClient,
      nodeID string,
      target nodes.TargetProvisionState,
      configDrive any,
      deploySteps []nodes.DeployStep,
      cleanSteps []nodes.CleanStep,
      serviceSteps []nodes.ServiceStep,  // add this parameter
  ) error
  ```

- [ ] **5.4** Improve error handling and recovery:
  ```go
  // Add logic to handle recovery from failure states
  // Implement retry logic for transient failures
  ```

### Phase 6: Testing and Validation

- [ ] **6.1** Create comprehensive test cases for all state transitions
- [ ] **6.2** Test failure state recovery scenarios  
- [ ] **6.3** Test abort operations in various states
- [ ] **6.4** Test service operations (if supported)
- [ ] **6.5** Validate against real Ironic deployment

## Implementation Notes

### Gophercloud Compatibility

✅ **All required constants are available** in gophercloud v2.0.1! The implementation can proceed without version compatibility concerns.

Available constants verified:
```go
// Provision States
nodes.Verifying, nodes.InspectWait, nodes.DeployWait, nodes.RescueWait
nodes.RescueFail, nodes.UnrescueFail, nodes.Servicing, nodes.ServiceWait
nodes.ServiceFail, nodes.ServiceHold, nodes.CleanHold, nodes.Rebuild

// Target States  
nodes.TargetService, nodes.TargetUnhold
```

Note: `nodes.DeployWait` is the correct constant name for "wait call-back" state.

### Automatic vs API-Driven Transitions

The implementation must distinguish between:
- **API-driven transitions**: Initiated by setting provision state with a target
- **Automatic transitions**: Happen internally by the Ironic conductor

The current implementation only handles API-driven transitions, which is correct for a Terraform provider.

### Backward Compatibility

When adding new states and transitions:
- Ensure older Ironic versions still work
- Add version detection if needed
- Gracefully handle unknown states

### Error Context

Improve error messages to include:
- Current node state
- Attempted target
- Available transitions from current state
- Ironic API error details

## Testing Strategy

1. **Unit Tests**: Test each transition in isolation
2. **Integration Tests**: Test complete workflows (enroll → active)
3. **Failure Tests**: Test recovery from each failure state
4. **Edge Cases**: Test abort operations, simultaneous operations
5. **Version Tests**: Test against different Ironic API versions

## Documentation Updates

After implementation:
- [ ] Update README with new state support
- [ ] Document service operation support
- [ ] Add troubleshooting guide for state machine issues
- [ ] Create state transition diagrams specific to Terraform provider

---

*This implementation guide is based on the official [Ironic Bare Metal State Machine documentation](https://docs.openstack.org/ironic/latest/user/states.html) and the [state diagram](https://docs.openstack.org/ironic/latest/_images/states.svg).*