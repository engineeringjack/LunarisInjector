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

package-linux: build-all
	@rm -rf dist/lunaris-deb dist/lunaris-tar
	# 1. Debian / Ubuntu / Mint .deb Package
	@mkdir -p dist/lunaris-deb/DEBIAN
	@mkdir -p dist/lunaris-deb/usr/bin
	@mkdir -p dist/lunaris-deb/usr/share/applications
	@mkdir -p dist/lunaris-deb/usr/share/icons/hicolor/512x512/apps
	@cp dist/lunaris-linux-amd64 dist/lunaris-deb/usr/bin/lunaris
	@chmod 755 dist/lunaris-deb/usr/bin/lunaris
	@cp assets/lunaris.png dist/lunaris-deb/usr/share/icons/hicolor/512x512/apps/lunaris.png
	@chmod 644 dist/lunaris-deb/usr/share/icons/hicolor/512x512/apps/lunaris.png
	@printf '[Desktop Entry]\nName=Lunaris\nGenericName=Minecraft Modpack Synchronizer\nComment=Automatic modpack synchronizer and launch wrapper for Minecraft\nExec=/usr/bin/lunaris gui\nIcon=lunaris\nTerminal=false\nType=Application\nCategories=Game;Utility;\nKeywords=minecraft;modpack;curseforge;sync;\nStartupNotify=true\n' > dist/lunaris-deb/usr/share/applications/lunaris.desktop
	@chmod 644 dist/lunaris-deb/usr/share/applications/lunaris.desktop
	@printf 'Package: lunaris\nVersion: 1.0.3\nSection: games\nPriority: optional\nArchitecture: amd64\nMaintainer: Jack Frederick <j49fkk@gmail.com>\nDescription: Lunaris Modpack Auto-Synchronizer & Launch Interceptor\n Lunaris is a zero-dependency synchronization engine and pre-launch interceptor\n designed to sit directly between CurseForge / Minecraft launchers and Minecraft.\n' > dist/lunaris-deb/DEBIAN/control
	@dpkg-deb --root-owner-group --build dist/lunaris-deb dist/lunaris_1.0.3_amd64.deb
	@rm -rf dist/lunaris-deb
	# 2. Portable Linux .tar.gz Bundle
	@mkdir -p dist/lunaris-tar
	@cp dist/lunaris-linux-amd64 dist/lunaris-tar/lunaris
	@chmod 755 dist/lunaris-tar/lunaris
	@cp assets/lunaris.png dist/lunaris-tar/lunaris.png
	@printf '[Desktop Entry]\nName=Lunaris\nComment=Minecraft Modpack Synchronizer\nExec=./lunaris gui\nIcon=./lunaris.png\nTerminal=false\nType=Application\nCategories=Game;Utility;\n' > dist/lunaris-tar/Lunaris.desktop
	@chmod 755 dist/lunaris-tar/Lunaris.desktop
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\necho "Starting Lunaris Installer..."\nchmod +x lunaris 2>/dev/null || true\n# Optionally install desktop shortcut if run with --install\nif [ "$$1" = "--install" ]; then\n  mkdir -p ~/.local/bin ~/.local/share/applications ~/.local/share/icons/hicolor/512x512/apps\n  cp lunaris ~/.local/bin/lunaris\n  cp lunaris.png ~/.local/share/icons/hicolor/512x512/apps/lunaris.png\n  cat << "EOF" > ~/.local/share/applications/lunaris.desktop\n[Desktop Entry]\nName=Lunaris\nComment=Minecraft Modpack Synchronizer\nExec=$$HOME/.local/bin/lunaris gui\nIcon=lunaris\nTerminal=false\nType=Application\nCategories=Game;Utility;\nEOF\n  echo "Lunaris installed to ~/.local/bin/lunaris and added to desktop applications."\nfi\n./lunaris gui\n' > dist/lunaris-tar/install.sh
	@chmod +x dist/lunaris-tar/install.sh
	@cd dist/lunaris-tar && tar -czf ../Lunaris-Linux-x86_64.tar.gz *
	@rm -rf dist/lunaris-tar
	@echo "Linux .deb and .tar.gz packages created in dist/"

