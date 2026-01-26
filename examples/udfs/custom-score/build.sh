#!/bin/bash
# Build script for Custom Score UDF

set -e

echo "Building Custom Score UDF (WAT → WASM)..."

# Check if wat2wasm is installed
if ! command -v wat2wasm &> /dev/null; then
    echo "Error: wat2wasm (WABT) is not installed"
    echo "Install: https://github.com/WebAssembly/wabt"
    echo "Ubuntu: sudo apt-get install wabt"
    exit 1
fi

# Convert WAT to WASM
echo "Converting WAT to WASM..."
wat2wasm custom_score.wat -o custom_score.wasm

# Create output directory
OUTPUT_DIR="./dist"
mkdir -p "$OUTPUT_DIR"
mv custom_score.wasm "$OUTPUT_DIR/"

# Show size
SIZE=$(wc -c < "$OUTPUT_DIR/custom_score.wasm")
echo "✅ Build complete!"
echo "   Output: $OUTPUT_DIR/custom_score.wasm"
echo "   Size: $SIZE bytes"

echo ""
echo "To use this UDF:"
echo "  {\"wasm_udf\": {\"name\": \"custom_score\", \"version\": \"1.0.0\","
echo "   \"parameters\": {\"min_score\": 0.7}}}"
