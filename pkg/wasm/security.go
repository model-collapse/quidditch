package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ResourceLimits defines execution limits for UDF modules
type ResourceLimits struct {
	MaxMemoryPages   uint32        // Maximum WASM memory pages (1 page = 64KB)
	MaxExecutionTime time.Duration // Maximum execution time per call
	MaxStackDepth    int           // Maximum call stack depth
	MaxInstances     int           // Maximum concurrent instances
}

// DefaultResourceLimits returns sensible default limits
func DefaultResourceLimits() *ResourceLimits {
	return &ResourceLimits{
		MaxMemoryPages:   256,              // 16MB (256 * 64KB)
		MaxExecutionTime: 5 * time.Second,  // 5 seconds max
		MaxStackDepth:    1024,             // 1024 call frames
		MaxInstances:     100,              // 100 concurrent instances
	}
}

// Permission represents a UDF capability permission
type Permission string

const (
	PermissionReadDocument  Permission = "read_document"  // Read document fields
	PermissionWriteLog      Permission = "write_log"      // Write to logs
	PermissionNetworkAccess Permission = "network_access" // Make network requests (future)
	PermissionFileAccess    Permission = "file_access"    // Access files (future)
	PermissionSystemCall    Permission = "system_call"    // Make system calls (future)
)

// UDFPermissions manages allowed permissions for a UDF
type UDFPermissions struct {
	Allowed []Permission // List of allowed permissions
	mu      sync.RWMutex
}

// NewUDFPermissions creates a permission set with specified permissions
func NewUDFPermissions(permissions ...Permission) *UDFPermissions {
	return &UDFPermissions{
		Allowed: permissions,
	}
}

// DefaultPermissions returns safe default permissions
func DefaultPermissions() *UDFPermissions {
	return NewUDFPermissions(
		PermissionReadDocument,
		PermissionWriteLog,
	)
}

// Has checks if a permission is allowed
func (p *UDFPermissions) Has(perm Permission) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, allowed := range p.Allowed {
		if allowed == perm {
			return true
		}
	}
	return false
}

// Add grants a permission
func (p *UDFPermissions) Add(perm Permission) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if already exists
	for _, allowed := range p.Allowed {
		if allowed == perm {
			return
		}
	}

	p.Allowed = append(p.Allowed, perm)
}

// Remove revokes a permission
func (p *UDFPermissions) Remove(perm Permission) {
	p.mu.Lock()
	defer p.mu.Unlock()

	filtered := make([]Permission, 0, len(p.Allowed))
	for _, allowed := range p.Allowed {
		if allowed != perm {
			filtered = append(filtered, allowed)
		}
	}
	p.Allowed = filtered
}

// UDFSignature represents a cryptographic signature for a WASM module
type UDFSignature struct {
	WASMHash  string    // SHA256 hash of WASM binary
	Signature string    // Cryptographic signature (hex-encoded)
	PublicKey string    // Public key for verification (hex-encoded)
	SignedAt  time.Time // When the signature was created
	Signer    string    // Identity of the signer
}

// SignWASM creates a signature for a WASM binary
// Note: This is a simplified implementation. Production would use proper
// cryptographic libraries (e.g., crypto/ed25519, crypto/ecdsa)
func SignWASM(wasmBytes []byte, signer string) *UDFSignature {
	// Calculate SHA256 hash
	hash := sha256.Sum256(wasmBytes)
	hashStr := hex.EncodeToString(hash[:])

	// In production, this would use actual private key signing
	// For now, we just create a dummy signature
	signature := fmt.Sprintf("sig_%s_%d", hashStr[:16], time.Now().Unix())

	return &UDFSignature{
		WASMHash:  hashStr,
		Signature: signature,
		PublicKey: "pubkey_placeholder",
		SignedAt:  time.Now(),
		Signer:    signer,
	}
}

// VerifyWASM verifies a WASM binary against a signature
func VerifyWASM(wasmBytes []byte, sig *UDFSignature) error {
	if sig == nil {
		return fmt.Errorf("signature is nil")
	}

	// Verify hash
	hash := sha256.Sum256(wasmBytes)
	hashStr := hex.EncodeToString(hash[:])

	if hashStr != sig.WASMHash {
		return fmt.Errorf("WASM hash mismatch: expected %s, got %s", sig.WASMHash, hashStr)
	}

	// In production, this would verify the cryptographic signature
	// using the public key

	return nil
}

