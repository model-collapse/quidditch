# Architecture Cleanup Plan

## Current State Analysis

### ✅ What's Currently Correct

1. **No Go in Diagon**: ✅ Verified - no `.go` files in upstream
2. **Go bindings in Quidditch**: ✅ Correct location (`pkg/data/diagon/*.go`)
3. **Go tests in Quidditch**: ✅ Correct (`*_test.go` in Quidditch)
4. **Analyzer C API in Diagon**: ✅ `analysis_c.h` properly in upstream

### ⚠️ What Needs Fixing

#### **Problem 1: Main C API in Wrong Location** 🔴 HIGH PRIORITY

**Current**:
```
quidditch/pkg/data/diagon/c_api_src/
├── diagon_c_api.h          ❌ Should be in Diagon upstream
├── diagon_c_api.cpp        ❌ Should be in Diagon upstream
├── minimal_wrapper.cpp     ⚠️ Review - might be OK as bridge
├── minimal_wrapper.h       ⚠️ Review - might be OK as bridge
├── MatchAllQuery.cpp       ❌ Should be in Diagon upstream
└── MatchAllQuery.h         ❌ Should be in Diagon upstream
```

**Problem**: The main Diagon C API (`diagon_c_api.{h,cpp}`) is in Quidditch, but it should be in Diagon upstream. This API is library functionality, not application bridge code.

**Impact**:
- Other language bindings (Python, Rust, etc.) can't use Diagon easily
- Unclear separation of library vs application
- Duplicate effort if multiple consumers need C API

#### **Problem 2: Query Implementation in Bridge Layer** 🔴 HIGH PRIORITY

**Current**:
```cpp
// In quidditch/pkg/data/diagon/c_api_src/MatchAllQuery.cpp
class MatchAllDocsQuery : public Query { ... }
```

**Problem**: Core query functionality (MatchAllDocsQuery) is in the bridge layer instead of Diagon core.

**Impact**:
- Core search functionality in wrong place
- Can't be reused by other bindings
- Not testable with C++ tests

---

## Cleanup Actions

### Phase 1: Move C API to Diagon (Priority: HIGH)

**Goal**: Move `diagon_c_api.{h,cpp}` from Quidditch bridge to Diagon upstream.

#### Step 1.1: Create Diagon C API Structure

**Location**: `diagon/src/core/src/c_api/`

```bash
cd /home/ubuntu/quidditch/pkg/data/diagon/upstream

# Create C API directory
mkdir -p src/core/src/c_api
mkdir -p src/core/include/diagon/c_api

# Plan:
# - Move diagon_c_api.h → src/core/include/diagon/c_api/diagon_c_api.h
# - Move diagon_c_api.cpp → src/core/src/c_api/diagon_c_api.cpp
# - Update CMakeLists.txt to include c_api sources
```

#### Step 1.2: Move MatchAllQuery to Diagon Core

```bash
# Move from bridge to core
# quidditch/pkg/data/diagon/c_api_src/MatchAllQuery.{h,cpp}
#   → diagon/src/core/include/diagon/search/MatchAllDocsQuery.h
#   → diagon/src/core/src/search/MatchAllDocsQuery.cpp
```

#### Step 1.3: Update CMakeLists.txt

```cmake
# In diagon/src/core/CMakeLists.txt
set(DIAGON_CORE_SOURCES
    # ... existing sources ...

    # C API
    src/c_api/diagon_c_api.cpp

    # Search
    src/search/MatchAllDocsQuery.cpp
)

set(DIAGON_CORE_HEADERS
    # ... existing headers ...

    # C API
    include/diagon/c_api/diagon_c_api.h

    # Search
    include/diagon/search/MatchAllDocsQuery.h
)
```

#### Step 1.4: Update Quidditch to Use Moved C API

```go
// In pkg/data/diagon/bridge.go
/*
#cgo CFLAGS: -I${SRCDIR}/upstream/src/core/include
#cgo LDFLAGS: -L${SRCDIR}/upstream/build/src/core -ldiagon_core ...
#include "diagon/c_api/diagon_c_api.h"  // Changed path
*/
```

**Files to Move**: 2,263 lines
**Estimated Time**: 2-3 hours

---

### Phase 2: Keep Only Minimal Bridge (Priority: MEDIUM)

**Goal**: Reduce bridge layer to absolute minimum - only type conversion helpers.

#### Step 2.1: Review minimal_wrapper.{h,cpp}

**Current Size**: ~8K (6K cpp + 2K h)

**Questions to Answer**:
1. Does it duplicate Diagon C API functionality? → Move to Diagon
2. Is it pure type conversion (C ↔ Go)? → Keep in bridge
3. Is it application-specific? → Move to Go if possible

