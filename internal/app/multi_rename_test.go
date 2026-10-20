package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
)

func multiRenameView() *view {
	v := newView([]panel{{path: "/sample", selected: 0, items: []entry{
		{name: "a.txt", file: browser.Entry{Name: "a.txt"}},
		{name: "b.txt", file: browser.Entry{Name: "b.txt"}},
		{name: "taken.txt", file: browser.Entry{Name: "taken.txt"}},
	}}})
	v.panels[0].marks = map[int]bool{0: true, 1: true}
	return v
}

func replaceExpression(v *view, text string) key.EditEvent {
	return key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(v.multiRenameInteraction().editor.Text())}, Text: text}
}

func TestMultiRenamePreviewAndCancel(t *testing.T) {
	v := multiRenameView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	s := v.multiRenameInteraction()
	if s == nil || v.nameInteraction() != nil || s.editor.Text() != "<name>" || !s.actions.isDisabled(0) {
		t.Fatal("F2 did not open a no-op expression widget")
	}
	send(replaceExpression(v, `<counter> <name>`))
	if !reflect.DeepEqual(s.preview, map[string]string{"a.txt": "1 a.txt", "b.txt": "2 b.txt"}) || s.actions.isDisabled(0) {
		t.Fatalf("unexpected preview: %v, %s", s.preview, s.message)
	}
	if v.panels[0].items[0].name != "a.txt" || len(v.panels[0].marks) != 2 || v.panels[0].selected != 0 {
		t.Fatal("preview changed listing, marks, selection or order")
	}
	send(pressed(key.NameTab))
	if s.focus != renameFocusInsert {
		t.Fatal("Tab did not focus the inserter")
	}
	send(pressed(key.NameTab))
	if s.focus != renameFocusApply || v.active != 0 {
		t.Fatal("Tab did not focus Apply")
	}
	send(pressed(key.NameTab))
	if s.focus != renameFocusCancel {
		t.Fatal("Tab did not focus Cancel")
	}
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || len(v.panels[0].marks) != 2 || v.panels[0].items[0].name != "a.txt" {
		t.Fatal("Cancel changed files or marks")
	}
	send(pressed(key.NameF2))
	send(replaceExpression(v, `<name case:upper>`), pressed(key.NameEscape))
	if !v.interaction.idle() {
		t.Fatal("Escape did not cancel")
	}
}

func TestMultiRenameValidationExamples(t *testing.T) {
	for _, example := range []struct {
		expression, message string
		invalid             []string
	}{
		{`<name from:0>`, "Invalid expression", nil},
		{`<name`, "Invalid expression", nil},
		{`CON`, "Invalid filename", []string{"a.txt", "b.txt"}},
		{`same.txt`, "Conflicting filenames", []string{"a.txt", "b.txt"}},
		{`taken.txt`, "Conflicting filenames", []string{"a.txt", "b.txt", "taken.txt"}},
		{`<name replace:a with:b>`, "Conflicting filenames", []string{"a.txt", "b.txt"}},
		{`<base case:upper>.<ext>`, "", nil},
		{`<name>.`, "", nil},
	} {
		t.Run(example.expression, func(t *testing.T) {
			v := multiRenameView()
			send, _ := panelInputFrames(v)
			send(pressed(key.NameF2))
			send(replaceExpression(v, example.expression))
			s := v.multiRenameInteraction()
			if s.message != example.message {
				t.Fatalf("got %q, want %q", s.message, example.message)
			}
			for _, name := range example.invalid {
				if !s.invalid[name] {
					t.Errorf("%s was not marked invalid", name)
				}
			}
			if example.message != "" && !s.actions.isDisabled(0) {
				t.Fatal("invalid expression allowed Apply")
			}
			if example.message == "Invalid expression" && s.preview != nil {
				t.Fatal("expression error retained previews")
			}
			if v.panels[0].feedback.Current() != nil {
				t.Fatal("draft validation leaked into panel notifications")
			}
			send(replaceExpression(v, `new-<name>`))
			if s.message != "" || len(s.invalid) != 0 || s.actions.isDisabled(0) {
				t.Fatal("corrected expression retained an error")
			}
		})
	}
}

func TestMultiRenameChecksCompleteCache(t *testing.T) {
	v := multiRenameView()
	v.panels[0].ensureEntries()
	v.panels[0].entries = append(v.panels[0].entries, browser.Entry{Name: "HIDDEN.TXT", Hidden: true})
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceExpression(v, `<name replace:a with:hidden>`))
	s := v.multiRenameInteraction()
	if s.message != "Conflicting filenames" || !s.invalid["a.txt"] || !s.invalid["HIDDEN.TXT"] {
		t.Fatal("preview missed a hidden or filtered case-insensitive destination")
	}
}

