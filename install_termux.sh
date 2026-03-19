#!/bin/bash

# Mayo Termux Installer Script
# This script prepares Termux environment and builds Mayo

echo "🚀 Starting Mayo Installation for Termux..."

# 1. Update and install dependencies
echo "📦 Updating packages and installing Go..."
pkg update -y && pkg upgrade -y
pkg install -y golang git termux-api

# 2. Check if storage access is needed
echo "📁 Requesting storage access (please allow the popup on your phone)..."
termux-setup-storage

# 3. Build the application
echo "🔨 Building Mayo binary..."
go build -o mayo main.go

# 4. Create a shortcut in /usr/bin for easy access
echo "🔧 Creating shortcut..."
chmod +x mayo
cp mayo $PREFIX/bin/mayo

echo "---"
echo "✅ Installation Complete!"
echo "💡 You can now run the app from anywhere by simply typing: mayo"
echo ""
echo "📝 Quick Tips for Android:"
echo "1. Use /setup to configure your AI profile."
echo "2. Use /connect file ~/storage/downloads/your_file.xlsx to analyze files from your phone."
echo "3. Use a keyboard with arrow keys for the best experience."
echo "---"
