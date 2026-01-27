package wasm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// UDFRegistry manages User-Defined Functions (UDFs) compiled as WASM modules
type UDFRegistry struct {
	runtime      *Runtime
	hostFuncs    *HostFunctions
	logger       *zap.Logger

	// UDF storage
	udfs         map[string]*RegisteredUDF // name@version → UDF
	pools        map[string]*ModulePool    // name@version → pool
	stats        map[string]*UDFStats      // name@version → stats

	// Configuration
	defaultPoolSize int
	enableStats     bool

	mu               sync.RWMutex
}

// RegisteredUDF represents a registered WASM UDF
type RegisteredUDF struct {
	Metadata   *UDFMetadata
	ModuleName string // Internal module name in runtime
	Pool       *ModulePool
	Stats      *UDFStats
}

// UDFRegistryConfig configures the UDF registry
type UDFRegistryConfig struct {
	Runtime         *Runtime
	DefaultPoolSize int  // Default module pool size (0 = no pooling)
	EnableStats     bool // Enable call statistics
	Logger          *zap.Logger
}

// NewUDFRegistry creates a new UDF registry
func NewUDFRegistry(cfg *UDFRegistryConfig) (*UDFRegistry, error) {
	if cfg.Runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}

	if cfg.Logger == nil {
		cfg.Logger = cfg.Runtime.logger
	}

	if cfg.DefaultPoolSize <= 0 {
		cfg.DefaultPoolSize = 10
	}

	// Create host functions
	hostFuncs := NewHostFunctions(cfg.Runtime)
	if err := hostFuncs.RegisterHostFunctions(cfg.Runtime.GetContext(), cfg.Runtime.GetWazeroRuntime()); err != nil {
		return nil, fmt.Errorf("failed to register host functions: %w", err)
	}

	registry := &UDFRegistry{
		runtime:         cfg.Runtime,
		hostFuncs:       hostFuncs,
		logger:          cfg.Logger.With(zap.String("component", "udf_registry")),
		udfs:            make(map[string]*RegisteredUDF),
		pools:           make(map[string]*ModulePool),
		stats:           make(map[string]*UDFStats),
		defaultPoolSize: cfg.DefaultPoolSize,
		enableStats:     cfg.EnableStats,
	}

	registry.logger.Info("UDF registry initialized",
		zap.Int("default_pool_size", cfg.DefaultPoolSize),
		zap.Bool("stats_enabled", cfg.EnableStats))

	return registry, nil
}

// Register registers a new UDF
func (r *UDFRegistry) Register(metadata *UDFMetadata) error {
	return r.RegisterWithPoolSize(metadata, r.defaultPoolSize)
}

// RegisterWithPoolSize registers a new UDF with a specific pool size
func (r *UDFRegistry) RegisterWithPoolSize(metadata *UDFMetadata, poolSize int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate metadata
	if err := metadata.Validate(); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	fullName := metadata.GetFullName()

	// Check if already registered
	if _, exists := r.udfs[fullName]; exists {
		return fmt.Errorf("UDF %s already registered", fullName)
	}

	r.logger.Info("Registering UDF",
		zap.String("name", metadata.Name),
		zap.String("version", metadata.Version),
		zap.String("function", metadata.FunctionName))

	// Compile module
	moduleName := fmt.Sprintf("udf_%s_%s", metadata.Name, metadata.Version)
	moduleMetadata := &ModuleMetadata{
		Name:        moduleName,
		Version:     metadata.Version,
		Description: metadata.Description,
		Author:      metadata.Author,
	}

	if err := r.runtime.CompileModule(moduleName, metadata.WASMBytes, moduleMetadata); err != nil {
		return fmt.Errorf("failed to compile UDF: %w", err)
	}

	// Create module pool if requested
	var pool *ModulePool
	if poolSize > 0 {
		var err error
		pool, err = r.runtime.NewModulePool(moduleName, poolSize)
		if err != nil {
			// Cleanup compiled module
			r.runtime.UnloadModule(moduleName)
			return fmt.Errorf("failed to create module pool: %w", err)
		}
		r.pools[fullName] = pool
	}

	// Set timestamps
	now := time.Now()
	if metadata.RegisteredAt.IsZero() {
		metadata.RegisteredAt = now
	}
	metadata.UpdatedAt = now
	metadata.WASMSize = len(metadata.WASMBytes)

	// Create registered UDF
	registered := &RegisteredUDF{
		Metadata:   metadata,
		ModuleName: moduleName,
		Pool:       pool,
	}

	// Initialize stats if enabled
	if r.enableStats {
		stats := &UDFStats{
			Name:    metadata.Name,
			Version: metadata.Version,
		}
		r.stats[fullName] = stats
		registered.Stats = stats
	}

	r.udfs[fullName] = registered

	r.logger.Info("UDF registered successfully",
		zap.String("name", metadata.Name),
		zap.String("version", metadata.Version),
		zap.Int("pool_size", poolSize))

	return nil
}

