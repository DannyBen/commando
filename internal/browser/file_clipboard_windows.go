package browser

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clipboardUser32            = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel32          = windows.NewLazySystemDLL("kernel32.dll")
	clipboardShell32           = windows.NewLazySystemDLL("shell32.dll")
	openClipboard              = clipboardUser32.NewProc("OpenClipboard")
	closeClipboard             = clipboardUser32.NewProc("CloseClipboard")
	emptyClipboard             = clipboardUser32.NewProc("EmptyClipboard")
	getClipboardData           = clipboardUser32.NewProc("GetClipboardData")
	setClipboardData           = clipboardUser32.NewProc("SetClipboardData")
	registerClipboardFormat    = clipboardUser32.NewProc("RegisterClipboardFormatW")
	getClipboardSequenceNumber = clipboardUser32.NewProc("GetClipboardSequenceNumber")
	createClipboardWindow      = clipboardUser32.NewProc("CreateWindowExW")
	destroyClipboardWindow     = clipboardUser32.NewProc("DestroyWindow")
	globalAlloc                = clipboardKernel32.NewProc("GlobalAlloc")
	globalLock                 = clipboardKernel32.NewProc("GlobalLock")
	globalUnlock               = clipboardKernel32.NewProc("GlobalUnlock")
	globalFree                 = clipboardKernel32.NewProc("GlobalFree")
	globalSize                 = clipboardKernel32.NewProc("GlobalSize")
	moveClipboardMemory        = clipboardKernel32.NewProc("RtlMoveMemory")
	dragQueryFile              = clipboardShell32.NewProc("DragQueryFileW")
)

const fileDropFormat = 15

func FileClipboardSequence() uint32 {
	sequence, _, _ := getClipboardSequenceNumber.Call()
	return uint32(sequence)
}

// Every operation remains on one OS thread. Writes use a short-lived,
// message-only owner window and eagerly rendered data, with no delayed provider.
func withFileClipboard(write bool, fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var hwnd uintptr
	if write {
		class, _ := windows.UTF16PtrFromString("STATIC")
		var err error
		hwnd, _, err = createClipboardWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0) // HWND_MESSAGE
		if hwnd == 0 {
			return fmt.Errorf("clipboard window: %w", err)
		}
		defer destroyClipboardWindow.Call(hwnd)
	}
	if ok, _, err := openClipboard.Call(hwnd); ok == 0 {
		return fmt.Errorf("open clipboard: %w", err)
	}
	defer closeClipboard.Call()
	return fn()
}

func preferredEffectFormat() (uintptr, error) {
	name, _ := windows.UTF16PtrFromString("Preferred DropEffect")
	format, _, err := registerClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	if format == 0 {
		return 0, err
	}
	return format, nil
}

func ReadFileClipboard() (snapshot FileClipboard, err error) {
	var dib bool
	err = withFileClipboard(false, func() error {
		hdrop, _, _ := getClipboardData.Call(fileDropFormat)
		if hdrop == 0 {
			var imageErr error
			snapshot.ImagePNG, dib, imageErr = readClipboardImageLocked()
			snapshot.Sequence = FileClipboardSequence()
			return imageErr
		}
		count, _, _ := dragQueryFile.Call(hdrop, 0xffffffff, 0, 0)
		if count == 0 || count > 100_000 {
			return ErrNoClipboardFiles
		}
		for i := uintptr(0); i < count; i++ {
			length, _, _ := dragQueryFile.Call(hdrop, i, 0, 0)
			if length == 0 || length > 32_768 {
				return ErrNoClipboardFiles
			}
			buffer := make([]uint16, length+1)
			written, _, _ := dragQueryFile.Call(hdrop, i, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
			if written != length {
				return ErrNoClipboardFiles
			}
			snapshot.Paths = append(snapshot.Paths, windows.UTF16ToString(buffer))
		}
		format, err := preferredEffectFormat()
		if err != nil {
			return err
		}
		handle, _, _ := getClipboardData.Call(format)
		if handle != 0 {
			size, _, _ := globalSize.Call(handle)
			if size >= 4 {
				pointer, _, _ := globalLock.Call(handle)
				if pointer != 0 {
					var effect uint32
					moveClipboardMemory.Call(uintptr(unsafe.Pointer(&effect)), pointer, 4)
					snapshot.Move = effect&2 != 0
					globalUnlock.Call(handle)
				}
			}
		}
		snapshot.Sequence = FileClipboardSequence()
		return nil
	})
	if err == nil && len(snapshot.ImagePNG) > 0 {
		snapshot.ImagePNG, err = clipboardPNG(snapshot.ImagePNG, dib)
	}
	return
}

func WriteFileClipboard(paths []string, move bool) (snapshot FileClipboard, err error) {
	if len(paths) == 0 {
		return snapshot, ErrNoClipboardFiles
	}
	err = withFileClipboard(true, func() error {
		var err error
		snapshot, err = writeFileClipboardLocked(paths, move)
		return err
	})
	return
}

func writeFileClipboardLocked(paths []string, move bool) (FileClipboard, error) {
	format, err := preferredEffectFormat()
	if err != nil {
		return FileClipboard{}, err
	}
	var drop, effect uintptr
	if len(paths) != 0 {
		data, err := encodeFileDrop(paths)
		if err != nil {
			return FileClipboard{}, err
		}
		drop, err = clipboardMemory(data)
		if err != nil {
			return FileClipboard{}, err
		}
		defer func() {
			if drop != 0 {
				globalFree.Call(drop)
			}
		}()
		var bytes [4]byte
		value := uint32(1)
		if move {
			value = 2
		}
		binary.LittleEndian.PutUint32(bytes[:], value)
		effect, err = clipboardMemory(bytes[:])
		if err != nil {
			return FileClipboard{}, err
		}
		defer func() {
			if effect != 0 {
				globalFree.Call(effect)
			}
		}()
	}
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		return FileClipboard{}, err
	}
	if drop != 0 {
		if ok, _, err := setClipboardData.Call(fileDropFormat, drop); ok == 0 {
			return FileClipboard{}, err
		}
		drop = 0 // Windows now owns this allocation.
		if ok, _, err := setClipboardData.Call(format, effect); ok == 0 {
			emptyClipboard.Call() // Never leave a cut payload accidentally labelled copy.
			return FileClipboard{}, err
		}
		effect = 0
	}
	return FileClipboard{Paths: append([]string(nil), paths...), Move: move, Sequence: FileClipboardSequence()}, nil
}

func clipboardMemory(data []byte) (uintptr, error) {
	handle, _, err := globalAlloc.Call(2, uintptr(len(data))) // GMEM_MOVEABLE
	if handle == 0 {
		return 0, err
	}
	pointer, _, err := globalLock.Call(handle)
	if pointer == 0 {
		globalFree.Call(handle)
		return 0, err
	}
	moveClipboardMemory.Call(pointer, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	globalUnlock.Call(handle)
	return handle, nil
}

func CompleteFileClipboard(snapshot FileClipboard, moved []string) (updated FileClipboard, err error) {
	updated = snapshot
	if !snapshot.Move || len(moved) == 0 {
		return
	}
	err = withFileClipboard(true, func() error {
		// The lock makes sequence check and replacement atomic relative to other
		// clipboard users. Never clear a newer selection or text clipboard.
		if FileClipboardSequence() != snapshot.Sequence {
			updated = FileClipboard{}
			return nil
		}
		var err error
		updated, err = writeFileClipboardLocked(remainingClipboardPaths(snapshot, moved), true)
		return err
	})
	return
}
