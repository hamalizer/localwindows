//go:build linux && !android

package input

/*
#cgo LDFLAGS: -lX11 -lXtst
#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>
#include <X11/keysym.h>
#include <stdlib.h>

static Display* inputOpenDisplay() {
    return XOpenDisplay(NULL);
}

static void inputMoveMouse(Display *dpy, int x, int y) {
    XTestFakeMotionEvent(dpy, -1, x, y, 0);
    XFlush(dpy);
}

static void inputMouseButton(Display *dpy, int button, int pressed) {
    XTestFakeButtonEvent(dpy, button, pressed, 0);
    XFlush(dpy);
}

static void inputKeyEvent(Display *dpy, unsigned int keysym, int pressed) {
    KeyCode kc = XKeysymToKeycode(dpy, keysym);
    if (kc != 0) {
        XTestFakeKeyEvent(dpy, kc, pressed, 0);
        XFlush(dpy);
    }
}
*/
import "C"

import "fmt"

// X11 keysym values.
var keyToXK = map[Key]uint32{
	KeyA: 0x0061, KeyB: 0x0062, KeyC: 0x0063, KeyD: 0x0064, KeyE: 0x0065,
	KeyF: 0x0066, KeyG: 0x0067, KeyH: 0x0068, KeyI: 0x0069, KeyJ: 0x006A,
	KeyK: 0x006B, KeyL: 0x006C, KeyM: 0x006D, KeyN: 0x006E, KeyO: 0x006F,
	KeyP: 0x0070, KeyQ: 0x0071, KeyR: 0x0072, KeyS: 0x0073, KeyT: 0x0074,
	KeyU: 0x0075, KeyV: 0x0076, KeyW: 0x0077, KeyX: 0x0078, KeyY: 0x0079, KeyZ: 0x007A,
	Key0: 0x0030, Key1: 0x0031, Key2: 0x0032, Key3: 0x0033, Key4: 0x0034,
	Key5: 0x0035, Key6: 0x0036, Key7: 0x0037, Key8: 0x0038, Key9: 0x0039,
	KeyF1: 0xFFBE, KeyF2: 0xFFBF, KeyF3: 0xFFC0, KeyF4: 0xFFC1,
	KeyF5: 0xFFC2, KeyF6: 0xFFC3, KeyF7: 0xFFC4, KeyF8: 0xFFC5,
	KeyF9: 0xFFC6, KeyF10: 0xFFC7, KeyF11: 0xFFC8, KeyF12: 0xFFC9,
	KeyReturn: 0xFF0D, KeyTab: 0xFF09, KeyBackspace: 0xFF08,
	KeyDelete: 0xFFFF, KeyEscape: 0xFF1B, KeySpace: 0x0020,
	KeyLeft: 0xFF51, KeyRight: 0xFF53, KeyUp: 0xFF52, KeyDown: 0xFF54,
	KeyHome: 0xFF50, KeyEnd: 0xFF57, KeyPageUp: 0xFF55, KeyPageDown: 0xFF56,
	KeyInsert: 0xFF63,
	KeyLeftShift: 0xFFE1, KeyRightShift: 0xFFE2,
	KeyLeftControl: 0xFFE3, KeyRightControl: 0xFFE4,
	KeyLeftAlt: 0xFFE9, KeyRightAlt: 0xFFEA,
	KeyLeftSuper: 0xFFEB, KeyRightSuper: 0xFFEC,
	KeyCapsLock: 0xFFE5, KeyNumLock: 0xFF7F, KeyScrollLock: 0xFF14,
	KeyPrintScreen: 0xFF61, KeyPause: 0xFF13,
	KeyMinus: 0x002D, KeyEqual: 0x003D,
	KeyLeftBracket: 0x005B, KeyRightBracket: 0x005D,
	KeyBackslash: 0x005C, KeySemicolon: 0x003B, KeyApostrophe: 0x0027,
	KeyGraveAccent: 0x0060, KeyComma: 0x002C, KeyPeriod: 0x002E, KeySlash: 0x002F,
	KeyNumpad0: 0xFFB0, KeyNumpad1: 0xFFB1, KeyNumpad2: 0xFFB2, KeyNumpad3: 0xFFB3,
	KeyNumpad4: 0xFFB4, KeyNumpad5: 0xFFB5, KeyNumpad6: 0xFFB6, KeyNumpad7: 0xFFB7,
	KeyNumpad8: 0xFFB8, KeyNumpad9: 0xFFB9,
	KeyNumpadAdd: 0xFFAB, KeyNumpadSubtract: 0xFFAD,
	KeyNumpadMultiply: 0xFFAA, KeyNumpadDivide: 0xFFAF,
	KeyNumpadDecimal: 0xFFAE, KeyNumpadEnter: 0xFF8D,
	KeyMenu: 0xFF67,
}

