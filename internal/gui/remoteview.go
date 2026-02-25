package gui

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"localwindows/internal/client"
	"localwindows/internal/input"
)

const mouseMoveMinInterval = 8 * time.Millisecond // ~120 Hz max mouse rate

// interactiveScreen wraps a screen image with full mouse and keyboard handling.
type interactiveScreen struct {
	widget.BaseWidget
	img     *canvas.Image
	client  *client.Client
	remoteW int
	remoteH int
	focused bool
	// hasKeyable tracks whether KeyDown has been called at least once,
	// indicating desktop.Keyable is active. When true, TypedKey/TypedRune
	// are suppressed to avoid double-firing key events.
	hasKeyable    bool
	lastMouseSend time.Time // throttle mouse moves
	mu            sync.Mutex
}

func newInteractiveScreen(img *canvas.Image, cl *client.Client, rw, rh int) *interactiveScreen {
	s := &interactiveScreen{
		img:     img,
		client:  cl,
		remoteW: rw,
		remoteH: rh,
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *interactiveScreen) CreateRenderer() fyne.WidgetRenderer {
	return &interactiveScreenRenderer{screen: s}
}

// normalize converts a widget-local position to 0.0–1.0 normalized coordinates,
// accounting for aspect-ratio fitting.
func (s *interactiveScreen) normalize(pos fyne.Position) (float64, float64) {
	size := s.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return 0, 0
	}
	aspect := float32(s.remoteW) / float32(s.remoteH)
	var drawW, drawH float32
	if size.Width/size.Height > aspect {
		drawH = size.Height
		drawW = drawH * aspect
	} else {
		drawW = size.Width
		drawH = drawW / aspect
	}
	offX := (size.Width - drawW) / 2
	offY := (size.Height - drawH) / 2

	nx := float64((pos.X - offX) / drawW)
	ny := float64((pos.Y - offY) / drawH)
	return clamp01(nx), clamp01(ny)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// --- Interface compliance ---

var (
	_ fyne.Tappable          = (*interactiveScreen)(nil)
	_ fyne.SecondaryTappable = (*interactiveScreen)(nil)
	_ fyne.Draggable         = (*interactiveScreen)(nil)
	_ fyne.Focusable         = (*interactiveScreen)(nil)
	_ desktop.Hoverable      = (*interactiveScreen)(nil)
	_ desktop.Keyable        = (*interactiveScreen)(nil)
)

// --- Focus ---

func (s *interactiveScreen) FocusGained() {
	s.mu.Lock()
	s.focused = true
	s.mu.Unlock()
}

func (s *interactiveScreen) FocusLost() {
	s.mu.Lock()
	s.focused = false
	s.mu.Unlock()
}

func (s *interactiveScreen) TypedRune(r rune) {
	// On desktop, KeyDown/KeyUp handles all keys — skip to avoid double-fire.
	s.mu.Lock()
	skip := s.hasKeyable
	s.mu.Unlock()
	if skip {
		return
	}
	if k, ok := runeToKey(r); ok {
		s.client.SendKeyEvent(uint16(k), true)
		s.client.SendKeyEvent(uint16(k), false)
	}
}

func (s *interactiveScreen) TypedKey(ev *fyne.KeyEvent) {
	// On desktop, KeyDown/KeyUp handles all keys — skip to avoid double-fire.
	s.mu.Lock()
	skip := s.hasKeyable
	s.mu.Unlock()
	if skip {
		return
	}
	// Fallback for platforms without desktop.Keyable: send press+release.
	if k, ok := fyneKeyMap[ev.Name]; ok {
		s.client.SendKeyEvent(uint16(k), true)
		s.client.SendKeyEvent(uint16(k), false)
	}
}

// --- desktop.Keyable: separate key down/up for desktop platforms ---

func (s *interactiveScreen) KeyDown(ev *fyne.KeyEvent) {
	s.mu.Lock()
	s.hasKeyable = true
	s.mu.Unlock()
	if k, ok := fyneKeyMap[ev.Name]; ok {
		s.client.SendKeyEvent(uint16(k), true)
	}
}

func (s *interactiveScreen) KeyUp(ev *fyne.KeyEvent) {
	if k, ok := fyneKeyMap[ev.Name]; ok {
		s.client.SendKeyEvent(uint16(k), false)
	}
}

// --- Mouse: Tap ---

func (s *interactiveScreen) Tapped(ev *fyne.PointEvent) {
	// Request focus on click.
	c := fyne.CurrentApp().Driver().CanvasForObject(s)
	if c != nil {
		c.Focus(s)
	}
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseButton(nx, ny, 0, true)
	s.client.SendMouseButton(nx, ny, 0, false)
}

func (s *interactiveScreen) TappedSecondary(ev *fyne.PointEvent) {
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseButton(nx, ny, 1, true)
	s.client.SendMouseButton(nx, ny, 1, false)
}

// --- Mouse: Hover ---

func (s *interactiveScreen) MouseIn(ev *desktop.MouseEvent) {}
func (s *interactiveScreen) MouseOut()                      {}

func (s *interactiveScreen) MouseMoved(ev *desktop.MouseEvent) {
	now := time.Now()
	s.mu.Lock()
	if now.Sub(s.lastMouseSend) < mouseMoveMinInterval {
		s.mu.Unlock()
		return
	}
	s.lastMouseSend = now
	s.mu.Unlock()
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseMove(nx, ny)
}

// --- Mouse: Drag ---

func (s *interactiveScreen) Dragged(ev *fyne.DragEvent) {
	now := time.Now()
	s.mu.Lock()
	if now.Sub(s.lastMouseSend) < mouseMoveMinInterval {
		s.mu.Unlock()
		return
	}
	s.lastMouseSend = now
	s.mu.Unlock()
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseMove(nx, ny)
}

func (s *interactiveScreen) DragEnd() {}

// --- Mouse: Scroll ---

func (s *interactiveScreen) Scrolled(ev *fyne.ScrollEvent) {
	var dy, dx float64
	if ev.Scrolled.DY > 0 {
		dy = 1
	} else if ev.Scrolled.DY < 0 {
		dy = -1
	}
	if ev.Scrolled.DX > 0 {
		dx = 1
	} else if ev.Scrolled.DX < 0 {
		dx = -1
	}
	s.client.SendMouseScroll(0.5, 0.5, dx, dy)
}

// --- Rune to Key mapping for TypedRune ---

func runeToKey(r rune) (input.Key, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return input.KeyA + input.Key(r-'a'), true
	case r >= 'A' && r <= 'Z':
		return input.KeyA + input.Key(r-'A'), true
	case r >= '0' && r <= '9':
		return input.Key0 + input.Key(r-'0'), true
	case r == ' ':
		return input.KeySpace, true
	case r == '-' || r == '_':
		return input.KeyMinus, true
	case r == '=' || r == '+':
		return input.KeyEqual, true
	case r == '[' || r == '{':
		return input.KeyLeftBracket, true
	case r == ']' || r == '}':
		return input.KeyRightBracket, true
	case r == '\\' || r == '|':
		return input.KeyBackslash, true
	case r == ';' || r == ':':
		return input.KeySemicolon, true
	case r == '\'' || r == '"':
		return input.KeyApostrophe, true
	case r == '`' || r == '~':
		return input.KeyGraveAccent, true
	case r == ',' || r == '<':
		return input.KeyComma, true
	case r == '.' || r == '>':
		return input.KeyPeriod, true
	case r == '/' || r == '?':
		return input.KeySlash, true
	}
	return 0, false
}

// --- Renderer ---

type interactiveScreenRenderer struct {
	screen *interactiveScreen
}

func (r *interactiveScreenRenderer) Layout(size fyne.Size) {
	r.screen.img.Resize(size)
	r.screen.img.Move(fyne.NewPos(0, 0))
}

func (r *interactiveScreenRenderer) MinSize() fyne.Size {
	return fyne.NewSize(320, 240)
}

func (r *interactiveScreenRenderer) Refresh() {
	canvas.Refresh(r.screen.img)
}

func (r *interactiveScreenRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.screen.img}
}

func (r *interactiveScreenRenderer) Destroy() {}
