//go:build android || ios

package capture

import (
	"fmt"
	"image"
)

type stubCapturer struct{}

func newPlatformCapturer() Capturer {
	return &stubCapturer{}
}

func (c *stubCapturer) Init() error {
	return fmt.Errorf("screen capture is not supported on this platform (use viewer mode)")
}

func (c *stubCapturer) ScreenSize() (int, int) {
	return 0, 0
}

func (c *stubCapturer) Capture() (*image.RGBA, error) {
	return nil, fmt.Errorf("screen capture not supported on this platform")
}

func (c *stubCapturer) Close() {}
