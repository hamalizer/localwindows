//go:build darwin

package capture

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>

static int getMainDisplayWidth() {
    return (int)CGDisplayPixelsWide(CGMainDisplayID());
}

static int getMainDisplayHeight() {
    return (int)CGDisplayPixelsHigh(CGMainDisplayID());
}

// captureScreen captures the main display into a caller-provided RGBA buffer.
// Returns 0 on success, -1 on failure.
static int captureScreen(void *buf, int w, int h) {
    CGDirectDisplayID displayID = CGMainDisplayID();
    CGImageRef img = CGDisplayCreateImage(displayID);
    if (!img) return -1;

    CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
    CGContextRef ctx = CGBitmapContextCreate(
        buf, w, h, 8, w * 4, cs,
        kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big
    );
    CGColorSpaceRelease(cs);
    if (!ctx) {
        CGImageRelease(img);
        return -1;
    }

    CGContextDrawImage(ctx, CGRectMake(0, 0, w, h), img);
    CGContextRelease(ctx);
    CGImageRelease(img);
    return 0;
}
*/
import "C"

import (
	"fmt"
	"image"
	"unsafe"
)

type darwinCapturer struct {
	screenW, screenH int
}

func newPlatformCapturer() Capturer {
	return &darwinCapturer{}
}

func (c *darwinCapturer) Init() error {
	c.screenW = int(C.getMainDisplayWidth())
	c.screenH = int(C.getMainDisplayHeight())
	if c.screenW == 0 || c.screenH == 0 {
		return fmt.Errorf("failed to get screen dimensions")
	}
	return nil
}

func (c *darwinCapturer) ScreenSize() (int, int) {
	return c.screenW, c.screenH
}

func (c *darwinCapturer) Capture() (*image.RGBA, error) {
	pixels := make([]byte, c.screenW*c.screenH*4)
	rc := C.captureScreen(unsafe.Pointer(&pixels[0]), C.int(c.screenW), C.int(c.screenH))
	if rc != 0 {
		return nil, fmt.Errorf("CGDisplayCreateImage failed")
	}
	img := &image.RGBA{
		Pix:    pixels,
		Stride: c.screenW * 4,
		Rect:   image.Rect(0, 0, c.screenW, c.screenH),
	}
	return img, nil
}

func (c *darwinCapturer) Close() {}
