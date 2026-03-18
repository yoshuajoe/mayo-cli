# Mayo Makefile

# Variables
BINARY_NAME=mayo
GO=go
BUILD_DIR=bin

.PHONY: all build run clean test setup install

all: build

# Build the binary
build:
	@echo "🔨 Building Mayo..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) main.go
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@$(GO) run main.go

# Run tests
test:
	@echo "🔍 Running tests..."
	@$(GO) test ./... -v

# Clean build artifacts
clean:
	@echo "🧹 Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@echo "✨ Cleaned!"

# Install dependencies
setup:
	@echo "📦 Tidying up modules..."
	@$(GO) mod tidy
	@echo "✅ Setup complete."

# Installation Variables
INSTALL_PATH=/usr/local/bin
CONFIG_DIR=$(HOME)/.mayo-cli

# Install the binary globally
install: build
	@echo "🚀 Installing $(BINARY_NAME) to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "📂 Creating configuration directories..."
	@mkdir -p $(CONFIG_DIR)/sessions
	@mkdir -p $(CONFIG_DIR)/data
	@echo "✅ Installation complete!"
	@echo "💡 If $(INSTALL_PATH) is not in your PATH, please add it or run the command using the full path."

# Uninstall the binary
uninstall:
	@echo "🗑️  Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "✅ Uninstalled."

# Run cross-platform setup script
setup-all:
	@$(GO) run scripts/setup.go

# Run environment prerequisite check and fix permissions
setup-env:
	@chmod +x scripts/setup_env.sh
	@./scripts/setup_env.sh

# Help command
help:
	@echo "Mayo Tasks:"
	@echo "  make build      - Build the binary"
	@echo "  make run        - Run the application using go run"
	@echo "  make test       - Run all unit tests"
	@echo "  make setup      - Tidy modules and install dependencies"
	@echo "  make setup-env  - Fix permissions and check prerequisites"
	@echo "  make setup-all  - Run cross-platform installer (Win/Mac/Linux)"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make install    - Install binary to GOPATH/bin"
	@echo "  make uninstall  - Remove binary from GOPATH/bin"
	@echo "  make release    - Run release automation (version, changelog, tags)"

# Release automation
release:
	@./scripts/release.sh
