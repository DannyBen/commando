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
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func singleFileConflictView(t *testing.T, move bool) (*view, *testFileClipboard, string, string) {
	t.Helper()
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	for name, data := range map[string]string{"a.txt": "old version", "b.txt": "other"} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v.load(1, target, "")
	waitLoaded(t, v, 1)
	selectName(t, &v.panels[0], "a.txt")
	selectName(t, &v.panels[1], "a.txt")
	v.panels[1].toggleMark(v.panels[1].selected)
	selectName(t, &v.panels[1], "b.txt")
	keyName := key.Name("C")
	if move {
		keyName = "X"
	}
	send, _ := panelInputFrames(v)
	send(modified(keyName, key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	send(pressed(key.NameRightArrow), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	if v.actions == nil || v.actions.panel != 1 || v.actions.selected != 0 || !slices.Equal(v.actions.options, []string{"Overwrite", "Cancel"}) || !v.transferPending || v.panels[1].loading {
		t.Fatal("single-file collision did not offer Overwrite selected and Cancel inline")
	}
	if len(v.actions.headers) != 0 || v.actions.count() != 2 || v.actions.subjectRow != v.actions.row-1 || v.panels[1].items[v.actions.subjectRow].file.Name != "a.txt" {
		t.Fatal("conflict actions were not attached directly to their file without headings")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Warning || message.Text != "File exists" {
		t.Fatal("single conflict did not explain itself briefly in the footer")
	}
	// Each caller installs a fresh input router for the interaction under test.
	v.started = false
	return v, c, source, target
}

func TestSingleFileConflictChoosingCancelPreservesFilesAndClipboard(t *testing.T) {
	v, c, source, target := singleFileConflictView(t, true)
	before, _ := c.read()
	items := slices.Clone(v.panels[1].items)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDownArrow), pressed(key.NameReturn))
	if v.actions != nil || v.transferPending || v.panels[1].loading {
		t.Fatal("Cancel retained pending state")
	}
	for path, want := range map[string]string{filepath.Join(source, "a.txt"): "alpha", filepath.Join(target, "a.txt"): "old version"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatal("Cancel changed files")
		}
	}
	current, _ := c.read()
	if !reflect.DeepEqual(current, before) || !slices.Equal(v.panels[1].items, items) || v.panels[1].items[v.panels[1].selected].file.Name != "a.txt" || len(v.panels[1].marks) != 1 || !v.panels[1].marks[v.panels[1].selected] {
		t.Fatal("Cancel lost clipboard, list state or target selection")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Info || message.Text != "Transfer cancelled" {
		t.Fatal("Cancel did not report result")
	}
}

func TestSingleFileConflictEnterOverwritesCopiesAndMoves(t *testing.T) {
	for _, move := range []bool{false, true} {
		v, c, source, target := singleFileConflictView(t, move)
		v.load(2, target, "")
		waitLoaded(t, v, 2)
		v.panels[2].path = target + string(filepath.Separator) + "."
		selectName(t, &v.panels[2], "a.txt")
		v.panels[2].toggleMark(v.panels[2].selected)
		selectName(t, &v.panels[2], "b.txt")
		send, _ := panelInputFrames(v)
		send(pressed(key.NameReturn))
		if v.panels[1].items[v.panels[1].selected].file.Name != "a.txt" {
			t.Fatal("Overwrite did not select the target when closing its actions")
		}
		waitLoaded(t, v, 1)
		send()
		if v.actions != nil || v.transferPending || v.active != 1 {
			t.Fatal("overwrite retained interaction or stole focus")
		}
		if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "alpha" {
			t.Fatal("overwrite did not publish source contents")
		}
		for _, pi := range []int{1, 2} {
			p := &v.panels[pi]
			for row, item := range p.items {
				if item.file.Name == "a.txt" && (item.file.Bytes != 5 || item.size != "5 B" || !p.marks[row]) {
					t.Fatalf("overwrite lost destination metadata or marks in panel %d", pi)
				}
			}
		}
		if v.panels[1].items[v.panels[1].selected].file.Name != "a.txt" || v.panels[2].items[v.panels[2].selected].file.Name != "b.txt" {
			t.Fatal("overwrite lost target selection or changed another panel's current item")
		}
		_, err := os.Stat(filepath.Join(source, "a.txt"))
		if move && !errors.Is(err, fs.ErrNotExist) || !move && err != nil {
			t.Fatalf("overwrite source retention: %v", err)
		}
		clip, err := c.read()
		if move && !errors.Is(err, browser.ErrNoClipboardFiles) || !move && (err != nil || len(clip.Paths) != 1) {
			t.Fatal("overwrite consumed wrong clipboard state")
		}
	}
}

func TestSingleFileConflictCancelAndEscapeLeaveDestinationUntouched(t *testing.T) {
	for _, escape := range []bool{false, true} {
		v, c, _, target := singleFileConflictView(t, false)
		before, _ := c.read()
		send, _ := panelInputFrames(v)
		if escape {
			send(pressed(key.NameEscape))
		} else {
			send(pressed(key.NameEnd), pressed(key.NameReturn))
		}
		if v.actions != nil || v.transferPending {
			t.Fatal("Cancel retained pending state")
		}
		if v.panels[1].items[v.panels[1].selected].file.Name != "a.txt" {
			t.Fatal("Cancel or Escape did not select the target")
		}
		if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "old version" {
			t.Fatal("Cancel replaced destination")
		}
		current, _ := c.read()
		if !reflect.DeepEqual(current, before) {
			t.Fatal("Cancel changed clipboard")
		}
		if message := v.panels[1].feedback.Current(); message == nil || message.Text != "Transfer cancelled" {
			t.Fatal("Cancel did not report result")
		}
	}
}

func TestPendingInlineConflictOwnsItsKeys(t *testing.T) {
	v, _, _, _ := singleFileConflictView(t, false)
	selected := v.panels[1].selected
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2), modified("N", key.ModCtrl), pressed(key.NameDeleteForward), modified("V", key.ModCtrl), modified("A", key.ModCtrl), key.EditEvent{Text: "overwrite"})
	if v.actions == nil || v.naming != nil || v.panels[1].selected != selected || len(v.panels[1].marks) != 1 || v.panels[1].prefix != "" || len(v.loader.requests) != 0 {
		t.Fatal("pending choice leaked file commands or changed list state")
	}
	send(pressed(key.NameEscape))
	if v.actions != nil || v.transferPending {
		t.Fatal("pending choice could not cancel")
	}
}

