.PHONY: test test-race build lint vet fmt clean tidy help

# Default target
all: vet lint test

## test: run tests (no race detector, faster)
test:
	go test ./... -timeout 120s

## test-race: run tests with race detector
test-race:
	go test -race ./... -timeout 120s

## test-cover: run tests with coverage report
test-cover:
	go test -race ./... -timeout 120s -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test-pkg: run tests for a single package, e.g. make test-pkg PKG=./logger/...
test-pkg:
	go test -race $(PKG) -v -timeout 30s

## build: build all packages
build:
	go build ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go source files
fmt:
	gofmt -w -l .

## lint: run golangci-lint (requires golangci-lint to be installed)
lint:
	golangci-lint run ./...

## tidy: tidy go.mod and go.sum
tidy:
	go mod tidy

## clean: remove build artifacts and coverage files
clean:
	go clean ./...
	rm -f coverage.out coverage.html

## example-build: build the example service
example-build:
	cd example && go build ./...

## example-run: run the example service
example-run:
	cd example && go run .

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