func TestMultiRenamePendingOwnershipAndInput(t *testing.T) {
	v := multiRenameView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceExpression(v, `new-<name>`), modified(key.NameReturn, key.ModCtrl))
	s := v.multiRenameInteraction()
	if s == nil || !s.pending || !v.mutationsBusy() || !s.editor.ReadOnly || v.requestClose() {
		t.Fatal("Apply did not hold foreground and close ownership")
	}
	request := <-v.loader.requests
	send(pressed(key.NameEscape), modified(key.NameReturn, key.ModCtrl))
	s.actions.buttons[1].Click()
	send()
	if v.multiRenameInteraction() != s || !s.pending || len(v.loader.requests) != 0 {
		t.Fatal("pending mutation was cancelled or resubmitted")
	}
	v.acceptLoad(loadResult{request: request, err: fs.ErrPermission})
	send()
	if v.multiRenameInteraction() != s || s.pending || s.editor.ReadOnly || v.mutationsBusy() {
		t.Fatal("failed mutation did not retain an editable expression")
	}
	send(replaceExpression(v, `<counter> <name>`))
	if v.panels[0].feedback.Current() != nil {
		t.Fatal("editing retained stale operation feedback")
	}
	s.actions.buttons[1].Click()
	send()
	if !v.interaction.idle() {
		t.Fatal("Cancel button did not release the widget")
	}
}

func TestMultiRenameStaleCompletionKeepsNewOwner(t *testing.T) {
	v := multiRenameView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	old := v.multiRenameInteraction()
	send(pressed(key.NameEscape))
	send(pressed(key.NameF2))
	current := v.multiRenameInteraction()
	v.acceptLoad(loadResult{request: loadRequest{multiRename: &multiRenameRequest{session: old}}})
	if current == nil || current == old || v.multiRenameInteraction() != current {
		t.Fatal("stale completion ended the new session")
	}
}

func waitMultiRenamed(t *testing.T, v *view) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for v.multiRenameInteraction() != nil && v.multiRenameInteraction().pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-timer.C:
			t.Fatal("batch rename did not complete")
		}
	}
}

func TestMultiRenameWorkerExamples(t *testing.T) {
	for _, mode := range []string{"success", "late conflict", "missing source", "partial"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"a.txt", "b.txt"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			v := newBrowserView([]string{root, root}, nil)
			defer v.loader.close()
			for pi := range v.panels {
				waitLoaded(t, v, pi)
				selectName(t, &v.panels[pi], "a.txt")
				v.panels[pi].toggleMark(v.panels[pi].selected)
			}
			selectName(t, &v.panels[0], "b.txt")
			v.panels[0].toggleMark(v.panels[0].selected)
			selectName(t, &v.panels[0], "a.txt")
			send, _ := panelInputFrames(v)
			send(pressed(key.NameF2))
			send(replaceExpression(v, `new-<name>`))
			switch mode {
			case "late conflict":
				if err := os.WriteFile(filepath.Join(root, "new-b.txt"), []byte("untouched"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing source":
				if err := os.Remove(filepath.Join(root, "b.txt")); err != nil {
					t.Fatal(err)
				}
			case "partial":
				v.loader.renameEntry = func(path, oldName, newName string) error {
					if oldName == "b.txt" {
						return fs.ErrPermission
					}
					return browser.Rename(path, oldName, newName)
				}
			}
			send(modified(key.NameReturn, key.ModCtrl))
			waitMultiRenamed(t, v)
			send()
			if v.mutationsBusy() {
				t.Fatal("worker stayed busy after reconciliation")
			}
			if mode == "late conflict" || mode == "missing source" {
				if v.multiRenameInteraction() == nil {
					t.Fatal("preflight error dismissed the expression")
				}
				if _, err := os.Stat(filepath.Join(root, "new-a.txt")); !errors.Is(err, fs.ErrNotExist) {
					t.Fatal("preflight error partly renamed files")
				}
				if mode == "late conflict" {
					data, err := os.ReadFile(filepath.Join(root, "new-b.txt"))
					if err != nil || string(data) != "untouched" {
						t.Fatal("conflicting destination was overwritten")
					}
				}
				return
			}
			if !v.interaction.idle() {
				t.Fatal("completed batch retained its widget")
			}
			for pi := range v.panels {
				p := &v.panels[pi]
				if p.items[p.selected].file.Name != "new-a.txt" || !p.marks[p.selected] {
					t.Fatal("rename lost selection or marks in an affected panel")
				}
			}
			if data, err := os.ReadFile(filepath.Join(root, "new-a.txt")); err != nil || string(data) != "a.txt" {
				t.Fatal("first rename lost contents")
			}
			if mode == "partial" {
				if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
					t.Fatal("failed item did not survive")
				}
				if message := v.panels[0].feedback.Current(); message == nil || message.Text != "1 renamed; Access denied" {
					t.Fatal("missing partial-result feedback")
				}
			} else if data, err := os.ReadFile(filepath.Join(root, "new-b.txt")); err != nil || string(data) != "b.txt" {
				t.Fatal("second rename lost contents")
			}
		})
	}
}

