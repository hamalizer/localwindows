package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"hash/fnv"
	"image"
	"image/jpeg"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"localwindows/internal/auth"
	"localwindows/internal/capture"
	"localwindows/internal/discovery"
	"localwindows/internal/input"
	"localwindows/internal/protocol"
)

const (
	DefaultPort        = 19283
	tileSize           = 32
	defaultQuality     = 75
	defaultFPS         = 30
	fullFrameInterval  = 60 // send full frame every N frames
	keepaliveInterval  = 5 * time.Second
	keepaliveTimeout   = 15 * time.Second
)

// Config holds server configuration.
type Config struct {
	Port       int
	ServerName string
	Quality    int
	MaxFPS     int
	ReceiveDir string // directory for received files; defaults to user home
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "LocalWindows Host"
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "."
	}
	return Config{
		Port:       DefaultPort,
		ServerName: hostname,
		Quality:    defaultQuality,
		MaxFPS:     defaultFPS,
		ReceiveDir: homeDir,
	}
}

// Server is the remote desktop host.
type Server struct {
	config    Config
	auth      *auth.Manager
	capturer  capture.Capturer
	injector  input.Injector
	listener  net.Listener
	broadcast *discovery.Broadcaster

	mu       sync.RWMutex
	clients  map[string]*clientConn
	running  bool
	ctx      context.Context
	cancel   context.CancelFunc

	// Per-server file transfer state.
	fileTransfers   map[string]*fileTransferState
	fileTransfersMu sync.Mutex

	// Callbacks
	OnClientConnect     func(addr string)
	OnClientDisconnect  func(addr string)
	OnError             func(err error)
	OnFileReceived      func(name string, size int64)
	OnClipboardReceived func(text string)
}

type clientConn struct {
	conn   *protocol.Conn
	addr   string
	cancel context.CancelFunc
}

// New creates a new server with the given auth manager and config.
func New(authMgr *auth.Manager, cfg Config) *Server {
	return &Server{
		config:        cfg,
		auth:          authMgr,
		clients:       make(map[string]*clientConn),
		fileTransfers: make(map[string]*fileTransferState),
	}
}

// Start initializes capture/input and begins listening for connections.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.mu.Unlock()

	// Init screen capture.
	s.capturer = capture.New()
	if err := s.capturer.Init(); err != nil {
		s.running = false
		return fmt.Errorf("init capture: %w", err)
	}

	// Init input injection.
	s.injector = input.New()
	if err := s.injector.Init(); err != nil {
		log.Printf("warning: input injection unavailable: %v", err)
		s.injector = nil // Mark as unavailable to prevent crash.
	}

	// Generate TLS config with self-signed cert.
	tlsCfg, err := generateTLSConfig()
	if err != nil {
		s.running = false
		return fmt.Errorf("tls config: %w", err)
	}

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.listener, err = tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		s.running = false
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	// Start LAN discovery broadcaster.
	localIP := discovery.GetLocalIP()
	s.broadcast = discovery.NewBroadcaster(discovery.Host{
		Name:     s.config.ServerName,
		IP:       localIP,
		Port:     s.config.Port,
		AuthMode: s.auth.Mode(),
		Version:  protocol.Version,
	})
	if err := s.broadcast.Start(); err != nil {
		log.Printf("discovery broadcast failed: %v (continuing without discovery)", err)
	}

	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	for {
		raw, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				if s.OnError != nil {
					s.OnError(fmt.Errorf("accept: %w", err))
				}
				continue
			}
		}
		go s.handleClient(raw)
	}
}

