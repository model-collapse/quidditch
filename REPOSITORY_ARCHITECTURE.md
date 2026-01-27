# Repository Architecture: Diagon vs Quidditch

## Executive Summary

**Problem**: Code organization between Diagon (C++ library) and Quidditch (Go application) has unclear boundaries, leading to confusion about what belongs where.

**Solution**: Define clear separation of concerns with strict language boundaries.

---

## Repository Boundaries

### 🔵 **Diagon Repository** (C++ Search Engine Library)

**Location**: `pkg/data/diagon/upstream/` (git submodule)
**GitHub**: `github.com/model-collapse/diagon`
**Language**: **100% C++** (C++20)
**Purpose**: Pure C++ search engine library (Lucene + ClickHouse architecture)

#### What Belongs in Diagon

**✅ ALLOWED:**
1. **C++ Implementation** (`src/core/`, `src/columns/`, `src/compression/`, `src/simd/`)
   - All search engine core logic
   - Inverted index (IndexWriter, IndexReader, IndexSearcher)
   - Column storage (IColumn, MergeTree)
   - Compression codecs (LZ4, ZSTD, Delta, Gorilla)
   - SIMD acceleration (AVX2, NEON)
   - Text analysis (Tokenizers, Analyzers, Filters)
   - Query execution (Queries, Scorers, Collectors)

2. **C++ Headers** (`src/core/include/`)
   - Public C++ APIs
   - Internal implementation headers

3. **C API Layer** (`src/core/include/diagon/*.h`, `src/core/src/*/analysis_c.cpp`)
   - Opaque handle-based C APIs for language bindings
   - Exception-safe wrappers around C++
   - Thread-local error storage
   - Example: `diagon_analyzer_t`, `diagon_create_standard_analyzer()`

4. **C++ Tests** (`tests/unit/`, `tests/integration/`)
   - GoogleTest-based tests
   - Benchmarks in C++
   - All testing in C++

5. **Build System** (`CMakeLists.txt`, `cmake/`)
   - CMake configuration
   - Dependency management (vcpkg, conan)
   - SIMD detection, compiler flags

6. **Documentation** (`docs/`, `design/`)
   - Design documents
   - API reference
   - Architecture guides
   - All C++ focused

**❌ NOT ALLOWED:**
- ❌ Go code (no `.go` files)
- ❌ Go tests
- ❌ Go bindings (belongs in Quidditch)
- ❌ Application logic (belongs in Quidditch)
- ❌ HTTP APIs (belongs in Quidditch)
- ❌ Distributed system code (belongs in Quidditch)

#### Diagon Structure

```
diagon/
├── CMakeLists.txt                 # Root CMake
├── cmake/                         # CMake modules
├── src/
│   ├── core/                      # Core search engine
│   │   ├── include/
│   │   │   ├── diagon/           # C API headers (*.h)
│   │   │   ├── analysis/         # C++ headers (*.h)
│   │   │   ├── index/            # C++ headers
│   │   │   ├── search/           # C++ headers
│   │   │   └── store/            # C++ headers
│   │   └── src/
│   │       ├── analysis/         # C++ implementation (*.cpp)
│   │       ├── index/            # C++ implementation
│   │       ├── search/           # C++ implementation
│   │       └── store/            # C++ implementation
│   ├── columns/                   # Column storage (C++)
│   ├── compression/               # Compression codecs (C++)
│   └── simd/                      # SIMD acceleration (C++)
├── tests/
│   ├── unit/                      # GoogleTest tests (*.cpp)
│   ├── integration/               # Integration tests (C++)
│   └── benchmark/                 # Benchmarks (C++)
├── docs/                          # Documentation
│   ├── designs/                   # Design documents
│   ├── guides/                    # User guides
│   └── reference/                 # API reference
└── design/                        # Architecture docs

# NO .go FILES ANYWHERE
```

---

### 🟢 **Quidditch Repository** (Go Distributed Search System)

**Location**: `/home/ubuntu/quidditch/`
**GitHub**: `github.com/model-collapse/quidditch`
**Primary Language**: **Go** (with minimal C++ bridge code)
**Purpose**: Distributed search system built on Diagon

#### What Belongs in Quidditch

**✅ ALLOWED:**

1. **Go Application Code** (`pkg/`, `cmd/`)
   - Master node (Raft, cluster management)
   - Coordination node (HTTP API, query routing)
   - Data node (shard management)
   - Query planning and execution
   - Pipeline framework
   - UDF registry and execution

