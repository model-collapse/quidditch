#!/bin/bash
# Build script for Geo Filter UDF

set -e

echo "Building Geo Filter UDF (C → WASM)..."

# Check if clang is installed
if ! command -v clang &> /dev/null; then
    echo "Error: clang is not installed"
    echo "Install: sudo apt-get install clang"
    exit 1
fi

# Compile to WASM
echo "Compiling to WASM..."
clang \
    --target=wasm32 \
    -nostdlib \
    -Wl,--no-entry \
    -Wl,--export-dynamic \
    -Wl,--allow-undefined \
    -O3 \
    -flto \
    -fno-builtin \
    -o geo_filter.wasm \
    geo_filter.c

# Create output directory
OUTPUT_DIR="./dist"
mkdir -p "$OUTPUT_DIR"
mv geo_filter.wasm "$OUTPUT_DIR/"

# Show size
SIZE=$(wc -c < "$OUTPUT_DIR/geo_filter.wasm")
echo "✅ Build complete!"
echo "   Output: $OUTPUT_DIR/geo_filter.wasm"
echo "   Size: $SIZE bytes"

# Optional: Optimize with wasm-opt
if command -v wasm-opt &> /dev/null; then
    echo "Optimizing with wasm-opt..."
    wasm-opt -Oz "$OUTPUT_DIR/geo_filter.wasm" -o "$OUTPUT_DIR/geo_filter.opt.wasm"
    OPT_SIZE=$(wc -c < "$OUTPUT_DIR/geo_filter.opt.wasm")
    echo "   Optimized: $OPT_SIZE bytes (saved $((SIZE - OPT_SIZE)) bytes)"
    mv "$OUTPUT_DIR/geo_filter.opt.wasm" "$OUTPUT_DIR/geo_filter.wasm"
fi

echo ""
echo "To test the UDF:"
echo "  cd ../../.. && go test ./examples/udfs/geo-filter -v"
echo ""
echo "To use this UDF:"
echo "  {\"wasm_udf\": {\"name\": \"geo_filter\", \"version\": \"1.0.0\","
echo "   \"parameters\": {\"target_lat\": 37.7749, \"target_lon\": -122.4194, \"max_distance_km\": 10}}}"
