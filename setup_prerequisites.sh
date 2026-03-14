#!/bin/bash

# Mayo Prerequisites Setup Script
# This script helps install extensions for Vector Search in SQLite

set -e

echo "🔍 Detecting System Environment..."
OS="$(uname -s)"
ARCH="$(uname -m)"

EXTENSION_DIR="$HOME/.insight-cli/extensions"
mkdir -p "$EXTENSION_DIR"

echo "📍 Setting up extensions in: $EXTENSION_DIR"

# Version of sqlite-vector
VERSION="v0.1.0" # Example version, can be updated

function download_extension() {
    local platform=$1
    local ext_name=$2
    local url="https://github.com/asg017/sqlite-vector/releases/download/${VERSION}/sqlite-vector-${VERSION}-${platform}.tar.gz"
    
    echo "📥 Downloading sqlite-vector for $platform..."
    curl -L "$url" -o "$EXTENSION_DIR/vector.tar.gz"
    tar -xzf "$EXTENSION_DIR/vector.tar.gz" -C "$EXTENSION_DIR"
    rm "$EXTENSION_DIR/vector.tar.gz"
    echo "✅ Successfully downloaded and extracted extension."
}

if [[ "$OS" == "Darwin" ]]; then
    echo "🍎 Detected macOS"
    if [[ "$ARCH" == "arm64" ]]; then
        # download_extension "macos-arm64" "vector0.dylib"
        echo "Note: For macOS Silicon, we recommend using homebrew or pre-built binaries from sqlite-vector repo."
    else
        # download_extension "macos-x86_64" "vector0.dylib"
        echo "Note: For macOS Intel, we recommend using pre-built binaries."
    fi
elif [[ "$OS" == "Linux" ]]; then
    if [[ -d "/data/data/com.termux" ]]; then
        echo "🤖 Detected Termux (Android)"
        echo "📦 Installing build dependencies in Termux..."
        pkg install -y clang make cmake sqlite
        echo "💡 Note: On Termux, it is often best to compile sqlite extensions for optimal performance."
    else
        echo "🐧 Detected Linux"
        # download_extension "linux-x86_64" "vector0.so"
    fi
fi

echo ""
echo "--- Installation Guide ---"
echo "1. The extensions will be searched in $EXTENSION_DIR"
echo "2. Mayo will automatically try to load these if available."
echo "3. For Vector Search, make sure you have the 'sqlite-vector' extension loaded."
echo "---"

echo "🚀 Ready to go! Run 'insight' to start your research."