2. **CGO Bindings to Diagon** (`pkg/data/diagon/`)
   - Go wrappers around Diagon C API
   - Type conversions (Go ↔ C)
   - Memory management (defer cleanup)
   - Example: `analysis.go`, `bridge.go`

3. **C++ API Bridge** (`pkg/data/diagon/c_api_src/`)
   - **MINIMAL C++ code** that wraps Diagon C API
   - Only when C API is insufficient
   - Compiles to library linked by CGO
   - Example: `diagon_c_api.cpp` (main bridge)
   - **Keep this layer THIN**

4. **Go Tests** (`*_test.go`)
   - Test Go code and Go bindings
   - Integration tests with Diagon via CGO
   - Example: `analysis_test.go`, `bridge_test.go`

5. **Build System**
   - Go modules (`go.mod`, `go.sum`)
   - Makefiles for building C++ bridge
   - Docker builds

6. **Application Documentation**
   - README, roadmaps
   - Deployment guides
   - API documentation

**❌ MINIMIZE:**
- ⚠️ C++ code in Quidditch (only for thin bridge layer)
- ⚠️ Keep `c_api_src/` as small as possible
- ⚠️ Prefer extending Diagon C API over adding bridge code

#### Quidditch Structure

```
quidditch/
├── go.mod, go.sum                 # Go modules
├── cmd/
│   ├── master/                    # Master node (Go)
│   ├── coordination/              # Coordination node (Go)
│   └── data/                      # Data node (Go)
├── pkg/
│   ├── master/                    # Master logic (Go)
│   ├── coordination/              # Coordination logic (Go)
│   ├── data/                      # Data node logic (Go)
│   │   ├── shard.go              # Shard management (Go)
│   │   ├── analyzer_settings.go  # Analyzer config (Go)
│   │   └── diagon/               # Diagon bindings
│   │       ├── analysis.go       # Go wrapper (CGO)
│   │       ├── analysis_test.go  # Go tests
│   │       ├── bridge.go         # Go wrapper (CGO)
│   │       ├── c_api_src/        # **THIN C++ bridge**
│   │       │   ├── diagon_c_api.h
│   │       │   ├── diagon_c_api.cpp
│   │       │   └── minimal_wrapper.cpp
│   │       └── upstream/         # Git submodule → Diagon
│   ├── query/                     # Query planner (Go)
│   ├── pipeline/                  # Pipeline framework (Go)
│   └── wasm/                      # WASM UDF (Go)
└── docs/                          # Application docs

# Primary language: Go
# C++ only in pkg/data/diagon/c_api_src/ (thin bridge)
```

---

## Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Quidditch (Go)                            │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Application Layer (Pure Go)                           │ │
│  │  - HTTP API, Cluster Management, Query Planning        │ │
│  └──────────────────────┬─────────────────────────────────┘ │
│                         │                                    │
│  ┌──────────────────────▼─────────────────────────────────┐ │
│  │  Go Bindings (pkg/data/diagon/*.go)                    │ │
│  │  - analysis.go, bridge.go                              │ │
│  │  - CGO calls to C API                                  │ │
│  └──────────────────────┬─────────────────────────────────┘ │
│                         │ CGO                                │
│  ┌──────────────────────▼─────────────────────────────────┐ │
│  │  C API Bridge (c_api_src/*.cpp) - THIN LAYER          │ │
│  │  - diagon_c_api.cpp                                    │ │
│  │  - Minimal wrappers around Diagon C API               │ │
│  └──────────────────────┬─────────────────────────────────┘ │
└─────────────────────────┼───────────────────────────────────┘
                          │ Shared Library (.so)
┌─────────────────────────▼───────────────────────────────────┐
│               Diagon (100% C++)                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  C API (src/core/include/diagon/*.h)                   │ │
│  │  - analysis_c.h, diagon_c_api.h                        │ │
│  │  - Opaque handles, exception-safe                      │ │
│  └──────────────────────┬─────────────────────────────────┘ │
│                         │                                    │
│  ┌──────────────────────▼─────────────────────────────────┐ │
│  │  C++ Implementation (src/core/src/)                    │ │
│  │  - Analyzer, Tokenizer, IndexWriter, etc.             │ │
│  │  - Core search engine logic                            │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                              │
│  Output: libdiagon_core.so + C headers                      │
└──────────────────────────────────────────────────────────────┘
```

---

## Design Principles

### 1. **Language Separation**
- **Diagon = 100% C++** (library)
- **Quidditch = Go + thin C++ bridge** (application)

### 2. **C API as Contract**
- All Diagon functionality exposed via C API
- Go never calls C++ directly (always via C API)
- C API is stable interface between repos

### 3. **Minimize Bridge Code**
- Keep `c_api_src/` as small as possible
- Prefer extending Diagon C API over adding bridge code
- Bridge only does type conversion, not logic

### 4. **Testing Boundaries**
- Diagon tests in C++ (GoogleTest)
- Quidditch tests in Go (testing package)
- Integration tests via CGO in Quidditch

### 5. **Build Separation**
- Diagon: CMake + C++ toolchain
- Quidditch: Go modules + CGO
- Diagon builds `libdiagon_core.so`
- Quidditch links against it

---

## Migration Plan (If Needed)

### Current Issues to Fix

1. **✅ No Go in Diagon**: Already correct (checked - no .go files in upstream)

2. **⚠️ Review C++ Bridge Layer**: Ensure `c_api_src/` is minimal
   - Current files: 6 files (~1,500 lines)
   - **Action**: Review each file, consider moving logic to Diagon C API

3. **✅ Test Organization**: Already correct
   - C++ tests in Diagon: GoogleTest
   - Go tests in Quidditch: Go testing package

### When to Add Code

**Add to Diagon when:**
- Core search functionality (indexing, search, analysis)
- Performance-critical code (SIMD, compression)
- Reusable across multiple language bindings
- Belongs in library (not application)

**Add to Quidditch when:**
- Distributed system logic (Raft, cluster management)
- HTTP API and REST endpoints
- Query planning and routing
- Pipeline orchestration
- Application configuration
- Integration with external systems

**Add to c_api_src/ when:**
- Diagon C API is insufficient (rare)
- Need custom type conversion
- Temporary bridge until C API is extended
- **Always minimize - prefer extending Diagon C API**

---

## Examples

### ✅ CORRECT: Analyzer Framework

**Diagon** (C++):
```cpp
// src/core/include/analysis/Analyzer.h
class Analyzer {
    virtual std::vector<Token> analyze(const std::string& text) = 0;
};

// src/core/include/diagon/analysis_c.h (C API)
typedef struct diagon_analyzer_t diagon_analyzer_t;
diagon_analyzer_t* diagon_create_standard_analyzer(void);
diagon_token_array_t* diagon_analyze_text(diagon_analyzer_t* analyzer, ...);
```

**Quidditch** (Go):
```go
// pkg/data/diagon/analysis.go
type Analyzer struct {
    handle *C.diagon_analyzer_t
}

func NewStandardAnalyzer() (*Analyzer, error) {
    handle := C.diagon_create_standard_analyzer()
    return &Analyzer{handle: handle}, nil
}

func (a *Analyzer) Analyze(text string) ([]Token, error) {
    cText := C.CString(text)
    defer C.free(unsafe.Pointer(cText))
    cTokens := C.diagon_analyze_text(a.handle, cText, ...)
    // Convert C tokens to Go tokens
}
```

### ❌ WRONG: Go Tests in Diagon

```
# ❌ WRONG - Don't put .go files in Diagon
diagon/
└── src/core/src/analysis/
    ├── Analyzer.cpp          ✅ C++ implementation
    ├── analysis_c.cpp        ✅ C API
    └── analysis_test.go      ❌ WRONG - Go test in C++ repo!

# ✅ CORRECT - Go tests go in Quidditch
quidditch/
└── pkg/data/diagon/
    ├── analysis.go           ✅ Go binding
    └── analysis_test.go      ✅ Go test for binding
```

---

## Benefits of Clear Boundaries

1. **Clear Ownership**: Know where code belongs
2. **Language Consistency**: Each repo has one primary language
3. **Build Simplicity**: Separate build systems don't interfere
4. **Reusability**: Diagon can be used by non-Go projects
5. **Testing Clarity**: C++ tests for library, Go tests for application
6. **Maintainability**: Easier to navigate and understand
7. **Performance**: Minimize CGO overhead by proper layering

---

## Summary

| Aspect | Diagon | Quidditch |
|--------|--------|-----------|
| **Language** | 100% C++ | Go + thin C++ bridge |
| **Purpose** | Search engine library | Distributed application |
| **Tests** | C++ (GoogleTest) | Go (testing package) |
| **Build** | CMake | Go modules + CGO |
| **Output** | libdiagon_core.so | Executable binaries |
| **C API** | Provides C API | Consumes C API |
| **Location** | Git submodule | Main repo |

**Golden Rule**:
- Diagon = Pure C++ library with C API
- Quidditch = Go application using Diagon via CGO
- Bridge layer = Minimal, only type conversion

---

**Last Updated**: January 27, 2026
**Status**: Defined - Ready for enforcement
