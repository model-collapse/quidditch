# Quidditch API Examples

**Comprehensive Guide to OpenSearch-Compatible APIs**

**Version**: 1.0.0
**Date**: 2026-01-25

---

## Table of Contents

1. [Index Management](#index-management)
2. [Document Operations](#document-operations)
3. [Search Queries](#search-queries)
4. [Aggregations](#aggregations)
5. [PPL Queries](#ppl-queries)
6. [Python Pipelines](#python-pipelines)
7. [Cluster Operations](#cluster-operations)
8. [Advanced Features](#advanced-features)

---

## Index Management

### Create Index with Mappings

```http
PUT /products
Content-Type: application/json

{
  "settings": {
    "number_of_shards": 5,
    "number_of_replicas": 1,
    "index": {
      "codec": "diagon_best_compression",
      "refresh_interval": "1s",
      "max_result_window": 10000
    }
  },
  "mappings": {
    "properties": {
      "title": {
        "type": "text",
        "analyzer": "standard",
        "fields": {
          "keyword": {
            "type": "keyword"
          }
        }
      },
      "description": {
        "type": "text",
        "analyzer": "english"
      },
      "category": {
        "type": "keyword"
      },
      "price": {
        "type": "float"
      },
      "stock": {
        "type": "integer"
      },
      "tags": {
        "type": "keyword",
        "multi_valued": true
      },
      "timestamp": {
        "type": "date",
        "format": "strict_date_optional_time||epoch_millis"
      },
      "location": {
        "type": "geo_point"
      },
      "metadata": {
        "type": "object",
        "properties": {
          "brand": {"type": "keyword"},
          "color": {"type": "keyword"}
        }
      }
    }
  }
}
```

**Response**:
```json
{
  "acknowledged": true,
  "shards_acknowledged": true,
  "index": "products"
}
```

---

### Update Mapping (Add Field)

```http
PUT /products/_mapping
Content-Type: application/json

{
  "properties": {
    "rating": {
      "type": "float"
    },
    "review_count": {
      "type": "integer"
    }
  }
}
```

---

### Create Index Alias

```http
POST /_aliases
Content-Type: application/json

{
  "actions": [
    {
      "add": {
        "index": "products-2026-01",
        "alias": "products",
        "filter": {
          "term": {"status": "published"}
        }
      }
    }
  ]
}
```

---

### Create Index Template

```http
PUT /_index_template/logs-template
Content-Type: application/json

{
  "index_patterns": ["logs-*"],
  "template": {
    "settings": {
      "number_of_shards": 3,
      "number_of_replicas": 1,
      "index.lifecycle.name": "logs-policy"
    },
    "mappings": {
      "properties": {
        "@timestamp": {"type": "date"},
        "message": {"type": "text"},
        "level": {"type": "keyword"},
        "host": {"type": "keyword"},
        "pid": {"type": "integer"}
      }
    }
  },
  "priority": 100,
  "version": 1
}
```

---

## Document Operations

### Index Single Document

```http
PUT /products/_doc/1
Content-Type: application/json

{
  "title": "Diagon Search Engine",
  "description": "High-performance C++ search engine with SIMD acceleration",
  "category": "software",
  "price": 99.99,
  "stock": 1000,
  "tags": ["search", "analytics", "performance"],
  "timestamp": "2026-01-25T10:00:00Z",
  "location": {
    "lat": 37.7749,
    "lon": -122.4194
  },
  "metadata": {
    "brand": "Quidditch",
    "color": "blue"
  },
  "rating": 4.8,
  "review_count": 142
}
```

**Response**:
```json
{
  "_index": "products",
  "_id": "1",
  "_version": 1,
  "result": "created",
  "_shards": {
    "total": 2,
    "successful": 2,
    "failed": 0
  }
}
```

---

### Bulk Indexing

```http
POST /_bulk
Content-Type: application/x-ndjson

{"index": {"_index": "products", "_id": "1"}}
{"title": "Product 1", "price": 10.0, "category": "electronics"}
{"index": {"_index": "products", "_id": "2"}}
{"title": "Product 2", "price": 20.0, "category": "books"}
{"index": {"_index": "products", "_id": "3"}}
{"title": "Product 3", "price": 30.0, "category": "electronics"}
{"update": {"_index": "products", "_id": "1"}}
{"doc": {"stock": 950}}
{"delete": {"_index": "products", "_id": "2"}}
```

**Response**:
```json
{
  "took": 45,
  "errors": false,
  "items": [
    {
      "index": {
        "_index": "products",
        "_id": "1",
        "_version": 1,
        "result": "created",
        "status": 201
      }
    },
    {
      "update": {
        "_index": "products",
        "_id": "1",
        "_version": 2,
        "result": "updated",
        "status": 200
      }
    }
  ]
}
```

---

### Update Document (Partial)

```http
POST /products/_update/1
Content-Type: application/json

{
  "doc": {
    "price": 89.99,
    "stock": 900
  },
  "doc_as_upsert": true
}
```

---

### Get Document

```http
GET /products/_doc/1
```

**Response**:
```json
{
  "_index": "products",
  "_id": "1",
  "_version": 2,
  "_seq_no": 1,
  "_primary_term": 1,
  "found": true,
  "_source": {
    "title": "Diagon Search Engine",
    "price": 89.99,
    "stock": 900,
    "category": "software"
  }
}
```

---

### Multi-Get

```http
GET /_mget
Content-Type: application/json

{
  "docs": [
    {"_index": "products", "_id": "1"},
    {"_index": "products", "_id": "2"},
    {"_index": "logs-2026-01", "_id": "100"}
  ]
}
```

---

## Search Queries

### Match All Query

```http
GET /products/_search
Content-Type: application/json

{
  "query": {
    "match_all": {}
  },
  "size": 10,
  "from": 0
}
```

---

### Full-Text Search (Match Query)

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "match": {
      "title": {
        "query": "search engine performance",
        "operator": "and",
        "fuzziness": "AUTO"
      }
    }
  }
}
```

---

### Multi-Match Query (Multiple Fields)

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "multi_match": {
      "query": "diagon search",
      "fields": ["title^3", "description", "tags"],
      "type": "best_fields",
      "tie_breaker": 0.3,
      "minimum_should_match": "75%"
    }
  }
}
```

---

### Boolean Query (Complex)

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "bool": {
      "must": [
        {
          "match": {
            "title": "search"
          }
        }
      ],
      "filter": [
        {
          "range": {
            "price": {
              "gte": 10,
              "lte": 100
            }
          }
        },
        {
          "term": {
            "category": "software"
          }
        },
        {
          "terms": {
            "tags": ["performance", "analytics"]
          }
        }
      ],
      "should": [
        {
          "match": {
            "description": "SIMD"
          }
        },
        {
          "term": {
            "metadata.brand": "Quidditch"
          }
        }
      ],
      "must_not": [
        {
          "term": {
            "status": "archived"
          }
        }
      ],
      "minimum_should_match": 1
    }
  },
  "sort": [
    {"_score": "desc"},
    {"price": "asc"}
  ],
  "size": 20
}
```

---

### Phrase Query (Exact Phrase)

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "match_phrase": {
      "description": {
        "query": "high performance search",
        "slop": 2
      }
    }
  }
}
```

---

### Range Query

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "range": {
      "timestamp": {
        "gte": "2026-01-01",
        "lt": "2026-02-01",
        "format": "yyyy-MM-dd",
        "time_zone": "+00:00"
      }
    }
  }
}
```

---

### Wildcard Query

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "wildcard": {
      "title": {
        "value": "*search*",
        "case_insensitive": true
      }
    }
  }
}
```

---

### Function Score Query (Custom Scoring)

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "function_score": {
      "query": {
        "match": {
          "title": "search"
        }
      },
      "functions": [
        {
          "filter": {"term": {"category": "software"}},
          "weight": 2.0
        },
        {
          "field_value_factor": {
            "field": "rating",
            "factor": 1.2,
            "modifier": "sqrt",
            "missing": 1
          }
        },
        {
          "gauss": {
            "timestamp": {
              "origin": "now",
              "scale": "30d",
              "offset": "7d",
              "decay": 0.5
            }
          }
        },
        {
          "script_score": {
            "script": {
              "source": "_score * (1 + doc['review_count'].value / 1000)"
            }
          }
        }
      ],
      "score_mode": "multiply",
      "boost_mode": "multiply",
      "max_boost": 10.0,
      "min_score": 1.0
    }
  }
}
```

---

### Nested Query

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "nested": {
      "path": "reviews",
      "query": {
        "bool": {
          "must": [
            {"match": {"reviews.text": "excellent"}},
            {"range": {"reviews.rating": {"gte": 4}}}
          ]
        }
      },
      "inner_hits": {
        "size": 3,
        "sort": [{"reviews.rating": "desc"}]
      }
    }
  }
}
```

