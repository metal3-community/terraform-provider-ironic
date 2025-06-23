package util

import (
	"context"
	"testing"
)

func TestMapToDynamicAndBack(t *testing.T) {
	ctx := context.Background()

	// Test case 1: Simple map with string values
	originalMap := map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": 123,
		"key4": true,
	}

	// Convert to dynamic
	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting map to dynamic: %v", err)
	}

	if dynamic.IsNull() {
		t.Fatal("Dynamic value should not be null")
	}

	// Convert back to map
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Check values
	if resultMap["key1"] != "value1" {
		t.Errorf("Expected key1=value1, got %v", resultMap["key1"])
	}
	if resultMap["key2"] != "value2" {
		t.Errorf("Expected key2=value2, got %v", resultMap["key2"])
	}
	if resultMap["key3"] != int64(123) { // Note: int becomes int64
		t.Errorf("Expected key3=123, got %v", resultMap["key3"])
	}
	if resultMap["key4"] != true {
		t.Errorf("Expected key4=true, got %v", resultMap["key4"])
	}
}

func TestNullDynamic(t *testing.T) {
	ctx := context.Background()

	// Test null map
	dynamic, err := MapToDynamic(ctx, nil)
	if err != nil {
		t.Fatalf("Error converting nil map to dynamic: %v", err)
	}

	if !dynamic.IsNull() {
		t.Fatal("Dynamic value should be null for nil map")
	}

	// Convert null dynamic back
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting null dynamic to map: %v", err)
	}

	if resultMap != nil {
		t.Errorf("Expected nil map, got %v", resultMap)
	}
}

func TestEmptyMap(t *testing.T) {
	ctx := context.Background()

	// Test empty map
	originalMap := map[string]any{}

	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting empty map to dynamic: %v", err)
	}

	if dynamic.IsNull() {
		t.Fatal("Dynamic value should not be null for empty map")
	}

	// Convert back
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	if len(resultMap) != 0 {
		t.Errorf("Expected empty map, got %v", resultMap)
	}
}

func TestNestedMap(t *testing.T) {
	ctx := context.Background()

	// Test nested map
	originalMap := map[string]any{
		"outer": map[string]any{
			"inner1": "value1",
			"inner2": 456,
		},
		"simple": "value",
	}

	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting nested map to dynamic: %v", err)
	}

	if dynamic.IsNull() {
		t.Fatal("Dynamic value should not be null")
	}

	// Convert back
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Check simple value
	if resultMap["simple"] != "value" {
		t.Errorf("Expected simple=value, got %v", resultMap["simple"])
	}

	// Check nested value
	outerMap, ok := resultMap["outer"].(map[string]any)
	if !ok {
		t.Fatalf("Expected outer to be map[string]any, got %T", resultMap["outer"])
	}

	if outerMap["inner1"] != "value1" {
		t.Errorf("Expected inner1=value1, got %v", outerMap["inner1"])
	}
	if outerMap["inner2"] != int64(456) {
		t.Errorf("Expected inner2=456, got %v", outerMap["inner2"])
	}
}

func TestDeeplyNestedMaps(t *testing.T) {
	ctx := context.Background()

	// Test deeply nested maps (4 levels deep)
	originalMap := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"level4": map[string]any{
						"deepest": "treasure",
						"number":  42,
						"boolean": true,
					},
					"level3_direct": "value3",
				},
				"level2_direct": "value2",
			},
			"level1_direct": "value1",
		},
		"root_level": "root_value",
	}

	// Convert to dynamic
	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting deeply nested map to dynamic: %v", err)
	}

	if dynamic.IsNull() {
		t.Fatal("Dynamic value should not be null")
	}

	// Convert back to map
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Check root level
	if resultMap["root_level"] != "root_value" {
		t.Errorf("Expected root_level=root_value, got %v", resultMap["root_level"])
	}

	// Navigate through nested levels
	level1, ok := resultMap["level1"].(map[string]any)
	if !ok {
		t.Fatalf("Expected level1 to be map[string]any, got %T", resultMap["level1"])
	}

	if level1["level1_direct"] != "value1" {
		t.Errorf("Expected level1_direct=value1, got %v", level1["level1_direct"])
	}

	level2, ok := level1["level2"].(map[string]any)
	if !ok {
		t.Fatalf("Expected level2 to be map[string]any, got %T", level1["level2"])
	}

	if level2["level2_direct"] != "value2" {
		t.Errorf("Expected level2_direct=value2, got %v", level2["level2_direct"])
	}

	level3, ok := level2["level3"].(map[string]any)
	if !ok {
		t.Fatalf("Expected level3 to be map[string]any, got %T", level2["level3"])
	}

	if level3["level3_direct"] != "value3" {
		t.Errorf("Expected level3_direct=value3, got %v", level3["level3_direct"])
	}

	level4, ok := level3["level4"].(map[string]any)
	if !ok {
		t.Fatalf("Expected level4 to be map[string]any, got %T", level3["level4"])
	}

	if level4["deepest"] != "treasure" {
		t.Errorf("Expected deepest=treasure, got %v", level4["deepest"])
	}
	if level4["number"] != int64(42) {
		t.Errorf("Expected number=42, got %v", level4["number"])
	}
	if level4["boolean"] != true {
		t.Errorf("Expected boolean=true, got %v", level4["boolean"])
	}
}

