.PHONY: all build build-all test clean

GO ?= $(shell which go 2>/dev/null || echo "$$HOME/.local/go/bin/go")

all: build

build:
	@mkdir -p bin
	$(GO) build -ldflags="-s -w" -o bin/lunaris ./cmd/lunaris
	@echo "Built bin/lunaris"

build-all:
	@mkdir -p dist
	# Linux 64-bit
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w" -o dist/lunaris-linux-amd64 ./cmd/lunaris
	# Windows 64-bit (.exe)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags="-s -w" -o dist/lunaris-windows-amd64.exe ./cmd/lunaris
	# macOS Apple Silicon
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -ldflags="-s -w" -o dist/lunaris-darwin-arm64 ./cmd/lunaris
	# macOS Intel
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -ldflags="-s -w" -o dist/lunaris-darwin-amd64 ./cmd/lunaris
	@echo "All binaries successfully compiled to dist/"

test:
	$(GO) test -v ./...

clean:
	rm -rf bin dist lunaris lunaris.exe
