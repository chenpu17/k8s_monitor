#!/bin/bash
# Build script for k8s-monitor

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Variables
VERSION=${VERSION:-"v0.1.0-dev"}
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
BUILD_DIR="./bin"

echo -e "${GREEN}🔨 Building k8s-monitor${NC}"
echo "Version: $VERSION"
echo "Build Time: $BUILD_TIME"
echo ""

# Check Go installation
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed${NC}"
    exit 1
fi

echo -e "${YELLOW}📦 Downloading dependencies...${NC}"
go mod download
go mod tidy

# Create build directory
mkdir -p "$BUILD_DIR"

# Build
echo -e "${YELLOW}🔨 Compiling...${NC}"
go build -ldflags "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME" \
    -o "$BUILD_DIR/k8s-monitor" \
    ./cmd/k8s-monitor

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✅ Build successful!${NC}"
    echo "Binary: $BUILD_DIR/k8s-monitor"
    echo ""
    echo "Run with: $BUILD_DIR/k8s-monitor --help"
else
    echo -e "${RED}❌ Build failed${NC}"
    exit 1
fi