func TestNestedMapsWithArrays(t *testing.T) {
	ctx := context.Background()

	// Test nested maps containing arrays
	originalMap := map[string]any{
		"config": map[string]any{
			"servers": []any{
				map[string]any{
					"name": "server1",
					"port": 8080,
					"ssl":  true,
				},
				map[string]any{
					"name": "server2",
					"port": 8081,
					"ssl":  false,
				},
			},
			"database": map[string]any{
				"hosts": []any{"db1.example.com", "db2.example.com"},
				"ports": []any{5432, 5433},
				"config": map[string]any{
					"max_connections": 100,
					"timeout":         30,
				},
			},
		},
		"metadata": map[string]any{
			"tags":    []any{"production", "api", "critical"},
			"numbers": []any{1, 2, 3, 4, 5},
			"mixed":   []any{"string", 42, true, 3.14},
		},
	}

	// Convert to dynamic
	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting nested map with arrays to dynamic: %v", err)
	}

	// Convert back to map
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Verify config.servers array
	config, ok := resultMap["config"].(map[string]any)
	if !ok {
		t.Fatalf("Expected config to be map[string]any, got %T", resultMap["config"])
	}

	servers, ok := config["servers"].([]any)
	if !ok {
		t.Fatalf("Expected servers to be []any, got %T", config["servers"])
	}

	if len(servers) != 2 {
		t.Fatalf("Expected 2 servers, got %d", len(servers))
	}

	server1, ok := servers[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected server1 to be map[string]any, got %T", servers[0])
	}

	if server1["name"] != "server1" {
		t.Errorf("Expected server1 name=server1, got %v", server1["name"])
	}
	if server1["port"] != int64(8080) {
		t.Errorf("Expected server1 port=8080, got %v", server1["port"])
	}
	if server1["ssl"] != true {
		t.Errorf("Expected server1 ssl=true, got %v", server1["ssl"])
	}

	// Verify database.hosts array
	database, ok := config["database"].(map[string]any)
	if !ok {
		t.Fatalf("Expected database to be map[string]any, got %T", config["database"])
	}

	hosts, ok := database["hosts"].([]any)
	if !ok {
		t.Fatalf("Expected hosts to be []any, got %T", database["hosts"])
	}

	if len(hosts) != 2 || hosts[0] != "db1.example.com" || hosts[1] != "db2.example.com" {
		t.Errorf("Expected hosts=[db1.example.com, db2.example.com], got %v", hosts)
	}

	// Verify metadata.mixed array with different types
	metadata, ok := resultMap["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("Expected metadata to be map[string]any, got %T", resultMap["metadata"])
	}

	mixed, ok := metadata["mixed"].([]any)
	if !ok {
		t.Fatalf("Expected mixed to be []any, got %T", metadata["mixed"])
	}

	if len(mixed) != 4 {
		t.Fatalf("Expected 4 mixed elements, got %d", len(mixed))
	}

	if mixed[0] != "string" {
		t.Errorf("Expected mixed[0]=string, got %v", mixed[0])
	}
	if mixed[1] != int64(42) {
		t.Errorf("Expected mixed[1]=42, got %v", mixed[1])
	}
	if mixed[2] != true {
		t.Errorf("Expected mixed[2]=true, got %v", mixed[2])
	}
	if mixed[3] != 3.14 {
		t.Errorf("Expected mixed[3]=3.14, got %v", mixed[3])
	}
}

