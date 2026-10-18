package app

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

type targetHarness struct {
	target   clickTarget
	router   input.Router
	ops      op.Ops
	disabled bool
	selected bool
	state    targetState
	clicks   []widget.Click
	owner    bool
}

func (h *targetHarness) frame(events ...event.Event) {
	h.router.Queue(events...)
	h.ops.Reset()
	gtx := layout.Context{Ops: &h.ops, Source: h.router.Source(), Now: time.Unix(1000, 0), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(200, 100))}
	event.Op(gtx.Ops, &h.owner)
	for {
		_, ok := gtx.Event(key.FocusFilter{Target: &h.owner})
		if !ok {
			break
		}
	}
	if !gtx.Focused(&h.owner) {
		gtx.Execute(key.FocusCmd{Tag: &h.owner})
	}
	if h.disabled {
		gtx = gtx.Disabled()
	}
	for {
		click, ok := h.target.Update(gtx)
		if !ok {
			break
		}
		h.clicks = append(h.clicks, click)
	}
	h.target.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		h.state = h.target.state(gtx, h.selected)
		return layout.Dimensions{Size: image.Pt(100, 30)}
	})
	h.router.Frame(&h.ops)
}

func newTargetHarness() *targetHarness {
	h := new(targetHarness)
	h.frame()
	h.frame()
	return h
}

func TestClickTargetTracksPointerWithoutChangingKeyboardFocus(t *testing.T) {
	h := newTargetHarness()
	h.selected = true
	h.frame(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(98, 28)})
	h.frame()
	if !h.state.Enabled || !h.state.Hovered || !h.state.Selected || h.state.Pressed || !h.router.Source().Focused(&h.owner) {
		t.Fatal("hover changed focus/selection or missed padded hit area", h.state)
	}
	h.frame(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(98, 28)})
	h.frame()
	if !h.state.Pressed || len(h.clicks) != 0 {
		t.Fatal("press activated early or lacked pressed state")
	}
	h.frame(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(98, 28)})
	h.frame()
	if h.state.Pressed || len(h.clicks) != 1 {
		t.Fatal("release did not activate exactly once")
	}
	h.frame(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(110, 40)})
	h.frame()
	if h.state.Hovered || !h.state.Selected {
		t.Fatal("leaving hit area lost selection or retained hover")
	}
}

func TestClickTargetRetainsModifiersAndClickCounts(t *testing.T) {
	h := newTargetHarness()
	for range 2 {
		h.frame(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 15), Modifiers: key.ModCtrl | key.ModShift},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(20, 15), Modifiers: key.ModCtrl | key.ModShift})
		h.frame()
	}
	if len(h.clicks) != 2 {
		t.Fatal("lost click sequence", h.clicks)
	}
	for i, c := range h.clicks {
		if c.NumClicks != i+1 || c.Modifiers != key.ModCtrl|key.ModShift {
			t.Fatal("click metadata changed", h.clicks)
		}
	}
}

func TestClickTargetReleaseOutsideAndCancellationDoNotActivate(t *testing.T) {
	for _, end := range []pointer.Event{
		{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(120, 40)},
		{Kind: pointer.Cancel, Source: pointer.Mouse},
	} {
		h := newTargetHarness()
		h.frame(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 15)})
		if end.Kind == pointer.Release {
			h.frame(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end.Position})
		}
		h.frame(end)
		h.frame()
		if len(h.clicks) != 0 || h.state.Pressed {
			t.Fatal("cancelled/outside release activated target", h.state)
		}
	}
}

func TestClickTargetDisablingDropsPendingActivation(t *testing.T) {
	h := newTargetHarness()
	h.frame(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 15)})
	h.disabled = true
	h.selected = true
	h.frame(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(20, 15)})
	h.target.Click()
	h.frame()
	if h.state.Enabled || h.state.Hovered || h.state.Pressed || !h.state.Selected || len(h.clicks) != 0 {
		t.Fatal("disabled target retained active input state", h.state)
	}
	h.disabled = false
	h.frame()
	h.frame()
	if len(h.clicks) != 0 {
		t.Fatal("pending click escaped when re-enabled")
	}
	h.frame(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 15)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(20, 15)})
	h.frame()
	if len(h.clicks) != 1 {
		t.Fatal("re-enabled target rejected fresh click")
	}
}

func TestFileHoverRetainsBrowsingSelectionAndFocus(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	active, selected := v.active, v.panels[1].selected
	height := v.panels[1].rows[3].size.Y
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(300, float32(68+3*height+height/2))})
	state := v.panels[1].rows[3].state(layout.Context{Source: router.Source()}, false)
	if !state.Hovered || v.active != active || v.panels[1].selected != selected || !router.Source().Focused(&v.focus) {
		t.Fatal("file hover changed browsing context or was not tracked")
	}
}
