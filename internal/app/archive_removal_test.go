package app

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestArchiveDeleteUsesPermanentGuardAndCancellation(t *testing.T) {
	v, filename := archiveView(t)
	selectName(t, &v.panels[0], "a2.txt")
	before, _ := os.ReadFile(filename)
	v.loader.recycle = func(string, []string, uintptr) (browser.RemovalResult, error) {
		t.Error("ZIP deletion reached recycling")
		return browser.RemovalResult{}, nil
	}
	v.loader.delete = func(string, []string, uintptr) (browser.RemovalResult, error) {
		t.Error("ZIP deletion reached filesystem deletion")
		return browser.RemovalResult{}, nil
	}
	send, _ := panelInputFrames(v)
	for _, c := range v.commandCatalogue() {
		if c.id == "entry.recycle" && (c.label() != "Entry › Delete from archive…" || !c.allowed(v)) {
			t.Fatal("archive delete catalogue label/availability wrong", c)
		}
	}
	send(pressed(key.NameDeleteForward))
	q := v.confirmationInteraction()
	if q == nil || q.selected != 1 || q.acceptLabel != "Delete from archive" || v.panels[0].loading {
		t.Fatal("archive did not default permanent deletion to Cancel")
	}
	if filename := os.Getenv("COMMANDO_PREVIEW_ARCHIVE_DELETE"); filename != "" {
		if err := renderViewPNG(filename, v); err != nil {
			t.Fatal(err)
		}
	}
	send(pressed(key.NameDeleteForward)) // Invocation repeats cannot accept.
	send(pressed(key.NameReturn))        // Cancel.
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) || !v.interaction.idle() {
		t.Fatal("cancelled ZIP deletion changed archive")
	}
	send(key.Event{Name: key.NameDeleteForward, State: key.Release})
	send(modified(key.NameDeleteForward, key.ModShift))
	send(pressed(key.NameLeftArrow), pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if slices.Contains(visibleNames(&v.panels[0]), "a2.txt") {
		t.Fatal("confirmed archive delete failed")
	}
	if message := v.panels[0].feedback.Current(); message == nil || message.Kind != feedback.Success || message.Text != "Item deleted" {
		t.Fatal("archive deletion did not use common completion feedback", message)
	}
	if v.mutationsBusy() || v.active != 0 {
		t.Fatal("deletion retained busy state or moved focus")
	}
}

func TestArchiveDeleteRefreshesSameZIPAndRecoversRemovedFolders(t *testing.T) {
	v, filename := archiveView(t)
	root := filepath.Dir(filename)
	v.panels = append(v.panels, newPreviewView().panels[0])
	v.panels[2].path = root
	v.loadLocation(1, browser.Location{Path: filename, Folder: "docs/sub"}, "report.txt")
	waitLoaded(t, v, 1)
	v.load(2, root, "sample.zip")
	waitLoaded(t, v, 2)
	// A second ZIP root retains a distinct filter, mark and selection.
	v.panels = append(v.panels, newPreviewView().panels[0])
	v.loadLocation(3, browser.Location{Path: filename, Folder: "."}, "a10.txt")
	waitLoaded(t, v, 3)
	other := &v.panels[3]
	other.toggleMark(other.selected)
	other.filter = "a"
	other.refilter()
	marks, selected := maps.Clone(other.marks), other.items[other.selected].file.Name
	v.active = 0
	selectName(t, &v.panels[0], "docs")
	v.panels[0].toggleMark(v.panels[0].selected)
	selectName(t, &v.panels[0], "empty")
	v.panels[0].toggleMark(v.panels[0].selected)
	selectName(t, &v.panels[0], "a10.txt")
	for _, name := range []string{"single_files", "directories", "multiple_entries"} {
		setGuard(t, v, true, name, "OFF")
	}
	v.removeSelected(layout.Context{}, false)
	if !v.panels[0].loading || !v.mutationsBusy() {
		t.Fatal("archive write did not use shared worker/busy policy")
	}
	waitLoaded(t, v, 0)
	if !slices.Equal(visibleNames(&v.panels[0]), []string{"..", "a2.txt", "a10.txt"}) || v.panels[0].items[v.panels[0].selected].file.Name != "a10.txt" {
		t.Fatal("marked directories not removed independently of current row", visibleNames(&v.panels[0]))
	}
	if v.panels[1].archiveFolder != "." || len(v.panels[1].marks) != 0 {
		t.Fatal("panel inside deleted folder did not recover to ZIP root")
	}
	if other.filter != "a" || !reflect.DeepEqual(other.marks, marks) || other.items[other.selected].file.Name != selected {
		t.Fatal("sibling archive panel lost filter, marks or selection")
	}
	if v.panels[2].archiveFolder != "" || v.panels[2].items[v.panels[2].selected].file.Name != "sample.zip" {
		t.Fatal("container listing lost its archive selection")
	}
	if v.active != 0 || v.mutationsBusy() {
		t.Fatal("completion changed focus or retained busy state")
	}
}

func TestArchiveDeleteFailureRetainsListingAndMarks(t *testing.T) {
	v, filename := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "docs")
	p.toggleMark(p.selected)
	before, marks := slices.Clone(p.items), maps.Clone(p.marks)
	writePanelArchive(t, filename, "a2.txt") // Captured selection disappeared externally.
	for _, name := range []string{"single_files", "directories", "multiple_entries"} {
		setGuard(t, v, true, name, "OFF")
	}
	v.removeSelected(layout.Context{}, false)
	waitLoaded(t, v, 0)
	if !slices.Equal(before, p.items) || !reflect.DeepEqual(marks, p.marks) {
		t.Fatal("failed ZIP mutation discarded cached listing or marks")
	}
	if message := p.feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "Item not found" {
		t.Fatal("failed ZIP mutation lost specific error", message)
	}
	if v.mutationsBusy() {
		t.Fatal("failed mutation retained busy state")
	}
}

func TestArchiveRemovalStaleResultAndCaseSensitiveIdentity(t *testing.T) {
	v, filename := archiveView(t)
	writePanelArchive(t, filename, "A.txt", "a.txt", "folder/file.txt")
	v.refreshPanel()
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], "A.txt")
	for _, name := range []string{"single_files", "directories", "multiple_entries"} {
		setGuard(t, v, true, name, "OFF")
	}
	v.removeSelected(layout.Context{}, true)
	waitLoaded(t, v, 0)
	if slices.Contains(visibleNames(&v.panels[0]), "A.txt") || !slices.Contains(visibleNames(&v.panels[0]), "a.txt") {
		t.Fatal("deletion merged case-distinct ZIP names")
	}
	// Same archive path, different folder: the result must not replace this panel.
	p := &v.panels[0]
	before := slices.Clone(p.items)
	result := loadResult{request: loadRequest{removal: &removalRequest{archiveLocations: []browser.Location{{Path: filename, Folder: "folder"}}}}, panelUpdates: []transferPanelUpdate{{needed: true, directory: browser.Directory{Path: filename, ArchiveFolder: "folder"}}}}
	v.applyArchivePanels(result, result.request.removal.archiveLocations)
	if !slices.Equal(p.items, before) || p.archiveFolder != "." {
		t.Fatal("stale folder result replaced current ZIP location")
	}
}