func TestComplexNestedStructure(t *testing.T) {
	ctx := context.Background()

	// Test very complex nested structure with mixed types
	originalMap := map[string]any{
		"application": map[string]any{
			"name":    "my-app",
			"version": "1.2.3",
			"config": map[string]any{
				"environment": map[string]any{
					"variables": map[string]any{
						"DATABASE_URL": "postgres://localhost:5432/mydb",
						"API_TIMEOUT":  30,
						"DEBUG_MODE":   true,
					},
					"secrets": []any{
						map[string]any{
							"name":  "db-password",
							"value": "secret123",
							"type":  "password",
						},
						map[string]any{
							"name":  "api-key",
							"value": "key456",
							"type":  "token",
						},
					},
				},
				"services": []any{
					map[string]any{
						"name":     "web",
						"replicas": 3,
						"resources": map[string]any{
							"cpu":    "100m",
							"memory": "128Mi",
							"limits": map[string]any{
								"cpu":    "500m",
								"memory": "256Mi",
							},
						},
						"ports":   []any{8080, 8081},
						"enabled": true,
					},
					map[string]any{
						"name":     "worker",
						"replicas": 2,
						"resources": map[string]any{
							"cpu":    "200m",
							"memory": "256Mi",
						},
						"queues":  []any{"high", "normal", "low"},
						"enabled": false,
					},
				},
			},
		},
		"metadata": map[string]any{
			"created_at": "2023-01-01T00:00:00Z",
			"labels": map[string]any{
				"environment": "production",
				"team":        "platform",
				"version":     "v1",
			},
			"annotations": []any{
				"managed-by=terraform",
				"backup=enabled",
			},
		},
	}

	// Convert to dynamic
	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting complex nested structure to dynamic: %v", err)
	}

	// Convert back to map
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Navigate and verify the complex structure
	app, ok := resultMap["application"].(map[string]any)
	if !ok {
		t.Fatalf("Expected application to be map[string]any, got %T", resultMap["application"])
	}

	if app["name"] != "my-app" {
		t.Errorf("Expected app name=my-app, got %v", app["name"])
	}

	config, ok := app["config"].(map[string]any)
	if !ok {
		t.Fatalf("Expected config to be map[string]any, got %T", app["config"])
	}

	env, ok := config["environment"].(map[string]any)
	if !ok {
		t.Fatalf("Expected environment to be map[string]any, got %T", config["environment"])
	}

	variables, ok := env["variables"].(map[string]any)
	if !ok {
		t.Fatalf("Expected variables to be map[string]any, got %T", env["variables"])
	}

	if variables["DATABASE_URL"] != "postgres://localhost:5432/mydb" {
		t.Errorf("Expected DATABASE_URL, got %v", variables["DATABASE_URL"])
	}
	if variables["API_TIMEOUT"] != int64(30) {
		t.Errorf("Expected API_TIMEOUT=30, got %v", variables["API_TIMEOUT"])
	}

	// Verify secrets array with nested maps
	secrets, ok := env["secrets"].([]any)
	if !ok {
		t.Fatalf("Expected secrets to be []any, got %T", env["secrets"])
	}

	if len(secrets) != 2 {
		t.Fatalf("Expected 2 secrets, got %d", len(secrets))
	}

	secret1, ok := secrets[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected secret1 to be map[string]any, got %T", secrets[0])
	}

	if secret1["name"] != "db-password" {
		t.Errorf("Expected secret1 name=db-password, got %v", secret1["name"])
	}

	// Verify services array with deeply nested structures
	services, ok := config["services"].([]any)
	if !ok {
		t.Fatalf("Expected services to be []any, got %T", config["services"])
	}

	if len(services) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(services))
	}

	webService, ok := services[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected webService to be map[string]any, got %T", services[0])
	}

	if webService["replicas"] != int64(3) {
		t.Errorf("Expected web replicas=3, got %v", webService["replicas"])
	}

	resources, ok := webService["resources"].(map[string]any)
	if !ok {
		t.Fatalf("Expected resources to be map[string]any, got %T", webService["resources"])
	}

	limits, ok := resources["limits"].(map[string]any)
	if !ok {
		t.Fatalf("Expected limits to be map[string]any, got %T", resources["limits"])
	}

	if limits["memory"] != "256Mi" {
		t.Errorf("Expected limits memory=256Mi, got %v", limits["memory"])
	}

	// Verify ports array
	ports, ok := webService["ports"].([]any)
	if !ok {
		t.Fatalf("Expected ports to be []any, got %T", webService["ports"])
	}

	if len(ports) != 2 || ports[0] != int64(8080) || ports[1] != int64(8081) {
		t.Errorf("Expected ports=[8080, 8081], got %v", ports)
	}
}

