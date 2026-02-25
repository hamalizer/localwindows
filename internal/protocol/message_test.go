package protocol

import (
	"bytes"
	"image"
	"testing"
)

func TestEncodeDecodeFullFrame(t *testing.T) {
	frameID := uint32(42)
	width, height := 1920, 1080
	quality := uint8(75)
	jpegData := []byte("fake-jpeg-data-for-testing")

	payload := EncodeFullFrame(frameID, width, height, quality, jpegData)

	gotID, gotW, gotH, gotQ, gotData, err := DecodeFullFrame(payload)
	if err != nil {
		t.Fatalf("DecodeFullFrame: %v", err)
	}
	if gotID != frameID {
		t.Errorf("frameID = %d, want %d", gotID, frameID)
	}
	if gotW != width || gotH != height {
		t.Errorf("dimensions = %dx%d, want %dx%d", gotW, gotH, width, height)
	}
	if gotQ != quality {
		t.Errorf("quality = %d, want %d", gotQ, quality)
	}
	if !bytes.Equal(gotData, jpegData) {
		t.Errorf("jpegData mismatch")
	}
}

func TestDecodeFullFrameTooShort(t *testing.T) {
	_, _, _, _, _, err := DecodeFullFrame([]byte{0, 1, 2})
	if err == nil {
		t.Fatal("expected error for short payload")
	}
}

func TestEncodeDecodeDeltaFrame(t *testing.T) {
	frameID := uint32(100)
	tiles := []Tile{
		{X: 0, Y: 0, W: 32, H: 32, Data: []byte("tile0")},
		{X: 32, Y: 0, W: 32, H: 32, Data: []byte("tile1")},
		{X: 0, Y: 32, W: 32, H: 32, Data: []byte("tile2")},
	}

	payload := EncodeDeltaFrame(frameID, tiles)

	gotID, gotTiles, err := DecodeDeltaFrame(payload)
	if err != nil {
		t.Fatalf("DecodeDeltaFrame: %v", err)
	}
	if gotID != frameID {
		t.Errorf("frameID = %d, want %d", gotID, frameID)
	}
	if len(gotTiles) != len(tiles) {
		t.Fatalf("tile count = %d, want %d", len(gotTiles), len(tiles))
	}
	for i, tile := range gotTiles {
		if tile.X != tiles[i].X || tile.Y != tiles[i].Y {
			t.Errorf("tile[%d] pos = (%d,%d), want (%d,%d)", i, tile.X, tile.Y, tiles[i].X, tiles[i].Y)
		}
		if tile.W != tiles[i].W || tile.H != tiles[i].H {
			t.Errorf("tile[%d] size = %dx%d, want %dx%d", i, tile.W, tile.H, tiles[i].W, tiles[i].H)
		}
		if !bytes.Equal(tile.Data, tiles[i].Data) {
			t.Errorf("tile[%d] data mismatch", i)
		}
	}
}

func TestDecodeDeltaFrameEmpty(t *testing.T) {
	frameID := uint32(1)
	payload := EncodeDeltaFrame(frameID, nil)
	gotID, gotTiles, err := DecodeDeltaFrame(payload)
	if err != nil {
		t.Fatalf("DecodeDeltaFrame empty: %v", err)
	}
	if gotID != frameID {
		t.Errorf("frameID = %d, want %d", gotID, frameID)
	}
	if len(gotTiles) != 0 {
		t.Errorf("expected 0 tiles, got %d", len(gotTiles))
	}
}

func TestDecodeDeltaFrameTooShort(t *testing.T) {
	_, _, err := DecodeDeltaFrame([]byte{0, 1})
	if err == nil {
		t.Fatal("expected error for short payload")
	}
}

func TestEncodeDecodeFileChunk(t *testing.T) {
	transferID := "abc123def456"
	seqNum := uint32(7)
	data := []byte("chunk-data-here")

	payload := EncodeFileChunk(transferID, seqNum, data)

	gotID, gotSeq, gotData, err := DecodeFileChunk(payload)
	if err != nil {
		t.Fatalf("DecodeFileChunk: %v", err)
	}
	if gotID != transferID {
		t.Errorf("transferID = %q, want %q", gotID, transferID)
	}
	if gotSeq != seqNum {
		t.Errorf("seqNum = %d, want %d", gotSeq, seqNum)
	}
	if !bytes.Equal(gotData, data) {
		t.Errorf("data mismatch")
	}
}

func TestDecodeFileChunkTooShort(t *testing.T) {
	_, _, _, err := DecodeFileChunk([]byte{0})
	if err == nil {
		t.Fatal("expected error for short payload")
	}
}

func TestEncodeDecodeJSON(t *testing.T) {
	original := MouseMoveMsg{X: 0.5, Y: 0.75}
	data, err := EncodeJSON(original)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	var decoded MouseMoveMsg
	if err := DecodeJSON(data, &decoded); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if decoded.X != original.X || decoded.Y != original.Y {
		t.Errorf("got (%f, %f), want (%f, %f)", decoded.X, decoded.Y, original.X, original.Y)
	}
}

func TestImageBounds(t *testing.T) {
	w, h := ImageBounds(image.Rect(0, 0, 1920, 1080))
	if w != 1920 || h != 1080 {
		t.Errorf("ImageBounds = %dx%d, want 1920x1080", w, h)
	}
}