---

## Aggregations

### Terms Aggregation (Group By)

```http
POST /products/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "by_category": {
      "terms": {
        "field": "category",
        "size": 10,
        "order": {"_count": "desc"}
      },
      "aggs": {
        "avg_price": {
          "avg": {
            "field": "price"
          }
        },
        "total_stock": {
          "sum": {
            "field": "stock"
          }
        }
      }
    }
  }
}
```

**Response**:
```json
{
  "aggregations": {
    "by_category": {
      "buckets": [
        {
          "key": "software",
          "doc_count": 152,
          "avg_price": {"value": 45.67},
          "total_stock": {"value": 25000}
        },
        {
          "key": "electronics",
          "doc_count": 98,
          "avg_price": {"value": 299.99},
          "total_stock": {"value": 5000}
        }
      ]
    }
  }
}
```

---

### Date Histogram (Time-Series)

```http
POST /logs-2026-01/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "requests_over_time": {
      "date_histogram": {
        "field": "@timestamp",
        "fixed_interval": "1h",
        "time_zone": "America/Los_Angeles",
        "min_doc_count": 1
      },
      "aggs": {
        "avg_response_time": {
          "avg": {"field": "response_time_ms"}
        },
        "error_count": {
          "filter": {
            "term": {"level": "ERROR"}
          }
        }
      }
    }
  }
}
```

