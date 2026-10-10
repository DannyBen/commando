package app

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// Exercise real Gio routing and lists: changing panels must preserve their
// independent selection/scroll state, and keyboard selection must stay visible.
func TestWorkspaceKeyboardNavigation(t *testing.T) {
	v := newView()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{
			Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(960, 420)),
		})
		router.Frame(&ops)
	}
	frame()
	frame()
	press := func(name key.Name) {
		t.Helper()
		router.Queue(key.Event{Name: name, State: key.Press})
		frame()
		frame()
	}
	press(key.NameDownArrow)
	if v.panels[0].selected != 4 {
		t.Fatalf("Down selected %d, want 4", v.panels[0].selected)
	}
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Release})
	frame()
	if v.panels[0].selected != 4 {
		t.Fatal("key release advanced the selection")
	}
	press(key.NameRightArrow)
	if v.active != 1 || v.panels[1].selected != 5 {
		t.Fatal("Right did not switch to the second panel's existing selection")
	}
	press(key.NameEnd)
	p := &v.panels[1]
	if p.selected != len(p.items)-1 || p.list.Position.First == 0 {
		t.Fatalf("End did not reveal the last entry: selected=%d, position=%+v", p.selected, p.list.Position)
	}
	position := p.list.Position
	press(key.NameRightArrow)
	press(key.NameRightArrow)
	if v.active != 2 {
		t.Fatal("Right crossed the last panel")
	}
	press(key.NameDownArrow)
	press(key.NameLeftArrow)
	if v.active != 1 || p.selected != len(p.items)-1 || p.list.Position != position {
		t.Fatal("returning to the second panel changed its selection or scroll position")
	}
	press(key.NamePageUp)
	if p.selected >= len(p.items)-1 {
		t.Fatal("Page Up did not navigate the list")
	}
	press(key.NameHome)
	press(key.NameUpArrow)
	if p.selected != 0 || p.list.Position.First != 0 {
		t.Fatal("Home/Up did not remain at the first entry")
	}
	press(key.NameLeftArrow)
	press(key.NameLeftArrow)
	if v.active != 0 || v.panels[0].selected != 4 || v.panels[2].selected != 5 {
		t.Fatal("panel navigation lost an independent selection or crossed the first panel")
	}
	v.panels[2].rows[7].Click()
	frame()
	frame()
	press(key.NameDownArrow)
	if v.active != 2 || v.panels[2].selected != 8 {
		t.Fatal("keyboard navigation after a row click did not operate in the clicked panel")
	}
}
