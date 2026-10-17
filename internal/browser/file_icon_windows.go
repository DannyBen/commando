package browser

import (
	"image"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iconShellInfo    = windows.NewLazySystemDLL("shell32.dll").NewProc("SHGetFileInfoW")
	iconFromList     = windows.NewLazySystemDLL("comctl32.dll").NewProc("ImageList_GetIcon")
	iconDraw         = windows.NewLazySystemDLL("user32.dll").NewProc("DrawIconEx")
	iconDestroy      = windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon")
	iconGDI          = windows.NewLazySystemDLL("gdi32.dll")
	iconCreateDC     = iconGDI.NewProc("CreateCompatibleDC")
	iconDeleteDC     = iconGDI.NewProc("DeleteDC")
	iconCreateDIB    = iconGDI.NewProc("CreateDIBSection")
	iconSelect       = iconGDI.NewProc("SelectObject")
	iconDeleteObject = iconGDI.NewProc("DeleteObject")
	iconFlush        = iconGDI.NewProc("GdiFlush")
)

// NewFileIconReader reuses Windows' system image list. Call the returned reader
// only on the directory worker; its small image cache is memory-only.
func NewFileIconReader() func(string, int) image.Image {
	type key struct {
		list  uintptr
		index int32
		size  int
	}
	type cachedIcon struct {
		image image.Image
		used  uint64
	}
	cache := make(map[key]cachedIcon)
	var clock uint64
	return func(path string, size int) image.Image {
		if size < 1 || size > 128 {
			return nil
		}
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil
		}
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
			return nil
		}
		defer windows.CoUninitialize()
		var info struct {
			icon       windows.Handle
			index      int32
			attributes uint32
			name       [260]uint16
			typeName   [80]uint16
		}
		// The real path is important: EXEs, shortcuts and custom folders can
		// have their own artwork rather than an extension-wide default.
		list, _, _ := iconShellInfo.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0x4000|0x1)
		runtime.KeepAlive(name)
		if list == 0 {
			return nil
		}
		k := key{list, info.index, size}
		clock++
		if icon, ok := cache[k]; ok {
			icon.used = clock
			cache[k] = icon
			return icon.image
		}
		h, _, _ := iconFromList.Call(list, uintptr(info.index), 0)
		if h == 0 {
			return nil
		}
		defer iconDestroy.Call(h)
		img := rasterizeFileIcon(h, size)
		if len(cache) >= 256 {
			var oldest key
			var oldestUse uint64 = ^uint64(0)
			for key, icon := range cache {
				if icon.used < oldestUse {
					oldest, oldestUse = key, icon.used
				}
			}
			delete(cache, oldest)
		}
		cache[k] = cachedIcon{image: img, used: clock}
		return img
	}
}

func rasterizeFileIcon(icon uintptr, size int) image.Image {
	dc, _, _ := iconCreateDC.Call(0)
	if dc == 0 {
		return nil
	}
	defer iconDeleteDC.Call(dc)
	var header struct {
		size                   uint32
		width, height          int32
		planes, depth          uint16
		compression, imageSize uint32
		xppm, yppm             int32
		used, important        uint32
		colors                 uint32
	}
	header.size, header.width, header.height = 40, int32(size), -int32(size)
	header.planes, header.depth = 1, 32
	var bits unsafe.Pointer
	bitmap, _, _ := iconCreateDIB.Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		return nil
	}
	defer iconDeleteObject.Call(bitmap)
	old, _, _ := iconSelect.Call(dc, bitmap)
	if old == 0 || old == ^uintptr(0) {
		return nil
	}
	defer iconSelect.Call(dc, old)
	pixels := unsafe.Slice((*byte)(bits), size*size*4)
	// Drawing over black and white recovers transparency for both modern
	// alpha icons and legacy mask icons, retaining the original shading.
	clear(pixels)
	if ok, _, _ := iconDraw.Call(dc, 0, 0, icon, uintptr(size), uintptr(size), 0, 0, 3); ok == 0 {
		return nil
	}
	iconFlush.Call()
	black := append([]byte(nil), pixels...)
	for i := range pixels {
		pixels[i] = 255
	}
	if ok, _, _ := iconDraw.Call(dc, 0, 0, icon, uintptr(size), uintptr(size), 0, 0, 3); ok == 0 {
		return nil
	}
	iconFlush.Call()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(pixels); i += 4 {
		a := byte(255 - max(0, int(pixels[i])-int(black[i])))
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = min(black[i+2], a), min(black[i+1], a), min(black[i], a), a
	}
	return img
}
