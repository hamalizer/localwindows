//go:build android || ios

package input

import "fmt"

type stubInjector struct{}

func newPlatformInjector() Injector {
	return &stubInjector{}
}

func (s *stubInjector) Init() error {
	return fmt.Errorf("input injection not supported on this platform (use viewer mode)")
}
func (s *stubInjector) MouseMove(x, y int) error           { return nil }
func (s *stubInjector) MouseButton(button uint8, p bool) error { return nil }
func (s *stubInjector) MouseScroll(dx, dy int) error        { return nil }
func (s *stubInjector) KeyPress(key Key, pressed bool) error { return nil }
func (s *stubInjector) SendSpecialCombo(combo uint8) error  { return nil }
func (s *stubInjector) Close()                              {}
