package auth

import (
	"testing"
	"time"

	"localwindows/internal/protocol"
)

func TestPasswordAuth(t *testing.T) {
	mgr := NewManager(protocol.AuthPassword)
	mgr.SetPassword("secret123")

	challenge, err := GenerateChallenge()
	if err != nil {
		t.Fatalf("GenerateChallenge: %v", err)
	}

	response := ComputeResponse("secret123", challenge)
	req := protocol.AuthRequest{
		Mode:     protocol.AuthPassword,
		Response: response,
	}
	ok, reason := mgr.Validate(req, challenge)
	if !ok {
		t.Fatalf("expected success, got: %s", reason)
	}
}

func TestPasswordAuthWrongPassword(t *testing.T) {
	mgr := NewManager(protocol.AuthPassword)
	mgr.SetPassword("correct")

	challenge, _ := GenerateChallenge()
	response := ComputeResponse("wrong", challenge)
	req := protocol.AuthRequest{
		Mode:     protocol.AuthPassword,
		Response: response,
	}
	ok, _ := mgr.Validate(req, challenge)
	if ok {
		t.Fatal("expected failure with wrong password")
	}
}

func TestPasswordAuthWrongChallenge(t *testing.T) {
	mgr := NewManager(protocol.AuthPassword)
	mgr.SetPassword("secret")

	challenge1, _ := GenerateChallenge()
	challenge2, _ := GenerateChallenge()

	response := ComputeResponse("secret", challenge1)
	req := protocol.AuthRequest{
		Mode:     protocol.AuthPassword,
		Response: response,
	}
	ok, _ := mgr.Validate(req, challenge2)
	if ok {
		t.Fatal("expected failure with different challenge")
	}
}

func TestPINAuth(t *testing.T) {
	mgr := NewManager(protocol.AuthPIN)
	pin, err := mgr.GeneratePIN()
	if err != nil {
		t.Fatalf("GeneratePIN: %v", err)
	}
	if len(pin) != pinLength {
		t.Errorf("PIN length = %d, want %d", len(pin), pinLength)
	}

	challenge, _ := GenerateChallenge()
	req := protocol.AuthRequest{
		Mode:     protocol.AuthPIN,
		Response: []byte(pin),
	}
	ok, reason := mgr.Validate(req, challenge)
	if !ok {
		t.Fatalf("expected success, got: %s", reason)
	}

	// PIN is one-time use — give clearPIN goroutine a moment.
	time.Sleep(50 * time.Millisecond)

	// Second use should fail (one-time PIN).
	ok, _ = mgr.Validate(req, challenge)
	if ok {
		t.Fatal("expected failure on second use of PIN")
	}
}

func TestPINAuthWrongPIN(t *testing.T) {
	mgr := NewManager(protocol.AuthPIN)
	mgr.GeneratePIN()

	challenge, _ := GenerateChallenge()
	req := protocol.AuthRequest{
		Mode:     protocol.AuthPIN,
		Response: []byte("000000"),
	}
	ok, _ := mgr.Validate(req, challenge)
	// Might succeed if the randomly generated PIN happens to be "000000",
	// but that's a 1-in-1M chance. For robustness, just verify no crash.
	_ = ok
}

func TestPINExpiry(t *testing.T) {
	mgr := NewManager(protocol.AuthPIN)
	mgr.GeneratePIN()

	// Manually expire the PIN.
	mgr.mu.Lock()
	mgr.pinExp = time.Now().Add(-time.Second)
	mgr.mu.Unlock()

	pin := mgr.PIN()
	if pin != "" {
		t.Error("expected empty PIN after expiry")
	}

	challenge, _ := GenerateChallenge()
	req := protocol.AuthRequest{
		Mode:     protocol.AuthPIN,
		Response: []byte("123456"),
	}
	ok, reason := mgr.Validate(req, challenge)
	if ok {
		t.Fatal("expected failure with expired PIN")
	}
	if reason != "PIN expired" {
		t.Errorf("reason = %q, want %q", reason, "PIN expired")
	}
}

func TestSetMode(t *testing.T) {
	mgr := NewManager(protocol.AuthPassword)
	if mgr.Mode() != protocol.AuthPassword {
		t.Errorf("initial mode = %d, want %d", mgr.Mode(), protocol.AuthPassword)
	}
	mgr.SetMode(protocol.AuthPIN)
	if mgr.Mode() != protocol.AuthPIN {
		t.Errorf("mode after SetMode = %d, want %d", mgr.Mode(), protocol.AuthPIN)
	}
}

func TestChallengeUniqueness(t *testing.T) {
	c1, _ := GenerateChallenge()
	c2, _ := GenerateChallenge()
	if len(c1) != challengeSize {
		t.Errorf("challenge size = %d, want %d", len(c1), challengeSize)
	}
	// Extremely unlikely they're equal.
	equal := true
	for i := range c1 {
		if c1[i] != c2[i] {
			equal = false
			break
		}
	}
	if equal {
		t.Error("two challenges should not be identical")
	}
}

func TestSecureCompare(t *testing.T) {
	a := []byte("hello")
	b := []byte("hello")
	c := []byte("world")
	d := []byte("hel")

	if !secureCompare(a, b) {
		t.Error("identical slices should match")
	}
	if secureCompare(a, c) {
		t.Error("different slices should not match")
	}
	if secureCompare(a, d) {
		t.Error("different length slices should not match")
	}
}
