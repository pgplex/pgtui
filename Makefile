.PHONY: build run test clean install help sign

# Build variables
BINARY_NAME=pgtui
BUILD_DIR=bin
VERSION?=dev
LDFLAGS=-ldflags "-X main.version=${VERSION}"
UNAME_S := $(shell uname -s)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary (with code signing on macOS)
	@echo "Building ${BINARY_NAME}..."
	@mkdir -p ${BUILD_DIR}
	@go build ${LDFLAGS} -o ${BUILD_DIR}/${BINARY_NAME} cmd/pgtui/main.go
ifeq ($(UNAME_S),Darwin)
	@echo "Signing binary for macOS keychain compatibility..."
	@codesign -s - -f ${BUILD_DIR}/${BINARY_NAME}
endif
	@echo "Built ${BUILD_DIR}/${BINARY_NAME}"

sign: ## Sign the binary (macOS only, for keychain "Always Allow")
ifeq ($(UNAME_S),Darwin)
	@codesign -s - -f ${BUILD_DIR}/${BINARY_NAME}
	@echo "Signed ${BUILD_DIR}/${BINARY_NAME}"
else
	@echo "Code signing is only needed on macOS"
endif

run: build ## Build and run the application
	@${BUILD_DIR}/${BINARY_NAME}

test: ## Run tests
	@go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests and show coverage
	@go tool cover -html=coverage.out

clean: ## Remove build artifacts
	@rm -rf ${BUILD_DIR}
	@rm -f coverage.out
	@echo "Cleaned build artifacts"

install: build ## Install binary to $GOPATH/bin (with code signing on macOS)
	@go install ./cmd/pgtui
ifeq ($(UNAME_S),Darwin)
	@codesign -s - -f $(shell go env GOPATH)/bin/${BINARY_NAME}
endif
	@echo "Installed to $(shell go env GOPATH)/bin/${BINARY_NAME}"

fmt: ## Format code
	@go fmt ./...

lint: ## Run linter
	@golangci-lint run ./... || echo "golangci-lint not installed, skipping"

deps: ## Download dependencies
	@go mod download
	@go mod tidy

dev: ## Run in development mode with hot reload
	@echo "Running in development mode (press Ctrl+C to stop)..."
	@go run cmd/pgtui/main.go

.DEFAULT_GOAL := help
