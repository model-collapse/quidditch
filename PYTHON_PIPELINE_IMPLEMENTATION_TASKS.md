# Python Pipeline Framework - Implementation Tasks
## Decision: Python → WASM Approach
## Date: January 27, 2026

---

## Task Overview

**Total Estimated Time**: 2-3 days (16-24 hours)
**Complexity**: Medium (leveraging existing WASM infrastructure)
**Dependencies**: Phase 2 complete (WASM runtime, UDF system)

---

## Phase 3.1: Core Pipeline Framework (Day 1 - 8 hours)

### Task 1: Define Core Types and Interfaces
**File**: `pkg/coordination/pipeline/types.go`
**Status**: ✅ **COMPLETE** (already created)
**Time**: 0 hours (done)

**What's included**:
- Pipeline, Stage, StageContext interfaces
- PipelineType, StageType enums
- PipelineDefinition, StageDefinition structs
- Error types (PipelineError, ValidationError)
- Statistics tracking (PipelineStats, StageStats)

---

### Task 2: Build Pipeline Registry
**File**: `pkg/coordination/pipeline/registry.go`
**Time**: 3 hours
**Dependencies**: types.go

**Requirements**:
```go
type Registry struct {
    pipelines      map[string]*pipelineImpl  // name -> pipeline
    indexPipelines map[string]map[PipelineType]string  // index -> type -> pipeline name
    stats          map[string]*PipelineStats
    mu             sync.RWMutex
    logger         *zap.Logger
}

// Methods to implement:
func NewRegistry(logger *zap.Logger) *Registry
func (r *Registry) Register(def *PipelineDefinition) error
func (r *Registry) Get(name string) (Pipeline, error)
func (r *Registry) List(filterType PipelineType) []*PipelineDefinition
func (r *Registry) Unregister(name string) error
func (r *Registry) AssociatePipeline(indexName string, pipelineType PipelineType, pipelineName string) error
func (r *Registry) GetPipelineForIndex(indexName string, pipelineType PipelineType) (Pipeline, error)
func (r *Registry) GetStats(name string) (*PipelineStats, error)
```

**Validation logic**:
- Pipeline name uniqueness
- Stage name uniqueness within pipeline
- Valid pipeline and stage types
- Required fields present
- Stage config validation (udf_name for python stages)

**Test coverage**:
- Register valid pipeline
- Register duplicate (should fail)
- Get non-existent pipeline
- Associate pipeline with index
- List filtered by type

---

### Task 3: Implement Pipeline Executor
**File**: `pkg/coordination/pipeline/executor.go`
**Time**: 2 hours
**Dependencies**: types.go, registry.go

**Requirements**:
```go
type pipelineImpl struct {
    def    *PipelineDefinition
    stages []Stage
    logger *zap.Logger
}

func (p *pipelineImpl) Execute(ctx context.Context, input interface{}) (interface{}, error) {
    // 1. Validate input type matches pipeline type
    // 2. Create execution context
    // 3. Execute stages sequentially
    // 4. Handle errors per stage failure policy
    // 5. Track metrics
    // 6. Return final output
}

func (p *pipelineImpl) executeStage(stage Stage, ctx *StageContext, input interface{}) (interface{}, error) {
    // 1. Start timer
    // 2. Apply timeout if configured
    // 3. Execute stage
    // 4. Record metrics
    // 5. Handle errors
}
```

**Error handling**:
- FailurePolicyContinue: Log error, return original input
- FailurePolicyAbort: Stop execution, return error
- FailurePolicyRetry: Retry up to N times (configurable)

**Metrics**:
- Execution count per pipeline
- Success/failure count
- Duration percentiles (P50, P95, P99)
- Per-stage metrics

**Test coverage**:
- Happy path (all stages succeed)
- Stage failure with continue policy
- Stage failure with abort policy
- Timeout handling
- Metrics collection

---

