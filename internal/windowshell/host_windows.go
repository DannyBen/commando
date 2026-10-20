package windowshell

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	user32           = windows.NewLazySystemDLL("user32.dll")
	comctl32         = windows.NewLazySystemDLL("comctl32.dll")
	parseDisplayName = shell32.NewProc("SHParseDisplayName")
	bindToParent     = shell32.NewProc("SHBindToParent")
	bindToObject     = shell32.NewProc("SHBindToObject")
	doDragDrop       = shell32.NewProc("SHDoDragDrop")
	oleInitialize    = ole32.NewProc("OleInitialize")
	oleUninitialize  = ole32.NewProc("OleUninitialize")
	createPopupMenu  = user32.NewProc("CreatePopupMenu")
	destroyMenu      = user32.NewProc("DestroyMenu")
	trackPopupMenu   = user32.NewProc("TrackPopupMenuEx")
	clientToScreen   = user32.NewProc("ClientToScreen")
	getCursorPos     = user32.NewProc("GetCursorPos")
	windowFromPoint  = user32.NewProc("WindowFromPoint")
	getAncestor      = user32.NewProc("GetAncestor")
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	getKeyState      = user32.NewProc("GetKeyState")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	windowThread     = user32.NewProc("GetWindowThreadProcessId")
	releaseCapture   = user32.NewProc("ReleaseCapture")
	setSubclass      = comctl32.NewProc("SetWindowSubclass")
	removeSubclass   = comctl32.NewProc("RemoveWindowSubclass")
	defSubclass      = comctl32.NewProc("DefSubclassProc")
	hosts            sync.Map // HWND -> *Host, roots Go objects until WM_NCDESTROY.
	subclassCallback uintptr
	iidFolder        = windows.GUID{Data1: 0x000214e6, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidData          = windows.GUID{Data1: 0x0000010e, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidMenu          = windows.GUID{Data1: 0x000214e4, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidMenu2         = windows.GUID{Data1: 0x000214f4, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidMenu3         = windows.GUID{Data1: 0xbcfce0a0, Data2: 0xec17, Data3: 0x11d0, Data4: [8]byte{0x8d, 0x10, 0, 0xa0, 0xc9, 0x0f, 0x27, 0x19}}
)

const subclassID = 0x434d44

func init() { subclassCallback = windows.NewCallback(menuWindowProc) }

// Host keeps OLE initialized for the lifetime of its native window, including
// modeless shell dialogs. NewHost must run on that window's owning OS thread.
type Host struct {
	hwnd         uintptr
	thread       uint32
	busy         bool
	oleAlive     bool
	menu2, menu3 *object
	menuMessages uint32
}

func NewHost(hwnd uintptr) (*Host, error) {
	if hwnd == 0 {
		return nil, fmt.Errorf("native window unavailable")
	}
	thread, _, _ := windowThread.Call(hwnd, 0)
	if thread == 0 || uint32(thread) != windows.GetCurrentThreadId() {
		return nil, fmt.Errorf("shell host must use its window's owning thread")
	}
	if _, exists := hosts.Load(hwnd); exists {
		return nil, fmt.Errorf("shell host already installed")
	}
	hr, _, _ := oleInitialize.Call(0)
	if err := hresult(hr); err != nil {
		return nil, fmt.Errorf("OleInitialize: %w", err)
	}
	h := &Host{hwnd: hwnd, thread: windows.GetCurrentThreadId(), oleAlive: true}
	hosts.Store(hwnd, h)
	if ok, _, err := setSubclass.Call(hwnd, subclassCallback, subclassID, 0); ok == 0 {
		hosts.Delete(hwnd)
		oleUninitialize.Call()
		return nil, fmt.Errorf("SetWindowSubclass: %w", err)
	}
	return h, nil
}

func (h *Host) begin() error {
	if windows.GetCurrentThreadId() != h.thread {
		return fmt.Errorf("shell operation on wrong OS thread")
	}
	if h.busy || h.hwnd == 0 {
		return fmt.Errorf("shell host unavailable")
	}
	h.busy = true
	return nil
}

func (h *Host) end() {
	h.busy = false
	if h.hwnd == 0 && h.oleAlive {
		oleUninitialize.Call()
		h.oleAlive = false
	}
}

// Drag is outbound and copy-only. The target/shell owns the transfer;
// the source must never interpret the returned effect as permission to delete.
func (h *Host) Drag(paths []string, armed bool) (Result, error) {
	r := Result{Action: "drag", ThreadID: h.thread}
	if err := h.begin(); err != nil {
		return r, err
	}
	defer h.end()
	data, err := shellObject(h.hwnd, paths, &iidData)
	if err != nil {
		return r, err
	}
	defer data.call(2)
	source := &dropSource{table: &sourceTable, state: dragState{standard: !armed}, owner: h.hwnd}
	if armed {
		// GetAsyncKeyState reports physical buttons; honor the user's logical
		// primary button when Windows has swapped left and right.
		source.primaryButton = 1                                      // VK_LBUTTON
		if swapped, _, _ := getSystemMetrics.Call(23); swapped != 0 { // SM_SWAPBUTTON
			source.primaryButton = 2 // VK_RBUTTON
		}
		source.state.waitRelease = source.primaryDown()
	}
	source.refs.Store(1)
	var pin runtime.Pinner
	pin.Pin(source)
	pin.Pin(&sourceTable)
	sourcePins.Store(source, &pin)
	defer releaseSource(source)
	// Gio captures the pointer during a press; OLE owns it during the drag.
	releaseCapture.Call()
	hr, _, _ := doDragDrop.Call(h.hwnd, uintptr(unsafe.Pointer(data)), uintptr(unsafe.Pointer(source)), 1, uintptr(unsafe.Pointer(&r.Effect)))
	runtime.KeepAlive(source)
	runtime.KeepAlive(data)
	r.Cancelled = uint32(hr) == 0x00040101 // DRAGDROP_S_CANCEL
	return r, hresult(hr)
}

// ContextMenu hosts the traditional Windows shell menu for sibling items or a folder background.
// Menu and command callbacks remain on the native thread throughout.
func (h *Host) ContextMenu(paths []string, options MenuOptions) (Result, error) {
	r := Result{Action: "menu", ThreadID: h.thread}
	start := time.Now()
	extended := options.Extended
	if err := h.begin(); err != nil {
		return r, err
	}
	defer h.end()
	point, err := h.invocationPoint(options)
	if err != nil {
		return r, err
	}
	var menu *object
	if options.Background {
		menu, err = backgroundMenu(h.hwnd, paths, &r.Timings)
	} else {
		menu, err = timedShellObject(h.hwnd, paths, &iidMenu, &r.Timings, false)
	}
	if err != nil {
		return r, err
	}
	defer menu.call(2)
	h.menu3 = menu.query(&iidMenu3)
	if h.menu3 != nil {
		r.MenuVersion = 3
	} else {
		h.menu2 = menu.query(&iidMenu2)
		if h.menu2 != nil {
			r.MenuVersion = 2
		} else {
			r.MenuVersion = 1
		}
	}
	h.menuMessages = 0
	defer func() {
		if h.menu3 != nil {
			h.menu3.call(2)
			h.menu3 = nil
		}
		if h.menu2 != nil {
			h.menu2.call(2)
			h.menu2 = nil
		}
	}()
	hm, _, err := createPopupMenu.Call()
	if hm == 0 {
		return r, fmt.Errorf("CreatePopupMenu: %w", err)
	}
	defer destroyMenu.Call(hm)
	flags := uintptr(0)
	if extended {
		flags = 0x100
	} // CMF_EXTENDEDVERBS
	populate := time.Now()
	hr := menu.call(3, hm, 0, 1, 0x7fff, flags)
	r.Timings.Populate = time.Since(populate)
	if err := hresult(uintptr(hr)); err != nil {
		return r, err
	}
	r.Commands = hr & 0xffff
	// RETURNCMD avoids WM_COMMAND routing into Gio. RIGHTBUTTON permits
	// choosing commands with either mouse button; nested menu messages go
	// through our window subclass before Gio/default processing.
	r.Timings.Ready = time.Since(start)
	releaseCapture.Call()
	command, _, _ := trackPopupMenu.Call(hm, 0x100|0x2, uintptr(point.X), uintptr(point.Y), h.hwnd, 0)
	r.MenuMessages = h.menuMessages
	if command == 0 {
		r.Cancelled = true
		return r, nil
	}
	info := invokeInfo{Mask: 0x4000 | 0x100 | 0x20000000, Window: h.hwnd,
		Verb: command - 1, VerbW: command - 1, Show: windows.SW_SHOWNORMAL, Point: point}
	info.Size = uint32(unsafe.Sizeof(info)) // UNICODE | NOASYNC | PTINVOKE
	if extended {
		info.Mask |= 0x10000000
	} // SHIFT_DOWN
	if state, _, _ := getKeyState.Call(0x11); state&0x8000 != 0 {
		info.Mask |= 0x40000000
	}
	var verb [256]uint16
	if hr := menu.call(5, command-1, 4, 0, uintptr(unsafe.Pointer(&verb[0])), uintptr(len(verb))); int32(hr) >= 0 {
		r.Action = "menu: " + windows.UTF16ToString(verb[:])
	}
	return r, hresult(uintptr(menu.call(4, uintptr(unsafe.Pointer(&info)))))
}

// Properties invokes the canonical shell verb for siblings or one drive root.
func (h *Host) Properties(paths []string, options MenuOptions) (Result, error) {
	r := Result{Action: "properties", ThreadID: h.thread}
	if err := h.begin(); err != nil {
		return r, err
	}
	defer h.end()
	point, err := h.invocationPoint(options)
	if err != nil {
		return r, err
	}
	menu, err := timedShellObject(h.hwnd, paths, &iidMenu, nil, true)
	if err != nil {
		return r, err
	}
	defer menu.call(2)
	verb := []byte("properties\x00")
	verbW, _ := windows.UTF16PtrFromString("properties")
	info := invokeInfo{Mask: 0x4000 | 0x100 | 0x20000000, Window: h.hwnd,
		Verb: uintptr(unsafe.Pointer(&verb[0])), VerbW: uintptr(unsafe.Pointer(verbW)), Show: windows.SW_SHOWNORMAL, Point: point}
	info.Size = uint32(unsafe.Sizeof(info)) // UNICODE | NOASYNC | PTINVOKE
	err = hresult(uintptr(menu.call(4, uintptr(unsafe.Pointer(&info)))))
	runtime.KeepAlive(verb)
	runtime.KeepAlive(verbW)
	return r, err
}

func (h *Host) invocationPoint(options MenuOptions) (nativePoint, error) {
	var point nativePoint
	if options.ClientPoint != nil {
		point = nativePoint{int32(options.ClientPoint.X), int32(options.ClientPoint.Y)}
		if ok, _, err := clientToScreen.Call(h.hwnd, uintptr(unsafe.Pointer(&point))); ok == 0 {
			return point, fmt.Errorf("ClientToScreen: %w", err)
		}
	} else if ok, _, err := getCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return point, fmt.Errorf("GetCursorPos: %w", err)
	}
	return point, nil
}

type nativePoint struct{ X, Y int32 }

// CMINVOKECOMMANDINFOEX, using pointer-sized integers for MAKEINTRESOURCE verbs.
type invokeInfo struct {
	Size, Mask                                          uint32
	Window, Verb, Parameters, Directory                 uintptr
	Show                                                int32
	HotKey                                              uint32
	Icon, Title, VerbW, ParametersW, DirectoryW, TitleW uintptr
	Point                                               nativePoint
}

type object struct{ table *[14]uintptr }

//go:uintptrescapes
func (o *object) call(slot int, args ...uintptr) uint32 {
	arguments := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	hr, _, _ := syscall.SyscallN(o.table[slot], arguments...)
	runtime.KeepAlive(o)
	return uint32(hr)
}

func (o *object) query(iid *windows.GUID) *object {
	var out *object
	if int32(o.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))) < 0 {
		return nil
	}
	return out
}

