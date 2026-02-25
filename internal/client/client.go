package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"log"
	"net"
	"sync"
	"time"

	"localwindows/internal/auth"
	"localwindows/internal/protocol"
)

const (
	connectTimeout   = 10 * time.Second
	keepaliveEvery   = 5 * time.Second
	readTimeout      = 15 * time.Second
)

// Client connects to a remote desktop host.
type Client struct {
	conn    *protocol.Conn
	rawConn net.Conn
	ctx     context.Context
	cancel  context.CancelFunc

	mu       sync.RWMutex
	frame    *image.RGBA
	screenW  int
	screenH  int
	running  bool
	frameID  uint32

	// Callbacks
	OnFrameUpdate  func(*image.RGBA)
	OnDisconnect   func(error)
	OnFileOffer    func(protocol.FileOffer)
	OnFileProgress func(transferID string, received, total int64)
	OnFileDone     func(transferID string)
	OnClipboard    func(text string)
}

// ConnectConfig holds connection parameters.
type ConnectConfig struct {
	Host       string
	Port       int
	AuthMode   protocol.AuthMode
	Credential string // password or PIN
}

// New creates a new client.
func New() *Client {
	return &Client{}
}

// Connect establishes a connection to the host and authenticates.
func (c *Client) Connect(cfg ConnectConfig) (*protocol.ServerHello, error) {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	dialer := &net.Dialer{Timeout: connectTimeout}
	tlsCfg := &tls.Config{InsecureSkipVerify: true} // Accept self-signed certs for LAN
	rawConn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}

	c.rawConn = rawConn
	c.conn = protocol.WrapConn(rawConn)

	// Read server hello.
	var hello protocol.ServerHello
	msgType, err := c.conn.ReadJSONMessage(&hello)
	if err != nil {
		c.conn.Close()
		return nil, fmt.Errorf("read server hello: %w", err)
	}
	if msgType != protocol.MsgServerHello {
		c.conn.Close()
		return nil, fmt.Errorf("unexpected message type: 0x%02x", msgType)
	}

	// Send auth request.
	var authReq protocol.AuthRequest
	authReq.Mode = cfg.AuthMode
	switch cfg.AuthMode {
	case protocol.AuthPassword:
		authReq.Response = auth.ComputeResponse(cfg.Credential, hello.Challenge)
	case protocol.AuthPIN:
		authReq.Response = []byte(cfg.Credential)
	}
	if err := c.conn.WriteJSONMessage(protocol.MsgAuthRequest, authReq); err != nil {
		c.conn.Close()
		return nil, fmt.Errorf("send auth: %w", err)
	}

	// Read auth response.
	var authResp protocol.AuthResponse
	msgType, err = c.conn.ReadJSONMessage(&authResp)
	if err != nil {
		c.conn.Close()
		return nil, fmt.Errorf("read auth response: %w", err)
	}
	if !authResp.Success {
		c.conn.Close()
		return nil, fmt.Errorf("authentication failed: %s", authResp.Reason)
	}

	// Read screen info.
	var screenInfo protocol.ScreenInfo
	c.conn.ReadJSONMessage(&screenInfo)

	c.mu.Lock()
	c.screenW = screenInfo.Width
	c.screenH = screenInfo.Height
	c.frame = image.NewRGBA(image.Rect(0, 0, c.screenW, c.screenH))
	c.running = true
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	// Start receiving frames.
	go c.receiveLoop()
	go c.keepaliveLoop()

	return &hello, nil
}

func (c *Client) receiveLoop() {
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(readTimeout))
		msgType, payload, err := c.conn.ReadMessage()
		if err != nil {
			if c.OnDisconnect != nil {
				c.OnDisconnect(err)
			}
			return
		}

		switch msgType {
		case protocol.MsgFrameFull:
			c.handleFullFrame(payload)
		case protocol.MsgFrameDelta:
			c.handleDeltaFrame(payload)
		case protocol.MsgFileOffer:
			var offer protocol.FileOffer
			if err := protocol.DecodeJSON(payload, &offer); err == nil && c.OnFileOffer != nil {
				c.OnFileOffer(offer)
			}
		case protocol.MsgFileAccept:
			// File transfer accepted; handled by transfer manager.
		case protocol.MsgFileProgress:
			var prog protocol.FileProgressMsg
			if err := protocol.DecodeJSON(payload, &prog); err == nil && c.OnFileProgress != nil {
				c.OnFileProgress(prog.TransferID, prog.BytesSent, prog.TotalBytes)
			}
		case protocol.MsgFileDone:
			var done protocol.FileDoneMsg
			if err := protocol.DecodeJSON(payload, &done); err == nil && c.OnFileDone != nil {
				c.OnFileDone(done.TransferID)
			}
		case protocol.MsgClipboard:
			var clip protocol.ClipboardMsg
			if err := protocol.DecodeJSON(payload, &clip); err == nil && c.OnClipboard != nil {
				c.OnClipboard(clip.Text)
			}
		case protocol.MsgPong:
			// Keepalive response
		case protocol.MsgDisconnect:
			if c.OnDisconnect != nil {
				c.OnDisconnect(nil)
			}
			return
		}
	}
}

