//go:build windows

package input

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mousefAbsolute  = 0x8000
	mousefMove      = 0x0001
	mousefLeftDown  = 0x0002
	mousefLeftUp    = 0x0004
	mousefRightDown = 0x0008
	mousefRightUp   = 0x0010
	mousefMiddleDown = 0x0020
	mousefMiddleUp  = 0x0040
	mousefWheel     = 0x0800
	mousefHWheel    = 0x01000

	keyfExtendedKey = 0x0001
	keyfKeyUp       = 0x0002

	wheelDelta = 120

	// Windows INPUT struct layout on AMD64:
	//   offset 0:  DWORD  type        (4 bytes)
	//   offset 4:  padding             (4 bytes, alignment for union)
	//   offset 8:  union data          (32 bytes, sizeof MOUSEINPUT on x64)
	//   total: 40 bytes
	//
	// MOUSEINPUT on AMD64 (32 bytes at offset 8):
	//   offset  8: LONG      dx          (4 bytes)
	//   offset 12: LONG      dy          (4 bytes)
	//   offset 16: DWORD     mouseData   (4 bytes)
	//   offset 20: DWORD     dwFlags     (4 bytes)
	//   offset 24: DWORD     time        (4 bytes)
	//   offset 28: padding               (4 bytes)
	//   offset 32: ULONG_PTR dwExtraInfo (8 bytes)
	//
	// KEYBDINPUT on AMD64 (24 bytes at offset 8):
	//   offset  8: WORD      wVk         (2 bytes)
	//   offset 10: WORD      wScan       (2 bytes)
	//   offset 12: DWORD     dwFlags     (4 bytes)
	//   offset 16: DWORD     time        (4 bytes)
	//   offset 20: padding               (4 bytes)
	//   offset 24: ULONG_PTR dwExtraInfo (8 bytes)
	inputSize = 40 // sizeof(INPUT) on Windows AMD64
)

var keyToVK = map[Key]uint16{
	KeyA: 0x41, KeyB: 0x42, KeyC: 0x43, KeyD: 0x44, KeyE: 0x45,
	KeyF: 0x46, KeyG: 0x47, KeyH: 0x48, KeyI: 0x49, KeyJ: 0x4A,
	KeyK: 0x4B, KeyL: 0x4C, KeyM: 0x4D, KeyN: 0x4E, KeyO: 0x4F,
	KeyP: 0x50, KeyQ: 0x51, KeyR: 0x52, KeyS: 0x53, KeyT: 0x54,
	KeyU: 0x55, KeyV: 0x56, KeyW: 0x57, KeyX: 0x58, KeyY: 0x59, KeyZ: 0x5A,
	Key0: 0x30, Key1: 0x31, Key2: 0x32, Key3: 0x33, Key4: 0x34,
	Key5: 0x35, Key6: 0x36, Key7: 0x37, Key8: 0x38, Key9: 0x39,
	KeyF1: 0x70, KeyF2: 0x71, KeyF3: 0x72, KeyF4: 0x73,
	KeyF5: 0x74, KeyF6: 0x75, KeyF7: 0x76, KeyF8: 0x77,
	KeyF9: 0x78, KeyF10: 0x79, KeyF11: 0x7A, KeyF12: 0x7B,
	KeyReturn: 0x0D, KeyTab: 0x09, KeyBackspace: 0x08,
	KeyDelete: 0x2E, KeyEscape: 0x1B, KeySpace: 0x20,
	KeyLeft: 0x25, KeyRight: 0x27, KeyUp: 0x26, KeyDown: 0x28,
	KeyHome: 0x24, KeyEnd: 0x23, KeyPageUp: 0x21, KeyPageDown: 0x22,
	KeyInsert: 0x2D,
	KeyLeftShift: 0xA0, KeyRightShift: 0xA1,
	KeyLeftControl: 0xA2, KeyRightControl: 0xA3,
	KeyLeftAlt: 0xA4, KeyRightAlt: 0xA5,
	KeyLeftSuper: 0x5B, KeyRightSuper: 0x5C,
	KeyCapsLock: 0x14, KeyNumLock: 0x90, KeyScrollLock: 0x91,
	KeyPrintScreen: 0x2C, KeyPause: 0x13,
	KeyMinus: 0xBD, KeyEqual: 0xBB,
	KeyLeftBracket: 0xDB, KeyRightBracket: 0xDD,
	KeyBackslash: 0xDC, KeySemicolon: 0xBA, KeyApostrophe: 0xDE,
	KeyGraveAccent: 0xC0, KeyComma: 0xBC, KeyPeriod: 0xBE, KeySlash: 0xBF,
	KeyNumpad0: 0x60, KeyNumpad1: 0x61, KeyNumpad2: 0x62, KeyNumpad3: 0x63,
	KeyNumpad4: 0x64, KeyNumpad5: 0x65, KeyNumpad6: 0x66, KeyNumpad7: 0x67,
	KeyNumpad8: 0x68, KeyNumpad9: 0x69,
	KeyNumpadAdd: 0x6B, KeyNumpadSubtract: 0x6D,
	KeyNumpadMultiply: 0x6A, KeyNumpadDivide: 0x6F,
	KeyNumpadDecimal: 0x6E, KeyNumpadEnter: 0x0D,
	KeyMenu: 0x5D,
}

