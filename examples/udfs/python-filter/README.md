# Python UDF Example: Text Similarity Filter

This example demonstrates how to write a User-Defined Function (UDF) in Python for Quidditch.

## Overview

The `text_similarity.py` UDF filters search results based on text similarity using the Levenshtein (edit) distance algorithm. It allows you to find documents where a specific field is "similar" to a query string, not just an exact match.

## UDF Metadata

```python
# @udf: name=text_similarity
# @udf: version=1.0.0
# @udf: author=quidditch
# @udf: category=filter
# @udf: tags=text,similarity,filter,string
```

## Function Signature

```python
def udf_main() -> bool:
    """Returns True if document passes similarity threshold"""
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | The text to compare against |
| `threshold` | i64 | Yes | Maximum edit distance (0 = exact match, higher = more lenient) |

## Document Fields

The UDF accesses the following document fields:
- `title` (string) - The document title to compare

## Usage

### 1. Upload the UDF

**Pre-compiled WASM** (if you have a .wasm file):

```bash
curl -X POST http://localhost:9200/api/v1/udfs \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "text_similarity",
    "version": "1.0.0",
    "description": "Filter by text similarity using Levenshtein distance",
    "category": "filter",
    "author": "quidditch",
    "language": "wasm",
    "function_name": "udf_main",
    "wasm_base64": "<base64-encoded-wasm>",
    "parameters": [
      {
        "name": "query",
        "type": "string",
        "required": true,
        "description": "Text to compare against"
      },
      {
        "name": "threshold",
        "type": "i64",
        "required": true,
        "description": "Maximum edit distance"
      }
    ],
    "returns": [
      {"type": "bool"}
    ],
    "tags": ["text", "similarity", "filter"]
  }'
```

**Python source** (automatic compilation):

```bash
curl -X POST http://localhost:9200/api/v1/udfs \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "text_similarity",
    "version": "1.0.0",
    "language": "python",
    "source": "<base64-encoded-python-source>",
    "metadata": {
      "description": "Filter by text similarity",
      "category": "filter"
    }
  }'
```

### 2. Use in Search Query

```bash
curl -X POST http://localhost:9200/api/v1/indices/products/search \
  -H 'Content-Type: application/json' \
  -d '{
    "query": {
      "bool": {
        "filter": [
          {
            "wasm_udf": {
              "name": "text_similarity",
              "version": "1.0.0",
              "parameters": {
                "query": "laptop computer",
                "threshold": 5
              }
            }
          }
        ]
      }
    }
  }'
```

This will return documents where the `title` field is within 5 edit operations of "laptop computer".

### 3. Test the UDF

```bash
curl -X POST http://localhost:9200/api/v1/udfs/text_similarity/test \
  -H 'Content-Type: application/json' \
  -d '{
    "version": "1.0.0",
    "document": {
      "_id": "doc1",
      "title": "laptop",
      "price": 999
    },
    "parameters": {
      "query": {
        "type": "string",
        "data": "laptop computer"
      },
      "threshold": {
        "type": "i64",
        "data": 10
      }
    }
  }'
```

Expected result:
```json
{
  "name": "text_similarity",
  "version": "1.0.0",
  "results": [true],
  "execution_time": 0.05,
  "document_id": "doc1"
}
```

## Compilation

### Option 1: Pre-compile with External Tools

Python to WASM compilation requires external tooling. Here are the options:

#### A. MicroPython (Recommended for simple UDFs)

**Pros**: Small binary (~500KB), fast startup, simple
**Cons**: Limited standard library, Python 3.4 syntax

```bash
# Install MicroPython WASM toolchain
git clone https://github.com/micropython/micropython.git
cd micropython/ports/webassembly
make

# Compile Python to WASM
micropython -m mpy-cross --target wasm32 -o text_similarity.wasm text_similarity.py
```

#### B. Pyodide (Full Python)

**Pros**: Full Python 3.10+, complete stdlib, numpy/pandas support
**Cons**: Large binary (~15MB), slower startup

```bash
# Install Pyodide
npm install -g pyodide

# Compile Python to WASM
pyodide compile text_similarity.py -o text_similarity.wasm
```

#### C. PyScript

**Pros**: Easy to use, web-based
**Cons**: Requires JavaScript runtime

```bash
# Use PyScript build tools
pyscript build text_similarity.py
```

### Option 2: Upload Python Source (Automatic Compilation)

If Quidditch is configured with a Python compiler, you can upload Python source directly:

```bash
# Upload Python source
python_source=$(base64 -w 0 text_similarity.py)

curl -X POST http://localhost:9200/api/v1/udfs \
  -H 'Content-Type: application/json' \
  -d "{
    \"name\": \"text_similarity\",
    \"version\": \"1.0.0\",
    \"language\": \"python\",
    \"source\": \"$python_source\"
  }"
