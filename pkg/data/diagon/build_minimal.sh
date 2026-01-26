#!/bin/bash
# Build minimal Diagon wrapper for Quidditch CGO integration

set -e

echo "Building minimal Diagon wrapper..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$SCRIPT_DIR/build_minimal"

# Create build directory
mkdir -p "$BUILD_DIR"

# Compile C++ wrapper
g++ -std=c++20 -O2 -fPIC -shared \
    "$SCRIPT_DIR/minimal_wrapper.cpp" \
    -o "$BUILD_DIR/libdiagon_minimal.so"

echo "✓ Built: $BUILD_DIR/libdiagon_minimal.so"

# Create symlink for easier CGO linking
ln -sf "$BUILD_DIR/libdiagon_minimal.so" "$SCRIPT_DIR/libdiagon.so"

echo "✓ Created symlink: $SCRIPT_DIR/libdiagon.so"
echo ""
echo "Success! Minimal Diagon wrapper ready for CGO integration."
