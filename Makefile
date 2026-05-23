.PHONY: build test run lint clean fmt

BINARY_NAME=vision-mcp

build:
	go build -ldflags="-s -w" -o $(BINARY_NAME) .

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
