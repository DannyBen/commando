//go:build !windows

package shellprobe

import (
	"fmt"

	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"github.com/dannyben/commando/internal/windowshell"
)

type nativeBridge struct{}

func nativeHandle(event.Event) uintptr { return 0 }

func (*nativeBridge) perform(*gioapp.Window, uintptr, request) (windowshell.Result, error) {
	return windowshell.Result{}, fmt.Errorf("native shell tests require Windows")
}
