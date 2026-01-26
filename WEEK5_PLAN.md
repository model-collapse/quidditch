# Week 5 Plan: C++ Indexing Integration

**Dates**: Week of 2026-01-26 (continued)
**Status**: Planning
**Goal**: Complete C++ indexing integration for production-ready full-text search

---

## Week 5 Overview

Replace the Go stub mode with production C++ indexing using the Diagon library. Enable real full-text search, BM25 scoring, and term statistics. Complete the integration between Go query layer and C++ indexing engine.

**Target**: 1,400 lines of code
**Focus**: C++ ↔ Go integration via CGO

---

## Week 5 Days

### Day 1: CGO Bridge Implementation ✅ (Planned)
**Goal**: Implement CGO bridge between Go and C++ Diagon library

**Tasks**:
- [ ] Design CGO interface (C-compatible wrapper)
- [ ] Implement C wrapper functions for Diagon
- [ ] Create Go CGO bindings
- [ ] Handle memory management (Go ↔ C++)
- [ ] Error handling across language boundary
- [ ] Basic integration tests

**Deliverables**:
- `pkg/data/diagon/bridge.c` - C wrapper functions
- `pkg/data/diagon/bridge.h` - C header file
- `pkg/data/diagon/bridge.go` - Go CGO bindings (replace stub)
- `pkg/data/diagon/bridge_test.go` - Integration tests

**Estimated Lines**: 400 lines

### Day 2: Document Indexing
**Goal**: Implement document indexing via C++ backend

**Tasks**:
- [ ] Document serialization (Go → C++)
- [ ] Field mapping and schema
- [ ] Indexing pipeline
- [ ] Batch indexing support
- [ ] Index statistics
- [ ] Integration tests

**Deliverables**:
- C++ indexing functions
- Go indexing API
- Schema management
- Test suite

**Estimated Lines**: 350 lines

### Day 3: Search Implementation
**Goal**: Implement search via C++ backend with BM25 scoring

**Tasks**:
- [ ] Query translation (Go → C++)
- [ ] BM25 scoring integration
- [ ] Result retrieval and deserialization
- [ ] Snippet generation
- [ ] Search statistics
- [ ] Performance benchmarks

**Deliverables**:
- C++ search functions
- Go search API
- Result handling
- Benchmarks

**Estimated Lines**: 350 lines

### Day 4: Advanced Features & Testing
**Goal**: Complete integration with advanced features

**Tasks**:
- [ ] Phrase queries
- [ ] Proximity search
- [ ] Faceting support
- [ ] Sorting options
- [ ] End-to-end testing
- [ ] Performance optimization

**Deliverables**:
- Advanced query support
- Complete test suite
- Performance tuning
- Documentation

**Estimated Lines**: 300 lines

---

## Technical Approach

### CGO Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Go Layer                            │
│  - Query parsing                                        │
│  - Result aggregation                                   │
│  - UDF filtering                                        │
└─────────────────────┬───────────────────────────────────┘
                      │ CGO Interface
┌─────────────────────▼───────────────────────────────────┐
│                   C Wrapper                             │
│  - Type conversion (Go ↔ C++)                          │
│  - Memory management                                    │
│  - Error handling                                       │
└─────────────────────┬───────────────────────────────────┘
                      │ C++ calls
┌─────────────────────▼───────────────────────────────────┐
│                 Diagon Library                          │
│  - Document indexing                                    │
│  - Full-text search                                     │
│  - BM25 scoring                                         │
│  - Term statistics                                      │
└─────────────────────────────────────────────────────────┘
```

### Memory Management Strategy

1. **Go → C++**:
   - Allocate in Go
   - Pass pointers via CGO
   - C++ reads (no ownership transfer)
   - Go frees after call

2. **C++ → Go**:
   - C++ allocates
   - Return via C wrapper
   - Go copies data
   - C wrapper frees C++ memory

3. **Persistent Objects**:
   - C++ index handle stored in Go
   - Go tracks lifetime
   - Explicit close/cleanup

### Error Handling

```go
// C returns int error codes
// 0 = success, negative = error

func IndexDocument(indexID int64, doc *Document) error {
    result := C.diagon_index_document(C.int64_t(indexID), ...)
    if result != 0 {
        return fmt.Errorf("indexing failed: code %d", result)
    }
    return nil
}
```

---

## Success Criteria

### Week 5 Goals

- [ ] Complete CGO bridge implementation
- [ ] Document indexing working
- [ ] Search with BM25 scoring
- [ ] All existing tests pass with C++ backend
- [ ] Performance ≥ stub mode
- [ ] No memory leaks
- [ ] Clear error messages
- [ ] Complete integration tests

### Performance Targets

- **Indexing**: >10,000 docs/sec
- **Search**: <10ms for simple queries
- **Memory**: <100MB overhead per index
- **CPU**: <50% during idle

### Quality Targets

- All tests passing (100%)
- No memory leaks (valgrind clean)
- Thread-safe operations
- Graceful error handling
- Complete documentation

---

## Risks and Mitigations

### Risk 1: CGO Performance Overhead
**Impact**: High
**Mitigation**:
- Batch operations where possible
- Minimize cross-language calls
- Cache frequently accessed data
- Use profiling to identify bottlenecks

### Risk 2: Memory Management Complexity
**Impact**: High
**Mitigation**:
- Clear ownership rules
- Systematic testing with valgrind
- RAII patterns in C++
- Defer cleanup in Go

### Risk 3: Build Complexity
**Impact**: Medium
**Mitigation**:
- Document build requirements
- Provide build scripts
- Use Docker for consistent environment
- CI/CD integration

### Risk 4: Debugging Across Languages
**Impact**: Medium
**Mitigation**:
- Comprehensive logging
- Separate unit tests per layer
- Integration test suite
- GDB + Delve debugging

---

## Dependencies

### Build Tools
- Go 1.21+
- GCC/Clang with C++17 support
- CGO enabled
- Make

### Libraries
- Diagon (C++ indexing library)
- Google Test (C++ testing)
- testify (Go testing)

### Development
- valgrind (memory leak detection)
- gdb (C++ debugging)
- delve (Go debugging)
- pprof (profiling)

---

## Deliverables Summary

### Code
- CGO bridge: ~400 lines
- Indexing: ~350 lines
- Search: ~350 lines
- Advanced features: ~300 lines
- **Total**: ~1,400 lines

### Tests
- CGO bridge tests: ~150 lines
- Indexing tests: ~150 lines
- Search tests: ~150 lines
- Integration tests: ~200 lines
- **Total**: ~650 lines

### Documentation
- CGO integration guide: ~300 lines
- API documentation updates: ~200 lines
- Build instructions: ~100 lines
- **Total**: ~600 lines

---

## Timeline

**Week 5 Schedule**:
- Day 1 (Mon): CGO bridge implementation
- Day 2 (Tue): Document indexing
- Day 3 (Wed): Search implementation
- Day 4 (Thu): Advanced features & testing

**Checkpoint**: End of Day 2 - Basic indexing working
**Milestone**: End of Day 3 - Search working with BM25
**Goal**: End of Day 4 - Complete integration

---

## Next Steps

1. Review Diagon C++ API
2. Design CGO interface
3. Implement C wrapper layer
4. Create Go bindings
5. Test basic functionality
6. Iterate and optimize

---

**Let's build production-ready indexing!** 🚀
