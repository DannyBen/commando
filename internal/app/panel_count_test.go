package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func TestPanelAddCloseReopenAndFocus(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	p.metadata, p.visibility, p.sortByMetadata = metadataModified, visibilityHideHidden, true
	p.list.Position.First, p.list.Position.Offset = 2, 7
	selected, items, position := p.selected, slices.Clone(p.items), p.list.Position
	v.cutClipboard = browser.FileClipboard{Move: true, Sequence: 7, Paths: []string{"backing/file"}}
	v.cutPanelPaths = []string{p.path, "second", "third"}
	v.cutLocations = []string{"backing", "second-backing", "third-backing"}
	// Exercise routing without rendering away the deliberately chosen viewport.
	v.browseKey(layout.Context{}, modified(key.NameRightArrow, key.ModAlt))
	if len(v.panels) != 4 || v.active != 1 {
		t.Fatal("Add right did not insert/focus beside the source")
	}
	clone, source := &v.panels[1], &v.panels[0]
	if clone.path != source.path || clone.selected != selected || clone.list.Position != position || len(clone.marks) != 0 ||
		clone.metadata != metadataModified || clone.visibility != visibilityHideHidden || !clone.sortByMetadata ||
		!slices.Equal(source.items, items) || source.list.Position != position || len(source.marks) != 3 ||
		!slices.Equal(v.cutLocations, []string{"backing", "backing", "second-backing", "third-backing"}) {
		t.Fatal("clone lost preferences/viewport or changed source/clipboard aliases")
	}
	clone.items[1].name = "independent"
	clone.ensureEntries()
	clone.entries[1].Name = "independent"
	clone.toggleMark(2)
	if source.items[1].name == "independent" {
		t.Fatal("cloned listing shares mutable storage")
	}
	clone.items[1].name = clone.items[1].file.Name
	want := *clone
	v.browseKey(layout.Context{}, modified("W", key.ModCtrl))
	if len(v.panels) != 3 || v.active != 1 || v.panels[1].path == source.path || len(v.closedPanels) != 1 {
		t.Fatal("close did not focus the right successor")
	}
	v.browseKey(layout.Context{}, modified("W", key.ModCtrl|key.ModShift))
	if len(v.panels) != 4 || v.active != 1 || len(v.closedPanels) != 0 || !reflect.DeepEqual(v.panels[1], want) || v.cutLocations[1] != "backing" {
		t.Fatal("reopen lost the complete pane state or original position")
	}
	send, _ := panelInputFrames(v)
	send(modified("4", key.ModAlt))
	if v.active != 3 {
		t.Fatal("Alt+number did not focus its header number")
	}
	send(modified("9", key.ModAlt))
	if v.active != 3 {
		t.Fatal("missing number changed focus")
	}
	send(key.EditEvent{Text: "2026"})
	if v.active != 3 || v.panels[3].prefix != "2026" {
		t.Fatal("bare digits no longer type filename prefixes")
	}
	send(modified(key.NameLeftArrow, key.ModAlt))
	if len(v.panels) != 5 || v.active != 3 {
		t.Fatal("Add left did not insert beside the active panel")
	}
}

func TestPanelCloseMinimumAndHistory(t *testing.T) {
	v := newPreviewView()
	v.active = 2
	v.closePanel()
	if v.active != 1 {
		t.Fatal("closing the rightmost panel did not focus left")
	}
	v.closePanel()
	v.closePanel()
	if len(v.panels) != 1 || len(v.closedPanels) != 2 {
		t.Fatal("closing the last panel changed the workspace")
	}
	v.reopenPanel()
	v.reopenPanel()
	v.reopenPanel()
	if len(v.panels) != 3 || v.active != 2 || len(v.closedPanels) != 0 {
		t.Fatal("closed panes did not reopen in reverse order")
	}
}

func TestPanelLifecycleBusyAndEditorGuards(t *testing.T) {
	for _, prepare := range []func(*view){
		func(v *view) { v.panels[1].loading = true }, func(v *view) { v.panels[1].refreshing = true },
		func(v *view) { v.transferPending = true }, func(v *view) { v.pendingNameDraft = &loadResult{} },
	} {
		v := newPreviewView()
		v.closePanel()
		before := len(v.panels)
		prepare(v)
		v.addPanel(1)
		v.closePanel()
		v.reopenPanel()
		if len(v.panels) != before || len(v.closedPanels) != 1 {
			t.Fatal("panel indices changed while captured work was pending")
		}
	}
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	text := v.location.Text()
	send(modified(key.NameRightArrow, key.ModAlt), modified("W", key.ModCtrl), modified("W", key.ModCtrl|key.ModShift), modified("2", key.ModAlt))
	if len(v.panels) != 3 || v.active != 0 || !v.locationEditing() || v.location.Text() != text {
		t.Fatal("panel shortcuts escaped an editor")
	}
	send(pressed(key.NameEscape), pressed("`"))
	send(key.EditEvent{Text: "Panel Add right"}, pressed(key.NameReturn))
	if len(v.panels) != 4 || v.active != 1 || !v.interaction.idle() {
		t.Fatal("palette did not dispatch panel addition through browsing")
	}
}

