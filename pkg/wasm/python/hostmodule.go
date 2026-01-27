package python

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"go.uber.org/zap"
)

// HostModule provides Python-specific host functions for WASM
type HostModule struct {
	logger *zap.Logger
}

// NewHostModule creates a new Python host module
func NewHostModule(logger *zap.Logger) *HostModule {
	return &HostModule{
		logger: logger,
	}
}

// RegisterHostFunctions registers Python-specific host functions
func (h *HostModule) RegisterHostFunctions(ctx context.Context, runtime wazero.Runtime) error {
	// Create host module builder
	builder := runtime.NewHostModuleBuilder("python")

	// Python memory allocation (for Python heap)
	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.pyAlloc), []api.ValueType{
			api.ValueTypeI32, // size
		}, []api.ValueType{
			api.ValueTypeI32, // pointer
		}).
		Export("py_alloc")

	// Python memory deallocation
	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.pyFree), []api.ValueType{
			api.ValueTypeI32, // pointer
		}, []api.ValueType{}).
		Export("py_free")

	// Python print() function
	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.pyPrint), []api.ValueType{
			api.ValueTypeI32, // string pointer
			api.ValueTypeI32, // string length
		}, []api.ValueType{}).
		Export("py_print")

	// Python error handler
	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.pyError), []api.ValueType{
			api.ValueTypeI32, // error message pointer
			api.ValueTypeI32, // error message length
		}, []api.ValueType{}).
		Export("py_error")

	// Python object reference counting (stub for now)
	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.pyIncref), []api.ValueType{
			api.ValueTypeI32, // object pointer
		}, []api.ValueType{}).
		Export("py_incref")

	builder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.pyDecref), []api.ValueType{
			api.ValueTypeI32, // object pointer
		}, []api.ValueType{}).
		Export("py_decref")

	// Instantiate the host module
	if _, err := builder.Instantiate(ctx); err != nil {
		h.logger.Error("Failed to instantiate Python host module", zap.Error(err))
		return fmt.Errorf("failed to instantiate Python host module: %w", err)
	}

	h.logger.Info("Python host functions registered",
		zap.Int("functions", 6))

	return nil
}

// pyAlloc allocates memory for Python heap
func (h *HostModule) pyAlloc(ctx context.Context, mod api.Module, stack []uint64) {
	size := uint32(stack[0])

	// Try to allocate from WASM linear memory
	// In a real implementation, we'd maintain a Python heap allocator
	// For now, we return a placeholder pointer
	ptr := h.allocateFromWASM(mod, size)

	if ptr == 0 {
		h.logger.Warn("Python memory allocation failed",
			zap.Uint32("size", size))
	}

	stack[0] = uint64(ptr)
}

// allocateFromWASM allocates memory from WASM linear memory
func (h *HostModule) allocateFromWASM(mod api.Module, size uint32) uint32 {
	// In a real implementation, we'd call a WASM allocator function
	// or maintain our own heap management

	// For now, return a simple pointer (this is a simplified implementation)
	// Production would need proper memory management

	// Get memory size
	mem := mod.Memory()
	if mem == nil {
		return 0
	}

	memSize := mem.Size()

	// Simple bump allocator (not production-ready)
	// Real implementation would track allocations properly
	if size > memSize {
		return 0
	}

	// Return a pointer in the middle of memory (simplified)
	// Real implementation would maintain free lists
	ptr := uint32(memSize / 2)

	return ptr
}

// pyFree frees Python heap memory
func (h *HostModule) pyFree(ctx context.Context, mod api.Module, stack []uint64) {
	ptr := uint32(stack[0])

	// In a real implementation, we'd return memory to the heap
	// For now, this is a no-op

	h.logger.Debug("Python memory freed",
		zap.Uint32("ptr", ptr))
}

// pyPrint handles Python print() calls
func (h *HostModule) pyPrint(ctx context.Context, mod api.Module, stack []uint64) {
	ptr := uint32(stack[0])
	length := uint32(stack[1])

	// Read string from WASM memory
	mem := mod.Memory()
	if mem == nil {
		h.logger.Error("Module has no memory")
		return
	}

	strBytes, ok := mem.Read(ptr, length)
	if !ok {
		h.logger.Error("Failed to read string from WASM memory",
			zap.Uint32("ptr", ptr),
			zap.Uint32("length", length))
		return
	}

	// Log the printed string
	str := string(strBytes)
	h.logger.Info("Python print()",
		zap.String("output", str))
}