---

### Range Aggregation (Price Buckets)

```http
POST /products/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "price_ranges": {
      "range": {
        "field": "price",
        "ranges": [
          {"key": "cheap", "to": 50},
          {"key": "medium", "from": 50, "to": 100},
          {"key": "expensive", "from": 100}
        ]
      }
    }
  }
}
```

---

### Stats Aggregation

```http
POST /products/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "price_stats": {
      "stats": {
        "field": "price"
      }
    },
    "extended_stats": {
      "extended_stats": {
        "field": "rating"
      }
    }
  }
}
```

**Response**:
```json
{
  "aggregations": {
    "price_stats": {
      "count": 1000,
      "min": 9.99,
      "max": 999.99,
      "avg": 125.45,
      "sum": 125450.00
    },
    "extended_stats": {
      "count": 1000,
      "min": 1.0,
      "max": 5.0,
      "avg": 4.2,
      "sum": 4200.0,
      "std_deviation": 0.8,
      "variance": 0.64
    }
  }
}
```

---

### Percentiles Aggregation

```http
POST /products/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "price_percentiles": {
      "percentiles": {
        "field": "price",
        "percents": [25, 50, 75, 95, 99]
      }
    }
  }
}
```

---

### Cardinality Aggregation (Distinct Count)

```http
POST /products/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "unique_categories": {
      "cardinality": {
        "field": "category",
        "precision_threshold": 100
      }
    }
  }
}
```

---

### Pipeline Aggregation (Derivative)

```http
POST /metrics/_search
Content-Type: application/json

{
  "size": 0,
  "aggs": {
    "sales_per_month": {
      "date_histogram": {
        "field": "timestamp",
        "fixed_interval": "1M"
      },
      "aggs": {
        "total_sales": {
          "sum": {"field": "amount"}
        },
        "sales_derivative": {
          "derivative": {
            "buckets_path": "total_sales"
          }
        }
      }
    }
  }
}
```

---

## PPL Queries

### Basic PPL Query

```sql
-- Search logs with filters
source=logs-2026-01
| where level = "ERROR" and host = "prod-server-01"
| fields timestamp, message, pid
| sort -timestamp
| head 100
```

**HTTP API**:
```http
POST /_plugins/_ppl
Content-Type: application/json

{
  "query": "source=logs-2026-01 | where level = 'ERROR' and host = 'prod-server-01' | fields timestamp, message, pid | sort -timestamp | head 100"
}
```

---

### PPL Aggregation

```sql
-- Count logs by level
source=logs-2026-01
| stats count() by level
| sort -count()
```

---

### PPL Time-Series Analysis

```sql
-- Average CPU usage over time
source=metrics
| where timestamp > now() - 1h
| stats avg(cpu_usage), max(memory_usage) by host, span(timestamp, 1m)
| where avg(cpu_usage) > 80
| sort -avg(cpu_usage)
| head 10
```

---

### PPL with Eval (Computed Fields)

```sql
-- Calculate revenue
source=orders
| eval revenue = price * quantity
| stats sum(revenue) by category, span(timestamp, 1d)
| sort -sum(revenue)
```

---

### PPL Join (Basic)

```sql
-- Join orders with users
source=orders
| join type=left orders.user_id = users.id [search source=users]
| fields order_id, user_name, total_price
| sort -total_price
| head 20
```

