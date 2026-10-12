package app

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"golang.org/x/sys/windows"
)

var (
	commonControls       = windows.NewLazySystemDLL("comctl32.dll")
	setWindowSubclass    = commonControls.NewProc("SetWindowSubclass")
	removeWindowSubclass = commonControls.NewProc("RemoveWindowSubclass")
	defSubclassProc      = commonControls.NewProc("DefSubclassProc")
	shortcutCallback     uintptr
	user32               = windows.NewLazySystemDLL("user32.dll")
	setWindowsHookEx     = user32.NewProc("SetWindowsHookExW")
	unhookWindowsHookEx  = user32.NewProc("UnhookWindowsHookEx")
	callNextHookEx       = user32.NewProc("CallNextHookEx")
	keyboardCallback     uintptr
	getWindowPlacement   = user32.NewProc("GetWindowPlacement")
	setWindowPlacement   = user32.NewProc("SetWindowPlacement")
	getKeyState          = user32.NewProc("GetKeyState")
	nativeWindows        sync.Map // HWND -> *nativeWindow, held until WM_NCDESTROY.
)

const shortcutSubclassID = 1

type nativeWindow struct {
	state      *windowTracker
	keys       *nativeKeyInput
	hwnd       uintptr
	configured bool
}

type nativePoint struct{ X, Y int32 }
type nativeRect struct{ Left, Top, Right, Bottom int32 }

type windowPlacement struct {
	Length, Flags, ShowCmd uint32
	Min, Max               nativePoint
	Normal                 nativeRect
}

func (*nativeWindow) options() []gioapp.Option { return nil }

func init() {
	shortcutCallback = windows.NewCallback(shortcutWindowProc)
	keyboardCallback = windows.NewCallback(shortcutKeyboardProc)
}

func (n *nativeWindow) configure(w *gioapp.Window, e event.Event) {
	if view, ok := e.(gioapp.Win32ViewEvent); ok {
		previous := n.hwnd
		n.hwnd = view.HWND
		if n.hwnd != 0 && n.hwnd != previous {
			w.Run(func() { n.restore(n.hwnd) })
		}
	}
	// Install after the first rendered frame, so our keyboard hook precedes
	// the renderer's fullscreen shortcut handling in the thread's hook chain.
	if _, ok := e.(gioapp.FrameEvent); !ok || n.hwnd == 0 || n.configured {
		return
	}
	n.configured = true
	hwnd := n.hwnd
	// Both the subclass and keyboard hook belong to the window's owning thread.
	w.Run(func() {
		const whKeyboard = 2
		hook, _, err := setWindowsHookEx.Call(whKeyboard, keyboardCallback, 0, uintptr(windows.GetCurrentThreadId()))
		if hook == 0 {
			fmt.Fprintln(os.Stderr, "cannot install window keyboard hook:", err)
		}
		nativeWindows.Store(hwnd, n)
		installed, _, err := setWindowSubclass.Call(hwnd, shortcutCallback, shortcutSubclassID, hook)
		if installed == 0 {
			nativeWindows.Delete(hwnd)
			if hook != 0 {
				unhookWindowsHookEx.Call(hook)
			}
			fmt.Fprintln(os.Stderr, "cannot install window shortcut handler:", err)
		}
		n.capture(hwnd)
	})
}

func (n *nativeWindow) restore(hwnd uintptr) {
	saved := n.state.snapshot()
	if !saved.valid() || !saved.Positioned {
		return
	}
	placement := saved.placement()
	if ok, _, err := setWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement))); ok == 0 {
		fmt.Fprintln(os.Stderr, "cannot restore window position:", err)
	}
}

func (s windowState) placement() windowPlacement {
	placement := windowPlacement{ShowCmd: 1,
		Min: nativePoint{-1, -1}, Max: nativePoint{-1, -1},
		Normal: nativeRect{int32(s.X), int32(s.Y), int32(s.X + s.Width), int32(s.Y + s.Height)},
	}
	placement.Length = uint32(unsafe.Sizeof(placement))
	if s.Maximized {
		placement.ShowCmd = 3
	}
	return placement
}

