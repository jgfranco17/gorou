# Development scripts

COVERAGE_FILE := "coverage.out"

# Default command
_default:
    @just --list --unsorted

# Sync Go modules
tidy:
    go mod tidy
    @echo "All modules synced, Go workspace ready!"

# Run Go package
run path:
    #!/usr/bin/env bash
    echo "Running package: {{ path }}"
    cd {{ path }}
    go run .

# Run all unit tests with race detector
test:
    @echo "Running unit tests (race detector enabled)..."
    go clean -testcache
    go test -race -count=1 -cover ./...

# Generate test coverage report
coverage:
    @echo "Generating coverage report..."
    go test -race -count=1 -coverprofile={{ COVERAGE_FILE }} ./...
    go tool cover -func={{ COVERAGE_FILE }}

# Update the project dependencies
update-deps:
    @echo "Updating project dependencies..."
    go get -u ./...
    go mod tidy