---

## Python Pipelines

### Simple Pipeline (Query Rewriting)

```python
# my_pipeline.py
from quidditch.pipeline import Processor

class QueryBoostProcessor(Processor):
    """Boost queries for premium users"""

    def process_request(self, request):
        if request.user and "premium" in request.user.roles:
            # Boost relevance for premium users
            if "match" in request.query:
                field, text = next(iter(request.query["match"].items()))
                request.query = {
                    "function_score": {
                        "query": {"match": {field: text}},
                        "boost": 1.5
                    }
                }

        return request
```

**Deploy**:
```http
PUT /_search/pipeline/boost-premium
Content-Type: application/json

{
  "description": "Boost search results for premium users",
  "processors": [
    {
      "type": "python",
      "module": "my_pipeline",
      "class": "QueryBoostProcessor"
    }
  ]
}
```

**Use**:
```http
POST /products/_search?pipeline=boost-premium
Content-Type: application/json

{
  "query": {"match": {"title": "search engine"}}
}
```

---

### ML Re-Ranking Pipeline

```python
from quidditch.pipeline import Processor
import onnxruntime as ort
import numpy as np

class MLRerankProcessor(Processor):
    def __init__(self, model_path="/models/rerank.onnx"):
        self.session = ort.InferenceSession(model_path)

    def process_response(self, response, request):
        if not response.hits.hits:
            return response

        # Extract features
        features = []
        for hit in response.hits.hits:
            features.append([
                hit._score,
                hit._source.get("rating", 0),
                hit._source.get("review_count", 0),
                len(hit._source.get("title", "")),
                self.text_overlap(request.query, hit._source.get("title", ""))
            ])

        # Run ML model
        features_array = np.array(features, dtype=np.float32)
        new_scores = self.session.run(None, {"features": features_array})[0]

        # Update scores
        for hit, score in zip(response.hits.hits, new_scores):
            hit._score = float(score)

        # Re-sort
        response.hits.hits.sort(key=lambda x: x._score, reverse=True)
        response.hits.max_score = response.hits.hits[0]._score

        return response

    def text_overlap(self, query_dict, text):
        query_text = str(query_dict.get("match", {}).get("title", ""))
        query_terms = set(query_text.lower().split())
        text_terms = set(text.lower().split())
        if not query_terms:
            return 0.0
        return len(query_terms & text_terms) / len(query_terms)
```

---

## Cluster Operations

### Cluster Health

```http
GET /_cluster/health?pretty
```

**Response**:
```json
{
  "cluster_name": "quidditch-prod",
  "status": "green",
  "timed_out": false,
  "number_of_nodes": 18,
  "number_of_data_nodes": 10,
  "active_primary_shards": 50,
  "active_shards": 100,
  "relocating_shards": 0,
  "initializing_shards": 0,
  "unassigned_shards": 0
}
```

---

### Cluster Stats

```http
GET /_cluster/stats?pretty
```

---

### Node Stats

```http
GET /_nodes/stats?pretty
GET /_nodes/stats/indices,os,jvm?pretty
```

---

### Cat APIs (Human-Readable)

```bash
# List indices
GET /_cat/indices?v&h=index,docs.count,store.size,health

# List shards
GET /_cat/shards?v&h=index,shard,prirep,state,node

# List nodes
GET /_cat/nodes?v&h=name,role,cpu,load,heap.percent

# List allocation
GET /_cat/allocation?v&h=node,shards,disk.used,disk.avail,disk.percent
```

---

## Advanced Features

### Scroll API (Deep Pagination)

```http
# Initial request
POST /products/_search?scroll=1m
Content-Type: application/json

{
  "size": 1000,
  "query": {"match_all": {}}
}

# Continue scrolling
POST /_search/scroll
Content-Type: application/json

{
  "scroll": "1m",
  "scroll_id": "DXF1ZXJ5QW5kRmV0Y2gBAAAAAAAAAD4WYm9laVYtZndUQlNsdDcwakFMNjU1QQ=="
}
```

---

### Point-in-Time (PIT)

```http
# Create PIT
POST /products/_pit?keep_alive=1m

# Search with PIT
POST /_search
Content-Type: application/json

{
  "pit": {
    "id": "46ToAwMDaWR5BXV1aWQyKwZub2RlXzMA...==",
    "keep_alive": "1m"
  },
  "query": {"match_all": {}},
  "size": 100,
  "sort": [{"price": "asc"}]
}
```

---

### Search Templates

