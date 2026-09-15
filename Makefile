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
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n<key>CFBundleExecutable</key><string>lunaris</string>\n<key>CFBundleIdentifier</key><string>com.engineeringjack.lunaris</string>\n<key>CFBundleName</key><string>Lunaris</string>\n<key>CFBundlePackageType</key><string>APPL</string>\n<key>CFBundleShortVersionString</key><string>1.0.2</string>\n<key>CFBundleVersion</key><string>1.0.2</string>\n<key>LSMinimumSystemVersion</key><string>10.15</string>\n<key>NSHighResolutionCapable</key><true/>\n</dict>\n</plist>' > dist/Lunaris-AppleSilicon.app/Contents/Info.plist
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\necho "Starting Lunaris Installer..."\nxattr -cr Lunaris-AppleSilicon.app 2>/dev/null || true\nchmod +x Lunaris-AppleSilicon.app/Contents/MacOS/lunaris 2>/dev/null || true\nopen Lunaris-AppleSilicon.app\n' > dist/Install-Lunaris-AppleSilicon.command
	@chmod +x dist/Install-Lunaris-AppleSilicon.command
	@cd dist && rm -f Lunaris-macOS-AppleSilicon.zip && zip -r -y Lunaris-macOS-AppleSilicon.zip Lunaris-AppleSilicon.app Install-Lunaris-AppleSilicon.command
	@mkdir -p dist/Lunaris-Intel.app/Contents/MacOS
	@cp dist/lunaris-darwin-amd64 dist/Lunaris-Intel.app/Contents/MacOS/lunaris
	@chmod +x dist/Lunaris-Intel.app/Contents/MacOS/lunaris
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n<key>CFBundleExecutable</key><string>lunaris</string>\n<key>CFBundleIdentifier</key><string>com.engineeringjack.lunaris</string>\n<key>CFBundleName</key><string>Lunaris</string>\n<key>CFBundlePackageType</key><string>APPL</string>\n<key>CFBundleShortVersionString</key><string>1.0.2</string>\n<key>CFBundleVersion</key><string>1.0.2</string>\n<key>LSMinimumSystemVersion</key><string>10.15</string>\n<key>NSHighResolutionCapable</key><true/>\n</dict>\n</plist>' > dist/Lunaris-Intel.app/Contents/Info.plist
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\necho "Starting Lunaris Installer..."\nxattr -cr Lunaris-Intel.app 2>/dev/null || true\nchmod +x Lunaris-Intel.app/Contents/MacOS/lunaris 2>/dev/null || true\nopen Lunaris-Intel.app\n' > dist/Install-Lunaris-Intel.command
	@chmod +x dist/Install-Lunaris-Intel.command
	@cd dist && rm -f Lunaris-macOS-Intel.zip && zip -r -y Lunaris-macOS-Intel.zip Lunaris-Intel.app Install-Lunaris-Intel.command
	@echo "macOS permission-preserving .app and .zip packages created in dist/"

package-all: package-macos

test:
	$(GO) test -v ./...

clean:
	rm -rf bin dist lunaris lunaris.exe
