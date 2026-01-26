# Quidditch UDF Examples

Collection of example User-Defined Functions (UDFs) for the Quidditch search engine, demonstrating WASM-based custom filtering in multiple programming languages.

## Available Examples

### 1. String Distance (Rust) 🦀

**Path**: `string-distance/`
**Language**: Rust
**Use Case**: Fuzzy string matching, typo tolerance

Implements Levenshtein distance algorithm for approximate string matching. Perfect for handling typos and variations in search queries.

```json
{
  "wasm_udf": {
    "name": "string_distance",
    "version": "1.0.0",
    "parameters": {
      "field": "product_name",
      "target": "iPhone",
      "max_distance": 2
    }
  }
}
```

**Size**: ~2-3 KB (optimized)
**Complexity**: O(m×n) where m, n are string lengths

### 2. Geo Filter (C) 🌍

**Path**: `geo-filter/`
**Language**: C
**Use Case**: Location-based filtering

Filters documents by geographic distance using the Haversine formula. Returns documents within a specified radius of target coordinates.

```json
{
  "wasm_udf": {
    "name": "geo_filter",
    "version": "1.0.0",
    "parameters": {
      "lat_field": "latitude",
      "lon_field": "longitude",
      "target_lat": 37.7749,
      "target_lon": -122.4194,
      "max_distance_km": 10.0
    }
  }
}
```

**Size**: ~1-2 KB
**Complexity**: O(1) - constant time calculation

### 3. Custom Score (WebAssembly Text) 📊

**Path**: `custom-score/`
**Language**: WAT (WebAssembly Text Format)
**Use Case**: Custom scoring logic, relevance tuning

Demonstrates field access and arithmetic operations. Calculates a custom score and filters based on threshold.

```json
{
  "wasm_udf": {
    "name": "custom_score",
    "version": "1.0.0",
    "parameters": {
      "min_score": 0.7
    }
  }
}
```

**Size**: <1 KB
**Complexity**: O(1) - simple arithmetic

## Language Support

| Language | Toolchain | Binary Size | Difficulty | Performance |
|----------|-----------|-------------|------------|-------------|
| **Rust** | rustc + wasm32 target | ~2-5 KB | Medium | Excellent |
| **C/C++** | clang --target=wasm32 | ~1-3 KB | Low | Excellent |
| **WAT** | wat2wasm (WABT) | <1 KB | Low | Excellent |
| **Go** | TinyGo | ~10-50 KB | Medium | Good |
| **AssemblyScript** | asc | ~2-10 KB | Low | Good |
| **Zig** | zig build-lib | ~1-5 KB | Medium | Excellent |

## Quick Start

### 1. Build an Example

```bash
cd string-distance
./build.sh
```

### 2. Test Locally

```bash
# Run Go integration tests
cd ../../..
go test ./examples/udfs/... -v
```

### 3. Register with Quidditch

```bash
curl -X POST http://localhost:8080/api/v1/udfs \
  -H 'Content-Type: multipart/form-data' \
  -F 'name=string_distance' \
  -F 'version=1.0.0' \
  -F 'wasm=@examples/udfs/string-distance/dist/string_distance.wasm'
```

### 4. Use in Query

```bash
curl -X POST http://localhost:8080/api/v1/search \
  -H 'Content-Type: application/json' \
  -d '{
    "index": "products",
    "query": {
      "wasm_udf": {
        "name": "string_distance",
        "version": "1.0.0",
        "parameters": {
          "field": "name",
          "target": "iPhone",
          "max_distance": 2
        }
      }
    }
  }'
```

## Writing Your Own UDF

### Basic Structure

All UDFs follow this pattern:

```rust
// 1. Import host functions
extern "C" {
    fn get_field_string(ctx_id: i64, ...) -> i32;
    fn has_field(ctx_id: i64, ...) -> i32;
    // ... other host functions
}

// 2. Export filter function
#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    // 3. Get parameters from query
    let target = get_param_string("target")?;

    // 4. Access document fields
    let value = get_field_string(ctx_id, "name")?;

    // 5. Apply custom logic
    let matches = custom_logic(value, target);

    // 6. Return 1 (true) or 0 (false)
    if matches { 1 } else { 0 }
}
```

### Available Host Functions

```c
// Field access
int has_field(i64 ctx_id, char* field, int len);
int get_field_string(i64 ctx_id, char* field, int field_len, char* out, int* out_len);
int get_field_i64(i64 ctx_id, char* field, int field_len, i64* out);
int get_field_f64(i64 ctx_id, char* field, int field_len, double* out);
int get_field_bool(i64 ctx_id, char* field, int field_len, int* out);

// Parameter access
int get_param_string(char* name, int name_len, char* out, int* out_len);
int get_param_i64(char* name, int name_len, i64* out);
int get_param_f64(char* name, int name_len, double* out);
int get_param_bool(char* name, int name_len, int* out);

// Utilities
void log(int level, char* msg, int len);  // 0=debug, 1=info, 2=warn, 3=error
```

### Function Signature

```wasm
(func (export "filter") (param i64) (result i32))
```

