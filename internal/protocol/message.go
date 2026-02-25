package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"io"
)

const Version uint8 = 1

// Message types.
const (
	// Control
	MsgServerHello  uint8 = 0x01
	MsgAuthRequest  uint8 = 0x02
	MsgAuthResponse uint8 = 0x03
	MsgDisconnect   uint8 = 0x04

	// Screen
	MsgScreenInfo  uint8 = 0x10
	MsgFrameFull   uint8 = 0x11
	MsgFrameDelta  uint8 = 0x12
	MsgCursorPos   uint8 = 0x13

	// Input
	MsgMouseMove   uint8 = 0x20
	MsgMouseButton uint8 = 0x21
	MsgMouseScroll uint8 = 0x22
	MsgKeyEvent    uint8 = 0x23
	MsgSpecialKeys uint8 = 0x24

	// Clipboard
	MsgClipboard uint8 = 0x30

	// File transfer
	MsgFileOffer    uint8 = 0x40
	MsgFileAccept   uint8 = 0x41
	MsgFileReject   uint8 = 0x42
	MsgFileChunk    uint8 = 0x43
	MsgFileDone     uint8 = 0x44
	MsgFileCancel   uint8 = 0x45
	MsgFileProgress uint8 = 0x46

	// Keepalive
	MsgPing uint8 = 0xFE
	MsgPong uint8 = 0xFF
)

// AuthMode defines the authentication method.
type AuthMode uint8

const (
	AuthPassword AuthMode = 0
	AuthPIN      AuthMode = 1
)

// ServerHello is sent by the host upon connection.
type ServerHello struct {
	ServerName string   `json:"server_name"`
	AuthMode   AuthMode `json:"auth_mode"`
	ScreenW    int      `json:"screen_w"`
	ScreenH    int      `json:"screen_h"`
	Challenge  []byte   `json:"challenge"`
	Version    uint8    `json:"version"`
}

// AuthRequest is sent by the viewer to authenticate.
type AuthRequest struct {
	Mode     AuthMode `json:"mode"`
	Response []byte   `json:"response"` // SHA-256(credential + challenge) for password, raw PIN for PIN mode
}

// AuthResponse is sent by the host after authentication.
type AuthResponse struct {
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

// ScreenInfo describes the host's screen configuration.
type ScreenInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// MouseMoveMsg represents a mouse move event.
type MouseMoveMsg struct {
	X float64 `json:"x"` // Normalized 0.0 - 1.0
	Y float64 `json:"y"`
}

// MouseButtonMsg represents a mouse button event.
type MouseButtonMsg struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Button  uint8   `json:"button"`  // 0=left, 1=right, 2=middle
	Pressed bool    `json:"pressed"`
}

