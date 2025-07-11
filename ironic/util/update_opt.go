package util

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func AddDynamicUpdateOpsForField(
	ctx context.Context,
	updateOpts *nodes.UpdateOpts,
	diagnostics *diag.Diagnostics,
	planValue, stateValue types.Dynamic,
	fieldName string,
) {
	if planValue.Equal(stateValue) {
		return
	}

	planMap, err := DynamicToMap(ctx, planValue)
	if err != nil {
		diagnostics.AddAttributeError(
			path.Root(fieldName),
			"Error Converting Plan Value",
			fmt.Sprintf("Could not convert %s to map: %s", fieldName, err),
		)
		return
	}

	stateMap, err := DynamicToMap(ctx, stateValue)
	if err != nil {
		diagnostics.AddAttributeError(
			path.Root(fieldName),
			"Error Converting State Value",
			fmt.Sprintf("Could not convert %s to map: %s", fieldName, err),
		)
		return
	}

	AddUpdateOptForField(
		ctx,
		updateOpts,
		diagnostics,
		planMap,
		stateMap,
		fieldName,
	)
}

func AddDynamicUpdateOptForFieldWithMap(
	ctx context.Context,
	updateOpts *nodes.UpdateOpts,
	diagnostics *diag.Diagnostics,
	planValue types.Dynamic,
	stateMap map[string]any,
	fieldName string,
) {
	planMap, err := DynamicToMap(ctx, planValue)
	if err != nil {
		diagnostics.AddAttributeError(
			path.Root(fieldName),
			"Error Converting Plan Value",
			fmt.Sprintf("Could not convert %s to map: %s", fieldName, err),
		)
		return
	}
	AddUpdateOptForField(
		ctx,
		updateOpts,
		diagnostics,
		planMap,
		stateMap,
		fieldName,
	)
}

func AddUpdateOptForField(
	ctx context.Context,
	updateOpts *nodes.UpdateOpts,
	diagnostics *diag.Diagnostics,
	planMap, stateMap map[string]any,
	fieldName string,
) {
	basePath := "/" + fieldName
	// If one of the maps is nil (e.g. attribute is null or empty),
	// we replace the whole object.
	if planMap == nil || stateMap == nil {
		*updateOpts = append(*updateOpts, nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  basePath,
			Value: planMap, // if planMap is nil, this will set the field to null.
		})
		return
	}

	// Compare maps and generate update operations
	for key, planVal := range planMap {
		stateVal, ok := stateMap[key]
		if !ok {
			// Add operation
			*updateOpts = append(*updateOpts, nodes.UpdateOperation{
				Op:    nodes.AddOp,
				Path:  fmt.Sprintf("%s/%s", basePath, key),
				Value: planVal,
			})
		} else if !reflect.DeepEqual(planVal, stateVal) {
			// Replace operation
			*updateOpts = append(*updateOpts, nodes.UpdateOperation{
				Op:    nodes.ReplaceOp,
				Path:  fmt.Sprintf("%s/%s", basePath, key),
				Value: planVal,
			})
		}
	}

	for key := range stateMap {
		if _, ok := planMap[key]; !ok {
			// Remove operation
			*updateOpts = append(*updateOpts, nodes.UpdateOperation{
				Op:   nodes.RemoveOp,
				Path: fmt.Sprintf("%s/%s", basePath, key),
			})
		}
	}
}
