# Document Retrieval Implementation Status

**Date**: 2026-01-26
**Component**: Diagon Document Retrieval Integration
**Status**: Implementation Complete, Testing in Progress

---

## Summary

Successfully integrated Diagon's new StoredFieldsReader API for document retrieval:
- ✅ Pulled latest Diagon upstream changes (24,306 lines, 123 files)
- ✅ Rebuilt Diagon C++ library with StoredFieldsReader support
- ✅ Rebuilt C API wrapper (libdiagon.so)
- ✅ Implemented GetDocument in bridge.go using new C API
- ✅ Data node binary compiles successfully
- ⚠️ Document retrieval returns `found: false` (debugging in progress)

---

## Implementation Details

### Diagon Upstream Changes

Pulled latest changes including:
```
pkg/data/diagon/upstream/src/core/include/diagon/codecs/StoredFieldsReader.h
pkg/data/diagon/upstream/src/core/include/diagon/codecs/StoredFieldsWriter.h
pkg/data/diagon/upstream/src/core/src/codecs/StoredFieldsReader.cpp
pkg/data/diagon/upstream/src/core/src/codecs/StoredFieldsWriter.cpp
```

### New C API Functions

Added to `c_api_src/diagon_c_api.h`:
```c
// Get stored document by ID
DiagonDocument diagon_reader_get_document(DiagonIndexReader reader, int doc_id);

// Get field value from document
bool diagon_document_get_field_value(DiagonDocument doc, const char* field_name,
                                     char* out_value, size_t out_value_len);

// Get numeric field values
bool diagon_document_get_long_value(DiagonDocument doc, const char* field_name,
                                    int64_t* out_value);
bool diagon_document_get_double_value(DiagonDocument doc, const char* field_name,
                                      double* out_value);
```

### GetDocument Implementation

Location: `pkg/data/diagon/bridge.go` lines 431-526

**Algorithm**:
1. Search for document by `_id` field to get internal doc ID
2. Retrieve stored fields using `diagon_reader_get_document()`
3. Extract field values using `diagon_document_get_field_value()`
4. Return map of field name → value

**Code Structure**:
```go
func (s *Shard) GetDocument(docID string) (map[string]interface{}, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Ensure reader is initialized
    if s.reader == nil {
        return nil, fmt.Errorf("reader not initialized")
    }

    // Search for the document by _id field to get internal doc ID
    // Create term query for _id field
    cIDField := C.CString("_id")
    cDocID := C.CString(docID)
    term := C.diagon_create_term(cIDField, cDocID)
    query := C.diagon_create_term_query(term)

    // Execute search to find internal doc ID
    topDocs := C.diagon_search(s.searcher, query, 1)
    totalHits := int64(C.diagon_top_docs_total_hits(topDocs))

    if totalHits == 0 {
        return nil, fmt.Errorf("document not found")
    }

    scoreDoc := C.diagon_top_docs_score_doc_at(topDocs, 0)
    internalDocID := int(C.diagon_score_doc_get_doc(scoreDoc))

    // Retrieve stored fields using reader
    diagonDoc := C.diagon_reader_get_document(s.reader, C.int(internalDocID))

    // Extract fields from Diagon document
    doc := make(map[string]interface{})

    // Get _id field
    idBuf := make([]byte, 1024)
    if C.diagon_document_get_field_value(diagonDoc, cIDFieldName,
        (*C.char)(unsafe.Pointer(&idBuf[0])), C.size_t(len(idBuf))) {
        // Extract string value
        doc["_id"] = string(idBuf[:nullIdx])
    }

    return doc, nil
}
```

---

## Build Process

### Diagon Library Build
```bash
cd /home/ubuntu/quidditch/pkg/data/diagon/upstream
rm -rf build && mkdir build && cd build
cmake ..
make -j$(nproc)
```

**Result**: `libdiagon_core.so` (934K)

### C API Wrapper Build
```bash
cd /home/ubuntu/quidditch/pkg/data/diagon
./build_c_api.sh
```

**Result**: `libdiagon.so` (88K)

### Data Node Build
```bash
cd /home/ubuntu/quidditch
CGO_LDFLAGS="-Wl,-rpath-link,/home/ubuntu/quidditch/pkg/data/diagon/upstream/build/src/core" \
go build -o bin/quidditch-data-new cmd/data/main.go
```

**Result**: `quidditch-data-new` (23M)

---

## Testing

### Test Environment
- Master: port 9400
- Data: port 9500
- Coordination: port 9200

### Test Results

#### Document Indexing ✅
```bash
curl -X PUT http://localhost:9200/products/_doc/laptop-001 \
  -H 'Content-Type: application/json' \
  -d '{"title": "Dell XPS 15 Laptop", "price": 1299.99}'

Response: {"result": "created", "_version": 1}  # SUCCESS
```

#### Document Retrieval ❌
```bash
curl http://localhost:9200/products/_doc/laptop-001

Response: {
  "_id": "laptop-001",
  "_index": "products",
  "found": false,          # ISSUE
  "_source": {},           # ISSUE
  "_version": 0            # ISSUE
}
```

---

## Current Issue: Document Retrieval Returns `found: false`

### Symptoms
- Documents index successfully (201 Created)
- Retrieval returns `found: false` with empty `_source`
- No `GetDocument` RPC calls appear in logs
- No errors in data node logs

