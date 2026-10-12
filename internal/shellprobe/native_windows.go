package shellprobe

import (
	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"github.com/dannyben/commando/internal/windowshell"
)

type nativeBridge struct{ host *windowshell.Host }

func nativeHandle(e event.Event) uintptr {
	if e, ok := e.(gioapp.Win32ViewEvent); ok {
		return e.HWND
	}
	return 0
}

func (b *nativeBridge) perform(w *gioapp.Window, hwnd uintptr, req request) (r windowshell.Result, err error) {
	// Caller is a worker goroutine. The Gio event consumer must continue
	// draining events while Windows pumps nested messages in its modal loop.
	w.Run(func() {
		if b.host == nil {
			b.host, err = windowshell.NewHost(hwnd)
			if err != nil {
				return
			}
		}
		if req.kind == "drag" {
			r, err = b.host.Drag(req.paths)
		} else {
			r, err = b.host.ContextMenu(req.paths, req.extended)
		}
	})
	return r, err
}
