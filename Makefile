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
	@chmod +x dist/lunaris-*
	@echo "All binaries successfully compiled to dist/"

package-macos: build-all
	@mkdir -p dist/Lunaris-AppleSilicon.app/Contents/MacOS
	@cp dist/lunaris-darwin-arm64 dist/Lunaris-AppleSilicon.app/Contents/MacOS/lunaris
	@chmod +x dist/Lunaris-AppleSilicon.app/Contents/MacOS/lunaris
	@cd dist && zip -r -y Lunaris-macOS-AppleSilicon.zip Lunaris-AppleSilicon.app
	@mkdir -p dist/Lunaris-Intel.app/Contents/MacOS
	@cp dist/lunaris-darwin-amd64 dist/Lunaris-Intel.app/Contents/MacOS/lunaris
	@chmod +x dist/Lunaris-Intel.app/Contents/MacOS/lunaris
	@cd dist && zip -r -y Lunaris-macOS-Intel.zip Lunaris-Intel.app
	@echo "macOS permission-preserving .app and .zip packages created in dist/"

package-all: package-macos

test:
	$(GO) test -v ./...

clean:
	rm -rf bin dist lunaris lunaris.exe
