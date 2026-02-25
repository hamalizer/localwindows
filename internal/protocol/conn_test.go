package protocol

import (
	"bytes"
	"net"
	"testing"
)

func TestConnReadWriteMessage(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sc := WrapConn(server)
	cc := WrapConn(client)

	payload := []byte("hello world")
	done := make(chan error, 1)

	go func() {
		done <- cc.WriteMessage(0x20, payload)
	}()

	msgType, got, err := sc.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msgType != 0x20 {
		t.Errorf("msgType = 0x%02x, want 0x20", msgType)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch")
	}

	if err := <-done; err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
}

func TestConnReadWriteNilPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sc := WrapConn(server)
	cc := WrapConn(client)

	done := make(chan error, 1)
	go func() {
		done <- cc.WriteMessage(MsgPing, nil)
	}()

	msgType, payload, err := sc.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msgType != MsgPing {
		t.Errorf("msgType = 0x%02x, want 0x%02x", msgType, MsgPing)
	}
	if payload != nil {
		t.Errorf("expected nil payload, got %d bytes", len(payload))
	}
}

func TestConnWriteReadJSON(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sc := WrapConn(server)
	cc := WrapConn(client)

	original := KeyEventMsg{Key: 42, Pressed: true}
	done := make(chan error, 1)
	go func() {
		done <- cc.WriteJSONMessage(MsgKeyEvent, original)
	}()

	var decoded KeyEventMsg
	msgType, err := sc.ReadJSONMessage(&decoded)
	if err != nil {
		t.Fatalf("ReadJSONMessage: %v", err)
	}
	if msgType != MsgKeyEvent {
		t.Errorf("msgType = 0x%02x, want 0x%02x", msgType, MsgKeyEvent)
	}
	if decoded.Key != original.Key || decoded.Pressed != original.Pressed {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestConnCloseIdempotent(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	cc := WrapConn(client)
	if err := cc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if !cc.IsClosed() {
		t.Error("expected IsClosed after Close")
	}
	// Second close should be no-op.
	if err := cc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestConnMultipleMessages(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sc := WrapConn(server)
	cc := WrapConn(client)

	const count = 50
	done := make(chan error, 1)
	go func() {
		for i := range count {
			payload := []byte{byte(i)}
			if err := cc.WriteMessage(0x11, payload); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	for i := range count {
		msgType, payload, err := sc.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage[%d]: %v", i, err)
		}
		if msgType != 0x11 {
			t.Errorf("msg[%d] type = 0x%02x", i, msgType)
		}
		if len(payload) != 1 || payload[0] != byte(i) {
			t.Errorf("msg[%d] payload = %v, want [%d]", i, payload, i)
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("writer error: %v", err)
	}
}
