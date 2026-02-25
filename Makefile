APP_NAME := localwindows
VERSION := 1.0.0

# Go build flags
LDFLAGS := -s -w -X main.version=$(VERSION)
BUILD_FLAGS := -trimpath -ldflags "$(LDFLAGS)"

.PHONY: all clean build linux windows macos android ios

# Default: build for current platform
build:
	go build $(BUILD_FLAGS) -o $(APP_NAME) .

# --- Desktop Platforms ---

linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
		go build $(BUILD_FLAGS) -o build/$(APP_NAME)-linux-amd64 .

linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
		go build $(BUILD_FLAGS) -o build/$(APP_NAME)-linux-arm64 .

windows:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
		go build $(BUILD_FLAGS) -o build/$(APP_NAME)-windows-amd64.exe .

macos:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
		go build $(BUILD_FLAGS) -o build/$(APP_NAME)-darwin-amd64 .

macos-arm64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
		go build $(BUILD_FLAGS) -o build/$(APP_NAME)-darwin-arm64 .

# --- Mobile Platforms (require fyne tool) ---

android:
	fyne package -os android -appID com.localwindows.app -name $(APP_NAME)

ios:
	fyne package -os ios -appID com.localwindows.app -name $(APP_NAME)

# --- All desktop platforms ---
all-desktop: linux windows macos macos-arm64

# --- Utilities ---

clean:
	rm -rf build/ $(APP_NAME) $(APP_NAME).exe
	rm -f $(APP_NAME).apk $(APP_NAME).app

deps:
	go mod tidy

test:
	go test ./...

vet:
	go vet ./...

# --- Fyne packaging (creates distributable bundles) ---

package-linux:
	fyne package -os linux -name $(APP_NAME) -appID com.localwindows.app

package-windows:
	fyne package -os windows -name $(APP_NAME) -appID com.localwindows.app

package-macos:
	fyne package -os darwin -name $(APP_NAME) -appID com.localwindows.app
