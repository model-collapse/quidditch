package wasm

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestUDFMetadataValidation(t *testing.T) {
	// Valid metadata
	metadata := &UDFMetadata{
		Name:         "test_udf",
		Version:      "1.0.0",
		Description:  "Test UDF",
		FunctionName: "test_func",
		WASMBytes:    addWasmBytes,
		Parameters: []UDFParameter{
			{Name: "param1", Type: ValueTypeI32, Required: true},
			{Name: "param2", Type: ValueTypeF64, Required: false, Default: 0.0},
		},
		Returns: []UDFReturnType{
			{Type: ValueTypeI32},
		},
	}

	if err := metadata.Validate(); err != nil {
		t.Errorf("Valid metadata failed validation: %v", err)
	}

	t.Log("✅ Valid metadata passed validation")
}

func TestUDFMetadataValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		metadata *UDFMetadata
		wantErr  bool
	}{
		{
			name: "missing name",
			metadata: &UDFMetadata{
				Version:      "1.0.0",
				FunctionName: "test",
				WASMBytes:    addWasmBytes,
			},
			wantErr: true,
		},
		{
			name: "missing version",
			metadata: &UDFMetadata{
				Name:         "test",
				FunctionName: "test",
				WASMBytes:    addWasmBytes,
			},
			wantErr: true,
		},
		{
			name: "missing function name",
			metadata: &UDFMetadata{
				Name:      "test",
				Version:   "1.0.0",
				WASMBytes: addWasmBytes,
			},
			wantErr: true,
		},
		{
			name: "missing WASM bytes",
			metadata: &UDFMetadata{
				Name:         "test",
				Version:      "1.0.0",
				FunctionName: "test",
			},
			wantErr: true,
		},
		{
			name: "duplicate parameter names",
			metadata: &UDFMetadata{
				Name:         "test",
				Version:      "1.0.0",
				FunctionName: "test",
				WASMBytes:    addWasmBytes,
				Parameters: []UDFParameter{
					{Name: "param", Type: ValueTypeI32},
					{Name: "param", Type: ValueTypeI64},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.metadata.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Log("✅ Metadata validation errors caught correctly")
}

func TestUDFRegistryRegister(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	runtime, err := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := NewUDFRegistry(&UDFRegistryConfig{
		Runtime:         runtime,
		DefaultPoolSize: 3,
		EnableStats:     true,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer registry.Close()

	// Register UDF
	metadata := &UDFMetadata{
		Name:         "add_udf",
		Version:      "1.0.0",
		Description:  "Addition UDF",
		FunctionName: "add",
		WASMBytes:    addWasmBytes,
		Parameters: []UDFParameter{
			{Name: "a", Type: ValueTypeI32, Required: true},
			{Name: "b", Type: ValueTypeI32, Required: true},
		},
		Returns: []UDFReturnType{
			{Type: ValueTypeI32},
		},
	}

	if err := registry.Register(metadata); err != nil {
		t.Fatalf("Failed to register UDF: %v", err)
	}

	// Verify registration
	registered, err := registry.Get("add_udf", "1.0.0")
	if err != nil {
		t.Fatalf("Failed to get UDF: %v", err)
	}

	if registered.Metadata.Name != "add_udf" {
		t.Errorf("Expected name 'add_udf', got '%s'", registered.Metadata.Name)
	}

	if registered.Pool == nil {
		t.Error("Expected pool to be created")
	}

	if registered.Stats == nil {
		t.Error("Expected stats to be initialized")
	}

	t.Log("✅ UDF registration working")
}

func TestUDFRegistryDuplicateRegistration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	runtime, err := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := NewUDFRegistry(&UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer registry.Close()

	metadata := &UDFMetadata{
		Name:         "test",
		Version:      "1.0.0",
		Description:  "Test",
		FunctionName: "add",
		WASMBytes:    addWasmBytes,
	}

	// First registration should succeed
	if err := registry.Register(metadata); err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Second registration should fail
	if err := registry.Register(metadata); err == nil {
		t.Error("Expected duplicate registration to fail")
	}

	t.Log("✅ Duplicate registration prevented")
}

func TestUDFRegistryUnregister(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	runtime, err := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := NewUDFRegistry(&UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer registry.Close()

	// Register UDF
	metadata := &UDFMetadata{
		Name:         "test",
		Version:      "1.0.0",
		Description:  "Test",
		FunctionName: "add",
		WASMBytes:    addWasmBytes,
	}

	if err := registry.Register(metadata); err != nil {
		t.Fatalf("Failed to register UDF: %v", err)
	}

	// Verify registration
	if _, err := registry.Get("test", "1.0.0"); err != nil {
		t.Fatalf("UDF should exist: %v", err)
	}

	// Unregister
	if err := registry.Unregister("test", "1.0.0"); err != nil {
		t.Fatalf("Failed to unregister UDF: %v", err)
	}

	// Verify unregistration
	if _, err := registry.Get("test", "1.0.0"); err == nil {
		t.Error("UDF should not exist after unregister")
	}

	t.Log("✅ UDF unregistration working")
}

func TestUDFRegistryList(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	runtime, err := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := NewUDFRegistry(&UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer registry.Close()

	// Initially empty
	udfs := registry.List()
	if len(udfs) != 0 {
		t.Errorf("Expected 0 UDFs, got %d", len(udfs))
	}

	// Register two UDFs
	for i := 1; i <= 2; i++ {
		metadata := &UDFMetadata{
			Name:         "test",
			Version:      string(rune('0' + i)) + ".0.0",
			Description:  "Test",
			FunctionName: "add",
			WASMBytes:    addWasmBytes,
		}

		if err := registry.Register(metadata); err != nil {
			t.Fatalf("Failed to register UDF %d: %v", i, err)
		}
	}

	// Should have 2 UDFs
	udfs = registry.List()
	if len(udfs) != 2 {
		t.Errorf("Expected 2 UDFs, got %d", len(udfs))
	}

	t.Log("✅ UDF listing working")
}

func TestUDFRegistryGetLatest(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	runtime, err := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := NewUDFRegistry(&UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer registry.Close()

	// Register v1.0.0
	metadata1 := &UDFMetadata{
		Name:         "test",
		Version:      "1.0.0",
		Description:  "Test v1",
		FunctionName: "add",
		WASMBytes:    addWasmBytes,
	}

	if err := registry.Register(metadata1); err != nil {
		t.Fatalf("Failed to register v1: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	// Register v2.0.0
	metadata2 := &UDFMetadata{
		Name:         "test",
		Version:      "2.0.0",
		Description:  "Test v2",
		FunctionName: "add",
		WASMBytes:    addWasmBytes,
	}

	if err := registry.Register(metadata2); err != nil {
		t.Fatalf("Failed to register v2: %v", err)
	}

	// Get latest should return v2.0.0
	latest, err := registry.GetLatest("test")
	if err != nil {
		t.Fatalf("Failed to get latest: %v", err)
	}

	if latest.Metadata.Version != "2.0.0" {
		t.Errorf("Expected latest version '2.0.0', got '%s'", latest.Metadata.Version)
	}

	t.Log("✅ GetLatest working correctly")
}

func TestUDFRegistryQuery(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	runtime, err := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	registry, err := NewUDFRegistry(&UDFRegistryConfig{
		Runtime: runtime,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}
	defer registry.Close()

	// Register UDFs with different metadata
	udfs := []*UDFMetadata{
		{
			Name:         "string_distance",
			Version:      "1.0.0",
			Description:  "String distance",
			FunctionName: "calculate",
			WASMBytes:    addWasmBytes,
			Category:     "string",
			Tags:         []string{"string", "similarity"},
		},
		{
			Name:         "custom_score",
			Version:      "1.0.0",
			Description:  "Custom scoring",
			FunctionName: "score",
			WASMBytes:    addWasmBytes,
			Category:     "scoring",
			Tags:         []string{"score", "ranking"},
		},
		{
			Name:         "date_range",
			Version:      "1.0.0",
			Description:  "Date range",
			FunctionName: "check",
			WASMBytes:    addWasmBytes,
			Category:     "date",
			Tags:         []string{"date", "time"},
		},
	}

	for _, udf := range udfs {
		if err := registry.Register(udf); err != nil {
			t.Fatalf("Failed to register UDF %s: %v", udf.Name, err)
		}
	}

	// Query by category
	results := registry.Query(&UDFQuery{Category: "string"})
	if len(results) != 1 {
		t.Errorf("Expected 1 result for category 'string', got %d", len(results))
	}

	// Query by tag
	results = registry.Query(&UDFQuery{Tags: []string{"score"}})
	if len(results) != 1 {
		t.Errorf("Expected 1 result for tag 'score', got %d", len(results))
	}

	// Query by name
	results = registry.Query(&UDFQuery{Name: "date_range"})
	if len(results) != 1 {
		t.Errorf("Expected 1 result for name 'date_range', got %d", len(results))
	}

	t.Log("✅ UDF query working correctly")
}

func TestUDFMetadataHelpers(t *testing.T) {
	metadata := &UDFMetadata{
		Name:    "test",
		Version: "1.0.0",
		Parameters: []UDFParameter{
			{Name: "param1", Type: ValueTypeI32},
			{Name: "param2", Type: ValueTypeF64},
		},
		Tags: []string{"test", "example"},
	}

	// Test GetFullName
	fullName := metadata.GetFullName()
	if fullName != "test@1.0.0" {
		t.Errorf("Expected full name 'test@1.0.0', got '%s'", fullName)
	}

	// Test GetParameterByName
	param, exists := metadata.GetParameterByName("param1")
	if !exists {
		t.Error("Expected param1 to exist")
	}
	if param.Type != ValueTypeI32 {
		t.Errorf("Expected param1 type I32, got %v", param.Type)
	}

	// Test HasTag
	if !metadata.HasTag("test") {
		t.Error("Expected HasTag('test') to be true")
	}

	if metadata.HasTag("nonexistent") {
		t.Error("Expected HasTag('nonexistent') to be false")
	}

	t.Log("✅ Metadata helper functions working")
}

func TestUDFStatsUpdate(t *testing.T) {
	stats := &UDFStats{
		Name:    "test",
		Version: "1.0.0",
	}

	// Simulate successful calls
	stats.UpdateStats(100*time.Microsecond, nil)
	stats.UpdateStats(200*time.Microsecond, nil)
	stats.UpdateStats(150*time.Microsecond, nil)

	if stats.CallCount != 3 {
		t.Errorf("Expected 3 calls, got %d", stats.CallCount)
	}

	if stats.MinDuration != 100*time.Microsecond {
		t.Errorf("Expected min duration 100μs, got %v", stats.MinDuration)
	}

	if stats.MaxDuration != 200*time.Microsecond {
		t.Errorf("Expected max duration 200μs, got %v", stats.MaxDuration)
	}

	expectedAvg := 150 * time.Microsecond
	if stats.AverageDuration != expectedAvg {
		t.Errorf("Expected average duration %v, got %v", expectedAvg, stats.AverageDuration)
	}

	// Simulate error
	stats.UpdateStats(100*time.Microsecond, fmt.Errorf("test error"))

	if stats.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", stats.ErrorCount)
	}

	if stats.LastError != "test error" {
		t.Errorf("Expected last error 'test error', got '%s'", stats.LastError)
	}

	errorRate := stats.ErrorRate()
	expectedRate := 25.0 // 1 error out of 4 calls
	if errorRate != expectedRate {
		t.Errorf("Expected error rate %.2f%%, got %.2f%%", expectedRate, errorRate)
	}

	t.Log("✅ Stats tracking working correctly")
}

func BenchmarkUDFRegistryRegister(b *testing.B) {
	logger, _ := zap.NewProduction()

	runtime, _ := NewRuntime(&Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	})
	defer runtime.Close()

	registry, _ := NewUDFRegistry(&UDFRegistryConfig{
		Runtime:         runtime,
		DefaultPoolSize: 0, // Disable pooling for benchmark
		EnableStats:     false,
		Logger:          logger,
	})
	defer registry.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metadata := &UDFMetadata{
			Name:         fmt.Sprintf("test_%d", i),
			Version:      "1.0.0",
			Description:  "Test",
			FunctionName: "add",
			WASMBytes:    addWasmBytes,
		}

		registry.Register(metadata)
	}
}