// MouseScrollMsg represents a mouse scroll event.
type MouseScrollMsg struct {
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

// KeyEventMsg represents a keyboard event.
type KeyEventMsg struct {
	Key     uint16 `json:"key"`
	Pressed bool   `json:"pressed"`
}

// SpecialKeysMsg triggers a special key combination on the host.
type SpecialKeysMsg struct {
	Combo uint8 `json:"combo"`
}

// Special key combos.
const (
	ComboCtrlAltDel uint8 = 0
	ComboAltTab     uint8 = 1
	ComboAltF4      uint8 = 2
	ComboCtrlEsc    uint8 = 3
	ComboWinL       uint8 = 4
	ComboWinD       uint8 = 5
)

// ClipboardMsg carries clipboard text.
type ClipboardMsg struct {
	Text string `json:"text"`
}

// FileOffer proposes a file transfer.
type FileOffer struct {
	TransferID string `json:"transfer_id"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
}

// FileAcceptMsg acknowledges a file transfer.
type FileAcceptMsg struct {
	TransferID string `json:"transfer_id"`
}

// FileRejectMsg rejects a file transfer.
type FileRejectMsg struct {
	TransferID string `json:"transfer_id"`
	Reason     string `json:"reason"`
}

// FileDoneMsg signals file transfer completion.
type FileDoneMsg struct {
	TransferID string `json:"transfer_id"`
	Checksum   string `json:"checksum"`
}

// FileCancelMsg cancels an in-progress transfer.
type FileCancelMsg struct {
	TransferID string `json:"transfer_id"`
}

// FileProgressMsg reports transfer progress.
type FileProgressMsg struct {
	TransferID  string `json:"transfer_id"`
	BytesSent   int64  `json:"bytes_sent"`
	TotalBytes  int64  `json:"total_bytes"`
}

// Tile represents a changed rectangular region in a delta frame.
type Tile struct {
	X, Y, W, H int
	Data        []byte
}

// EncodeJSON marshals a JSON-serializable message into a payload.
func EncodeJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// DecodeJSON unmarshals a JSON payload.
func DecodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// EncodeFullFrame packs a full JPEG frame into a binary payload.
// Format: [4:frameID][2:width][2:height][1:quality][jpeg_data]
func EncodeFullFrame(frameID uint32, width, height int, quality uint8, jpegData []byte) []byte {
	buf := make([]byte, 9+len(jpegData))
	binary.BigEndian.PutUint32(buf[0:4], frameID)
	binary.BigEndian.PutUint16(buf[4:6], uint16(width))
	binary.BigEndian.PutUint16(buf[6:8], uint16(height))
	buf[8] = quality
	copy(buf[9:], jpegData)
	return buf
}

// DecodeFullFrame unpacks a full frame payload.
func DecodeFullFrame(data []byte) (frameID uint32, width, height int, quality uint8, jpegData []byte, err error) {
	if len(data) < 9 {
		return 0, 0, 0, 0, nil, fmt.Errorf("full frame payload too short: %d bytes", len(data))
	}
	frameID = binary.BigEndian.Uint32(data[0:4])
	width = int(binary.BigEndian.Uint16(data[4:6]))
	height = int(binary.BigEndian.Uint16(data[6:8]))
	quality = data[8]
	jpegData = data[9:]
	return
}

// EncodeDeltaFrame packs changed tiles into a binary payload.
// Format: [4:frameID][2:tileCount][foreach tile: [2:x][2:y][2:w][2:h][4:dataLen][data]]
func EncodeDeltaFrame(frameID uint32, tiles []Tile) []byte {
	var buf bytes.Buffer
	hdr := make([]byte, 6)
	binary.BigEndian.PutUint32(hdr[0:4], frameID)
	binary.BigEndian.PutUint16(hdr[4:6], uint16(len(tiles)))
	buf.Write(hdr)

	tileBuf := make([]byte, 12)
	for _, t := range tiles {
		binary.BigEndian.PutUint16(tileBuf[0:2], uint16(t.X))
		binary.BigEndian.PutUint16(tileBuf[2:4], uint16(t.Y))
		binary.BigEndian.PutUint16(tileBuf[4:6], uint16(t.W))
		binary.BigEndian.PutUint16(tileBuf[6:8], uint16(t.H))
		binary.BigEndian.PutUint32(tileBuf[8:12], uint32(len(t.Data)))
		buf.Write(tileBuf)
		buf.Write(t.Data)
	}
	return buf.Bytes()
}

// DecodeDeltaFrame unpacks a delta frame payload.
func DecodeDeltaFrame(data []byte) (frameID uint32, tiles []Tile, err error) {
	if len(data) < 6 {
		return 0, nil, fmt.Errorf("delta frame payload too short: %d bytes", len(data))
	}
	r := bytes.NewReader(data)
	hdr := make([]byte, 6)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return 0, nil, err
	}
	frameID = binary.BigEndian.Uint32(hdr[0:4])
	tileCount := int(binary.BigEndian.Uint16(hdr[4:6]))

	tiles = make([]Tile, tileCount)
	tileBuf := make([]byte, 12)
	for i := range tileCount {
		if _, err := io.ReadFull(r, tileBuf); err != nil {
			return 0, nil, fmt.Errorf("tile %d header: %w", i, err)
		}
		tiles[i].X = int(binary.BigEndian.Uint16(tileBuf[0:2]))
		tiles[i].Y = int(binary.BigEndian.Uint16(tileBuf[2:4]))
		tiles[i].W = int(binary.BigEndian.Uint16(tileBuf[4:6]))
		tiles[i].H = int(binary.BigEndian.Uint16(tileBuf[6:8]))
		dataLen := int(binary.BigEndian.Uint32(tileBuf[8:12]))
		tiles[i].Data = make([]byte, dataLen)
		if _, err := io.ReadFull(r, tiles[i].Data); err != nil {
			return 0, nil, fmt.Errorf("tile %d data: %w", i, err)
		}
	}
	return
}

// EncodeFileChunk packs a file chunk.
// Format: [16:transferID_len][transferID][4:seqNum][data]
func EncodeFileChunk(transferID string, seqNum uint32, data []byte) []byte {
	idBytes := []byte(transferID)
	buf := make([]byte, 2+len(idBytes)+4+len(data))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(idBytes)))
	copy(buf[2:2+len(idBytes)], idBytes)
	off := 2 + len(idBytes)
	binary.BigEndian.PutUint32(buf[off:off+4], seqNum)
	copy(buf[off+4:], data)
	return buf
}

// DecodeFileChunk unpacks a file chunk.
func DecodeFileChunk(payload []byte) (transferID string, seqNum uint32, data []byte, err error) {
	if len(payload) < 2 {
		return "", 0, nil, fmt.Errorf("file chunk too short")
	}
	idLen := int(binary.BigEndian.Uint16(payload[0:2]))
	if len(payload) < 2+idLen+4 {
		return "", 0, nil, fmt.Errorf("file chunk too short for id")
	}
	transferID = string(payload[2 : 2+idLen])
	off := 2 + idLen
	seqNum = binary.BigEndian.Uint32(payload[off : off+4])
	data = payload[off+4:]
	return
}

// ImageBounds is a helper to encode image.Rectangle dimensions.
func ImageBounds(r image.Rectangle) (w, h int) {
	return r.Dx(), r.Dy()
}
