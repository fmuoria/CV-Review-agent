.PHONY: build run test clean fmt vet help

# Build the application
build:
	@echo "Building CV Review Agent..."
	@go build -o cv-review-agent .
	@echo "Build complete: cv-review-agent"

# Run the application
run:
	@echo "Starting CV Review Agent..."
	@go run main.go

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f cv-review-agent
	@rm -rf uploads/
	@echo "Clean complete"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Formatting complete"

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies installed"

# Show help
help:
	@echo "Available targets:"
	@echo "  build  - Build the application"
	@echo "  run    - Run the application"
	@echo "  test   - Run tests"
	@echo "  clean  - Clean build artifacts"
	@echo "  fmt    - Format code"
	@echo "  vet    - Run go vet"
	@echo "  deps   - Install dependencies"
	@echo "  help   - Show this help message"
