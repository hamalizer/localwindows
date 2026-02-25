package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sync"
	"time"

	"localwindows/internal/protocol"
)

const (
	challengeSize = 32
	pinLength     = 6
	pinTimeout    = 5 * time.Minute
)

// Manager handles authentication for the host.
type Manager struct {
	mu       sync.RWMutex
	mode     protocol.AuthMode
	password string
	pin      string
	pinExp   time.Time
}

// NewManager creates an auth manager with the given mode.
func NewManager(mode protocol.AuthMode) *Manager {
	return &Manager{mode: mode}
}

// Mode returns the current auth mode.
func (m *Manager) Mode() protocol.AuthMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// SetMode switches the auth mode.
func (m *Manager) SetMode(mode protocol.AuthMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mode = mode
}

// SetPassword sets the password for password-mode auth.
func (m *Manager) SetPassword(pw string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.password = pw
}

// GeneratePIN creates a new one-time PIN and returns it.
func (m *Manager) GeneratePIN() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(pinLength), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate pin: %w", err)
	}
	m.pin = fmt.Sprintf("%0*d", pinLength, n)
	m.pinExp = time.Now().Add(pinTimeout)
	return m.pin, nil
}

// PIN returns the current PIN (empty if not generated or expired).
func (m *Manager) PIN() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if time.Now().After(m.pinExp) {
		return ""
	}
	return m.pin
}

// GenerateChallenge creates a random challenge for password auth.
func GenerateChallenge() ([]byte, error) {
	ch := make([]byte, challengeSize)
	if _, err := rand.Read(ch); err != nil {
		return nil, err
	}
	return ch, nil
}

// ComputeResponse computes SHA-256(credential + challenge).
func ComputeResponse(credential string, challenge []byte) []byte {
	h := sha256.New()
	h.Write([]byte(credential))
	h.Write(challenge)
	return h.Sum(nil)
}

// Validate checks the client's auth response against stored credentials.
func (m *Manager) Validate(req protocol.AuthRequest, challenge []byte) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch req.Mode {
	case protocol.AuthPassword:
		expected := ComputeResponse(m.password, challenge)
		if !secureCompare(expected, req.Response) {
			return false, "invalid password"
		}
		return true, ""

	case protocol.AuthPIN:
		pin := string(req.Response)
		if time.Now().After(m.pinExp) {
			return false, "PIN expired"
		}
		if pin != m.pin {
			return false, "invalid PIN"
		}
		// PIN is one-time use; clear it.
		// Note: we hold RLock here, so defer the clear.
		go m.clearPIN()
		return true, ""

	default:
		return false, "unsupported auth mode"
	}
}

func (m *Manager) clearPIN() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pin = ""
	m.pinExp = time.Time{}
}

// secureCompare performs constant-time comparison of two byte slices.
func secureCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := range a {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
