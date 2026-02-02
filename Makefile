# Makefile for CMON - Complaint Monitoring System
# Builds for Windows 11 64-bit, Linux 64-bit, and Android/Termux

# Application name
APP_NAME = cmon
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build directories
BUILD_DIR = build
DIST_DIR = dist

# Go build flags
LDFLAGS = -ldflags "-s -w -X main.Version=$(VERSION)"
GO = go
GOFLAGS = -trimpath

# Output binaries
BINARY_LINUX = $(BUILD_DIR)/$(APP_NAME)-linux-amd64
BINARY_WINDOWS = $(BUILD_DIR)/$(APP_NAME)-windows-amd64.exe
BINARY_TERMUX = $(BUILD_DIR)/$(APP_NAME)-termux-arm64

.PHONY: all clean help build-linux build-windows build-termux install-termux test deps

# Default target - build all platforms
all: clean deps build-linux build-windows build-termux
	@echo "✅ All builds completed successfully!"
	@echo "📦 Binaries are in the $(BUILD_DIR) directory"

# Build for Linux 64-bit
build-linux:
	@echo "🐧 Building for Linux AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_LINUX) .
	@echo "✓ Linux build complete: $(BINARY_LINUX)"

# Build for Windows 11 64-bit
build-windows:
	@echo "🪟 Building for Windows AMD64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_WINDOWS) .
	@echo "✓ Windows build complete: $(BINARY_WINDOWS)"

# Build for Android/Termux (ARM64)
build-termux:
	@echo "📱 Building for Android/Termux ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_TERMUX) .
	@echo "✓ Termux build complete: $(BINARY_TERMUX)"

# Install dependencies
deps:
	@echo "📦 Installing dependencies..."
	$(GO) mod download
	$(GO) mod verify
	@echo "✓ Dependencies installed"

# Run tests
test:
	@echo "🧪 Running tests..."
	$(GO) test -v ./...

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR) $(DIST_DIR)
	@echo "✓ Clean complete"

# Create distribution packages
dist: all
	@echo "📦 Creating distribution packages..."
	@mkdir -p $(DIST_DIR)
	@# Linux package
	@tar -czf $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BUILD_DIR) $(APP_NAME)-linux-amd64
	@# Windows package
	@zip -j $(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_WINDOWS)
	@# Termux package
	@tar -czf $(DIST_DIR)/$(APP_NAME)-$(VERSION)-termux-arm64.tar.gz -C $(BUILD_DIR) $(APP_NAME)-termux-arm64
	@echo "✅ Distribution packages created in $(DIST_DIR)"

# Install on current system (detects OS automatically)
install: deps
	@echo "🔧 Installing $(APP_NAME) for current system..."
	$(GO) install $(LDFLAGS) .
	@echo "✓ Installation complete"

# Install directly on Termux (when running from Termux)
install-termux: build-termux
	@echo "📱 Installing on Termux..."
	@cp $(BINARY_TERMUX) $$PREFIX/bin/$(APP_NAME)
	@chmod +x $$PREFIX/bin/$(APP_NAME)
	@echo "✓ Installed to $$PREFIX/bin/$(APP_NAME)"

# Run on current system
run: deps
	@echo "🚀 Running $(APP_NAME)..."
	$(GO) run .

# Build for current platform only
build:
	@echo "🔨 Building for current platform..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) .
	@echo "✓ Build complete: $(BUILD_DIR)/$(APP_NAME)"

# Display version information
version:
	@echo "$(APP_NAME) version: $(VERSION)"

# Show help
help:
	@echo "CMON Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all              Build for all platforms (default)"
	@echo "  build-linux      Build for Linux 64-bit"
	@echo "  build-windows    Build for Windows 11 64-bit"
	@echo "  build-termux     Build for Android/Termux ARM64"
	@echo "  build            Build for current platform only"
	@echo "  deps             Install Go dependencies"
	@echo "  test             Run tests"
	@echo "  run              Run the application"
	@echo "  install          Install on current system"
	@echo "  install-termux   Install on Termux (run from Termux)"
	@echo "  dist             Create distribution packages"
	@echo "  clean            Remove build artifacts"
	@echo "  version          Display version information"
	@echo "  help             Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make                    # Build all platforms"
	@echo "  make build-linux        # Build only for Linux"
	@echo "  make clean all          # Clean and rebuild all"
	@echo "  make dist               # Create distribution packages"
