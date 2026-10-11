//go:build !windows

package app

import (
	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/unit"
)

type nativeWindow struct {
	state *windowTracker
	keys  *nativeKeyInput
	mode  gioapp.WindowMode
}

func (n *nativeWindow) options() []gioapp.Option {
	saved := n.state.snapshot()
	if !saved.valid() {
		return nil
	}
	opts := []gioapp.Option{gioapp.Size(unit.Dp(max(720, saved.Width)), unit.Dp(max(360, saved.Height)))}
	if saved.Maximized {
		opts = append(opts, gioapp.Maximized.Option())
	}
	return opts
}

func (n *nativeWindow) configure(_ *gioapp.Window, e event.Event) {
	state := n.state.snapshot()
	switch e := e.(type) {
	case gioapp.ConfigEvent:
		n.mode = e.Config.Mode
		if n.mode != gioapp.Minimized {
			state.Maximized = n.mode == gioapp.Maximized
		}
	case gioapp.FrameEvent:
		if n.mode == gioapp.Windowed && e.Metric.PxPerDp > 0 {
			state.Width = int(float32(e.Size.X)/e.Metric.PxPerDp + 0.5)
			state.Height = int(float32(e.Size.Y)/e.Metric.PxPerDp + 0.5)
		}
	}
	n.state.set(state)
}
