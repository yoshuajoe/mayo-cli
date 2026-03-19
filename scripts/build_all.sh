#!/bin/bash

# Mayo CLI Universal Build Script
# Targets all supported OS and Architectures for distribution.

set -e

# Colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}🏗  Building Mayo CLI for all distributions...${NC}"

# Ensure output directory exists
mkdir -p bin

# 1. Dependency check
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed.${NC}"
    exit 1
fi

# 2. Define build function
function build_one() {
    local os=$1
    local arch=$2
    local target="bin/mayo-$os-$arch"
    if [ "$os" == "windows" ]; then
        target="${target}.exe"
    fi
    
    echo -e "🔨 Building for ${YELLOW}$os/$arch${NC}..."
    if GOOS=$os GOARCH=$arch go build -v -o "$target" main.go; then
        echo -e "   ${GREEN}✓ Done: $target${NC}"
    else
        echo -e "   ${RED}✗ Failed to build for $os/$arch${NC}"
    fi
}

# --- BUILD MATRIX ---

echo -e "\n${BLUE}🍎 Apple macOS (Intel & Silicon)${NC}"
build_one "darwin" "amd64"
build_one "darwin" "arm64"

echo -e "\n${BLUE}🐧 Linux (64-bit, ARM, 32-bit)${NC}"
build_one "linux" "amd64"
build_one "linux" "arm64"
build_one "linux" "386"

echo -e "\n${BLUE}🪟 Windows (64-bit & ARM)${NC}"
build_one "windows" "amd64"
build_one "windows" "arm64"

echo -e "\n${GREEN}✨ All artifacts created successfully in './bin/' folder.${NC}"