```bash
# Review file
cat /home/ubuntu/quidditch/pkg/data/diagon/c_api_src/minimal_wrapper.cpp

# Decision tree:
# - If duplicates C API → DELETE (use Diagon C API)
# - If pure conversion → KEEP (needed for CGO)
# - If application logic → MOVE to Go
```

#### Step 2.2: Document Bridge Layer Contract

Create `pkg/data/diagon/c_api_src/README.md`:

```markdown
# Diagon CGO Bridge Layer

**Purpose**: Minimal C++ bridge code for Go ↔ Diagon C API integration.

**Guidelines**:
- Keep this layer as THIN as possible
- Only type conversion helpers
- No business logic
- No duplicate C API functionality
- Prefer extending Diagon C API over adding code here

**When to Add Code Here**:
1. CGO-specific type conversions (Go string ↔ C char*)
2. Temporary bridges until Diagon C API is extended
3. Platform-specific CGO workarounds

**When NOT to Add Code Here**:
- ❌ Search functionality (goes in Diagon)
- ❌ Query implementations (goes in Diagon)
- ❌ Application logic (goes in Go)
- ❌ Anything reusable by other languages (goes in Diagon C API)
```

**Estimated Time**: 1-2 hours

---

### Phase 3: Standardize C API Pattern (Priority: LOW)

**Goal**: Ensure all Diagon C APIs follow same pattern as `analysis_c.h`.

#### Step 3.1: Review Existing C APIs

```bash
# Check what C APIs already exist in Diagon
find /home/ubuntu/quidditch/pkg/data/diagon/upstream \
  -name "*_c.h" -o -name "*_c.cpp"

# Currently: only analysis_c.{h,cpp}
```

#### Step 3.2: Establish C API Naming Convention

**Pattern** (from `analysis_c.h`):
```c
// Header: include/diagon/<module>/<module>_c.h
// Implementation: src/<module>/<module>_c.cpp

// Naming:
// - Types: diagon_<module>_<type>_t (opaque struct)
// - Functions: diagon_<module>_<action>()
// - Enums: DIAGON_<MODULE>_<VALUE>

// Example:
typedef struct diagon_analyzer_t diagon_analyzer_t;
diagon_analyzer_t* diagon_create_standard_analyzer(void);
void diagon_destroy_analyzer(diagon_analyzer_t* analyzer);
```

#### Step 3.3: Refactor Main C API

**Before** (in bridge):
```c
// diagon_c_api.h
typedef void* DiagonDirectory;  // Bad: void*, unclear naming
DiagonDirectory diagon_open_directory(const char* path);
```

**After** (in Diagon):
```c
// diagon/c_api/directory_c.h
typedef struct diagon_directory_t diagon_directory_t;
diagon_directory_t* diagon_directory_open(const char* path);
void diagon_directory_close(diagon_directory_t* dir);
```

**Estimated Time**: 3-4 hours (refactoring)

---

## Detailed Migration Steps

### Step-by-Step: Move C API to Diagon

#### A. Preparation (5 minutes)

```bash
cd /home/ubuntu/quidditch/pkg/data/diagon/upstream

# Create target directories
mkdir -p src/core/src/c_api
mkdir -p src/core/include/diagon/c_api

# Create branch for changes
git checkout -b move-c-api-to-upstream
```

#### B. Move Files (10 minutes)

```bash
# Copy C API files from Quidditch to Diagon
cp /home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.h \
   src/core/include/diagon/c_api/

cp /home/ubuntu/quidditch/pkg/data/diagon/c_api_src/diagon_c_api.cpp \
   src/core/src/c_api/

# Copy MatchAllQuery to proper location
cp /home/ubuntu/quidditch/pkg/data/diagon/c_api_src/MatchAllQuery.h \
   src/core/include/diagon/search/MatchAllDocsQuery.h

cp /home/ubuntu/quidditch/pkg/data/diagon/c_api_src/MatchAllQuery.cpp \
   src/core/src/search/MatchAllDocsQuery.cpp
```

#### C. Update CMakeLists.txt (10 minutes)

```cmake
# Edit src/core/CMakeLists.txt
# Add to DIAGON_CORE_SOURCES:
src/c_api/diagon_c_api.cpp
src/search/MatchAllDocsQuery.cpp

# Add to DIAGON_CORE_HEADERS:
include/diagon/c_api/diagon_c_api.h
include/diagon/search/MatchAllDocsQuery.h
```

#### D. Update Include Paths (20 minutes)

