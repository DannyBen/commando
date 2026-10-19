package app

import (
	"bytes"
	"maps"
	"os"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestArchiveRenameUsesF2AndPreservesIndependentMarksAndSelection(t *testing.T) {
	v, filename := archiveView(t)
	v.loadLocation(1, browser.Location{Path: filename, Folder: "."}, "a10.txt")
	waitLoaded(t, v, 1)
	other := &v.panels[1]
	other.toggleMark(other.selected)
	other.prefix = "a"
	p := &v.panels[0]
	selectName(t, p, "docs")
	p.toggleMark(p.selected)
	selectName(t, p, "a2.txt")
	p.toggleMark(p.selected)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	if n := v.nameInteraction(); n == nil || n.archiveFolder != "." || v.nameEditor.SelectedText() != "a2" {
		t.Fatal("ZIP F2 did not use ordinary basename selection")
	}
	send(pressed(key.NameTab))
	if v.active != 0 || v.nameEditor.SelectedText() != "a2" {
		t.Fatal("ZIP rename lost editor focus on Tab")
	}
	send(replaceRename(v, "zulu.md"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if v.nameInteraction() != nil || p.items[p.selected].file.Name != "zulu.md" || !p.marks[p.selected] {
		t.Fatal("ZIP rename lost current identity or mark")
	}
	selectName(t, p, "docs")
	if !p.marks[p.selected] {
		t.Fatal("renaming current file lost an unrelated mark")
	}
	if other.items[other.selected].file.Name != "a10.txt" || !other.marks[other.selected] || other.prefix != "a" || !slices.Contains(visibleNames(other), "zulu.md") {
		t.Fatal("other ZIP panel did not preserve its own browsing state")
	}
	if message := p.feedback.Current(); message == nil || message.Kind != feedback.Success || message.Text != "Renamed" {
		t.Fatal("missing rename completion feedback", message)
	}
	if v.mutationsBusy() {
		t.Fatal("ZIP rename retained worker ownership")
	}
}

func TestArchiveFolderRenameFollowsOpenDescendants(t *testing.T) {
	v, filename := archiveView(t)
	v.loadLocation(1, browser.Location{Path: filename, Folder: "docs/sub"}, "report.txt")
	waitLoaded(t, v, 1)
	other := &v.panels[1]
	other.toggleMark(other.selected)
	other.filter = "report"
	other.refilter()
	marks, position := maps.Clone(other.marks), other.list.Position
	selectName(t, &v.panels[0], "docs")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	if v.nameEditor.SelectedText() != "docs" {
		t.Fatal("folder rename did not select whole name")
	}
	send(replaceRename(v, "documents"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if other.archiveFolder != "documents/sub" || other.filter != "report" || !maps.Equal(other.marks, marks) || (other.list.Position.First != position.First || other.list.Position.Offset != position.Offset) || other.items[other.selected].file.Name != "report.txt" {
		t.Fatal("panel did not follow renamed folder retaining state", other.archiveFolder)
	}
	if v.panels[0].items[v.panels[0].selected].file.Name != "documents" || v.active != 0 {
		t.Fatal("folder rename lost source selection or focus")
	}
	// Renaming a file in the followed subfolder captures that archive location.
	v.active = 1
	send(pressed(key.NameF2))
	send(replaceRename(v, "report-renamed.txt"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if !slices.Contains(visibleNames(other), "report-renamed.txt") || other.archiveFolder != "documents/sub" {
		t.Fatal("nested ZIP rename used the wrong member path")
	}
}

func TestArchiveRenameConflictRetainsEditorAndCanRetry(t *testing.T) {
	v, filename := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "a2.txt")
	p.toggleMark(p.selected)
	original, _ := os.ReadFile(filename)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "a10.txt"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if n := v.nameInteraction(); n == nil || n.pending || n.err == nil || v.nameEditor.ReadOnly || v.nameEditor.Text() != "a10.txt" {
		t.Fatal("conflict did not retain editable draft")
	}
	if message := p.feedback.Current(); message == nil || message.Text != "Name already exists" {
		t.Fatal("conflict reason lost", message)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(original, after) || !p.marks[p.selected] {
		t.Fatal("conflict changed ZIP or marks")
	}
	send(replaceRename(v, "renamed.txt"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if v.nameInteraction() != nil || p.items[p.selected].file.Name != "renamed.txt" || !p.marks[p.selected] {
		t.Fatal("retry failed to finish rename")
	}
}

func TestArchiveRenameCancelAndCaseOnlyNames(t *testing.T) {
	v, filename := archiveView(t)
	writePanelArchive(t, filename, "A.txt", "a.txt")
	v.refreshPanel()
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	selectName(t, p, "A.txt")
	original, _ := os.ReadFile(filename)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "changed.txt"))
	send(pressed(key.NameEscape))
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(original, after) || !v.interaction.idle() {
		t.Fatal("cancelled rename changed ZIP")
	}
	send(pressed(key.NameF2))
	send(replaceRename(v, "a.txt"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	if v.nameInteraction().err == nil {
		t.Fatal("case-distinct existing name was overwritten")
	}
	send(replaceRename(v, "B.txt"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if !slices.Equal(visibleNames(p), []string{"..", "a.txt", "B.txt"}) {
		t.Fatal("rename conflated case-sensitive ZIP names", visibleNames(p))
	}
}
