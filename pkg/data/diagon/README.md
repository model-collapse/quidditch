# Diagon Bridge - Go ↔ C++ Interface

This package provides a Go interface to the Diagon C++ search engine core using CGO.

## Current Status

**Mode**: Stub (In-Memory)
**CGO**: Disabled (will enable when Diagon C++ core is ready)

The bridge currently operates in **stub mode**, using in-memory Go data structures to simulate Diagon functionality. This allows the rest of the Quidditch system to be developed and tested without waiting for the C++ implementation.

## Architecture

```
┌─────────────────────────────────────────┐
│   Go Data Node (pkg/data/data.go)      │
├─────────────────────────────────────────┤
│   Shard Manager (pkg/data/shard.go)    │
├─────────────────────────────────────────┤
│   Diagon Bridge (this package)          │
│   ┌──────────────────────────────────┐  │
│   │  Go Wrapper (bridge.go)          │  │
│   ├──────────────────────────────────┤  │
│   │  CGO Bindings (commented out)    │  │
│   ├──────────────────────────────────┤  │
│   │  C API (diagon.h) - TODO         │  │
│   └──────────────────────────────────┘  │
├─────────────────────────────────────────┤
│   Diagon C++ Core (../../diagon/)      │
│   • Inverted Index                      │
│   • Forward Index (Columnar)            │
│   • SIMD BM25 Scoring                   │
│   • Compression (LZ4, ZSTD)             │
└─────────────────────────────────────────┘
```

## Files

- **bridge.go** - Main CGO bridge with C API declarations
- **README.md** - This file

## Stub Mode (Current)

In stub mode, the bridge:
- ✅ Implements all required interfaces
- ✅ Stores documents in memory (Go maps)
- ✅ Returns mock search results
- ✅ Logs all operations
- ⚠️ Does NOT persist to disk
- ⚠️ Does NOT perform real search
- ⚠️ Does NOT use SIMD

**Use Case**: Development and testing of the distributed layer without C++

## Enabling CGO (Future)

When the Diagon C++ core is ready:

### Step 1: Implement C API

Create `diagon/include/diagon.h`:

```c
#ifndef DIAGON_H
#define DIAGON_H

#ifdef __cplusplus
extern "C" {
#endif

// Engine management
typedef struct diagon_engine diagon_engine_t;
diagon_engine_t* diagon_create_engine(const char* data_dir, int simd_enabled);
void diagon_destroy_engine(diagon_engine_t* engine);

// Shard management
typedef struct diagon_shard diagon_shard_t;
diagon_shard_t* diagon_create_shard(diagon_engine_t* engine, const char* path);
void diagon_destroy_shard(diagon_shard_t* shard);

// Document operations
int diagon_index_document(diagon_shard_t* shard, const char* doc_id, const char* doc_json);
char* diagon_get_document(diagon_shard_t* shard, const char* doc_id);
int diagon_delete_document(diagon_shard_t* shard, const char* doc_id);

// Search operations
char* diagon_search(diagon_shard_t* shard, const char* query_json);

// Maintenance operations
int diagon_refresh(diagon_shard_t* shard);
int diagon_flush(diagon_shard_t* shard);

#ifdef __cplusplus
}
#endif

#endif // DIAGON_H
```

### Step 2: Build C++ Library

```bash
cd diagon
mkdir build
cd build
cmake .. -DCMAKE_BUILD_TYPE=Release
make
# This produces libdiagon.so (Linux) or libdiagon.dylib (macOS)
```

### Step 3: Enable CGO in bridge.go

1. Uncomment the `#include "diagon.h"` line
2. Set `cgoEnabled: true` in `NewDiagonBridge()`
3. Uncomment all C function calls in the methods
4. Remove stub implementations

### Step 4: Build Go with CGO

```bash
# Ensure CGO is enabled
export CGO_ENABLED=1

# Set library path
export LD_LIBRARY_PATH=$PWD/diagon/lib:$LD_LIBRARY_PATH

# Build
go build -tags cgo ./cmd/data
```

