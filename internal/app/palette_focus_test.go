package app

import (
	"image"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/unit"
)

func TestPaletteFocusEntriesFollowLivePanelCount(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	assertFocusEntries := func(count int) {
		t.Helper()
		send(pressed("`"))
		var names []string
		for _, result := range v.paletteInteraction().results {
			c := result.command
			if c.group == "Panel" && strings.HasPrefix(c.name, "Focus ") {
				number := strconv.Itoa(len(names) + 1)
				if c.shortcut() != "Alt+"+number {
					t.Fatalf("focus %s has wrong shortcut %q", number, c.shortcut())
				}
				names = append(names, c.label())
			}
		}
		want := make([]string, count)
		for i := range want {
			want[i] = "Panel › Focus " + strconv.Itoa(i+1)
		}
		if !slices.Equal(names, want) {
			t.Fatalf("focus entries %v, want %v", names, want)
		}
		send(pressed(key.NameEscape))
	}
	assertFocusEntries(3)
	send(modified(key.NameRightArrow, key.ModAlt))
	assertFocusEntries(4)
	send(modified("W", key.ModCtrl))
	assertFocusEntries(3)
	send(modified("W", key.ModCtrl|key.ModShift))
	assertFocusEntries(4)
}

func TestPaletteFocusExecutesEveryPanelAndRetainsBrowsing(t *testing.T) {
	v := newPreviewView()
	for len(v.panels) < 12 {
		v.addPanel(1)
	}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	for _, target := range []int{0, 8, 9, 11, 11} {
		selections := make([]int, len(v.panels))
		for i, p := range v.panels {
			selections[i] = p.selected
		}
		send(pressed("`"))
		send(key.EditEvent{Text: "Panel Focus " + strconv.Itoa(target+1)})
		result := v.paletteInteraction().results[0].command
		if result.name != "Focus "+strconv.Itoa(target+1) {
			t.Fatalf("search chose %s", result.label())
		}
		if target >= 9 && result.shortcut() != "" {
			t.Fatal("panel above nine displayed an unsupported shortcut")
		}
		send(pressed(key.NameReturn))
		if v.active != target || !v.interaction.idle() || !router.Source().Focused(&v.focus) {
			t.Fatal("focus action lost target, browsing focus, or palette dismissal")
		}
		for i, p := range v.panels {
			if p.selected != selections[i] {
				t.Fatal("focus action changed a panel's current row")
			}
		}
	}
}

func TestPaletteFocusPreview(t *testing.T) {
	path := os.Getenv("COMMANDO_PREVIEW_FOCUS")
	if path == "" {
		t.Skip("optional headless panel-focus discovery preview")
	}
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "focus"})
	if err := renderViewPNG(path, v); err != nil {
		t.Fatal(err)
	}
}
