#!/bin/bash
# Build script for String Distance UDF

set -e

echo "Building String Distance UDF (Rust → WASM)..."

# Check if Rust is installed
if ! command -v rustc &> /dev/null; then
    echo "Error: Rust is not installed. Install from https://rustup.rs/"
    exit 1
fi

# Check if wasm32 target is installed
if ! rustup target list | grep -q "wasm32-unknown-unknown (installed)"; then
    echo "Installing wasm32-unknown-unknown target..."
    rustup target add wasm32-unknown-unknown
fi

# Build for WASM
echo "Compiling to WASM..."
cargo build --target wasm32-unknown-unknown --release

# Copy output
OUTPUT_DIR="./dist"
mkdir -p "$OUTPUT_DIR"
cp target/wasm32-unknown-unknown/release/string_distance_udf.wasm "$OUTPUT_DIR/string_distance.wasm"

# Show size
SIZE=$(wc -c < "$OUTPUT_DIR/string_distance.wasm")
echo "✅ Build complete!"
echo "   Output: $OUTPUT_DIR/string_distance.wasm"
echo "   Size: $SIZE bytes"

# Optional: Optimize with wasm-opt (if installed)
if command -v wasm-opt &> /dev/null; then
    echo "Optimizing with wasm-opt..."
    wasm-opt -Oz "$OUTPUT_DIR/string_distance.wasm" -o "$OUTPUT_DIR/string_distance.opt.wasm"
    OPT_SIZE=$(wc -c < "$OUTPUT_DIR/string_distance.opt.wasm")
    echo "   Optimized: $OPT_SIZE bytes (saved $((SIZE - OPT_SIZE)) bytes)"
    mv "$OUTPUT_DIR/string_distance.opt.wasm" "$OUTPUT_DIR/string_distance.wasm"
fi

echo ""
echo "To use this UDF:"
echo "  1. Register with Quidditch:"
echo "     curl -X POST http://localhost:8080/api/v1/udfs \\"
echo "       -H 'Content-Type: multipart/form-data' \\"
echo "       -F 'name=string_distance' \\"
echo "       -F 'version=1.0.0' \\"
echo "       -F 'wasm=@$OUTPUT_DIR/string_distance.wasm'"
echo ""
echo "  2. Use in query:"
echo "     {\"wasm_udf\": {\"name\": \"string_distance\", \"version\": \"1.0.0\","
echo "      \"parameters\": {\"field\": \"name\", \"target\": \"iPhone\", \"max_distance\": 2}}}"