## API Reference

### DiagonBridge

Main interface to the Diagon engine.

```go
bridge, err := diagon.NewDiagonBridge(&diagon.Config{
    DataDir:     "/var/lib/quidditch/data",
    SIMDEnabled: true,
    Logger:      logger,
})

err = bridge.Start()
defer bridge.Stop()
```

### Shard

Represents a single shard (index partition).

```go
shard, err := bridge.CreateShard("/path/to/shard")

// Index document
err = shard.IndexDocument("doc1", map[string]interface{}{
    "title": "Quidditch Search Engine",
    "content": "Fast and distributed",
})

// Search
results, err := shard.Search([]byte(`{"query": {"match": {"title": "search"}}}`))

// Get document
doc, err := shard.GetDocument("doc1")

// Delete document
err = shard.DeleteDocument("doc1")

// Maintenance
err = shard.Refresh() // Make changes visible
err = shard.Flush()   // Persist to disk
err = shard.Close()   // Close shard
```

## Performance Considerations

### Memory Management

- C++ allocates memory for documents and search results
- Go must free C-allocated memory using `C.free()`
- Use `defer C.free()` immediately after C calls

### String Conversion

- Go strings → C strings: `C.CString()` (allocates)
- C strings → Go strings: `C.GoString()` (copies)
- Always free C strings with `C.free()`

### Concurrency

- CGO calls are NOT goroutine-safe by default
- Use mutexes around C++ calls
- Consider thread-local storage in C++ if needed

## Testing

### Stub Mode Testing

```go
// No CGO required - runs everywhere
go test ./pkg/data/diagon
```

### CGO Mode Testing

```go
// Requires C++ library
CGO_ENABLED=1 go test ./pkg/data/diagon
```

## Debugging

### Enable CGO Debug

```bash
export CGO_CFLAGS="-g -O0"
export CGO_LDFLAGS="-g"
go build -gcflags="all=-N -l" ./cmd/data
```

### GDB Debugging

```bash
gdb ./bin/quidditch-data
(gdb) break diagon_search
(gdb) run --config config/dev-data.yaml
```

### Valgrind Memory Check

```bash
CGO_ENABLED=1 go build -buildmode=c-shared ./cmd/data
valgrind --leak-check=full ./bin/quidditch-data
```

## Error Handling

### C++ Exceptions

C++ exceptions CANNOT cross the CGO boundary. The C API must:
1. Catch all C++ exceptions
2. Convert to error codes
3. Return NULL on failure

Example:
```cpp
extern "C" diagon_shard_t* diagon_create_shard(diagon_engine_t* engine, const char* path) {
    try {
        return reinterpret_cast<diagon_shard_t*>(
            new DiagonShard(engine, path)
        );
    } catch (const std::exception& e) {
        // Log error
        return nullptr;
    }
}
```

## Performance Targets

With Diagon C++ core:
- **Indexing**: 100k docs/sec/node
- **Query Latency**: <10ms p99 (term queries)
- **Memory**: 40-70% less than Lucene (compression)
- **SIMD Speedup**: 4-8× (BM25 scoring)

## Resources

- [CGO Documentation](https://golang.org/cmd/cgo/)
- [Diagon Architecture](../../../design/QUIDDITCH_ARCHITECTURE.md)
- [C++ Best Practices for CGO](https://github.com/golang/go/wiki/cgo)

## Status Checklist

- [x] Go interface defined
- [x] Stub implementation working
- [ ] C API header defined
- [ ] C++ implementation complete
- [ ] CGO bindings tested
- [ ] Performance benchmarks
- [ ] Memory leak testing
- [ ] Production deployment

## Next Steps

1. Complete Diagon C++ core (Phase 0)
2. Implement C API wrapper
3. Enable CGO in bridge.go
4. Run integration tests
5. Performance benchmarking

---

**Last Updated**: 2026-01-25
**Status**: Stub Mode (CGO Disabled)
