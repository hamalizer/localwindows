package input

// Key represents a platform-independent virtual key code.
type Key uint16

// Virtual key constants used across all platforms.
const (
	KeyA Key = iota
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ

	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9

	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12

	KeyReturn
	KeyTab
	KeyBackspace
	KeyDelete
	KeyEscape
	KeySpace

	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert

	KeyLeftShift
	KeyRightShift
	KeyLeftControl
	KeyRightControl
	KeyLeftAlt
	KeyRightAlt
	KeyLeftSuper
	KeyRightSuper

	KeyCapsLock
	KeyNumLock
	KeyScrollLock
	KeyPrintScreen
	KeyPause

	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeySemicolon
	KeyApostrophe
	KeyGraveAccent
	KeyComma
	KeyPeriod
	KeySlash

	KeyNumpad0
	KeyNumpad1
	KeyNumpad2
	KeyNumpad3
	KeyNumpad4
	KeyNumpad5
	KeyNumpad6
	KeyNumpad7
	KeyNumpad8
	KeyNumpad9
	KeyNumpadAdd
	KeyNumpadSubtract
	KeyNumpadMultiply
	KeyNumpadDivide
	KeyNumpadDecimal
	KeyNumpadEnter

	KeyMenu

	KeyCount // sentinel
)

// Injector simulates mouse and keyboard input on the host machine.
type Injector interface {
	// Init initializes the input system.
	Init() error

	// MouseMove moves the cursor to absolute screen coordinates.
	MouseMove(x, y int) error

	// MouseButton presses or releases a mouse button (0=left,1=right,2=middle).
	MouseButton(button uint8, pressed bool) error

	// MouseScroll scrolls by dx, dy (in abstract units).
	MouseScroll(dx, dy int) error

	// KeyPress presses or releases a key.
	KeyPress(key Key, pressed bool) error

	// SendSpecialCombo sends a special key combination (e.g. Ctrl+Alt+Del).
	SendSpecialCombo(combo uint8) error

	// Close releases input resources.
	Close()
}

// New returns a platform-appropriate Injector.
func New() Injector {
	return newPlatformInjector()
}