func (s *Server) handleClient(raw net.Conn) {
	conn := protocol.WrapConn(raw)
	addr := raw.RemoteAddr().String()

	// Authenticate.
	challenge, err := auth.GenerateChallenge()
	if err != nil {
		conn.Close()
		return
	}

	w, h := s.capturer.ScreenSize()
	hello := protocol.ServerHello{
		ServerName: s.config.ServerName,
		AuthMode:   s.auth.Mode(),
		ScreenW:    w,
		ScreenH:    h,
		Challenge:  challenge,
		Version:    protocol.Version,
	}
	if err := conn.WriteJSONMessage(protocol.MsgServerHello, hello); err != nil {
		conn.Close()
		return
	}

	// Read auth request with timeout.
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	msgType, payload, err := conn.ReadMessage()
	if err != nil || msgType != protocol.MsgAuthRequest {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	var authReq protocol.AuthRequest
	if err := protocol.DecodeJSON(payload, &authReq); err != nil {
		conn.Close()
		return
	}

	ok, reason := s.auth.Validate(authReq, challenge)
	resp := protocol.AuthResponse{Success: ok, Reason: reason}
	conn.WriteJSONMessage(protocol.MsgAuthResponse, resp)
	if !ok {
		conn.Close()
		return
	}

	// Send screen info.
	conn.WriteJSONMessage(protocol.MsgScreenInfo, protocol.ScreenInfo{Width: w, Height: h})

	// Register client.
	clientCtx, clientCancel := context.WithCancel(s.ctx)
	cc := &clientConn{conn: conn, addr: addr, cancel: clientCancel}
	s.mu.Lock()
	s.clients[addr] = cc
	s.mu.Unlock()

	if s.OnClientConnect != nil {
		s.OnClientConnect(addr)
	}

	// Start goroutines for this client.
	// When either goroutine exits (e.g. conn error), cancel the context
	// and close the conn so the other goroutine unblocks immediately
	// instead of waiting up to the keepalive timeout.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer clientCancel()
		defer conn.Close()
		s.streamFrames(clientCtx, conn)
	}()
	go func() {
		defer wg.Done()
		defer clientCancel()
		defer conn.Close()
		s.handleInputEvents(clientCtx, conn)
	}()
	wg.Wait()

	// Cleanup.
	conn.Close()
	s.mu.Lock()
	delete(s.clients, addr)
	s.mu.Unlock()

	if s.OnClientDisconnect != nil {
		s.OnClientDisconnect(addr)
	}
}

// quality returns the current JPEG quality, safe for concurrent reads.
func (s *Server) quality() int {
	s.mu.RLock()
	q := s.config.Quality
	s.mu.RUnlock()
	return q
}

// maxFPS returns the current max FPS, safe for concurrent reads.
func (s *Server) maxFPS() int {
	s.mu.RLock()
	fps := s.config.MaxFPS
	s.mu.RUnlock()
	return fps
}

func (s *Server) streamFrames(ctx context.Context, conn *protocol.Conn) {
	fps := s.maxFPS()
	interval := time.Duration(1000/fps) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prevHashes []uint64
	var frameID uint32
	var lastFPS int = fps

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Adapt ticker if FPS setting changed.
		curFPS := s.maxFPS()
		if curFPS != lastFPS {
			lastFPS = curFPS
			ticker.Reset(time.Duration(1000/curFPS) * time.Millisecond)
		}

		frame, err := s.capturer.Capture()
		if err != nil {
			continue
		}

		quality := s.quality()

		frameID++
		bounds := frame.Rect
		w, h := bounds.Dx(), bounds.Dy()
		tilesX := (w + tileSize - 1) / tileSize
		tilesY := (h + tileSize - 1) / tileSize
		totalTiles := tilesX * tilesY

		// Compute hashes for each tile.
		newHashes := make([]uint64, totalTiles)
		for ty := range tilesY {
			for tx := range tilesX {
				idx := ty*tilesX + tx
				x0 := tx * tileSize
				y0 := ty * tileSize
				x1 := min(x0+tileSize, w)
				y1 := min(y0+tileSize, h)
				newHashes[idx] = hashRegion(frame, x0, y0, x1, y1)
			}
		}

		// Decide: full frame or delta.
		sendFull := prevHashes == nil ||
			len(prevHashes) != totalTiles ||
			frameID%fullFrameInterval == 0

		if sendFull {
			jpegData := encodeJPEG(frame, quality)
			payload := protocol.EncodeFullFrame(frameID, w, h, uint8(quality), jpegData)
			if err := conn.WriteMessage(protocol.MsgFrameFull, payload); err != nil {
				return
			}
		} else {
			// Find changed tiles.
			var tiles []protocol.Tile
			for ty := range tilesY {
				for tx := range tilesX {
					idx := ty*tilesX + tx
					if newHashes[idx] != prevHashes[idx] {
						x0 := tx * tileSize
						y0 := ty * tileSize
						x1 := min(x0+tileSize, w)
						y1 := min(y0+tileSize, h)
						sub := frame.SubImage(image.Rect(x0, y0, x1, y1)).(*image.RGBA)
						tiles = append(tiles, protocol.Tile{
							X: x0, Y: y0,
							W: x1 - x0, H: y1 - y0,
							Data: encodeJPEG(sub, quality),
						})
					}
				}
			}
			if len(tiles) == 0 {
				prevHashes = newHashes
				continue
			}
			// If more than half changed, send full frame instead.
			if len(tiles) > totalTiles/2 {
				jpegData := encodeJPEG(frame, quality)
				payload := protocol.EncodeFullFrame(frameID, w, h, uint8(quality), jpegData)
				if err := conn.WriteMessage(protocol.MsgFrameFull, payload); err != nil {
					return
				}
			} else {
				payload := protocol.EncodeDeltaFrame(frameID, tiles)
				if err := conn.WriteMessage(protocol.MsgFrameDelta, payload); err != nil {
					return
				}
			}
		}
		prevHashes = newHashes
	}
}

