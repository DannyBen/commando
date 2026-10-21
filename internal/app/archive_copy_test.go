package app

import (
	"bytes"
	"errors"
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

func TestArchiveCopyUsesSharedPanelFlowAndNativeTransfer(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, marked := range []bool{false, true} {
			v, filename := archiveView(t)
			p := &v.panels[0]
			before, _ := os.ReadFile(filename)
			selectName(t, p, "a2.txt")
			names := []string{"a2.txt"}
			if marked {
				p.toggleMark(p.selected)
				selectName(t, p, "docs")
				p.toggleMark(p.selected)
				selectName(t, p, "empty")
				p.toggleMark(p.selected)
				selectName(t, p, "a10.txt")
				names = []string{"docs", "empty", "a2.txt"}
			}
			selected, marks := p.selected, maps.Clone(p.marks)
			destination := t.TempDir()
			writeBatchFiles(t, destination, map[string]string{"keep.txt": "unchanged"})
			v.load(1, destination, "keep.txt")
			waitLoaded(t, v, 1)
			forbidTransferClipboard(t, v)
			called := make(chan browser.TransferRequest, 1)
			v.ownerWindow = 123
			v.loader.transfer = func(r browser.TransferRequest) (browser.TransferResult, error) {
				called <- r
				return fixtureTransfer(r)
			}
			send, _ := panelInputFrames(v)
			if direct {
				send(pressed("]"), key.EditEvent{Text: "]"})
			} else {
				send(pressed(key.NameF5))
				if v.active != 1 || v.inlineInteraction() == nil || !slices.Equal(v.inlineInteraction().options, []string{"Copy here"}) {
					t.Fatal("archive did not use ordinary destination targeting")
				}
				send(pressed(key.NameReturn))
			}
			resultPanel := 1
			if direct {
				resultPanel = 0
			}
			waitLoaded(t, v, resultPanel)
			send()
			if len(called) != 1 {
				t.Fatal("archive copy did not reach shared native transfer once")
			}
			request := <-called
			if request.Move || request.Duplicate || request.Directory != destination || request.Owner != 123 {
				t.Fatal("wrong archive transfer", request)
			}
			for i, name := range names {
				if filepath.Base(request.Paths[i]) != name {
					t.Fatal("archive selection changed", request.Paths)
				}
				if _, err := os.Stat(request.Paths[i]); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("staged source survived native completion", err)
				}
			}
			if marked {
				assertBatchFile(t, filepath.Join(destination, "docs", "sub", "report.txt"), "docs/sub/report.txt")
				if info, err := os.Stat(filepath.Join(destination, "empty")); err != nil || !info.IsDir() {
					t.Fatal("empty folder lost", err)
				}
			}
			assertBatchFile(t, filepath.Join(destination, "a2.txt"), "a2.txt")
			assertBatchFile(t, filepath.Join(destination, "keep.txt"), "unchanged")
			if p.archiveFolder != "." || p.path != filename || p.selected != selected || !reflect.DeepEqual(p.marks, marks) {
				t.Fatal("copy changed archive browsing state")
			}
			if p := &v.panels[1]; p.items[p.selected].file.Name != names[0] || !slices.Equal(p.selectedNames(), names) {
				t.Fatal("destination did not select and mark the copied items")
			}
			if v.active != resultPanel || v.transferPending || v.mutationsBusy() {
				t.Fatal("copy changed focus or retained busy state")
			}
			if message := v.panels[resultPanel].feedback.Current(); message == nil || message.Kind != feedback.Success {
				t.Fatal("missing copy completion feedback", message)
			}
			after, _ := os.ReadFile(filename)
			if !bytes.Equal(before, after) {
				t.Fatal("copy modified ZIP")
			}
		}
	}
}

func TestArchiveCopyAndMoveCancellation(t *testing.T) {
	v, _ := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "docs")
	p.toggleMark(p.selected)
	forbidTransferClipboard(t, v)
	v.loader.transfer = func(browser.TransferRequest) (browser.TransferResult, error) {
		t.Error("cancelled targeting reached native transfer")
		return browser.TransferResult{}, nil
	}
	send, _ := panelInputFrames(v)
	for _, c := range v.commandCatalogue() {
		if slices.Contains([]string{"entry.copy_to_panel", "entry.copy_right"}, c.id) && !c.allowed(v) {
			t.Fatal("readable archive copy unavailable", c.id)
		}
	}
	send(pressed(key.NameF6))
	send(pressed(key.NameEscape))
	send(pressed(key.NameF5))
	send(pressed(key.NameEscape))
	if v.active != 0 || !v.interaction.idle() || !p.marks[p.selected] || v.transferPending {
		t.Fatal("copy targeting cancellation changed source state")
	}
}

func TestArchiveCopyExtractionFailureDoesNotTouchDestination(t *testing.T) {
	v, filename := archiveView(t)
	selectName(t, &v.panels[0], "docs")
	writePanelArchive(t, filename, "docs/../escape.txt")
	v.loader.transfer = func(browser.TransferRequest) (browser.TransferResult, error) {
		t.Error("failed extraction reached native transfer")
		return browser.TransferResult{}, nil
	}
	forbidTransferClipboard(t, v)
	v.startPanelTransfer(false)
	v.chooseInlineAction()
	waitLoaded(t, v, 1)
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "Invalid filename" {
		t.Fatal("extraction failure did not report error", message)
	}
	if v.transferPending || v.mutationsBusy() {
		t.Fatal("extraction failure retained busy state")
	}
}

func TestArchiveCopyCleansStagingAfterNativeCancellationOrFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		v, _ := archiveView(t)
		selectName(t, &v.panels[0], "a2.txt")
		destination := t.TempDir()
		writeBatchFiles(t, destination, map[string]string{"keep.txt": "original"})
		scratch := t.TempDir()
		t.Setenv("TMPDIR", scratch)
		t.Setenv("TMP", scratch)
		t.Setenv("TEMP", scratch)
		v.load(1, destination, "keep.txt")
		waitLoaded(t, v, 1)
		paths := make(chan []string, 1)
		v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
			paths <- request.Paths
			// Simulate partial native changes: reconciliation must accept them even
			// when the shell subsequently reports cancellation or a failure.
			if err := os.WriteFile(filepath.Join(destination, "partial.txt"), []byte("partial"), 0600); err != nil {
				return browser.TransferResult{}, err
			}
			result := browser.TransferResult{Directory: destination, Attempted: true, Aborted: !fail}
			if fail {
				return result, os.ErrPermission
			}
			return result, nil
		}
		forbidTransferClipboard(t, v)
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF5), pressed(key.NameReturn))
		waitLoaded(t, v, 1)
		for _, path := range <-paths {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("native cancellation/failure leaked staging", err)
			}
		}
		assertBatchFile(t, filepath.Join(destination, "keep.txt"), "original")
		assertBatchFile(t, filepath.Join(destination, "partial.txt"), "partial")
		if !slices.Contains(visibleNames(&v.panels[1]), "partial.txt") || v.transferPending || v.mutationsBusy() {
			t.Fatal("partial result was not reconciled")
		}
		if entries, err := os.ReadDir(filepath.Join(scratch, "Commando", "session")); err != nil || len(entries) != 0 {
			t.Fatal("native cancellation/failure leaked staging", entries, err)
		}
		message := v.panels[1].feedback.Current()
		if message == nil || fail && message.Text != "Access denied" || !fail && message.Text != "Transfer stopped" {
			t.Fatal("native cancellation/failure feedback lost", message)
		}
	}
}
