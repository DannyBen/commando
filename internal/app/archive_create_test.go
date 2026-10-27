package app

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestArchiveCreationUsesSharedEditorAndReconcilesPeers(t *testing.T) {
	for _, folder := range []bool{false, true} {
		for _, nested := range []bool{false, true} {
			v, filename := archiveView(t)
			if !v.addPanel(1) {
				t.Fatal("could not clone archive panel")
			}
			v.active = 0
			selectName(t, &v.panels[0], "a2.txt")
			v.panels[0].toggleMark(v.panels[0].selected)
			selectName(t, &v.panels[1], "a10.txt")
			v.panels[1].toggleMark(v.panels[1].selected)
			selectName(t, &v.panels[2], "sample.zip")
			v.panels[2].toggleMark(v.panels[2].selected)
			forbidTransferClipboard(t, v)
			scratch := t.TempDir()
			t.Setenv("TMPDIR", scratch)
			t.Setenv("TMP", scratch)
			t.Setenv("TEMP", scratch)
			send, _ := panelInputFrames(v)
			shortcut := modified("N", key.ModCtrl)
			if folder {
				shortcut = newFolderKey()
			}
			name := "新しい.txt"
			if nested {
				name = `docs\new\新しい.txt`
			}
			send(shortcut)
			r := v.nameInteraction()
			if r == nil || r.archiveFolder != "." || !r.draft() {
				t.Fatal("archive did not use shared creation draft")
			}
			send(replaceCreate(v, name), pressed(key.NameReturn))
			if r.pending && (!v.mutationsBusy() || !v.nameEditor.ReadOnly) || !r.pending && !r.finished {
				t.Fatal("submitted creation did not own interaction/worker")
			}
			waitCreated(t, v)
			send()
			p := &v.panels[0]
			wantFolder := "."
			member := "新しい.txt"
			if nested {
				wantFolder, member = "docs/new", "docs/new/新しい.txt"
			}
			if folder {
				member += "/"
			}
			if p.archiveFolder != wantFolder || p.items[p.selected].name != "新しい.txt" || p.items[p.selected].file.Directory != folder || !v.interaction.idle() || v.mutationsBusy() {
				t.Fatal("creation did not select new archive entry", p.archiveFolder, visibleNames(p))
			}
			if len(p.marks) != 0 {
				t.Fatal("creation retained marks in the active archive panel")
			}
			peer := &v.panels[1]
			if peer.archiveFolder != "." || peer.items[peer.selected].name != "a10.txt" || !peer.marks[peer.selected] || !slices.Contains(visibleNames(peer), func() string {
				if nested {
					return "docs"
				}
				return "新しい.txt"
			}()) {
				t.Fatal("archive peer was not reconciled")
			}
			native := &v.panels[2]
			if native.items[native.selected].name != "sample.zip" || !native.marks[native.selected] {
				t.Fatal("archive container refresh lost native selection/marks")
			}
			if archiveMemberContents(t, filename, member) != "" {
				t.Fatal("new entry was not empty")
			}
			entries, err := os.ReadDir(scratch)
			if err != nil || len(entries) != 0 {
				t.Fatal("creation used temporary extracted files", entries, err)
			}
			message := p.feedback.Current()
			if message == nil || message.Kind != feedback.Success {
				t.Fatal("missing creation success", message)
			}
		}
	}
}

func TestArchiveCreationCollisionRetryCancelAndPalette(t *testing.T) {
	v, filename := archiveView(t)
	send, _ := panelInputFrames(v)
	before, _ := os.ReadFile(filename)
	send(modified("N", key.ModCtrl))
	send(replaceCreate(v, "a2.txt"), pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if v.nameInteraction() == nil || !errors.Is(v.nameInteraction().err, fs.ErrExist) || v.nameEditor.ReadOnly || v.nameEditor.Text() != "a2.txt" {
		t.Fatal("collision did not retain editable draft")
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("collision changed ZIP")
	}
	send(replaceCreate(v, "fresh/../invalid"))
	send(pressed(key.NameReturn))
	if !errors.Is(v.nameInteraction().err, browser.ErrInvalidName) {
		t.Fatal("invalid path was not refused before dispatch")
	}
	send(replaceCreate(v, "retry.txt"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if !v.interaction.idle() || v.panels[0].items[v.panels[0].selected].name != "retry.txt" {
		t.Fatal("retry did not complete")
	}
	before, _ = os.ReadFile(filename)
	send(newFolderKey())
	send(replaceCreate(v, "cancelled"), pressed(key.NameEscape))
	after, _ = os.ReadFile(filename)
	if !bytes.Equal(before, after) || !v.interaction.idle() {
		t.Fatal("cancelled draft changed ZIP")
	}
	v.loadLocation(0, browser.Location{Path: filename, Folder: "empty"}, "")
	waitLoaded(t, v, 0)
	send()
	send(pressed("`"))
	send(key.EditEvent{Text: "Entry New folder"}, pressed(key.NameReturn))
	if v.nameInteraction() == nil {
		t.Fatal("palette creation is unavailable in ZIP")
	}
	send(replaceCreate(v, "child"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if v.panels[0].archiveFolder != "empty" || archiveMemberContents(t, filename, "empty/child/") != "" {
		t.Fatal("creation did not target open ZIP folder")
	}
}
