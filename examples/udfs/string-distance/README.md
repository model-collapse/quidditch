# String Distance UDF

Fuzzy string matching UDF using Levenshtein distance for typo-tolerant search.

## Overview

This UDF allows you to match documents where a field value is "close" to a target string, within a specified edit distance. Perfect for handling typos, variations, and approximate matches.

## Algorithm

Uses **Levenshtein distance** (edit distance) to measure similarity:
- Counts minimum number of single-character edits (insertions, deletions, substitutions)
- Optimized 2-row implementation for memory efficiency
- O(m×n) time complexity where m, n are string lengths

## Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `field` | string | No | "name" | Document field to compare |
| `target` | string | Yes | - | Target string to match against |
| `max_distance` | integer | No | 2 | Maximum edit distance to allow |

## Examples

### Basic Usage

Match product names within 2 edits of "iPhone":

```json
{
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
```

**Matches**:
- "iPhone" (distance: 0)
- "IPhone" (distance: 1)
- "iphon" (distance: 1)
- "iPone" (distance: 1)
- "iPhones" (distance: 1)

**Doesn't Match**:
- "Android" (distance: 7)
- "Samsung" (distance: 7)

### Combined with Bool Query

Find electronics with names similar to "MacBook":

```json
{
  "bool": {
    "must": [
      {"term": {"category": "electronics"}}
    ],
    "filter": [
      {
        "wasm_udf": {
          "name": "string_distance",
          "version": "1.0.0",
          "parameters": {
            "field": "product_name",
            "target": "MacBook",
            "max_distance": 3
          }
        }
      }
    ]
  }
}
```

### Strict Matching

Allow only 1-character differences:

```json
{
  "wasm_udf": {
    "name": "string_distance",
    "parameters": {
      "field": "title",
      "target": "Neural Networks",
      "max_distance": 1
    }
  }
}
```

## Building

### Prerequisites

- Rust toolchain (install from [rustup.rs](https://rustup.rs))
- wasm32 target: `rustup target add wasm32-unknown-unknown`
- (Optional) wasm-opt from [binaryen](https://github.com/WebAssembly/binaryen)

### Build Commands

```bash
# Quick build
./build.sh

# Or manually
cargo build --target wasm32-unknown-unknown --release
cp target/wasm32-unknown-unknown/release/string_distance_udf.wasm dist/
```

### Output

- **Unoptimized**: ~15-20 KB
- **Optimized** (with wasm-opt -Oz): ~2-3 KB

## Registration

### Via API

```bash
curl -X POST http://localhost:8080/api/v1/udfs \
  -H 'Content-Type: multipart/form-data' \
  -F 'name=string_distance' \
  -F 'version=1.0.0' \
  -F 'function_name=filter' \
  -F 'wasm=@dist/string_distance.wasm' \
  -F 'metadata={
    "description": "Fuzzy string matching using Levenshtein distance",
    "parameters": [
      {"name": "field", "type": "string", "required": false, "default": "name"},
      {"name": "target", "type": "string", "required": true},
      {"name": "max_distance", "type": "i64", "required": false, "default": 2}
    ],
    "returns": [{"type": "i32", "description": "1 if match, 0 otherwise"}]
  }'
```

### Programmatically (Go)

```go
wasmBytes, _ := os.ReadFile("dist/string_distance.wasm")

err := registry.Register(&wasm.UDFMetadata{
    Name:         "string_distance",
    Version:      "1.0.0",
    FunctionName: "filter",
    Description:  "Fuzzy string matching using Levenshtein distance",
    WASMBytes:    wasmBytes,
    Parameters: []wasm.UDFParameter{
        {Name: "field", Type: wasm.ValueTypeString, Default: "name"},
        {Name: "target", Type: wasm.ValueTypeString, Required: true},
        {Name: "max_distance", Type: wasm.ValueTypeI64, Default: int64(2)},
    },
    Returns: []wasm.UDFReturnType{
        {Type: wasm.ValueTypeI32, Description: "1 if match, 0 otherwise"},
    },
})
```

## Use Cases

1. **Typo Tolerance**: Handle common spelling mistakes in search
2. **Name Variations**: Match "Jon" vs "John", "Smith" vs "Smyth"
3. **OCR Errors**: Search documents with OCR-induced typos
4. **Transliteration**: Match names across different romanization systems
5. **Fuzzy Product Search**: "iPhome" → "iPhone"

## Performance

- **Distance Calculation**: O(m×n) where m, n are string lengths
- **Memory**: 2 rows × target length (constant workspace)
- **Typical Latency**:
  - Short strings (≤10 chars): ~1μs
  - Medium strings (≤50 chars): ~5μs
  - Long strings (≤200 chars): ~50μs

**Optimization Tips**:
- Set reasonable `max_distance` (typically 1-3)
- Use as filter in bool query (not standalone)
- Consider field length limits
- Combine with term queries for better selectivity

## Limitations

1. **Case Sensitive**: "iPhone" ≠ "iphone" (distance: 1)
   - Workaround: Normalize in application or add lowercase param
2. **No Unicode Normalization**: "café" ≠ "cafe" with combining accents
3. **Computational Cost**: Scales with string length × max_distance
4. **No Phonetic Matching**: "Smith" vs "Smyth" (distance: 2)

## Future Enhancements

- [ ] Case-insensitive mode parameter
- [ ] Unicode normalization support
- [ ] Damerau-Levenshtein (transpositions)
- [ ] Phonetic distance (Soundex, Metaphone)
- [ ] Configurable early termination
- [ ] Multi-field support

## Related

- **Geo Distance UDF**: Distance-based filtering for coordinates
- **Regex UDF**: Pattern matching (more flexible, less fuzzy)
- **N-gram UDF**: Token-based similarity

## License

MIT License - Part of Quidditch Search Engine examples
