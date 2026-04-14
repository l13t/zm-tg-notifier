.PHONY: build run clean docker-build docker-run test

# Version can be overridden: make build VERSION=1.0.0
VERSION ?= dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Build the application
build:
	go build $(LDFLAGS) -o zm-tg-notifier ./cmd/zm-tg-notifier

# Run locally
run: build
	./zm-tg-notifier run

# Clean build artifacts
clean:
	rm -f zm-tg-notifier
	go clean

# Build Docker image
docker-build:
	docker build -t zm-tg-notifier .

# Run Docker container
docker-run:
	docker run --rm -e TOKEN=$(TOKEN) -v $(ZM_FOLDER):/zm zm-tg-notifier

# Run tests
test:
	go test -v ./...

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	go vet ./...

# Lint markdown files
lint-md:
	npx markdownlint-cli2 "**/*.md" "#node_modules"