func TestEmptyNestedStructures(t *testing.T) {
	ctx := context.Background()

	// Test empty nested structures
	originalMap := map[string]any{
		"empty_map":   map[string]any{},
		"empty_array": []any{},
		"nested": map[string]any{
			"empty_sub_map":   map[string]any{},
			"empty_sub_array": []any{},
			"mixed": map[string]any{
				"value": "test",
				"empty": map[string]any{},
			},
		},
	}

	// Convert to dynamic
	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting empty nested structures to dynamic: %v", err)
	}

	// Convert back to map
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Verify empty structures are preserved
	emptyMap, ok := resultMap["empty_map"].(map[string]any)
	if !ok {
		t.Fatalf("Expected empty_map to be map[string]any, got %T", resultMap["empty_map"])
	}

	if len(emptyMap) != 0 {
		t.Errorf("Expected empty map, got %v", emptyMap)
	}

	emptyArray, ok := resultMap["empty_array"].([]any)
	if !ok {
		t.Fatalf("Expected empty_array to be []any, got %T", resultMap["empty_array"])
	}

	if len(emptyArray) != 0 {
		t.Errorf("Expected empty array, got %v", emptyArray)
	}

	// Verify nested empty structures
	nested, ok := resultMap["nested"].(map[string]any)
	if !ok {
		t.Fatalf("Expected nested to be map[string]any, got %T", resultMap["nested"])
	}

	emptySubMap, ok := nested["empty_sub_map"].(map[string]any)
	if !ok {
		t.Fatalf("Expected empty_sub_map to be map[string]any, got %T", nested["empty_sub_map"])
	}

	if len(emptySubMap) != 0 {
		t.Errorf("Expected empty sub map, got %v", emptySubMap)
	}
}

