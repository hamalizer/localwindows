package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"localwindows/internal/client"
)

// interactiveScreen wraps a screen image with mouse event handling.
type interactiveScreen struct {
	widget.BaseWidget
	img      *canvas.Image
	client   *client.Client
	remoteW  int
	remoteH  int
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

// Normalize a position within this widget to 0.0-1.0 coordinates relative to
// the remote screen, accounting for the aspect-ratio fit.
func (s *interactiveScreen) normalize(pos fyne.Position) (float64, float64) {
	size := s.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return 0, 0
	}

	// Compute the actual drawn area (aspect-ratio fitted).
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

	// Clamp.
	if nx < 0 {
		nx = 0
	}
	if nx > 1 {
		nx = 1
	}
	if ny < 0 {
		ny = 0
	}
	if ny > 1 {
		ny = 1
	}
	return nx, ny
}

// --- Mouse events via desktop interfaces ---

var _ fyne.Tappable = (*interactiveScreen)(nil)
var _ fyne.SecondaryTappable = (*interactiveScreen)(nil)
var _ desktop.Hoverable = (*interactiveScreen)(nil)
var _ fyne.Draggable = (*interactiveScreen)(nil)

func (s *interactiveScreen) Tapped(ev *fyne.PointEvent) {
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseButton(nx, ny, 0, true)
	s.client.SendMouseButton(nx, ny, 0, false)
}

func (s *interactiveScreen) TappedSecondary(ev *fyne.PointEvent) {
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseButton(nx, ny, 1, true)
	s.client.SendMouseButton(nx, ny, 1, false)
}

func (s *interactiveScreen) MouseIn(ev *desktop.MouseEvent) {}

func (s *interactiveScreen) MouseOut() {}

func (s *interactiveScreen) MouseMoved(ev *desktop.MouseEvent) {
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseMove(nx, ny)
}

func (s *interactiveScreen) Dragged(ev *fyne.DragEvent) {
	nx, ny := s.normalize(ev.Position)
	s.client.SendMouseMove(nx, ny)
}

func (s *interactiveScreen) DragEnd() {}

func (s *interactiveScreen) Scrolled(ev *fyne.ScrollEvent) {
	dy := 0.0
	dx := 0.0
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
