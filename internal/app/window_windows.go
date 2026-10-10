package app

import (
	"fmt"
	"os"

	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows"
)

var (
	commonControls       = windows.NewLazySystemDLL("comctl32.dll")
	setWindowSubclass    = commonControls.NewProc("SetWindowSubclass")
	removeWindowSubclass = commonControls.NewProc("RemoveWindowSubclass")
	defSubclassProc      = commonControls.NewProc("DefSubclassProc")
	shortcutCallback     uintptr
)

const shortcutSubclassID = 1

func init() { shortcutCallback = windows.NewCallback(shortcutWindowProc) }

func configureNativeWindow(w *gioapp.Window, e event.Event) {
	view, ok := e.(gioapp.Win32ViewEvent)
	if !ok || view.HWND == 0 {
		return
	}
	// Windows requires installing the subclass on the window's owning thread.
	w.Run(func() {
		installed, _, err := setWindowSubclass.Call(view.HWND, shortcutCallback, shortcutSubclassID, 0)
		if installed == 0 {
			fmt.Fprintln(os.Stderr, "cannot install window shortcut handler:", err)
		}
	})
}

func shortcutWindowProc(hwnd uintptr, message uint32, wParam, lParam, id, data uintptr) uintptr {
	const wmSysChar, wmNCDestroy = 0x0106, 0x0082
	// Gio already receives the key event for Alt+D. Its extra system-character
	// message otherwise reaches the default Windows menu handler and rings a bell.
	if message == wmSysChar && (wParam == 'd' || wParam == 'D') && lParam&(1<<29) != 0 {
		return 0
	}
	if message == wmNCDestroy {
		removeWindowSubclass.Call(hwnd, shortcutCallback, id)
	}
	result, _, _ := defSubclassProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}