func (s *Server) handleInputEvents(ctx context.Context, conn *protocol.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(keepaliveTimeout))
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch msgType {
		case protocol.MsgMouseMove:
			if s.injector == nil {
				continue
			}
			var msg protocol.MouseMoveMsg
			if err := protocol.DecodeJSON(payload, &msg); err != nil {
				continue
			}
			screenW, screenH := s.capturer.ScreenSize()
			x := int(msg.X * float64(screenW))
			y := int(msg.Y * float64(screenH))
			s.injector.MouseMove(x, y)

		case protocol.MsgMouseButton:
			if s.injector == nil {
				continue
			}
			var msg protocol.MouseButtonMsg
			if err := protocol.DecodeJSON(payload, &msg); err != nil {
				continue
			}
			screenW, screenH := s.capturer.ScreenSize()
			x := int(msg.X * float64(screenW))
			y := int(msg.Y * float64(screenH))
			s.injector.MouseMove(x, y)
			s.injector.MouseButton(msg.Button, msg.Pressed)

		case protocol.MsgMouseScroll:
			if s.injector == nil {
				continue
			}
			var msg protocol.MouseScrollMsg
			if err := protocol.DecodeJSON(payload, &msg); err != nil {
				continue
			}
			s.injector.MouseScroll(int(msg.DX), int(msg.DY))

		case protocol.MsgKeyEvent:
			if s.injector == nil {
				continue
			}
			var msg protocol.KeyEventMsg
			if err := protocol.DecodeJSON(payload, &msg); err != nil {
				continue
			}
			s.injector.KeyPress(input.Key(msg.Key), msg.Pressed)

		case protocol.MsgSpecialKeys:
			if s.injector == nil {
				continue
			}
			var msg protocol.SpecialKeysMsg
			if err := protocol.DecodeJSON(payload, &msg); err != nil {
				continue
			}
			s.injector.SendSpecialCombo(msg.Combo)

		case protocol.MsgFileOffer:
			s.handleFileOffer(conn, payload)

		case protocol.MsgFileChunk:
			s.handleFileChunk(payload)

		case protocol.MsgFileDone:
			s.handleFileDone(payload)

		case protocol.MsgClipboard:
			var clip protocol.ClipboardMsg
			if err := protocol.DecodeJSON(payload, &clip); err == nil && s.OnClipboardReceived != nil {
				s.OnClipboardReceived(clip.Text)
			}

		case protocol.MsgPing:
			conn.WriteMessage(protocol.MsgPong, nil)

		case protocol.MsgDisconnect:
			return
		}
	}
}

