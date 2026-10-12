package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func replaceRename(v *view, text string) key.EditEvent {
	return key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(v.nameEditor.Text())}, Text: text}
}

func waitRenamed(t *testing.T, v *view) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for v.nameInteraction() != nil && v.nameInteraction().pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-timer.C:
			t.Fatal("rename did not complete")
		}
	}
}

func TestRenameEditorSelectionAndCancel(t *testing.T) {
	for _, tc := range []struct {
		name, selection string
		directory       bool
	}{
		{"notes.txt", "notes", false}, {"archive.tar.gz", "archive.tar", false},
		{".gitignore", ".gitignore", false}, {"Ångström.md", "Ångström", false},
		{"folder.with.dot", "folder.with.dot", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newView([]panel{{items: []entry{{name: tc.name, file: browser.Entry{Name: tc.name, Directory: tc.directory}}}, selected: 0}, {}})
			v.panels[0].metadata = metadataExtension
			send, _ := panelInputFrames(v)
			send(pressed(key.NameF2))
			if v.nameEditor.Text() != tc.name {
				t.Fatal("Extension mode omitted part of the editable filename")
			}
			if v.nameInteraction() == nil || v.nameEditor.SelectedText() != tc.selection {
				t.Fatalf("initial selection %q", v.nameEditor.SelectedText())
			}
			send(key.Event{Name: key.NameTab, State: key.Press})
			send(key.Event{Name: key.NameTab, State: key.Press, Modifiers: key.ModShift})
			if v.active != 0 || v.nameEditor.SelectedText() != tc.selection {
				t.Fatal("Tab changed editor or panel")
			}
			send(pressed(key.NameLeftArrow))
			if v.active != 0 {
				t.Fatal("editor arrow switched panels")
			}
			send(pressed(key.NameF2))
			if v.nameEditor.SelectedText() != tc.name {
				t.Fatal("second F2 did not select extension")
			}
			send(key.Event{Name: key.NameF2, State: key.Release})
			send(replaceRename(v, "changed.ext"))
			send(pressed(key.NameLeftArrow))
			send(pressed(key.NameF2))
			if v.nameEditor.SelectedText() != "changed.ext" {
				t.Fatal("F2 did not select the current draft")
			}
			send(replaceRename(v, "changed"))
			send(pressed(key.NameEscape))
			if v.nameInteraction() != nil || v.panels[0].items[0].name != tc.name {
				t.Fatal("Escape changed entry or kept editor")
			}
			send(pressed(key.NameRightArrow))
			if v.active != 1 {
				t.Fatal("Escape did not restore browsing focus")
			}
		})
	}
}

func TestRenameSkipsNonEntries(t *testing.T) {
	for _, p := range []panel{
		{items: []entry{{name: "..", file: browser.Entry{Name: "..", Parent: true}}}, selected: 0},
		{drives: true, items: []entry{{name: "C:", file: browser.Entry{Name: "C:", Path: `C:\`}}}, selected: 0},
		{loading: true, items: []entry{{name: "x", file: browser.Entry{Name: "x"}}}, selected: 0},
		{selected: -1},
	} {
		v := newView([]panel{p})
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		if v.nameInteraction() != nil {
			t.Fatal("F2 edited a parent, drive, busy panel or empty list")
		}
	}
}

func TestRealRenamePreservesMarksAndUpdatesPanels(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha.txt", "middle.txt", "zulu.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root}, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	selectName(t, &v.panels[0], "alpha.txt")
	selectName(t, &v.panels[1], "middle.txt")
	v.panels[0].toggleMark(v.panels[0].selected)
	v.panels[1].toggleMark(v.panels[1].selected)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "zz-last.md"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if v.nameInteraction() != nil || v.panels[0].items[v.panels[0].selected].name != "zz-last.md" || !v.panels[0].marks[v.panels[0].selected] {
		t.Fatal("renamed item lost focus or mark")
	}
	if v.panels[1].items[v.panels[1].selected].name != "middle.txt" || !v.panels[1].marks[v.panels[1].selected] {
		t.Fatal("other panel selection/marks changed")
	}
	selectName(t, &v.panels[1], "zz-last.md")
	if data, err := os.ReadFile(filepath.Join(root, "zz-last.md")); err != nil || string(data) != "alpha.txt" {
		t.Fatalf("file rename: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("source still exists")
	}
	send(pressed(key.NameUpArrow))
	if v.panels[0].items[v.panels[0].selected].name != "zulu.txt" {
		t.Fatal("rename did not restore list focus/order")
	}
}

func TestRenameFailureRetainsEditorAndCanRetry(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root}, nil)
	defer v.loader.close()
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], "a.txt")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "b.txt"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if v.nameInteraction() == nil || !errors.Is(v.nameInteraction().err, fs.ErrExist) || v.nameEditor.ReadOnly {
		t.Fatal("collision did not retain editable field")
	}
	if message := v.panels[0].feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "Name already exists" {
		t.Fatal("collision did not use the animated feedback channel")
	}
	send(replaceRename(v, "../outside"))
	send(pressed(key.NameReturn))
	if v.nameInteraction() == nil || !errors.Is(v.nameInteraction().err, browser.ErrInvalidName) || v.nameInteraction().pending {
		t.Fatal("path was accepted as filename")
	}
	send(replaceRename(v, "new.txt"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if v.nameInteraction() != nil || v.panels[0].items[v.panels[0].selected].name != "new.txt" {
		t.Fatal("retry failed")
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "b.txt" {
		t.Fatal("collision overwrote destination")
	}
}

func TestRenameFolderUpdatesOpenDescendant(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "folder", "child")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, child}, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	selectName(t, &v.panels[0], "folder")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "renamed"))
	send(pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if v.panels[1].path != filepath.Join(root, "renamed", "child") {
		t.Fatal("open descendant retained old path")
	}
}

func TestPendingRenameKeepsUIResponsiveAndCannotBeCancelled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "old.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, err := browser.ReadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	v := newView([]panel{{path: root}, {path: root}})
	for i := range v.panels {
		v.acceptLoad(loadResult{request: loadRequest{panel: i}, directory: directory})
	}
	selectName(t, &v.panels[0], "old.txt")
	blocked, release := make(chan struct{}), make(chan struct{})
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		close(blocked)
		<-release
		return browser.ReadDirectory(path)
	}, nil)
	defer v.loader.close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	v.load(1, root, "")
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "new.txt"))
	send(pressed(key.NameReturn))
	if v.nameInteraction() == nil || !v.nameInteraction().pending {
		t.Fatal("submission was not queued")
	}
	// The worker is blocked. Rendering, Escape, repeated Enter and mouse input
	// must still return, without claiming the submitted mutation was cancelled.
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	v.panels[1].rows[0].Click()
	send()
	if v.nameInteraction() == nil || !v.nameInteraction().pending || v.active != 0 {
		t.Fatal("pending mutation was cancelled or duplicated")
	}
	close(release)
	released = true
	waitRenamed(t, v)
	send()
	if v.nameInteraction() != nil || v.panels[0].items[v.panels[0].selected].name != "new.txt" {
		t.Fatal("queued mutation did not finish")
	}
}

func TestClickCancelsRenameDraft(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "draft"))
	v.panels[1].rows[1].Click()
	send()
	if v.nameInteraction() != nil || v.active != 1 || v.panels[0].items[3].name != "commando" {
		t.Fatal("row click did not discard the draft")
	}
	send(pressed(key.NameDownArrow))
	if v.panels[1].selected != 2 {
		t.Fatal("row click did not restore browsing")
	}
}
