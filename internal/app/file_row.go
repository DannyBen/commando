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
	clickTarget
	secondary     bool
	pressed       bool
	dragCandidate bool
	dragOrigin    image.Point
	suppressClick bool
}

func (r *fileRow) Layout(gtx layout.Context, contents layout.Widget) layout.Dimensions {
	dims := r.clickTarget.Layout(gtx, contents)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &r.secondary)
	pass.Pop()
	area.Pop()
	return dims
}

func (r *fileRow) updatePointer(gtx layout.Context) (clicked, dragged bool) {
	enabled := r.enabled(gtx)
	if !enabled {
		r.pressed, r.dragCandidate = false, false
	}
	for {
		e, ok := gtx.Event(pointer.Filter{Target: &r.secondary, Kinds: pointer.Press | pointer.Release | pointer.Cancel | pointer.Move | pointer.Drag})
		if !ok {
			break
		}
		if !enabled {
			continue
		}
		pe := e.(pointer.Event)
		switch pe.Kind {
		case pointer.Press:
			r.pressed = pe.Source == pointer.Mouse && pe.Buttons == pointer.ButtonSecondary
			r.dragCandidate = pe.Source == pointer.Mouse && pe.Buttons == pointer.ButtonPrimary
			r.dragOrigin = pe.Position.Round()
			r.suppressClick = false
		case pointer.Move, pointer.Drag:
			delta := pe.Position.Round().Sub(r.dragOrigin)
			threshold := gtx.Dp(6)
			if r.dragCandidate && pe.Buttons == pointer.ButtonPrimary && delta.X*delta.X+delta.Y*delta.Y >= threshold*threshold {
				dragged, r.dragCandidate, r.suppressClick = true, false, true
			}
		case pointer.Release:
			clicked = clicked || r.pressed && r.contains(pe.Position.Round())
			r.pressed, r.dragCandidate = false, false
		case pointer.Cancel:
			r.pressed, r.dragCandidate = false, false
		}
	}
	return
}

func (r *fileRow) Update(gtx layout.Context) (widget.Click, bool) {
	click, ok := r.clickTarget.Update(gtx)
	return click, ok && !r.suppressClick
}
