# Development scripts

# Default command
_default:
    @just --list --unsorted

# Sync Go modules
tidy:
    go mod tidy
    @echo "All modules synced, Go workspace ready!"

# Run all unit tests
test:
    @echo "Running unit tests!"
    go clean -testcache
    go test -cover ./...

# Update the project dependencies
update-deps:
    @echo "Updating project dependencies..."
    go get -u ./...
    go mod tidy
