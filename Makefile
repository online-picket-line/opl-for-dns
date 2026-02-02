# Makefile for OPL DNS Server

# Binary name
BINARY_NAME = opl-dns

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod

# Build directory
BUILD_DIR = build

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Targets
.PHONY: all build clean test coverage lint install help

all: build

build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/opl-dns
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/opl-dns
	@echo "Linux build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test:
	$(GOTEST) -v ./...

test-race:
	$(GOTEST) -v -race ./...

coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

deps:
	$(GOMOD) download
	$(GOMOD) tidy

install: build
	@echo "Installing to /usr/local/bin/$(BINARY_NAME)"
	@echo "Note: You may need to run this with sudo"
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) -config config.json

generate-config: build
	./$(BUILD_DIR)/$(BINARY_NAME) -generate-config

help:
	@echo "OPL DNS Server Build System"
	@echo ""
	@echo "Targets:"
	@echo "  build          - Build the binary (default)"
	@echo "  build-linux    - Build for Linux amd64"
	@echo "  clean          - Remove build artifacts"
	@echo "  test           - Run tests"
	@echo "  test-race      - Run tests with race detector"
	@echo "  coverage       - Generate coverage report"
	@echo "  lint           - Run linter"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  install        - Install binary to /usr/local/bin"
	@echo "  run            - Build and run with config.json"
	@echo "  generate-config - Generate example config file"
	@echo "  help           - Show this help message"
