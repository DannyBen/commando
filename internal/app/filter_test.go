package app

import (
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestFilterOpeningSlashNeverSurvivesAFrame(t *testing.T) {
	for _, query := range []string{"", "go"} {
		for _, sameFrame := range []bool{true, false} {
			v := newPreviewView()
			v.panels[0].setFilter(query)
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
					Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))})
				router.Frame(&ops)
			}
			frame()
			frame()
			router.Queue(pressed("/"))
			if !sameFrame {
				frame()
				if v.filterInteraction() != nil {
					t.Fatal("focus moved before the opening slash text was consumed")
				}
			}
			router.Queue(key.EditEvent{Range: key.Range{End: len(query)}, Text: "/"})
			frame()
			if v.filterEditor.Text() != query || v.panels[0].filter != query || v.filterEditor.SelectedText() != query {
				t.Fatalf("query %q, same frame %v: opening slash changed rendered text/selection to %q/%q", query, sameFrame, v.filterEditor.Text(), v.filterEditor.SelectedText())
			}
		}
	}
}

func TestFilterFromPalette(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(replacePalette(v, "Filter"), pressed(key.NameReturn))
	if v.filterInteraction() == nil || v.filterEditor.Text() != "" {
		t.Fatal("palette did not open the filter without an invocation text event")
	}
	send(key.EditEvent{Text: "go"})
	if v.panels[0].filter != "go" {
		t.Fatal("palette filter did not accept its first text input")
	}
}

func TestFilterEnterSound(t *testing.T) {
	for _, name := range []key.Name{key.NameReturn, key.NameEnter} {
		for _, enabled := range []bool{true, false} {
			v := newPreviewView()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundEnabled, settings.Value{Bool: enabled})
			recorded := &recordedSounds{}
			v.sounds = recorded
			send, _ := panelInputFrames(v)
			send(pressed("/"), key.EditEvent{Text: "/"})
			send(key.EditEvent{Text: "go"})
			selected, path := v.panels[0].selected, v.panels[0].path
			recorded.played = nil
			send(pressed(name))
			if !v.interaction.idle() || v.panels[0].filter != "go" || v.panels[0].selected != selected || v.panels[0].path != path {
				t.Fatal("Enter did not retain the filtered listing and return to browsing")
			}
			send(key.Event{Name: name, State: key.Release})
			var want []sound.Name
			if enabled {
				want = []sound.Name{sound.Click}
			}
			if !slices.Equal(recorded.played, want) {
				t.Fatalf("key %s, sounds enabled %v: got %v, want %v", name, enabled, recorded.played, want)
			}
		}
	}
}

func TestFilterCachedProjectionAndMarks(t *testing.T) {
	v, reads := visibilityView(t)
	p := &v.panels[0]
	p.markAll()
	p.selected = len(p.items) - 1
	before := reads.Load()
	p.setFilter("VISIBLE")
	if !slices.Equal(visibleNames(p), []string{"..", "visible-dir", "visible.txt"}) || reads.Load() != before {
		t.Fatal("filter did not match substrings in cached files and folders")
	}
	if p.items[p.selected].name != "visible.txt" || len(p.marks) != 2 || len(p.selectedPaths()) != 2 {
		t.Fatal("filter lost current item or retained hidden marks")
	}
	p.setFilter("no matches")
	if !slices.Equal(visibleNames(p), []string{".."}) || len(p.marks) != 0 {
		t.Fatal("unmatched query lost parent or retained marks")
	}
	p.setFilter("")
	if len(p.items) != 8 || len(p.marks) != 0 {
		t.Fatal("clearing filter lost cached entries or resurrected marks")
	}
}

func TestFilterEditorAndPanelFocus(t *testing.T) {
	v, _ := visibilityView(t)
	send, _ := panelInputFrames(v)
	send(pressed("/"), key.EditEvent{Text: "/"})
	if v.filterInteraction() == nil || v.filterEditor.Text() != "" {
		t.Fatal("opening slash entered the query")
	}
	send(key.EditEvent{Text: "visible"})
	if v.panels[0].filter != "visible" || len(v.panels[0].items) != 3 {
		t.Fatal("editor did not filter immediately")
	}
	send(pressed(key.NameDownArrow))
	if v.panels[0].selected != 2 || v.filterInteraction() == nil {
		t.Fatal("arrow did not navigate matches while retaining editor")
	}
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || v.panels[0].filter != "visible" {
		t.Fatal("Enter did not finish editing without opening a folder")
	}
	send(pressed(key.NameTab))
	send(key.Event{Name: key.NameTab, State: key.Press, Modifiers: key.ModShift})
	if v.active != 0 || !v.interaction.idle() || v.panels[0].filter != "visible" {
		t.Fatal("returning to panel did not retain filter and browsing focus")
	}
	send(pressed("/"), key.EditEvent{Text: "/"})
	if v.filterEditor.SelectedText() != "visible" {
		t.Fatal("reopening filter did not select its existing query")
	}
	send(key.EditEvent{Range: key.Range{End: len("visible")}, Text: "hidden"})
	if v.panels[0].filter != "hidden" {
		t.Fatal("reopening editor did not select existing query for replacement")
	}
	send(pressed(key.NameTab))
	if v.active != 1 || !v.interaction.idle() || v.panels[0].filter != "hidden" {
		t.Fatal("switching during editing lost query or kept editing focus")
	}
	send(key.Event{Name: key.NameTab, State: key.Press, Modifiers: key.ModShift})
	send(pressed(key.NameEscape))
	if v.panels[0].filter != "" || len(v.panels[0].items) != 8 {
		t.Fatal("Escape did not restore full listing")
	}
	send(pressed("/"), key.EditEvent{Text: "/"})
	send(key.EditEvent{Text: "txt"})
	send(pressed(key.NameEscape))
	if !v.interaction.idle() || v.panels[0].filter != "" {
		t.Fatal("Escape in editor did not clear filter and finish session")
	}
}

