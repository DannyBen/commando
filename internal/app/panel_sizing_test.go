package app

import (
	"math"
	"os"
	"reflect"
	"testing"

	"gioui.org/io/key"
)

func TestPanelSizingInputAndLimits(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	selected, marks, position := v.panels[0].selected, v.panels[0].marks, v.panels[0].list.Position
	for _, tc := range []struct {
		name key.Name
		mods key.Modifiers
		text string
		want float64
	}{
		{"+", key.ModShift, "+", 1.2}, // Main keyboard: symbol key plus text.
		{"+", 0, "=", 1.5},            // Windows reports OEM_PLUS even without Shift.
		{"+", 0, "+", 2},              // Numpad.
		{"+", 0, "+", 2},
		{"-", 0, "-", 1.5},
		{"-", 0, "-", 1.2},
		{"0", key.ModCtrl, "", 1},
		{"-", 0, "-", 0.8},
		{"-", 0, "-", 0.6},
		{"-", 0, "-", 0.6},
		{"0", key.ModCtrl, "", 1},
	} {
		send(modified(tc.name, tc.mods))
		if tc.text != "" {
			send(key.EditEvent{Text: tc.text})
		}
		send(key.Event{Name: tc.name, Modifiers: tc.mods, State: key.Release})
		if v.panels[0].sizeRatio != tc.want || v.panels[0].prefix != "" || v.panels[1].sizeRatio != 1 {
			t.Fatalf("%s/%q: ratios %g/%g, want %g/1", tc.name, tc.text, v.panels[0].sizeRatio, v.panels[1].sizeRatio, tc.want)
		}
	}
	if v.panels[0].selected != selected || !reflect.DeepEqual(v.panels[0].marks, marks) || v.panels[0].list.Position != position {
		t.Fatal("resizing changed selection, marks or viewport")
	}
	// Linux and other backends may report the equals key by its own name.
	send(pressed("="), key.EditEvent{Text: "="})
	if v.panels[0].sizeRatio != 1.2 {
		t.Fatal("equals did not increase exactly once")
	}
}

func TestPanelSizingLayoutAndLifecycle(t *testing.T) {
	v := newPreviewView()
	v.panels[0].sizeRatio = 2
	v.panels[1].sizeRatio = 1
	v.panels[2].sizeRatio = 1
	send, _ := panelInputFrames(v)
	padding := int(v.appearance.ContentPaddingHorizontal)
	// 800 pixels minus two dividers: 399 + 200 + 199 pixels.
	for i, want := range []int{padding, 400 + padding, 601 + padding} {
		if v.panels[i].selectionAnchor.X != want {
			t.Fatalf("panel %d native menu anchor: got %d, want %d", i, v.panels[i].selectionAnchor.X, want)
		}
	}
	if path := os.Getenv("COMMANDO_PREVIEW_PANEL_SIZING"); path != "" {
		if err := renderViewPNG(path, v); err != nil {
			t.Fatal(err)
		}
	}
	send(pressed(key.NameRightArrow))
	if v.panels[0].sizeRatio != 2 || v.panels[1].sizeRatio != 1 {
		t.Fatal("ordinary focus navigation moved the size")
	}
	send(pressed(key.NameLeftArrow), modified(key.NameRightArrow, key.ModShift))
	if v.active != 1 || v.panels[1].sizeRatio != 2 || v.panels[0].sizeRatio != 1 {
		t.Fatal("Shift move left the size behind")
	}
	v.addPanel(1)
	if v.panels[v.active].sizeRatio != 2 {
		t.Fatal("clone lost its source size")
	}
	v.closePanel()
	v.reopenPanel()
	if v.panels[v.active].sizeRatio != 2 {
		t.Fatal("close/reopen lost the size")
	}
}

func TestPanelSizingPersistence(t *testing.T) {
	v := newPreviewView()
	v.panels[0].sizeRatio = 1.5
	state := v.sessionState(windowState{})
	dir := t.TempDir()
	if err := saveState(dir, state); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	locations, err := startingLocations([]string{t.TempDir()}, saved.Panels)
	if err != nil || locations[0].SizeRatio != 1.5 {
		t.Fatal("startup paths discarded saved size", err)
	}
	restored := newLocationView(locations, 0, nil)
	defer restored.loader.close()
	if restored.panels[0].sizeRatio != 1.5 {
		t.Fatal("startup did not restore size")
	}
	for _, invalid := range []float64{0, -1, 1.1, 100, math.NaN(), math.Inf(1)} {
		if panelSizeRatio(invalid) != 1 {
			t.Fatalf("invalid size %g did not default to normal", invalid)
		}
	}
}

func TestPanelSizingCommandsAndEditorIsolation(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "panel.increase_size", 0, "Ctrl+I")
	setTestShortcut(t, v, "panel.increase_size", 1, "none")
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "+"})
	if v.panels[0].sizeRatio != 1 {
		t.Fatal("replaced shortcut still resized")
	}
	send(modified("I", key.ModCtrl))
	if v.panels[0].sizeRatio != 1.2 {
		t.Fatal("replacement shortcut did not resize")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Panel Reset size"}, pressed(key.NameReturn))
	if v.panels[0].sizeRatio != 1 || !v.interaction.idle() {
		t.Fatal("palette did not reset the originating panel")
	}
	send(modified("D", key.ModAlt))
	send(key.EditEvent{Text: "=-+"}, modified("I", key.ModCtrl), modified("0", key.ModCtrl))
	if v.panels[0].sizeRatio != 1 || !v.locationEditing() {
		t.Fatal("sizing escaped the location editor")
	}
	v.panels[0].loading = true
	v.resizePanel(1)
	if v.panels[0].sizeRatio != 1 {
		t.Fatal("busy panel resized")
	}
}
