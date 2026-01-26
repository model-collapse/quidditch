# UDF Performance Guide

Optimize your User-Defined Functions for maximum performance in the Quidditch search engine.

## Table of Contents

- [Performance Overview](#performance-overview)
- [Optimization Strategies](#optimization-strategies)
- [Benchmarking](#benchmarking)
- [Profiling](#profiling)
- [Common Bottlenecks](#common-bottlenecks)
- [Best Practices](#best-practices)
- [Case Studies](#case-studies)

---

## Performance Overview

### Target Metrics

| Metric | Excellent | Good | Acceptable | Poor |
|--------|-----------|------|------------|------|
| **Execution Time** | <5μs | <10μs | <50μs | >50μs |
| **Binary Size** | <5KB | <20KB | <100KB | >100KB |
| **Memory Usage** | <100KB | <500KB | <1MB | >1MB |
| **Compilation Time** | <10ms | <50ms | <100ms | >100ms |

### Performance Budget

For a query processing 1000 documents:
- **Total UDF overhead**: <5ms target, <10ms acceptable
- **Per-document**: <5μs average
- **Query latency impact**: <10% increase

---

## Optimization Strategies

### 1. Algorithm Optimization

#### Use Efficient Algorithms

**Bad** (O(n²)):
```rust
fn has_duplicate_chars(s: &str) -> bool {
    for (i, c1) in s.chars().enumerate() {
        for (j, c2) in s.chars().enumerate() {
            if i != j && c1 == c2 {
                return true;
            }
        }
    }
    false
}
```

**Good** (O(n)):
```rust
fn has_duplicate_chars(s: &str) -> bool {
    let mut seen = [false; 256];
    for byte in s.bytes() {
        if seen[byte as usize] {
            return true;
        }
        seen[byte as usize] = true;
    }
    false
}
```

#### Early Exit

**Bad**:
```rust
fn filter(ctx_id: i64) -> i32 {
    let price = get_field_f64(ctx_id, "price");
    let category = get_field_string(ctx_id, "category");
    let rating = get_field_f64(ctx_id, "rating");

    // All fields fetched even if price check fails
    if price < 100.0 {
        return 0;
    }
    // ... more checks
}
```

**Good**:
```rust
fn filter(ctx_id: i64) -> i32 {
    // Exit immediately if price check fails
    let price = get_field_f64(ctx_id, "price");
    if price < 100.0 {
        return 0;
    }

    // Only fetch other fields if needed
    let category = get_field_string(ctx_id, "category");
    // ... more checks
}
```

### 2. Memory Optimization

#### Stack vs Heap Allocation

**Bad** (heap allocation):
```rust
fn filter(ctx_id: i64) -> i32 {
    let buffer = vec![0u8; 256]; // Heap allocation (slow)
    // ...
}
```

**Good** (stack allocation):
```rust
fn filter(ctx_id: i64) -> i32 {
    let mut buffer = [0u8; 256]; // Stack allocation (fast)
    // ...
}
```

#### Static Buffers

**Bad** (repeated allocation):
```rust
fn filter(ctx_id: i64) -> i32 {
    let mut buffer = [0u8; 256]; // Allocated on each call
    // ...
}
```

**Good** (static buffer):
```rust
static mut BUFFER: [u8; 256] = [0; 256];

fn filter(ctx_id: i64) -> i32 {
    unsafe {
        // Reuse same buffer
        // ...
    }
}
```

#### Memory Pooling

For complex data structures, consider pre-allocating:

```rust
static mut DISTANCE_MATRIX: [[usize; 100]; 100] = [[0; 100]; 100];

fn levenshtein_distance(s1: &str, s2: &str) -> usize {
    unsafe {
        // Reuse pre-allocated matrix
        let matrix = &mut DISTANCE_MATRIX;
        // ...
    }
}
```

### 3. Minimize Host Calls

Host function calls have overhead (~10-50ns each).

**Bad** (5 host calls):
```rust
fn filter(ctx_id: i64) -> i32 {
    if has_field(ctx_id, "price") == 0 {
        return 0;
    }
    let price = get_field_f64(ctx_id, "price");
    if has_field(ctx_id, "category") == 0 {
        return 0;
    }
    let category = get_field_string(ctx_id, "category");
    // ...
}
```

**Good** (2 host calls):
```rust
fn filter(ctx_id: i64) -> i32 {
    // Try to get field, handle error if missing
    let price = match get_field_f64(ctx_id, "price") {
        Ok(v) => v,
        Err(_) => return 0,
    };

    let category = match get_field_string(ctx_id, "category") {
        Ok(v) => v,
        Err(_) => return 0,
    };
    // ...
}
```

#### Batch Field Access

If you need multiple fields, fetch them together:

```rust
struct DocumentFields {
    price: f64,
    rating: f64,
    category: String,
}

fn get_fields(ctx_id: i64) -> Option<DocumentFields> {
    Some(DocumentFields {
        price: get_field_f64(ctx_id, "price").ok()?,
        rating: get_field_f64(ctx_id, "rating").ok()?,
        category: get_field_string(ctx_id, "category").ok()?,
    })
}
```

### 4. Binary Size Optimization

#### Compiler Flags

**Cargo.toml**:
```toml
[profile.release]
opt-level = "z"      # Optimize for size
lto = true           # Link-time optimization
codegen-units = 1    # Better optimization
strip = true         # Strip symbols
panic = "abort"      # No unwinding
```

#### Remove Standard Library

```rust
#![no_std]

// Use core instead of std
use core::slice;
use core::str;
```

#### Use wasm-opt

```bash
# Aggressive size optimization
wasm-opt -Oz input.wasm -o output.wasm

# Also try:
wasm-opt -Os input.wasm -o output.wasm  # Balanced
wasm-opt -O3 input.wasm -o output.wasm  # Speed over size
```

#### Avoid Heavy Dependencies

```toml
# Bad: Pulls in lots of code
[dependencies]
regex = "1.0"

# Good: Use simple string operations
# No dependencies needed
```

### 5. Computation Optimization

#### Use Integer Arithmetic

**Bad** (floating point):
```rust
fn discount_check(price: f64) -> bool {
    let discount = price * 0.20;
    discount > 10.0
}
```

**Good** (integer, if precision allows):
```rust
fn discount_check(price_cents: i64) -> bool {
    let discount_cents = (price_cents * 20) / 100;
    discount_cents > 1000
}
```

#### Lookup Tables

**Bad** (repeated calculation):
```rust
fn char_value(c: char) -> i32 {
    match c {
        'a'..='z' => 1,
        'A'..='Z' => 2,
        '0'..='9' => 3,
        _ => 0,
    }
}
```

**Good** (lookup table):
```rust
static CHAR_VALUES: [i32; 256] = {
    let mut table = [0; 256];
    // Initialize once at compile time
    // ...
    table
};

fn char_value(c: u8) -> i32 {
    CHAR_VALUES[c as usize]
}
```

---

## Benchmarking

### Local Benchmarks

```go
func BenchmarkMyUDF(b *testing.B) {
    // Setup
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

    // Test documents of varying complexity
    documents := []map[string]interface{}{
        {"price": 99.99},                                    // Simple
        {"price": 99.99, "name": "Product", "rating": 4.5}, // Medium
        {"price": 99.99, "name": "Long product name with many characters",
         "description": "...", "tags": []string{"a", "b", "c"}}, // Complex
    }

    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        doc := documents[i%len(documents)]
        docCtx := wasm.NewDocumentContextFromMap("doc", 1.0, doc)
        registry.Call(ctx, "my_udf", "1.0.0", docCtx, nil)
    }
}
```

**Run**:
```bash
go test -bench=BenchmarkMyUDF -benchtime=10s -benchmem

# Output:
# BenchmarkMyUDF-64    5000000    3.8 μs/op    0 B/op    0 allocs/op
```

### Production Monitoring

```bash
# Get real-world stats
curl http://localhost:8080/api/v1/udfs/my_udf/1.0.0/stats

# Monitor over time
while true; do
    curl -s http://localhost:8080/api/v1/udfs/my_udf/1.0.0/stats | \
        jq '.statistics | {calls: .call_count, p99: .p99_latency_us}'
    sleep 60
done
```

### Comparative Benchmarking

Compare multiple implementations:

```bash
# Test version 1.0.0
go test -bench=BenchmarkV1 > v1_results.txt

# Test version 2.0.0 (optimized)
go test -bench=BenchmarkV2 > v2_results.txt

# Compare
benchstat v1_results.txt v2_results.txt
```

---

## Profiling

### CPU Profiling

```go
import "runtime/pprof"

func TestProfileUDF(t *testing.T) {
    f, _ := os.Create("cpu.prof")
    defer f.Close()
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()

    // Run UDF many times
    for i := 0; i < 100000; i++ {
        registry.Call(ctx, "my_udf", "1.0.0", docCtx, nil)
    }
}
```

**Analyze**:
```bash
go tool pprof cpu.prof
(pprof) top10
(pprof) list filter
```

### Memory Profiling

```go
func TestMemoryProfile(t *testing.T) {
    // Run UDF
    for i := 0; i < 100000; i++ {
        registry.Call(ctx, "my_udf", "1.0.0", docCtx, nil)
    }

    // Capture heap profile
    f, _ := os.Create("mem.prof")
    defer f.Close()
    pprof.WriteHeapProfile(f)
}
```

**Analyze**:
```bash
go tool pprof mem.prof
(pprof) top10
(pprof) list filter
```

### WASM-Specific Profiling

```rust
// Add timing to your UDF
static mut START_TIME: u64 = 0;

#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    unsafe { START_TIME = get_time_us(); }

    // Your logic here

    unsafe {
        let elapsed = get_time_us() - START_TIME;
        debug_log(&format!("Filter took {}μs", elapsed));
    }

    1
}
```

---

## Common Bottlenecks

### 1. String Operations

**Problem**: String concatenation, searching, parsing

**Solution**:
- Use stack buffers
- Avoid repeated allocations
- Use byte operations instead of char operations

**Example**:
```rust
// Bad: Repeated allocation
fn concat_strings(s1: &str, s2: &str) -> String {
    format!("{}{}", s1, s2) // Allocates
}

// Good: Write to pre-allocated buffer
fn concat_strings(s1: &str, s2: &str, buffer: &mut [u8]) -> &str {
    let len1 = s1.len();
    buffer[..len1].copy_from_slice(s1.as_bytes());
    buffer[len1..len1+s2.len()].copy_from_slice(s2.as_bytes());
    core::str::from_utf8(&buffer[..len1+s2.len()]).unwrap()
}
```

### 2. Complex Data Structures

**Problem**: Trees, graphs, dynamic collections

**Solution**:
- Use arrays instead of vectors
- Pre-allocate maximum size
- Use indices instead of pointers

**Example**:
```rust
// Bad: Dynamic vector
fn store_items(items: Vec<Item>) { ... }

// Good: Fixed-size array
const MAX_ITEMS: usize = 100;
fn store_items(items: &[Item; MAX_ITEMS]) { ... }
```

### 3. Floating Point Math

**Problem**: Slow transcendental functions (sin, cos, sqrt)

**Solution**:
- Use lookup tables for repeated calculations
- Use integer math when precision allows
- Cache results

**Example**:
```rust
// Bad: Repeated sqrt
fn distance(x1: f64, y1: f64, x2: f64, y2: f64) -> f64 {
    ((x2-x1).powi(2) + (y2-y1).powi(2)).sqrt()
}

// Good: Compare squared distances (avoid sqrt)
fn is_within_distance(x1: f64, y1: f64, x2: f64, y2: f64, max: f64) -> bool {
    let sq_dist = (x2-x1).powi(2) + (y2-y1).powi(2);
    sq_dist <= max * max  // No sqrt needed
}
```

### 4. Excessive Host Calls

**Problem**: Many calls to get_field_* functions

**Solution**:
- Batch field access
- Cache field values
- Check field existence only when necessary

### 5. Large WASM Binaries

**Problem**: Slow compilation, high memory usage

**Solution**:
- Remove unused code
- Use wasm-opt
- Split into multiple smaller UDFs

---

## Best Practices

### 1. Design for Performance

**Before Writing Code**:
- Profile similar operations
- Estimate complexity
- Choose efficient algorithms
- Plan memory usage

### 2. Measure Early

**Don't Guess**:
- Benchmark before optimizing
- Profile to find hot spots
- Measure after each change
- Compare against baseline

### 3. Optimize Iteratively

**Process**:
1. Write correct implementation
2. Add benchmarks
3. Identify slowest part
4. Optimize that part
5. Measure improvement
6. Repeat if needed

### 4. Balance Trade-offs

Consider:
- **Speed vs Size**: Lookup tables vs calculation
- **Speed vs Readability**: Optimization vs maintenance
- **Speed vs Memory**: Caching vs recomputation

### 5. Set Performance Budgets

**Per UDF**:
- Max execution time: 10μs
- Max binary size: 10KB
- Max memory: 500KB

**Monitor and Alert**:
- P99 latency exceeds 20μs
- Error rate exceeds 0.1%
- Memory usage exceeds budget

---

## Case Studies

### Case Study 1: String Distance UDF

**Initial Version** (O(m×n) space):
```rust
fn levenshtein(s1: &str, s2: &str) -> usize {
    let mut matrix = vec![vec![0; s2.len()+1]; s1.len()+1];
    // ... algorithm
}
```

**Performance**: 50μs for 50-character strings
**Binary Size**: 8KB

**Optimized Version** (O(n) space):
```rust
fn levenshtein(s1: &str, s2: &str) -> usize {
    let mut prev_row = [0usize; 256];
    let mut curr_row = [0usize; 256];
    // ... algorithm
}
```

**Performance**: 30μs for 50-character strings (40% improvement)
**Binary Size**: 3KB (62% reduction)

**Result**: ✅ Meets performance targets

### Case Study 2: Geo Filter UDF

**Initial Version**:
```c
double haversine(double lat1, double lon1, double lat2, double lon2) {
    double dlat = deg_to_rad(lat2 - lat1);
    double dlon = deg_to_rad(lon2 - lon1);
    double a = sin(dlat/2) * sin(dlat/2) +
               cos(deg_to_rad(lat1)) * cos(deg_to_rad(lat2)) *
               sin(dlon/2) * sin(dlon/2);
    double c = 2 * atan2(sqrt(a), sqrt(1-a));
    return EARTH_RADIUS * c;
}
```

**Performance**: 5μs per call
**Binary Size**: 3KB

**Optimization Attempts**:
1. Lookup tables for sin/cos: ❌ No improvement (modern CPUs fast)
2. Integer arithmetic: ❌ Precision loss unacceptable
3. Early exit for obvious cases: ✅ 20% improvement

**Final Version**:
```c
double haversine_optimized(double lat1, double lon1, double lat2, double lon2) {
    // Quick rejection for far distances
    if (abs(lat2 - lat1) > 1.0 && abs(lon2 - lon1) > 1.0) {
        return 1000000.0; // Obviously far
    }

    // Full calculation
    // ... same as before
}
```

**Performance**: 4μs average (20% improvement)
**Binary Size**: 2KB

**Result**: ✅ Excellent performance

### Case Study 3: Custom Score UDF

**Initial Version** (Rust):
```rust
fn filter(ctx_id: i64) -> i32 {
    let base_score: f64 = get_field_f64(ctx_id, "base_score")?;
    let boost: f64 = get_field_f64(ctx_id, "boost").unwrap_or(1.0);
    let threshold: f64 = get_param_f64("min_score").unwrap_or(0.5);

    let final_score = base_score * boost;
    if final_score >= threshold { 1 } else { 0 }
}
```

**Performance**: 2μs
**Binary Size**: 15KB

**Optimized Version** (WAT):
```wasm
(func (export "filter") (param i64) (result i32)
    ;; Inline all operations
    ;; Direct memory access
    ;; ...
)
```

**Performance**: 0.5μs (75% improvement!)
**Binary Size**: 600 bytes (96% reduction!)

**Result**: ✅ Exceptional performance

---

## Performance Checklist

Before deploying a UDF:

- [ ] Execution time < 10μs for typical documents
- [ ] Binary size < 20KB
- [ ] Memory usage < 1MB
- [ ] Benchmarks show consistent performance
- [ ] No unnecessary host function calls
- [ ] Proper error handling (no panics)
- [ ] Early exit for common cases
- [ ] Stack allocation preferred over heap
- [ ] Compiled with optimization flags
- [ ] Optimized with wasm-opt
- [ ] Profiled and hot spots addressed
- [ ] Tested with production-like data
- [ ] Monitoring and alerts configured

---

## Resources

- [Writing UDFs Guide](./writing-udfs.md)
- [API Reference](./api-reference.md)
- [Examples](../../examples/udfs/)
- [WebAssembly Performance](https://v8.dev/blog/wasm-code-caching)
- [Rust Performance Book](https://nnethercote.github.io/perf-book/)

---

**Remember**: Premature optimization is the root of all evil. Profile first, optimize second! 🚀
