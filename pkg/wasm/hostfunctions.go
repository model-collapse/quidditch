package wasm

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"go.uber.org/zap"
)

// HostFunctions manages the host functions available to WASM modules
type HostFunctions struct {
	logger   *zap.Logger
	contexts map[uint64]*DocumentContext // Context ID → DocumentContext
	nextID   uint64
	runtime  *Runtime
	mu       sync.RWMutex // Protects contexts and nextID
}

// NewHostFunctions creates a new host functions manager
func NewHostFunctions(runtime *Runtime) *HostFunctions {
	return &HostFunctions{
		logger:   runtime.logger.With(zap.String("component", "host_functions")),
		contexts: make(map[uint64]*DocumentContext),
		nextID:   1,
		runtime:  runtime,
	}
}

// RegisterContext registers a document context and returns its ID
func (hf *HostFunctions) RegisterContext(ctx *DocumentContext) uint64 {
	hf.mu.Lock()
	defer hf.mu.Unlock()
	id := hf.nextID
	hf.nextID++
	hf.contexts[id] = ctx
	return id
}

// UnregisterContext removes a document context
func (hf *HostFunctions) UnregisterContext(id uint64) {
	hf.mu.Lock()
	defer hf.mu.Unlock()
	delete(hf.contexts, id)
}

// GetContext retrieves a document context by ID
func (hf *HostFunctions) GetContext(id uint64) (*DocumentContext, bool) {
	hf.mu.RLock()
	defer hf.mu.RUnlock()
	ctx, exists := hf.contexts[id]
	return ctx, exists
}

// RegisterHostFunctions registers all host functions with the WASM runtime
// This must be called before instantiating modules that use these functions
func (hf *HostFunctions) RegisterHostFunctions(ctx context.Context, runtime wazero.Runtime) error {
	// Create host module builder
	hostBuilder := runtime.NewHostModuleBuilder("env")

	// Register field access functions using GoModuleFunction
	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.getFieldString), []api.ValueType{
			api.ValueTypeI64, // ctx_id
			api.ValueTypeI32, // field_ptr
			api.ValueTypeI32, // field_len
			api.ValueTypeI32, // result_ptr
			api.ValueTypeI32, // result_len_ptr
		}, []api.ValueType{api.ValueTypeI32}).
		Export("get_field_string")

	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.getFieldInt64), []api.ValueType{
			api.ValueTypeI64, // ctx_id
			api.ValueTypeI32, // field_ptr
			api.ValueTypeI32, // field_len
		}, []api.ValueType{api.ValueTypeI64}).
		Export("get_field_int64")

	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.getFieldFloat64), []api.ValueType{
			api.ValueTypeI64, // ctx_id
			api.ValueTypeI32, // field_ptr
			api.ValueTypeI32, // field_len
		}, []api.ValueType{api.ValueTypeF64}).
		Export("get_field_float64")

	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.getFieldBool), []api.ValueType{
			api.ValueTypeI64, // ctx_id
			api.ValueTypeI32, // field_ptr
			api.ValueTypeI32, // field_len
		}, []api.ValueType{api.ValueTypeI32}).
		Export("get_field_bool")

	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.hasField), []api.ValueType{
			api.ValueTypeI64, // ctx_id
			api.ValueTypeI32, // field_ptr
			api.ValueTypeI32, // field_len
		}, []api.ValueType{api.ValueTypeI32}).
		Export("has_field")

	// Register document metadata functions
	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.getDocumentID), []api.ValueType{
			api.ValueTypeI64, // ctx_id
			api.ValueTypeI32, // result_ptr
			api.ValueTypeI32, // result_len_ptr
		}, []api.ValueType{api.ValueTypeI32}).
		Export("get_document_id")

	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.getScore), []api.ValueType{
			api.ValueTypeI64, // ctx_id
		}, []api.ValueType{api.ValueTypeF64}).
		Export("get_score")

	// Register logging function for debugging
	hostBuilder.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(hf.log), []api.ValueType{
			api.ValueTypeI32, // msg_ptr
			api.ValueTypeI32, // msg_len
		}, []api.ValueType{}).
		Export("log")

	// Instantiate the host module
	if _, err := hostBuilder.Instantiate(ctx); err != nil {
		return fmt.Errorf("failed to instantiate host module: %w", err)
	}

	hf.logger.Info("Host functions registered")
	return nil
}

// getFieldString retrieves a string field value
// Parameters: ctx_id, field_ptr, field_len, result_ptr, result_len_ptr
// Returns: 1 if field exists, 0 if not
func (hf *HostFunctions) getFieldString(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]
	fieldPtr := uint32(stack[1])
	fieldLen := uint32(stack[2])
	resultPtr := uint32(stack[3])
	resultLenPtr := uint32(stack[4])

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = 0 // Field not found
		return
	}

	// Read field path from WASM memory
	fieldPath, ok := mod.Memory().Read(fieldPtr, fieldLen)
	if !ok {
		hf.logger.Warn("Failed to read field path from WASM memory")
		stack[0] = 0
		return
	}

	// Get field value
	value, exists := docCtx.GetFieldString(string(fieldPath))
	if !exists {
		stack[0] = 0 // Field not found
		return
	}

	// Write result to WASM memory
	valueBytes := []byte(value)
	if !mod.Memory().Write(resultPtr, valueBytes) {
		hf.logger.Warn("Failed to write result to WASM memory")
		stack[0] = 0
		return
	}

	// Write result length
	lengthBytes := uint32ToBytes(uint32(len(valueBytes)))
	if !mod.Memory().Write(resultLenPtr, lengthBytes) {
		hf.logger.Warn("Failed to write result length to WASM memory")
		stack[0] = 0
		return
	}

	stack[0] = 1 // Success
}