// pyError handles Python errors
func (h *HostModule) pyError(ctx context.Context, mod api.Module, stack []uint64) {
	ptr := uint32(stack[0])
	length := uint32(stack[1])

	// Read error message from WASM memory
	mem := mod.Memory()
	if mem == nil {
		h.logger.Error("Module has no memory")
		return
	}

	errBytes, ok := mem.Read(ptr, length)
	if !ok {
		h.logger.Error("Failed to read error message from WASM memory",
			zap.Uint32("ptr", ptr),
			zap.Uint32("length", length))
		return
	}

	// Log the error
	errMsg := string(errBytes)
	h.logger.Error("Python error",
		zap.String("message", errMsg))
}

// pyIncref increments Python object reference count
func (h *HostModule) pyIncref(ctx context.Context, mod api.Module, stack []uint64) {
	objPtr := uint32(stack[0])

	// In a real implementation, we'd increment the refcount
	// For now, this is a no-op

	h.logger.Debug("Python INCREF",
		zap.Uint32("object", objPtr))
}

// pyDecref decrements Python object reference count
func (h *HostModule) pyDecref(ctx context.Context, mod api.Module, stack []uint64) {
	objPtr := uint32(stack[0])

	// In a real implementation, we'd decrement the refcount
	// and possibly free the object
	// For now, this is a no-op

	h.logger.Debug("Python DECREF",
		zap.Uint32("object", objPtr))
}

// Helper functions for Python type conversion

// WritePythonInt writes a Python integer to WASM memory
func WritePythonInt(mem api.Memory, ptr uint32, value int64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(value))
	if !mem.Write(ptr, buf) {
		return fmt.Errorf("failed to write integer to memory")
	}
	return nil
}

// ReadPythonInt reads a Python integer from WASM memory
func ReadPythonInt(mem api.Memory, ptr uint32) (int64, error) {
	buf, ok := mem.Read(ptr, 8)
	if !ok {
		return 0, fmt.Errorf("failed to read integer from memory")
	}
	return int64(binary.LittleEndian.Uint64(buf)), nil
}

// WritePythonFloat writes a Python float to WASM memory
func WritePythonFloat(mem api.Memory, ptr uint32, value float64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(value))
	if !mem.Write(ptr, buf) {
		return fmt.Errorf("failed to write float to memory")
	}
	return nil
}

// ReadPythonFloat reads a Python float from WASM memory
func ReadPythonFloat(mem api.Memory, ptr uint32) (float64, error) {
	buf, ok := mem.Read(ptr, 8)
	if !ok {
		return 0, fmt.Errorf("failed to read float from memory")
	}
	return float64(binary.LittleEndian.Uint64(buf)), nil
}

// WritePythonString writes a Python string to WASM memory
func WritePythonString(mem api.Memory, ptr uint32, value string) error {
	if !mem.Write(ptr, []byte(value)) {
		return fmt.Errorf("failed to write string to memory")
	}
	return nil
}

// ReadPythonString reads a Python string from WASM memory
func ReadPythonString(mem api.Memory, ptr uint32, length uint32) (string, error) {
	buf, ok := mem.Read(ptr, length)
	if !ok {
		return "", fmt.Errorf("failed to read string from memory")
	}
	return string(buf), nil
}

// WritePythonBool writes a Python boolean to WASM memory
func WritePythonBool(mem api.Memory, ptr uint32, value bool) error {
	var b byte
	if value {
		b = 1
	} else {
		b = 0
	}
	if !mem.Write(ptr, []byte{b}) {
		return fmt.Errorf("failed to write boolean to memory")
	}
	return nil
}

// ReadPythonBool reads a Python boolean from WASM memory
func ReadPythonBool(mem api.Memory, ptr uint32) (bool, error) {
	buf, ok := mem.Read(ptr, 1)
	if !ok {
		return false, fmt.Errorf("failed to read boolean from memory")
	}
	return buf[0] != 0, nil
}