func TestIronicLikeStructures(t *testing.T) {
	ctx := context.Background()

	// Test Ironic-like structures that would be typical in driver_info, properties, etc.
	originalMap := map[string]any{
		"driver_info": map[string]any{
			"ipmi_address":    "192.168.1.100",
			"ipmi_username":   "admin",
			"ipmi_password":   "secret",
			"ipmi_port":       623,
			"ipmi_priv_level": "ADMINISTRATOR",
		},
		"properties": map[string]any{
			"cpu_arch":  "x86_64",
			"cpus":      16,
			"memory_mb": 32768,
			"local_gb":  500,
			"capabilities": map[string]any{
				"boot_mode":   "uefi",
				"secure_boot": true,
				"cpu_features": []any{
					"vmx",
					"aes",
					"avx2",
				},
			},
			"root_device": map[string]any{
				"size":   ">= 100",
				"model":  "PERC H730",
				"vendor": "DELL",
				"wwn":    "0x500a075123456789",
			},
		},
		"instance_info": map[string]any{
			"image_source":   "http://example.com/image.qcow2",
			"image_checksum": "abc123def456",
			"root_gb":        20,
			"swap_mb":        2048,
			"ephemeral_gb":   10,
			"configdrive":    true,
			"image_properties": map[string]any{
				"os_distro":    "ubuntu",
				"os_version":   "20.04",
				"architecture": "x86_64",
				"kernel_params": []any{
					"console=ttyS0",
					"nomodeset",
					"quiet",
				},
			},
		},
		"extra": map[string]any{
			"system_vendor": map[string]any{
				"manufacturer":  "Dell Inc.",
				"product_name":  "PowerEdge R640",
				"serial_number": "ABC123",
			},
			"bmc": map[string]any{
				"firmware_version": "2.75.75.75",
				"mac_address":      "aa:bb:cc:dd:ee:ff",
			},
			"network_interfaces": []any{
				map[string]any{
					"name": "eth0",
					"mac":  "11:22:33:44:55:66",
					"pxe":  true,
				},
				map[string]any{
					"name": "eth1",
					"mac":  "66:55:44:33:22:11",
					"pxe":  false,
				},
			},
			"tags": []any{"production", "web-server", "east-coast"},
			"metadata": map[string]any{
				"deployment_id": "deploy-123",
				"created_at":    "2023-01-01T00:00:00Z",
				"environment":   "prod",
			},
		},
	}

	// Convert to dynamic
	dynamic, err := MapToDynamic(ctx, originalMap)
	if err != nil {
		t.Fatalf("Error converting Ironic-like structure to dynamic: %v", err)
	}

	if dynamic.IsNull() {
		t.Fatal("Dynamic value should not be null")
	}

	// Convert back to map
	resultMap, err := DynamicToMap(ctx, dynamic)
	if err != nil {
		t.Fatalf("Error converting dynamic to map: %v", err)
	}

	// Verify driver_info
	driverInfo, ok := resultMap["driver_info"].(map[string]any)
	if !ok {
		t.Fatalf("Expected driver_info to be map[string]any, got %T", resultMap["driver_info"])
	}

	if driverInfo["ipmi_address"] != "192.168.1.100" {
		t.Errorf("Expected ipmi_address=192.168.1.100, got %v", driverInfo["ipmi_address"])
	}
	if driverInfo["ipmi_port"] != int64(623) {
		t.Errorf("Expected ipmi_port=623, got %v", driverInfo["ipmi_port"])
	}

	// Verify properties with nested capabilities
	properties, ok := resultMap["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Expected properties to be map[string]any, got %T", resultMap["properties"])
	}

	if properties["cpus"] != int64(16) {
		t.Errorf("Expected cpus=16, got %v", properties["cpus"])
	}

	capabilities, ok := properties["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("Expected capabilities to be map[string]any, got %T", properties["capabilities"])
	}

	if capabilities["secure_boot"] != true {
		t.Errorf("Expected secure_boot=true, got %v", capabilities["secure_boot"])
	}

	cpuFeatures, ok := capabilities["cpu_features"].([]any)
	if !ok {
		t.Fatalf("Expected cpu_features to be []any, got %T", capabilities["cpu_features"])
	}

	if len(cpuFeatures) != 3 || cpuFeatures[0] != "vmx" || cpuFeatures[1] != "aes" ||
		cpuFeatures[2] != "avx2" {
		t.Errorf("Expected cpu_features=[vmx, aes, avx2], got %v", cpuFeatures)
	}

	// Verify instance_info with nested image_properties
	instanceInfo, ok := resultMap["instance_info"].(map[string]any)
	if !ok {
		t.Fatalf("Expected instance_info to be map[string]any, got %T", resultMap["instance_info"])
	}

	imageProperties, ok := instanceInfo["image_properties"].(map[string]any)
	if !ok {
		t.Fatalf(
			"Expected image_properties to be map[string]any, got %T",
			instanceInfo["image_properties"],
		)
	}

	kernelParams, ok := imageProperties["kernel_params"].([]any)
	if !ok {
		t.Fatalf("Expected kernel_params to be []any, got %T", imageProperties["kernel_params"])
	}

	if len(kernelParams) != 3 || kernelParams[0] != "console=ttyS0" {
		t.Errorf("Expected kernel_params=[console=ttyS0, nomodeset, quiet], got %v", kernelParams)
	}

	// Verify extra with network interfaces array
	extra, ok := resultMap["extra"].(map[string]any)
	if !ok {
		t.Fatalf("Expected extra to be map[string]any, got %T", resultMap["extra"])
	}

	networkInterfaces, ok := extra["network_interfaces"].([]any)
	if !ok {
		t.Fatalf("Expected network_interfaces to be []any, got %T", extra["network_interfaces"])
	}

	if len(networkInterfaces) != 2 {
		t.Fatalf("Expected 2 network interfaces, got %d", len(networkInterfaces))
	}

	eth0, ok := networkInterfaces[0].(map[string]any)
	if !ok {
		t.Fatalf("Expected eth0 to be map[string]any, got %T", networkInterfaces[0])
	}

	if eth0["name"] != "eth0" || eth0["pxe"] != true {
		t.Errorf("Expected eth0 name=eth0 pxe=true, got name=%v pxe=%v", eth0["name"], eth0["pxe"])
	}

	// Verify tags array
	tags, ok := extra["tags"].([]any)
	if !ok {
		t.Fatalf("Expected tags to be []any, got %T", extra["tags"])
	}

	expectedTags := []string{"production", "web-server", "east-coast"}
	if len(tags) != len(expectedTags) {
		t.Fatalf("Expected %d tags, got %d", len(expectedTags), len(tags))
	}

	for i, expectedTag := range expectedTags {
		if tags[i] != expectedTag {
			t.Errorf("Expected tag[%d]=%s, got %v", i, expectedTag, tags[i])
		}
	}
}
