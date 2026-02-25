//go:build windows

package capture

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	gdi32                = syscall.NewLazyDLL("gdi32.dll")
	procGetDesktopWindow = user32.NewProc("GetDesktopWindow")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procCreateCompatDC   = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procBitBlt           = gdi32.NewProc("BitBlt")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procDeleteDC         = gdi32.NewProc("DeleteDC")
	procGetDIBits        = gdi32.NewProc("GetDIBits")
)

const (
	smCxScreen = 0
	smCyScreen = 1
	srccopy    = 0x00CC0020
	biRGBVal   = 0
)

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
}

type windowsCapturer struct {
	screenW, screenH int
}

func newPlatformCapturer() Capturer {
	return &windowsCapturer{}
}

func (c *windowsCapturer) Init() error {
	w, _, _ := procGetSystemMetrics.Call(smCxScreen)
	h, _, _ := procGetSystemMetrics.Call(smCyScreen)
	c.screenW = int(w)
	c.screenH = int(h)
	if c.screenW == 0 || c.screenH == 0 {
		return fmt.Errorf("failed to get screen dimensions")
	}
	return nil
}

func (c *windowsCapturer) ScreenSize() (int, int) {
	return c.screenW, c.screenH
}

func (c *windowsCapturer) Capture() (*image.RGBA, error) {
	hwnd, _, _ := procGetDesktopWindow.Call()
	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}
	defer procReleaseDC.Call(hwnd, hdc)

	memDC, _, _ := procCreateCompatDC.Call(hdc)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	bmp, _, _ := procCreateCompatBmp.Call(hdc, uintptr(c.screenW), uintptr(c.screenH))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bmp)

	procSelectObject.Call(memDC, bmp)
	procBitBlt.Call(memDC, 0, 0, uintptr(c.screenW), uintptr(c.screenH), hdc, 0, 0, srccopy)

	// Read pixel data via GetDIBits.
	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(c.screenW),
			BiHeight:      -int32(c.screenH), // top-down
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRGBVal,
		},
	}

	pixelBytes := c.screenW * c.screenH * 4
	pixels := make([]byte, pixelBytes)
	ret, _, _ := procGetDIBits.Call(memDC, bmp, 0, uintptr(c.screenH),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bi)), 0)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}

	// Convert BGRA → RGBA in-place.
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+2] = pixels[i+2], pixels[i]
	}

	img := &image.RGBA{
		Pix:    pixels,
		Stride: c.screenW * 4,
		Rect:   image.Rect(0, 0, c.screenW, c.screenH),
	}
	return img, nil
}

func (c *windowsCapturer) Close() {}