// Unregister removes a UDF from the registry
func (r *UDFRegistry) Unregister(name, version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullName := fmt.Sprintf("%s@%s", name, version)

	registered, exists := r.udfs[fullName]
	if !exists {
		return fmt.Errorf("UDF %s not found", fullName)
	}

	r.logger.Info("Unregistering UDF",
		zap.String("name", name),
		zap.String("version", version))

	// Close pool if exists
	if pool, exists := r.pools[fullName]; exists {
		pool.Close()
		delete(r.pools, fullName)
	}

	// Unload module
	if err := r.runtime.UnloadModule(registered.ModuleName); err != nil {
		r.logger.Warn("Failed to unload module",
			zap.String("module", registered.ModuleName),
			zap.Error(err))
	}

	// Remove from registry
	delete(r.udfs, fullName)
	delete(r.stats, fullName)

	r.logger.Info("UDF unregistered successfully",
		zap.String("name", name),
		zap.String("version", version))

	return nil
}

// Get retrieves a registered UDF
func (r *UDFRegistry) Get(name, version string) (*RegisteredUDF, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fullName := fmt.Sprintf("%s@%s", name, version)

	registered, exists := r.udfs[fullName]
	if !exists {
		return nil, fmt.Errorf("UDF %s not found", fullName)
	}

	return registered, nil
}

// GetLatest retrieves the latest version of a UDF
func (r *UDFRegistry) GetLatest(name string) (*RegisteredUDF, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *RegisteredUDF
	var latestTime time.Time

	for _, registered := range r.udfs {
		if registered.Metadata.Name == name {
			if latest == nil || registered.Metadata.RegisteredAt.After(latestTime) {
				latest = registered
				latestTime = registered.Metadata.RegisteredAt
			}
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("UDF %s not found", name)
	}

	return latest, nil
}

// List returns all registered UDFs
func (r *UDFRegistry) List() []*UDFMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadatas := make([]*UDFMetadata, 0, len(r.udfs))
	for _, registered := range r.udfs {
		metadatas = append(metadatas, registered.Metadata.Clone())
	}

	return metadatas
}

// Query finds UDFs matching the query criteria
func (r *UDFRegistry) Query(query *UDFQuery) []*UDFMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*UDFMetadata
	for _, registered := range r.udfs {
		if query.Matches(registered.Metadata) {
			results = append(results, registered.Metadata.Clone())
		}
	}

	return results
}

// Call executes a UDF with the provided document context and parameters
func (r *UDFRegistry) Call(ctx context.Context, name, version string, docCtx *DocumentContext, params map[string]Value) ([]Value, error) {
	startTime := time.Now()

	// Get registered UDF
	registered, err := r.Get(name, version)
	if err != nil {
		return nil, err
	}

	// Register document context with host functions
	ctxID := r.hostFuncs.RegisterContext(docCtx)
	defer r.hostFuncs.UnregisterContext(ctxID)

	// Validate parameters against metadata
	if err := r.validateParameters(registered.Metadata, params); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// Register parameters for host function access
	paramMap := make(map[string]interface{})
	for name, val := range params {
		// Convert Value to native Go type
		switch val.Type {
		case ValueTypeI32:
			if v, err := val.AsInt32(); err == nil {
				paramMap[name] = v
			}
		case ValueTypeI64:
			if v, err := val.AsInt64(); err == nil {
				paramMap[name] = v
			}
		case ValueTypeF32:
			if v, err := val.AsFloat32(); err == nil {
				paramMap[name] = v
			}
		case ValueTypeF64:
			if v, err := val.AsFloat64(); err == nil {
				paramMap[name] = v
			}
		case ValueTypeString:
			if v, err := val.AsString(); err == nil {
				paramMap[name] = v
			}
		case ValueTypeBool:
			if v, err := val.AsBool(); err == nil {
				paramMap[name] = v
			}
		}
	}
	r.hostFuncs.RegisterParameters(paramMap)
	defer r.hostFuncs.UnregisterParameters()

	// Convert parameters to uint64 array
	wasmParams, err := r.prepareParameters(registered.Metadata, params, ctxID)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare parameters: %w", err)
	}

	// Get module instance
	var instance *ModuleInstance
	if registered.Pool != nil {
		instance, err = registered.Pool.Get()
		if err != nil {
			return nil, fmt.Errorf("failed to get instance from pool: %w", err)
		}
		defer registered.Pool.Put(instance)
	} else {
		instance, err = r.runtime.NewModuleInstance(registered.ModuleName)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance: %w", err)
		}
		defer instance.Close()
	}

	// Call function
	results, err := instance.CallFunction(ctx, registered.Metadata.FunctionName, wasmParams...)

	// Update stats
	duration := time.Since(startTime)
	if r.enableStats && registered.Stats != nil {
		registered.Stats.UpdateStats(duration, err)
	}

	if err != nil {
		return nil, fmt.Errorf("UDF call failed: %w", err)
	}

	// Convert results
	values, err := r.convertResults(registered.Metadata, results)
	if err != nil {
		return nil, fmt.Errorf("failed to convert results: %w", err)
	}

	return values, nil
}