type fileTransferState struct {
	fileName string
	fileSize int64
	received int64
	file     *os.File
}

func (s *Server) handleFileOffer(conn *protocol.Conn, payload []byte) {
	var offer protocol.FileOffer
	if err := protocol.DecodeJSON(payload, &offer); err != nil {
		return
	}

	// Save to receive directory with dedup.
	savePath := filepath.Join(s.config.ReceiveDir, filepath.Base(offer.FileName))
	savePath = uniqueFilePath(savePath)
	f, err := os.Create(savePath)
	if err != nil {
		conn.WriteJSONMessage(protocol.MsgFileReject, protocol.FileRejectMsg{
			TransferID: offer.TransferID,
			Reason:     err.Error(),
		})
		return
	}

	s.fileTransfersMu.Lock()
	s.fileTransfers[offer.TransferID] = &fileTransferState{
		fileName: offer.FileName,
		fileSize: offer.FileSize,
		file:     f,
	}
	s.fileTransfersMu.Unlock()

	conn.WriteJSONMessage(protocol.MsgFileAccept, protocol.FileAcceptMsg{
		TransferID: offer.TransferID,
	})
}

func (s *Server) handleFileChunk(payload []byte) {
	transferID, _, data, err := protocol.DecodeFileChunk(payload)
	if err != nil {
		return
	}
	s.fileTransfersMu.Lock()
	state, ok := s.fileTransfers[transferID]
	s.fileTransfersMu.Unlock()
	if !ok {
		return
	}
	n, werr := state.file.Write(data)
	if werr != nil {
		log.Printf("file write error for %s: %v", transferID, werr)
		// Close and remove the failed transfer.
		state.file.Close()
		s.fileTransfersMu.Lock()
		delete(s.fileTransfers, transferID)
		s.fileTransfersMu.Unlock()
		return
	}
	state.received += int64(n)
}

func (s *Server) handleFileDone(payload []byte) {
	var msg protocol.FileDoneMsg
	if err := protocol.DecodeJSON(payload, &msg); err != nil {
		return
	}
	s.fileTransfersMu.Lock()
	state, ok := s.fileTransfers[msg.TransferID]
	if ok {
		state.file.Close()
		delete(s.fileTransfers, msg.TransferID)
	}
	s.fileTransfersMu.Unlock()

	if ok && s.OnFileReceived != nil {
		s.OnFileReceived(state.fileName, state.received)
	}
}

// Stop shuts down the server gracefully.
func (s *Server) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()

	if s.broadcast != nil {
		s.broadcast.Stop()
	}
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all client connections.
	s.mu.RLock()
	for _, cc := range s.clients {
		cc.cancel()
		cc.conn.Close()
	}
	s.mu.RUnlock()

	if s.capturer != nil {
		s.capturer.Close()
	}
	if s.injector != nil {
		s.injector.Close()
	}
}

// IsRunning reports whether the server is active.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// ClientCount returns the number of connected clients.
func (s *Server) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// SetQuality updates the JPEG quality for future frames.
func (s *Server) SetQuality(q int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q < 1 {
		q = 1
	}
	if q > 100 {
		q = 100
	}
	s.config.Quality = q
}

// SetMaxFPS updates the maximum frame rate.
func (s *Server) SetMaxFPS(fps int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fps < 1 {
		fps = 1
	}
	if fps > 60 {
		fps = 60
	}
	s.config.MaxFPS = fps
}

// Address returns the listening address.
func (s *Server) Address() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// --- Helpers ---

func hashRegion(img *image.RGBA, x0, y0, x1, y1 int) uint64 {
	h := fnv.New64a()
	stride := img.Stride
	for y := y0; y < y1; y++ {
		start := y*stride + x0*4
		end := y*stride + x1*4
		if end > len(img.Pix) {
			end = len(img.Pix)
		}
		if start >= len(img.Pix) {
			break
		}
		h.Write(img.Pix[start:end])
	}
	return h.Sum64()
}

func encodeJPEG(img image.Image, quality int) []byte {
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	return buf.Bytes()
}

func uniqueFilePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func generateTLSConfig() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}
