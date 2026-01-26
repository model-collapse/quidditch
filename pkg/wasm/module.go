package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"go.uber.org/zap"
)

// ModuleInstance represents an instantiated WASM module
type ModuleInstance struct {
	name     string
	module   api.Module
	runtime  *Runtime
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewModuleInstance creates a new module instance
func (r *Runtime) NewModuleInstance(moduleName string) (*ModuleInstance, error) {
	compiledModule, err := r.GetModule(moduleName)
	if err != nil {
		return nil, err
	}

	r.logger.Debug("Instantiating WASM module", zap.String("name", moduleName))

	// Instantiate module
	module, err := r.runtime.InstantiateModule(r.ctx, compiledModule.CompiledModule, wazero.NewModuleConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module %s: %w", moduleName, err)
	}

	instance := &ModuleInstance{
		name:    moduleName,
		module:  module,
		runtime: r,
		logger:  r.logger.With(zap.String("module", moduleName)),
	}

	instance.logger.Debug("WASM module instantiated successfully")

	return instance, nil
}

// CallFunction calls an exported function in the module
func (mi *ModuleInstance) CallFunction(ctx context.Context, functionName string, params ...uint64) ([]uint64, error) {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	// Get the function
	fn := mi.module.ExportedFunction(functionName)
	if fn == nil {
		return nil, fmt.Errorf("function %s not found in module %s", functionName, mi.name)
	}

	mi.logger.Debug("Calling WASM function",
		zap.String("function", functionName),
		zap.Int("param_count", len(params)))

	// Call the function
	results, err := fn.Call(ctx, params...)
	if err != nil {
		return nil, fmt.Errorf("error calling function %s: %w", functionName, err)
	}

	mi.logger.Debug("WASM function returned",
		zap.String("function", functionName),
		zap.Int("result_count", len(results)))

	return results, nil
}

// GetFunction returns an exported function
func (mi *ModuleInstance) GetFunction(functionName string) api.Function {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	return mi.module.ExportedFunction(functionName)
}

// GetMemory returns the module's memory
func (mi *ModuleInstance) GetMemory() api.Memory {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	return mi.module.Memory()
}

// ReadString reads a string from WASM memory
func (mi *ModuleInstance) ReadString(offset, length uint32) (string, error) {
	memory := mi.GetMemory()
	if memory == nil {
		return "", fmt.Errorf("module has no memory")
	}

	bytes, ok := memory.Read(offset, length)
	if !ok {
		return "", fmt.Errorf("failed to read memory at offset %d, length %d", offset, length)
	}

	return string(bytes), nil
}

// WriteString writes a string to WASM memory
func (mi *ModuleInstance) WriteString(str string, offset uint32) error {
	memory := mi.GetMemory()
	if memory == nil {
		return fmt.Errorf("module has no memory")
	}

	ok := memory.Write(offset, []byte(str))
	if !ok {
		return fmt.Errorf("failed to write memory at offset %d", offset)
	}

	return nil
}

// ReadBytes reads bytes from WASM memory
func (mi *ModuleInstance) ReadBytes(offset, length uint32) ([]byte, error) {
	memory := mi.GetMemory()
	if memory == nil {
		return nil, fmt.Errorf("module has no memory")
	}

	bytes, ok := memory.Read(offset, length)
	if !ok {
		return nil, fmt.Errorf("failed to read memory at offset %d, length %d", offset, length)
	}

	return bytes, nil
}

// WriteBytes writes bytes to WASM memory
func (mi *ModuleInstance) WriteBytes(data []byte, offset uint32) error {
	memory := mi.GetMemory()
	if memory == nil {
		return fmt.Errorf("module has no memory")
	}

	ok := memory.Write(offset, data)
	if !ok {
		return fmt.Errorf("failed to write memory at offset %d", offset)
	}

	return nil
}

// GetMemorySize returns the current memory size in bytes
func (mi *ModuleInstance) GetMemorySize() uint32 {
	memory := mi.GetMemory()
	if memory == nil {
		return 0
	}

	return memory.Size()
}

// ListFunctions returns all exported function names
func (mi *ModuleInstance) ListFunctions() []string {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	// Note: wazero doesn't provide a direct way to list all functions
	// This is a simplified implementation
	// In practice, you'd track function names during compilation
	return []string{}
}

// Close closes the module instance
func (mi *ModuleInstance) Close() error {
	mi.mu.Lock()
	defer mi.mu.Unlock()

	if mi.module == nil {
		return nil
	}

	mi.logger.Debug("Closing WASM module instance")

	if err := mi.module.Close(context.Background()); err != nil {
		return fmt.Errorf("error closing module instance: %w", err)
	}

	mi.module = nil

	mi.logger.Debug("WASM module instance closed successfully")

	return nil
}

// GetName returns the module name
func (mi *ModuleInstance) GetName() string {
	return mi.name
}

// ModulePool manages a pool of reusable module instances
type ModulePool struct {
	runtime  *Runtime
	module   string
	pool     chan *ModuleInstance
	size     int
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewModulePool creates a pool of module instances for reuse
func (r *Runtime) NewModulePool(moduleName string, poolSize int) (*ModulePool, error) {
	if poolSize <= 0 {
		poolSize = 10 // Default pool size
	}

	pool := &ModulePool{
		runtime: r,
		module:  moduleName,
		pool:    make(chan *ModuleInstance, poolSize),
		size:    poolSize,
		logger:  r.logger.With(zap.String("module_pool", moduleName)),
	}

	// Pre-create instances
	for i := 0; i < poolSize; i++ {
		instance, err := r.NewModuleInstance(moduleName)
		if err != nil {
			// Clean up any instances we've created
			pool.Close()
			return nil, fmt.Errorf("failed to create instance %d: %w", i, err)
		}
		pool.pool <- instance
	}

	pool.logger.Info("Module pool created",
		zap.Int("pool_size", poolSize))

	return pool, nil
}

// Get retrieves an instance from the pool
func (mp *ModulePool) Get() (*ModuleInstance, error) {
	select {
	case instance := <-mp.pool:
		return instance, nil
	default:
		// Pool exhausted, create a new instance
		mp.logger.Warn("Module pool exhausted, creating new instance")
		return mp.runtime.NewModuleInstance(mp.module)
	}
}

// Put returns an instance to the pool
func (mp *ModulePool) Put(instance *ModuleInstance) {
	select {
	case mp.pool <- instance:
		// Instance returned to pool
	default:
		// Pool is full, close the instance
		mp.logger.Debug("Module pool full, closing instance")
		instance.Close()
	}
}

// Close closes all instances in the pool
func (mp *ModulePool) Close() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	close(mp.pool)

	for instance := range mp.pool {
		if err := instance.Close(); err != nil {
			mp.logger.Warn("Error closing pooled instance",
				zap.Error(err))
		}
	}

	mp.logger.Info("Module pool closed")

	return nil
}
