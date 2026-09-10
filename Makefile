.PHONY: all build run test test-race lint generate clean

# Toolchain commands
GO ?= go
OAPI_CODEGEN ?= go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1

all: generate build test

generate:
	@echo "Generating API models and server interfaces from OpenAPI 3.1..."
	@mkdir -p internal/generated/api
	$(OAPI_CODEGEN) -config api/openapi/v1/oapi-codegen.yaml api/openapi/v1/ledger.yaml

build:
	@echo "Building server binary..."
	$(GO) build -v -o bin/server.exe ./cmd/server

run: build
	./bin/server.exe

test:
	@echo "Running unit and invariant tests..."
	$(GO) test -v ./...

test-race:
	@echo "Running race condition and concurrency stress tests..."
	$(GO) test -v -race -run TestConcurrent ./tests/...

clean:
	@rm -rf bin/
	@rm -rf internal/generated/api/*
