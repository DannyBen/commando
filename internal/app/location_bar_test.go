package app

import (
	"image"
	"maps"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/unit"
)

func TestLocationBarClicksFocusThenEdit(t *testing.T) {
	v := selectionView()
	v.panels[1].marks = map[int]bool{2: true, 4: true}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	p := &v.panels[1]
	selected, position, marks := p.selected, p.list.Position, maps.Clone(p.marks)
	point := f32.Pt(450, 18)
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: point})
	if !p.locationBar.clickable.Hovered() {
		t.Fatal("location bar did not hover")
	}
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	if !p.locationBar.clickable.Pressed() || v.active != 0 {
		t.Fatal("press did not show feedback or activated before release")
	}
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point, Time: time.Second + time.Millisecond})
	if v.active != 1 || v.locationEditing() || !router.Source().Focused(&v.focus) {
		t.Fatal("single click did not focus panel")
	}
	if p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position.First != position.First || p.list.Position.Offset != position.Offset {
		t.Fatal("location click changed selection, marks or viewport")
	}
	rowClick(send, point, time.Second+100*time.Millisecond, 0)
	if !v.locationEditing() || v.location.Text() != p.locationPath() || v.location.SelectedText() != p.locationPath() || !router.Source().Focused(&v.location) {
		t.Fatal("double click did not open the existing path editor")
	}
	rowClick(send, point, 2*time.Second, 0)
	if !v.locationEditing() || v.location.SelectedText() != "" {
		t.Fatal("click in open editor did not place native caret")
	}
	send(pressed(key.NameEscape))
	if v.locationEditing() || v.active != 1 || !router.Source().Focused(&v.focus) || p.selected != selected || !maps.Equal(p.marks, marks) {
		t.Fatal("path cancellation did not restore browsing state and focus")
	}
}

func TestLocationBarHonorsEditorCancellationAndPendingMutation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		v := selectionView()
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		s := v.nameInteraction()
		if s == nil {
			t.Fatal("missing name editor")
		}
		s.pending = pending
		rowClick(send, f32.Pt(450, 18), time.Second, 0)
		if pending {
			if v.active != 0 || v.nameInteraction() != s {
				t.Fatal("header click interrupted submitted name mutation")
			}
		} else if v.active != 1 || v.nameInteraction() != nil || v.panels[0].selected != 1 {
			t.Fatal("header click did not use normal editor cancellation")
		}
	}
}

func TestLocationBarCannotClickThroughPalette(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	rowClick(send, f32.Pt(450, 18), time.Second, 0)
	send()
	if v.paletteInteraction() != nil || v.active != 0 || v.locationEditing() {
		t.Fatal("palette dismissal clicked through to location bar")
	}
	rowClick(send, f32.Pt(450, 18), 2*time.Second, 0)
	if v.active != 1 || v.locationEditing() {
		t.Fatal("fresh header click failed after palette dismissal")
	}
}
