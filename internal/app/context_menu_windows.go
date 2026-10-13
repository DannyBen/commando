package app

import (
	"fmt"
	"time"

	gioapp "gioui.org/app"
	"github.com/dannyben/commando/internal/windowshell"
)

func (n *nativeWindow) menuExecutor(w *gioapp.Window) func(menuRequest) menuCompletion {
	return func(request menuRequest) menuCompletion {
		var result menuCompletion
		w.Run(func() {
			result.queue = time.Since(request.queued)
			if n.shell == nil {
				result.err = fmt.Errorf("native shell unavailable: %w", n.shellErr)
				return
			}
			result.result, result.err = n.shell.ContextMenu(request.paths, request.options)
		})
		return result
	}
}

func (n *nativeWindow) initializeShell(hwnd uintptr) {
	n.shell, n.shellErr = windowshell.NewHost(hwnd)
}
