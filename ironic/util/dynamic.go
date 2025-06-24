package util

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// MapToDynamic converts a map[string]any to types.Dynamic using deep reflection.
func MapToDynamic(ctx context.Context, input map[string]any) (types.Dynamic, error) {
	if input == nil {
		return types.DynamicNull(), nil
	}

	if len(input) == 0 {
		// Empty map - create an empty object
		objectType := types.ObjectType{
			AttrTypes: map[string]attr.Type{},
		}
		objectValue := types.ObjectValueMust(objectType.AttrTypes, map[string]attr.Value{})
		return types.DynamicValue(objectValue), nil
	}

	// Convert the map to terraform types
	attrTypes := make(map[string]attr.Type)
	attrValues := make(map[string]attr.Value)

	for key, value := range input {
		attrType, attrValue, err := convertGoValueToTerraformValue(value)
		if err != nil {
			return types.DynamicNull(), fmt.Errorf("error converting key %s: %w", key, err)
		}
		attrTypes[key] = attrType
		attrValues[key] = attrValue
	}

	objectValue, diags := types.ObjectValue(attrTypes, attrValues)
	if diags.HasError() {
		return types.DynamicNull(), fmt.Errorf("error creating object value: %v", diags)
	}

	return types.DynamicValue(objectValue), nil
}

// DynamicToMap converts a types.Dynamic to map[string]any using deep reflection.
func DynamicToMap(ctx context.Context, dynamic types.Dynamic) (map[string]any, error) {
	if dynamic.IsNull() || dynamic.IsUnknown() {
		return nil, nil
	}

	underlyingValue := dynamic.UnderlyingValue()
	return convertTerraformValueToGoValue(underlyingValue)
}

// convertGoValueToTerraformValue converts a Go value to Terraform attr.Type and attr.Value.
func convertGoValueToTerraformValue(value any) (attr.Type, attr.Value, error) {
	if value == nil {
		return types.StringType, types.StringNull(), nil
	}

	switch v := value.(type) {
	case string:
		return types.StringType, types.StringValue(v), nil
	case bool:
		return types.BoolType, types.BoolValue(v), nil
	case int:
		return types.Int64Type, types.Int64Value(int64(v)), nil
	case int32:
		return types.Int64Type, types.Int64Value(int64(v)), nil
	case int64:
		return types.Int64Type, types.Int64Value(v), nil
	case float32:
		return types.Float64Type, types.Float64Value(float64(v)), nil
	case float64:
		return types.Float64Type, types.Float64Value(v), nil
	case []any:
		// Handle slices
		if len(v) == 0 {
			return types.ListType{ElemType: types.StringType}, types.ListValueMust(types.StringType, []attr.Value{}), nil
		}

		// Check if all elements are the same type
		firstType, _, err := convertGoValueToTerraformValue(v[0])
		if err != nil {
			return nil, nil, fmt.Errorf("error determining element type: %w", err)
		}

		allSameType := true
		for i := 1; i < len(v); i++ {
			elemType, _, err := convertGoValueToTerraformValue(v[i])
			if err != nil {
				return nil, nil, fmt.Errorf("error checking element %d type: %w", i, err)
			}
			if !elemType.Equal(firstType) {
				allSameType = false
				break
			}
		}

		if allSameType {
			// All elements are the same type - create a proper list
			elemValues := make([]attr.Value, len(v))
			for i, elem := range v {
				_, elemValue, err := convertGoValueToTerraformValue(elem)
				if err != nil {
					return nil, nil, fmt.Errorf("error converting element %d: %w", i, err)
				}
				elemValues[i] = elemValue
			}

			listType := types.ListType{ElemType: firstType}
			listValue, diags := types.ListValue(firstType, elemValues)
			if diags.HasError() {
				return nil, nil, fmt.Errorf("error creating list value: %v", diags)
			}
			return listType, listValue, nil
		} else {
			// Mixed types - create a tuple
			elemTypes := make([]attr.Type, len(v))
			elemValues := make([]attr.Value, len(v))

			for i, elem := range v {
				elemType, elemValue, err := convertGoValueToTerraformValue(elem)
				if err != nil {
					return nil, nil, fmt.Errorf("error converting element %d: %w", i, err)
				}
				elemTypes[i] = elemType
				elemValues[i] = elemValue
			}

			tupleType := types.TupleType{ElemTypes: elemTypes}
			tupleValue, diags := types.TupleValue(elemTypes, elemValues)
			if diags.HasError() {
				return nil, nil, fmt.Errorf("error creating tuple value: %v", diags)
			}
			return tupleType, tupleValue, nil
		}

	case map[string]any:
		// Handle nested maps
		if len(v) == 0 {
			objectType := types.ObjectType{AttrTypes: map[string]attr.Type{}}
			objectValue := types.ObjectValueMust(objectType.AttrTypes, map[string]attr.Value{})
			return objectType, objectValue, nil
		}

		attrTypes := make(map[string]attr.Type)
		attrValues := make(map[string]attr.Value)

		for key, nestedValue := range v {
			attrType, attrValue, err := convertGoValueToTerraformValue(nestedValue)
			if err != nil {
				return nil, nil, fmt.Errorf("error converting nested key %s: %w", key, err)
			}
			attrTypes[key] = attrType
			attrValues[key] = attrValue
		}

		objectType := types.ObjectType{AttrTypes: attrTypes}
		objectValue, diags := types.ObjectValue(attrTypes, attrValues)
		if diags.HasError() {
			return nil, nil, fmt.Errorf("error creating nested object value: %v", diags)
		}
		return objectType, objectValue, nil

	default:
		// For unknown types, convert to string representation
		return types.StringType, types.StringValue(fmt.Sprintf("%v", v)), nil
	}
}