### Diagnosis Flow

**Request Path**:
1. Client → Coordination node (`handleGetDocument`)
2. Coordination → DocRouter (`RouteGetDocument`)
3. DocRouter → Data node client (`client.GetDocument`)
4. Data node gRPC service (`DataService.GetDocument`)
5. Shard (`shard.GetDocument`)
6. Diagon bridge (`DiagonShard.GetDocument`)

### Evidence

1. **No RPC calls logged**:
```bash
grep -i "getdocument" /tmp/quidditch-docret-manual/logs/data.log
# Result: No matches
```

The gRPC service logs "GetDocument request" at DEBUG level, but this never appears. This means the RPC is never being invoked.

2. **Response pattern**:
```json
{"found": false, "_version": 0, "_source": {}}
```

This matches grpc_service.go line 214-217:
```go
// Get document
doc, err := shard.GetDocument(ctx, req.DocId)
if err != nil {
    // Document not found
    return &pb.GetDocumentResponse{
        Found: false,
        DocId: req.DocId,
    }, nil
}
```

The error is caught and converted to `found: false` instead of propagating.

### Possible Root Causes

1. **Reader not initialized**: Line 312-319 in bridge.go checks if `s.reader == nil` and initializes it. If this fails silently, GetDocument would error.

2. **Search by _id failing**: The implementation searches for the document by _id field to get the internal doc ID. If this search finds no results, GetDocument returns "document not found" error.

3. **_id field not indexed**: When documents are indexed via IndexDocument (bridge.go line 155-238), the _id field is added as a StringField. If this field isn't properly indexed, searches won't find it.

4. **Reader/Searcher synchronization**: Line 273-304 in bridge.go shows that Refresh() commits changes and reopens the reader. If documents are indexed but the reader isn't refreshed, they won't be searchable.

5. **Coordination node routing issue**: The coordination node might not be routing to the correct shard or data node.

---

## Next Steps

### Immediate (Debug Mode)

1. **Add comprehensive logging to GetDocument**:
```go
func (s *Shard) GetDocument(docID string) (map[string]interface{}, error) {
    s.logger.Info("GetDocument called", zap.String("doc_id", docID))

    s.mu.RLock()
    defer s.mu.RUnlock()

    if s.reader == nil {
        s.logger.Warn("Reader not initialized, initializing now")
        // Initialize reader...
    }

    s.logger.Debug("Searching for document by _id field")
    // ... rest of implementation with detailed logging ...
}
```

2. **Check reader initialization**:
   - Add logs when reader is opened (line 290, 314)
   - Verify reader state before GetDocument

3. **Test _id field indexing**:
   - Verify _id field is in the index
   - Check if term query for _id works
   - Try searching for indexed documents

4. **Force reader refresh**:
   - Call Refresh() after indexing documents
   - Verify commit happens before search

### Short Term (Fix)

1. **Implement explicit Refresh after indexing**
2. **Add reader state validation**
3. **Improve error handling and propagation**
4. **Add field enumeration to retrieve all fields (not just _id)**

### Long Term (Enhancement)

1. **Add C API for field enumeration**
2. **Optimize document retrieval (cache reader state)**
3. **Add comprehensive test suite**
4. **Document GetDocument limitations and requirements**

---

## Files Modified

1. **pkg/data/diagon/bridge.go** (lines 431-526): GetDocument implementation
2. **pkg/data/diagon/upstream/**: Pulled latest with StoredFieldsReader
3. **pkg/data/diagon/build/libdiagon.so**: Rebuilt C API wrapper
4. **bin/quidditch-data-new**: New binary with GetDocument

## Build Commands

```bash
# Clean rebuild Diagon
cd /home/ubuntu/quidditch/pkg/data/diagon/upstream
rm -rf build && mkdir build && cd build && cmake .. && make -j$(nproc)

# Rebuild C API wrapper
cd /home/ubuntu/quidditch/pkg/data/diagon && ./build_c_api.sh

# Rebuild data node
cd /home/ubuntu/quidditch
CGO_LDFLAGS="-Wl,-rpath-link,$(pwd)/pkg/data/diagon/upstream/build/src/core" \
go build -o bin/quidditch-data-new cmd/data/main.go
```

## Runtime Requirements

```bash
# Set LD_LIBRARY_PATH when running data node
export LD_LIBRARY_PATH="/home/ubuntu/quidditch/pkg/data/diagon/build:/home/ubuntu/quidditch/pkg/data/diagon/upstream/build/src/core:$LD_LIBRARY_PATH"

./bin/quidditch-data-new --config config/data.yaml
```

---

## References

- **Diagon Documentation**: `pkg/data/diagon/upstream/benchmarks/DOCUMENT_RETRIEVAL.md`
- **StoredFields API**: `pkg/data/diagon/upstream/src/core/include/diagon/codecs/StoredFieldsReader.h`
- **Phase 4 Limitations**: `DIAGON_PHASE4_LIMITATIONS.md`
- **Shard Loading Test**: `test/test_shard_loading.sh` (✅ PASSED)

---

**Last Updated**: 2026-01-26 23:40 UTC
**Status**: IN PROGRESS - Debugging document retrieval

