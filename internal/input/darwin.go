//go:build darwin

package input

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>

static void moveMouse(int x, int y) {
    CGEventRef ev = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved,
        CGPointMake(x, y), kCGMouseButtonLeft);
    CGEventPost(kCGHIDEventTap, ev);
    CFRelease(ev);
}

static void mouseButton(int x, int y, int button, int pressed) {
    CGEventType evType;
    CGMouseButton btn;
    switch (button) {
        case 0: btn = kCGMouseButtonLeft;
            evType = pressed ? kCGEventLeftMouseDown : kCGEventLeftMouseUp; break;
        case 1: btn = kCGMouseButtonRight;
            evType = pressed ? kCGEventRightMouseDown : kCGEventRightMouseUp; break;
        default: btn = kCGMouseButtonCenter;
            evType = pressed ? kCGEventOtherMouseDown : kCGEventOtherMouseUp; break;
    }
    CGEventRef ev = CGEventCreateMouseEvent(NULL, evType, CGPointMake(x, y), btn);
    CGEventPost(kCGHIDEventTap, ev);
    CFRelease(ev);
}

static void mouseScroll(int dx, int dy) {
    CGEventRef ev = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, dy, dx);
    CGEventPost(kCGHIDEventTap, ev);
    CFRelease(ev);
}

static void keyEvent(int keyCode, int pressed) {
    CGEventRef ev = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, pressed);
    CGEventPost(kCGHIDEventTap, ev);
    CFRelease(ev);
}

static void keyEventWithFlags(int keyCode, int pressed, int flags) {
    CGEventRef ev = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, pressed);
    CGEventSetFlags(ev, (CGEventFlags)flags);
    CGEventPost(kCGHIDEventTap, ev);
    CFRelease(ev);
}
*/
import "C"

import "fmt"

// macOS CGKeyCode values.
var keyToMac = map[Key]int{
	KeyA: 0x00, KeyB: 0x0B, KeyC: 0x08, KeyD: 0x02, KeyE: 0x0E,
	KeyF: 0x03, KeyG: 0x05, KeyH: 0x04, KeyI: 0x22, KeyJ: 0x26,
	KeyK: 0x28, KeyL: 0x25, KeyM: 0x2E, KeyN: 0x2D, KeyO: 0x1F,
	KeyP: 0x23, KeyQ: 0x0C, KeyR: 0x0F, KeyS: 0x01, KeyT: 0x11,
	KeyU: 0x20, KeyV: 0x09, KeyW: 0x0D, KeyX: 0x07, KeyY: 0x10, KeyZ: 0x06,
	Key0: 0x1D, Key1: 0x12, Key2: 0x13, Key3: 0x14, Key4: 0x15,
	Key5: 0x17, Key6: 0x16, Key7: 0x1A, Key8: 0x1C, Key9: 0x19,
	KeyF1: 0x7A, KeyF2: 0x78, KeyF3: 0x63, KeyF4: 0x76,
	KeyF5: 0x60, KeyF6: 0x61, KeyF7: 0x62, KeyF8: 0x64,
	KeyF9: 0x65, KeyF10: 0x6D, KeyF11: 0x67, KeyF12: 0x6F,
	KeyReturn: 0x24, KeyTab: 0x30, KeyBackspace: 0x33,
	KeyDelete: 0x75, KeyEscape: 0x35, KeySpace: 0x31,
	KeyLeft: 0x7B, KeyRight: 0x7C, KeyUp: 0x7E, KeyDown: 0x7D,
	KeyHome: 0x73, KeyEnd: 0x77, KeyPageUp: 0x74, KeyPageDown: 0x79,
	KeyLeftShift: 0x38, KeyRightShift: 0x3C,
	KeyLeftControl: 0x3B, KeyRightControl: 0x3E,
	KeyLeftAlt: 0x3A, KeyRightAlt: 0x3D,
	KeyLeftSuper: 0x37, KeyRightSuper: 0x36,
	KeyCapsLock: 0x39,
	KeyMinus: 0x1B, KeyEqual: 0x18,
	KeyLeftBracket: 0x21, KeyRightBracket: 0x1E,
	KeyBackslash: 0x2A, KeySemicolon: 0x29, KeyApostrophe: 0x27,
	KeyGraveAccent: 0x32, KeyComma: 0x2B, KeyPeriod: 0x2F, KeySlash: 0x2C,
	KeyNumpad0: 0x52, KeyNumpad1: 0x53, KeyNumpad2: 0x54, KeyNumpad3: 0x55,
	KeyNumpad4: 0x56, KeyNumpad5: 0x57, KeyNumpad6: 0x58, KeyNumpad7: 0x59,
	KeyNumpad8: 0x5B, KeyNumpad9: 0x5C,
	KeyNumpadAdd: 0x45, KeyNumpadSubtract: 0x4E,
	KeyNumpadMultiply: 0x43, KeyNumpadDivide: 0x4B,
	KeyNumpadDecimal: 0x41, KeyNumpadEnter: 0x4C,
}

type darwinInjector struct {
	lastX, lastY int
}

func newPlatformInjector() Injector {
	return &darwinInjector{}
}

func (inj *darwinInjector) Init() error {
	return nil
}

func (inj *darwinInjector) MouseMove(x, y int) error {
	inj.lastX = x
	inj.lastY = y
	C.moveMouse(C.int(x), C.int(y))
	return nil
}

func (inj *darwinInjector) MouseButton(button uint8, pressed bool) error {
	p := 0
	if pressed {
		p = 1
	}
	C.mouseButton(C.int(inj.lastX), C.int(inj.lastY), C.int(button), C.int(p))
	return nil
}

func (inj *darwinInjector) MouseScroll(dx, dy int) error {
	C.mouseScroll(C.int(dx), C.int(dy))
	return nil
}

func (inj *darwinInjector) KeyPress(key Key, pressed bool) error {
	code, ok := keyToMac[key]
	if !ok {
		return fmt.Errorf("unmapped key: %d", key)
	}
	p := 0
	if pressed {
		p = 1
	}
	C.keyEvent(C.int(code), C.int(p))
	return nil
}

func (inj *darwinInjector) SendSpecialCombo(combo uint8) error {
	switch combo {
	case 0: // Ctrl+Alt+Del → on macOS, send Ctrl+Option+Delete
		inj.KeyPress(KeyLeftControl, true)
		inj.KeyPress(KeyLeftAlt, true)
		inj.KeyPress(KeyDelete, true)
		inj.KeyPress(KeyDelete, false)
		inj.KeyPress(KeyLeftAlt, false)
		inj.KeyPress(KeyLeftControl, false)
	case 1: // Cmd+Tab
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyTab, true)
		inj.KeyPress(KeyTab, false)
		inj.KeyPress(KeyLeftSuper, false)
	case 2: // Cmd+Q
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyQ, true)
		inj.KeyPress(KeyQ, false)
		inj.KeyPress(KeyLeftSuper, false)
	case 3: // Ctrl+Eject / Ctrl+Power → not easily simulated, send Cmd+Space
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeySpace, true)
		inj.KeyPress(KeySpace, false)
		inj.KeyPress(KeyLeftSuper, false)
	default:
		return fmt.Errorf("unknown combo: %d", combo)
	}
	return nil
}

func (inj *darwinInjector) Close() {}
