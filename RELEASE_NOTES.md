## LocalWindows v0.1.0-dev.1 — Canary Dev Release

Lightweight LAN remote desktop application built with Go and Fyne.

### Build

```bash
# Build for current platform
make build

# Build Linux amd64
make linux

# Build all desktop platforms (requires cross-compilers)
make all-desktop

# Or use the release script
./scripts/release.sh v0.1.0-dev.1
```

### Install & Run

```bash
chmod +x localwindows-linux-amd64
./localwindows-linux-amd64
```

### Supported Platforms

| Platform | Architecture | Binary | Status |
|----------|-------------|--------|--------|
| Linux | x86_64 | `localwindows-linux-amd64` | Built |
| Linux | ARM64 | `localwindows-linux-arm64` | Requires cross-compiler |
| macOS | x86_64 | `localwindows-darwin-amd64` | Requires cross-compiler |
| macOS | ARM64 (Apple Silicon) | `localwindows-darwin-arm64` | Requires cross-compiler |
| Windows | x86_64 | `localwindows-windows-amd64.exe` | Requires MinGW cross-compiler |

### Features

- **Screen sharing** — tile-based delta compression with JPEG encoding
- **Remote input** — mouse (move, click, drag, scroll) and full keyboard
- **Authentication** — password (SHA-256 challenge-response) or one-time PIN
- **LAN discovery** — automatic UDP broadcast/listen for hosts on the network
- **File transfer** — viewer-to-host file sending with progress tracking
- **Clipboard sync** — bidirectional clipboard sharing
- **Special key combos** — Ctrl+Alt+Del, Alt+Tab, Win+D, etc.
- **TLS encrypted** — all connections use TLS

### Checksums (SHA-256)

```
e36318e969d9d900d0758756d8b4623bdc33b3280b3b6aecefbf4cf0bc17ea65  localwindows-linux-amd64
```

### Testing

```bash
make test    # 30 tests across protocol, auth, transfer
make vet     # Static analysis
```