func shellObject(hwnd uintptr, paths []string, iid *windows.GUID) (*object, error) {
	return timedShellObject(hwnd, paths, iid, nil, false)
}

func timedShellObject(hwnd uintptr, paths []string, iid *windows.GUID, timings *MenuTimings, allowRoot bool) (*object, error) {
	start := time.Now()
	paths, err := siblingPaths(paths, allowRoot)
	if err != nil {
		return nil, err
	}
	ids := make([]unsafe.Pointer, len(paths))
	children := make([]unsafe.Pointer, len(paths))
	defer func() {
		for _, id := range ids {
			if id != nil {
				windows.CoTaskMemFree(id)
			}
		}
	}()
	var parent *object
	for i, path := range paths {
		displayName := path
		if i > 0 {
			displayName = filepath.Base(path)
		}
		name, err := windows.UTF16PtrFromString(displayName)
		if err != nil {
			return nil, err
		}
		var hr uintptr
		if i == 0 {
			hr, _, _ = parseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&ids[i])), 0, 0)
		} else {
			// Siblings share a bound parent. Parse only their relative names, without
			// traversing the parent path and binding another folder for every item.
			hr = uintptr(parent.call(3, hwnd, 0, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&ids[i])), 0))
		}
		runtime.KeepAlive(name)
		if err := hresult(hr); err != nil {
			return nil, fmt.Errorf("SHParseDisplayName: %w", err)
		}
		if i == 0 {
			hr, _, _ = bindToParent.Call(uintptr(ids[i]), uintptr(unsafe.Pointer(&iidFolder)),
				uintptr(unsafe.Pointer(&parent)), uintptr(unsafe.Pointer(&children[i])))
			if err := hresult(hr); err != nil {
				return nil, fmt.Errorf("SHBindToParent: %w", err)
			}
			defer parent.call(2)
		} else {
			children[i] = ids[i]
		}
	}
	if timings != nil {
		timings.Resolve = time.Since(start)
	}
	acquire := time.Now()
	var out *object
	hr := parent.call(10, hwnd, uintptr(len(children)), uintptr(unsafe.Pointer(&children[0])),
		uintptr(unsafe.Pointer(iid)), 0, uintptr(unsafe.Pointer(&out)))
	if timings != nil {
		timings.Object = time.Since(acquire)
	}
	runtime.KeepAlive(ids)
	runtime.KeepAlive(children)
	if err := hresult(uintptr(hr)); err != nil {
		return nil, fmt.Errorf("GetUIObjectOf: %w", err)
	}
	return out, nil
}

