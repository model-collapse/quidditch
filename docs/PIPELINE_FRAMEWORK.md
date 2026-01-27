# Pipeline Framework Documentation

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Pipeline Types](#pipeline-types)
- [Creating Pipelines](#creating-pipelines)
- [Stage Types](#stage-types)
- [HTTP API](#http-api)
- [Integration](#integration)
- [Best Practices](#best-practices)
- [Examples](#examples)

## Overview

The Quidditch Pipeline Framework provides a flexible, extensible system for processing search queries, documents, and results. Pipelines are sequences of stages that transform data at different points in the search lifecycle.

### Key Features

- **Three Pipeline Types**: Query, Document, and Result pipelines
- **Multiple Stage Types**: Python UDFs, native Go, and composite stages
- **Failure Handling**: Configurable policies (continue, abort, retry)
- **Performance**: <10ms overhead per stage
- **Observability**: Built-in statistics and monitoring
- **Hot Reloading**: Update pipelines without restart

### Use Cases

| Use Case | Pipeline Type | Example |
|----------|---------------|---------|
| Query rewriting | Query | Synonym expansion, spell check |
| Document enrichment | Document | Field extraction, validation |
| Result re-ranking | Result | ML ranking, personalization |
| Access control | Query/Result | Filter by permissions |
| Analytics | All types | Log queries, track metrics |

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Pipeline Framework                       │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Registry   │───▶│   Executor   │───▶│    Stages    │  │
│  │              │    │              │    │              │  │
│  │ - Register   │    │ - Execute    │    │ - Python     │  │
│  │ - Validate   │    │ - Timeout    │    │ - Native     │  │
│  │ - Lookup     │    │ - Retry      │    │ - Composite  │  │
│  │ - Statistics │    │ - Metrics    │    │              │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Integration Points                       │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │ Query Pipeline: Coordination → Pipeline → Search     │   │
│  │ Document Pipeline: Indexing → Pipeline → Storage     │   │
│  │ Result Pipeline: Search → Pipeline → User            │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

#### Query Pipeline
```
User Request → Parse → [Query Pipeline] → Search → Results
                          ↓
                   Spell Check → Synonym Expansion
```

#### Document Pipeline
```
Document → Parse → [Document Pipeline] → Index → Storage
                        ↓
                  Validate → Enrich → Transform
```

#### Result Pipeline
```
Search → Results → [Result Pipeline] → User
                        ↓
                  ML Ranking → Filter → Format
```

## Pipeline Types

### Query Pipelines

**Execution Point**: Before search execution

**Input**: Search request object
```json
{
  "query": {
    "match": {
      "title": "laptop"
    }
  },
  "size": 10,
  "from": 0
}
```

**Output**: Modified search request
```json
{
  "query": {
    "match": {
      "title": "(laptop OR notebook OR computer)"
    }
  },
  "size": 10,
  "from": 0
}
```

**Use Cases**:
- Spell checking and correction
- Synonym expansion
- Query rewriting
- Access control (filter by permissions)
- Query analytics (logging, tracking)

### Document Pipelines

**Execution Point**: During document indexing

**Input**: Document object
```json
{
  "_id": "doc123",
  "title": "MacBook Pro",
  "price": 2399,
  "category": "laptops"
}
```

**Output**: Modified document
```json
{
  "_id": "doc123",
  "title": "MacBook Pro",
  "price": 2399,
  "category": "laptops",
  "price_range": "high",
  "indexed_at": "2026-01-27T10:00:00Z",
  "embeddings": [0.123, 0.456, ...]
}
```

**Use Cases**:
- Field enrichment (add computed fields)
- Data validation (check required fields)
- Format conversion (date parsing, normalization)
- Generate embeddings for vector search
- Extract metadata (keywords, categories)

### Result Pipelines

**Execution Point**: After search execution

**Input**: Search results object
```json
{
  "total": 100,
  "max_score": 2.5,
  "hits": [
    {
      "score": 2.5,
      "_id": "doc1",
      "_source": {...}
    },
    ...
  ]
}
```

**Output**: Modified search results
```json
{
  "total": 100,
  "max_score": 3.2,
  "hits": [
    {
      "score": 3.2,
      "_id": "doc2",
      "_source": {...},
      "_explanation": "Re-ranked by ML model"
    },
    ...
  ]
}
```

**Use Cases**:
- ML-based re-ranking
- Personalization (user preferences)
- Result filtering (hide sensitive data)
- Highlighting and formatting
- A/B testing (traffic splitting)

## Creating Pipelines

### Pipeline Definition

```json
{
  "name": "advanced_query_processor",
  "version": "1.0.0",
  "type": "query",
  "description": "Spell check and synonym expansion",
  "enabled": true,
  "on_failure": "continue",
  "timeout": "1s",
  "stages": [
    {
      "name": "spell_check",
      "type": "python",
      "config": {
        "udf_name": "spell_checker",
        "udf_version": "1.0.0"
      }
    },
    {
      "name": "expand_synonyms",
      "type": "python",
      "config": {
        "udf_name": "synonym_expander",
        "udf_version": "1.0.0"
      }
    }
  ],
  "metadata": {
    "author": "search-team",
    "last_reviewed": "2026-01-27",
    "tags": ["query-processing", "nlp"]
  }
}
```

### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique pipeline identifier |
| `version` | string | Yes | Semantic version (e.g., "1.0.0") |
| `type` | enum | Yes | When to execute: "query", "document", "result" |
| `description` | string | No | Human-readable description |
| `enabled` | boolean | No | Whether pipeline is active (default: true) |
| `on_failure` | enum | No | Failure policy: "continue", "abort", "retry" |
| `timeout` | duration | No | Max execution time (e.g., "1s", "500ms") |
| `stages` | array | Yes | Ordered list of processing stages |
| `metadata` | object | No | Custom metadata (tags, owner, etc.) |

### Failure Policies

**Continue** (default):
- Stage failure doesn't stop pipeline
- Logs error, continues to next stage
- Use for non-critical transformations

```json
{
  "on_failure": "continue"
}
```

**Abort**:
- Stage failure stops pipeline
- Returns error to caller
- Use for critical validations

```json
{
  "on_failure": "abort"
}
```

**Retry**:
- Retries failed stage (3 attempts with exponential backoff)
- Aborts if all retries fail
- Use for transient failures (network, rate limits)

```json
{
  "on_failure": "retry",
  "retry_config": {
    "max_attempts": 3,
    "initial_delay": "100ms",
    "max_delay": "1s",
    "multiplier": 2.0
  }
}
```

## Stage Types

### Python Stage

Execute Python UDF compiled to WASM.

```json
{
  "name": "spell_check",
  "type": "python",
  "config": {
    "udf_name": "spell_checker",
    "udf_version": "1.0.0",
    "timeout": "100ms"
  }
}
```

**Advantages**:
- Flexible (full Python language)
- Safe (sandboxed WASM execution)
- Fast (20-500ns per call)

**Limitations**:
- No external network access
- Limited stdlib (MicroPython)
- Memory constrained (16MB)

### Native Stage

Execute pure Go code.

```json
{
  "name": "add_timestamp",
  "type": "native",
  "config": {
    "handler": "AddTimestampStage"
  }
}
```

**Advantages**:
- Maximum performance (<1ns)
- Full Go stdlib access
- No sandboxing overhead

**Limitations**:
- Requires code deployment
- Less flexible (recompile to change)

### Composite Stage

Chain multiple stages together.

```json
{
  "name": "nlp_processor",
  "type": "composite",
  "config": {
    "stages": [
      {"ref": "spell_check"},
      {"ref": "synonym_expansion"},
      {"ref": "entity_extraction"}
    ]
  }
}
```

**Advantages**:
- Reusable sub-pipelines
- Clean organization
- Conditional execution

## HTTP API

### Create Pipeline

**Request**:
```http
POST /api/v1/pipelines/{name}
Content-Type: application/json

{
  "name": "synonym_expander",
  "version": "1.0.0",
  "type": "query",
  ...
}
```

**Response**:
```json
{
  "message": "Pipeline created successfully",
  "name": "synonym_expander",
  "version": "1.0.0"
}
```

### Get Pipeline

**Request**:
```http
GET /api/v1/pipelines/{name}
```

**Response**:
```json
{
  "name": "synonym_expander",
  "version": "1.0.0",
  "type": "query",
  "stages": [...],
  "created": "2026-01-27T10:00:00Z",
  "updated": "2026-01-27T10:00:00Z"
}
```

### Delete Pipeline

**Request**:
```http
DELETE /api/v1/pipelines/{name}
```

**Response**:
```json
{
  "message": "Pipeline deleted successfully"
}
```

### List Pipelines

**Request**:
```http
GET /api/v1/pipelines?type=query&enabled=true
```

**Response**:
```json
{
  "total": 5,
  "pipelines": [
    {
      "name": "synonym_expander",
      "version": "1.0.0",
      "type": "query",
      "enabled": true
    },
    ...
  ]
}
```

### Execute Pipeline (Test)

**Request**:
```http
POST /api/v1/pipelines/{name}/_execute
Content-Type: application/json

{
  "input": {
    "query": {
      "match": {"title": "laptop"}
    }
  }
}
```

**Response**:
```json
{
  "output": {
    "query": {
      "match": {"title": "(laptop OR notebook OR computer)"}
    }
  },
  "duration_ms": 5,
  "success": true
}
```

### Get Statistics

**Request**:
```http
GET /api/v1/pipelines/{name}/_stats
```

**Response**:
```json
{
  "name": "synonym_expander",
  "version": "1.0.0",
  "stats": {
    "execution_count": 10523,
    "error_count": 12,
    "total_duration_ms": 52615,
    "avg_duration_ms": 5.0,
    "p50_duration_ms": 4.2,
    "p95_duration_ms": 8.1,
    "p99_duration_ms": 12.5
  }
}
```

## Integration

### Associate Pipeline with Index

#### Query Pipeline (default_pipeline)

Runs on every search query:

```bash
PUT /products/_settings
{
  "index": {
    "default_pipeline": "synonym_expander"
  }
}
```

#### Document Pipeline (default_pipeline)

Runs during indexing:

```bash
PUT /products/_settings
{
  "index": {
    "default_pipeline": "document_enricher"
  }
}
```

#### Result Pipeline (search.default_pipeline)

Runs on search results:

```bash
PUT /products/_settings
{
  "index": {
    "search": {
      "default_pipeline": "ml_ranker"
    }
  }
}
```

### Per-Request Override

Override pipeline for a single request:

```bash
POST /products/_search?pipeline=custom_ranker
{
  "query": {
    "match": {"title": "laptop"}
  }
}
```

### Conditional Execution

Execute pipeline only if condition is met:

```json
{
  "name": "premium_ranker",
  "type": "result",
  "condition": {
    "field": "user.tier",
    "equals": "premium"
  },
  "stages": [...]
}
```

## Best Practices

### 1. Performance

**Target Latency**: <10ms per pipeline

```json
{
  "timeout": "10ms",
  "stages": [
    {
      "name": "fast_stage",
      "timeout": "5ms"
    }
  ]
}
```

**Optimization Tips**:
- Cache expensive operations
- Use native stages for hot paths
- Profile with `/_stats` endpoint
- Set appropriate timeouts

### 2. Error Handling

**Graceful Degradation**:

```json
{
  "on_failure": "continue",
  "stages": [
    {
      "name": "spell_check",
      "on_failure": "continue"
    },
    {
      "name": "validate_required_fields",
      "on_failure": "abort"
    }
  ]
}
```

**Retry Transient Failures**:

```json
{
  "on_failure": "retry",
  "retry_config": {
    "max_attempts": 3,
    "retryable_errors": ["timeout", "rate_limit"]
  }
}
```

### 3. Testing

**Test Before Deploy**:

```bash
# Test with sample data
curl -X POST /api/v1/pipelines/synonym_expander/_execute \
  -d '{"input": {...}}'

# Check performance
curl /api/v1/pipelines/synonym_expander/_stats

# Verify output format
```

**A/B Testing**:

```json
{
  "name": "ab_test_ranker",
  "type": "result",
  "stages": [
    {
      "name": "split_traffic",
      "config": {
        "control": 50,
        "treatment": 50
      }
    }
  ]
}
```

### 4. Versioning

**Semantic Versioning**:

```json
{
  "name": "synonym_expander",
  "version": "1.0.0",
  ...
}
```

**Update Strategy**:
1. Create new version
2. Test thoroughly
3. Update index settings
4. Monitor metrics
5. Roll back if needed

### 5. Monitoring

**Key Metrics**:
- Execution count
- Error rate (<1%)
- P99 latency (<50ms)
- Cache hit rate (>80%)

**Alerts**:
- Error rate > 5%
- P99 latency > 100ms
- Execution count drops to 0

## Examples

See [examples/pipelines/](../examples/pipelines/) for complete examples:

- **synonym_expansion.py**: Expand query terms with synonyms
- **spell_check.py**: Correct spelling mistakes
- **ml_ranking.py**: Re-rank results with ML model

## Troubleshooting

### Pipeline Not Executing

**Check pipeline is enabled**:
```bash
curl /api/v1/pipelines/synonym_expander | jq '.enabled'
```

**Check index settings**:
```bash
curl /products/_settings | jq '.settings.index.default_pipeline'
```

### High Latency

**Check statistics**:
```bash
curl /api/v1/pipelines/synonym_expander/_stats
```

**Profile stages**:
- Identify slow stages (P99 > 50ms)
- Optimize or replace with native implementation
- Add caching

### Errors

**Check logs**:
```bash
grep "pipeline_error" /var/log/quidditch/coordination.log
```

**Test execution**:
```bash
curl -X POST /api/v1/pipelines/synonym_expander/_execute \
  -d '{"input": {...}, "debug": true}'
```

## See Also

- [UDF Development Guide](UDF_DEVELOPMENT.md)
- [Pipeline Examples](../examples/pipelines/)
- [API Reference](API_REFERENCE.md)
- [Performance Tuning](PERFORMANCE_TUNING.md)
