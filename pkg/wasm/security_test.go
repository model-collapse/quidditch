package wasm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultResourceLimits(t *testing.T) {
	limits := DefaultResourceLimits()

	assert.Equal(t, uint32(256), limits.MaxMemoryPages)
	assert.Equal(t, 5*time.Second, limits.MaxExecutionTime)
	assert.Equal(t, 1024, limits.MaxStackDepth)
	assert.Equal(t, 100, limits.MaxInstances)
}

func TestUDFPermissions(t *testing.T) {
	t.Run("DefaultPermissions", func(t *testing.T) {
		perms := DefaultPermissions()

		assert.True(t, perms.Has(PermissionReadDocument))
		assert.True(t, perms.Has(PermissionWriteLog))
		assert.False(t, perms.Has(PermissionNetworkAccess))
	})

	t.Run("AddPermission", func(t *testing.T) {
		perms := NewUDFPermissions()

		assert.False(t, perms.Has(PermissionReadDocument))

		perms.Add(PermissionReadDocument)
		assert.True(t, perms.Has(PermissionReadDocument))

		// Adding again should be idempotent
		perms.Add(PermissionReadDocument)
		assert.True(t, perms.Has(PermissionReadDocument))
		assert.Len(t, perms.Allowed, 1)
	})

	t.Run("RemovePermission", func(t *testing.T) {
		perms := NewUDFPermissions(PermissionReadDocument, PermissionWriteLog)

		assert.True(t, perms.Has(PermissionReadDocument))
		assert.True(t, perms.Has(PermissionWriteLog))

		perms.Remove(PermissionReadDocument)
		assert.False(t, perms.Has(PermissionReadDocument))
		assert.True(t, perms.Has(PermissionWriteLog))
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		perms := DefaultPermissions()

		done := make(chan bool, 100)

		for i := 0; i < 100; i++ {
			go func(n int) {
				if n%2 == 0 {
					perms.Add(PermissionNetworkAccess)
				} else {
					_ = perms.Has(PermissionReadDocument)
				}
				done <- true
			}(i)
		}

		for i := 0; i < 100; i++ {
			<-done
		}

		// Should have network access after concurrent adds
		assert.True(t, perms.Has(PermissionNetworkAccess))
	})
}

