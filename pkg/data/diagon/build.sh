#!/bin/bash
# Build script for Diagon Expression Evaluator

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building Diagon Expression Evaluator${NC}"
echo ""

# Check for required dependencies
echo "Checking dependencies..."

# Check for CMake
if ! command -v cmake &> /dev/null; then
    echo -e "${RED}ERROR: CMake not found. Please install CMake 3.15+${NC}"
    exit 1
fi

# Check for nlohmann/json
if ! pkg-config --exists nlohmann_json; then
    echo -e "${YELLOW}WARNING: nlohmann/json not found via pkg-config${NC}"
    echo "Installing nlohmann/json..."

    # Try to install via apt (Ubuntu/Debian)
    if command -v apt-get &> /dev/null; then
        sudo apt-get update
        sudo apt-get install -y nlohmann-json3-dev
    else
        echo -e "${RED}ERROR: Please install nlohmann/json manually${NC}"
        echo "Visit: https://github.com/nlohmann/json"
        exit 1
    fi
fi

# Create build directory
BUILD_DIR="build"
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

# Configure with CMake
echo ""
echo "Configuring..."
cmake .. \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_TESTS=ON \
    -DBUILD_BENCHMARKS=OFF

# Build
echo ""
echo "Building..."
cmake --build . --config Release -j$(nproc)

echo ""
echo -e "${GREEN}Build complete!${NC}"
echo ""

# Check if tests were built
if [ -f "./diagon_tests" ]; then
    echo -e "${GREEN}Running tests...${NC}"
    echo ""
    ./diagon_tests
    echo ""
fi

echo -e "${GREEN}Build artifacts:${NC}"
echo "  Library: ${BUILD_DIR}/libdiagon_expression.so"
if [ -f "./diagon_tests" ]; then
    echo "  Tests:   ${BUILD_DIR}/diagon_tests"
fi

echo ""
echo -e "${GREEN}Installation (optional):${NC}"
echo "  sudo cmake --install ${BUILD_DIR}"
echo ""