```cpp
// In diagon_c_api.cpp, update includes:
#include "diagon/c_api/diagon_c_api.h"
#include "diagon/search/MatchAllDocsQuery.h"
// ... etc
```

#### E. Build and Test (30 minutes)

```bash
# Build Diagon
cd /home/ubuntu/quidditch/pkg/data/diagon/upstream
cmake -B build -S . -DCMAKE_BUILD_TYPE=Debug
cmake --build build -j$(nproc)

# Run C++ tests
cd build && ctest --output-on-failure

# If tests pass, commit
git add -A
git commit -m "Move C API from Quidditch bridge to Diagon upstream"
```

#### F. Update Quidditch (30 minutes)

```bash
cd /home/ubuntu/quidditch

# Update CGO includes in pkg/data/diagon/bridge.go
# Change:
#include "diagon_c_api.h"
# To:
#include "diagon/c_api/diagon_c_api.h"

# Remove old bridge files
rm pkg/data/diagon/c_api_src/diagon_c_api.h
rm pkg/data/diagon/c_api_src/diagon_c_api.cpp
rm pkg/data/diagon/c_api_src/MatchAllQuery.h
rm pkg/data/diagon/c_api_src/MatchAllQuery.cpp

# Update Makefile if needed
# Build and test
go build ./cmd/...
go test ./pkg/data/diagon/...

# Commit
git add -A
git commit -m "Use Diagon C API from upstream, remove bridge duplicates"
```

**Total Estimated Time**: 2 hours

---

## Timeline

| Phase | Task | Priority | Time | Status |
|-------|------|----------|------|--------|
| 1 | Move C API to Diagon | 🔴 HIGH | 2h | ⏳ Not Started |
| 2 | Review bridge layer | 🟡 MEDIUM | 1-2h | ⏳ Not Started |
| 3 | Standardize C API | 🟢 LOW | 3-4h | ⏳ Not Started |
| **Total** | | | **6-8h** | |

---

## Expected Outcomes

### After Phase 1 (C API Move)

**Diagon Repository**:
```
diagon/
└── src/core/
    ├── include/diagon/
    │   ├── c_api/
    │   │   └── diagon_c_api.h       ✅ NEW - Main C API
    │   ├── analysis_c.h             ✅ Existing
    │   └── search/
    │       └── MatchAllDocsQuery.h  ✅ NEW - Moved from bridge
    └── src/
        ├── c_api/
        │   └── diagon_c_api.cpp     ✅ NEW - Main C API impl
        └── search/
            └── MatchAllDocsQuery.cpp ✅ NEW - Moved from bridge
```

**Quidditch Repository**:
```
quidditch/pkg/data/diagon/
├── analysis.go                      ✅ Go binding
├── bridge.go                        ✅ Go binding
├── c_api_src/
│   ├── minimal_wrapper.h            ⚠️ Keep if needed
│   ├── minimal_wrapper.cpp          ⚠️ Keep if needed
│   ├── diagon_c_api.h              ❌ REMOVED
│   ├── diagon_c_api.cpp            ❌ REMOVED
│   ├── MatchAllQuery.h             ❌ REMOVED
│   └── MatchAllQuery.cpp           ❌ REMOVED
└── upstream/                        ✅ Submodule (now includes C API)
```

**Bridge Layer Reduction**:
- Before: 2,263 lines
- After: ~400 lines (only minimal_wrapper if still needed)
- **Reduction**: ~82% 🎉

### Benefits

1. **✅ Clear Separation**: Diagon = library, Quidditch = application
2. **✅ Reusability**: Other languages can use Diagon C API
3. **✅ Maintainability**: C API tested with C++ tests in Diagon
4. **✅ Smaller Bridge**: Quidditch bridge layer minimal
5. **✅ Proper Layering**: Library functionality in library repo

---

## Rollback Plan

If cleanup causes issues:

```bash
# Diagon
cd /home/ubuntu/quidditch/pkg/data/diagon/upstream
git checkout main  # Revert to before cleanup

# Quidditch
cd /home/ubuntu/quidditch
git checkout main  # Revert to before cleanup
```

---

## Next Actions

1. **Review this plan** with team/user
2. **Execute Phase 1** (2 hours)
   - Move C API to Diagon
   - Test thoroughly
   - Commit changes
3. **Execute Phase 2** (1-2 hours)
   - Review and minimize bridge layer
4. **(Optional) Execute Phase 3** (3-4 hours)
   - Standardize C API naming

**Total Time Investment**: 6-8 hours for complete cleanup

---

**Created**: January 27, 2026
**Status**: Plan Ready - Awaiting Approval
