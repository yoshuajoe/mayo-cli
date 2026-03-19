#!/bin/bash

# Mayo CLI Installer - Setup & Download
# This script is meant for end-users to quickly install Mayo CLI on their system.
# It detects your OS and architecture, downloads the latest binary, and sets up the environment.

set -e

# Configuration
REPO="yoshuajoe/mayo-cli"
BINARY_BASE="mayo"
INSTALL_DIR="/usr/local/bin"
MAYODIR="$HOME/.mayo-cli"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

printf "${BLUE}🚀 Beginning Mayo CLI Setup...${NC}\n"

# 1. SETUP DIRECTORY STRUCTURE
printf "${BLUE}📁 Initializing Mayo environment at $MAYODIR...${NC}\n"
mkdir -p "$MAYODIR" "$MAYODIR/data" "$MAYODIR/sessions"

# Ensure permissions are correct if running as root
if [[ -d "$MAYODIR" ]]; then
    OWNER=$(stat -f "%Su" "$MAYODIR" 2>/dev/null || stat -c "%U" "$MAYODIR" 2>/dev/null)
    if [[ "$OWNER" == "root" ]]; then
        sudo chown -R "$(whoami)" "$MAYODIR"
    fi
fi
chmod -R 755 "$MAYODIR"

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
    i386|i686) ARCH="386" ;;
    *)      printf "${RED}❌ Unsupported architecture: $ARCH_TYPE${NC}\n"; exit 1 ;;
esac

BINARY_NAME="$BINARY_BASE"
if [ "$OS" = "windows" ]; then
    BINARY_NAME="${BINARY_BASE}.exe"
fi

printf "${BLUE}🔍 Detected System: $OS ($ARCH)${NC}\n"

# 3. GET LATEST RELEASE INFO
printf "${BLUE}📡 Fetching latest version info from GitHub...${NC}\n"
LATEST_RELEASE=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_RELEASE" ]; then
    printf "${RED}❌ Error: Could not determine latest version from GitHub API.${NC}\n"
    printf "Please visit https://github.com/$REPO/releases/latest manually.\n"
    exit 1
fi

printf "${GREEN}✨ Found version: $LATEST_RELEASE${NC}\n"

# 4. DOWNLOAD BINARY
# Asset name convention: mayo-darwin-arm64, mayo-linux-amd64, etc.
DOWNLOAD_NAME="mayo-$OS-$ARCH"
if [ "$OS" = "windows" ]; then
    DOWNLOAD_NAME="${DOWNLOAD_NAME}.exe"
fi

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_RELEASE/$DOWNLOAD_NAME"
TMP_DIR=$(mktemp -d)
TMP_BINARY="$TMP_DIR/$BINARY_NAME"

printf "${BLUE}📥 Downloading $DOWNLOAD_NAME...${NC}\n"
if ! curl -L -o "$TMP_BINARY" "$DOWNLOAD_URL"; then
    printf "${RED}❌ Download failed!${NC}\n"
    printf "URL attempted: ${YELLOW}$DOWNLOAD_URL${NC}\n"
    printf "The binary for this platform might not be available in v$LATEST_RELEASE yet.\n"
    exit 1
fi

chmod +x "$TMP_BINARY"

# 5. INSTALL
printf "${BLUE}📦 Finalizing installation to $INSTALL_DIR...${NC}\n"
if [ "$OS" != "windows" ]; then
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
    else
        printf "${BLUE}🔑 Root permissions needed to move binary...${NC}\n"
        sudo mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
    fi
else
    # For Windows under bash shell, attempt to move to a path or give it to current dir
    mv "$TMP_BINARY" "./$BINARY_NAME"
    printf "${YELLOW}💡 On Windows, the binary has been placed in the current directory.${NC}\n"
fi

rm -rf "$TMP_DIR"

printf "\n${GREEN}✅ Mayo CLI has been successfully installed!${NC}\n"
printf "Run ${BLUE}mayo${NC} to confirm.${NC}\n"
printf "\n📝 Tip: Start with ${BLUE}mayo /wizard${NC} for quick setup.\n"