// Extended keys that require the KEYEVENTF_EXTENDEDKEY flag.
var extendedKeys = map[uint16]bool{
	0x25: true, 0x26: true, 0x27: true, 0x28: true, // arrows
	0x24: true, 0x23: true, 0x21: true, 0x22: true, // home/end/pgup/pgdn
	0x2D: true, 0x2E: true, // insert, delete
	0x5B: true, 0x5C: true, // win keys
	0xA3: true, 0xA5: true, // right ctrl, right alt
	0x2C: true, // print screen
	0x6F: true, // numpad divide
	0x0D: true, // numpad enter (shares VK with Return, extended distinguishes)
}

type windowsInjector struct {
	screenW, screenH int
}

func newPlatformInjector() Injector {
	return &windowsInjector{}
}

func (inj *windowsInjector) Init() error {
	w, _, _ := procGetSystemMetrics.Call(0)
	h, _, _ := procGetSystemMetrics.Call(1)
	inj.screenW = int(w)
	inj.screenH = int(h)
	if inj.screenW == 0 || inj.screenH == 0 {
		return fmt.Errorf("failed to get screen metrics")
	}
	return nil
}

func (inj *windowsInjector) MouseMove(x, y int) error {
	ret, _, _ := procSetCursorPos.Call(uintptr(x), uintptr(y))
	if ret == 0 {
		return fmt.Errorf("SetCursorPos failed")
	}
	return nil
}

func (inj *windowsInjector) MouseButton(button uint8, pressed bool) error {
	var flags uint32
	switch button {
	case 0:
		if pressed {
			flags = mousefLeftDown
		} else {
			flags = mousefLeftUp
		}
	case 1:
		if pressed {
			flags = mousefRightDown
		} else {
			flags = mousefRightUp
		}
	case 2:
		if pressed {
			flags = mousefMiddleDown
		} else {
			flags = mousefMiddleUp
		}
	default:
		return fmt.Errorf("unknown button: %d", button)
	}
	return sendMouseInputWin(0, 0, 0, flags)
}

func (inj *windowsInjector) MouseScroll(dx, dy int) error {
	if dy != 0 {
		if err := sendMouseInputWin(0, 0, uint32(int32(dy*wheelDelta)), mousefWheel); err != nil {
			return err
		}
	}
	if dx != 0 {
		if err := sendMouseInputWin(0, 0, uint32(int32(dx*wheelDelta)), mousefHWheel); err != nil {
			return err
		}
	}
	return nil
}

// sendMouseInputWin constructs a Windows INPUT struct with correct ABI layout
// and calls SendInput.
func sendMouseInputWin(dx, dy int32, mouseData, flags uint32) error {
	var buf [inputSize]byte
	// Type at offset 0
	binary.LittleEndian.PutUint32(buf[0:4], inputMouse)
	// MOUSEINPUT union at offset 8
	binary.LittleEndian.PutUint32(buf[8:12], uint32(dx))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(dy))
	binary.LittleEndian.PutUint32(buf[16:20], mouseData)
	binary.LittleEndian.PutUint32(buf[20:24], flags)
	// time = 0 (offset 24), dwExtraInfo = 0 (offset 32) — already zeroed

	ret, _, _ := procSendInput.Call(1, uintptr(unsafe.Pointer(&buf[0])), uintptr(inputSize))
	if ret == 0 {
		return fmt.Errorf("SendInput mouse failed")
	}
	return nil
}

// sendKeyInputWin constructs a KEYBDINPUT and calls SendInput.
func sendKeyInputWin(vk uint16, flags uint32) error {
	var buf [inputSize]byte
	// Type at offset 0
	binary.LittleEndian.PutUint32(buf[0:4], inputKeyboard)
	// KEYBDINPUT union at offset 8
	binary.LittleEndian.PutUint16(buf[8:10], vk)    // wVk
	// wScan = 0 (offset 10) — already zeroed
	binary.LittleEndian.PutUint32(buf[12:16], flags) // dwFlags
	// time = 0 (offset 16), dwExtraInfo = 0 (offset 24) — already zeroed

	ret, _, _ := procSendInput.Call(1, uintptr(unsafe.Pointer(&buf[0])), uintptr(inputSize))
	if ret == 0 {
		return fmt.Errorf("SendInput keyboard failed")
	}
	return nil
}

func (inj *windowsInjector) KeyPress(key Key, pressed bool) error {
	vk, ok := keyToVK[key]
	if !ok {
		return fmt.Errorf("unmapped key: %d", key)
	}
	var flags uint32
	if extendedKeys[vk] {
		flags |= keyfExtendedKey
	}
	if !pressed {
		flags |= keyfKeyUp
	}
	return sendKeyInputWin(vk, flags)
}

func (inj *windowsInjector) SendSpecialCombo(combo uint8) error {
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
	case 3: // Ctrl+Esc (Start menu)
		inj.KeyPress(KeyLeftControl, true)
		inj.KeyPress(KeyEscape, true)
		inj.KeyPress(KeyEscape, false)
		inj.KeyPress(KeyLeftControl, false)
	case 4: // Win+L (Lock)
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyL, true)
		inj.KeyPress(KeyL, false)
		inj.KeyPress(KeyLeftSuper, false)
	case 5: // Win+D (Desktop)
		inj.KeyPress(KeyLeftSuper, true)
		inj.KeyPress(KeyD, true)
		inj.KeyPress(KeyD, false)
		inj.KeyPress(KeyLeftSuper, false)
	default:
		return fmt.Errorf("unknown combo: %d", combo)
	}
	return nil
}

func (inj *windowsInjector) Close() {}
