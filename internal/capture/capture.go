package capture

import "image"

// Capturer provides screen capture capabilities.
type Capturer interface {
	// Init initializes the capture system. Must be called before Capture.
	Init() error

	// Capture takes a screenshot of the primary display and returns it as RGBA.
	Capture() (*image.RGBA, error)

	// ScreenSize returns the primary screen dimensions.
	ScreenSize() (width, height int)

	// Close releases capture resources.
	Close()
}

// New returns a platform-appropriate Capturer.
// Implemented in platform-specific files.
func New() Capturer {
	return newPlatformCapturer()
}
