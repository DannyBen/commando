package app

import (
	"fmt"
	"time"

	gioapp "gioui.org/app"
	"github.com/dannyben/commando/internal/windowshell"
)

func (n *nativeWindow) shellExecutor(w *gioapp.Window) func(shellRequest) shellCompletion {
	return func(request shellRequest) shellCompletion {
		var result shellCompletion
		w.Run(func() {
			result.queue = time.Since(request.queued)
			if n.shell == nil {
				result.err = fmt.Errorf("native shell unavailable: %w", n.shellErr)
				return
			}
			switch request.action {
			case shellDrag:
				result.result, result.err = n.shell.Drag(request.paths, request.armed)
			default:
				result.result, result.err = n.shell.ContextMenu(request.paths, request.options)
			}
		})
		return result
	}
}

func (n *nativeWindow) initializeShell(hwnd uintptr) {
	n.shell, n.shellErr = windowshell.NewHost(hwnd)
}