func hresult(hr uintptr) error {
	if int32(uint32(hr)) >= 0 {
		return nil
	}
	if uint32(hr)&0xffff0000 == 0x80070000 {
		return syscall.Errno(uint32(hr) & 0xffff)
	}
	return fmt.Errorf("Windows HRESULT 0x%08X", uint32(hr))
}

func menuWindowProc(hwnd uintptr, msg uint32, wparam, lparam, id, data uintptr) uintptr {
	if value, ok := hosts.Load(hwnd); ok {
		h := value.(*Host)
		// Closing a window inside its own shell modal operation would destroy
		// the owner and OLE apartment before their callbacks have unwound.
		if msg == 0x0010 && h.busy {
			return 0
		} // WM_CLOSE
		if menuMessage(msg, wparam) {
			if h.menu3 != nil {
				var result uintptr
				if hr := h.menu3.call(7, uintptr(msg), wparam, lparam, uintptr(unsafe.Pointer(&result))); hr == 0 {
					h.menuMessages++
					return result
				}
			} else if h.menu2 != nil && msg != 0x0120 {
				if hr := h.menu2.call(6, uintptr(msg), wparam, lparam); hr == 0 {
					h.menuMessages++
					if msg == 0x002b || msg == 0x002c {
						return 1
					}
					return 0
				}
			}
		}
		if msg == 0x0082 { // WM_NCDESTROY
			removeSubclass.Call(hwnd, subclassCallback, id)
			hosts.Delete(hwnd)
			h.hwnd = 0
			if !h.busy {
				h.end()
			}
		}
	}
	result, _, _ := defSubclass.Call(hwnd, uintptr(msg), wparam, lparam)
	return result
}

func menuMessage(msg uint32, wparam uintptr) bool {
	switch msg {
	case 0x0117, 0x0120:
		return true // INITMENUPOPUP, MENUCHAR
	case 0x002b, 0x002c:
		return wparam == 0 // DRAWITEM, MEASUREITEM for menus only.
	}
	return false
}
