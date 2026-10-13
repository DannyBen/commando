package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	registerWindowMessage    = user32.NewProc("RegisterWindowMessageW")
	enumWindows              = user32.NewProc("EnumWindows")
	setProp                  = user32.NewProc("SetPropW")
	getProp                  = user32.NewProc("GetPropW")
	removeProp               = user32.NewProc("RemovePropW")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	allowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	setForegroundWindow      = user32.NewProc("SetForegroundWindow")
	postMessage              = user32.NewProc("PostMessageW")
	isIconic                 = user32.NewProc("IsIconic")
	showWindow               = user32.NewProc("ShowWindow")
	getLastActivePopup       = user32.NewProc("GetLastActivePopup")
	isWindowVisible          = user32.NewProc("IsWindowVisible")
	instanceWindowCallback   = windows.NewCallback(findInstanceWindow)
)

type instanceGuard struct {
	property *uint16
	message  uint32
	release  func()
}

// Identity follows the shared data folder, across different executable paths.
// Local scopes the kernel lock to this Windows login session.
func instanceName(dataDir string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return fmt.Sprintf("Commando.Instance.%x", hash[:16])
}

func startInstance(dataDir string) (*instanceGuard, bool, error) {
	name := instanceName(dataDir)
	property, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, err
	}
	message, _, err := registerWindowMessage.Call(uintptr(unsafe.Pointer(property)))
	if message == 0 {
		return nil, false, fmt.Errorf("register activation message: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		release, err := claimInstance(`Local\` + name)
		if err != nil {
			return nil, false, err
		}
		if release != nil {
			return &instanceGuard{property: property, message: uint32(message), release: release}, true, nil
		}
		hwnd, err := existingInstanceWindow(property)
		if err != nil {
			return nil, false, err
		}
		if hwnd != 0 {
			var pid uint32
			getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			// A user-launched second process can give the first permission to
			// activate. Background launches remain subject to Windows focus rules.
			allowSetForegroundWindow.Call(uintptr(pid))
			if ok, _, err := postMessage.Call(hwnd, message, 0, 0); ok != 0 {
				return nil, false, nil
			} else if !errors.Is(err, windows.ERROR_INVALID_WINDOW_HANDLE) {
				return nil, false, fmt.Errorf("activate existing window: %w", err)
			}
		}
		if !time.Now().Before(deadline) {
			return nil, false, fmt.Errorf("existing Commando window is not ready")
		}
		// Another launch may still be creating its window or finishing shutdown.
		// Retry the lock too, so a crashed/exited owner cannot block startup.
		time.Sleep(50 * time.Millisecond)
	}
}

// Windows mutex ownership belongs to an OS thread. Keep acquisition, lifetime
// and release on one dedicated thread, independently of Gio's window thread.
func claimInstance(name string) (func(), error) {
	type result struct {
		release func()
		err     error
	}
	ready := make(chan result)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ptr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			ready <- result{err: err}
			return
		}
		mutex, err := windows.CreateMutex(nil, false, ptr)
		if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			ready <- result{err: err}
			return
		}
		defer func() {
			if mutex != 0 {
				windows.CloseHandle(mutex)
			}
		}()
		status, err := windows.WaitForSingleObject(mutex, 0)
		if err != nil {
			ready <- result{err: err}
			return
		}
		switch status {
		case windows.WAIT_OBJECT_0, windows.WAIT_ABANDONED:
			stop, done := make(chan struct{}), make(chan struct{})
			var once sync.Once
			ready <- result{release: func() { once.Do(func() { close(stop); <-done }) }}
			<-stop
			windows.ReleaseMutex(mutex)
			windows.CloseHandle(mutex)
			// Close before acknowledging release, so shutdown cannot race a new owner.
			mutex = 0
			close(done)
		case uint32(windows.WAIT_TIMEOUT):
			ready <- result{}
		default:
			ready <- result{err: fmt.Errorf("unexpected instance mutex status: %d", status)}
		}
	}()
	r := <-ready
	return r.release, r.err
}

var instanceSearchID atomic.Uintptr
var instanceSearches sync.Map

type instanceWindowSearch struct {
	property *uint16
	hwnd     uintptr
}

func findInstanceWindow(hwnd, data uintptr) uintptr {
	value, ok := instanceSearches.Load(data)
	if !ok {
		return 1
	}
	search := value.(*instanceWindowSearch)
	if value, _, _ := getProp.Call(hwnd, uintptr(unsafe.Pointer(search.property))); value != 0 {
		search.hwnd = hwnd
		return 0
	}
	return 1
}

func existingInstanceWindow(property *uint16) (uintptr, error) {
	search := &instanceWindowSearch{property: property}
	id := instanceSearchID.Add(1)
	instanceSearches.Store(id, search)
	defer instanceSearches.Delete(id)
	ok, _, err := enumWindows.Call(instanceWindowCallback, id)
	if ok == 0 && search.hwnd == 0 {
		return 0, fmt.Errorf("find existing Commando window: %w", err)
	}
	return search.hwnd, nil
}

func (i *instanceGuard) publish(hwnd uintptr) error {
	if i == nil {
		return nil
	}
	if ok, _, err := setProp.Call(hwnd, uintptr(unsafe.Pointer(i.property)), 1); ok == 0 {
		return fmt.Errorf("publish Commando window: %w", err)
	}
	return nil
}

func (i *instanceGuard) unpublish(hwnd uintptr) {
	if i != nil {
		removeProp.Call(hwnd, uintptr(unsafe.Pointer(i.property)))
	}
}

func (i *instanceGuard) close() {
	if i != nil {
		i.release()
	}
}

func activateInstanceWindow(hwnd uintptr) {
	if minimized, _, _ := isIconic.Call(hwnd); minimized != 0 {
		var placement windowPlacement
		placement.Length = uint32(unsafe.Sizeof(placement))
		getWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&placement)))
		command := uintptr(9) // SW_RESTORE, only for a minimized window.
		if placement.Flags&2 != 0 {
			command = 3
		} // WPF_RESTORETOMAXIMIZED.
		showWindow.Call(hwnd, command)
	}
	// Activate a visible shell dialog when it owns the current operation.
	if popup, _, _ := getLastActivePopup.Call(hwnd); popup != 0 && popup != hwnd {
		if visible, _, _ := isWindowVisible.Call(popup); visible != 0 {
			hwnd = popup
		}
	}
	setForegroundWindow.Call(hwnd)
}