// convertTerraformValueToGoValue converts a Terraform attr.Value to Go value.
func convertTerraformValueToGoValue(value attr.Value) (map[string]any, error) {
	if value == nil || value.IsNull() || value.IsUnknown() {
		return nil, nil
	}

	switch v := value.(type) {
	case basetypes.ObjectValue:
		result := make(map[string]any)
		attrs := v.Attributes()
		for key, attrValue := range attrs {
			goValue, err := convertSingleTerraformValueToGo(attrValue)
			if err != nil {
				return nil, fmt.Errorf("error converting attribute %s: %w", key, err)
			}
			result[key] = goValue
		}
		return result, nil
	default:
		// If it's not an object, try to convert it to a single value and wrap in a map
		goValue, err := convertSingleTerraformValueToGo(value)
		if err != nil {
			return nil, err
		}
		return map[string]any{"value": goValue}, nil
	}
}

// convertSingleTerraformValueToGo converts a single Terraform attr.Value to Go value.
func convertSingleTerraformValueToGo(value attr.Value) (any, error) {
	if value == nil || value.IsNull() || value.IsUnknown() {
		return nil, nil
	}

	switch v := value.(type) {
	case basetypes.StringValue:
		return v.ValueString(), nil
	case basetypes.BoolValue:
		return v.ValueBool(), nil
	case basetypes.Int64Value:
		return v.ValueInt64(), nil
	case basetypes.Float64Value:
		return v.ValueFloat64(), nil
	case basetypes.ListValue:
		elements := v.Elements()
		result := make([]any, len(elements))
		for i, elem := range elements {
			goValue, err := convertSingleTerraformValueToGo(elem)
			if err != nil {
				return nil, fmt.Errorf("error converting list element %d: %w", i, err)
			}
			result[i] = goValue
		}
		return result, nil
	case basetypes.TupleValue:
		elements := v.Elements()
		result := make([]any, len(elements))
		for i, elem := range elements {
			goValue, err := convertSingleTerraformValueToGo(elem)
			if err != nil {
				return nil, fmt.Errorf("error converting tuple element %d: %w", i, err)
			}
			result[i] = goValue
		}
		return result, nil
	case basetypes.ObjectValue:
		result := make(map[string]any)
		attrs := v.Attributes()
		for key, attrValue := range attrs {
			goValue, err := convertSingleTerraformValueToGo(attrValue)
			if err != nil {
				return nil, fmt.Errorf("error converting object attribute %s: %w", key, err)
			}
			result[key] = goValue
		}
		return result, nil
	default:
		// For unknown types, use reflection to get the underlying value
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Ptr && !rv.IsNil() {
			rv = rv.Elem()
		}

		// Try to find a Value method that returns the underlying value
		if method := rv.MethodByName("ValueString"); method.IsValid() {
			results := method.Call(nil)
			if len(results) == 1 {
				return results[0].Interface(), nil
			}
		}

		return fmt.Sprintf("%v", value), nil
	}
}

// StringMapToDynamic converts a map[string]string to types.Dynamic.
func StringMapToDynamic(ctx context.Context, input map[string]string) (types.Dynamic, error) {
	if input == nil {
		return types.DynamicNull(), nil
	}

	if len(input) == 0 {
		// Empty map - create an empty object
		objectType := types.ObjectType{
			AttrTypes: map[string]attr.Type{},
		}
		objectValue := types.ObjectValueMust(objectType.AttrTypes, map[string]attr.Value{})
		return types.DynamicValue(objectValue), nil
	}

	// Convert all values to string types
	attrTypes := make(map[string]attr.Type)
	attrValues := make(map[string]attr.Value)

	for key, value := range input {
		attrTypes[key] = types.StringType
		attrValues[key] = types.StringValue(value)
	}

	objectValue, diags := types.ObjectValue(attrTypes, attrValues)
	if diags.HasError() {
		return types.DynamicNull(), fmt.Errorf("error creating object value: %v", diags)
	}

	return types.DynamicValue(objectValue), nil
}

// DynamicToStringMap converts a types.Dynamic to map[string]string.
func DynamicToStringMap(ctx context.Context, dynamic types.Dynamic) (map[string]string, error) {
	if dynamic.IsNull() || dynamic.IsUnknown() {
		return nil, nil
	}

	// First convert to generic map
	genericMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		return nil, err
	}

	// Convert all values to strings
	result := make(map[string]string)
	for key, value := range genericMap {
		switch v := value.(type) {
		case string:
			result[key] = v
		default:
			result[key] = fmt.Sprintf("%v", v)
		}
	}

	return result, nil
}
