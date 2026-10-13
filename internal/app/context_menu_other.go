//go:build !windows

package app

import gioapp "gioui.org/app"

func (*nativeWindow) menuExecutor(*gioapp.Window) func(menuRequest) menuCompletion { return nil }