func TestLeavingConflictPanelCancelsTransfer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		event  key.Event
		target int
	}{
		{"Left", pressed(key.NameLeftArrow), 0},
		{"Right", pressed(key.NameRightArrow), 2},
		{"Tab", pressed(key.NameTab), 2},
		{"ShiftTab", modified(key.NameTab, key.ModShift), 0},
		{"Mouse", key.Event{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, c, source, target := singleFileConflictView(t, true)
			before, _ := c.read()
			send, _ := panelInputFrames(v)
			if tc.name == "Mouse" {
				v.panels[0].rows[1].Click()
				send()
				if v.panels[0].selected != 1 {
					t.Fatal("panel switch lost clicked row selection")
				}
			} else {
				send(tc.event)
			}
			current, _ := c.read()
			if v.active != tc.target || v.actions != nil || v.transferPending || v.panels[1].loading || !reflect.DeepEqual(current, before) {
				t.Fatal("leaving pane did not cancel transfer and preserve clipboard")
			}
			if p := &v.panels[1]; p.items[p.selected].name != "a.txt" || !p.marks[p.selected] {
				t.Fatal("leaving pane lost subject selection or marks")
			}
			assertBatchFile(t, filepath.Join(source, "a.txt"), "alpha")
			assertBatchFile(t, filepath.Join(target, "a.txt"), "old version")
			if message := v.panels[1].feedback.Current(); message == nil || message.Text != "Transfer cancelled" {
				t.Fatal("leaving pane did not report cancellation")
			}
			// Returning to the target cannot revive the old decision.
			if tc.target == 0 {
				send(pressed(key.NameRightArrow))
			} else {
				send(pressed(key.NameLeftArrow))
			}
			if v.actions != nil || v.transferPending {
				t.Fatal("navigation revived a cancelled transfer")
			}
		})
	}
}

