package protocol

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	maxMessageSize = 16 * 1024 * 1024 // 16 MB max message
	headerSize     = 5                 // 1 byte type + 4 bytes length
)

// Conn wraps a net.Conn with framed message reading and writing.
// Wire format per message: [1:type][4:payloadLen BE][payload]
type Conn struct {
	raw    net.Conn
	reader *bufio.Reader
	writeMu sync.Mutex
	closed  bool
	closeMu sync.Mutex
}

// WrapConn wraps a net.Conn for framed protocol I/O.
func WrapConn(c net.Conn) *Conn {
	return &Conn{
		raw:    c,
		reader: bufio.NewReaderSize(c, 256*1024),
	}
}

// ReadMessage reads a single framed message. Returns message type and payload.
func (c *Conn) ReadMessage() (msgType uint8, payload []byte, err error) {
	header := make([]byte, headerSize)
	if _, err = io.ReadFull(c.reader, header); err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}
	msgType = header[0]
	payloadLen := binary.BigEndian.Uint32(header[1:5])
	if payloadLen > maxMessageSize {
		return 0, nil, fmt.Errorf("message too large: %d bytes", payloadLen)
	}
	if payloadLen == 0 {
		return msgType, nil, nil
	}
	payload = make([]byte, payloadLen)
	if _, err = io.ReadFull(c.reader, payload); err != nil {
		return 0, nil, fmt.Errorf("read payload: %w", err)
	}
	return msgType, payload, nil
}

// WriteMessage writes a framed message. Thread-safe.
func (c *Conn) WriteMessage(msgType uint8, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	header := make([]byte, headerSize)
	header[0] = msgType
	binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))

	if _, err := c.raw.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := c.raw.Write(payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}
	return nil
}

// WriteJSONMessage encodes a value as JSON and writes it as a framed message.
func (c *Conn) WriteJSONMessage(msgType uint8, v any) error {
	data, err := EncodeJSON(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return c.WriteMessage(msgType, data)
}

// ReadJSONMessage reads a message and decodes the JSON payload into v.
// Returns the message type for verification.
func (c *Conn) ReadJSONMessage(v any) (uint8, error) {
	msgType, payload, err := c.ReadMessage()
	if err != nil {
		return 0, err
	}
	if err := DecodeJSON(payload, v); err != nil {
		return msgType, fmt.Errorf("unmarshal type 0x%02x: %w", msgType, err)
	}
	return msgType, nil
}

// SetReadDeadline sets the read deadline on the underlying connection.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.raw.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the underlying connection.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.raw.SetWriteDeadline(t)
}

// RemoteAddr returns the remote address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.raw.RemoteAddr()
}

// LocalAddr returns the local address.
func (c *Conn) LocalAddr() net.Addr {
	return c.raw.LocalAddr()
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.raw.Close()
}

// IsClosed reports whether the connection has been closed.
func (c *Conn) IsClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}
