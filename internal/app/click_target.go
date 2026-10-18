package app

import (
	"image"

	"gioui.org/layout"
	"gioui.org/widget"
)

// clickTarget gives interactive surfaces one input/state contract. Gio owns
// gestures, click counts, modifiers and clipping to the measured hit area.
// Owners still decide selection, focus, availability and what a click does.
type clickTarget struct {
	clickable widget.Clickable
	size      image.Point
	disabled  bool
}

// targetState keeps keyboard selection separate from passive pointer state.
// A selected target can be disabled; disabled targets never hover or press.
type targetState struct {
	Enabled, Selected, Hovered, Pressed bool
}

func (t *clickTarget) enabled(gtx layout.Context) bool {
	return gtx.Enabled() && !t.disabled
}

func (t *clickTarget) state(gtx layout.Context, selected bool) targetState {
	enabled := t.enabled(gtx)
	return targetState{
		Enabled: enabled, Selected: selected,
		Hovered: enabled && t.clickable.Hovered(),
		Pressed: enabled && t.clickable.Pressed(),
	}
}

func (t *clickTarget) Layout(gtx layout.Context, contents layout.Widget) layout.Dimensions {
	t.disabled = !gtx.Enabled()
	dims := t.clickable.Layout(gtx, contents)
	t.size = dims.Size
	return dims
}

func (t *clickTarget) contains(point image.Point) bool {
	return point.In(image.Rectangle{Max: t.size})
}

func (t *clickTarget) Update(gtx layout.Context) (widget.Click, bool) {
	click, ok := t.clickable.Update(gtx)
	// Consume queued clicks even when the previous layout was disabled. They
	// must not activate an underlying control when its owner becomes active again.
	return click, ok && t.enabled(gtx)
}

func (t *clickTarget) Clicked(gtx layout.Context) bool {
	_, ok := t.Update(gtx)
	return ok
}

// Click supports programmatic activation through the same delivery policy.
func (t *clickTarget) Click() { t.clickable.Click() }
