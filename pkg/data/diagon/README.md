# Diagon CGO Bindings

This package provides Go bindings to the Diagon C++ search engine library via CGO.

## Architecture

**Diagon Repository** (100% C++):
- Location: `upstream/` (git submodule to github.com/model-collapse/diagon)
- Contains: C++ implementation + C API
- C API headers: `upstream/src/core/include/diagon/c_api/`
- Built library: `upstream/build/src/core/libdiagon_core.so`

**Quidditch Repository** (Go + CGO):
- Go bindings: `*.go` files in this directory
- CGO configuration: Imports C API from `upstream/src/core/include/`
- No C++ bridge code (all C API in Diagon upstream)

## Directory Structure

```
pkg/data/diagon/
├── README.md              # This file
├── upstream/              # Git submodule → github.com/model-collapse/diagon
│   └── src/core/
│       ├── include/diagon/c_api/
│       │   └── diagon_c_api.h       # Main C API
│       └── build/src/core/
│           └── libdiagon_core.so    # Built library
├── analysis.go            # Go wrapper for text analysis
├── analysis_test.go       # Tests for analysis
├── analyzer_settings.go   # Analyzer configuration
├── bridge.go              # Main Go-to-C++ bridge
├── bridge_test.go         # Bridge tests
└── shard.go               # Shard management

NO C++ CODE IN QUIDDITCH - All C++ code is in Diagon upstream
```

## Building

The C++ library is built via CMake in the Diagon submodule:

```bash
cd upstream
cmake -B build -S . -DCMAKE_BUILD_TYPE=Release
cmake --build build -j$(nproc)
```

The Go bindings automatically link against the built library:

```bash
go build ./cmd/data
```

CGO configuration is in `bridge.go`:
```go
/*
#cgo CFLAGS: -I${SRCDIR}/upstream/src/core/include
#cgo LDFLAGS: -L${SRCDIR}/upstream/build/src/core -ldiagon_core ...
#include "diagon/c_api/diagon_c_api.h"
*/
```

## Design Principles

1. **No C++ in Quidditch**: All C++ code belongs in Diagon upstream
2. **C API Boundary**: Go never calls C++ directly, always via C API
3. **Minimal Bridge**: Keep Go bindings thin - just type conversion
4. **Proper Layering**: Diagon = library, Quidditch = application

## Migration History

**January 27, 2026**: Completed architecture cleanup
- Moved `diagon_c_api.{h,cpp}` from Quidditch to Diagon upstream
- Moved `MatchAllQuery` to Diagon core
- Removed obsolete stub implementations (`minimal_wrapper`)
- Removed `c_api_src/` directory (empty after cleanup)
- Bridge layer reduced from 2,263 lines to 0 lines (100% reduction)
- All C API functionality now properly in Diagon library

**Result**: Clean separation - Diagon = 100% C++, Quidditch = Go + CGO bindings

## See Also

- [Repository Architecture](../../../REPOSITORY_ARCHITECTURE.md) - Defines boundaries between Diagon and Quidditch
- [Architecture Cleanup Plan](../../../ARCHITECTURE_CLEANUP_PLAN.md) - Migration plan executed
- [Diagon README](upstream/README.md) - Upstream library documentation
