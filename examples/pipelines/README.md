# Pipeline Examples

This directory contains example pipeline implementations demonstrating real-world use cases for the Quidditch pipeline framework.

## Examples Overview

| Pipeline | Type | Purpose | Use Case |
|----------|------|---------|----------|
| **synonym_expansion.py** | Query | Expand search terms with synonyms | Improve recall by matching related terms |
| **spell_check.py** | Query | Correct spelling mistakes | Handle typos in user queries |
| **ml_ranking.py** | Result | Re-rank results with ML model | Personalized ranking using multiple signals |

## Quick Start

### 1. Register a Pipeline

Use the HTTP API to register a pipeline:

```bash
# Register synonym expansion pipeline
curl -X POST http://localhost:9200/api/v1/pipelines/synonym_expander \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "synonym_expander",
    "version": "1.0.0",
    "type": "query",
    "description": "Expands query terms with synonyms to improve recall",
    "enabled": true,
    "stages": [
      {
        "name": "expand_synonyms",
        "type": "python",
        "config": {
          "udf_name": "synonym_expansion",
          "udf_version": "1.0.0"
        }
      }
    ]
  }'
```

### 2. Upload Python UDF

Upload the Python implementation as a WASM UDF:

```bash
# Compile Python to WASM (requires micropython or py2wasm)
python3 -m py2wasm synonym_expansion.py -o synonym_expansion.wasm

# Upload UDF
curl -X POST http://localhost:9200/api/v1/udfs \
  -F "name=synonym_expansion" \
  -F "version=1.0.0" \
  -F "language=python" \
  -F "source=@synonym_expansion.py"
```

### 3. Associate Pipeline with Index

Associate the pipeline with an index so it runs automatically:

```bash
# Create index with pipeline
curl -X PUT http://localhost:9200/products \
  -H 'Content-Type: application/json' \
  -d '{
    "settings": {
      "index": {
        "default_pipeline": "synonym_expander"
      }
    },
    "mappings": {
      "properties": {
        "title": {"type": "text"},
        "description": {"type": "text"},
        "price": {"type": "float"}
      }
    }
  }'
```

### 4. Test Pipeline

Test the pipeline before deploying:

```bash
# Test synonym expansion
curl -X POST http://localhost:9200/api/v1/pipelines/synonym_expander/_execute \
  -H 'Content-Type: application/json' \
  -d '{
    "input": {
      "query": {
        "match": {
          "title": "laptop"
        }
      }
    }
  }'

# Expected output:
# {
#   "output": {
#     "query": {
#       "match": {
#         "title": "(laptop OR notebook OR computer)"
#       }
#     }
#   },
#   "duration_ms": 5,
#   "success": true
# }
```

## Example 1: Synonym Expansion

**File**: `synonym_expansion.py`

**Purpose**: Expand user search terms with synonyms to improve recall.

**How it works**:
1. Receives a search query (e.g., "laptop")
2. Looks up synonyms in dictionary
3. Expands query to include synonyms: "laptop OR notebook OR computer"
4. Returns modified query

**Use cases**:
- E-commerce: "phone" → "phone OR mobile OR smartphone"
- Job search: "developer" → "developer OR engineer OR programmer"
- Content search: "guide" → "guide OR tutorial OR howto"

**Complete Example**:

```bash
# 1. Register pipeline
curl -X POST http://localhost:9200/api/v1/pipelines/synonym_expander \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "synonym_expander",
    "version": "1.0.0",
    "type": "query",
    "description": "Expands query terms with synonyms",
    "enabled": true,
    "on_failure": "continue",
    "timeout": "1s",
    "stages": [
      {
        "name": "expand_synonyms",
        "type": "python",
        "config": {
          "udf_name": "synonym_expansion",
          "udf_version": "1.0.0"
        }
      }
    ],
    "metadata": {
      "author": "search-team",
      "last_reviewed": "2026-01-27"
    }
  }'

# 2. Associate with index
curl -X PUT http://localhost:9200/products/_settings \
  -H 'Content-Type: application/json' \
  -d '{
    "index": {
      "default_pipeline": "synonym_expander"
    }
  }'

# 3. Search (pipeline runs automatically)
curl -X POST http://localhost:9200/products/_search \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "match": {
        "title": "laptop"
      }
    }
  }'

# Query is automatically expanded to:
# "query": {"match": {"title": "(laptop OR notebook OR computer)"}}
```

## Example 2: Spell Check

**File**: `spell_check.py`

**Purpose**: Automatically correct common spelling mistakes in queries.

**How it works**:
1. Receives a search query (e.g., "labtop")
2. Checks each term against spelling dictionary
3. Corrects misspellings: "labtop" → "laptop"
4. Returns corrected query + metadata about corrections

**Use cases**:
- User typos: "computor" → "computer"
- Mobile keyboard errors: "phoen" → "phone"
- Non-native speakers: "expensiv" → "expensive"

**Complete Example**:

```bash
# 1. Register pipeline
curl -X POST http://localhost:9200/api/v1/pipelines/spell_checker \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "spell_checker",
    "version": "1.0.0",
    "type": "query",
    "description": "Corrects spelling mistakes in queries",
    "enabled": true,
    "stages": [
      {
        "name": "correct_spelling",
        "type": "python",
        "config": {
          "udf_name": "spell_check",
          "udf_version": "1.0.0",
          "auto_correct": true,
          "suggest": true
        }
      }
    ]
  }'

# 2. Test with misspelled query
curl -X POST http://localhost:9200/api/v1/pipelines/spell_checker/_execute \
  -H 'Content-Type: application/json' \
  -d '{
    "input": {
      "query": {
        "match": {
          "title": "labtop"
        }
      }
    }
  }'

# Expected output:
# {
#   "output": {
#     "query": {
#       "match": {
#         "title": "laptop"
#       }
#     },
#     "_meta": {
#       "spelling_corrections": [
#         {"original": "labtop", "corrected": "laptop"}
#       ]
#     }
#   },
#   "duration_ms": 3,
#   "success": true
# }
```

