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
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestArchiveClipboardCopiesReferencesThenPastesThroughSharedFlows(t *testing.T) {
	for _, intoZIP := range []bool{false, true} {
		t.Run(map[bool]string{false: "folder", true: "zip"}[intoZIP], func(t *testing.T) {
			v, source := archiveView(t)
			c := attachTestFileClipboard(v)
			target := t.TempDir()
			folder := ""
			if intoZIP {
				target = filepath.Join(target, "target.zip")
				folder = "inside"
				writePanelArchive(t, target, "inside/keep.txt")
				v.loadLocation(1, browser.Location{Path: target, Folder: folder}, "keep.txt")
			} else {
				writeBatchFiles(t, target, map[string]string{"keep.txt": "keep"})
				v.load(1, target, "keep.txt")
			}
			waitLoaded(t, v, 1)
			destination := &v.panels[1]
			destination.toggleMark(destination.selected)
			p := &v.panels[0]
			for _, name := range []string{"docs", "a2.txt"} {
				selectName(t, p, name)
				p.toggleMark(p.selected)
			}
			selectName(t, p, "a10.txt")
			marks := maps.Clone(p.marks)
			selected := p.selected
			scratch := t.TempDir()
			t.Setenv("TMPDIR", scratch)
			t.Setenv("TMP", scratch)
			t.Setenv("TEMP", scratch)
			calls := 0
			v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
				calls++
				if request.Move || len(request.Paths) != 2 {
					t.Error("wrong deferred transfer", request)
				}
				return fixtureTransfer(request)
			}
			send, _ := panelInputFrames(v)
			send(modified("C", key.ModCtrl))
			waitLoaded(t, v, 0)
			send()
			captured, err := c.read()
			if err != nil || captured.Archive == nil || len(captured.Paths) != 0 || captured.Move || !slices.Equal(captured.Archive.Names, []string{"docs", "a2.txt"}) {
				t.Fatal("copy did not capture deferred references", captured, err)
			}
			entries, _ := os.ReadDir(scratch)
			if len(entries) != 0 || calls != 0 {
				t.Fatal("Copy extracted files or dispatched a transfer")
			}
			send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
			waitLoaded(t, v, 1)
			send()
			if intoZIP {
				if archiveMemberContents(t, target, "inside/docs/sub/report.txt") != "docs/sub/report.txt" {
					t.Fatal("archive paste did not target the open subfolder")
				}
				if calls != 0 {
					t.Fatal("ZIP paste used native transfer")
				}
			} else {
				data, err := os.ReadFile(filepath.Join(target, "docs", "sub", "report.txt"))
				if err != nil || string(data) != "docs/sub/report.txt" || calls != 1 {
					t.Fatal("deferred native paste failed", string(data), err, calls)
				}
			}
			current, err := c.read()
			if err != nil || !reflect.DeepEqual(current, captured) || p.selected != selected || !maps.Equal(p.marks, marks) || destination.items[destination.selected].name != "keep.txt" || !destination.marks[destination.selected] || v.active != 1 || v.transferPending || v.mutationsBusy() {
				t.Fatal("paste changed clipboard or browsing state", err)
			}
			entries, err = os.ReadDir(scratch)
			if err != nil || len(entries) != 0 {
				t.Fatal("paste retained temporary files", entries, err)
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatal(err)
			}
			message := destination.feedback.Current()
			if message == nil || message.Kind != feedback.Success || message.Text != "2 copied" {
				t.Fatal("wrong paste feedback", message)
			}
		})
	}
}

func TestNativeClipboardPasteIntoZIPRejectsConflictsAndCut(t *testing.T) {
	v, target := archiveView(t)
	c := attachTestFileClipboard(v)
	source := t.TempDir()
	writeBatchFiles(t, source, map[string]string{"new.txt": "new"})
	v.load(1, source, "new.txt")
	waitLoaded(t, v, 1)
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("C", key.ModCtrl), pressed(key.NameTab), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	waitLoaded(t, v, 0)
	send()
	if archiveMemberContents(t, target, "new.txt") != "new" {
		t.Fatal("native copy did not paste into zip")
	}
	copied, _ := c.read()
	before, _ := os.ReadFile(target)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 0)
	if message := v.panels[0].feedback.Current(); message == nil || message.Text != "Name already exists" {
		t.Fatal("ZIP paste collision was not refused", message)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("colliding paste changed zip")
	}
	if snapshot, _ := c.read(); !reflect.DeepEqual(snapshot, copied) {
		t.Fatal("paste consumed copied files")
	}
	v.active = 1
	v.clipboardSelected(true)
	waitLoaded(t, v, 1)
	if !v.cutEntry(1, v.panels[1].selected) {
		t.Fatal("cut appearance missing before refused paste")
	}
	v.active = 0
	v.pasteFiles()
	waitLoaded(t, v, 0)
	if _, err := os.Stat(filepath.Join(source, "new.txt")); err != nil {
		t.Fatal("cut paste deleted source", err)
	}
	after, _ = os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("unsupported cut paste modified zip")
	}
	current, _ := c.read()
	if !current.Move || len(current.Paths) != 1 || !v.cutEntry(1, v.panels[1].selected) {
		t.Fatal("unsupported cut paste consumed clipboard")
	}
}

func TestArchiveClipboardStaleSourcesAndReplacement(t *testing.T) {
	v, source := archiveView(t)
	c := attachTestFileClipboard(v)
	v.loader.transfer = fixtureTransfer
	selectName(t, &v.panels[0], "a2.txt")
	target := t.TempDir()
	v.load(1, target, "")
	waitLoaded(t, v, 1)
	send, _ := panelInputFrames(v)
	// Capture and paste in one event batch, preserving worker order.
	send(modified("C", key.ModCtrl), pressed(key.NameTab), modified("V", key.ModCtrl))
	waitLoaded(t, v, 0)
	waitLoaded(t, v, 1)
	if _, err := os.Stat(filepath.Join(target, "a2.txt")); err != nil {
		t.Fatal("same-frame copy/paste lost archive references", err)
	}
	writePanelArchive(t, source, "a2.txt", "new.txt")
	v.pasteFiles()
	waitLoaded(t, v, 1)
	if message := v.panels[1].feedback.Current(); message == nil || message.Text != "Archive changed; copy again" {
		t.Fatal("stale reference did not fail clearly", message)
	}
	// A normal clipboard replacement must win, with no stale private selection.
	normal := t.TempDir()
	writeBatchFiles(t, normal, map[string]string{"normal.txt": "normal"})
	c.write([]string{filepath.Join(normal, "normal.txt")}, false)
	v.loader.transfer = fixtureTransfer
	v.pasteFiles()
	waitLoaded(t, v, 1)
	if _, err := os.Stat(filepath.Join(target, "normal.txt")); err != nil {
		t.Fatal("new clipboard was ignored", err)
	}
}

func TestArchiveClipboardProgressDoesNotReleaseWorker(t *testing.T) {
	v, _ := archiveView(t)
	p := &v.panels[0]
	p.loading = true
	v.loader.busy.Store(1)
	request := loadRequest{panel: 0, tracked: true}
	v.acceptLoad(loadResult{request: request, progress: "Extracting archive"})
	if !p.loading || !v.mutationsBusy() || p.feedback.Current() != nil {
		t.Fatal("progress completed the operation or bypassed its delay")
	}
	v.loader.busy.Store(0)
	p.loading = false
}
