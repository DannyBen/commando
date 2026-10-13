package app

import (
	"fmt"
	"image"
	"math"
	"os"
	"unsafe"

	gioapp "gioui.org/app"
)

var (
	getWindowRect     = user32.NewProc("GetWindowRect")
	getClientRect     = user32.NewProc("GetClientRect")
	getWindowDPI      = user32.NewProc("GetDpiForWindow")
	monitorFromWindow = user32.NewProc("MonitorFromWindow")
	getMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	setWindowPos      = user32.NewProc("SetWindowPos")
	isWindowArranged  = user32.NewProc("IsWindowArranged")
)

type monitorInfo struct {
	Size          uint32
	Monitor, Work nativeRect
	Flags         uint32
}

func (r nativeRect) rectangle() image.Rectangle {
	return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
}

type windowGeometry struct {
	window, work image.Rectangle
	clientWidth  int
	scale        float64
	monitor      uintptr
	ordinary     bool
}

// Read on the native window thread: WINDOWPLACEMENT uses workspace coordinates,
// while GetWindowRect/GetMonitorInfo provide the screen coordinates for resizing.
func (n *nativeWindow) geometry() (windowGeometry, bool) {
	var outer, client nativeRect
	if ok, _, _ := getWindowRect.Call(n.hwnd, uintptr(unsafe.Pointer(&outer))); ok == 0 {
		return windowGeometry{}, false
	}
	if ok, _, _ := getClientRect.Call(n.hwnd, uintptr(unsafe.Pointer(&client))); ok == 0 {
		return windowGeometry{}, false
	}
	monitor, _, _ := monitorFromWindow.Call(n.hwnd, 2)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return windowGeometry{}, false
	}
	dpi, _, _ := getWindowDPI.Call(n.hwnd)
	if dpi == 0 {
		dpi = 96
	}
	var placement windowPlacement
	placement.Length = uint32(unsafe.Sizeof(placement))
	if ok, _, _ := getWindowPlacement.Call(n.hwnd, uintptr(unsafe.Pointer(&placement))); ok == 0 {
		return windowGeometry{}, false
	}
	// Windows reports snap state separately from WINDOWPLACEMENT.ShowCmd.
	// https://learn.microsoft.com/windows/win32/winmsg/winuser/nf-winuser-iswindowarranged
	if err := isWindowArranged.Find(); err != nil {
		return windowGeometry{}, false
	}
	arranged, _, _ := isWindowArranged.Call(n.hwnd)
	g := windowGeometry{window: outer.rectangle(), work: info.Work.rectangle(),
		clientWidth: int(client.Right - client.Left), scale: float64(dpi) / 96, monitor: monitor, ordinary: placement.ShowCmd == 1 && arranged == 0}
	return g, g.clientWidth > 0 && g.work.Dx() > 0
}

// Only an explicit sizing gesture can rewrite the base width. Programmatic
// resizes, movement, DPI changes, snap/maximize/restore, and height-only drags
// never learn a temporarily constrained width.
func (n *nativeWindow) observeSizing(message uint32, edge uintptr) {
	switch message {
	case 0x0231: // WM_ENTERSIZEMOVE
		n.userSizing = false
		if g, ok := n.geometry(); ok {
			n.resizeStartWidth = float64(g.clientWidth) / g.scale
		}
	case 0x0214: // WM_SIZING
		// WMSZ_TOP/BOTTOM are height-only gestures.
		n.userSizing = n.userSizing || edge == 1 || edge == 2 || edge == 4 || edge == 5 || edge == 7 || edge == 8
	case 0x0232: // WM_EXITSIZEMOVE
		if g, ok := n.geometry(); ok && g.ordinary {
			width := float64(g.clientWidth) / g.scale
			if n.userSizing && math.Abs(width-n.resizeStartWidth) > 0.25 {
				n.state.rememberPanelWidth(width, true)
			}
			if g.window.Dx() < g.work.Dx() {
				n.alignment, n.alignmentSet = windowAlignment(g.window, g.work), true
			}
		}
		n.userSizing = false
	}
}

func (n *nativeWindow) resizePanels(w *gioapp.Window, count int) {
	if !n.state.panelCount(count) || n.hwnd == 0 {
		return
	}
	w.Run(func() {
		g, ok := n.geometry()
		if !ok || !g.ordinary {
			return
		}
		if !n.alignmentSet || n.monitor != g.monitor {
			n.alignment, n.alignmentSet = windowAlignment(g.window, g.work), true
		}
		n.monitor = g.monitor
		sizing := n.state.sizingSnapshot()
		if sizing.baseWidth <= 0 {
			return
		}
		frame := g.window.Dx() - g.clientWidth
		width := max(int(math.Round(sizing.width()*g.scale)), int(math.Round(minWindowWidth*g.scale))) + frame
		r := resizedWindow(g.window, g.work, width, n.alignment)
		if r == g.window {
			return
		}
		const noActivate, noZOrder = 0x0010, 0x0004
		if ok, _, err := setWindowPos.Call(n.hwnd, 0, uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(r.Dx()), uintptr(r.Dy()), noActivate|noZOrder); ok == 0 {
			fmt.Fprintln(os.Stderr, "cannot resize workspace:", err)
		}
	})
}
