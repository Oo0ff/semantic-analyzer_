.PHONY: build test lint benchmark clean integration-test

GO := go
BINARY_NAME := semantic-analyzer
BUILD_DIR := bin
COVERAGE_FILE := coverage.out
BENCHMARK_DIR := benchmarks

# Build targets
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/cli

# Test targets
test:
	@echo "Running unit tests..."
	$(GO) test -v -race ./... -coverprofile=$(COVERAGE_FILE)

test-coverage: test
	$(GO) tool cover -html=$(COVERAGE_FILE) -o coverage.html

determinism-test:
	@echo "Running determinism tests (100 iterations)..."
	$(GO) test -v -count=100 ./pkg/determinism -run TestDeterminism

integration-test:
	@echo "Running integration tests..."
	$(GO) test -v ./cmd/pipeline -run TestPipeline
	$(GO) test -v ./internal/engine/composite -run TestIntegration
	$(GO) test -v ./internal/engine/semantic -run TestIntegration

# Benchmark targets
benchmark:
	@echo "Running benchmarks..."
	$(GO) test -bench=. -benchmem -benchtime=5s ./internal/processor/text
	$(GO) test -bench=. -benchmem -benchtime=5s ./internal/engine/atomic
	$(GO) test -bench=. -benchmem -benchtime=5s ./internal/engine/composite
	$(GO) test -bench=. -benchmem -benchtime=5s ./internal/engine/semantic

# Performance test with large file
performance-test:
	@echo "Generating 10,000 word test file..."
	@./scripts/generate_test_file.sh 10000
	@echo "Running performance test..."
	$(GO) run ./cmd/cli --config ./configs/config.yaml --input ./test_large.txt --output ./tmp_results --pipeline=true
	@rm -f ./test_large.txt ./tmp_results/*

# Linting
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --timeout=5m

# Cleanup
clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR) $(COVERAGE_FILE) coverage.html tmp_results
	$(GO) clean -testcache

# Development
run:
	$(GO) run ./cmd/cli --config ./configs/config.yaml --input ./sample.txt --output ./results/ --pipeline=true

run-legacy:
	$(GO) run ./cmd/cli --config ./configs/config.yaml --input ./sample.txt --output ./results/ --pipeline=false

# Profile generation
profile-cpu:
	@echo "Generating CPU profile..."
	$(GO) test -cpuprofile cpu.prof ./internal/engine/composite -bench=. -benchtime=10s
	$(GO) tool pprof -web cpu.prof

profile-memory:
	@echo "Generating memory profile..."
	$(GO) test -memprofile mem.prof ./internal/engine/semantic -bench=. -benchtime=10s
	$(GO) tool pprof -web mem.prof

# Install dependencies
deps:
	$(GO) mod download
	$(GO) mod verify

# Default target
all: deps build test lint integration-test

# Quick test (fast unit tests only)
quick-test:
	$(GO) test ./pkg/determinism ./internal/domain ./internal/processor/text -short

# Generate test report
test-report: test
	@echo "Test coverage:"
	$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1