func (p windowPlacement) state() windowState {
	return windowState{
		X: int(p.Normal.Left), Y: int(p.Normal.Top),
		Width: int(p.Normal.Right - p.Normal.Left), Height: int(p.Normal.Bottom - p.Normal.Top),
		Positioned: true,
		Maximized:  p.ShowCmd == 3 || p.ShowCmd == 2 && p.Flags&2 != 0,
	}
}

func (n *nativeWindow) capture(hwnd uintptr) {
	var placement windowPlacement
	placement.Length = uint32(unsafe.Sizeof(placement))
	if ok, _, _ := getWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement))); ok != 0 {
		if state := placement.state(); state.valid() {
			n.state.set(state)
		}
	}
}

func shortcutKeyboardProc(code int32, wParam, lParam uintptr) uintptr {
	const vkReturn = 0x0d
	// Consume Alt+Enter before it reaches DXGI or Gio. The context bit applies
	// to both key presses and releases, including repeats and keypad Enter.
	if code >= 0 && wParam == vkReturn && lParam&(1<<29) != 0 {
		return 1
	}
	result, _, _ := callNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return result
}

func shortcutWindowProc(hwnd uintptr, message uint32, wParam, lParam, id, data uintptr) uintptr {
	const wmSysChar, wmNCDestroy = 0x0106, 0x0082
	const wmClose, wmDestroy, wmWindowPosChanged = 0x0010, 0x0002, 0x0047
	const wmKeyDown, wmKeyUp, vkInsert = 0x0100, 0x0101, 0x2d
	const wmSysKeyDown = 0x0104
	if (message == wmKeyDown || message == wmSysKeyDown) && wParam != vkInsert {
		if native, ok := nativeWindows.Load(hwnd); ok {
			if keys := native.(*nativeWindow).keys; keys != nil {
				keys.noticeInteraction()
			}
		}
	}
	if (message == wmKeyDown || message == wmKeyUp) && wParam == vkInsert {
		if native, ok := nativeWindows.Load(hwnd); ok {
			n := native.(*nativeWindow)
			if n.keys != nil {
				e := key.Event{Name: insertKey, State: key.Press}
				if message == wmKeyUp {
					e.State = key.Release
				}
				for _, modifier := range []struct {
					vk  uintptr
					mod key.Modifiers
				}{
					{0x10, key.ModShift}, {0x11, key.ModCtrl}, {0x12, key.ModAlt},
					{0x5b, key.ModSuper}, {0x5c, key.ModSuper},
				} {
					state, _, _ := getKeyState.Call(modifier.vk)
					if state&0x8000 != 0 {
						e.Modifiers |= modifier.mod
					}
				}
				n.keys.push(e)
			}
		}
	}
	// Capture before destruction while the HWND is still valid. Position/size
	// changes also keep a snapshot if the window is destroyed without WM_CLOSE.
	if message == wmClose || message == wmDestroy || message == wmWindowPosChanged {
		if native, ok := nativeWindows.Load(hwnd); ok {
			native.(*nativeWindow).capture(hwnd)
		}
	}
	// Gio already receives the key event for Alt+D. Its extra system-character
	// message otherwise reaches the default Windows menu handler and rings a bell.
	if message == wmSysChar && (wParam == 'd' || wParam == 'D') && lParam&(1<<29) != 0 {
		return 0
	}
	if message == wmNCDestroy {
		if data != 0 {
			unhookWindowsHookEx.Call(data)
		}
		removeWindowSubclass.Call(hwnd, shortcutCallback, id)
		nativeWindows.Delete(hwnd)
	}
	result, _, _ := defSubclassProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (n *nativeWindow) owner() uintptr { return n.hwnd }
