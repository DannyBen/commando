package app

import (
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
)

// A value target adds wheel input to the shared click/hover/press surface.
// Its owner decides whether the displayed value can currently be stepped.
type valueTarget struct {
	clickTarget
	wheel bool
}

func (t *valueTarget) layout(gtx layout.Context, wheel bool, content layout.Widget) layout.Dimensions {
	dims := t.Layout(gtx, content)
	if wheel && t.enabled(gtx) {
		defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
		defer pointer.PassOp{}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, &t.wheel)
	}
	return dims
}

func (t *valueTarget) scroll(gtx layout.Context) (int, bool) {
	e, ok := gtx.Event(pointer.Filter{Target: &t.wheel, Kinds: pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30}})
	if !ok {
		return 0, false
	}
	pe := e.(pointer.Event)
	if !t.enabled(gtx) || pe.Modifiers != 0 || pe.Scroll.Y == 0 {
		return 0, true
	}
	if pe.Scroll.Y < 0 {
		return -1, true
	}
	return 1, true
}
