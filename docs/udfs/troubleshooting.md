# UDF Troubleshooting Guide

Common issues, solutions, and debugging techniques for Quidditch UDFs.

## Table of Contents

- [Quick Diagnostics](#quick-diagnostics)
- [Compilation Issues](#compilation-issues)
- [Registration Issues](#registration-issues)
- [Runtime Errors](#runtime-errors)
- [Performance Problems](#performance-problems)
- [Debugging Techniques](#debugging-techniques)
- [Common Error Messages](#common-error-messages)

---

## Quick Diagnostics

### UDF Not Working Checklist

```
[ ] WASM binary compiled successfully?
[ ] Binary size reasonable (<100KB)?
[ ] UDF registered without errors?
[ ] Correct function name exported ("filter")?
[ ] Function signature correct (i64 -> i32)?
[ ] All required parameters provided?
[ ] Field names match document schema?
[ ] Field types match host functions?
[ ] Error logs checked?
```

### Quick Test Commands

```bash
# Check if UDF is registered
curl http://localhost:8080/api/v1/udfs/my_udf/1.0.0

# View UDF statistics
curl http://localhost:8080/api/v1/udfs/my_udf/1.0.0/stats

# Check recent errors (if logging enabled)
grep "UDF error" /var/log/quidditch/server.log | tail -20

# Validate WASM binary
wasm-validate my_udf.wasm

# Inspect WASM exports
wasm-objdump -x my_udf.wasm | grep export
```

---

## Compilation Issues

### Issue 1: Invalid Magic Number

**Error**:
```
failed to compile module: invalid magic number
```

**Cause**: Not a valid WASM binary file

**Solutions**:

1. **Check file format**:
```bash
# Should show: WebAssembly (wasm) binary module
file my_udf.wasm

# First 4 bytes should be: 00 61 73 6d
hexdump -C my_udf.wasm | head -1
```

2. **Rebuild from source**:
```bash
# Rust
cargo clean
cargo build --release --target wasm32-unknown-unknown

# C
clang --target=wasm32 -nostdlib -Wl,--no-entry \
      -Wl,--export-all -O3 my_udf.c -o my_udf.wasm
```

3. **Convert WAT to WASM**:
```bash
wat2wasm my_udf.wat -o my_udf.wasm
```

### Issue 2: Missing Export

**Error**:
```
function 'filter' not found in module exports
```

**Cause**: Function not exported or wrong name

**Solution** (Rust):
```rust
// Ensure #[no_mangle] and extern "C"
#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    // ...
}
```

**Solution** (C):
```c
// Mark as export
__attribute__((visibility("default")))
int filter(long long ctx_id) {
    // ...
}
```

**Verify exports**:
```bash
wasm-objdump -x my_udf.wasm | grep -A 10 "Export section"
```

Should show:
```
Export[0]:
  - func[0] <filter>
```

### Issue 3: Wrong Function Signature

**Error**:
```
function signature mismatch: expected (i64) -> (i32), got ...
```

**Cause**: Function parameters or return type incorrect

**Correct Signature**:
```rust
// Must take i64 (context ID) and return i32 (0 or 1)
pub extern "C" fn filter(ctx_id: i64) -> i32
```

**Common Mistakes**:
```rust
// Wrong: No parameter
pub extern "C" fn filter() -> i32

// Wrong: Wrong parameter type
pub extern "C" fn filter(ctx_id: i32) -> i32

// Wrong: Wrong return type
pub extern "C" fn filter(ctx_id: i64) -> bool
```

### Issue 4: Rust Compilation Fails

**Error**:
```
error: linking with `rust-lld` failed
```

**Solution**: Ensure wasm target installed:
```bash
rustup target add wasm32-unknown-unknown
rustup update
```

**Error**:
```
error: cannot find macro `println` in this scope
```

**Solution**: Remove std usage or use #![no_std]:
```rust
#![no_std]

// Don't use println!, use log() host function instead
// Don't use String, use &str
// Don't use Vec, use arrays
```

### Issue 5: C Compilation Fails

**Error**:
```
undefined reference to `printf`
```

**Solution**: Remove libc dependencies:
```c
// Don't use printf, sprintf, etc.
// Use log() host function

// Remove: #include <stdio.h>
// Remove: #include <stdlib.h>

// Use only: #include <stdint.h>, <stdbool.h>
```

**Error**:
```
entry symbol not defined
```

**Solution**: Add `-Wl,--no-entry` flag:
```bash
clang --target=wasm32 -nostdlib -Wl,--no-entry \
      -Wl,--export=filter -O3 my_udf.c -o my_udf.wasm
```

---

## Registration Issues

### Issue 1: UDF Already Exists

**Error**:
```
HTTP 409: UDF 'my_udf' version '1.0.0' already exists
```

**Solutions**:

1. **Use new version**:
```bash
curl -X POST http://localhost:8080/api/v1/udfs \
  -F 'name=my_udf' \
  -F 'version=1.0.1' \  # Increment version
  -F 'wasm=@my_udf.wasm'
```

2. **Delete old version first**:
```bash
curl -X DELETE http://localhost:8080/api/v1/udfs/my_udf/1.0.0
# Then re-register
```

3. **Use different name for testing**:
```bash
curl -X POST http://localhost:8080/api/v1/udfs \
  -F 'name=my_udf_test' \
  -F 'version=1.0.0' \
  -F 'wasm=@my_udf.wasm'
```

### Issue 2: Binary Too Large

**Error**:
```
HTTP 413: WASM binary too large (max 10MB)
```

**Solutions**:

1. **Optimize with wasm-opt**:
```bash
wasm-opt -Oz input.wasm -o output.wasm
ls -lh output.wasm  # Check new size
```

2. **Strip debug symbols** (Rust):
```toml
[profile.release]
strip = true
```

3. **Remove unused dependencies** (Rust):
```bash
# Check what's included
cargo tree
cargo bloat --release --target wasm32-unknown-unknown
```

4. **Use #![no_std]** to remove standard library

### Issue 3: Invalid Metadata

**Error**:
```
HTTP 400: invalid metadata: parameters[0].type: invalid value
```

**Solution**: Check parameter types:
```json
{
  "parameters": [
    {"name": "field", "type": "string"},       // ✓ Valid
    {"name": "max_dist", "type": "i64"},       // ✓ Valid
    {"name": "threshold", "type": "f64"},      // ✓ Valid
    {"name": "enabled", "type": "bool"},       // ✓ Valid
    {"name": "value", "type": "number"}        // ✗ Invalid (use i64/f64)
  ]
}
```

**Valid types**: `string`, `i32`, `i64`, `f32`, `f64`, `bool`

---

## Runtime Errors

### Issue 1: All Documents Filtered Out

**Symptom**: UDF returns 0 for all documents

**Causes**:

1. **Field name mismatch**:
```rust
// Wrong field name
get_field_f64(ctx_id, "price")  // Field is "Price" (capital P)

// Solution: Check document schema
get_field_f64(ctx_id, "Price")
```

2. **Field type mismatch**:
```rust
// Field "price" is f64 but calling wrong function
get_field_i64(ctx_id, "price")  // Returns None/error

// Solution: Use correct type
get_field_f64(ctx_id, "price")
```

3. **Missing parameter**:
```rust
let threshold = get_f64_param("min_price")?;
// If "min_price" not in query, returns None
// Then ? operator returns early with 0

// Solution: Provide default
let threshold = get_f64_param("min_price").unwrap_or(0.0);
```

4. **Logic error**:
```rust
// Wrong comparison
if price > max_price {  // Should be <
    return 1;
}

// Solution: Fix logic
if price < max_price {
    return 1;
}
```

**Debug**: Add logging:
```rust
use core::fmt::Write;

let mut msg = heapless::String::<64>::new();
write!(&mut msg, "price={}, threshold={}", price, threshold);
log(1, msg.as_bytes().as_ptr(), msg.len() as i32);
```

### Issue 2: Intermittent Failures

**Symptom**: UDF works sometimes, fails other times

**Causes**:

1. **Race condition** (fixed in Week 4 Day 2):
```go
// Old code (not thread-safe)
contexts := make(map[uint64]*DocumentContext)

// Fixed code
type HostFunctions struct {
    mu       sync.RWMutex
    contexts map[uint64]*DocumentContext
}
```

2. **Memory corruption**:
```rust
// Wrong: Buffer too small
let mut buffer = [0u8; 10];
get_field_string(ctx_id, "name", &mut buffer);  // Name might be >10 chars

// Solution: Use adequate buffer size
let mut buffer = [0u8; 256];
```

3. **Uninitialized memory**:
```c
// Wrong: Uninitialized
double price;
get_field_f64(ctx_id, "price", 5, &price);
// If error, price is uninitialized!

// Solution: Initialize
double price = 0.0;
if (get_field_f64(ctx_id, "price", 5, &price) != 0) {
    return 0;  // Handle error
}
```

### Issue 3: Panic/Crash

**Symptom**: UDF execution crashes entire query

**Causes**:

1. **Panic in Rust**:
```rust
// Don't use unwrap() in production
let price = get_field_f64(ctx_id, "price").unwrap();  // Panics if None

// Use ? or match instead
let price = get_field_f64(ctx_id, "price")?;
// Or
let price = match get_field_f64(ctx_id, "price") {
    Some(v) => v,
    None => return 0,
};
```

2. **Buffer overflow** (C):
```c
// Wrong: No bounds checking
char buffer[10];
int len = 10;
get_field_string(ctx_id, "name", 4, buffer, &len);
// If name is >10 chars, overflow!

// Solution: Check return value
if (get_field_string(ctx_id, "name", 4, buffer, &len) != 0) {
    return 0;  // Buffer too small or error
}
```

3. **Integer overflow**:
```rust
// Might overflow
let total = price * quantity;

// Use checked arithmetic
let total = price.checked_mul(quantity).unwrap_or(0);
```

### Issue 4: Wrong Results

**Symptom**: UDF includes/excludes wrong documents

**Debug Process**:

1. **Add logging**:
```rust
fn filter(ctx_id: i64) -> i32 {
    let price = get_field_f64(ctx_id, "price").unwrap_or(-1.0);
    let threshold = get_f64_param("threshold").unwrap_or(-1.0);

    let result = if price > threshold { 1 } else { 0 };

    // Log decision
    log(1,
        format!("price={:.2}, threshold={:.2}, result={}",
                price, threshold, result).as_ptr(),
        ...);

    result
}
```

2. **Test with known documents**:
```go
// Create controlled test case
doc := map[string]interface{}{
    "price": 150.0,
}
params := map[string]wasm.Value{
    "threshold": wasm.NewF64Value(100.0),
}
// Should return 1
```

3. **Check parameter passing**:
```bash
# Verify query syntax
{
  "wasm_udf": {
    "name": "price_filter",
    "version": "1.0.0",
    "parameters": {
      "threshold": 100.0  # ✓ Correct
      // Not: "threshold": "100"  # ✗ Wrong type
    }
  }
}
```

---

## Performance Problems

### Issue 1: UDF Too Slow

**Symptom**: Query latency increased significantly

**Diagnosis**:
```bash
# Check UDF statistics
curl http://localhost:8080/api/v1/udfs/my_udf/1.0.0/stats

# Look for:
# - avg_latency_us > 50
# - p99_latency_us > 100
```

**Solutions**:

1. **Enable JIT compilation**:
```go
runtime, err := wasm.NewRuntime(&wasm.Config{
    EnableJIT: true,  // ← Enable this
})
```

2. **Reduce host function calls**:
```rust
// Bad: Check then get (2 calls)
if has_field(ctx_id, "price") == 1 {
    let price = get_field_f64(ctx_id, "price").unwrap();
}

// Good: Try to get (1 call)
if let Some(price) = get_field_f64(ctx_id, "price") {
    // Use price
}
```

3. **Use early exit**:
```rust
// Get cheapest fields first
let in_stock = get_bool_field(ctx_id, "in_stock")?;
if !in_stock {
    return 0;  // Exit early, don't fetch other fields
}

// Only fetch expensive fields if needed
let description = get_string_field(ctx_id, "description")?;
```

4. **Optimize algorithm** (see [Performance Guide](./performance-guide.md))

### Issue 2: High Memory Usage

**Symptom**: Memory usage growing over time

**Causes**:

1. **Memory leaks** (shouldn't happen with WASM sandbox)
2. **Module instance pool too large**:
```go
registry, _ := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
    DefaultPoolSize: 100,  // Too many instances
})

// Reduce pool size
DefaultPoolSize: 10,  // Usually sufficient
```

3. **Large static buffers**:
```rust
// Bad: Wastes memory
static mut BUFFER: [u8; 1048576] = [0; 1048576];  // 1MB

// Good: Use reasonable size
static mut BUFFER: [u8; 4096] = [0; 4096];  // 4KB
```

### Issue 3: Compilation Slow

**Symptom**: First query with UDF is very slow

**Cause**: Module compilation happens on first use

**Solutions**:

1. **Pre-warm UDFs**:
```go
// After registration, call once to trigger compilation
docCtx := wasm.NewDocumentContextFromMap("warmup", 1.0, map[string]interface{}{})
registry.Call(ctx, "my_udf", "1.0.0", docCtx, nil)
```

2. **Optimize binary size** (smaller = faster compile):
```bash
wasm-opt -Oz input.wasm -o output.wasm
```

3. **Increase pool size** (compile once, reuse many times):
```go
DefaultPoolSize: 20,  // More pre-compiled instances
```

---

## Debugging Techniques

### Technique 1: Use Logging

```rust
extern "C" {
    fn log(level: i32, msg_ptr: *const u8, msg_len: i32);
}

fn debug_log(msg: &str) {
    unsafe {
        log(0, msg.as_ptr(), msg.len() as i32);  // level 0 = DEBUG
    }
}

pub extern "C" fn filter(ctx_id: i64) -> i32 {
    debug_log("Starting filter");

    let price = match get_field_f64(ctx_id, "price") {
        Some(v) => {
            // Format is tricky without std, but you can log values
            debug_log("Got price field");
            v
        }
        None => {
            debug_log("No price field");
            return 0;
        }
    };

    debug_log("Filter complete");
    1
}
```

### Technique 2: Test Outside Query Engine

```go
// Create standalone test
func TestMyUDFDirectly(t *testing.T) {
    wasmBytes, _ := os.ReadFile("my_udf.wasm")
    runtime, _ := wasm.NewRuntime(&wasm.Config{
        EnableJIT:   true,
        EnableDebug: true,  // More logging
    })
    defer runtime.Close()

    registry, _ := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
        Runtime: runtime,
    })

    registry.Register(&wasm.UDFMetadata{
        Name:         "test_udf",
        Version:      "1.0.0",
        FunctionName: "filter",
        WASMBytes:    wasmBytes,
    })

    // Test with known data
    doc := map[string]interface{}{
        "price": 150.0,
        "name":  "Test Product",
    }

    docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, doc)

    params := map[string]wasm.Value{
        "threshold": wasm.NewF64Value(100.0),
    }

    results, err := registry.Call(
        context.Background(),
        "test_udf",
        "1.0.0",
        docCtx,
        params,
    )

    require.NoError(t, err)
    result, _ := results[0].AsInt32()

    t.Logf("Result: %d (expected 1)", result)
    assert.Equal(t, int32(1), result)
}
```

### Technique 3: Inspect WASM Binary

```bash
# View all exports
wasm-objdump -x my_udf.wasm | grep -A 20 "Export section"

# View function signatures
wasm-objdump -x my_udf.wasm | grep -A 50 "Type section"

# View imports (host functions)
wasm-objdump -x my_udf.wasm | grep -A 20 "Import section"

# Disassemble function
wasm-objdump -d my_udf.wasm | grep -A 100 "filter"

# View data sections
wasm-objdump -s my_udf.wasm
```

### Technique 4: Binary Search for Bug

If UDF works with simple documents but fails with real data:

1. Start with minimal document:
```go
doc := map[string]interface{}{}  // Empty
```

2. Add fields one by one:
```go
doc := map[string]interface{}{
    "price": 100.0,  // Works?
}

doc := map[string]interface{}{
    "price": 100.0,
    "name": "Test",  // Still works?
}
```

3. Find which field causes issue

### Technique 5: Compare with Reference Implementation

```rust
// Implement same logic in Rust test
#[test]
fn test_filter_logic() {
    let price = 150.0;
    let threshold = 100.0;

    let result = if price > threshold { 1 } else { 0 };

    assert_eq!(result, 1);
}
```

---

## Common Error Messages

### "context ID not found"

**Meaning**: Invalid context ID passed to host function

**Fix**: Don't cache or reuse context IDs; use the provided `ctx_id` parameter

### "invalid UTF-8"

**Meaning**: String field contains invalid UTF-8

**Fix**: Validate string handling:
```rust
match core::str::from_utf8(&buffer[..len]) {
    Ok(s) => s,
    Err(_) => return 0,  // Invalid UTF-8
}
```

### "stack overflow"

**Meaning**: Recursion too deep or large stack allocation

**Fix**:
- Reduce recursion depth
- Use heap allocation for large buffers
- Increase WASM stack size (compile flag)

### "out of bounds memory access"

**Meaning**: Accessing memory outside allocated range

**Fix**:
- Check array bounds
- Validate buffer sizes
- Use safe Rust constructs (no unsafe needed for most UDFs)

### "unreachable executed"

**Meaning**: Hit unreachable instruction (panic in Rust)

**Fix**: Remove panics:
```rust
// Don't use: unwrap(), expect(), panic!(), unreachable!()
// Use: ?, match, unwrap_or(), unwrap_or_default()
```

---

## Getting Help

### Before Asking for Help

1. Check this troubleshooting guide
2. Review [Writing UDFs Guide](./writing-udfs.md)
3. Check [Examples](../../examples/udfs/)
4. Test with minimal example
5. Collect error logs

### Information to Include

```
1. WASM binary size and source language
2. Complete error message
3. UDF registration command/metadata
4. Sample query using the UDF
5. Sample document that fails
6. Expected vs actual behavior
7. Quidditch version
8. Steps to reproduce
```

### Where to Get Help

- GitHub Issues: https://github.com/quidditch/quidditch/issues
- Documentation: https://github.com/quidditch/quidditch/tree/main/docs
- Examples: https://github.com/quidditch/quidditch/tree/main/examples/udfs

---

## Debugging Checklist

When debugging a UDF issue:

- [ ] WASM binary validates with `wasm-validate`
- [ ] Function exported with correct name
- [ ] Function signature is `(i64) -> (i32)`
- [ ] Compilation used release/optimized mode
- [ ] Host function calls check return values
- [ ] Field names match document schema exactly (case-sensitive)
- [ ] Field types match host function calls
- [ ] All parameters provided in query
- [ ] Parameter types match declarations
- [ ] No panics or unwrap() calls
- [ ] Buffer sizes adequate for data
- [ ] Logic tested with unit tests
- [ ] Integration tests pass
- [ ] Logs checked for errors
- [ ] Statistics show reasonable performance

---

**Still stuck?** Create a minimal reproducible example and open an issue!
