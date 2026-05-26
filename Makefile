.PHONY: build test run lint clean fmt

BINARY_NAME=vision-mcp
VERSION ?= dev

build:
	go build -ldflags="-s -w -X github.com/cristian-guerrero/go-vision-mcp/internal/version.Version=$(VERSION)" -o $(BINARY_NAME) .

run:
	go run .

test:
	go test ./internal/...

test-integ:
	go test -tags=integration ./internal/...

test-all:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY_NAME) vision-mcp-*

fmt:
	go fmt ./...

tidy:
	go mod tidy
