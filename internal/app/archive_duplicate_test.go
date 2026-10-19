package app

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestSingleArchiveDuplicateUsesSharedNameEditor(t *testing.T) {
	for _, folder := range []bool{false, true} {
		v, filename := archiveView(t)
		if !v.addPanel(1) {
			t.Fatal("clone failed")
		}
		v.active = 0
		p, peer := &v.panels[0], &v.panels[1]
		source, suggestion, custom, member := "a2.txt", "a2 - Copy.txt", "A2.txt", "A2.txt"
		if folder {
			source, suggestion, custom, member = "docs", "docs - Copy", "new docs", "new docs/sub/report.txt"
		}
		selectName(t, p, source)
		p.toggleMark(p.selected)
		selectName(t, p, "a10.txt") // One mark wins over the highlighted row.
		selectName(t, peer, "a10.txt")
		peer.toggleMark(peer.selected)
		marks := slices.Clone(p.selectedNames())
		before, _ := os.ReadFile(filename)
		forbidTransferClipboard(t, v)
		scratch := t.TempDir()
		t.Setenv("TMPDIR", scratch)
		t.Setenv("TMP", scratch)
		t.Setenv("TEMP", scratch)
		send, _ := panelInputFrames(v)
		send(modified("D", key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
		r := v.nameInteraction()
		if r == nil || r.kind != nameDuplicate || r.archiveFolder != "." || v.nameEditor.Text() != suggestion || p.items[r.row-1].name != source {
			t.Fatal("archive duplicate did not use source-anchored inline editor", r, v.nameEditor.Text())
		}
		wantSelection := "a2 - Copy"
		if folder {
			wantSelection = suggestion
		}
		if v.nameEditor.SelectedText() != wantSelection {
			t.Fatal("wrong filename selection", v.nameEditor.SelectedText())
		}
		unchanged, _ := os.ReadFile(filename)
		if !bytes.Equal(before, unchanged) {
			t.Fatal("draft created data before Enter")
		}
		send(replaceCreate(v, custom))
		send(pressed(key.NameReturn))
		if r.pending && !v.mutationsBusy() || !r.pending && !r.finished {
			t.Fatal("duplicate bypassed shared worker policy")
		}
		waitCreated(t, v)
		send()
		if p.items[p.selected].name != custom || !slices.Equal(p.selectedNames(), marks) || !v.interaction.idle() || v.transferPending {
			t.Fatal("copy did not select exact new name or preserve marks")
		}
		original := "a2.txt"
		if folder {
			original = "docs/sub/report.txt"
		}
		if archiveMemberContents(t, filename, member) != original || archiveMemberContents(t, filename, original) != original {
			t.Fatal("copy or original contents changed")
		}
		if peer.items[peer.selected].name != "a10.txt" || !peer.marks[peer.selected] || !slices.Contains(visibleNames(peer), custom) {
			t.Fatal("archive peer lost state")
		}
		entries, err := os.ReadDir(scratch)
		if err != nil || len(entries) != 0 {
			t.Fatal("duplicate extracted temporary files", entries, err)
		}
	}
}

func TestArchiveDuplicateCancelCollisionRetryAndBatch(t *testing.T) {
	v, filename := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "a2.txt")
	forbidTransferClipboard(t, v)
	send, _ := panelInputFrames(v)
	before, _ := os.ReadFile(filename)
	send(modified("D", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(pressed(key.NameEscape))
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) || v.transferPending || !v.interaction.idle() {
		t.Fatal("cancel changed ZIP or retained pending state")
	}
	send(modified("D", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(replaceCreate(v, "a10.txt"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if v.nameInteraction() == nil || !errors.Is(v.nameInteraction().err, fs.ErrExist) || v.nameEditor.ReadOnly {
		t.Fatal("collision did not retain editable draft")
	}
	send(replaceCreate(v, "retry.txt"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if archiveMemberContents(t, filename, "retry.txt") != "a2.txt" {
		t.Fatal("collision retry failed")
	}
	for _, name := range []string{"docs", "empty", "a2.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	selectName(t, p, "a10.txt")
	marks := maps.Clone(p.marks)
	send(modified("D", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	if !v.interaction.idle() || v.transferPending || p.items[p.selected].name != "a10.txt" || len(p.marks) != len(marks) {
		t.Fatal("batch retained editor or changed browsing state")
	}
	for _, member := range []string{"docs - Copy/sub/report.txt", "empty - Copy/", "a2 - Copy.txt"} {
		archiveMemberContents(t, filename, member)
	}
	if message := p.feedback.Current(); message == nil || message.Kind != feedback.Success || message.Text != "3 copied" {
		t.Fatal("wrong batch feedback", message)
	}
}

func TestArchiveDuplicatePaletteTargetsOpenSubfolder(t *testing.T) {
	v, filename := archiveView(t)
	v.loadLocation(0, browser.Location{Path: filename, Folder: "docs/sub"}, "report.txt")
	waitLoaded(t, v, 0)
	forbidTransferClipboard(t, v)
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "Entry Duplicate"}, pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if v.nameInteraction() == nil || v.nameEditor.Text() != "report - Copy.txt" {
		t.Fatal("palette did not offer archive duplicate")
	}
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if v.panels[0].archiveFolder != "docs/sub" || archiveMemberContents(t, filename, "docs/sub/report - Copy.txt") != "docs/sub/report.txt" {
		t.Fatal("duplicate escaped current subfolder")
	}
}
