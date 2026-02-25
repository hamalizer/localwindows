# LocalWindows

Lightweight, secure, cross-platform LAN remote desktop application written in Go.

## Features

- **Full Remote Desktop** — View and control another computer on your LAN
- **Screen Sharing** — Adaptive JPEG streaming with tile-based delta encoding
- **Input Control** — Full mouse and keyboard passthrough
- **Special Keys** — Ctrl+Alt+Del, Alt+Tab, Win+D, Alt+F4 and more
- **File Transfer** — Send files between host and viewer with progress tracking
- **Authentication** — Password-based (challenge-response) or one-time PIN
- **LAN Discovery** — Auto-discover hosts on the local network via UDP broadcast
- **TLS Encrypted** — All connections secured with TLS (auto-generated certificates)
- **Single Binary** — One executable runs as either Host or Viewer

## Supported Platforms

| Platform | Host (Share Screen) | Viewer (Connect) |
|----------|-------------------|------------------|
| Windows  | Yes | Yes |
| macOS    | Yes | Yes |
| Ubuntu / Linux | Yes | Yes |
| AlmaLinux / RHEL | Yes | Yes |
| Android  | — | Yes |
| iOS      | — | Yes |

## Quick Start

```bash
# Build for current platform
make build

# Run the application
./localwindows
```

Choose **Host** to share your screen, or **Viewer** to connect to a remote host.

### Host Mode
1. Select authentication mode (Password or PIN)
2. Set a password or generate a PIN
3. Click **Start Sharing**
4. Share the displayed IP/port and credentials with the viewer

### Viewer Mode
1. Discovered hosts appear automatically via LAN broadcast
2. Or enter the host IP and port manually
3. Enter the password or PIN
4. Click **Connect**

## Building

### Prerequisites

**Linux (Ubuntu/Debian):**
```bash
sudo apt-get install libx11-dev libxtst-dev libxrandr-dev libgl-dev \
    libxxf86vm-dev libxi-dev libxcursor-dev libxinerama-dev
```

**Linux (AlmaLinux/RHEL/Fedora):**
```bash
sudo dnf install libX11-devel libXtst-devel libXrandr-devel mesa-libGL-devel \
    libXxf86vm-devel libXi-devel libXcursor-devel libXinerama-devel
```

**macOS:** Xcode command line tools (`xcode-select --install`)

**Windows:** MinGW-w64 or TDM-GCC

### Build Targets

```bash
make build          # Current platform
make linux          # Linux amd64
make linux-arm64    # Linux arm64
make windows        # Windows amd64 (requires cross-compiler)
make macos          # macOS amd64
make macos-arm64    # macOS arm64 (Apple Silicon)
make android        # Android APK (requires fyne tool + Android SDK)
make ios            # iOS app (requires fyne tool + Xcode)
make all-desktop    # All desktop platforms
```

## Architecture

```
main.go                         Entry point (mode selector GUI)
internal/
├── protocol/
│   ├── message.go              Wire protocol messages and encoding
│   └── conn.go                 Framed TCP connection wrapper
├── auth/
│   └── auth.go                 Password + PIN authentication
├── capture/
│   ├── capture.go              Screen capture interface
│   ├── windows.go              Windows GDI capture
│   ├── darwin.go               macOS CoreGraphics capture
│   ├── linux.go                Linux X11 capture
│   └── stub.go                 Mobile stub (viewer-only)
├── input/
│   ├── input.go                Input injection interface + key mapping
│   ├── windows.go              Windows SendInput
│   ├── darwin.go               macOS CoreGraphics events
│   ├── linux.go                Linux XTest extension
│   └── stub.go                 Mobile stub
├── server/
│   └── server.go               Host server (capture + stream + input)
├── client/
│   └── client.go               Viewer client (connect + render + send input)
├── transfer/
│   └── transfer.go             Chunked file transfer manager
├── discovery/
│   └── discovery.go            LAN discovery (UDP broadcast)
└── gui/
    ├── app.go                  Fyne app and mode selector
    ├── host.go                 Host configuration screen
    ├── viewer.go               Viewer connection screen + remote desktop
    └── remoteview.go           Interactive remote screen widget
```

## Protocol

Custom binary protocol over TLS/TCP:

- **Wire format:** `[1 byte type][4 byte payload length][payload]`
- **Control messages:** JSON-encoded (auth, config, clipboard)
- **Screen frames:** Binary-encoded JPEG tiles with delta detection
- **File transfer:** 64KB chunked with SHA-256 checksums
- **Keepalive:** Ping/pong every 5 seconds, 15-second timeout

### Optimizations

- **Tile-based delta encoding** — Screen divided into 32×32 tiles; only changed tiles are sent
- **Adaptive full frames** — Full frame every 60 frames to prevent drift
- **JPEG quality control** — Adjustable 10–100% quality
- **FPS limiting** — Configurable 1–60 FPS

## Network

- **TCP port:** 19283 (configurable)
- **UDP discovery:** 19284

## License

MIT
