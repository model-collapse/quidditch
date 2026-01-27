#!/bin/bash
#
# Build Diagon C API wrapper
#
# This builds a shared library that wraps the real Diagon C++ engine
# with a C interface for CGO integration.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UPSTREAM_DIR="$SCRIPT_DIR/upstream"
BUILD_DIR="$SCRIPT_DIR/build"

echo "Building Diagon C API wrapper..."
echo "Script dir: $SCRIPT_DIR"
echo "Upstream dir: $UPSTREAM_DIR"

# Create build directory
mkdir -p "$BUILD_DIR"

# Build Diagon core first if not built
if [ ! -f "$UPSTREAM_DIR/build/src/core/libdiagon_core.so" ]; then
    echo "Building Diagon core library first..."
    cd "$UPSTREAM_DIR"
    mkdir -p build
    cd build
    cmake .. -DCMAKE_BUILD_TYPE=Release \
             -DDIAGON_BUILD_TESTS=OFF \
             -DDIAGON_BUILD_BENCHMARKS=OFF
    cmake --build . -j$(nproc)
    cd "$SCRIPT_DIR"
fi

echo "Building C API wrapper..."

# Compile C API wrapper
g++ -std=c++20 -O2 -fPIC -shared \
    "$SCRIPT_DIR/c_api_src/diagon_c_api.cpp" \
    -I"$UPSTREAM_DIR/src/core/include" \
    -I"$UPSTREAM_DIR/src/core/src" \
    -L"$UPSTREAM_DIR/build/src/core" \
    -ldiagon_core \
    -lz -lzstd -llz4 \
    -Wl,-rpath,'$ORIGIN/../upstream/build/src/core' \
    -o "$BUILD_DIR/libdiagon.so"

echo "✓ Built: $BUILD_DIR/libdiagon.so"

# Verify library
if [ -f "$BUILD_DIR/libdiagon.so" ]; then
    echo "✓ C API wrapper library created successfully"
    ls -lh "$BUILD_DIR/libdiagon.so"

    # Check symbols
    echo ""
    echo "Exported C API symbols:"
    nm -D "$BUILD_DIR/libdiagon.so" | grep "diagon_" | head -20

    echo ""
    echo "Success! Diagon C API ready for CGO integration."
    echo ""
    echo "To use in CGO:"
    echo "  // #cgo CFLAGS: -I$SCRIPT_DIR"
    echo "  // #cgo LDFLAGS: -L$BUILD_DIR -ldiagon"
    echo "  // #include \"diagon_c_api.h\""
    echo ""
else
    echo "✗ Failed to build C API wrapper"
    exit 1
fi