```http
# Create template
POST /_scripts/product-search-template
Content-Type: application/json

{
  "script": {
    "lang": "mustache",
    "source": {
      "query": {
        "bool": {
          "must": [
            {"match": {"{{field}}": "{{query}}"}}
          ],
          "filter": [
            {"range": {"price": {"gte": {{min_price}}, "lte": {{max_price}}}}}
          ]
        }
      }
    }
  }
}

# Use template
POST /_search/template
Content-Type: application/json

{
  "id": "product-search-template",
  "params": {
    "field": "title",
    "query": "search engine",
    "min_price": 10,
    "max_price": 100
  }
}
```

---

### Explain API (Scoring Details)

```http
GET /products/_explain/1
Content-Type: application/json

{
  "query": {
    "match": {
      "title": "search"
    }
  }
}
```

**Response**:
```json
{
  "matched": true,
  "explanation": {
    "value": 3.14,
    "description": "sum of:",
    "details": [
      {
        "value": 3.14,
        "description": "weight(title:search in 0) [BM25Similarity], result of:",
        "details": [
          {"value": 2.0, "description": "tf, computed as freq / (freq + k1 * (1 - b + b * dl / avgdl))"},
          {"value": 1.57, "description": "idf, computed as log(1 + (N - n + 0.5) / (n + 0.5))"}
        ]
      }
    ]
  }
}
```

---

## Complete Example: E-Commerce Search

```http
POST /products/_search
Content-Type: application/json

{
  "query": {
    "function_score": {
      "query": {
        "bool": {
          "must": [
            {
              "multi_match": {
                "query": "wireless bluetooth headphones",
                "fields": ["title^3", "description", "brand^2"],
                "type": "best_fields"
              }
            }
          ],
          "filter": [
            {"range": {"price": {"gte": 20, "lte": 200}}},
            {"term": {"category": "electronics"}},
            {"term": {"in_stock": true}}
          ],
          "should": [
            {"term": {"brand": {"value": "sony", "boost": 1.5}}},
            {"range": {"rating": {"gte": 4.0, "boost": 1.2}}}
          ]
        }
      },
      "functions": [
        {
          "filter": {"term": {"featured": true}},
          "weight": 1.5
        },
        {
          "field_value_factor": {
            "field": "sales_count",
            "factor": 0.1,
            "modifier": "log1p"
          }
        },
        {
          "gauss": {
            "created_at": {
              "origin": "now",
              "scale": "30d",
              "decay": 0.5
            }
          }
        }
      ],
      "score_mode": "sum",
      "boost_mode": "multiply"
    }
  },
  "aggs": {
    "brands": {
      "terms": {"field": "brand", "size": 10}
    },
    "price_ranges": {
      "range": {
        "field": "price",
        "ranges": [
          {"key": "$0-$50", "to": 50},
          {"key": "$50-$100", "from": 50, "to": 100},
          {"key": "$100-$200", "from": 100, "to": 200},
          {"key": "$200+", "from": 200}
        ]
      }
    },
    "avg_rating": {
      "avg": {"field": "rating"}
    }
  },
  "highlight": {
    "fields": {
      "title": {},
      "description": {}
    },
    "pre_tags": ["<strong>"],
    "post_tags": ["</strong>"]
  },
  "sort": [
    {"_score": "desc"},
    {"sales_count": "desc"}
  ],
  "size": 20,
  "from": 0
}
```

---

## Curl Examples

```bash
# Create index
curl -X PUT "http://quidditch:9200/products" \
  -H 'Content-Type: application/json' \
  -d @index-mapping.json

# Index document
curl -X PUT "http://quidditch:9200/products/_doc/1" \
  -H 'Content-Type: application/json' \
  -d '{"title": "Product 1", "price": 99.99}'

# Bulk index
curl -X POST "http://quidditch:9200/_bulk" \
  -H 'Content-Type: application/x-ndjson' \
  --data-binary @bulk-data.ndjson

# Search
curl -X POST "http://quidditch:9200/products/_search" \
  -H 'Content-Type: application/json' \
  -d '{"query": {"match": {"title": "search"}}}'

# Aggregation
curl -X POST "http://quidditch:9200/products/_search?size=0" \
  -H 'Content-Type: application/json' \
  -d '{"aggs": {"by_category": {"terms": {"field": "category"}}}}'

# Cluster health
curl "http://quidditch:9200/_cluster/health?pretty"
```

---

**Version**: 1.0.0
**Last Updated**: 2026-01-25
**Documentation**: [Full API Reference](https://docs.quidditch.io/api)
