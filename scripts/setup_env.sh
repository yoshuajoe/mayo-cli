#!/bin/bash

# Mayo Prerequisite & Setup Script
# This script handles permissions, missing dependencies, and system checks for Mayo-CLI.

set -e

# Detect OS and Arch
OS="$(uname -s)"
ARCH="$(uname -m)"
MAYODIR="$HOME/.mayo-cli"

echo "🔍 System Diagnosis..."
echo "----------------------------------------"
echo "  OS:   $OS"
echo "  Arch: $ARCH"
echo "  Path: $MAYODIR"
echo "----------------------------------------"

# 1. FIX PERMISSIONS (Common issue if installed with sudo)
if [[ -d "$MAYODIR" ]]; then
    OWNER=$(stat -f "%Su" "$MAYODIR" 2>/dev/null || stat -c "%U" "$MAYODIR" 2>/dev/null)
    if [[ "$OWNER" == "root" ]]; then
        echo "🚨 Permission Alert: $MAYODIR is owned by 'root'!"
        echo "   This prevents Mayo from saving sessions or dataframes as your normal user."
        echo "   Fixing ownership to $(whoami)..."
        sudo chown -R "$(whoami)" "$MAYODIR"
        echo "✅ Ownership fixed."
    else
        echo "✅ Permissions: Correct (Owned by $(whoami))"
    fi
else
    echo "📁 Creating Mayo directory structure..."
    mkdir -p "$MAYODIR/data"
    mkdir -p "$MAYODIR/sessions"
    echo "✅ Directory created."
fi

# 2. CHECK DEPENDENCIES
echo "📦 Checking dependencies..."

# SQLite3 Check
if command -v sqlite3 &> /dev/null; then
    SQL_VER=$(sqlite3 --version | awk '{print $1}')
    echo "✅ SQLite: Found v$SQL_VER"
else
    echo "❌ SQLite: Not found!"
    if [[ "$OS" == "Darwin" ]]; then
        echo "   -> Recommendation: brew install sqlite"
    elif [[ "$OS" == "Linux" ]]; then
        echo "   -> Recommendation: sudo apt install sqlite3"
    fi
fi

# Go Check (For building Mayo)
if command -v go &> /dev/null; then
    GO_VER=$(go version | awk '{print $3}')
    echo "✅ Go: Found $GO_VER"
else
    echo "⚠️  Go: Not found. You won't be able to re-build Mayo from source."
fi

# macOS Specific: Homebrew & Vector Extensions
if [[ "$OS" == "Darwin" ]]; then
    if command -v brew &> /dev/null; then
        echo "✅ Homebrew: Found"
        # Check for sqlite-vector if they want it
        echo "💡 Hint: For vector support, you can run: brew install asg017/tap/sqlite-vector"
    else
        echo "⚠️  Homebrew: Not found. Recommended for macOS users: https://brew.sh"
    fi
fi

echo "----------------------------------------"
echo "🚀 Pre-requisite check complete!"
echo "If you still see 'unable to open database file', try running: chmod -R 755 $MAYODIR"
echo "----------------------------------------"
