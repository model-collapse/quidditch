package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"go.uber.org/zap"
)

// Runtime manages the WASM runtime and module compilation
type Runtime struct {
	runtime wazero.Runtime
	config  *Config
	logger  *zap.Logger
	modules map[string]*CompiledModule
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// Config holds WASM runtime configuration
type Config struct {
	// EnableJIT enables JIT compilation (faster, but needs more memory)
	EnableJIT bool

	// EnableDebug enables debug logging
	EnableDebug bool

	// MaxMemoryPages limits WASM memory (64KB per page)
	MaxMemoryPages uint32

	// Logger for runtime events
	Logger *zap.Logger
}

// CompiledModule represents a compiled WASM module
type CompiledModule struct {
	Name           string
	CompiledModule wazero.CompiledModule
	Metadata       *ModuleMetadata
}

// ModuleMetadata contains information about a WASM module
type ModuleMetadata struct {
	Name        string
	Version     string
	Description string
	Author      string
	Functions   []FunctionInfo
}

// FunctionInfo describes an exported function
type FunctionInfo struct {
	Name       string
	ParamTypes []api.ValueType
	ResultTypes []api.ValueType
}

// NewRuntime creates a new WASM runtime
func NewRuntime(cfg *Config) (*Runtime, error) {
	if cfg == nil {
		cfg = &Config{
			EnableJIT:      true,
			EnableDebug:    false,
			MaxMemoryPages: 256, // 16MB
		}
	}

	if cfg.Logger == nil {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("failed to create logger: %w", err)
		}
		cfg.Logger = logger
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create runtime configuration
	var runtimeConfig wazero.RuntimeConfig
	if cfg.EnableJIT {
		runtimeConfig = wazero.NewRuntimeConfig()
	} else {
		// Interpreter mode (slower but uses less memory)
		runtimeConfig = wazero.NewRuntimeConfigInterpreter()
	}

	// Create wazero runtime
	wasmRuntime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	// Instantiate WASI for standard library support
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, wasmRuntime); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	runtime := &Runtime{
		runtime: wasmRuntime,
		config:  cfg,
		logger:  cfg.Logger,
		modules: make(map[string]*CompiledModule),
		ctx:     ctx,
		cancel:  cancel,
	}

	runtime.logger.Info("WASM runtime initialized",
		zap.Bool("jit_enabled", cfg.EnableJIT),
		zap.Uint32("max_memory_pages", cfg.MaxMemoryPages))

	return runtime, nil
}

// CompileModule compiles a WASM module from bytes
func (r *Runtime) CompileModule(name string, wasmBytes []byte, metadata *ModuleMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if module already exists
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("module %s already compiled", name)
	}

	r.logger.Debug("Compiling WASM module",
		zap.String("name", name),
		zap.Int("size_bytes", len(wasmBytes)))

	// Compile module
	compiled, err := r.runtime.CompileModule(r.ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("failed to compile module %s: %w", name, err)
	}

	// Store compiled module
	r.modules[name] = &CompiledModule{
		Name:           name,
		CompiledModule: compiled,
		Metadata:       metadata,
	}

	r.logger.Info("WASM module compiled successfully",
		zap.String("name", name),
		zap.Int("functions", len(compiled.ExportedFunctions())))

	return nil
}

// GetModule retrieves a compiled module
func (r *Runtime) GetModule(name string) (*CompiledModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, exists := r.modules[name]
	if !exists {
		return nil, fmt.Errorf("module %s not found", name)
	}

	return module, nil
}

// ListModules returns all compiled module names
func (r *Runtime) ListModules() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}

	return names
}

// UnloadModule removes a compiled module
func (r *Runtime) UnloadModule(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	module, exists := r.modules[name]
	if !exists {
		return fmt.Errorf("module %s not found", name)
	}

	// Close the compiled module
	if err := module.CompiledModule.Close(r.ctx); err != nil {
		r.logger.Warn("Error closing module",
			zap.String("name", name),
			zap.Error(err))
	}

	delete(r.modules, name)

	r.logger.Info("WASM module unloaded",
		zap.String("name", name))

	return nil
}

// Close shuts down the WASM runtime
func (r *Runtime) Close() error {
	r.logger.Info("Shutting down WASM runtime")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Close all modules
	for name, module := range r.modules {
		if err := module.CompiledModule.Close(r.ctx); err != nil {
			r.logger.Warn("Error closing module",
				zap.String("name", name),
				zap.Error(err))
		}
	}

	r.modules = nil

	// Close runtime
	if err := r.runtime.Close(r.ctx); err != nil {
		r.logger.Warn("Error closing runtime", zap.Error(err))
	}

	r.cancel()

	r.logger.Info("WASM runtime shut down successfully")

	return nil
}

// GetContext returns the runtime context
func (r *Runtime) GetContext() context.Context {
	return r.ctx
}

// GetWazeroRuntime returns the underlying wazero runtime (for advanced use)
func (r *Runtime) GetWazeroRuntime() wazero.Runtime {
	return r.runtime
}