// validateParameters validates parameters against UDF metadata
func (r *UDFRegistry) validateParameters(metadata *UDFMetadata, params map[string]Value) error {
	// Check required parameters
	for _, param := range metadata.Parameters {
		if param.Required {
			if _, exists := params[param.Name]; !exists {
				return fmt.Errorf("required parameter missing: %s", param.Name)
			}
		}
	}

	// Check parameter types
	for name, value := range params {
		param, exists := metadata.GetParameterByName(name)
		if !exists {
			return fmt.Errorf("unknown parameter: %s", name)
		}

		if value.Type != param.Type {
			return fmt.Errorf("parameter %s: expected type %v, got %v", name, param.Type, value.Type)
		}
	}

	return nil
}

// prepareParameters converts parameters to uint64 array for WASM
func (r *UDFRegistry) prepareParameters(metadata *UDFMetadata, params map[string]Value, ctxID uint64) ([]uint64, error) {
	// First parameter is always context ID
	wasmParams := []uint64{ctxID}

	// Add parameters in order defined in metadata
	for _, param := range metadata.Parameters {
		value, exists := params[param.Name]
		if !exists {
			// Use default value if available
			if param.Default != nil {
				value = r.createValueFromDefault(param.Type, param.Default)
			} else {
				// Use zero value for optional parameters
				value = r.createZeroValue(param.Type)
			}
		}

		// Convert to uint64
		u64, err := value.ToUint64()
		if err != nil {
			return nil, fmt.Errorf("parameter %s: %w", param.Name, err)
		}

		wasmParams = append(wasmParams, u64)
	}

	return wasmParams, nil
}

// convertResults converts WASM results to Value array
func (r *UDFRegistry) convertResults(metadata *UDFMetadata, results []uint64) ([]Value, error) {
	if len(results) != len(metadata.Returns) {
		return nil, fmt.Errorf("expected %d results, got %d", len(metadata.Returns), len(results))
	}

	values := make([]Value, len(results))
	for i, result := range results {
		returnType := metadata.Returns[i]
		value, err := FromUint64(result, returnType.Type)
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", i, err)
		}
		values[i] = value
	}

	return values, nil
}

// createValueFromDefault creates a Value from a default value
func (r *UDFRegistry) createValueFromDefault(typ ValueType, def interface{}) Value {
	switch typ {
	case ValueTypeI32:
		switch v := def.(type) {
		case int:
			return NewI32Value(int32(v))
		case int32:
			return NewI32Value(v)
		case int64:
			return NewI32Value(int32(v))
		case float64:
			return NewI32Value(int32(v))
		}
	case ValueTypeI64:
		switch v := def.(type) {
		case int:
			return NewI64Value(int64(v))
		case int32:
			return NewI64Value(int64(v))
		case int64:
			return NewI64Value(v)
		case float64:
			return NewI64Value(int64(v))
		}
	case ValueTypeF32:
		switch v := def.(type) {
		case float32:
			return NewF32Value(v)
		case float64:
			return NewF32Value(float32(v))
		case int:
			return NewF32Value(float32(v))
		}
	case ValueTypeF64:
		switch v := def.(type) {
		case float64:
			return NewF64Value(v)
		case float32:
			return NewF64Value(float64(v))
		case int:
			return NewF64Value(float64(v))
		}
	case ValueTypeString:
		if v, ok := def.(string); ok {
			return NewStringValue(v)
		}
	case ValueTypeBool:
		if v, ok := def.(bool); ok {
			return NewBoolValue(v)
		}
	}

	// Fallback to zero value
	return r.createZeroValue(typ)
}

// createZeroValue creates a zero Value for a type
func (r *UDFRegistry) createZeroValue(typ ValueType) Value {
	switch typ {
	case ValueTypeI32:
		return NewI32Value(0)
	case ValueTypeI64:
		return NewI64Value(0)
	case ValueTypeF32:
		return NewF32Value(0)
	case ValueTypeF64:
		return NewF64Value(0)
	case ValueTypeString:
		return NewStringValue("")
	case ValueTypeBool:
		return NewBoolValue(false)
	default:
		return NewI32Value(0)
	}
}

// GetStats returns statistics for a UDF
func (r *UDFRegistry) GetStats(name, version string) (*UDFStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fullName := fmt.Sprintf("%s@%s", name, version)

	stats, exists := r.stats[fullName]
	if !exists {
		return nil, fmt.Errorf("UDF %s not found or stats not enabled", fullName)
	}

	// Return a copy
	statsCopy := *stats
	return &statsCopy, nil
}

// GetAllStats returns statistics for all UDFs
func (r *UDFRegistry) GetAllStats() map[string]*UDFStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	allStats := make(map[string]*UDFStats)
	for name, stats := range r.stats {
		statsCopy := *stats
		allStats[name] = &statsCopy
	}

	return allStats
}

// Close shuts down the registry and cleans up resources
func (r *UDFRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Shutting down UDF registry")

	// Close all pools
	for _, pool := range r.pools {
		pool.Close()
	}

	// Clear maps
	r.udfs = nil
	r.pools = nil
	r.stats = nil

	r.logger.Info("UDF registry shut down successfully")

	return nil
}
