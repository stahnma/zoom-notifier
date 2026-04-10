NAME=zoom-notifier
BUILDDIR=./cmd/zoom-notifier/
export CGO_ENABLED=0

default: help

all: platforms

fmt: ## Format code with go fmt
	go fmt ./...

tidy: fmt ## Run go mod tidy (runs fmt first)
	go mod tidy

VERSION=$(shell git describe --tags --always --dirty)
COMMIT=$(shell git rev-parse --short HEAD)
BUILDDATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILDDATE)"

generate: ## Regenerate API code from OpenAPI spec
	oapi-codegen --config internal/api/oapi-codegen.yaml api/openapi.yaml

build: tidy ## Build binary (runs fmt + tidy first)
	go build $(LDFLAGS) -o $(NAME) $(BUILDDIR)

lint: ## Run golangci-lint
	golangci-lint run ./...

vet: ## Run go vet
	go vet ./...

test: ## Run all tests
	go test ./internal/...

test-verbose: ## Run tests with verbose output
	go test -v ./internal/...

test-coverage: ## Run tests with coverage report
	go test -coverprofile=coverage.out ./internal/... && go tool cover -html=coverage.out

dev: build ## Build and run with config.dev.toml
	./zoom-notifier --config config.dev.toml

clean: ## Remove built binary, bin/, coverage.out
	rm -rf $(NAME) bin coverage.out

linux-arm64: tidy
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(NAME).linux-arm64 $(BUILDDIR)

linux-amd64: tidy
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(NAME).linux-amd64 $(BUILDDIR)

darwin-arm64: tidy
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(NAME).mac-arm64 $(BUILDDIR)

darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(NAME).mac-amd64 $(BUILDDIR)

mac-arm64: darwin-arm64

mac-amd64: darwin-amd64

linux: linux-arm64 linux-amd64

mac: darwin-arm64 darwin-amd64

install: build ## Build and install to /usr/local/bin
	sudo install -p -m0755 $(NAME) /usr/local/bin

platforms: mac linux ## Cross-compile for all platforms

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
