#!/bin/bash

# Mayo CLI Installer
# Inspired by Homebrew and other modern CLI installers

set -e

# Configuration
REPO="yoshuajoe/mayo-cli" # Adjust this to the actual repository path
BINARY_NAME="mayo"
INSTALL_DIR="/usr/local/bin"
MAYODIR="$HOME/.mayo-cli"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

printf "${BLUE}🚀 Installing Mayo CLI...${NC}\n"

# 1. SETUP DIRECTORY STRUCTURE
printf "${BLUE}📁 Initializing Mayo environment...${NC}\n"
mkdir -p "$MAYODIR"
mkdir -p "$MAYODIR/data"
mkdir -p "$MAYODIR/sessions"
# Ensure permissions
if [[ -d "$MAYODIR" ]]; then
    OWNER=$(stat -f "%Su" "$MAYODIR" 2>/dev/null || stat -c "%U" "$MAYODIR" 2>/dev/null)
    if [[ "$OWNER" == "root" ]]; then
        sudo chown -R "$(whoami)" "$MAYODIR"
    fi
fi
chmod -R 755 "$MAYODIR"
printf "${GREEN}✅ Environment set up at $MAYODIR${NC}\n"

# 2. DETECT SYSTEM
OS_TYPE=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS_TYPE" in
    darwin*)  OS="darwin" ;;
    linux*)   OS="linux" ;;
    msys*|cygwin*|mingw*) OS="windows" ;;
    *)        printf "${RED}❌ Unsupported OS: $OS_TYPE${NC}\n"; exit 1 ;;
esac

ARCH_TYPE=$(uname -m)
case "$ARCH_TYPE" in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)      printf "${RED}❌ Unsupported architecture: $ARCH_TYPE${NC}\n"; exit 1 ;;
esac

if [ "$OS" = "windows" ]; then
    BINARY_NAME="mayo.exe"
fi

printf "${BLUE}🔍 Detected System: $OS ($ARCH)${NC}\n"

# 3. GET LATEST RELEASE
printf "${BLUE}📡 Fetching latest version info...${NC}\n"
LATEST_RELEASE=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_RELEASE" ]; then
    # Fallback to a default or check if we can get it from another way
    # For now, if it fails, try a fixed version or exit
    printf "${RED}❌ Could not determine latest version from GitHub API.${NC}\n"
    exit 1
fi

printf "${GREEN}✨ Latest version: $LATEST_RELEASE${NC}\n"

# 4. DOWNLOAD BINARY
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_RELEASE/mayo-$OS-$ARCH"
if [ "$OS" = "windows" ]; then
    DOWNLOAD_URL="${DOWNLOAD_URL}.exe"
fi

TMP_DIR=$(mktemp -d)
TMP_BINARY="$TMP_DIR/$BINARY_NAME"

printf "${BLUE}📥 Downloading $BINARY_NAME from GitHub...${NC}\n"
if ! curl -L -o "$TMP_BINARY" "$DOWNLOAD_URL"; then
    printf "${RED}❌ Download failed. The binary for $OS ($ARCH) might not be released yet.${NC}\n"
    exit 1
fi

chmod +x "$TMP_BINARY"

# 5. INSTALL
printf "${BLUE}📦 Moving binary to $INSTALL_DIR...${NC}\n"
if [ "$OS" != "windows" ]; then
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
    else
        printf "${BLUE}🔑 Root password required for installation to $INSTALL_DIR${NC}\n"
        sudo mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
    fi
else
    # Better path for windows? Usually ~/bin or something in PATH
    mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
fi

rm -rf "$TMP_DIR"

printf "\n${GREEN}🚀 Mayo CLI installed successfully!${NC}\n"
printf "Run ${BLUE}$BINARY_NAME help${NC} to confirm.${NC}\n"