// getFieldInt64 retrieves an int64 field value
// Parameters: ctx_id, field_ptr, field_len
// Returns: value (or 0 if not found) in lower 32 bits, exists flag in upper 32 bits
func (hf *HostFunctions) getFieldInt64(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]
	fieldPtr := uint32(stack[1])
	fieldLen := uint32(stack[2])

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = 0 // Not found
		return
	}

	// Read field path from WASM memory
	fieldPath, ok := mod.Memory().Read(fieldPtr, fieldLen)
	if !ok {
		hf.logger.Warn("Failed to read field path from WASM memory")
		stack[0] = 0
		return
	}

	// Get field value
	value, exists := docCtx.GetFieldInt64(string(fieldPath))
	if !exists {
		stack[0] = 0 // Not found
		return
	}

	stack[0] = uint64(value)
}

// getFieldFloat64 retrieves a float64 field value
// Parameters: ctx_id, field_ptr, field_len
// Returns: value (or 0 if not found)
func (hf *HostFunctions) getFieldFloat64(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]
	fieldPtr := uint32(stack[1])
	fieldLen := uint32(stack[2])

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = api.EncodeF64(0)
		return
	}

	// Read field path from WASM memory
	fieldPath, ok := mod.Memory().Read(fieldPtr, fieldLen)
	if !ok {
		hf.logger.Warn("Failed to read field path from WASM memory")
		stack[0] = api.EncodeF64(0)
		return
	}

	// Get field value
	value, exists := docCtx.GetFieldFloat64(string(fieldPath))
	if !exists {
		stack[0] = api.EncodeF64(0)
		return
	}

	stack[0] = api.EncodeF64(value)
}

// getFieldBool retrieves a bool field value
// Parameters: ctx_id, field_ptr, field_len
// Returns: 1 for true, 0 for false/not found
func (hf *HostFunctions) getFieldBool(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]
	fieldPtr := uint32(stack[1])
	fieldLen := uint32(stack[2])

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = 0
		return
	}

	// Read field path from WASM memory
	fieldPath, ok := mod.Memory().Read(fieldPtr, fieldLen)
	if !ok {
		hf.logger.Warn("Failed to read field path from WASM memory")
		stack[0] = 0
		return
	}

	// Get field value
	value, exists := docCtx.GetFieldBool(string(fieldPath))
	if !exists || !value {
		stack[0] = 0
		return
	}

	stack[0] = 1
}

// hasField checks if a field exists
// Parameters: ctx_id, field_ptr, field_len
// Returns: 1 if exists, 0 if not
func (hf *HostFunctions) hasField(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]
	fieldPtr := uint32(stack[1])
	fieldLen := uint32(stack[2])

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = 0
		return
	}

	// Read field path from WASM memory
	fieldPath, ok := mod.Memory().Read(fieldPtr, fieldLen)
	if !ok {
		hf.logger.Warn("Failed to read field path from WASM memory")
		stack[0] = 0
		return
	}

	// Check if field exists
	if docCtx.HasField(string(fieldPath)) {
		stack[0] = 1
	} else {
		stack[0] = 0
	}
}

// getDocumentID retrieves the document ID
// Parameters: ctx_id, result_ptr, result_len_ptr
// Returns: 1 on success, 0 on failure
func (hf *HostFunctions) getDocumentID(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]
	resultPtr := uint32(stack[1])
	resultLenPtr := uint32(stack[2])

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = 0
		return
	}

	// Get document ID
	docID := docCtx.GetDocumentID()
	docIDBytes := []byte(docID)

	// Write to WASM memory
	if !mod.Memory().Write(resultPtr, docIDBytes) {
		hf.logger.Warn("Failed to write document ID to WASM memory")
		stack[0] = 0
		return
	}

	// Write length
	lengthBytes := uint32ToBytes(uint32(len(docIDBytes)))
	if !mod.Memory().Write(resultLenPtr, lengthBytes) {
		hf.logger.Warn("Failed to write document ID length to WASM memory")
		stack[0] = 0
		return
	}

	stack[0] = 1
}

// getScore retrieves the document score
// Parameters: ctx_id
// Returns: score as float64
func (hf *HostFunctions) getScore(ctx context.Context, mod api.Module, stack []uint64) {
	ctxID := stack[0]

	// Get document context
	docCtx, exists := hf.GetContext(ctxID)
	if !exists {
		stack[0] = api.EncodeF64(0)
		return
	}

	score := docCtx.GetScore()
	stack[0] = api.EncodeF64(score)
}

// log logs a message from WASM (for debugging)
// Parameters: msg_ptr, msg_len
func (hf *HostFunctions) log(ctx context.Context, mod api.Module, stack []uint64) {
	msgPtr := uint32(stack[0])
	msgLen := uint32(stack[1])

	// Read message from WASM memory
	msgBytes, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		hf.logger.Warn("Failed to read log message from WASM memory")
		return
	}

	hf.logger.Debug("WASM log", zap.String("message", string(msgBytes)))
}

// uint32ToBytes converts uint32 to byte slice
func uint32ToBytes(v uint32) []byte {
	bytes := make([]byte, 4)
	*(*uint32)(unsafe.Pointer(&bytes[0])) = v
	return bytes
}

// bytesToUint32 converts byte slice to uint32
func bytesToUint32(bytes []byte) uint32 {
	if len(bytes) < 4 {
		return 0
	}
	return *(*uint32)(unsafe.Pointer(&bytes[0]))
}
