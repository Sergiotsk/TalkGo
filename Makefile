.PHONY: build test lint fmt check setup clean help

# ============================================
# Core commands (used by CI)
# ============================================

build:                          ## Compile all packages
	go build ./...

test:                           ## Run tests with race detector
	go test -race -cover ./...

test-verbose:                   ## Run tests with verbose output
	go test -race -v -cover ./...

lint:                           ## Run golangci-lint (enforces depguard)
	golangci-lint run ./...

fmt:                            ## Format Go files
	gofmt -w .
	goimports -w .

check: fmt lint test            ## Format, lint, and test (must pass before push)

# ============================================
# Development helpers
# ============================================

setup:                          ## Install local tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "Setup complete. Run 'make check' to verify."

clean:                          ## Clean build and test cache artifacts
	go clean -testcache

cover:                          ## Generate and open HTML coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# ============================================
# Help details
# ============================================

help:                           ## Show help information
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
