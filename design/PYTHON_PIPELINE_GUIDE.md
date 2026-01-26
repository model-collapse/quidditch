# Quidditch Python Pipeline Development Guide

**Version**: 1.0.0
**Date**: 2026-01-25

---

## Table of Contents

1. [Overview](#overview)
2. [Pipeline Architecture](#pipeline-architecture)
3. [Getting Started](#getting-started)
4. [Processor Types](#processor-types)
5. [API Reference](#api-reference)
6. [Examples](#examples)
7. [Testing](#testing)
8. [Deployment](#deployment)
9. [Best Practices](#best-practices)
10. [Troubleshooting](#troubleshooting)

---

## Overview

Quidditch Python Pipelines allow you to customize search behavior using Python code. Pipelines execute at two points:

1. **Pre-Processing**: Before query execution (query rewriting, expansion, filtering)
2. **Post-Processing**: After query execution (re-ranking, highlighting, transformations)

**Use Cases**:
- Synonym expansion
- Spell correction
- Query understanding (intent classification)
- ML-based re-ranking
- Access control (filter results by user permissions)
- Response transformation
- A/B testing

---

## Pipeline Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    Search Request Flow                        │
└──────────────────────────────────────────────────────────────┘

Client Request
      ↓
┌─────────────────────────────────────┐
│  1. Pre-Processing Processors       │
│     • Query rewriting                │
│     • Synonym expansion              │
│     • Spell correction               │
│     • Access control                 │
└─────────────────────────────────────┘
      ↓
┌─────────────────────────────────────┐
│  2. Query Execution (Diagon)        │
│     • Parse DSL/PPL                  │
│     • Execute on data nodes          │
│     • Aggregate results              │
└─────────────────────────────────────┘
      ↓
┌─────────────────────────────────────┐
│  3. Post-Processing Processors      │
│     • ML re-ranking                  │
│     • Highlighting                   │
│     • Result filtering               │
│     • Response transformation        │
└─────────────────────────────────────┘
      ↓
Client Response
```

---

## Getting Started

### Installation

```bash
# Install Quidditch Python SDK
pip install quidditch-sdk

# Verify installation
python -c "import quidditch; print(quidditch.__version__)"
```

### Project Structure

```
my-pipeline/
├── README.md
├── requirements.txt
├── setup.py
├── my_pipeline/
│   ├── __init__.py
│   ├── processors.py
│   ├── config.py
│   └── utils.py
└── tests/
    ├── __init__.py
    └── test_processors.py
```

### Basic Pipeline

```python
# my_pipeline/processors.py

from quidditch.pipeline import Processor

class HelloProcessor(Processor):
    """Simple processor that logs requests"""

    def process_request(self, request):
        """Pre-processing: modify request before execution"""
        print(f"Processing query: {request.query}")
        return request

    def process_response(self, response, request):
        """Post-processing: modify response after execution"""
        print(f"Got {response.hits.total.value} results")
        return response
```

### Registration

```python
# my_pipeline/__init__.py

from quidditch.pipeline import SearchPipeline
from .processors import HelloProcessor

pipeline = SearchPipeline(
    name="hello-pipeline",
    description="Simple hello pipeline",
    processors=[
        HelloProcessor()
    ]
)
```

---

## Processor Types

### 1. Request Processor

Modifies the search request before execution.

```python
from quidditch.pipeline import Processor

class RequestProcessor(Processor):
    """Base class for request processors"""

    def process_request(self, request):
        """
        Args:
            request (SearchRequest): Original request

        Returns:
            SearchRequest: Modified request
        """
        # Modify request.query, request.filters, etc.
        return request
```

**Common Use Cases**:
- Query rewriting
- Synonym expansion
- Query expansion (add clauses)
- Filter injection (access control)

---

### 2. Response Processor

Modifies the search response after execution.

```python
from quidditch.pipeline import Processor

class ResponseProcessor(Processor):
    """Base class for response processors"""

    def process_response(self, response, request):
        """
        Args:
            response (SearchResponse): Search results
            request (SearchRequest): Original request

        Returns:
            SearchResponse: Modified response
        """
        # Modify response.hits, add fields, etc.
        return response
```

**Common Use Cases**:
- Re-ranking results
- Result filtering (post-query)
- Highlighting
- Response transformation
- Analytics tracking

---

### 3. Hybrid Processor

Implements both request and response processing.

```python
from quidditch.pipeline import Processor

class HybridProcessor(Processor):
    """Processor with both request and response logic"""

    def process_request(self, request):
        # Store state for response processing
        self.original_query = request.query
        return request

    def process_response(self, response, request):
        # Use stored state
        print(f"Original query: {self.original_query}")
        return response
```

---

## API Reference

### SearchRequest

```python
class SearchRequest:
    """Represents a search request"""

    # Query DSL
    query: Dict[str, Any]

    # Filters (applied without scoring)
    filters: List[Dict[str, Any]]

    # Pagination
    size: int
    from_: int

    # Sorting
    sort: List[Dict[str, Any]]

    # Aggregations
    aggs: Dict[str, Any]

    # Source filtering
    _source: Union[bool, List[str], Dict[str, Any]]

    # Highlighting
    highlight: Dict[str, Any]

    # Index name
    index: str

    # User context
    user: Optional[UserContext]

    # Custom attributes
    attributes: Dict[str, Any]
```

**Example**:
```python
def process_request(self, request):
    # Add filter
    if not request.filters:
        request.filters = []
    request.filters.append({
        "term": {"status": "published"}
    })

    # Modify query
    if "match" in request.query:
        field, text = next(iter(request.query["match"].items()))
        request.query = {
            "bool": {
                "should": [
                    {"match": {field: text}},
                    {"match": {field: {"query": text, "boost": 0.5}}}
                ]
            }
        }

    return request
```

---

### SearchResponse

```python
class SearchResponse:
    """Represents a search response"""

    # Results
    hits: Hits

    # Aggregations
    aggregations: Dict[str, Any]

    # Metadata
    took: int  # milliseconds
    timed_out: bool
    _shards: ShardInfo

class Hits:
    """Search hits"""

    total: Total  # total.value, total.relation
    max_score: Optional[float]
    hits: List[Hit]

class Hit:
    """Single search result"""

    _index: str
    _id: str
    _score: float
    _source: Dict[str, Any]
    highlight: Optional[Dict[str, List[str]]]
    sort: Optional[List[Any]]
```

**Example**:
```python
def process_response(self, response, request):
    # Re-rank results
    for hit in response.hits.hits:
        # Custom scoring
        hit._score = self.compute_custom_score(hit, request)

    # Re-sort by new score
    response.hits.hits.sort(key=lambda h: h._score, reverse=True)

    # Update max score
    if response.hits.hits:
        response.hits.max_score = response.hits.hits[0]._score

    return response
```

---

### UserContext

```python
class UserContext:
    """User context for access control"""

    user_id: str
    username: str
    roles: List[str]
    permissions: List[str]
    attributes: Dict[str, Any]
```

**Example**:
```python
def process_request(self, request):
    if request.user:
        # Filter by user's department
        request.filters.append({
            "term": {"department": request.user.attributes.get("department")}
        })
    return request
```

---

## Examples

### Example 1: Synonym Expansion

```python
from quidditch.pipeline import Processor
import json

class SynonymExpansionProcessor(Processor):
    """Expand query with synonyms"""

    def __init__(self, synonym_file="synonyms.json"):
        with open(synonym_file) as f:
            self.synonyms = json.load(f)

    def process_request(self, request):
        if "match" in request.query:
            field, text = next(iter(request.query["match"].items()))
            terms = text.lower().split()

            # Find synonyms
            expanded_terms = []
            for term in terms:
                expanded_terms.append(term)
                if term in self.synonyms:
                    expanded_terms.extend(self.synonyms[term])

            # Build expanded query
            request.query = {
                "bool": {
                    "should": [
                        {"match": {field: text}},  # Original
                        {"match": {field: " ".join(expanded_terms)}}  # Expanded
                    ],
                    "minimum_should_match": 1
                }
            }

        return request
```

**synonyms.json**:
```json
{
  "search": ["find", "lookup", "query"],
  "fast": ["quick", "rapid", "speedy"],
  "database": ["db", "datastore", "storage"]
}
```

---

### Example 2: ML Re-Ranking with ONNX

```python
from quidditch.pipeline import Processor
import onnxruntime as ort
import numpy as np

class MLRerankProcessor(Processor):
    """Re-rank results using ONNX model"""

    def __init__(self, model_path):
        self.session = ort.InferenceSession(model_path)
        self.input_name = self.session.get_inputs()[0].name

    def process_response(self, response, request):
        hits = response.hits.hits

        if not hits:
            return response

        # Extract features
        features = []
        for hit in hits:
            features.append(self.extract_features(hit, request))

        # Run inference
        features_array = np.array(features, dtype=np.float32)
        scores = self.session.run(None, {self.input_name: features_array})[0]

        # Update scores
        for hit, score in zip(hits, scores):
            hit._score = float(score)

        # Re-sort
        hits.sort(key=lambda x: x._score, reverse=True)
        response.hits.max_score = hits[0]._score if hits else None

        return response

    def extract_features(self, hit, request):
        """Extract features for ML model"""
        source = hit._source

        return [
            hit._score,  # BM25 score
            source.get("view_count", 0),
            source.get("like_count", 0),
            source.get("comment_count", 0),
            self.text_similarity(request.query, source.get("title", "")),
            self.freshness_score(source.get("timestamp")),
        ]

    def text_similarity(self, query_dict, title):
        """Simple overlap similarity"""
        # Extract query text
        if "match" in query_dict:
            query_text = next(iter(query_dict["match"].values()))
        else:
            query_text = ""

        query_terms = set(str(query_text).lower().split())
        title_terms = set(title.lower().split())

        if not query_terms:
            return 0.0

        return len(query_terms & title_terms) / len(query_terms)

    def freshness_score(self, timestamp):
        """Exponential decay based on age"""
        from datetime import datetime
        if not timestamp:
            return 0.0

        now = datetime.utcnow()
        doc_time = datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
        age_days = (now - doc_time).days

        # 30-day half-life
        import math
        return math.exp(-age_days / 30)
```

---

### Example 3: Access Control

```python
from quidditch.pipeline import Processor

class AccessControlProcessor(Processor):
    """Filter results based on user permissions"""

    def process_request(self, request):
        """Inject access control filters"""
        if request.user:
            # User can only see their own documents or public ones
            request.filters.append({
                "bool": {
                    "should": [
                        {"term": {"owner": request.user.user_id}},
                        {"term": {"visibility": "public"}}
                    ],
                    "minimum_should_match": 1
                }
            })

        return request

    def process_response(self, response, request):
        """Post-filter sensitive fields"""
        if not request.user or "admin" not in request.user.roles:
            # Remove sensitive fields for non-admins
            for hit in response.hits.hits:
                hit._source.pop("internal_notes", None)
                hit._source.pop("salary", None)

        return response
```

---

### Example 4: A/B Testing

```python
from quidditch.pipeline import Processor
import random

class ABTestProcessor(Processor):
    """A/B test different ranking algorithms"""

    def __init__(self, variant_weights=None):
        self.variants = variant_weights or {"control": 0.5, "variant_a": 0.5}

    def process_request(self, request):
        """Assign user to A/B test variant"""
        variant = self.select_variant()
        request.attributes["ab_variant"] = variant

        if variant == "variant_a":
            # Boost recent documents in variant A
            if "function_score" not in request.query:
                request.query = {
                    "function_score": {
                        "query": request.query,
                        "functions": [
                            {
                                "gauss": {
                                    "timestamp": {
                                        "origin": "now",
                                        "scale": "7d",
                                        "decay": 0.5
                                    }
                                }
                            }
                        ]
                    }
                }

        return request

    def process_response(self, response, request):
        """Track metrics for A/B test"""
        variant = request.attributes.get("ab_variant", "control")

        # Log metrics (send to analytics service)
        self.log_metrics({
            "variant": variant,
            "query": str(request.query),
            "num_results": response.hits.total.value,
            "latency_ms": response.took
        })

        return response

    def select_variant(self):
        """Randomly select variant based on weights"""
        r = random.random()
        cumulative = 0
        for variant, weight in self.variants.items():
            cumulative += weight
            if r < cumulative:
                return variant
        return "control"

    def log_metrics(self, metrics):
        """Send metrics to analytics (stub)"""
        # TODO: Send to Prometheus, Datadog, etc.
        print(f"AB Test Metrics: {metrics}")
```

---

## Testing

### Unit Testing

```python
# tests/test_processors.py

import unittest
from my_pipeline.processors import SynonymExpansionProcessor
from quidditch.pipeline import SearchRequest

class TestSynonymExpansion(unittest.TestCase):

    def setUp(self):
        self.processor = SynonymExpansionProcessor("synonyms.json")

    def test_expands_synonyms(self):
        request = SearchRequest(
            query={"match": {"title": "fast search"}}
        )

        result = self.processor.process_request(request)

        # Check that query was expanded
        self.assertIn("bool", result.query)
        self.assertEqual(len(result.query["bool"]["should"]), 2)

    def test_no_expansion_for_unknown_terms(self):
        request = SearchRequest(
            query={"match": {"title": "unknown_term"}}
        )

        result = self.processor.process_request(request)

        # Should still wrap in bool query
        self.assertIn("bool", result.query)

if __name__ == "__main__":
    unittest.main()
```

### Integration Testing

```python
# tests/test_integration.py

import unittest
from quidditch.testing import TestCluster
from my_pipeline import pipeline

class TestPipelineIntegration(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        # Start test cluster
        cls.cluster = TestCluster()
        cls.cluster.start()

        # Deploy pipeline
        cls.cluster.deploy_pipeline(pipeline)

        # Index test data
        cls.cluster.bulk_index("test-index", [
            {"title": "Fast search engine", "id": 1},
            {"title": "Quick database lookup", "id": 2},
            {"title": "Rapid data retrieval", "id": 3},
        ])

    @classmethod
    def tearDownClass(cls):
        cls.cluster.stop()

    def test_synonym_expansion_works(self):
        response = self.cluster.search(
            index="test-index",
            query={"match": {"title": "fast search"}},
            pipeline="hello-pipeline"
        )

        # Should find all 3 documents (fast, quick, rapid are synonyms)
        self.assertEqual(response.hits.total.value, 3)

if __name__ == "__main__":
    unittest.main()
```

---

## Deployment

### Deploy to Cluster

```bash
# Package pipeline
cd my-pipeline
python setup.py sdist

# Upload to cluster
quidditch pipeline deploy \
  --cluster quidditch-prod \
  --namespace quidditch \
  --package dist/my-pipeline-1.0.0.tar.gz

# Verify deployment
quidditch pipeline list --cluster quidditch-prod
```

### REST API

```http
# Deploy via REST API
PUT /_search/pipeline/my-pipeline
{
  "description": "My custom pipeline",
  "processors": [
    {
      "type": "python",
      "module": "my_pipeline",
      "processor": "SynonymExpansionProcessor",
      "settings": {
        "synonym_file": "/etc/quidditch/synonyms.json"
      }
    }
  ]
}

# Get pipeline
GET /_search/pipeline/my-pipeline

# Delete pipeline
DELETE /_search/pipeline/my-pipeline
```

### Kubernetes ConfigMap

```yaml
# pipeline-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-pipeline
  namespace: quidditch
data:
  processors.py: |
    from quidditch.pipeline import Processor

    class HelloProcessor(Processor):
        def process_request(self, request):
            print(f"Hello from {request.index}")
            return request
```

---

## Best Practices

### 1. Performance

- **Avoid Blocking I/O**: Use async/await for external calls
- **Cache Expensive Operations**: Cache model predictions, API calls
- **Limit Iterations**: Don't loop over all hits unnecessarily
- **Use Vectorization**: NumPy for batch operations
- **Profile Code**: Use cProfile to identify bottlenecks

```python
import functools

# Cache decorator
@functools.lru_cache(maxsize=1000)
def get_synonyms(term):
    """Cached synonym lookup"""
    # Expensive operation
    return fetch_from_api(term)
```

---

### 2. Error Handling

- **Graceful Degradation**: Pipeline errors shouldn't break queries
- **Logging**: Log errors with context
- **Timeouts**: Set timeouts for external calls
- **Fallbacks**: Have default behavior if pipeline fails

```python
from quidditch.pipeline import Processor
import logging

logger = logging.getLogger(__name__)

class SafeProcessor(Processor):

    def process_request(self, request):
        try:
            # Pipeline logic
            return self.transform(request)
        except Exception as e:
            logger.error(f"Pipeline failed: {e}", exc_info=True)
            # Return original request (fallback)
            return request
```

---

### 3. Security

- **Validate Inputs**: Check user inputs for injection attacks
- **Sanitize Outputs**: Don't leak sensitive data
- **Resource Limits**: Prevent runaway CPU/memory usage
- **Least Privilege**: Only request necessary permissions

```python
def process_request(self, request):
    # Validate query size
    if len(str(request.query)) > 10000:
        raise ValueError("Query too large")

    # Sanitize user input
    if request.user:
        user_id = request.user.user_id
        if not user_id.isalnum():
            raise ValueError("Invalid user ID")

    return request
```

---

### 4. Testing

- **Unit Tests**: Test processors in isolation
- **Integration Tests**: Test with real cluster
- **Load Tests**: Test under realistic load
- **A/B Tests**: Validate improvements with experiments

---

## Troubleshooting

### Pipeline Not Executing

```bash
# Check pipeline registration
curl http://quidditch:9200/_search/pipeline/my-pipeline

# Check coordination node logs
kubectl logs -n quidditch <coord-pod> | grep pipeline

# Verify Python environment
kubectl exec -n quidditch <coord-pod> -- python -c "import my_pipeline"
```

### High Latency

```python
# Add timing logs
import time

def process_request(self, request):
    start = time.time()
    result = self.transform(request)
    elapsed = time.time() - start
    print(f"Pipeline took {elapsed*1000:.2f}ms")
    return result
```

### Memory Leaks

```bash
# Monitor memory usage
kubectl top pods -n quidditch

# Profile Python code
python -m memory_profiler my_script.py
```

---

## Reference

- [Quidditch Architecture](QUIDDITCH_ARCHITECTURE.md)
- [API Reference](https://docs.quidditch.io/api)
- [Examples Repository](https://github.com/quidditch/examples)

---

**Version**: 1.0.0
**Last Updated**: 2026-01-25
