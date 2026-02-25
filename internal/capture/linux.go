//go:build linux && !android

package capture

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <stdlib.h>
#include <string.h>

static Display* openDisplay() {
    return XOpenDisplay(NULL);
}

static void getScreenSize(Display *dpy, int *w, int *h) {
    Screen *scr = DefaultScreenOfDisplay(dpy);
    *w = scr->width;
    *h = scr->height;
}

// captureX11 captures the root window into an RGBA buffer.
// Returns 0 on success, -1 on failure.
static int captureX11(Display *dpy, void *buf, int w, int h) {
    Window root = DefaultRootWindow(dpy);
    XImage *img = XGetImage(dpy, root, 0, 0, w, h, AllPlanes, ZPixmap);
    if (!img) return -1;

    unsigned char *dst = (unsigned char*)buf;
    for (int y = 0; y < h; y++) {
        for (int x = 0; x < w; x++) {
            unsigned long pixel = XGetPixel(img, x, y);
            int idx = (y * w + x) * 4;
            dst[idx+0] = (pixel >> 16) & 0xFF; // R
            dst[idx+1] = (pixel >> 8)  & 0xFF; // G
            dst[idx+2] = pixel         & 0xFF; // B
            dst[idx+3] = 0xFF;                 // A
        }
    }

    XDestroyImage(img);
    return 0;
}
*/
import "C"

import (
	"fmt"
	"image"
	"unsafe"
)

type linuxCapturer struct {
	display          *C.Display
	screenW, screenH int
}

func newPlatformCapturer() Capturer {
	return &linuxCapturer{}
}

func (c *linuxCapturer) Init() error {
	c.display = C.openDisplay()
	if c.display == nil {
		return fmt.Errorf("cannot open X11 display (is DISPLAY set?)")
	}
	var w, h C.int
	C.getScreenSize(c.display, &w, &h)
	c.screenW = int(w)
	c.screenH = int(h)
	if c.screenW == 0 || c.screenH == 0 {
		return fmt.Errorf("failed to get screen dimensions")
	}
	return nil
}

func (c *linuxCapturer) ScreenSize() (int, int) {
	return c.screenW, c.screenH
}

func (c *linuxCapturer) Capture() (*image.RGBA, error) {
	pixels := make([]byte, c.screenW*c.screenH*4)
	rc := C.captureX11(c.display, unsafe.Pointer(&pixels[0]), C.int(c.screenW), C.int(c.screenH))
	if rc != 0 {
		return nil, fmt.Errorf("XGetImage failed")
	}
	img := &image.RGBA{
		Pix:    pixels,
		Stride: c.screenW * 4,
		Rect:   image.Rect(0, 0, c.screenW, c.screenH),
	}
	return img, nil
}

func (c *linuxCapturer) Close() {
	if c.display != nil {
		C.XCloseDisplay(c.display)
		c.display = nil
	}
}
