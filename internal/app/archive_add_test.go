package app

import (
	"archive/zip"
	"bytes"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func archiveMemberContents(t *testing.T, filename, name string) string {
	t.Helper()
	reader, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, member := range reader.File {
		if member.Name != name {
			continue
		}
		stream, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		contents, err := io.ReadAll(stream)
		if err != nil {
			t.Fatal(err)
		}
		return string(contents)
	}
	t.Fatal("archive member missing", name)
	return ""
}

func TestPanelCopiesIntoArchiveUseSharedFlowAndPreservePanels(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, marked := range []bool{false, true} {
			v, filename := archiveView(t)
			source := t.TempDir()
			writeBatchFiles(t, source, map[string]string{"new.txt": "new", "added/nested/file.bin": "nested", "unselected.txt": "skip"})
			if err := os.Mkdir(filepath.Join(source, "added", "empty"), 0700); err != nil {
				t.Fatal(err)
			}
			v.load(1, source, "new.txt")
			waitLoaded(t, v, 1)
			v.active = 1
			p := &v.panels[1]
			if marked {
				p.toggleMark(p.selected)
				selectName(t, p, "added")
				p.toggleMark(p.selected)
				selectName(t, p, "unselected.txt")
			}
			selected, marks := p.selected, maps.Clone(p.marks)
			destination := &v.panels[0]
			selectName(t, destination, "a2.txt")
			destination.toggleMark(destination.selected)
			selectName(t, destination, "a10.txt")
			v.loader.transfer = func(browser.TransferRequest) (browser.TransferResult, error) {
				t.Error("ZIP addition reached native file transfer")
				return browser.TransferResult{}, nil
			}
			forbidTransferClipboard(t, v)
			send, _ := panelInputFrames(v)
			if direct {
				send(pressed("["), key.EditEvent{Text: "["})
			} else {
				send(pressed(key.NameF5))
				if v.active != 0 || v.inlineInteraction() == nil {
					t.Fatal("ZIP target did not use Copy here")
				}
				send(pressed(key.NameReturn))
			}
			resultPanel := 0
			if direct {
				resultPanel = 1
			}
			waitLoaded(t, v, resultPanel)
			send()
			if archiveMemberContents(t, filename, "new.txt") != "new" {
				t.Fatal("copied file contents lost")
			}
			if marked {
				if archiveMemberContents(t, filename, "added/nested/file.bin") != "nested" {
					t.Fatal("nested contents lost")
				}
				if archiveMemberContents(t, filename, "added/empty/") != "" {
					t.Fatal("empty folder lost")
				}
			}
			if !slices.Contains(visibleNames(destination), "new.txt") || destination.items[destination.selected].file.Name != "a10.txt" {
				t.Fatal("ZIP panel did not reconcile independently")
			}
			selectName(t, destination, "a2.txt")
			if !destination.marks[destination.selected] {
				t.Fatal("destination mark lost")
			}
			if p.selected != selected || !maps.Equal(p.marks, marks) || v.active != resultPanel || v.transferPending || v.mutationsBusy() {
				t.Fatal("copy changed source state, focus or retained busy state")
			}
			assertBatchFile(t, filepath.Join(source, "new.txt"), "new")
			if message := v.panels[resultPanel].feedback.Current(); message == nil || message.Kind != feedback.Success {
				t.Fatal("copy-in completion feedback missing", message)
			}
		}
	}
}

func TestArchiveCopyIntoAnotherArchiveAndItsCurrentSubfolder(t *testing.T) {
	for _, sameArchive := range []bool{false, true} {
		v, source := archiveView(t)
		before, _ := os.ReadFile(source)
		target := source
		folder := "empty"
		if !sameArchive {
			target = filepath.Join(t.TempDir(), "target.zip")
			writePanelArchive(t, target, "inside/", "keep.txt")
			folder = "inside"
		}
		v.loadLocation(1, browser.Location{Path: target, Folder: folder}, "")
		waitLoaded(t, v, 1)
		selectName(t, &v.panels[0], "docs")
		forbidTransferClipboard(t, v)
		scratch := t.TempDir()
		t.Setenv("TMPDIR", scratch)
		t.Setenv("TMP", scratch)
		t.Setenv("TEMP", scratch)
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF5))
		send(pressed(key.NameReturn))
		waitLoaded(t, v, 1)
		send()
		if archiveMemberContents(t, target, folder+"/docs/sub/report.txt") != "docs/sub/report.txt" {
			t.Fatal("archive copy targeted wrong folder")
		}
		if v.panels[1].archiveFolder != folder || !slices.Contains(visibleNames(&v.panels[1]), "docs") {
			t.Fatal("destination archive location changed")
		}
		leftovers, err := os.ReadDir(scratch)
		if err != nil || len(leftovers) != 0 {
			t.Fatal("archive copy leaked temporary sources", leftovers, err)
		}
		if !sameArchive {
			after, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) {
				t.Fatal("source archive changed")
			}
		}
	}
}

func TestArchiveCopyInCollisionAndMoveRefusal(t *testing.T) {
	v, filename := archiveView(t)
	source := t.TempDir()
	writeBatchFiles(t, source, map[string]string{"a2.txt": "collision", "fresh.txt": "new"})
	v.load(1, source, "a2.txt")
	waitLoaded(t, v, 1)
	v.active = 1
	p := &v.panels[1]
	p.toggleMark(p.selected)
	selectName(t, p, "fresh.txt")
	p.toggleMark(p.selected)
	before, _ := os.ReadFile(filename)
	send, _ := panelInputFrames(v)
	if !panelTransferReady(v, -1, false) || panelTransferReady(v, -1, true) {
		t.Fatal("ZIP target capabilities ignored copy/move distinction")
	}
	send(pressed(key.NameF6))
	send(pressed(key.NameReturn))
	if v.inlineInteraction() == nil || v.transferPending {
		t.Fatal("move into ZIP was submitted")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameF5))
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if message := v.panels[0].feedback.Current(); message == nil || message.Text != "Name already exists" {
		t.Fatal("copy-in conflict not reported", message)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) || v.transferPending || v.mutationsBusy() {
		t.Fatal("failed batch modified ZIP or retained busy state")
	}
	v.active = 0
	for _, command := range v.commandCatalogue() {
		if slices.Contains([]string{"entry.paste", "entry.cut", "entry.duplicate", "entry.new_file", "entry.new_folder"}, command.id) && command.allowed(v) {
			t.Fatal("copy-in enabled a separately planned action", command.id)
		}
	}
}
