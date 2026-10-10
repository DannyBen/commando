//go:build !windows

package app

import (
	gioapp "gioui.org/app"
	"gioui.org/io/event"
)

func configureNativeWindow(*gioapp.Window, event.Event) {}