func (c *Client) handleFullFrame(payload []byte) {
	_, w, h, _, jpegData, err := protocol.DecodeFullFrame(payload)
	if err != nil {
		log.Printf("decode full frame: %v", err)
		return
	}

	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		log.Printf("decode jpeg: %v", err)
		return
	}

	c.mu.Lock()
	if c.screenW != w || c.screenH != h {
		c.screenW = w
		c.screenH = h
		c.frame = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	draw.Draw(c.frame, c.frame.Bounds(), img, image.Point{}, draw.Src)
	c.mu.Unlock()

	if c.OnFrameUpdate != nil {
		c.mu.RLock()
		f := c.frame
		c.mu.RUnlock()
		c.OnFrameUpdate(f)
	}
}

func (c *Client) handleDeltaFrame(payload []byte) {
	_, tiles, err := protocol.DecodeDeltaFrame(payload)
	if err != nil {
		log.Printf("decode delta frame: %v", err)
		return
	}

	c.mu.Lock()
	for _, tile := range tiles {
		tileImg, err := jpeg.Decode(bytes.NewReader(tile.Data))
		if err != nil {
			continue
		}
		r := image.Rect(tile.X, tile.Y, tile.X+tile.W, tile.Y+tile.H)
		draw.Draw(c.frame, r, tileImg, tileImg.Bounds().Min, draw.Src)
	}
	c.mu.Unlock()

	if c.OnFrameUpdate != nil {
		c.mu.RLock()
		f := c.frame
		c.mu.RUnlock()
		c.OnFrameUpdate(f)
	}
}

func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(keepaliveEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.conn.WriteMessage(protocol.MsgPing, nil); err != nil {
				return
			}
		}
	}
}

// SendMouseMove sends a mouse move event with normalized coordinates.
func (c *Client) SendMouseMove(nx, ny float64) error {
	return c.conn.WriteJSONMessage(protocol.MsgMouseMove, protocol.MouseMoveMsg{X: nx, Y: ny})
}

// SendMouseButton sends a mouse button event.
func (c *Client) SendMouseButton(nx, ny float64, button uint8, pressed bool) error {
	return c.conn.WriteJSONMessage(protocol.MsgMouseButton, protocol.MouseButtonMsg{
		X: nx, Y: ny, Button: button, Pressed: pressed,
	})
}

// SendMouseScroll sends a scroll event.
func (c *Client) SendMouseScroll(nx, ny, dx, dy float64) error {
	return c.conn.WriteJSONMessage(protocol.MsgMouseScroll, protocol.MouseScrollMsg{
		X: nx, Y: ny, DX: dx, DY: dy,
	})
}

// SendKeyEvent sends a key event.
func (c *Client) SendKeyEvent(key uint16, pressed bool) error {
	return c.conn.WriteJSONMessage(protocol.MsgKeyEvent, protocol.KeyEventMsg{
		Key: key, Pressed: pressed,
	})
}

// SendSpecialCombo sends a special key combination.
func (c *Client) SendSpecialCombo(combo uint8) error {
	return c.conn.WriteJSONMessage(protocol.MsgSpecialKeys, protocol.SpecialKeysMsg{Combo: combo})
}

// SendClipboard sends clipboard text.
func (c *Client) SendClipboard(text string) error {
	return c.conn.WriteJSONMessage(protocol.MsgClipboard, protocol.ClipboardMsg{Text: text})
}

// Frame returns the current frame (may be nil before first frame).
func (c *Client) Frame() *image.RGBA {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.frame
}

// ScreenSize returns the remote screen dimensions.
func (c *Client) ScreenSize() (int, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.screenW, c.screenH
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// Conn returns the underlying protocol connection for file transfers.
func (c *Client) Conn() *protocol.Conn {
	return c.conn
}

// Disconnect closes the connection.
func (c *Client) Disconnect() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	c.mu.Unlock()

	c.cancel()
	c.conn.WriteMessage(protocol.MsgDisconnect, nil)
	c.conn.Close()
}
