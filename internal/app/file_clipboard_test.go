package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type testFileClipboard struct {
	sync.Mutex
	data browser.FileClipboard
}

func (c *testFileClipboard) read() (browser.FileClipboard, error) {
	c.Lock()
	defer c.Unlock()
	data := c.data
	data.Paths = slices.Clone(data.Paths)
	if len(data.Paths) == 0 {
		return data, browser.ErrNoClipboardFiles
	}
	return data, nil
}

func (c *testFileClipboard) write(paths []string, move bool) (browser.FileClipboard, error) {
	c.Lock()
	defer c.Unlock()
	c.data = browser.FileClipboard{Paths: slices.Clone(paths), Move: move, Sequence: c.data.Sequence + 1}
	return c.data, nil
}

func (c *testFileClipboard) complete(snapshot browser.FileClipboard, moved []string) (browser.FileClipboard, error) {
	c.Lock()
	defer c.Unlock()
	if c.data.Sequence != snapshot.Sequence {
		return c.data, nil
	}
	var remaining []string
	for _, path := range snapshot.Paths {
		if !slices.Contains(moved, path) {
			remaining = append(remaining, path)
		}
	}
	c.data = browser.FileClipboard{Paths: remaining, Move: true, Sequence: snapshot.Sequence + 1}
	return c.data, nil
}

func attachTestFileClipboard(v *view) *testFileClipboard {
	c := new(testFileClipboard)
	v.loader.readClipboard, v.loader.writeClipboard, v.loader.completeClipboard = c.read, c.write, c.complete
	v.clipboardSequence = func() uint32 { c.Lock(); defer c.Unlock(); return c.data.Sequence }
	return c
}

func clipboardView(t *testing.T) (*view, string, string) {
	t.Helper()
	source, target := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "folder", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"a.txt": "alpha", "z.txt": "keep", "folder/nested/file": "nested"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{source, target, source}, nil)
	t.Cleanup(v.loader.close)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	v.panels[2].path = source + string(filepath.Separator) + "."
	return v, source, target
}