```

Quidditch will automatically:
1. Extract metadata from docstrings and annotations
2. Compile Python to WASM (if compiler is available)
3. Register the UDF

## Host Functions

Python UDFs can call these host functions provided by Quidditch:

### Document Field Access

- `get_field_string(field_name: str) -> str` - Get string field
- `get_field_int(field_name: str) -> int` - Get integer field
- `get_field_float(field_name: str) -> float` - Get float field
- `get_field_bool(field_name: str) -> bool` - Get boolean field

### Query Parameter Access

- `get_param_string(param_name: str) -> str` - Get string parameter
- `get_param_i64(param_name: str) -> int` - Get integer parameter
- `get_param_f64(param_name: str) -> float` - Get float parameter
- `get_param_bool(param_name: str) -> bool` - Get boolean parameter

### Utility Functions

- `log(message: str)` - Log a debug message
- `get_document_id() -> str` - Get current document ID
- `get_score() -> float` - Get current document's search score

## Algorithm: Levenshtein Distance

The Levenshtein distance (edit distance) is the minimum number of single-character edits (insertions, deletions, or substitutions) required to change one string into another.

**Example**:
- `levenshtein_distance("laptop", "laptop")` → 0 (identical)
- `levenshtein_distance("laptop", "latop")` → 1 (delete 'p')
- `levenshtein_distance("laptop", "desktop")` → 5 (multiple edits)
- `levenshtein_distance("laptop", "laptop computer")` → 9 (append " computer")

**Threshold Guidelines**:
- `0` - Exact match only
- `1-2` - Allow typos (missing/extra character)
- `3-5` - Similar words
- `6-10` - Related words
- `>10` - Different words

## Performance

- **Complexity**: O(n*m) where n and m are string lengths
- **Memory**: O(m) with space optimization
- **Typical Runtime**: ~1-10μs for short strings (< 100 chars)

For better performance with large datasets:
- Pre-filter with a simpler query (e.g., prefix match)
- Use shorter field values
- Increase threshold cautiously

## Limitations

### MicroPython Mode

- Python 3.4 syntax (no f-strings, walrus operators, etc.)
- Limited standard library
- No third-party packages (numpy, pandas, etc.)
- Integer precision limited to WASM i64

### Pyodide Mode

- Large binary size (~15MB first load)
- Slower startup time
- Higher memory usage

### General

- UDFs run in sandboxed WASM environment
- No network access (unless permission granted)
- No file system access (unless permission granted)
- Execution timeout: 5 seconds (default)
- Memory limit: 16MB (default)

## Advanced Usage

### Multiple Fields

```python
def udf_main() -> bool:
    title = get_field_string("title")
    description = get_field_string("description")
    query = get_param_string("query")
    threshold = get_param_i64("threshold")

    title_distance = levenshtein_distance(title, query)
    desc_distance = levenshtein_distance(description, query)

    # Match if either field is similar
    return min(title_distance, desc_distance) <= threshold
```

### Case Insensitive

```python
def udf_main() -> bool:
    title = get_field_string("title").lower()
    query = get_param_string("query").lower()
    threshold = get_param_i64("threshold")

    distance = levenshtein_distance(title, query)
    return distance <= threshold
```

### Scoring

```python
def udf_main() -> float:
    """Return a similarity score instead of boolean"""
    title = get_field_string("title")
    query = get_param_string("query")

    distance = levenshtein_distance(title, query)
    max_len = max(len(title), len(query))

    # Normalize to 0-1 (1 = identical, 0 = completely different)
    similarity = 1.0 - (distance / max_len)
    return similarity
```

## Troubleshooting

### UDF Returns False for All Documents

**Problem**: UDF always returns `False`

**Solutions**:
1. Check field name spelling: `get_field_string("title")` must match document field
2. Verify parameter names: `get_param_string("query")` must match query parameter
3. Add debug logging: `log(f"title={title}, query={query}, distance={distance}")`
4. Test with known document: Use the test endpoint first

### Compilation Errors

**Problem**: Python won't compile to WASM

**Solutions**:
1. Check Python version compatibility (MicroPython = 3.4, Pyodide = 3.10+)
2. Avoid unsupported stdlib modules
3. Use pre-compiled mode and compile manually
4. Check error logs: `docker logs quidditch-coordination`

### Performance Issues

**Problem**: Queries are slow

**Solutions**:
1. Reduce threshold value (fewer documents pass filter)
2. Add a fast pre-filter (term query, range, etc.)
3. Use shorter field values
4. Profile with `execution_time` in test response
5. Consider caching for frequently-used UDFs

## See Also

- [UDF HTTP API Documentation](../../../UDF_HTTP_API_COMPLETE.md)
- [Memory Management & Security](../../../MEMORY_SECURITY_COMPLETE.md)
- [Rust UDF Example](../string-distance/)
- [C UDF Example](../geo-filter/)
- [WAT UDF Example](../custom-score/)

## License

This example is part of the Quidditch project and is licensed under the MIT License.
