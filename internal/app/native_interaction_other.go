//go:build !windows

package app

import gioapp "gioui.org/app"

func (*nativeWindow) shellExecutor(*gioapp.Window) func(shellRequest) shellCompletion { return nil }