func markClipboardItems(t *testing.T, p *panel) {
	t.Helper()
	for _, name := range []string{"folder", "a.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	selectName(t, p, "z.txt")
}

func TestClipboardCopiesMarkedMixedItemsIntoActiveFolder(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	markClipboardItems(t, &v.panels[0])
	if err := os.Mkdir(filepath.Join(target, "highlighted"), 0700); err != nil {
		t.Fatal(err)
	}
	v.load(1, target, "")
	waitLoaded(t, v, 1)
	selectName(t, &v.panels[1], "highlighted")
	v.panels[1].toggleMark(v.panels[1].selected)
	send, _ := panelInputFrames(v)
	send(modified("C", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	snapshot, err := c.read()
	want := []string{filepath.Join(source, "folder"), filepath.Join(source, "a.txt")}
	if err != nil || snapshot.Move || !reflect.DeepEqual(snapshot.Paths, want) {
		t.Fatalf("wrong clipboard selection: %+v %v", snapshot, err)
	}
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	for _, name := range []string{"a.txt", "folder/nested/file"} {
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(name))); err != nil {
			t.Fatal("paste did not use active folder:", err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "highlighted", "a.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("paste used highlighted subfolder")
	}
	p := &v.panels[1]
	if p.items[p.selected].file.Name != "highlighted" || len(p.marks) != 1 || !p.marks[p.selected] || v.active != 1 {
		t.Fatal("paste changed selection, marks or focus")
	}
	current, _ := c.read()
	if !reflect.DeepEqual(current, snapshot) || len(v.panels[0].marks) != 2 {
		t.Fatal("copy consumed clipboard or source marks")
	}
	if message := p.feedback.Current(); message == nil || message.Kind != feedback.Success || message.Text != "2 copied" {
		t.Fatalf("wrong copy feedback: %v", message)
	}
}

func TestClipboardCutMoveUpdatesAliasPanelsAndRetainsSurvivors(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	markClipboardItems(t, &v.panels[0])
	selectName(t, &v.panels[2], "z.txt")
	v.panels[2].toggleMark(v.panels[2].selected)
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	for _, pi := range []int{0, 2} {
		selectName(t, &v.panels[pi], "a.txt")
		if !v.cutEntry(pi, v.panels[pi].selected) {
			t.Fatalf("cut filename not dimmed in alias panel %d", pi)
		}
		selectName(t, &v.panels[pi], "z.txt")
	}
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	for _, pi := range []int{0, 2} {
		p := &v.panels[pi]
		if len(p.items) != 2 || p.items[p.selected].file.Name != "z.txt" {
			t.Fatalf("source panel %d lost surviving current item", pi)
		}
	}
	if len(v.panels[0].marks) != 0 || len(v.panels[2].marks) != 1 {
		t.Fatal("move lost surviving marks or retained moved marks")
	}
	if _, err := os.Stat(filepath.Join(source, "folder")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("cut source not removed")
	}
	if _, err := os.Stat(filepath.Join(target, "folder", "nested", "file")); err != nil {
		t.Fatal("cut destination incomplete")
	}
	if _, err := c.read(); !errors.Is(err, browser.ErrNoClipboardFiles) || len(v.cutClipboard.Paths) != 0 {
		t.Fatal("completed cut remained pending")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Text != "2 moved" {
		t.Fatalf("wrong move feedback: %v", message)
	}
}

func TestClipboardConflictRetainsCutSelectionForRetry(t *testing.T) {
	v, _, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	markClipboardItems(t, &v.panels[0])
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	before, _ := c.read()
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	after, _ := c.read()
	if !reflect.DeepEqual(before, after) || len(v.panels[0].marks) != 2 || len(v.cutClipboard.Paths) != 2 {
		t.Fatal("refusal consumed cut selection")
	}
	if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "original" {
		t.Fatal("refusal replaced existing file")
	}
	if _, err := os.Stat(filepath.Join(target, "folder")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("refusal transferred earlier folder")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Warning || message.Text != "1 file exists" || v.actions == nil {
		t.Fatalf("wrong conflict feedback: %v", message)
	}
	send(pressed(key.NameEscape))
	if err := os.Remove(filepath.Join(target, "a.txt")); err != nil {
		t.Fatal(err)
	}
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	if len(v.cutClipboard.Paths) != 0 {
		t.Fatal("retry did not complete retained cut")
	}
}

func TestClipboardPartialMoveRetainsFailedItemForRetry(t *testing.T) {
	v, _, _ := clipboardView(t)
	c := attachTestFileClipboard(v)
	markClipboardItems(t, &v.panels[0])
	source := v.panels[0].path
	v.loader.executeTransfer = func(plan *browser.TransferPlan, move bool, policy browser.TransferPolicy) (browser.TransferResult, error) {
		paths, directory := []string{filepath.Join(source, "folder"), filepath.Join(source, "a.txt")}, plan.Directory
		result, err := browser.Transfer(paths[:1], directory, move)
		if err != nil {
			return result, err
		}
		return result, &os.PathError{Op: "paste", Path: paths[1], Err: fs.ErrPermission}
	}
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	remaining, err := c.read()
	if err != nil || len(remaining.Paths) != 1 || filepath.Base(remaining.Paths[0]) != "a.txt" {
		t.Fatalf("partial clipboard: %+v %v", remaining, err)
	}
	selectName(t, &v.panels[0], "a.txt")
	if !v.cutEntry(0, v.panels[0].selected) || len(v.panels[0].marks) != 1 || !v.panels[0].marks[v.panels[0].selected] {
		t.Fatal("failed cut item lost its mark or dimming")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "1 transferred; Access denied" {
		t.Fatalf("wrong partial feedback: %v", message)
	}
}

func TestFileClipboardShortcutsSkipBusyDrivesAndEditors(t *testing.T) {
	for _, state := range []string{"busy", "drives", "parent", "location", "rename", "create"} {
		t.Run(state, func(t *testing.T) {
			v := newPreviewView()
			v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
			v.panels[0].clearMarks()
			router, send := clipboardFrames(v)
			switch state {
			case "busy":
				v.panels[0].loading = true
			case "drives":
				v.panels[0].drives = true
			case "parent":
				v.panels[0].selected = 0
			case "location":
				send(modified("D", key.ModAlt))
			case "rename":
				send(pressed(key.NameF2))
			case "create":
				send(newFolderKey())
			}
			if editor := v.activeEditor(); editor != nil {
				editor.SetText("text")
				editor.SetCaret(0, 4)
				send()
				send(modified("X", key.ModCtrl))
				_, data, ok := router.WriteClipboard()
				if !ok || string(data) != "text" || editor.Text() != "" {
					t.Fatal("Ctrl+X stopped cutting editor text")
				}
				send(modified("V", key.ModCtrl))
				if !router.ClipboardRequested() {
					t.Fatal("Ctrl+V stopped requesting editor text")
				}
				send(clipboardData("replacement"))
				if editor.Text() != "replacement" {
					t.Fatal("Ctrl+V stopped pasting editor text")
				}
				send(modified("D", key.ModCtrl))
			} else {
				send(modified("C", key.ModCtrl), modified("X", key.ModCtrl), modified("D", key.ModCtrl))
			}
			if len(v.loader.requests) != 0 {
				t.Fatal("ignored/editor clipboard shortcut queued file operation")
			}
		})
	}
}

func TestFilePasteSnapshotsTargetAndHasNoProgressUI(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl), modified("V", key.ModCtrl), pressed(key.NameRightArrow), modified("V", key.ModCtrl), pressed(key.NameF2))
	if len(v.loader.requests) != 1 || !v.transferPending || v.naming != nil {
		t.Fatal("pending paste duplicated or allowed a name mutation")
	}
	request := <-v.loader.requests
	if request.path != v.panels[0].path || request.panel != 0 || v.panels[0].feedback.Current() != nil {
		t.Fatal("paste used wrong target or introduced progress UI")
	}
	send(modified("D", key.ModAlt))
	v.location.SetText("draft path")
	v.location.SetCaret(1, 4)
	v.acceptLoad(loadResult{request: request, err: browser.ErrNoClipboardFiles})
	send()
	start, end := v.location.Selection()
	if v.transferPending || v.panels[0].loading || v.active != 1 || !v.editing || v.location.Text() != "draft path" || start != 1 || end != 4 {
		t.Fatal("completion stole focus or disturbed location editor")
	}
}

func TestReplacingClipboardClearsCutAppearance(t *testing.T) {
	v, _, _ := clipboardView(t)
	c := attachTestFileClipboard(v)
	selectName(t, &v.panels[0], "a.txt")
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	if !v.cutEntry(0, v.panels[0].selected) {
		t.Fatal("cut appearance missing")
	}
	c.Lock()
	c.data = browser.FileClipboard{Sequence: c.data.Sequence + 1}
	c.Unlock()
	send()
	if v.cutEntry(0, v.panels[0].selected) || len(v.cutClipboard.Paths) != 0 {
		t.Fatal("external clipboard replacement left stale cut appearance")
	}
}

func TestClipboardUsesExternalSelectionAndKeepsCopyReusable(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	paths := []string{filepath.Join(source, "a.txt")}
	if _, err := c.write(paths, false); err != nil {
		t.Fatal(err)
	}
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatal("external clipboard selection not pasted")
	}
	current, err := c.read()
	if err != nil || current.Move || !reflect.DeepEqual(current.Paths, paths) {
		t.Fatal("external copy clipboard was consumed")
	}
}

func TestMoveUpdatesOpenDescendantWithoutChangingItsListing(t *testing.T) {
	v, source, target := clipboardView(t)
	attachTestFileClipboard(v)
	descendant := filepath.Join(source, "folder", "nested")
	v.load(2, descendant, "")
	waitLoaded(t, v, 2)
	selectName(t, &v.panels[2], "file")
	v.panels[2].toggleMark(v.panels[2].selected)
	before := slices.Clone(v.panels[2].items)
	selectName(t, &v.panels[0], "folder")
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	p := &v.panels[2]
	if p.path != filepath.Join(target, "folder", "nested") || !slices.Equal(p.items, before) || p.items[p.selected].file.Name != "file" || !p.marks[p.selected] {
		t.Fatal("move stranded open descendant or changed its listing state")
	}
}

func TestMoveCompletionDoesNotChangeNewClipboardOrMisreportOperation(t *testing.T) {
	v, source, _ := clipboardView(t)
	c := attachTestFileClipboard(v)
	if _, err := c.write([]string{filepath.Join(source, "a.txt")}, true); err != nil {
		t.Fatal(err)
	}
	v.loader.executeTransfer = func(plan *browser.TransferPlan, move bool, policy browser.TransferPolicy) (browser.TransferResult, error) {
		result, err := browser.ExecuteTransfer(plan, move, policy)
		_, writeErr := c.write([]string{filepath.Join(source, "z.txt")}, false)
		return result, errors.Join(err, writeErr)
	}
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	current, err := c.read()
	if err != nil || current.Move || filepath.Base(current.Paths[0]) != "z.txt" {
		t.Fatal("completion replaced clipboard changed during transfer")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Text != "1 moved" {
		t.Fatalf("clipboard change misreported move: %v", message)
	}
}

func TestCopySwitchPasteFollowsInputOrder(t *testing.T) {
	v, source, target := clipboardView(t)
	attachTestFileClipboard(v)
	selectName(t, &v.panels[0], "a.txt")
	send, _ := panelInputFrames(v)
	send(modified("C", key.ModCtrl), pressed(key.NameTab), modified("V", key.ModCtrl))
	waitLoaded(t, v, 0)
	waitLoaded(t, v, 1)
	send()
	if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatal("same-frame copy/switch/paste lost source or target")
	}
	if v.active != 1 || v.transferPending {
		t.Fatal("completion lost active panel or stayed pending")
	}
	if _, err := os.Stat(filepath.Join(source, "a.txt")); err != nil {
		t.Fatal("copy removed source")
	}
}
