BIN_DIR   ?= bin
TMP_DIR   ?= .tmp
COVER_OUT ?= $(TMP_DIR)/coverage.out

.PHONY: all build install test race bench cover lint format clean help

all: format test

build:
	go build -ldflags="-s -w" -o $(BIN_DIR)/c2plan9.exe .

install:
	go install .

test:
	go test -timeout 60s ./...

race:
	go test -race -timeout 120s ./...

bench:
	go test -bench=. -benchmem -run=^$$ ./...

cover:
	go test -coverprofile=$(COVER_OUT) ./...
	go tool cover -func=$(COVER_OUT)

lint:
	golangci-lint run --timeout=5m ./...

format:
	go fmt ./...
	addlicense -c "Lemon4ksan" -l bsd -ignore "**/*.yml" .
	golangci-lint run --fix ./...

clean:
	go clean -cache -testcache

help:
	@echo c2plan9 targets: build, install, test, race, bench, cover, lint, format, clean