package-macos: build-all
	@mkdir -p dist/Lunaris-AppleSilicon.app/Contents/MacOS
	@mkdir -p dist/Lunaris-AppleSilicon.app/Contents/Resources
	@cp dist/lunaris-darwin-arm64 dist/Lunaris-AppleSilicon.app/Contents/MacOS/lunaris
	@chmod +x dist/Lunaris-AppleSilicon.app/Contents/MacOS/lunaris
	@cp assets/AppIcon.icns dist/Lunaris-AppleSilicon.app/Contents/Resources/AppIcon.icns
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n<key>CFBundleExecutable</key><string>lunaris</string>\n<key>CFBundleIconFile</key><string>AppIcon</string>\n<key>CFBundleIdentifier</key><string>com.engineeringjack.lunaris</string>\n<key>CFBundleName</key><string>Lunaris</string>\n<key>CFBundleDisplayName</key><string>Lunaris</string>\n<key>CFBundlePackageType</key><string>APPL</string>\n<key>CFBundleShortVersionString</key><string>1.0.3</string>\n<key>CFBundleVersion</key><string>1.0.3</string>\n<key>LSMinimumSystemVersion</key><string>10.15</string>\n<key>NSHighResolutionCapable</key><true/>\n</dict>\n</plist>' > dist/Lunaris-AppleSilicon.app/Contents/Info.plist
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\necho "Starting Lunaris Installer..."\nxattr -cr Lunaris-AppleSilicon.app 2>/dev/null || true\nchmod +x Lunaris-AppleSilicon.app/Contents/MacOS/lunaris 2>/dev/null || true\nopen Lunaris-AppleSilicon.app\n' > dist/Install-Lunaris-AppleSilicon.command
	@chmod +x dist/Install-Lunaris-AppleSilicon.command
	@cd dist && rm -f Lunaris-macOS-AppleSilicon.zip && zip -r -y Lunaris-macOS-AppleSilicon.zip Lunaris-AppleSilicon.app Install-Lunaris-AppleSilicon.command
	@mkdir -p dist/Lunaris-Intel.app/Contents/MacOS
	@mkdir -p dist/Lunaris-Intel.app/Contents/Resources
	@cp dist/lunaris-darwin-amd64 dist/Lunaris-Intel.app/Contents/MacOS/lunaris
	@chmod +x dist/Lunaris-Intel.app/Contents/MacOS/lunaris
	@cp assets/AppIcon.icns dist/Lunaris-Intel.app/Contents/Resources/AppIcon.icns
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n<key>CFBundleExecutable</key><string>lunaris</string>\n<key>CFBundleIconFile</key><string>AppIcon</string>\n<key>CFBundleIdentifier</key><string>com.engineeringjack.lunaris</string>\n<key>CFBundleName</key><string>Lunaris</string>\n<key>CFBundleDisplayName</key><string>Lunaris</string>\n<key>CFBundlePackageType</key><string>APPL</string>\n<key>CFBundleShortVersionString</key><string>1.0.3</string>\n<key>CFBundleVersion</key><string>1.0.3</string>\n<key>LSMinimumSystemVersion</key><string>10.15</string>\n<key>NSHighResolutionCapable</key><true/>\n</dict>\n</plist>' > dist/Lunaris-Intel.app/Contents/Info.plist
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\necho "Starting Lunaris Installer..."\nxattr -cr Lunaris-Intel.app 2>/dev/null || true\nchmod +x Lunaris-Intel.app/Contents/MacOS/lunaris 2>/dev/null || true\nopen Lunaris-Intel.app\n' > dist/Install-Lunaris-Intel.command
	@chmod +x dist/Install-Lunaris-Intel.command
	@cd dist && rm -f Lunaris-macOS-Intel.zip && zip -r -y Lunaris-macOS-Intel.zip Lunaris-Intel.app Install-Lunaris-Intel.command
	@echo "macOS permission-preserving .app and .zip packages created in dist/"

package-all: package-linux package-macos

test:
	$(GO) test -v ./...

clean:
	rm -rf bin dist lunaris lunaris.exe
