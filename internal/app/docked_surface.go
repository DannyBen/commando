package app

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// A docked surface covers only its own bounds, leaving the workspace geometry,
// contrast and panel footers intact. Its pointer barrier never covers the panels.
type dockedSurface struct{ barrier bool }

func (s *dockedSurface) layout(gtx layout.Context, bottom, width, height int, colors ThemeColors, content layout.Widget) layout.Dimensions {
	for {
		_, ok := gtx.Event(pointer.Filter{Target: &s.barrier, Kinds: pointer.Press | pointer.Release | pointer.Scroll})
		if !ok {
			break
		}
	}
	width = min(gtx.Constraints.Max.X, max(0, width))
	height = min(max(0, gtx.Constraints.Max.Y-bottom), max(0, height))
	position := op.Offset(image.Pt((gtx.Constraints.Max.X-width)/2, max(0, gtx.Constraints.Max.Y-bottom-height))).Push(gtx.Ops)
	defer position.Pop()
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	defer bounds.Pop()
	event.Op(gtx.Ops, &s.barrier)
	paint.Fill(gtx.Ops, colors.Surface)
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	return widget.Border{Color: colors.Border, Width: 1}.Layout(gtx, content)
}
