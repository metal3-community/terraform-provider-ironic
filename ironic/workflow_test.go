package ironic

import (
    "testing"

    "github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
)

func TestIsTerminalFailureState(t *testing.T) {
    tests := []struct {
        state    nodes.ProvisionState
        expected bool
    }{
        {nodes.InspectFail, true},
        {nodes.CleanFail, true},
        {nodes.DeployFail, true},
        {nodes.Error, true},
        {nodes.RescueFail, true},
        {nodes.UnrescueFail, true}, // newly added
        {nodes.AdoptFail, true},
        {nodes.ServiceFail, true}, // newly added
        {nodes.Active, false},
        {nodes.Available, false},
        {nodes.Manageable, false},
    }

    for _, test := range tests {
        result := isTerminalFailureState(test.state)
        if result != test.expected {
            t.Errorf("isTerminalFailureState(%s) = %v, expected %v", test.state, result, test.expected)
        }
    }
}

func TestIsTransientState(t *testing.T) {
    tests := []struct {
        state    nodes.ProvisionState
        expected bool
    }{
        {nodes.Verifying, true},     // newly added
        {nodes.Inspecting, true},
        {nodes.InspectWait, true},   // newly added
        {nodes.Cleaning, true},
        {nodes.CleanWait, true},
        {nodes.Deploying, true},
        {nodes.DeployWait, true},    // newly added
        {nodes.Deleting, true},
        {nodes.Rescuing, true},
        {nodes.RescueWait, true},    // newly added
        {nodes.Unrescuing, true},
        {nodes.Adopting, true},
        {nodes.Servicing, true},     // newly added
        {nodes.ServiceWait, true},   // newly added
        {nodes.Active, false},
        {nodes.Available, false},
        {nodes.Manageable, false},
        {nodes.Error, false},
    }

    for _, test := range tests {
        result := isTransientState(test.state)
        if result != test.expected {
            t.Errorf("isTransientState(%s) = %v, expected %v", test.state, result, test.expected)
        }
    }
}

func TestValidateProvisionState(t *testing.T) {
    validStates := []string{
        "enroll", "verifying", "manageable",
        "inspecting", "inspect wait", "inspect failed",
        "cleaning", "clean failed", "clean wait", "clean hold",
        "available",
        "active", "deploying", "wait call-back", "deploy failed",
        "deleting", "error", "rebuild",
        "rescuing", "rescue wait", "rescue", "rescue failed",
        "unrescuing", "unrescue failed",
        "adopting", "adopt failed",
        "servicing", "service wait", "service failed", "service hold",
    }

    for _, state := range validStates {
        if err := ValidateProvisionState(state); err != nil {
            t.Errorf("ValidateProvisionState(%s) returned error: %v", state, err)
        }
    }

    invalidStates := []string{
        "invalid", "unknown", "not-a-state",
    }

    for _, state := range invalidStates {
        if err := ValidateProvisionState(state); err == nil {
            t.Errorf("ValidateProvisionState(%s) should have returned error", state)
        }
    }
}

func TestValidTransitions(t *testing.T) {
    tests := []struct {
        from     nodes.ProvisionState
        target   nodes.TargetProvisionState
        expected bool
    }{
        // New transitions
        {nodes.Enroll, nodes.TargetManage, true},        // should go to verifying
        {nodes.Verifying, nodes.TargetManage, true},     // verifying to manageable
        {nodes.Active, nodes.TargetService, true},       // service from active
        {nodes.Available, nodes.TargetManage, true},     // available to manageable
        {nodes.Error, nodes.TargetDeleted, true},        // error to deleting (fixed)
        
        // Recovery transitions
        {nodes.RescueFail, nodes.TargetRescue, true},    // retry rescue
        {nodes.ServiceFail, nodes.TargetService, true},  // retry service
        {nodes.ServiceFail, nodes.TargetRescue, true},   // rescue from service failure
        
        // Invalid transitions
        {nodes.Active, nodes.TargetInspect, false},      // can't inspect active node
        {nodes.Enroll, nodes.TargetActive, false},       // can't deploy directly from enroll
    }

    for _, test := range tests {
        result := isValidTransition(test.from, test.target)
        if result != test.expected {
            t.Errorf("isValidTransition(%s, %s) = %v, expected %v", 
                test.from, test.target, result, test.expected)
        }
    }
}

func TestGetExpectedState(t *testing.T) {
    tests := []struct {
        from     nodes.ProvisionState
        target   nodes.TargetProvisionState
        expected nodes.ProvisionState
        found    bool
    }{
        {nodes.Enroll, nodes.TargetManage, nodes.Verifying, true},
        {nodes.Verifying, nodes.TargetManage, nodes.Manageable, true},
        {nodes.Active, nodes.TargetService, nodes.Servicing, true},
        {nodes.Error, nodes.TargetDeleted, nodes.Deleting, true},
        {nodes.Active, nodes.TargetInspect, "", false}, // invalid transition
    }

    for _, test := range tests {
        result, found := getExpectedState(test.from, test.target)
        if found != test.found {
            t.Errorf("getExpectedState(%s, %s) found = %v, expected %v", 
                test.from, test.target, found, test.found)
        }
        if found && result != test.expected {
            t.Errorf("getExpectedState(%s, %s) = %s, expected %s", 
                test.from, test.target, result, test.expected)
        }
    }
}
