package transfer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"localwindows/internal/protocol"
)

const chunkSize = 64 * 1024 // 64 KB

// Manager handles file transfers.
type Manager struct {
	mu        sync.Mutex
	active    map[string]*Transfer
	saveDir   string
	onProgress func(id string, sent, total int64)
	onComplete func(id string, name string)
	onError    func(id string, err error)
}

// Transfer represents a single file transfer.
type Transfer struct {
	ID       string
	FileName string
	FileSize int64
	Sent     int64
	Inbound  bool
	file     *os.File
}

// NewManager creates a file transfer manager.
func NewManager(saveDir string) *Manager {
	return &Manager{
		active:  make(map[string]*Transfer),
		saveDir: saveDir,
	}
}

// SetCallbacks sets event callbacks.
func (m *Manager) SetCallbacks(
	onProgress func(id string, sent, total int64),
	onComplete func(id string, name string),
	onError func(id string, err error),
) {
	m.onProgress = onProgress
	m.onComplete = onComplete
	m.onError = onError
}

// SendFile initiates sending a file over the connection.
func (m *Manager) SendFile(conn *protocol.Conn, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return "", fmt.Errorf("stat file: %w", err)
	}

	id := generateTransferID()
	name := filepath.Base(filePath)

	offer := protocol.FileOffer{
		TransferID: id,
		FileName:   name,
		FileSize:   stat.Size(),
	}
	if err := conn.WriteJSONMessage(protocol.MsgFileOffer, offer); err != nil {
		f.Close()
		return "", fmt.Errorf("send offer: %w", err)
	}

	t := &Transfer{
		ID:       id,
		FileName: name,
		FileSize: stat.Size(),
		Inbound:  false,
		file:     f,
	}
	m.mu.Lock()
	m.active[id] = t
	m.mu.Unlock()

	// Send in background.
	go m.sendChunks(conn, t)

	return id, nil
}

func (m *Manager) sendChunks(conn *protocol.Conn, t *Transfer) {
	defer t.file.Close()

	hasher := sha256.New()
	buf := make([]byte, chunkSize)
	var seq uint32

	for {
		n, err := t.file.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
			payload := protocol.EncodeFileChunk(t.ID, seq, buf[:n])
			if werr := conn.WriteMessage(protocol.MsgFileChunk, payload); werr != nil {
				if m.onError != nil {
					m.onError(t.ID, werr)
				}
				m.removeTransfer(t.ID)
				return
			}
			seq++
			t.Sent += int64(n)
			if m.onProgress != nil {
				m.onProgress(t.ID, t.Sent, t.FileSize)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if m.onError != nil {
				m.onError(t.ID, err)
			}
			m.removeTransfer(t.ID)
			return
		}
	}

	checksum := fmt.Sprintf("%x", hasher.Sum(nil))
	conn.WriteJSONMessage(protocol.MsgFileDone, protocol.FileDoneMsg{
		TransferID: t.ID,
		Checksum:   checksum,
	})

	if m.onComplete != nil {
		m.onComplete(t.ID, t.FileName)
	}
	m.removeTransfer(t.ID)
}

// AcceptFile begins receiving a file based on an incoming offer.
func (m *Manager) AcceptFile(conn *protocol.Conn, offer protocol.FileOffer) error {
	savePath := filepath.Join(m.saveDir, offer.FileName)
	// Avoid overwrites by appending a number.
	savePath = uniquePath(savePath)

	f, err := os.Create(savePath)
	if err != nil {
		conn.WriteJSONMessage(protocol.MsgFileReject, protocol.FileRejectMsg{
			TransferID: offer.TransferID,
			Reason:     err.Error(),
		})
		return err
	}

	t := &Transfer{
		ID:       offer.TransferID,
		FileName: filepath.Base(savePath),
		FileSize: offer.FileSize,
		Inbound:  true,
		file:     f,
	}
	m.mu.Lock()
	m.active[offer.TransferID] = t
	m.mu.Unlock()

	return conn.WriteJSONMessage(protocol.MsgFileAccept, protocol.FileAcceptMsg{
		TransferID: offer.TransferID,
	})
}

// HandleChunk processes an incoming file chunk.
func (m *Manager) HandleChunk(transferID string, data []byte) {
	m.mu.Lock()
	t, ok := m.active[transferID]
	m.mu.Unlock()
	if !ok {
		return
	}

	n, err := t.file.Write(data)
	if err != nil {
		if m.onError != nil {
			m.onError(transferID, err)
		}
		return
	}
	t.Sent += int64(n)
	if m.onProgress != nil {
		m.onProgress(transferID, t.Sent, t.FileSize)
	}
}

// HandleDone processes a file transfer completion message.
func (m *Manager) HandleDone(transferID string) {
	m.mu.Lock()
	t, ok := m.active[transferID]
	if ok {
		t.file.Close()
		delete(m.active, transferID)
	}
	m.mu.Unlock()

	if ok && m.onComplete != nil {
		m.onComplete(transferID, t.FileName)
	}
}

// CancelTransfer cancels an active transfer.
func (m *Manager) CancelTransfer(conn *protocol.Conn, transferID string) {
	m.mu.Lock()
	t, ok := m.active[transferID]
	if ok {
		t.file.Close()
		delete(m.active, transferID)
	}
	m.mu.Unlock()
	if ok {
		conn.WriteJSONMessage(protocol.MsgFileCancel, protocol.FileCancelMsg{
			TransferID: transferID,
		})
	}
}

func (m *Manager) removeTransfer(id string) {
	m.mu.Lock()
	delete(m.active, id)
	m.mu.Unlock()
}

// ActiveTransfers returns a snapshot of active transfers.
func (m *Manager) ActiveTransfers() []Transfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Transfer, 0, len(m.active))
	for _, t := range m.active {
		result = append(result, *t)
	}
	return result
}

func generateTransferID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func uniquePath(path string) string {
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