## Example 3: ML Ranking

**File**: `ml_ranking.py`

**Purpose**: Re-rank search results using a machine learning model.

**How it works**:
1. Receives search results with BM25 scores
2. Extracts features for each result (BM25, CTR, recency, etc.)
3. Applies ML model to compute new relevance scores
4. Re-ranks results by ML scores
5. Returns re-ranked results

**Features used**:
- **BM25 score** (0.4 weight): Text relevance
- **Click-through rate** (0.3 weight): Historical engagement
- **Recency** (0.15 weight): How recent the document is
- **Has image** (0.10 weight): Visual appeal
- **Price score** (0.05 weight): Price attractiveness

**Use cases**:
- E-commerce: Personalized product ranking
- News: Balance relevance with recency
- Job search: Match skills + company preferences

**Complete Example**:

```bash
# 1. Register pipeline
curl -X POST http://localhost:9200/api/v1/pipelines/ml_ranker \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "ml_ranker",
    "version": "1.0.0",
    "type": "result",
    "description": "Re-ranks results using ML model",
    "enabled": true,
    "timeout": "100ms",
    "stages": [
      {
        "name": "apply_ml_ranking",
        "type": "python",
        "config": {
          "udf_name": "ml_ranking",
          "udf_version": "1.0.0",
          "model": "learning_to_rank_v1"
        }
      }
    ],
    "metadata": {
      "model_version": "v1",
      "training_date": "2026-01-20",
      "features": ["bm25", "ctr", "recency", "has_image", "price"]
    }
  }'

# 2. Associate with index
curl -X PUT http://localhost:9200/products/_settings \
  -H 'Content-Type: application/json' \
  -d '{
    "index": {
      "search": {
        "default_pipeline": "ml_ranker"
      }
    }
  }'

# 3. Search (pipeline runs after results are retrieved)
curl -X POST http://localhost:9200/products/_search \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "match": {
        "title": "laptop"
      }
    },
    "size": 10
  }'

# Results are automatically re-ranked by ML model
```

## Chaining Multiple Pipelines

You can chain multiple stages in a single pipeline:

```bash
curl -X POST http://localhost:9200/api/v1/pipelines/advanced_query_processor \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "advanced_query_processor",
    "version": "1.0.0",
    "type": "query",
    "description": "Spell check + synonym expansion",
    "enabled": true,
    "stages": [
      {
        "name": "correct_spelling",
        "type": "python",
        "config": {
          "udf_name": "spell_check",
          "udf_version": "1.0.0"
        }
      },
      {
        "name": "expand_synonyms",
        "type": "python",
        "config": {
          "udf_name": "synonym_expansion",
          "udf_version": "1.0.0"
        }
      }
    ]
  }'
```

Processing flow:
1. User query: "labtop" (misspelled)
2. After spell check: "laptop" (corrected)
3. After synonym expansion: "(laptop OR notebook OR computer)" (expanded)

## Pipeline Types

### Query Pipelines (type: "query")
- Execute **before** search
- Modify the query before it's executed
- Use cases: spell check, synonym expansion, query rewriting

### Document Pipelines (type: "document")
- Execute **during** indexing
- Transform documents before they're stored
- Use cases: field enrichment, data validation, format conversion

### Result Pipelines (type: "result")
- Execute **after** search
- Modify search results before returning to user
- Use cases: ML ranking, filtering, field transformation

## Best Practices

1. **Test pipelines before deployment**
   - Use `/_execute` endpoint to test with sample data
   - Verify performance (target: <10ms per stage)

2. **Handle failures gracefully**
   - Set `on_failure: "continue"` for non-critical pipelines
   - Use `on_failure: "abort"` for critical transformations

3. **Monitor performance**
   - Check pipeline statistics: `GET /api/v1/pipelines/{name}/_stats`
   - Set appropriate timeouts

4. **Version your pipelines**
   - Use semantic versioning (1.0.0, 1.1.0, etc.)
   - Test new versions before updating production

5. **Document your pipelines**
   - Add clear descriptions
   - Include metadata (author, last_reviewed, etc.)
   - Explain expected input/output formats

## Troubleshooting

### Pipeline not executing

Check if pipeline is enabled:
```bash
curl http://localhost:9200/api/v1/pipelines/synonym_expander | jq '.enabled'
```

Check index settings:
```bash
curl http://localhost:9200/products/_settings | jq '.settings.index.default_pipeline'
```

### Performance issues

Check pipeline statistics:
```bash
curl http://localhost:9200/api/v1/pipelines/synonym_expander/_stats
```

Look for:
- High execution count with errors
- Long P99 latency (>100ms)
- Low cache hit rate

### Debugging pipelines

Enable detailed logging and test execution:
```bash
curl -X POST http://localhost:9200/api/v1/pipelines/synonym_expander/_execute \
  -H 'Content-Type: application/json' \
  -d '{
    "input": {...},
    "debug": true
  }'
```

## See Also

- [Pipeline Framework Documentation](../../docs/PIPELINE_FRAMEWORK.md)
- [UDF Development Guide](../../docs/UDF_DEVELOPMENT.md)
- [Pipeline API Reference](../../docs/API_REFERENCE.md#pipelines)