### Task 4: Create Python Stage Adapter
**File**: `pkg/coordination/pipeline/stages/python_stage.go`
**Time**: 3 hours
**Dependencies**: executor.go, existing pkg/wasm/* infrastructure

**Requirements**:
```go
type PythonStage struct {
    name       string
    udfName    string
    udfVersion string
    parameters map[string]interface{}
    registry   *wasm.Registry
    logger     *zap.Logger
}

func NewPythonStage(name string, config map[string]interface{}, udfRegistry *wasm.Registry, logger *zap.Logger) (*PythonStage, error) {
    // Parse config:
    //   - udf_name (required)
    //   - udf_version (optional, default "latest")
    //   - parameters (optional)
}

func (s *PythonStage) Execute(ctx *StageContext, input interface{}) (interface{}, error) {
    // 1. Convert input to DocumentContext for WASM
    //    - Query pipeline: SearchRequest → JSON → DocumentContext
    //    - Document pipeline: map[string]interface{} → DocumentContext
    //    - Result pipeline: SearchResult → JSON → DocumentContext
    // 2. Convert parameters to wasm.Value map
    // 3. Call UDF via registry
    // 4. Parse output and convert back to Go type
}
```

**Data conversion**:
- Query pipeline input/output: `parser.SearchRequest` as JSON
- Document pipeline input/output: `map[string]interface{}`
- Result pipeline input: `SearchResult` + original request
- Result pipeline output: Modified `SearchResult`

**Integration with existing WASM**:
```go
// Reuse existing infrastructure
func (s *PythonStage) Execute(ctx *StageContext, input interface{}) (interface{}, error) {
    // Convert input to DocumentContext
    docCtx := s.inputToDocumentContext(input)

    // Convert parameters to WASM values
    wasmParams := s.parametersToWasmValues()

    // Execute via existing registry
    result, err := s.registry.Call(s.udfName, s.udfVersion, docCtx, wasmParams)
    if err != nil {
        return nil, err
    }

    // Convert result back to expected output type
    return s.wasmResultToOutput(result, input)
}
```

**Test coverage**:
- Execute with valid UDF
- Execute with non-existent UDF
- Parameter passing
- Error handling
- Input/output conversion for each pipeline type

---

## Phase 3.2: HTTP API Integration (Day 1-2 - 4 hours)

### Task 5: Pipeline Management HTTP Handlers
**File**: `pkg/coordination/pipeline_handlers.go`
**Time**: 3 hours
**Dependencies**: registry.go, executor.go

**Endpoints to implement**:
```
POST   /_pipeline/{name}           - Create/update pipeline
GET    /_pipeline/{name}           - Get pipeline definition
DELETE /_pipeline/{name}           - Delete pipeline
GET    /_pipeline                  - List all pipelines
POST   /_pipeline/{name}/_execute  - Test pipeline with sample data
GET    /_pipeline/{name}/_stats    - Get pipeline statistics
```

**Handler structure** (similar to udf_handlers.go):
```go
type PipelineHandlers struct {
    registry *pipeline.Registry
    logger   *zap.Logger
}

func (h *PipelineHandlers) createPipeline(c *gin.Context)
func (h *PipelineHandlers) getPipeline(c *gin.Context)
func (h *PipelineHandlers) deletePipeline(c *gin.Context)
func (h *PipelineHandlers) listPipelines(c *gin.Context)
func (h *PipelineHandlers) executePipeline(c *gin.Context)
func (h *PipelineHandlers) getStats(c *gin.Context)
```

**Request/Response formats**:
```json
// POST /_pipeline/synonym-expansion
{
  "name": "synonym-expansion",
  "version": "1.0.0",
  "type": "query",
  "description": "Expands query with synonyms",
  "stages": [
    {
      "name": "expand",
      "type": "python",
      "config": {
        "udf_name": "synonym_expander",
        "udf_version": "1.0.0",
        "parameters": {
          "boost_original": 2.0,
          "boost_synonyms": 1.0
        }
      }
    }
  ]
}

// Response
{
  "acknowledged": true,
  "name": "synonym-expansion",
  "version": "1.0.0"
}
```

**Test coverage**:
- Create pipeline (valid)
- Create pipeline (invalid config)
- Get existing pipeline
- Get non-existent pipeline
- Delete pipeline
- List pipelines with filtering
- Execute test pipeline

---

### Task 6: Index Settings Integration
**File**: `pkg/coordination/indices_handler.go` (modify existing)
**Time**: 1 hour
**Dependencies**: pipeline_handlers.go

**New index settings**:
```json
{
  "settings": {
    "index": {
      "search": {
        "default_pipeline": "synonym-expansion"
      },
      "query": {
        "default_pipeline": "spell-correction"
      },
      "result": {
        "default_pipeline": "ml-reranking"
      }
    }
  }
}
```

**Modify**:
- PUT /{index}/_settings - Accept pipeline settings
- GET /{index}/_settings - Return pipeline settings
- Store in index metadata

**Test coverage**:
- Set index pipeline settings
- Get index pipeline settings
- Override with query parameter: `?pipeline=custom`

---

## Phase 3.3: Query/Document Pipeline Integration (Day 2 - 4 hours)

### Task 7: Query Pipeline Execution
**File**: `pkg/coordination/query_service.go` (modify existing)
**Time**: 2 hours
**Dependencies**: registry.go

**Integration point**:
```go
// In Search() method, before query execution
func (qs *QueryService) Search(ctx context.Context, indexName string, req *parser.SearchRequest) (*SearchResult, error) {
    // NEW: Execute query pipeline if configured
    if queryPipeline := qs.getQueryPipeline(indexName); queryPipeline != nil {
        modifiedReq, err := queryPipeline.Execute(ctx, req)
        if err != nil {
            qs.logger.Warn("Query pipeline failed",
                zap.String("index", indexName),
                zap.Error(err))
            // Continue with original request (graceful degradation)
        } else {
            req = modifiedReq.(*parser.SearchRequest)
        }
    }

    // Existing query execution logic...

    // NEW: Execute result pipeline if configured
    if resultPipeline := qs.getResultPipeline(indexName); resultPipeline != nil {
        modifiedResult, err := resultPipeline.Execute(ctx, result, req)
        if err != nil {
            qs.logger.Warn("Result pipeline failed",
                zap.String("index", indexName),
                zap.Error(err))
            // Continue with original results
        } else {
            result = modifiedResult.(*SearchResult)
        }
    }

    return result, nil
}
```

**Helper methods**:
```go
func (qs *QueryService) getQueryPipeline(indexName string) pipeline.Pipeline
func (qs *QueryService) getResultPipeline(indexName string) pipeline.Pipeline
```

**Test coverage**:
- Query with no pipeline (existing behavior)
- Query with pipeline that modifies query
- Query with pipeline that fails (graceful degradation)
- Result pipeline that re-ranks results

---

### Task 8: Document Pipeline Execution
**File**: `pkg/coordination/indices_handler.go` (modify existing)
**Time**: 2 hours
**Dependencies**: registry.go

**Integration point**:
```go
// In indexDocument() handler, before indexing
func (h *IndicesHandler) indexDocument(c *gin.Context) {
    // Parse document
    var doc map[string]interface{}
    c.BindJSON(&doc)

    // NEW: Execute document pipeline if configured
    if docPipeline := h.getDocumentPipeline(indexName); docPipeline != nil {
        modifiedDoc, err := docPipeline.Execute(c.Request.Context(), doc)
        if err != nil {
            c.JSON(500, gin.H{"error": fmt.Sprintf("Document pipeline failed: %v", err)})
            return
        }
        doc = modifiedDoc.(map[string]interface{})
    }

    // Existing indexing logic...
}
```

**Test coverage**:
- Index document without pipeline
- Index document with enrichment pipeline
- Index document with validation pipeline (that rejects)
- Index document with pipeline that fails

---

## Phase 3.4: Example Pipelines (Day 2-3 - 8 hours)

### Task 9: Synonym Expansion Pipeline
**Files**:
- `examples/pipelines/synonym-expansion/pipeline.json`
- `examples/pipelines/synonym-expansion/synonyms.py`
- `examples/pipelines/synonym-expansion/synonyms.json`
- `examples/pipelines/synonym-expansion/README.md`

**Time**: 3 hours

**Implementation**:
```python
# synonyms.py
"""
Synonym Expansion UDF
Expands query terms with synonyms
"""
# @udf: name=synonym_expander
# @udf: version=1.0.0
# @udf: category=query_pipeline

import json

SYNONYMS = {
    "laptop": ["notebook", "computer", "pc"],
    "phone": ["mobile", "smartphone", "cell"],
    "car": ["auto", "vehicle", "automobile"]
}

def udf_main():
    # Get original query as JSON string
    query_json = get_field_string("query")
    query = json.loads(query_json)

    # Expand with synonyms
    expanded_query = expand_query(query)

    # Return modified query as JSON
    return json.dumps(expanded_query)

def expand_query(query):
    """Expand match queries with synonyms using bool should"""
    if "match" in query:
        field = list(query["match"].keys())[0]
        value = query["match"][field]

        # Get synonyms
        synonyms = SYNONYMS.get(value.lower(), [])
        if not synonyms:
            return query  # No synonyms, return original

        # Convert to bool query with should clauses
        return {
            "bool": {
                "should": [
                    {"match": {field: {"query": value, "boost": 2.0}}},  # Original
                    *[{"match": {field: {"query": syn, "boost": 1.0}}} for syn in synonyms]
                ],
                "minimum_should_match": 1
            }
        }

    return query
```

**Test cases**:
```bash
# Original query
{"match": {"title": "laptop"}}

# After pipeline
{
  "bool": {
    "should": [
      {"match": {"title": {"query": "laptop", "boost": 2.0}}},
      {"match": {"title": {"query": "notebook", "boost": 1.0}}},
      {"match": {"title": {"query": "computer", "boost": 1.0}}},
      {"match": {"title": {"query": "pc", "boost": 1.0}}}
    ],
    "minimum_should_match": 1
  }
}
```

---

### Task 10: Spell Correction Pipeline
**Files**:
- `examples/pipelines/spell-correction/pipeline.json`
- `examples/pipelines/spell-correction/spell_checker.py`
- `examples/pipelines/spell-correction/dictionary.txt`
- `examples/pipelines/spell-correction/README.md`

**Time**: 3 hours

**Implementation**:
```python
# spell_checker.py
"""
Spell Correction UDF
Corrects spelling in query terms using edit distance
"""
# @udf: name=spell_corrector
# @udf: version=1.0.0
# @udf: category=query_pipeline

import json

# Simple dictionary (in production, load from file)
DICTIONARY = {"laptop", "computer", "phone", "mobile", "table", "chair"}

def udf_main():
    query_json = get_field_string("query")
    query = json.loads(query_json)

    corrected_query, corrected = correct_spelling(query)

    # Add metadata if correction occurred
    if corrected:
        corrected_query["_pipeline_metadata"] = {
            "spell_corrected": True,
            "original_query": query
        }

    return json.dumps(corrected_query)

def correct_spelling(query):
    """Correct spelling in query"""
    if "match" not in query:
        return query, False

    field = list(query["match"].keys())[0]
    value = query["match"][field]

    # Check each word
    words = value.split()
    corrected_words = []
    corrected = False

    for word in words:
        if word.lower() in DICTIONARY:
            corrected_words.append(word)
        else:
            # Find closest match
            closest = find_closest(word.lower())
            if closest:
                corrected_words.append(closest)
                corrected = True
            else:
                corrected_words.append(word)

    if corrected:
        query["match"][field] = " ".join(corrected_words)

    return query, corrected

def find_closest(word):
    """Find closest dictionary word using edit distance"""
    min_distance = float('inf')
    closest = None

    for dict_word in DICTIONARY:
        dist = levenshtein_distance(word, dict_word)
        if dist < min_distance:
            min_distance = dist
            closest = dict_word

    # Only return if reasonably close (distance <= 2)
    return closest if min_distance <= 2 else None

def levenshtein_distance(s1, s2):
    """Calculate edit distance"""
    # (Implementation from existing text_similarity.py)
    pass
```

---

### Task 11: ML Re-ranking Pipeline
**Files**:
- `examples/pipelines/ml-reranking/pipeline.json`
- `examples/pipelines/ml-reranking/reranker.py`
- `examples/pipelines/ml-reranking/model.onnx` (dummy for now)
- `examples/pipelines/ml-reranking/README.md`

**Time**: 2 hours

**Implementation** (simplified without actual ONNX):
```python
# reranker.py
"""
ML Re-ranking UDF
Re-ranks search results using learned features
"""
# @udf: name=ml_reranker
# @udf: version=1.0.0
# @udf: category=result_pipeline

import json

def udf_main():
    # Get search results as JSON
    results_json = get_field_string("results")
    results = json.loads(results_json)

    # Get original query
    query_json = get_field_string("query")
    query = json.loads(query_json)

    # Re-rank hits
    hits = results.get("hits", {}).get("hits", [])
    reranked_hits = rerank(hits, query)

    results["hits"]["hits"] = reranked_hits
    return json.dumps(results)

def rerank(hits, query):
    """Re-rank hits using ML model (simplified)"""
    # Extract features for each hit
    scored_hits = []
    for hit in hits:
        features = extract_features(hit, query)
        # Simple scoring function (in production, use ONNX model)
        score = score_hit(features)
        hit["_score"] = score
        scored_hits.append(hit)

    # Sort by new score
    scored_hits.sort(key=lambda x: x["_score"], reverse=True)
    return scored_hits

def extract_features(hit, query):
    """Extract features for ML model"""
    return {
        "original_score": hit.get("_score", 0),
        "title_length": len(hit.get("_source", {}).get("title", "")),
        "has_description": "description" in hit.get("_source", {}),
        "popularity": hit.get("_source", {}).get("popularity", 0)
    }

def score_hit(features):
    """Simple scoring (replace with ONNX inference)"""
    # Weighted sum of features
    score = (
        features["original_score"] * 0.5 +
        min(features["title_length"] / 100, 1.0) * 0.2 +
        (1.0 if features["has_description"] else 0.0) * 0.1 +
        features["popularity"] * 0.2
    )
    return score
```

---

## Phase 3.5: Documentation & Testing (Day 3 - 4 hours)

### Task 12: Documentation
**Files**:
- `docs/PYTHON_PIPELINE_GUIDE.md`
- `examples/pipelines/README.md`
- Each pipeline example README

**Time**: 2 hours

**Content**:
1. **Pipeline Guide** (docs/PYTHON_PIPELINE_GUIDE.md):
   - What are pipelines?
   - When to use each type (query, document, result)
   - How to create a pipeline
   - How to write Python stages
   - API reference
   - Best practices

2. **Examples README** (examples/pipelines/README.md):
   - Overview of example pipelines
   - How to install and test
   - How to customize

3. **Per-pipeline README**:
   - What the pipeline does
   - How it works
   - Configuration options
   - Usage example
   - Performance considerations

---

### Task 13: Integration Tests
**File**: `pkg/coordination/pipeline/integration_test.go`
**Time**: 2 hours

**Test scenarios**:
```go
func TestQueryPipelineIntegration(t *testing.T) {
    // 1. Start test cluster
    // 2. Create index
    // 3. Register synonym expansion UDF
    // 4. Create synonym expansion pipeline
    // 5. Associate pipeline with index
    // 6. Execute query
    // 7. Verify query was expanded
    // 8. Check results contain synonyms
}

func TestDocumentPipelineIntegration(t *testing.T) {
    // 1. Create document enrichment pipeline
    // 2. Index document
    // 3. Verify document was enriched
}

func TestResultPipelineIntegration(t *testing.T) {
    // 1. Index documents with scores
    // 2. Create re-ranking pipeline
    // 3. Search
    // 4. Verify results were re-ranked
}

func TestPipelineFailureHandling(t *testing.T) {
    // 1. Create pipeline with failing stage
    // 2. Test continue policy
    // 3. Test abort policy
}

func TestPipelineMetrics(t *testing.T) {
    // 1. Execute pipeline multiple times
    // 2. Get stats
    // 3. Verify metrics (count, duration, success rate)
}
```

---

## Task Summary & Checklist

### Day 1 (8 hours) ✅ COMPLETE
- [x] **Task 1**: Core types (DONE)
- [x] **Task 2**: Pipeline registry (3h) ✅
- [x] **Task 3**: Pipeline executor (2h) ✅
- [x] **Task 4**: Python stage adapter (3h) ✅

### Day 2 (8 hours) - 4/8 hours complete
- [x] **Task 5**: HTTP handlers (3h) ✅ COMPLETE (Jan 27 18:00)
- [x] **Task 6**: Index settings (1h) ✅ COMPLETE (Jan 27 18:30 - Already Implemented!)
- [ ] **Task 7**: Query pipeline integration (2h)
- [ ] **Task 8**: Document pipeline integration (2h)

### Day 3 (8 hours)
- [ ] **Task 9**: Synonym expansion example (3h)
- [ ] **Task 10**: Spell correction example (3h)
- [ ] **Task 11**: ML re-ranking example (2h)

### Day 3 (continued - 4 hours)
- [ ] **Task 12**: Documentation (2h)
- [ ] **Task 13**: Integration tests (2h)

**Total**: 24 hours (3 days)

---

## Dependencies & Prerequisites

### Already Complete ✅
- WASM runtime (pkg/wasm/runtime.go)
- Python compiler (pkg/wasm/python/compiler.go)
- UDF registry (pkg/wasm/registry.go)
- Host functions (pkg/wasm/hostfunctions.go)
- Query parser (pkg/coordination/parser/)
- Query planner (pkg/coordination/planner/)

### Need to Install
- MicroPython WASM build (for example pipelines)
- Or: Use existing Python UDF infrastructure

### Configuration
None - reuses existing configuration

---

## Success Criteria

**Phase 3 Complete When**:
1. ✅ Pipeline framework implemented and tested
2. ✅ 3 example pipelines working end-to-end
3. ✅ HTTP API functional
4. ✅ Integration with query/document flows complete
5. ✅ Documentation written
6. ✅ 80%+ test coverage
7. ✅ Performance benchmarks run

**Deliverables**:
- Production-ready pipeline framework
- 3 working example pipelines
- Comprehensive documentation
- Integration tests passing
- Ready for Phase 4

---

## Risk Mitigation

**Risk 1**: WASM Python limitations
- **Mitigation**: Use MicroPython for simple pipelines, document workarounds for complex cases
- **Fallback**: Add native Python in Phase 4

**Risk 2**: Performance overhead
- **Mitigation**: Benchmark early, optimize hot paths
- **Acceptable**: 5-15% overhead for security benefits

**Risk 3**: Integration complexity
- **Mitigation**: Leverage existing WASM infrastructure, minimal changes to query flow
- **Success**: Pipeline execution is opt-in, doesn't break existing functionality

---

## Next Steps After Completion

**Phase 4 Enhancements**:
1. Add native Python support (opt-in)
2. Pipeline graph visualization
3. Parallel stage execution
4. Conditional stage execution
5. Pipeline composition (pipelines calling pipelines)
6. Remote pipeline execution (distributed)
7. Pipeline versioning and A/B testing

**Production Readiness**:
1. Performance benchmarks (target: <10ms overhead per pipeline)
2. Load testing (1000 concurrent pipelines)
3. Security audit
4. Production deployment guide