func TestBackspaceClearsListFilterBeforeParentNavigation(t *testing.T) {
	for _, query := range []string{"visible", "no matches"} {
		t.Run(query, func(t *testing.T) {
			v, reads := visibilityView(t)
			p := &v.panels[0]
			root := p.path
			v.panels[1].setFilter("txt")
			send, _ := panelInputFrames(v)
			send(key.EditEvent{Text: "/"})
			send(key.EditEvent{Text: query + "x"})
			send(key.SelectionEvent{Start: len(query) + 1, End: len(query) + 1})
			send(pressed(key.NameDeleteBackward))
			if v.filterInteraction() == nil || v.filterEditor.Text() != query || p.filter != query || p.path != root {
				t.Fatalf("Backspace did not edit the filter text: session=%T editor=%q filter=%q path=%q root=%q", v.interaction.current, v.filterEditor.Text(), p.filter, p.path, root)
			}
			send(pressed(key.NameReturn))
			if !v.interaction.idle() || p.filter != query {
				t.Fatal("Enter did not return to the filtered list")
			}
			p.markAll()
			marks := p.selectedNames()
			if query == "visible" {
				send(key.EditEvent{Text: "v"})
				send(pressed(key.NameDeleteBackward))
				if p.prefix != "" || p.filter != query || p.path != root || p.loading {
					t.Fatal("Backspace did not clear the active typing prefix before the filter")
				}
			}
			selected, before := p.items[p.selected].name, reads.Load()
			send(pressed(key.NameDeleteBackward))
			if p.filter != "" || p.path != root || p.loading || reads.Load() != before || len(p.items) != 8 {
				t.Fatal("Backspace navigated or reread instead of clearing the cached filter")
			}
			if p.items[p.selected].name != selected || !slices.Equal(p.selectedNames(), marks) || v.panels[1].filter != "txt" || v.active != 0 {
				t.Fatal("clearing filter changed the current entry, marks, or another panel")
			}
			send(pressed(key.NameDeleteBackward))
			waitLoaded(t, v, 0)
			if p.path != filepath.Dir(root) || reads.Load() != before+1 {
				t.Fatal("second Backspace did not navigate to the parent")
			}
		})
	}
}

func TestBackspaceClearsArchiveListFilterBeforeLeavingArchive(t *testing.T) {
	v, archive := archiveView(t)
	p := &v.panels[0]
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "/"})
	send(key.EditEvent{Text: "a"}, pressed(key.NameReturn))
	selectName(t, p, "a2.txt")
	p.toggleMark(p.selected)
	send(pressed(key.NameDeleteBackward))
	if p.filter != "" || p.path != archive || p.archiveFolder != "." || p.loading || p.items[p.selected].name != "a2.txt" || !p.marks[p.selected] {
		t.Fatal("Backspace left the archive or lost selection/marks instead of clearing its filter")
	}
	send(pressed(key.NameDeleteBackward))
	waitLoaded(t, v, 0)
	if p.path != filepath.Dir(archive) || p.archiveFolder != "" {
		t.Fatal("second Backspace did not leave the archive")
	}
}

func TestFilterPreview(t *testing.T) {
	path := os.Getenv("COMMANDO_PREVIEW_FILTER")
	if path == "" {
		t.Skip("optional headless filter preview")
	}
	v := newPreviewView()
	v.panels[0].setFilter("go")
	v.panels[1].setFilter("zip")
	if err := renderViewPNG(path, v); err != nil {
		t.Fatal(err)
	}
}

func TestFilterRefreshAndNavigation(t *testing.T) {
	v, _ := visibilityView(t)
	p := &v.panels[0]
	p.setFilter("txt")
	v.refreshPanel()
	waitLoaded(t, v, 0)
	if p.filter != "txt" || len(p.items) != 3 {
		t.Fatal("refresh cleared filter")
	}
	p.reconcileDirectoryListing(browser.Directory{Path: p.path, Entries: p.entries})
	if p.filter != "txt" || len(p.items) != 3 {
		t.Fatal("reconciliation cleared filter")
	}
	v.load(0, p.path+"/child", "")
	waitLoaded(t, v, 0)
	if p.filter != "" || len(p.items) != 8 {
		t.Fatal("navigation retained previous folder filter")
	}
}
