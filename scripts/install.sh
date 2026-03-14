#!/bin/bash

# Mayo - Unified Installation Script for Unix (Mac/Linux)
# Mayo by Teleskop.id

set -e

# Colors for UI
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}🐶 Mayo Setup - Unix Intelligence Installer${NC}"

# 1. Check Go installation
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go from https://golang.org/dl/${NC}"
    exit 1
fi

# 2. Build the application
echo -e "${YELLOW}🔨 Building binary...${NC}"
mkdir -p bin
go build -o bin/mayo main.go

# 3. Define installation directory
GOPATH_BIN=$(go env GOPATH)/bin
mkdir -p "$GOPATH_BIN"

# 4. Copy binary
echo -e "${YELLOW}🚀 Installing to $GOPATH_BIN...${NC}"
cp bin/mayo "$GOPATH_BIN/mayo"
chmod +x "$GOPATH_BIN/mayo"

# 5. Handle configuration folders
CONFIG_DIR="$HOME/.mayo-cli"
mkdir -p "$CONFIG_DIR/sessions"
mkdir -p "$CONFIG_DIR/data"

# 6. Path Verification
if [[ ":$PATH:" == *":$GOPATH_BIN:"* ]]; then
    echo -e "${GREEN}✅ Installation successful!${NC}"
    echo -e "You can now run ${BLUE}mayo${NC} from anywhere."
else
    echo -e "${YELLOW}⚠️  Installation complete, but $GOPATH_BIN is not in your PATH.${NC}"
    
    # Detect shell
    SHELL_PROFILE=""
    if [[ "$SHELL" == */zsh ]]; then
        SHELL_PROFILE="$HOME/.zshrc"
    elif [[ "$SHELL" == */bash ]]; then
        SHELL_PROFILE="$HOME/.bashrc"
        if [[ "$OSTYPE" == "darwin"* ]]; then
            SHELL_PROFILE="$HOME/.bash_profile"
        fi
    fi

    if [ -n "$SHELL_PROFILE" ]; then
        echo -e "To fix this, run the following command:"
        echo -e "${BLUE}echo 'export PATH=\"\$PATH:$GOPATH_BIN\"' >> $SHELL_PROFILE && source $SHELL_PROFILE${NC}"
        
        # Automatic fix attempt
        read -p "Do you want me to add this to your $SHELL_PROFILE automatically? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "export PATH=\"\$PATH:$GOPATH_BIN\"" >> "$SHELL_PROFILE"
            echo -e "${GREEN}✅ Added to $SHELL_PROFILE. Please restart your terminal or run: source $SHELL_PROFILE${NC}"
        fi
    fi
fi
