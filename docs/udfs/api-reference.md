# UDF API Reference

Complete API reference for Quidditch User-Defined Functions.

## Table of Contents

- [Query API](#query-api)
- [Management API](#management-api)
- [Go SDK](#go-sdk)
- [Host Functions](#host-functions)
- [Data Types](#data-types)
- [Error Codes](#error-codes)

---

## Query API

### UDF Query Syntax

Include UDFs in search queries using the `wasm_udf` clause.

#### Standalone UDF Query

```json
{
  "query": {
    "wasm_udf": {
      "name": "string_distance",
      "version": "1.0.0",
      "parameters": {
        "field": "product_name",
        "target": "iPhone",
        "max_distance": 2
      }
    }
  }
}
```

#### UDF in Bool Query

```json
{
  "query": {
    "bool": {
      "must": [
        {"term": {"category": "electronics"}}
      ],
      "filter": [
        {
          "wasm_udf": {
            "name": "price_filter",
            "version": "1.0.0",
            "parameters": {
              "min": 100,
              "max": 500
            }
          }
        }
      ]
    }
  }
}
```

#### Multiple UDFs

```json
{
  "query": {
    "bool": {
      "must": [
        {"match": {"description": "laptop"}}
      ],
      "filter": [
        {
          "wasm_udf": {
            "name": "price_range",
            "version": "1.0.0",
            "parameters": {"min": 500, "max": 2000}
          }
        },
        {
          "wasm_udf": {
            "name": "geo_filter",
            "version": "1.0.0",
            "parameters": {
              "target_lat": 37.7749,
              "target_lon": -122.4194,
              "max_distance_km": 50
            }
          }
        }
      ]
    }
  }
}
```

### Parameters

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | UDF name |
| `version` | string | Yes | UDF version (semver) |
| `parameters` | object | No | Query-specific parameters |

---

## Management API

### Register UDF

Register a new UDF or update existing version.

**Endpoint**: `POST /api/v1/udfs`

**Request** (multipart/form-data):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | UDF name (alphanumeric + underscore) |
| `version` | string | Yes | Semantic version (e.g., "1.0.0") |
| `function_name` | string | No | WASM export name (default: "filter") |
| `wasm` | file | Yes | WASM binary file |
| `metadata` | JSON | No | Additional metadata |

**metadata JSON**:
```json
{
  "description": "Fuzzy string matching using Levenshtein distance",
  "author": "john@example.com",
  "parameters": [
    {
      "name": "field",
      "type": "string",
      "required": false,
      "default": "name",
      "description": "Field to compare"
    },
    {
      "name": "target",
      "type": "string",
      "required": true,
      "description": "Target string to match"
    },
    {
      "name": "max_distance",
      "type": "i64",
      "required": false,
      "default": 2,
      "description": "Maximum edit distance"
    }
  ],
  "returns": [
    {
      "type": "i32",
      "description": "1 if match, 0 otherwise"
    }
  ],
  "tags": ["fuzzy", "string", "matching"]
}
```

**Response** (200 OK):
```json
{
  "name": "string_distance",
  "version": "1.0.0",
  "function_name": "filter",
  "wasm_size": 2847,
  "registered_at": "2026-01-26T12:00:00Z",
  "status": "active"
}
```

**Example**:
```bash
curl -X POST http://localhost:8080/api/v1/udfs \
  -F 'name=string_distance' \
  -F 'version=1.0.0' \
  -F 'function_name=filter' \
  -F 'wasm=@string_distance.wasm' \
  -F 'metadata={
    "description": "Fuzzy string matching",
    "parameters": [
      {"name": "field", "type": "string"},
      {"name": "target", "type": "string", "required": true},
      {"name": "max_distance", "type": "i64", "default": 2}
    ]
  }'
```

### Get UDF

Retrieve UDF metadata.

**Endpoint**: `GET /api/v1/udfs/{name}/{version}`

**Response** (200 OK):
```json
{
  "name": "string_distance",
  "version": "1.0.0",
  "function_name": "filter",
  "description": "Fuzzy string matching using Levenshtein distance",
  "author": "john@example.com",
  "wasm_size": 2847,
  "parameters": [...],
  "returns": [...],
  "tags": ["fuzzy", "string"],
  "registered_at": "2026-01-26T12:00:00Z",
  "last_used_at": "2026-01-26T14:30:00Z",
  "status": "active"
}
```

### List UDFs

List all registered UDFs.

**Endpoint**: `GET /api/v1/udfs`

**Query Parameters**:
- `name` (optional): Filter by name
- `tag` (optional): Filter by tag
- `page` (optional): Page number (default: 1)
- `size` (optional): Page size (default: 20)

**Response** (200 OK):
```json
{
  "udfs": [
    {
      "name": "string_distance",
      "versions": ["1.0.0", "1.1.0"],
      "latest_version": "1.1.0",
      "description": "Fuzzy string matching",
      "tags": ["fuzzy", "string"]
    },
    {
      "name": "geo_filter",
      "versions": ["1.0.0"],
      "latest_version": "1.0.0",
      "description": "Geographic distance filter",
      "tags": ["geo", "location"]
    }
  ],
  "total": 2,
  "page": 1,
  "size": 20
}
```

### Delete UDF

Delete a specific UDF version.

**Endpoint**: `DELETE /api/v1/udfs/{name}/{version}`

**Response** (204 No Content)

**Example**:
```bash
curl -X DELETE http://localhost:8080/api/v1/udfs/string_distance/1.0.0
```

### Get Statistics

Get UDF execution statistics.

**Endpoint**: `GET /api/v1/udfs/{name}/{version}/stats`

**Response** (200 OK):
```json
{
  "name": "string_distance",
  "version": "1.0.0",
  "statistics": {
    "call_count": 1000000,
    "error_count": 42,
    "error_rate": 0.000042,
    "avg_latency_us": 3.8,
    "p50_latency_us": 3.2,
    "p95_latency_us": 8.5,
    "p99_latency_us": 12.5,
    "p999_latency_us": 45.0,
    "max_latency_us": 127.3,
    "total_execution_time_ms": 3800,
    "last_executed_at": "2026-01-26T14:30:00Z"
  },
  "performance": {
    "rating": "excellent",
    "recommendations": []
  }
}
```

### Reset Statistics

Reset UDF statistics counters.

**Endpoint**: `POST /api/v1/udfs/{name}/{version}/stats/reset`

**Response** (200 OK):
```json
{
  "message": "Statistics reset successfully"
}
```

---

## Go SDK

### Runtime Management

#### Create Runtime

```go
import "github.com/quidditch/quidditch/pkg/wasm"

runtime, err := wasm.NewRuntime(&wasm.Config{
    EnableJIT:      true,              // Enable JIT compilation
    EnableDebug:    false,             // Disable debug mode
    MaxMemoryPages: 256,               // Max 16MB memory (256 * 64KB)
    Logger:         logger,            // zap.Logger instance
})
if err != nil {
    return fmt.Errorf("failed to create runtime: %w", err)
}
defer runtime.Close()
```

**Config Options**:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `EnableJIT` | bool | true | Enable JIT compilation for performance |
| `EnableDebug` | bool | false | Enable debug mode with detailed logging |
| `MaxMemoryPages` | uint32 | 256 | Maximum WASM memory pages (64KB each) |
| `Logger` | *zap.Logger | nil | Logger instance |

### UDF Registry

#### Create Registry

```go
registry, err := wasm.NewUDFRegistry(&wasm.UDFRegistryConfig{
    Runtime:         runtime,          // WASM runtime
    DefaultPoolSize: 10,               // Module instances per UDF
    EnableStats:     true,             // Enable statistics tracking
    Logger:          logger,           // Logger instance
})
if err != nil {
    return fmt.Errorf("failed to create registry: %w", err)
}
```

**Config Options**:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Runtime` | *Runtime | required | WASM runtime instance |
| `DefaultPoolSize` | int | 10 | Pre-instantiated modules per UDF |
| `EnableStats` | bool | true | Track execution statistics |
| `Logger` | *zap.Logger | nil | Logger instance |

#### Register UDF

```go
wasmBytes, err := os.ReadFile("my_udf.wasm")
if err != nil {
    return err
}

err = registry.Register(&wasm.UDFMetadata{
    Name:         "price_filter",
    Version:      "1.0.0",
    FunctionName: "filter",
    Description:  "Filter products by price range",
    Author:       "john@example.com",
    WASMBytes:    wasmBytes,
    Parameters: []wasm.UDFParameter{
        {
            Name:        "min_price",
            Type:        wasm.ValueTypeF64,
            Required:    false,
            Default:     0.0,
            Description: "Minimum price",
        },
        {
            Name:        "max_price",
            Type:        wasm.ValueTypeF64,
            Required:    false,
            Default:     math.MaxFloat64,
            Description: "Maximum price",
        },
    },
    Returns: []wasm.UDFReturnType{
        {
            Type:        wasm.ValueTypeI32,
            Description: "1 if in range, 0 otherwise",
        },
    },
    Tags: []string{"price", "filter", "range"},
})
```

#### Get UDF

```go
udf, err := registry.Get("price_filter", "1.0.0")
if err != nil {
    return fmt.Errorf("UDF not found: %w", err)
}
```

#### Call UDF

```go
// Create document context
docCtx := wasm.NewDocumentContextFromMap(
    "doc123",                          // Document ID
    0.85,                              // Search score
    map[string]interface{}{            // Document fields
        "price": 299.99,
        "category": "electronics",
        "name": "Laptop",
    },
)

// Prepare parameters
params := map[string]wasm.Value{
    "min_price": wasm.NewF64Value(200.0),
    "max_price": wasm.NewF64Value(500.0),
}

// Call UDF
results, err := registry.Call(
    context.Background(),
    "price_filter",
    "1.0.0",
    docCtx,
    params,
)
if err != nil {
    return fmt.Errorf("UDF call failed: %w", err)
}

// Extract result
if len(results) > 0 {
    include, err := results[0].AsInt32()
    if err != nil {
        return fmt.Errorf("invalid result type: %w", err)
    }
    if include == 1 {
        // Include document in results
    }
}
```

#### Get Statistics

```go
stats, err := registry.GetStats("price_filter", "1.0.0")
if err != nil {
    return err
}

fmt.Printf("Call count: %d\n", stats.CallCount)
fmt.Printf("Error count: %d\n", stats.ErrorCount)
fmt.Printf("Avg latency: %.2fμs\n", stats.AvgLatencyUs)
fmt.Printf("P99 latency: %.2fμs\n", stats.P99LatencyUs)
```

### Value Types

#### Creating Values

```go
// Boolean
boolVal := wasm.NewBoolValue(true)

// Integers
i32Val := wasm.NewI32Value(42)
i64Val := wasm.NewI64Value(1234567890)

// Floats
f32Val := wasm.NewF32Value(3.14)
f64Val := wasm.NewF64Value(2.718281828)

// String
strVal := wasm.NewStringValue("hello")
```

#### Extracting Values

```go
// From i32
i32Val, err := value.AsInt32()

// From i64
i64Val, err := value.AsInt64()

// From f32
f32Val, err := value.AsFloat32()

// From f64
f64Val, err := value.AsFloat64()

// From bool
boolVal, err := value.AsBool()

// From string
strVal, err := value.AsString()
```

### Document Context

```go
// From map
ctx := wasm.NewDocumentContextFromMap(
    "doc_id",
    0.95,  // score
    map[string]interface{}{
        "name": "Product",
        "price": 99.99,
        "tags": []string{"new", "sale"},
    },
)

// From JSON
jsonData := []byte(`{"name":"Product","price":99.99}`)
ctx, err := wasm.NewDocumentContextFromJSON("doc_id", 0.95, jsonData)
```

---

## Host Functions

Complete reference of functions available to UDFs.

### Field Access Functions

#### has_field

```c
int has_field(i64 ctx_id, char* field_name, int name_len)
```

Check if document has a field.

**Parameters**:
- `ctx_id`: Document context ID
- `field_name`: Pointer to field name string
- `name_len`: Length of field name

**Returns**:
- `1` if field exists
- `0` if field does not exist

**Example**:
```rust
let has = has_field(ctx_id, "price".as_ptr(), 5);
if has == 0 {
    return 0; // Field missing
}
```

#### get_field_string

```c
int get_field_string(i64 ctx_id, char* field_name, int name_len,
                     char* out_buffer, int* out_len)
```

Get string field value.

**Parameters**:
- `ctx_id`: Document context ID
- `field_name`: Field name pointer
- `name_len`: Field name length
- `out_buffer`: Output buffer pointer
- `out_len`: Input: buffer size, Output: actual string length

**Returns**:
- `0` on success
- `-1` on error (field missing, wrong type, buffer too small)

**Example**:
```rust
let mut buffer = [0u8; 256];
let mut len = 256;
if get_field_string(ctx_id, "name".as_ptr(), 4,
                    buffer.as_mut_ptr(), &mut len) == 0 {
    let name = core::str::from_utf8(&buffer[..len as usize]).unwrap();
    // Use name
}
```

#### get_field_i64

```c
int get_field_i64(i64 ctx_id, char* field_name, int name_len, i64* out)
```

Get integer field value.

**Returns**: `0` on success, `-1` on error

#### get_field_f64

```c
int get_field_f64(i64 ctx_id, char* field_name, int name_len, double* out)
```

Get floating-point field value.

**Returns**: `0` on success, `-1` on error

#### get_field_bool

```c
int get_field_bool(i64 ctx_id, char* field_name, int name_len, int* out)
```

Get boolean field value (0 or 1).

**Returns**: `0` on success, `-1` on error

### Parameter Access Functions

#### get_param_string

```c
int get_param_string(char* param_name, int name_len,
                     char* out_buffer, int* out_len)
```

Get query parameter as string.

**Returns**: `0` on success, `-1` if parameter not provided

#### get_param_i64

```c
int get_param_i64(char* param_name, int name_len, i64* out)
```

Get query parameter as integer.

**Returns**: `0` on success, `-1` if parameter not provided

#### get_param_f64

```c
int get_param_f64(char* param_name, int name_len, double* out)
```

Get query parameter as float.

**Returns**: `0` on success, `-1` if parameter not provided

#### get_param_bool

```c
int get_param_bool(char* param_name, int name_len, int* out)
```

Get query parameter as boolean (0 or 1).

**Returns**: `0` on success, `-1` if parameter not provided

### Utility Functions

#### log

```c
void log(int level, char* message, int msg_len)
```

Log message for debugging.

**Parameters**:
- `level`: Log level
  - `0` = DEBUG
  - `1` = INFO
  - `2` = WARN
  - `3` = ERROR
- `message`: Message pointer
- `msg_len`: Message length

**Example**:
```rust
let msg = "Processing document";
unsafe {
    log(1, msg.as_ptr(), msg.len() as i32);
}
```

---

## Data Types

### Value Types

| Type | WASM Type | Size | Go Type | Description |
|------|-----------|------|---------|-------------|
| `ValueTypeBool` | i32 | 4 bytes | bool | Boolean (0 or 1) |
| `ValueTypeI32` | i32 | 4 bytes | int32 | 32-bit signed integer |
| `ValueTypeI64` | i64 | 8 bytes | int64 | 64-bit signed integer |
| `ValueTypeF32` | f32 | 4 bytes | float32 | 32-bit float |
| `ValueTypeF64` | f64 | 8 bytes | float64 | 64-bit float |
| `ValueTypeString` | - | variable | string | UTF-8 string |

### UDFMetadata

Complete UDF metadata structure:

```go
type UDFMetadata struct {
    // Basic info
    Name        string `json:"name"`
    Version     string `json:"version"`
    Description string `json:"description"`
    Author      string `json:"author"`

    // Function signature
    FunctionName string            `json:"function_name"`
    Parameters   []UDFParameter    `json:"parameters"`
    Returns      []UDFReturnType   `json:"returns"`

    // WASM module
    WASMBytes []byte `json:"-"`
    WASMSize  int    `json:"wasm_size"`

    // Performance hints
    ExpectedLatency time.Duration `json:"expected_latency,omitempty"`
    MemoryRequired  uint32        `json:"memory_required,omitempty"`

    // Metadata
    Tags        []string  `json:"tags,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    LastUsedAt  time.Time `json:"last_used_at,omitempty"`
}
```

### UDFParameter

```go
type UDFParameter struct {
    Name        string      `json:"name"`
    Type        ValueType   `json:"type"`
    Required    bool        `json:"required"`
    Default     interface{} `json:"default,omitempty"`
    Description string      `json:"description,omitempty"`
}
```

### UDFReturnType

```go
type UDFReturnType struct {
    Name        string    `json:"name,omitempty"`
    Type        ValueType `json:"type"`
    Description string    `json:"description,omitempty"`
}
```

### UDFStats

```go
type UDFStats struct {
    CallCount       uint64        `json:"call_count"`
    ErrorCount      uint64        `json:"error_count"`
    AvgLatencyUs    float64       `json:"avg_latency_us"`
    P50LatencyUs    float64       `json:"p50_latency_us"`
    P95LatencyUs    float64       `json:"p95_latency_us"`
    P99LatencyUs    float64       `json:"p99_latency_us"`
    P999LatencyUs   float64       `json:"p999_latency_us"`
    MaxLatencyUs    float64       `json:"max_latency_us"`
    TotalExecTimeMs float64       `json:"total_execution_time_ms"`
    LastExecutedAt  time.Time     `json:"last_executed_at"`
}
```

---

## Error Codes

### HTTP Status Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | UDF registered successfully |
| 204 | UDF deleted successfully |
| 400 | Bad request (invalid parameters) |
| 404 | UDF not found |
| 409 | Conflict (UDF version already exists) |
| 413 | WASM binary too large |
| 422 | Invalid WASM binary |
| 500 | Internal server error |
| 503 | Service unavailable (registry full) |

### Error Response Format

```json
{
  "error": {
    "code": "UDF_NOT_FOUND",
    "message": "UDF 'price_filter' version '1.0.0' not found",
    "details": {
      "name": "price_filter",
      "version": "1.0.0"
    }
  }
}
```

### Error Codes

| Code | HTTP | Description |
|------|------|-------------|
| `UDF_NOT_FOUND` | 404 | UDF not registered |
| `UDF_ALREADY_EXISTS` | 409 | Version already registered |
| `INVALID_WASM` | 422 | WASM binary invalid |
| `INVALID_PARAMETERS` | 400 | Invalid query parameters |
| `COMPILATION_FAILED` | 422 | Failed to compile WASM |
| `EXECUTION_FAILED` | 500 | UDF execution error |
| `TIMEOUT` | 504 | UDF execution timeout |
| `MEMORY_LIMIT` | 500 | Memory limit exceeded |
| `REGISTRY_FULL` | 503 | Too many UDFs registered |

---

## Rate Limits

| Operation | Limit | Period |
|-----------|-------|--------|
| Register UDF | 10 | per minute |
| Delete UDF | 10 | per minute |
| Query with UDF | 1000 | per second |
| Get Stats | 100 | per second |

Rate limit headers:
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1643212800
```

---

## Versioning

API version is included in the URL:
```
/api/v1/udfs
```

Current version: **v1**

Breaking changes will increment major version (v2, v3, etc.)

---

## See Also

- [Writing UDFs Guide](./writing-udfs.md)
- [Performance Guide](./performance-guide.md)
- [Migration Guide](./migration-guide.md)
- [Examples](../../examples/udfs/)
