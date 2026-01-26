package wasm

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

// Simple WASM module that adds two numbers
// (wat format, compiled to wasm bytes)
var addWasmBytes = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic number
	0x01, 0x00, 0x00, 0x00, // Version
	// Type section
	0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
	// Function section
	0x03, 0x02, 0x01, 0x00,
	// Export section
	0x07, 0x07, 0x01, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00,
	// Code section
	0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b,
}

func TestNewRuntime(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	if runtime == nil {
		t.Fatal("Runtime should not be nil")
	}

	t.Log("✅ WASM runtime created successfully")
}

func TestCompileModule(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		t.Fatalf("Failed to compile module: %v", err)
	}

	// Verify module was compiled
	module, err := runtime.GetModule("add")
	if err != nil {
		t.Fatalf("Failed to get module: %v", err)
	}

	if module.Name != "add" {
		t.Errorf("Expected module name 'add', got '%s'", module.Name)
	}

	t.Log("✅ WASM module compiled successfully")
}

func TestInstantiateModule(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	// Compile module
	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		t.Fatalf("Failed to compile module: %v", err)
	}

	// Instantiate module
	instance, err := runtime.NewModuleInstance("add")
	if err != nil {
		t.Fatalf("Failed to instantiate module: %v", err)
	}
	defer instance.Close()

	if instance == nil {
		t.Fatal("Instance should not be nil")
	}

	// Check that the "add" function exists
	addFunc := instance.GetFunction("add")
	if addFunc == nil {
		t.Fatal("'add' function not found")
	}

	t.Log("✅ WASM module instantiated successfully")
}

func TestCallFunction(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	// Compile and instantiate module
	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		t.Fatalf("Failed to compile module: %v", err)
	}

	instance, err := runtime.NewModuleInstance("add")
	if err != nil {
		t.Fatalf("Failed to instantiate module: %v", err)
	}
	defer instance.Close()

	// Call the add function (2 + 3 = 5)
	results, err := instance.CallFunction(context.Background(), "add", 2, 3)
	if err != nil {
		t.Fatalf("Failed to call function: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0] != 5 {
		t.Errorf("Expected result 5, got %d", results[0])
	}

	t.Logf("✅ WASM function call successful: 2 + 3 = %d", results[0])
}

func TestModulePool(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	// Compile module
	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		t.Fatalf("Failed to compile module: %v", err)
	}

	// Create pool with 3 instances
	pool, err := runtime.NewModulePool("add", 3)
	if err != nil {
		t.Fatalf("Failed to create module pool: %v", err)
	}
	defer pool.Close()

	// Get instance from pool
	instance, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get instance from pool: %v", err)
	}

	// Use instance
	results, err := instance.CallFunction(context.Background(), "add", 10, 20)
	if err != nil {
		t.Fatalf("Failed to call function: %v", err)
	}

	if results[0] != 30 {
		t.Errorf("Expected result 30, got %d", results[0])
	}

	// Return instance to pool
	pool.Put(instance)

	t.Log("✅ Module pool working correctly")
}

func TestListModules(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	// Initially should be empty
	modules := runtime.ListModules()
	if len(modules) != 0 {
		t.Errorf("Expected 0 modules, got %d", len(modules))
	}

	// Compile a module
	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		t.Fatalf("Failed to compile module: %v", err)
	}

	// Should have 1 module
	modules = runtime.ListModules()
	if len(modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(modules))
	}

	if modules[0] != "add" {
		t.Errorf("Expected module name 'add', got '%s'", modules[0])
	}

	t.Log("✅ Module listing working correctly")
}

func TestUnloadModule(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: true,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	// Compile module
	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		t.Fatalf("Failed to compile module: %v", err)
	}

	// Verify module exists
	_, err = runtime.GetModule("add")
	if err != nil {
		t.Fatalf("Module should exist: %v", err)
	}

	// Unload module
	err = runtime.UnloadModule("add")
	if err != nil {
		t.Fatalf("Failed to unload module: %v", err)
	}

	// Verify module is gone
	_, err = runtime.GetModule("add")
	if err == nil {
		t.Fatal("Module should not exist after unload")
	}

	t.Log("✅ Module unloading working correctly")
}

func BenchmarkCallFunction(b *testing.B) {
	logger, _ := zap.NewProduction()
	cfg := &Config{
		EnableJIT:   true,
		EnableDebug: false,
		Logger:      logger,
	}

	runtime, err := NewRuntime(cfg)
	if err != nil {
		b.Fatalf("Failed to create runtime: %v", err)
	}
	defer runtime.Close()

	// Compile and instantiate module
	metadata := &ModuleMetadata{
		Name:        "add",
		Version:     "1.0.0",
		Description: "Simple addition module",
	}

	err = runtime.CompileModule("add", addWasmBytes, metadata)
	if err != nil {
		b.Fatalf("Failed to compile module: %v", err)
	}

	instance, err := runtime.NewModuleInstance("add")
	if err != nil {
		b.Fatalf("Failed to instantiate module: %v", err)
	}
	defer instance.Close()

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := instance.CallFunction(ctx, "add", uint64(i), 1)
		if err != nil {
			b.Fatalf("Failed to call function: %v", err)
		}
	}
}