func TestOverwriteDecisionUsesPresentedSnapshotAndPreservesNewClipboard(t *testing.T) {
	v, c, source, target := singleFileConflictView(t, true)
	newer, err := c.write([]string{filepath.Join(source, "z.txt")}, false)
	if err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "alpha" {
		t.Fatal("decision used a later clipboard selection")
	}
	current, err := c.read()
	if err != nil || !reflect.DeepEqual(current, newer) {
		t.Fatal("overwrite cleared newer clipboard")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Text != "1 moved" {
		t.Fatal("new clipboard misreported presented operation")
	}
}

func TestChangedFileRequiresNewOverwriteDecision(t *testing.T) {
	v, _, _, target := singleFileConflictView(t, false)
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("external change"), 0600); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	if v.actions == nil || v.actions.selected != 0 || !v.transferPending {
		t.Fatal("changed file did not require a fresh overwrite decision")
	}
	if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "external change" {
		t.Fatal("stale confirmation overwrote external change")
	}
	if message := v.panels[1].feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "Files changed; choose again" {
		t.Fatal("changed file was not explained")
	}
}

func TestConflictArrivingDuringLocationEditingDoesNotStealFocus(t *testing.T) {
	for _, leave := range []bool{false, true} {
		t.Run(fmt.Sprint("leave=", leave), func(t *testing.T) {
			v, source, target := clipboardView(t)
			c := attachTestFileClipboard(v)
			if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := c.write([]string{filepath.Join(source, "a.txt")}, false); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			v.loader.planTransfer = func(paths []string, directory string) (*browser.TransferPlan, error) {
				close(entered)
				<-release
				return browser.PlanTransfer(paths, directory)
			}
			v.active = 1
			router, send := clipboardFrames(v)
			send(modified("V", key.ModCtrl))
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("transfer worker did not start")
			}
			if leave {
				send(pressed(key.NameLeftArrow))
			}
			send(modified("D", key.ModAlt))
			v.location.SetText("draft path")
			v.location.SetCaret(1, 4)
			close(release)
			waitLoaded(t, v, 1)
			send()
			first, last := v.location.Selection()
			if v.actions != nil || !v.editing || v.location.Text() != "draft path" || first != 1 || last != 4 || !router.Source().Focused(&v.location) {
				t.Fatal("conflict stole editor text, selection or focus")
			}
			if leave {
				if v.active != 0 || v.pendingFileConflict != nil || v.transferPending {
					t.Fatal("arriving conflict retained a decision after leaving its pane")
				}
				assertBatchFile(t, filepath.Join(target, "a.txt"), "old")
			} else if v.active != 1 || v.pendingFileConflict == nil || !v.transferPending {
				t.Fatal("arriving conflict was not deferred behind its pane's editor")
			}
			send(pressed(key.NameEscape))
			if (v.actions == nil) != leave || v.pendingFileConflict != nil || !router.Source().Focused(&v.focus) {
				t.Fatal("deferred conflict did not appear without stealing panel focus")
			}
		})
	}
}

func TestExistingFolderMergesWithoutFileConflictChoice(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	if err := os.Mkdir(filepath.Join(target, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := c.write([]string{filepath.Join(source, "folder")}, false); err != nil {
		t.Fatal(err)
	}
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	if v.actions != nil || v.transferPending {
		t.Fatal("existing folder alone required a conflict decision")
	}
	if data, err := os.ReadFile(filepath.Join(target, "folder", "nested", "file")); err != nil || string(data) != "nested" {
		t.Fatal("existing folder did not merge its new content")
	}
}

func TestInlineConflictIncludesFileMissingFromCachedListing(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	if err := os.WriteFile(filepath.Join(target, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	v.load(1, target, "")
	waitLoaded(t, v, 1)
	selectName(t, &v.panels[1], "keep.txt")
	v.panels[1].toggleMark(v.panels[1].selected)
	// The conflict arrives after the last read of the destination listing.
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("old version"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.write([]string{filepath.Join(source, "a.txt")}, false); err != nil {
		t.Fatal(err)
	}
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	p, a := &v.panels[1], v.actions
	if a == nil || len(a.headers) != 0 || a.subjectRow != a.row-1 || p.items[a.subjectRow].file.Name != "a.txt" || p.items[a.subjectRow].file.Bytes != 11 {
		t.Fatal("compact actions appeared without their real conflicting file")
	}
	if p.items[p.selected].file.Name != "keep.txt" || len(p.marks) != 1 || !p.marks[p.selected] {
		t.Fatal("discovering conflict changed existing current item or marks")
	}
	send(pressed(key.NameEscape))
	if data, err := os.ReadFile(filepath.Join(target, "a.txt")); err != nil || string(data) != "old version" {
		t.Fatal("displaying missing conflict changed its contents")
	}
}
