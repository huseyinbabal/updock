.PHONY: build run test clean docker

BINARY_NAME=updock
VERSION?=0.1.0
LDFLAGS=-ldflags "-X github.com/huseyinbabal/updock/internal/config.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/updock

run:
	go run $(LDFLAGS) ./cmd/updock

test:
	go test -v -race ./...

clean:
	rm -rf bin/ dist/

docker:
	docker build -t updock:$(VERSION) .

lint:
	golangci-lint run ./...

.DEFAULT_GOAL := build
