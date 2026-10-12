package app

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// overlay is a modal surface anchored below a caller-supplied top inset.
// It owns a pointer barrier over the whole workspace, without changing the
// geometry of the content underneath.
type overlay struct {
	barrier bool
	bounds  image.Rectangle
}

func overlayWidth(available, minimum, maximum int) int {
	return min(available, max(0, min(maximum, max(minimum, available*4/5))))
}

func (o *overlay) outsidePress(gtx layout.Context) bool {
	outside := false
	for {
		e, ok := gtx.Event(pointer.Filter{Target: &o.barrier, Kinds: pointer.Press | pointer.Scroll})
		if !ok {
			return outside
		}
		if e := e.(pointer.Event); e.Kind == pointer.Press && !image.Pt(int(e.Position.X), int(e.Position.Y)).In(o.bounds) {
			outside = true
		}
	}
}

func (o *overlay) layout(gtx layout.Context, top int, minimum, maximum unit.Dp, colors ThemeColors, backdropOpacity float32, content layout.Widget) layout.Dimensions {
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &o.barrier)
	area.Pop()
	width := overlayWidth(gtx.Constraints.Max.X, gtx.Dp(minimum), gtx.Dp(maximum))
	x := (gtx.Constraints.Max.X - width) / 2
	top = max(0, min(top, gtx.Constraints.Max.Y))
	backdrop := colors.Surface
	backdrop.A = uint8(float32(backdrop.A) * max(0, min(1, backdropOpacity)))
	paint.FillShape(gtx.Ops, backdrop, clip.Rect{Min: image.Pt(0, top), Max: gtx.Constraints.Max}.Op())
	position := op.Offset(image.Pt(x, top)).Push(gtx.Ops)
	gtx.Constraints.Min = image.Pt(width, 0)
	gtx.Constraints.Max.X = width
	gtx.Constraints.Max.Y -= top
	macro := op.Record(gtx.Ops)
	dims := widget.Border{Color: colors.Border, Width: 1}.Layout(gtx, content)
	call := macro.Stop()
	paint.FillShape(gtx.Ops, colors.Surface, clip.Rect{Max: dims.Size}.Op())
	call.Add(gtx.Ops)
	position.Pop()
	o.bounds = image.Rectangle{Min: image.Pt(x, top), Max: image.Pt(x+width, top+dims.Size.Y)}
	return dims
}
