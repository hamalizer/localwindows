package transfer

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"localwindows/internal/protocol"
)

func TestTransferIDUniqueness(t *testing.T) {
	id1 := generateTransferID()
	id2 := generateTransferID()
	if id1 == id2 {
		t.Error("two transfer IDs should not be identical")
	}
	if len(id1) != 16 { // 8 bytes hex-encoded = 16 chars
		t.Errorf("id length = %d, want 16", len(id1))
	}
}

func TestUniquePathNoConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	got := uniquePath(path)
	if got != path {
		t.Errorf("uniquePath = %q, want %q (no conflict)", got, path)
	}
}

func TestUniquePathWithConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Create the conflicting file.
	f, _ := os.Create(path)
	f.Close()

	got := uniquePath(path)
	expected := filepath.Join(dir, "test_1.txt")
	if got != expected {
		t.Errorf("uniquePath = %q, want %q", got, expected)
	}
}

func TestUniquePathMultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")

	for _, name := range []string{"data.bin", "data_1.bin", "data_2.bin"} {
		f, _ := os.Create(filepath.Join(dir, name))
		f.Close()
	}

	got := uniquePath(path)
	expected := filepath.Join(dir, "data_3.bin")
	if got != expected {
		t.Errorf("uniquePath = %q, want %q", got, expected)
	}
}

func TestManagerSendFileNonexistent(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	conn := protocol.WrapConn(client)
	mgr := NewManager(t.TempDir())

	_, err := mgr.SendFile(conn, "/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestManagerSendFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "hello.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	server, client := net.Pipe()

	conn := protocol.WrapConn(client)
	mgr := NewManager(dir)

	completedCh := make(chan string, 1)
	mgr.SetCallbacks(
		func(id string, sent, total int64) {},
		func(id string, name string) {
			completedCh <- name
		},
		func(id string, err error) {
			// Expected when pipe closes; ignore.
		},
	)

	// Drain the server side in a goroutine to prevent blocking.
	// Keep it alive until send completes.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		sc := protocol.WrapConn(server)
		for {
			_, _, err := sc.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	id, err := mgr.SendFile(conn, testFile)
	if err != nil {
		server.Close()
		client.Close()
		t.Fatalf("SendFile: %v", err)
	}
	if id == "" {
		server.Close()
		client.Close()
		t.Fatal("expected non-empty transfer ID")
	}

	// Wait for completion callback.
	select {
	case name := <-completedCh:
		if name != "hello.txt" {
			t.Errorf("completedName = %q, want %q", name, "hello.txt")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for send completion")
	}

	// Clean up pipe.
	server.Close()
	client.Close()
	<-drainDone
}

func TestManagerActiveTransfers(t *testing.T) {
	mgr := NewManager(t.TempDir())
	active := mgr.ActiveTransfers()
	if len(active) != 0 {
		t.Errorf("expected 0 active transfers, got %d", len(active))
	}
}
