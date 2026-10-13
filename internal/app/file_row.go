package app

import (
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"image"
)

// File rows share ordinary clicks and secondary-button invocation. Passing
// pointer events through keeps list scrolling and primary clicks independent.
type fileRow struct {
	widget.Clickable
	secondary bool
	pressed   bool
	size      image.Point
}

func (r *fileRow) Layout(gtx layout.Context, contents layout.Widget) layout.Dimensions {
	dims := r.Clickable.Layout(gtx, contents)
	r.size = dims.Size
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &r.secondary)
	pass.Pop()
	area.Pop()
	return dims
}

func (r *fileRow) contextClicked(gtx layout.Context) bool {
	clicked := false
	for {
		e, ok := gtx.Event(pointer.Filter{Target: &r.secondary, Kinds: pointer.Press | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		pe := e.(pointer.Event)
		switch pe.Kind {
		case pointer.Press:
			r.pressed = pe.Source == pointer.Mouse && pe.Buttons == pointer.ButtonSecondary
		case pointer.Release:
			clicked = clicked || r.pressed && pe.Position.Round().In(image.Rectangle{Max: r.size})
			r.pressed = false
		case pointer.Cancel:
			r.pressed = false
		}
	}
	return clicked
}