// ExecutionLimiter enforces resource limits during UDF execution
type ExecutionLimiter struct {
	limits        *ResourceLimits
	instances     sync.Map              // instanceID -> instanceInfo
	instanceCount map[string]int        // udfName -> count
	mu            sync.Mutex
}

// NewExecutionLimiter creates a new execution limiter
func NewExecutionLimiter(limits *ResourceLimits) *ExecutionLimiter {
	return &ExecutionLimiter{
		limits:        limits,
		instanceCount: make(map[string]int),
	}
}

// AcquireInstance attempts to acquire an execution slot
func (el *ExecutionLimiter) AcquireInstance(udfName string) error {
	el.mu.Lock()
	defer el.mu.Unlock()

	// Check current count for this UDF
	count := el.instanceCount[udfName]

	if count >= el.limits.MaxInstances {
		return fmt.Errorf("instance limit exceeded: %d/%d active", count, el.limits.MaxInstances)
	}

	// Generate instance ID
	instanceID := fmt.Sprintf("%s_%d", udfName, time.Now().UnixNano())
	el.instances.Store(instanceID, udfName)

	// Increment count
	el.instanceCount[udfName]++

	return nil
}

// ReleaseInstance releases an execution slot
func (el *ExecutionLimiter) ReleaseInstance(instanceID string) {
	el.mu.Lock()
	defer el.mu.Unlock()

	// Get the UDF name from the instance
	if udfNameVal, ok := el.instances.Load(instanceID); ok {
		udfName := udfNameVal.(string)

		// Delete the instance
		el.instances.Delete(instanceID)

		// Decrement count
		if el.instanceCount[udfName] > 0 {
			el.instanceCount[udfName]--
		}

		// Clean up map entry if count reaches 0
		if el.instanceCount[udfName] == 0 {
			delete(el.instanceCount, udfName)
		}
	}
}

// ExecuteWithLimits executes a function with resource limits applied
func (el *ExecutionLimiter) ExecuteWithLimits(ctx context.Context, fn func() error) error {
	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, el.limits.MaxExecutionTime)
	defer cancel()

	// Execute in goroutine with timeout
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("execution timeout after %v", el.limits.MaxExecutionTime)
		}
		return ctx.Err()
	}
}

// AuditLog represents a log entry for UDF operations
type AuditLog struct {
	Timestamp   time.Time         // When the operation occurred
	Operation   string            // Operation type (register, call, delete, etc.)
	UDFName     string            // UDF name
	UDFVersion  string            // UDF version
	User        string            // User who performed the operation
	Success     bool              // Whether the operation succeeded
	Error       string            // Error message if failed
	Duration    time.Duration     // How long the operation took
	Metadata    map[string]string // Additional metadata
}

// AuditLogger logs UDF operations for security and compliance
type AuditLogger struct {
	logs   []AuditLog
	mu     sync.RWMutex
	maxLogs int
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(maxLogs int) *AuditLogger {
	return &AuditLogger{
		logs:    make([]AuditLog, 0, maxLogs),
		maxLogs: maxLogs,
	}
}

// Log adds an audit log entry
func (al *AuditLogger) Log(log AuditLog) {
	al.mu.Lock()
	defer al.mu.Unlock()

	log.Timestamp = time.Now()

	// Add to logs
	al.logs = append(al.logs, log)

	// Keep only recent logs (ring buffer)
	if len(al.logs) > al.maxLogs {
		al.logs = al.logs[len(al.logs)-al.maxLogs:]
	}
}

// GetLogs returns recent audit logs
func (al *AuditLogger) GetLogs(limit int) []AuditLog {
	al.mu.RLock()
	defer al.mu.RUnlock()

	if limit <= 0 || limit > len(al.logs) {
		limit = len(al.logs)
	}

	// Return most recent logs
	start := len(al.logs) - limit
	result := make([]AuditLog, limit)
	copy(result, al.logs[start:])

	return result
}

// GetLogsByUDF returns logs for a specific UDF
func (al *AuditLogger) GetLogsByUDF(udfName string, limit int) []AuditLog {
	al.mu.RLock()
	defer al.mu.RUnlock()

	filtered := make([]AuditLog, 0)
	for i := len(al.logs) - 1; i >= 0 && len(filtered) < limit; i-- {
		if al.logs[i].UDFName == udfName {
			filtered = append(filtered, al.logs[i])
		}
	}

	return filtered
}

// Clear removes all logs
func (al *AuditLogger) Clear() {
	al.mu.Lock()
	defer al.mu.Unlock()

	al.logs = make([]AuditLog, 0, al.maxLogs)
}
