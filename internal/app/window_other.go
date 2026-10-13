//go:build !windows

package app

import (
	gioapp "gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/unit"
)

type nativeWindow struct {
	state         *windowTracker
	keys          *nativeKeyInput
	mode          gioapp.WindowMode
	width         int
	suppressWidth bool
}

func (n *nativeWindow) options() []gioapp.Option {
	saved := n.state.snapshot()
	if !saved.valid() {
		return nil
	}
	opts := []gioapp.Option{gioapp.Size(unit.Dp(max(minWindowWidth, saved.Width)), unit.Dp(max(360, saved.Height)))}
	if saved.Maximized {
		opts = append(opts, gioapp.Maximized.Option())
	}
	return opts
}

func (n *nativeWindow) configure(_ *gioapp.Window, e event.Event) {
	state := n.state.snapshot()
	switch e := e.(type) {
	case gioapp.ConfigEvent:
		if n.mode != e.Config.Mode {
			n.suppressWidth = true
		}
		n.mode = e.Config.Mode
		if n.mode != gioapp.Minimized {
			state.Maximized = n.mode == gioapp.Maximized
		}
	case gioapp.FrameEvent:
		if n.mode == gioapp.Windowed && e.Metric.PxPerDp > 0 {
			state.Width = int(float32(e.Size.X)/e.Metric.PxPerDp + 0.5)
			n.state.rememberPanelWidth(float64(state.Width), n.width != 0 && n.width != state.Width && !n.suppressWidth)
			n.width, n.suppressWidth = state.Width, false
			state.Height = int(float32(e.Size.Y)/e.Metric.PxPerDp + 0.5)
		}
	}
	n.state.set(state)
}

// Development hosts support resizing through Gio. Windows owns screen-bound
// placement and explicit manual-resize detection; headless previews bypass both.
func (n *nativeWindow) resizePanels(w *gioapp.Window, count int) {
	if !n.state.panelCount(count) || n.mode != gioapp.Windowed {
		return
	}
	sizing, state := n.state.sizingSnapshot(), n.state.snapshot()
	if sizing.baseWidth <= 0 || w == nil {
		return
	}
	n.suppressWidth = true
	w.Option(gioapp.Size(unit.Dp(max(minWindowWidth, sizing.width())), unit.Dp(max(360, state.Height))))
}

func (*nativeWindow) owner() uintptr { return 0 }
