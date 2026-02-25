NAME=zoom-notifier
BUILDDIR=./cmd/zoom-notifier/

default: build

all: platforms

fmt:
	go fmt ./...

tidy:   fmt
	go mod tidy

VERSION=$(shell git describe --tags --always --dirty)
COMMIT=$(shell git rev-parse --short HEAD)
BUILDDATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILDDATE)"

generate:
	oapi-codegen --config internal/api/oapi-codegen.yaml api/openapi.yaml

build: tidy
	go build $(LDFLAGS) -o $(NAME) $(BUILDDIR)

test:
	go test ./internal/...

test-verbose:
	go test -v ./internal/...

test-coverage:
	go test -coverprofile=coverage.out ./internal/... && go tool cover -html=coverage.out

dev: build
	./zoom-notifier --config config.dev.toml

clean:
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

install: build
	sudo install -p -m0755 $(NAME) /usr/local/bin

platforms: mac linux
