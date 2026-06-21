.PHONY: all build test lint clean

# Definições Padrão
APP_NAME := $(shell basename $(CURDIR))
BIN_DIR := bin
GO_FILES := $(shell find . -name '*.go' -not -path "./vendor/*")

all: lint test build

build:
	@echo "==> Building $(APP_NAME)..."
	@mkdir -p $(BIN_DIR)
	go build -v -o $(BIN_DIR)/$(APP_NAME) ./...

test:
	@echo "==> Running tests..."
	go test -v -race -cover ./...

lint:
	@echo "==> Running linter..."
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint is not installed. Run: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.54.2"; \
		exit 1; \
	fi

clean:
	@echo "==> Cleaning..."
	go clean
	rm -rf $(BIN_DIR)