func TestAdjacentEdgeCreatesPanel(t *testing.T) {
	for _, direction := range []int{-1, 1} {
		t.Run(fmt.Sprint(direction), func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "child")
			if err := os.Mkdir(child, 0700); err != nil {
				t.Fatal(err)
			}
			v := newBrowserView([]string{root}, nil)
			defer v.loader.close()
			waitLoaded(t, v, 0)
			selectName(t, &v.panels[0], "child")
			v.panels[0].toggleMark(v.panels[0].selected)
			before := v.panels[0]
			send, _ := panelInputFrames(v)
			send(adjacentKey(direction))
			waitLoaded(t, v, v.active)
			source := 0
			if direction < 0 {
				source = 1
			}
			if len(v.panels) != 2 || v.panels[v.active].path != child || v.panels[source].path != root ||
				v.panels[source].selected != before.selected || !reflect.DeepEqual(v.panels[source].marks, before.marks) {
				t.Fatal("edge open did not create/focus a folder pane while preserving source")
			}
		})
	}
}

func TestReopenedPanelReconcilesClosedDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, root}, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	selectName(t, &v.panels[0], "keep.txt")
	v.panels[0].toggleMark(v.panels[0].selected)
	v.closePanel()
	if err := os.WriteFile(filepath.Join(root, "new.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	v.reopenPanel()
	v.updateAutoRefresh(layout.Context{})
	result := <-v.loader.results
	v.acceptLoad(result)
	p := &v.panels[v.active]
	if !slices.Contains(visibleNames(p), "new.txt") || p.items[p.selected].name != "keep.txt" || !p.marks[p.selected] {
		t.Fatal("reopen did not refresh changes while retaining surviving browsing state")
	}
}

func TestVariablePanelCountAndWidthPersistence(t *testing.T) {
	root := t.TempDir()
	saved := make([]panelState, 12)
	for i := range saved {
		saved[i] = panelState{Path: root, Metadata: "extension", SortByMetadata: i%2 == 0}
	}
	locations, err := startingLocations(nil, saved)
	if err != nil || !reflect.DeepEqual(locations, saved) {
		t.Fatal("startup did not retain the saved panel count")
	}
	v := newRestoredView(locations, 11, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	state := v.sessionState(windowState{})
	state.PanelWidth = 300
	dir := t.TempDir()
	if err := saveState(dir, state); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(dir)
	if err != nil || len(got.Panels) != 12 || got.ActivePanel != 11 || got.PanelWidth != 300 || !got.Panels[0].SortByMetadata {
		t.Fatal("workspace count/preferences/base width did not survive persistence")
	}
	locations, err = startingLocations([]string{root}, got.Panels)
	if err != nil || len(locations) != 12 {
		t.Fatal("explicit startup paths discarded the panel-count preference")
	}
}

func TestFullWorkerQueueDoesNotBlockOrLeaveBusyState(t *testing.T) {
	l := &directoryLoader{requests: make(chan loadRequest, 1)}
	l.requests <- loadRequest{}
	if l.submit(loadRequest{action: actionMutation}) || l.busy.Load() != 0 {
		t.Fatal("full worker queue accepted or retained a mutation")
	}
	v := newPreviewView()
	v.loader = l
	v.clipboardSelected(false)
	if v.panels[0].loading {
		t.Fatal("rejected clipboard capture left a pane busy")
	}
	v.pasteFiles()
	if v.panels[0].loading || v.transferPending {
		t.Fatal("rejected transfer left the workspace busy")
	}
}

func TestPanelCountPreview(t *testing.T) {
	path := os.Getenv("COMMANDO_PREVIEW_PANELS")
	if path == "" {
		t.Skip("optional headless panel-count preview")
	}
	v := newPreviewView()
	v.addPanel(1)
	v.addPanel(1)
	if err := renderViewPNG(path, v); err != nil {
		t.Fatal(err)
	}
}
