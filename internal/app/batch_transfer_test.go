package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func writeBatchFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBatchFile(t *testing.T, path, want string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Fatalf("%s: got %q, %v; want %q", path, data, err, want)
	}
}

func batchConflictView(t *testing.T, move bool) (*view, *testFileClipboard, string, string) {
	t.Helper()
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	writeBatchFiles(t, source, map[string]string{"folder/new.txt": "new child"})
	writeBatchFiles(t, target, map[string]string{"folder/nested/file": "old nested", "folder/extra.txt": "extra", "a.txt": "old a"})
	v.load(1, target, "")
	waitLoaded(t, v, 1)
	selectName(t, &v.panels[1], "a.txt")
	v.panels[1].toggleMark(v.panels[1].selected)
	v.panels[1].selected = 0
	markClipboardItems(t, &v.panels[0])
	v.panels[0].toggleMark(v.panels[0].selected) // Include non-conflicting z.txt.
	send, _ := panelInputFrames(v)
	name := key.Name("C")
	if move {
		name = "X"
	}
	send(modified(name, key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	a := v.actions
	if a == nil || a.panel != 1 || a.selected != 0 || !slices.Equal(a.options, []string{"Overwrite all", "Skip existing", "Cancel"}) || len(a.headers) != 0 || v.panels[1].items[a.subjectRow].name != "folder" {
		t.Fatal("batch did not offer compact choices at its top-level conflict folder")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Warning || message.Text != "2 files exist" {
		t.Fatal("batch footer did not report file conflict count")
	}
	assertBatchFile(t, filepath.Join(target, "a.txt"), "old a")
	assertBatchFile(t, filepath.Join(target, "folder", "nested", "file"), "old nested")
	if _, err := os.Stat(filepath.Join(target, "z.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("batch began before its choice")
	}
	v.started = false
	return v, c, source, target
}

func TestBatchCopyAndMoveOverwriteOrSkipExisting(t *testing.T) {
	for _, move := range []bool{false, true} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("move=%t/skip=%t", move, skip), func(t *testing.T) {
				v, c, source, target := batchConflictView(t, move)
				send, _ := panelInputFrames(v)
				if skip {
					send(pressed(key.NameDownArrow))
				}
				send(pressed(key.NameReturn))
				waitLoaded(t, v, 1)
				send()
				if v.actions != nil || v.transferPending || v.panels[1].items[v.panels[1].selected].name != "folder" {
					t.Fatal("batch retained chooser or lost target selection")
				}
				assertBatchFile(t, filepath.Join(target, "folder", "extra.txt"), "extra")
				assertBatchFile(t, filepath.Join(target, "folder", "new.txt"), "new child")
				assertBatchFile(t, filepath.Join(target, "z.txt"), "keep")
				selectName(t, &v.panels[1], "a.txt")
				if !v.panels[1].marks[v.panels[1].selected] {
					t.Fatal("batch lost destination mark")
				}
				if skip {
					assertBatchFile(t, filepath.Join(target, "folder", "nested", "file"), "old nested")
					assertBatchFile(t, filepath.Join(target, "a.txt"), "old a")
					assertBatchFile(t, filepath.Join(source, "folder", "nested", "file"), "nested")
					if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Warning || message.Text != "2 transferred; 2 skipped" {
						t.Fatalf("wrong skip result: %+v", message)
					}
				} else {
					assertBatchFile(t, filepath.Join(target, "folder", "nested", "file"), "nested")
					assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
				}
				clipboard, err := c.read()
				if move && !skip {
					if !errors.Is(err, browser.ErrNoClipboardFiles) || len(v.cutClipboard.Paths) != 0 || len(v.panels[0].marks) != 0 {
						t.Fatal("completed batch move retained cut selection or source marks")
					}
				} else if move {
					if err != nil || len(clipboard.Paths) != 2 || len(v.panels[0].marks) != 2 {
						t.Fatal("partial folder or skipped file was consumed from cut selection")
					}
					if _, err := os.Stat(filepath.Join(source, "folder", "new.txt")); !errors.Is(err, fs.ErrNotExist) {
						t.Fatal("non-conflicting child did not move out of partial folder")
					}
					// A new paste retries the retained source contents only.
					v.panels[1].selected = 0
					send(modified("V", key.ModCtrl))
					waitLoaded(t, v, 1)
					send()
					if v.actions == nil {
						t.Fatal("retained cut selection was not retryable")
					}
					send(pressed(key.NameReturn))
					waitLoaded(t, v, 1)
					send()
					if _, err := c.read(); !errors.Is(err, browser.ErrNoClipboardFiles) {
						t.Fatal("retry did not finish retained folder and file")
					}
				} else if err != nil || len(clipboard.Paths) != 3 {
					t.Fatal("batch copy consumed reusable clipboard")
				}
			})
		}
	}
}

func TestBatchCancellationLeavesEntireTransferUntouched(t *testing.T) {
	for _, events := range [][]key.Event{
		{pressed(key.NameEnd), pressed(key.NameReturn)},
		{pressed(key.NameEscape)},
		{pressed(key.NameLeftArrow)},
		{pressed(key.NameTab)},
	} {
		v, c, source, target := batchConflictView(t, true)
		before, _ := c.read()
		send, _ := panelInputFrames(v)
		for _, event := range events {
			send(event)
		}
		current, _ := c.read()
		if v.actions != nil || v.transferPending || !reflect.DeepEqual(current, before) || v.panels[1].items[v.panels[1].selected].name != "folder" {
			t.Fatal("batch cancellation changed clipboard or lost subject selection")
		}
		assertBatchFile(t, filepath.Join(source, "a.txt"), "alpha")
		assertBatchFile(t, filepath.Join(source, "folder", "new.txt"), "new child")
		assertBatchFile(t, filepath.Join(target, "a.txt"), "old a")
		if _, err := os.Stat(filepath.Join(target, "z.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("batch cancellation left transferred content")
		}
	}
}

func TestMergedMoveFollowsOpenDescendantAndIncludesDestinationExtras(t *testing.T) {
	v, c, source, target := batchConflictView(t, true)
	writeBatchFiles(t, target, map[string]string{"folder/nested/extra.txt": "destination extra"})
	v.load(2, filepath.Join(source, "folder", "nested"), "")
	waitLoaded(t, v, 2)
	selectName(t, &v.panels[2], "file")
	v.panels[2].toggleMark(v.panels[2].selected)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	p := &v.panels[2]
	if p.path != filepath.Join(target, "folder", "nested") || p.items[p.selected].name != "file" || !p.marks[p.selected] {
		t.Fatal("merged move stranded descendant or lost its selection/marks")
	}
	selectName(t, p, "extra.txt")
	if p.items[p.selected].file.Bytes != int64(len("destination extra")) {
		t.Fatal("relocated descendant omitted destination-only entries")
	}
	if _, err := c.read(); !errors.Is(err, browser.ErrNoClipboardFiles) {
		t.Fatal("merged move did not consume completed selection")
	}
}

func TestBatchPartialFailureUpdatesOpenSourceFolderAndRetainsClipboard(t *testing.T) {
	v, c, source, target := batchConflictView(t, true)
	v.load(2, filepath.Join(source, "folder"), "")
	waitLoaded(t, v, 2)
	selectName(t, &v.panels[2], "new.txt")
	v.panels[2].toggleMark(v.panels[2].selected)
	v.loader.executeTransfer = func(plan *browser.TransferPlan, move bool, policy browser.TransferPolicy) (browser.TransferResult, error) {
		// Execute a real partial merge, then report a later failed top-level item.
		partial, err := browser.PlanTransfer([]string{filepath.Join(source, "folder")}, target)
		if err != nil {
			return browser.TransferResult{}, err
		}
		result, err := browser.ExecuteTransfer(partial, move, browser.TransferSkipExisting)
		return result, errors.Join(err, fs.ErrPermission)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	remaining, err := c.read()
	if err != nil || len(remaining.Paths) != 3 || v.panels[2].path != filepath.Join(source, "folder") || len(v.panels[2].marks) != 0 {
		t.Fatal("partial merge lost clipboard/source path or kept mark on moved child")
	}
	for _, item := range v.panels[2].items {
		if item.name == "new.txt" {
			t.Fatal("open source folder retained moved child")
		}
	}
	assertBatchFile(t, filepath.Join(source, "folder", "nested", "file"), "nested")
	assertBatchFile(t, filepath.Join(target, "folder", "new.txt"), "new child")
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "1 transferred; Access denied" {
		t.Fatalf("partial failure feedback: %+v", message)
	}
}

func TestBatchChangedFileRequiresNewChoiceBeforeAnyTransfers(t *testing.T) {
	v, _, _, target := batchConflictView(t, false)
	writeBatchFiles(t, target, map[string]string{"a.txt": "external change"})
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	if v.actions == nil || v.actions.selected != 0 || !v.transferPending {
		t.Fatal("stale batch did not request a fresh decision")
	}
	assertBatchFile(t, filepath.Join(target, "a.txt"), "external change")
	if _, err := os.Stat(filepath.Join(target, "folder", "new.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("stale batch transferred an earlier child")
	}
}

func TestBatchDecisionPreservesNewerClipboard(t *testing.T) {
	v, c, source, target := batchConflictView(t, true)
	newer, err := c.write([]string{filepath.Join(source, "z.txt")}, false)
	if err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	current, err := c.read()
	if err != nil || !reflect.DeepEqual(current, newer) {
		t.Fatal("batch replaced newer clipboard selection")
	}
	assertBatchFile(t, filepath.Join(target, "a.txt"), "alpha")
}
