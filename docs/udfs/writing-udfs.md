# Writing User-Defined Functions (UDFs) for Quidditch

Complete guide to creating custom WASM-based filtering functions for the Quidditch search engine.

## Table of Contents

- [Introduction](#introduction)
- [Quick Start](#quick-start)
- [UDF Basics](#udf-basics)
- [Development Workflow](#development-workflow)
- [Host Functions](#host-functions)
- [Language-Specific Guides](#language-specific-guides)
- [Testing UDFs](#testing-udfs)
- [Deployment](#deployment)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Introduction

### What are UDFs?

User-Defined Functions (UDFs) are custom filtering functions written in any language that compiles to WebAssembly (WASM). They allow you to implement complex business logic that goes beyond standard query DSL capabilities.

### Why UDFs?

**Flexibility**: Implement any filtering logic you need
**Performance**: Compiled WASM runs at near-native speed
**Safety**: WASM sandbox prevents malicious code
**Portability**: Write once in any WASM-compatible language
**Composability**: Combine with standard queries

### Use Cases

- **Fuzzy Matching**: Typo tolerance, approximate string matching
- **Geo Filtering**: Complex location-based rules
- **Custom Scoring**: Business-specific relevance algorithms
- **Data Validation**: Complex field validation rules
- **ML Inference**: Run models for result filtering
- **Access Control**: Document-level permissions

---

## Quick Start

### 1. Choose Your Language

```bash
# Rust (recommended for production)
rustup target add wasm32-unknown-unknown

# C/C++
sudo apt-get install clang

# WAT (for learning)
sudo apt-get install wabt
```

### 2. Write Your UDF

**Minimal Example (Rust)**:

```rust
// Filter documents where price < 100
#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    unsafe {
        let mut price: f64 = 0.0;
        if get_field_f64(ctx_id, "price".as_ptr(), 5, &mut price) != 0 {
            return 0; // Field missing
        }
        if price < 100.0 { 1 } else { 0 }
    }
}

extern "C" {
    fn get_field_f64(ctx_id: i64, field: *const u8, len: i32, out: *mut f64) -> i32;
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! { loop {} }
```

### 3. Compile to WASM

```bash
cargo build --target wasm32-unknown-unknown --release
```

### 4. Register with Quidditch

```bash
curl -X POST http://localhost:8080/api/v1/udfs \
  -F 'name=price_filter' \
  -F 'version=1.0.0' \
  -F 'wasm=@target/wasm32-unknown-unknown/release/price_filter.wasm'
```

### 5. Use in Query

```json
{
  "query": {
    "bool": {
      "must": [{"term": {"category": "electronics"}}],
      "filter": [
        {
          "wasm_udf": {
            "name": "price_filter",
            "version": "1.0.0"
          }
        }
      ]
    }
  }
}
```

---

## UDF Basics

### Function Signature

Every UDF must export a `filter` function:

```wasm
(func (export "filter") (param i64) (result i32))
```

- **Input**: Document context ID (i64)
- **Output**: 1 (include) or 0 (exclude)

### Memory Requirements

UDFs must export memory:

```wasm
(memory (export "memory") 1)
```

Or in Rust:
```rust
// Memory is automatically exported
```

### Execution Model

```
┌─────────────────────────────────────────────────┐
│  1. Query arrives with wasm_udf clause          │
│  2. For each document in results:               │
│     a. Create document context                  │
│     b. Call filter(context_id)                  │
│     c. Include if returns 1, exclude if 0       │
│  3. Return filtered results                     │
└─────────────────────────────────────────────────┘
```

### State and Context

- **No Global State**: Each invocation is independent
- **No Persistence**: Can't store data between calls
- **Single Document**: Only current document is accessible
- **Read-Only**: Cannot modify document fields

---

## Development Workflow

### Step 1: Define Requirements

```markdown
# UDF: Price Range Filter
## Purpose
Filter products by price range with category-specific thresholds

## Inputs
- Query parameters: min_price, max_price, category_multiplier
- Document fields: price, category

## Logic
1. Get document price
2. Get category multiplier (default 1.0)
3. Adjust max price: max_price * multiplier
4. Return true if min_price <= price <= adjusted_max

## Test Cases
- Price = 50, range [40, 60]: PASS
- Price = 70, range [40, 60]: FAIL
- Price = 55, range [40, 60], multiplier 1.5: PASS
```

### Step 2: Write Code

```rust
use core::slice;

extern "C" {
    fn get_field_f64(ctx_id: i64, field: *const u8, len: i32, out: *mut f64) -> i32;
    fn get_field_string(ctx_id: i64, field: *const u8, len: i32,
                        out: *mut u8, out_len: *mut i32) -> i32;
    fn get_param_f64(name: *const u8, len: i32, out: *mut f64) -> i32;
}

#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    unsafe {
        // Get price
        let mut price: f64 = 0.0;
        if get_field_f64(ctx_id, "price".as_ptr(), 5, &mut price) != 0 {
            return 0;
        }

        // Get parameters
        let mut min_price: f64 = 0.0;
        let mut max_price: f64 = f64::MAX;
        get_param_f64("min_price".as_ptr(), 9, &mut min_price);
        get_param_f64("max_price".as_ptr(), 9, &mut max_price);

        // Get category multiplier
        let mut multiplier: f64 = 1.0;
        get_param_f64("category_multiplier".as_ptr(), 19, &mut multiplier);

        // Apply logic
        let adjusted_max = max_price * multiplier;
        if price >= min_price && price <= adjusted_max {
            1
        } else {
            0
        }
    }
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! { loop {} }
```

### Step 3: Build and Test Locally

```rust
// tests/lib_test.rs
#[test]
fn test_price_range() {
    // Test the levenshtein distance function directly
    assert_eq!(price_in_range(50.0, 40.0, 60.0, 1.0), true);
    assert_eq!(price_in_range(70.0, 40.0, 60.0, 1.0), false);
}
```

```bash
# Run unit tests
cargo test

# Build WASM
cargo build --target wasm32-unknown-unknown --release

# Check size
ls -lh target/wasm32-unknown-unknown/release/*.wasm
```

### Step 4: Integration Test

```go
func TestPriceRangeUDF(t *testing.T) {
    wasmBytes, _ := os.ReadFile("price_range.wasm")

    runtime, _ := wasm.NewRuntime(&wasm.Config{EnableJIT: true})
    defer runtime.Close()

    registry, _ := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
        Runtime: runtime,
    })

    registry.Register(&wasm.UDFMetadata{
        Name:         "price_range",
        Version:      "1.0.0",
        FunctionName: "filter",
        WASMBytes:    wasmBytes,
        Parameters: []wasm.UDFParameter{
            {Name: "min_price", Type: wasm.ValueTypeF64},
            {Name: "max_price", Type: wasm.ValueTypeF64},
        },
        Returns: []wasm.UDFReturnType{
            {Type: wasm.ValueTypeI32},
        },
    })

    // Test with document
    docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
        "price": 55.0,
    })

    params := map[string]wasm.Value{
        "min_price": wasm.NewF64Value(40.0),
        "max_price": wasm.NewF64Value(60.0),
    }

    results, err := registry.Call(context.Background(), "price_range", "1.0.0", docCtx, params)
    require.NoError(t, err)

    result, _ := results[0].AsInt32()
    assert.Equal(t, int32(1), result)
}
```

### Step 5: Deploy

```bash
# Upload to Quidditch
curl -X POST http://localhost:8080/api/v1/udfs \
  -F 'name=price_range' \
  -F 'version=1.0.0' \
  -F 'function_name=filter' \
  -F 'wasm=@price_range.wasm' \
  -F 'metadata={
    "description": "Filter by price range",
    "parameters": [
      {"name": "min_price", "type": "f64"},
      {"name": "max_price", "type": "f64"}
    ]
  }'

# Verify registration
curl http://localhost:8080/api/v1/udfs/price_range/1.0.0

# Test with query
curl -X POST http://localhost:8080/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "index": "products",
    "query": {
      "wasm_udf": {
        "name": "price_range",
        "version": "1.0.0",
        "parameters": {
          "min_price": 40,
          "max_price": 60
        }
      }
    }
  }'
```

---

## Host Functions

### Field Access

#### has_field
```c
int has_field(i64 ctx_id, char* field_name, int name_len)
```
Check if document has a field.
- Returns: 1 if exists, 0 if not

**Example**:
```rust
let has_price = has_field(ctx_id, "price".as_ptr(), 5);
if has_price == 0 {
    return 0; // Field doesn't exist
}
```

#### get_field_string
```c
int get_field_string(i64 ctx_id, char* field_name, int name_len,
                     char* out_buffer, int* out_len)
```
Get string field value.
- Returns: 0 on success, -1 on error
- out_len: Input = buffer size, Output = actual string length

**Example**:
```rust
let mut buffer = [0u8; 256];
let mut len = 256;
if get_field_string(ctx_id, "name".as_ptr(), 4,
                    buffer.as_mut_ptr(), &mut len) == 0 {
    let name = core::str::from_utf8(&buffer[..len as usize]).unwrap();
    // Use name
}
```

#### get_field_i64
```c
int get_field_i64(i64 ctx_id, char* field_name, int name_len, i64* out)
```
Get integer field value.

**Example**:
```rust
let mut count: i64 = 0;
if get_field_i64(ctx_id, "count".as_ptr(), 5, &mut count) == 0 {
    // Use count
}
```

#### get_field_f64
```c
int get_field_f64(i64 ctx_id, char* field_name, int name_len, double* out)
```
Get floating-point field value.

#### get_field_bool
```c
int get_field_bool(i64 ctx_id, char* field_name, int name_len, int* out)
```
Get boolean field value (0 or 1).

### Parameter Access

#### get_param_string
```c
int get_param_string(char* param_name, int name_len,
                     char* out_buffer, int* out_len)
```
Get query parameter as string.

#### get_param_i64
```c
int get_param_i64(char* param_name, int name_len, i64* out)
```
Get query parameter as integer.

#### get_param_f64
```c
int get_param_f64(char* param_name, int name_len, double* out)
```
Get query parameter as float.

#### get_param_bool
```c
int get_param_bool(char* param_name, int name_len, int* out)
```
Get query parameter as boolean.

### Utilities

#### log
```c
void log(int level, char* message, int msg_len)
```
Log message for debugging.
- Levels: 0=DEBUG, 1=INFO, 2=WARN, 3=ERROR

**Example**:
```rust
fn debug_log(msg: &str) {
    unsafe {
        log(0, msg.as_ptr(), msg.len() as i32);
    }
}
```

---

## Language-Specific Guides

### Rust

**Advantages**: Best tooling, memory safety, great performance, small binaries

**Setup**:
```bash
rustup target add wasm32-unknown-unknown
```

**Project Structure**:
```toml
# Cargo.toml
[package]
name = "my-udf"
version = "1.0.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[profile.release]
opt-level = "z"
lto = true
strip = true
panic = "abort"
```

**Template**:
```rust
#![no_std]

extern "C" {
    fn has_field(ctx_id: i64, field: *const u8, len: i32) -> i32;
    fn get_field_string(ctx_id: i64, field: *const u8, len: i32,
                        out: *mut u8, out_len: *mut i32) -> i32;
}

#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    // Your logic here
    1
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    loop {}
}
```

**Build**:
```bash
cargo build --target wasm32-unknown-unknown --release
wasm-opt -Oz target/wasm32-unknown-unknown/release/my_udf.wasm -o my_udf.wasm
```

### C/C++

**Advantages**: Smallest binaries, direct control, familiar to many

**Template**:
```c
#include <stdint.h>

__attribute__((import_module("env"), import_name("has_field")))
int has_field(int64_t ctx_id, const char* field, int len);

__attribute__((import_module("env"), import_name("get_field_f64")))
int get_field_f64(int64_t ctx_id, const char* field, int len, double* out);

__attribute__((export_name("filter")))
int filter(int64_t ctx_id) {
    double price;
    if (get_field_f64(ctx_id, "price", 5, &price) != 0) {
        return 0;
    }
    return (price < 100.0) ? 1 : 0;
}

__attribute__((export_name("memory")))
unsigned char __heap_base;
```

**Build**:
```bash
clang --target=wasm32 -nostdlib -Wl,--no-entry -Wl,--export-dynamic \
  -Wl,--allow-undefined -O3 -o my_udf.wasm my_udf.c
```

### WebAssembly Text (WAT)

**Advantages**: Direct WASM control, educational, very small

**Template**:
```wasm
(module
  (import "env" "get_field_f64"
    (func $get_field_f64 (param i64 i32 i32 i32) (result i32)))

  (memory (export "memory") 1)
  (data (i32.const 0) "price")

  (func (export "filter") (param $ctx_id i64) (result i32)
    (local $price_out i32)
    (local $result i32)

    ;; Allocate space for output
    (local.set $price_out (i32.const 16))

    ;; Get price field
    (local.set $result
      (call $get_field_f64
        (local.get $ctx_id)
        (i32.const 0)      ;; "price" pointer
        (i32.const 5)      ;; length
        (local.get $price_out)))

    ;; Check if successful
    (if (i32.ne (local.get $result) (i32.const 0))
      (then (return (i32.const 0))))

    ;; Compare price < 100.0
    (if (f64.lt (f64.load (local.get $price_out)) (f64.const 100.0))
      (then (return (i32.const 1)))
      (else (return (i32.const 0))))
  )
)
```

**Build**:
```bash
wat2wasm my_udf.wat -o my_udf.wasm
```

### Go (TinyGo)

**Advantages**: Familiar syntax, good tooling, easier than Rust/C

**Setup**:
```bash
wget https://github.com/tinygo-org/tinygo/releases/download/v0.31.0/tinygo_0.31.0_amd64.deb
sudo dpkg -i tinygo_0.31.0_amd64.deb
```

**Template**:
```go
package main

//export filter
func filter(ctxID int64) int32 {
    price := getFieldF64(ctxID, "price")
    if price < 0 {
        return 0 // Field missing
    }
    if price < 100.0 {
        return 1
    }
    return 0
}

//go:wasm-module env
//export get_field_f64
func getFieldF64(ctxID int64, field string) float64

func main() {}
```

**Build**:
```bash
tinygo build -o my_udf.wasm -target=wasi my_udf.go
```

---

## Testing UDFs

### Unit Testing

Test pure logic independently:

```rust
// In your UDF code
pub fn calculate_distance(s1: &str, s2: &str) -> usize {
    // Levenshtein distance logic
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_distance() {
        assert_eq!(calculate_distance("cat", "cat"), 0);
        assert_eq!(calculate_distance("cat", "hat"), 1);
        assert_eq!(calculate_distance("kitten", "sitting"), 3);
    }
}
```

### Integration Testing

Test with WASM runtime:

```go
func TestMyUDF(t *testing.T) {
    wasmBytes, err := os.ReadFile("my_udf.wasm")
    require.NoError(t, err)

    logger := zap.NewNop()
    runtime, err := wasm.NewRuntime(&wasm.Config{
        EnableJIT: true,
        Logger:    logger,
    })
    require.NoError(t, err)
    defer runtime.Close()

    registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
        Runtime: runtime,
        Logger:  logger,
    })
    require.NoError(t, err)

    err = registry.Register(&wasm.UDFMetadata{
        Name:         "my_udf",
        Version:      "1.0.0",
        FunctionName: "filter",
        WASMBytes:    wasmBytes,
        Parameters:   []wasm.UDFParameter{},
        Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
    })
    require.NoError(t, err)

    tests := []struct {
        name       string
        doc        map[string]interface{}
        params     map[string]wasm.Value
        wantResult int32
    }{
        {
            name:       "price below threshold",
            doc:        map[string]interface{}{"price": 50.0},
            params:     nil,
            wantResult: 1,
        },
        {
            name:       "price above threshold",
            doc:        map[string]interface{}{"price": 150.0},
            params:     nil,
            wantResult: 0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, tt.doc)

            results, err := registry.Call(
                context.Background(),
                "my_udf",
                "1.0.0",
                docCtx,
                tt.params,
            )
            require.NoError(t, err)
            require.Len(t, results, 1)

            result, err := results[0].AsInt32()
            require.NoError(t, err)
            assert.Equal(t, tt.wantResult, result)
        })
    }
}
```

### Performance Testing

Benchmark UDF execution:

```go
func BenchmarkMyUDF(b *testing.B) {
    // Setup (same as integration test)
    wasmBytes, _ := os.ReadFile("my_udf.wasm")
    runtime, _ := wasm.NewRuntime(&wasm.Config{EnableJIT: true})
    defer runtime.Close()

    registry, _ := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
        Runtime: runtime,
    })

    registry.Register(&wasm.UDFMetadata{
        Name:         "my_udf",
        Version:      "1.0.0",
        FunctionName: "filter",
        WASMBytes:    wasmBytes,
        Parameters:   []wasm.UDFParameter{},
        Returns:      []wasm.UDFReturnType{{Type: wasm.ValueTypeI32}},
    })

    docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
        "price": 75.0,
    })

    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        registry.Call(ctx, "my_udf", "1.0.0", docCtx, nil)
    }
}
```

**Performance Targets**:
- Simple UDFs: <5μs
- Medium complexity: <10μs
- Complex UDFs: <50μs

---

## Deployment

### Versioning

Use semantic versioning:
- `1.0.0` - Initial release
- `1.1.0` - New features (backwards compatible)
- `2.0.0` - Breaking changes

**Multiple versions can coexist**:
```bash
# Register v1.0.0
curl -X POST .../udfs -F 'version=1.0.0' ...

# Register v1.1.0 (new features)
curl -X POST .../udfs -F 'version=1.1.0' ...

# Queries can specify which version to use
{"wasm_udf": {"name": "my_udf", "version": "1.0.0"}}
```

### Rollout Strategy

**1. Canary Deployment**:
```bash
# Deploy new version
register_udf("my_udf", "2.0.0")

# Test with small percentage
if random() < 0.01:
    version = "2.0.0"
else:
    version = "1.0.0"

# Monitor errors and performance
# If good, gradually increase percentage
```

**2. Blue-Green Deployment**:
```bash
# Current: v1.0.0 (blue)
# Deploy: v2.0.0 (green)

# Switch all traffic
update_default_version("my_udf", "2.0.0")

# If issues, instant rollback
update_default_version("my_udf", "1.0.0")
```

### Monitoring

Track UDF performance:

```bash
# Get statistics
curl http://localhost:8080/api/v1/udfs/my_udf/1.0.0/stats

# Response
{
  "name": "my_udf",
  "version": "1.0.0",
  "call_count": 1000000,
  "error_count": 42,
  "avg_latency_us": 3.8,
  "p50_latency_us": 3.2,
  "p99_latency_us": 12.5,
  "p999_latency_us": 45.0,
  "total_execution_time_ms": 3800
}
```

**Alert on**:
- Error rate > 1%
- P99 latency > 50μs
- Memory usage > 1MB

---

## Best Practices

### Performance

**1. Early Exit**
```rust
// Good
if !has_field(ctx_id, "price") {
    return 0; // Exit immediately
}

// Bad
let has = has_field(ctx_id, "price");
let price = get_field_f64(...);
if !has { return 0; }
```

**2. Minimize Host Calls**
```rust
// Good: Cache field value
let price = get_field_f64(...);
let in_range = price >= min && price <= max;

// Bad: Multiple host calls
if get_field_f64(...) >= min && get_field_f64(...) <= max { }
```

**3. Use Stack Allocation**
```rust
// Good
let mut buffer = [0u8; 256];

// Bad
let buffer = vec![0u8; 256]; // heap allocation
```

### Size Optimization

**1. Enable Optimization**
```toml
[profile.release]
opt-level = "z"     # Optimize for size
lto = true          # Link-time optimization
strip = true        # Strip symbols
codegen-units = 1   # Better optimization
```

**2. Use wasm-opt**
```bash
wasm-opt -Oz input.wasm -o output.wasm
```

**3. Avoid Standard Library**
```rust
#![no_std]  // Don't link std

// Use core instead
use core::slice;
use core::str;
```

### Error Handling

**1. Graceful Degradation**
```rust
// Return 0 on error, don't panic
let value = match get_field(ctx_id, "field") {
    Ok(v) => v,
    Err(_) => return 0,
};
```

**2. Validate Inputs**
```rust
// Check field exists before reading
if has_field(ctx_id, "price") == 0 {
    return 0;
}
```

**3. Log Errors**
```rust
if let Err(e) = operation() {
    log_error(&format!("Operation failed: {:?}", e));
    return 0;
}
```

### Security

**1. Bounds Checking**
```rust
// Always check buffer sizes
if len > buffer.len() {
    return 0;
}
```

**2. Input Validation**
```rust
// Validate coordinates
if lat < -90.0 || lat > 90.0 {
    return 0;
}
```

**3. Resource Limits**
```rust
// Limit string lengths
const MAX_STRING_LEN: usize = 1024;
if field_len > MAX_STRING_LEN {
    return 0;
}
```

---

## Troubleshooting

### Compilation Errors

**"undefined reference to `__linear_memory`"**
- Cause: Missing memory export
- Fix: Add `(memory (export "memory") 1)` or ensure cdylib exports memory

**"function signature mismatch"**
- Cause: Wrong parameter types in extern declaration
- Fix: Match exact signature: `(param i64) (result i32)`

### Runtime Errors

**"failed to compile module"**
- Cause: Invalid WASM binary
- Fix: Validate with `wasm-validate my_udf.wasm`

**"failed to get instance from pool"**
- Cause: All instances in use
- Fix: Increase pool size or optimize UDF performance

**"UDF execution timeout"**
- Cause: Infinite loop or very slow logic
- Fix: Add early exits, optimize algorithm

### Performance Issues

**High latency (>50μs)**
- Profile with benchmarks
- Check for unnecessary host calls
- Optimize algorithm complexity

**High memory usage**
- Reduce buffer sizes
- Avoid heap allocations
- Use static buffers

**High error rate**
- Add field existence checks
- Validate all inputs
- Handle edge cases

### Debugging

**Add Logging**:
```rust
fn debug(msg: &str) {
    unsafe {
        log(0, msg.as_ptr(), msg.len() as i32);
    }
}

#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    debug("Starting filter");
    // ...
    debug("Filter complete");
    1
}
```

**Test Locally**:
```bash
# Run with test runtime
RUST_LOG=debug cargo test -- --nocapture
```

**Inspect WASM**:
```bash
# View exports
wasm-objdump -x my_udf.wasm

# View imports
wasm-objdump -j import my_udf.wasm

# View functions
wasm-objdump -d my_udf.wasm
```

---

## Examples

See complete examples in `examples/udfs/`:

- **string-distance** (Rust): Fuzzy matching
- **geo-filter** (C): Location-based filtering
- **custom-score** (WAT): Custom scoring logic

---

## Resources

- [Quidditch UDF API Reference](./api-reference.md)
- [Performance Guide](./performance-guide.md)
- [Migration Guide](./migration-guide.md)
- [WebAssembly Spec](https://webassembly.org/specs/)
- [Rust WASM Book](https://rustwasm.github.io/docs/book/)
- [wazero Documentation](https://wazero.io/)

---

## Getting Help

- GitHub Issues: https://github.com/quidditch/quidditch/issues
- Discord: https://discord.gg/quidditch
- Email: support@quidditch.dev

---

**Happy UDF writing!** 🚀