// X11 button mapping: left=1, middle=2, right=3, scrollUp=4, scrollDown=5.
var x11ButtonMap = map[uint8]int{
	0: 1, // left
	1: 3, // right
	2: 2, // middle
}

type linuxInjector struct {
	display *C.Display
}

func newPlatformInjector() Injector {
	return &linuxInjector{}
}

func (inj *linuxInjector) Init() error {
	inj.display = C.inputOpenDisplay()
	if inj.display == nil {
		return fmt.Errorf("cannot open X11 display for input")
	}
	return nil
}

func (inj *linuxInjector) MouseMove(x, y int) error {
	C.inputMoveMouse(inj.display, C.int(x), C.int(y))
	return nil
}

func (inj *linuxInjector) MouseButton(button uint8, pressed bool) error {
	xBtn, ok := x11ButtonMap[button]
	if !ok {
		return fmt.Errorf("unknown button: %d", button)
	}
	p := 0
	if pressed {
		p = 1
	}
	C.inputMouseButton(inj.display, C.int(xBtn), C.int(p))
	return nil
}

func (inj *linuxInjector) MouseScroll(dx, dy int) error {
	// X11 scroll: button 4=up, 5=down, 6=left, 7=right
	for range abs(dy) {
		btn := 5 // down
		if dy > 0 {
			btn = 4 // up
		}
		C.inputMouseButton(inj.display, C.int(btn), 1)
		C.inputMouseButton(inj.display, C.int(btn), 0)
	}
	for range abs(dx) {
		btn := 7 // right
		if dx > 0 {
			btn = 6 // left
		}
		C.inputMouseButton(inj.display, C.int(btn), 1)
		C.inputMouseButton(inj.display, C.int(btn), 0)
	}
	return nil
}

func (inj *linuxInjector) KeyPress(key Key, pressed bool) error {
	ks, ok := keyToXK[key]
	if !ok {
		return fmt.Errorf("unmapped key: %d", key)
	}
	p := 0
	if pressed {
		p = 1
	}
	C.inputKeyEvent(inj.display, C.uint(ks), C.int(p))
	return nil
}

func (inj *linuxInjector) SendSpecialCombo(combo uint8) error {
	switch combo {
	case 0: // Ctrl+Alt+Del
		inj.KeyPress(KeyLeftControl, true)
		inj.KeyPress(KeyLeftAlt, true)
		inj.KeyPress(KeyDelete, true)
		inj.KeyPress(KeyDelete, false)
		inj.KeyPress(KeyLeftAlt, false)
		inj.KeyPress(KeyLeftControl, false)
	case 1: // Alt+Tab
		inj.KeyPress(KeyLeftAlt, true)
		inj.KeyPress(KeyTab, true)
		inj.KeyPress(KeyTab, false)
		inj.KeyPress(KeyLeftAlt, false)
	case 2: // Alt+F4
		inj.KeyPress(KeyLeftAlt, true)
		inj.KeyPress(KeyF4, true)
		inj.KeyPress(KeyF4, false)
		inj.KeyPress(KeyLeftAlt, false)
	case 3: // Super (open launcher)
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyLeftSuper, false)
	case 4: // Super+L (lock screen)
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyL, true)
		inj.KeyPress(KeyL, false)
		inj.KeyPress(KeyLeftSuper, false)
	case 5: // Super+D (show desktop)
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyD, true)
		inj.KeyPress(KeyD, false)
		inj.KeyPress(KeyLeftSuper, false)
	default:
		return fmt.Errorf("unknown combo: %d", combo)
	}
	return nil
}

func (inj *linuxInjector) Close() {
	if inj.display != nil {
		C.XCloseDisplay(inj.display)
		inj.display = nil
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