func TestSignWASM(t *testing.T) {
	wasmBytes := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	t.Run("SignAndVerify", func(t *testing.T) {
		sig := SignWASM(wasmBytes, "test-signer")

		require.NotNil(t, sig)
		assert.NotEmpty(t, sig.WASMHash)
		assert.NotEmpty(t, sig.Signature)
		assert.Equal(t, "test-signer", sig.Signer)
		assert.False(t, sig.SignedAt.IsZero())

		// Verify signature
		err := VerifyWASM(wasmBytes, sig)
		assert.NoError(t, err)
	})

	t.Run("VerifyModifiedWASM", func(t *testing.T) {
		sig := SignWASM(wasmBytes, "test-signer")

		// Modify the WASM bytes
		modifiedBytes := make([]byte, len(wasmBytes))
		copy(modifiedBytes, wasmBytes)
		modifiedBytes[0] = 0xFF

		// Verification should fail
		err := VerifyWASM(modifiedBytes, sig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hash mismatch")
	})

	t.Run("VerifyNilSignature", func(t *testing.T) {
		err := VerifyWASM(wasmBytes, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature is nil")
	})
}

func TestExecutionLimiter(t *testing.T) {
	t.Run("AcquireAndRelease", func(t *testing.T) {
		limits := &ResourceLimits{
			MaxInstances: 5,
		}
		limiter := NewExecutionLimiter(limits)

		// Acquire instances
		for i := 0; i < 5; i++ {
			err := limiter.AcquireInstance("test_udf")
			assert.NoError(t, err)
		}

		// Should fail to acquire more
		err := limiter.AcquireInstance("test_udf")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "instance limit exceeded")
	})

	t.Run("ExecuteWithTimeout", func(t *testing.T) {
		limits := &ResourceLimits{
			MaxExecutionTime: 100 * time.Millisecond,
		}
		limiter := NewExecutionLimiter(limits)

		// Fast function should succeed
		err := limiter.ExecuteWithLimits(context.Background(), func() error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})
		assert.NoError(t, err)
	})

	t.Run("ExecuteWithTimeoutExceeded", func(t *testing.T) {
		limits := &ResourceLimits{
			MaxExecutionTime: 50 * time.Millisecond,
		}
		limiter := NewExecutionLimiter(limits)

		// Slow function should timeout
		err := limiter.ExecuteWithLimits(context.Background(), func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	t.Run("ExecuteWithError", func(t *testing.T) {
		limits := DefaultResourceLimits()
		limiter := NewExecutionLimiter(limits)

		expectedErr := errors.New("test error")

		err := limiter.ExecuteWithLimits(context.Background(), func() error {
			return expectedErr
		})
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}

func TestAuditLogger(t *testing.T) {
	t.Run("LogAndRetrieve", func(t *testing.T) {
		logger := NewAuditLogger(100)

		log := AuditLog{
			Operation:  "register",
			UDFName:    "test_udf",
			UDFVersion: "1.0.0",
			User:       "test-user",
			Success:    true,
			Duration:   100 * time.Millisecond,
		}

		logger.Log(log)

		logs := logger.GetLogs(10)
		require.Len(t, logs, 1)

		retrieved := logs[0]
		assert.Equal(t, "register", retrieved.Operation)
		assert.Equal(t, "test_udf", retrieved.UDFName)
		assert.Equal(t, "1.0.0", retrieved.UDFVersion)
		assert.False(t, retrieved.Timestamp.IsZero())
	})

	t.Run("RingBuffer", func(t *testing.T) {
		logger := NewAuditLogger(10) // Max 10 logs

		// Add 20 logs
		for i := 0; i < 20; i++ {
			logger.Log(AuditLog{
				Operation:  "call",
				UDFName:    "test_udf",
				UDFVersion: "1.0.0",
			})
		}

		// Should only keep last 10
		logs := logger.GetLogs(100)
		assert.Len(t, logs, 10)
	})

	t.Run("GetLogsByUDF", func(t *testing.T) {
		logger := NewAuditLogger(100)

		// Add logs for different UDFs
		for i := 0; i < 5; i++ {
			logger.Log(AuditLog{
				Operation: "call",
				UDFName:   "udf1",
			})
		}
		for i := 0; i < 3; i++ {
			logger.Log(AuditLog{
				Operation: "call",
				UDFName:   "udf2",
			})
		}

		// Get logs for udf1
		logs := logger.GetLogsByUDF("udf1", 10)
		assert.Len(t, logs, 5)
		for _, log := range logs {
			assert.Equal(t, "udf1", log.UDFName)
		}

		// Get logs for udf2
		logs = logger.GetLogsByUDF("udf2", 10)
		assert.Len(t, logs, 3)
	})

	t.Run("Clear", func(t *testing.T) {
		logger := NewAuditLogger(100)

		// Add some logs
		for i := 0; i < 10; i++ {
			logger.Log(AuditLog{
				Operation: "call",
				UDFName:   "test_udf",
			})
		}

		assert.Len(t, logger.GetLogs(100), 10)

		// Clear
		logger.Clear()
		assert.Len(t, logger.GetLogs(100), 0)
	})

	t.Run("ConcurrentLogging", func(t *testing.T) {
		logger := NewAuditLogger(1000)

		done := make(chan bool, 100)

		for i := 0; i < 100; i++ {
			go func(n int) {
				logger.Log(AuditLog{
					Operation: "call",
					UDFName:   "test_udf",
				})
				done <- true
			}(i)
		}

		for i := 0; i < 100; i++ {
			<-done
		}

		logs := logger.GetLogs(1000)
		assert.Len(t, logs, 100)
	})
}
