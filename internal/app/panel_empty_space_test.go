package app

import (
	"image"
	"maps"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/unit"
)

func TestEmptyPanelSpaceFocusPreservesBrowsingState(t *testing.T) {
	for _, empty := range []bool{false, true} {
		v := selectionView()
		if empty {
			v.panels[1].items = nil
			v.panels[1].selected = -1
		} else {
			v.panels[1].marks = map[int]bool{2: true, 4: true}
		}
		send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
		selected := v.panels[1].selected
		marks := maps.Clone(v.panels[1].marks)
		position := v.panels[1].list.Position
		rowClick(send, f32.Pt(450, 350), time.Second, 0)
		p := &v.panels[1]
		if v.active != 1 || p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position.First != position.First || p.list.Position.Offset != position.Offset || !router.Source().Focused(&v.focus) {
			t.Fatal("empty-space click lost focus, selection, marks or viewport")
		}
		rowClick(send, f32.Pt(450, 350), time.Second+100*time.Millisecond, 0)
		if p.selected != selected || !v.interaction.idle() {
			t.Fatal("double-clicking empty space activated an entry")
		}
		if !empty {
			send(pressed(key.NameDownArrow))
			if p.selected != selected+1 {
				t.Fatal("keyboard navigation did not follow clicked panel")
			}
		}
	}
}

func TestEmptyPanelSpaceHonorsEditorCancellationAndPendingMutation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		v := selectionView()
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		s := v.nameInteraction()
		if s == nil {
			t.Fatal("missing name editor")
		}
		s.pending = pending
		rowClick(send, f32.Pt(450, 350), time.Second, 0)
		if pending {
			if v.active != 0 || v.nameInteraction() != s {
				t.Fatal("empty click interrupted a submitted mutation")
			}
		} else if v.active != 1 || v.nameInteraction() != nil || v.panels[0].selected != 1 || v.panels[1].selected != 1 {
			t.Fatal("empty click did not cancel the editor and focus without selecting another entry")
		}
	}
}

func TestEmptyPanelSpaceDoesNotClickThroughPalette(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	rowClick(send, f32.Pt(450, 420), time.Second, 0)
	send()
	if v.paletteInteraction() != nil || v.active != 0 {
		t.Fatal("palette dismissal focused an underlying panel")
	}
}