func TestMultiRenameFolderFollowsDescendant(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "folder.v2", "child")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, child}, nil)
	defer v.loader.close()
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	for _, name := range []string{"folder.v2", "a.txt"} {
		selectName(t, &v.panels[0], name)
		v.panels[0].toggleMark(v.panels[0].selected)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceExpression(v, `<base>-new.<ext>`), modified(key.NameReturn, key.ModCtrl))
	waitMultiRenamed(t, v)
	send()
	if v.panels[1].path != filepath.Join(root, "folder.v2-new", "child") || v.panels[1].err != nil {
		t.Fatal("folder batch did not follow an open descendant")
	}
}

func TestMultiRenameNoOpAndApplyButton(t *testing.T) {
	v := multiRenameView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2), modified(key.NameReturn, key.ModCtrl))
	v.multiRenameInteraction().actions.buttons[0].Click()
	send()
	if len(v.loader.requests) != 0 {
		t.Fatal("no-op expression submitted a mutation")
	}
	send(replaceExpression(v, `new-<name>`))
	s := v.multiRenameInteraction()
	send(pressed(key.NameReturn), pressed(key.NameReturn))
	if len(v.loader.requests) != 0 || s.focus != 0 {
		t.Fatal("expression Enter unexpectedly applied or left the editor")
	}
	s.actions.buttons[0].Click()
	send()
	if !s.pending || len(v.loader.requests) != 1 {
		t.Fatal("Apply button did not submit the expression")
	}
}

func TestMultiRenameAliasesKeepSelectionAndFollowFolders(t *testing.T) {
	for _, sourceAlias := range []bool{false, true} {
		v, root, alias := nameAliasView(t, true, sourceAlias)
		for _, name := range []string{"folder", "a.txt"} {
			selectName(t, &v.panels[0], name)
			v.panels[0].toggleMark(v.panels[0].selected)
		}
		selectName(t, &v.panels[1], "middle.txt")
		v.panels[1].toggleMark(v.panels[1].selected)
		originalPath := v.panels[1].path
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		send(replaceExpression(v, `new-<name>`), modified(key.NameReturn, key.ModCtrl))
		waitMultiRenamed(t, v)
		send()
		p := &v.panels[1]
		if p.path != originalPath || p.items[p.selected].name != "middle.txt" || !p.marks[p.selected] {
			t.Fatal("batch disturbed an alias's independent state")
		}
		// Match single rename: descendants sharing the source spelling retain
		// that spelling; other aliases follow the resolved destination.
		if sourceAlias && v.panels[2].path != filepath.Join(alias, "new-folder", "nested") {
			t.Fatal("batch lost the source alias for an open descendant")
		}
		if resolved, err := resolveFileLocation(v.panels[2].path); err != nil || resolved != filepath.Join(root, "new-folder", "nested") {
			t.Fatal("batch stranded an alias descendant", err)
		}
	}
}

func TestMultiRenamePreviewPNG(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_RENAME")
	if dir == "" {
		t.Skip("optional rename preview")
	}
	v := newPreviewView()
	v.panels = v.panels[:1]
	v.appearance.Feedback.Enabled = false // Render steady messages in the snapshots.
	var ops op.Ops
	v.startMultiRename(layout.Context{Ops: &ops})
	s := v.multiRenameInteraction()
	s.editor.SetText(`<counter pad:0> <name>`)
	v.previewMultiRename(s)
	if err := renderViewPNG(filepath.Join(dir, "multi-rename.png"), v); err != nil {
		t.Fatal(err)
	}
	s.editor.SetText(`same`)
	v.previewMultiRename(s)
	if err := renderViewPNG(filepath.Join(dir, "multi-rename-conflicts.png"), v); err != nil {
		t.Fatal(err)
	}
	s.editor.SetText(`<name`)
	v.previewMultiRename(s)
	if err := renderViewPNG(filepath.Join(dir, "multi-rename-invalid.png"), v); err != nil {
		t.Fatal(err)
	}
}