- **Input**: Document context ID (i64)
- **Output**: 0 (false/exclude) or 1 (true/include)

### Memory Management

- UDFs must export `memory` for host access
- Use static buffers or stack allocation (no malloc)
- Keep memory usage minimal (< 1MB recommended)

## Build Requirements

### Common Tools

```bash
# Install Rust
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
rustup target add wasm32-unknown-unknown

# Install C/C++ for WASM
sudo apt-get install clang lld

# Install WABT (WebAssembly Binary Toolkit)
sudo apt-get install wabt

# Optional: wasm-opt for size optimization
sudo apt-get install binaryen
```

### Per-Language Setup

**Rust**:
```bash
rustup target add wasm32-unknown-unknown
```

**C/C++**:
```bash
# clang with wasm32 target (usually included)
clang --version
```

**Go**:
```bash
# Install TinyGo
wget https://github.com/tinygo-org/tinygo/releases/download/v0.31.0/tinygo_0.31.0_amd64.deb
sudo dpkg -i tinygo_0.31.0_amd64.deb
```

**AssemblyScript**:
```bash
npm install -g assemblyscript
```

## Best Practices

### Performance

1. **Keep it Simple**: Complex logic = slower queries
2. **Early Return**: Exit as soon as decision is made
3. **Use Integers**: Faster than floating point when possible
4. **Avoid Allocations**: Use stack or static buffers
5. **Minimize Host Calls**: Cache field values

### Size Optimization

1. **Strip Debug Info**: Use release builds
2. **Enable LTO**: Link-time optimization
3. **Use wasm-opt**: `-Oz` flag for size
4. **Avoid Standard Library**: Use `nostdlib` when possible
5. **Dead Code Elimination**: Only include what's used

### Error Handling

1. **Graceful Failures**: Return 0 instead of panicking
2. **Validate Inputs**: Check field existence and types
3. **Log Errors**: Use log() host function for debugging
4. **Document Assumptions**: What fields are required?

### Testing

1. **Unit Tests**: Test logic independently
2. **Integration Tests**: Test with real Quidditch engine
3. **Benchmark**: Measure performance impact
4. **Edge Cases**: Empty strings, missing fields, invalid data

## Common Patterns

### Field Existence Check

```rust
if !has_field(ctx_id, "field_name") {
    return 0;  // Field doesn't exist
}
```

### Safe Parameter Access

```rust
let threshold = match get_param_i64("threshold") {
    Ok(v) => v,
    Err(_) => 10,  // Default value
};
```

### Multi-Field Logic

```rust
let price = get_field_f64(ctx_id, "price")?;
let rating = get_field_f64(ctx_id, "rating")?;
let score = price * rating;
return if score > threshold { 1 } else { 0 };
```

### String Comparison

```rust
let category = get_field_string(ctx_id, "category")?;
return if category == "electronics" { 1 } else { 0 };
```

## Debugging

### Local Testing

```go
// Load and test UDF
wasmBytes, _ := os.ReadFile("path/to/udf.wasm")
runtime, _ := wasm.NewRuntime(&wasm.Config{EnableJIT: true})
defer runtime.Close()

// Compile
compiledMod, _ := runtime.CompileModule("test_udf", wasmBytes)

// Create test document
docCtx := wasm.NewDocumentContextFromMap("doc1", 1.0, map[string]interface{}{
    "name": "test product",
    "price": 99.99,
})

// Execute
results, err := instance.CallFunction(ctx, "filter", docCtx.ID)
fmt.Printf("Result: %d, Error: %v\n", results[0], err)
```

### Logging

```rust
fn debug_log(msg: &str) {
    unsafe {
        log(0, msg.as_ptr(), msg.len() as i32);
    }
}

// In filter function
debug_log(&format!("Processing doc, field value: {}", value));
```

### Common Issues

1. **Memory Access Violations**
   - Check buffer sizes
   - Validate pointers before dereferencing

2. **Stack Overflow**
   - Reduce local variable sizes
   - Use heap allocation sparingly

3. **Type Mismatches**
   - Verify field types match expectations
   - Handle type conversion errors

4. **Performance Issues**
   - Profile with benchmarks
   - Reduce host function calls
   - Optimize algorithms

## Performance Targets

| Metric | Target | Good | Acceptable |
|--------|--------|------|------------|
| **UDF Execution** | <5μs | <10μs | <50μs |
| **Binary Size** | <5KB | <20KB | <100KB |
| **Memory Usage** | <100KB | <500KB | <1MB |
| **Compilation Time** | <10ms | <50ms | <100ms |

## Contributing

To add a new example:

1. Create directory: `examples/udfs/your-udf/`
2. Add source files
3. Create `build.sh` and `README.md`
4. Add integration test
5. Submit PR with example

## Resources

- [Quidditch UDF Guide](../../docs/udfs/writing-udfs.md)
- [WASM Reference](https://webassembly.org/)
- [Wazero Documentation](https://wazero.io/)
- [Rust WASM Book](https://rustwasm.github.io/docs/book/)
- [WebAssembly Studio](https://webassembly.studio/)

## License

All examples are MIT licensed and part of the Quidditch project.
