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

func TestSingleDuplicateUsesInlineNameEditor(t *testing.T) {
	for _, paste := range []bool{false, true} {
		for _, folder := range []bool{false, true} {
			v, root, _ := clipboardView(t)
			c := attachTestFileClipboard(v)
			name, suggestion, custom := "a.txt", "a - Copy.txt", "custom.txt"
			if folder {
				name, suggestion, custom = "folder", "folder - Copy", "custom folder"
			}
			selectName(t, &v.panels[0], name)
			v.panels[0].toggleMark(v.panels[0].selected)
			// Marked source wins even when another row is current.
			selectName(t, &v.panels[0], "z.txt")
			selectName(t, &v.panels[2], "z.txt")
			v.panels[2].toggleMark(v.panels[2].selected)
			clipboardPaths, move := []string{filepath.Join(root, "z.txt")}, true
			if paste {
				clipboardPaths, move = []string{filepath.Join(root, name)}, false
			}
			before, err := c.write(clipboardPaths, move)
			if err != nil {
				t.Fatal(err)
			}
			if !paste {
				v.rememberCutClipboard(before, v.fileOperationLocations(), v.fileOperationLocations())
				v.loader.readClipboard = func() (browser.FileClipboard, error) {
					t.Error("Ctrl+D read the clipboard")
					return browser.FileClipboard{}, errors.New("unexpected clipboard read")
				}
			}
			router, send := clipboardFrames(v)
			shortcut := "D"
			if paste {
				shortcut = "V"
			}
			send(modified(key.Name(shortcut), key.ModCtrl))
			waitLoaded(t, v, 0)
			send()
			if v.nameInteraction() == nil || v.nameInteraction().kind != nameDuplicate || v.nameEditor.Text() != suggestion || !router.Source().Focused(&v.nameEditor) {
				t.Fatalf("duplicate did not use name editor: paste=%v folder=%v naming=%+v text=%q", paste, folder, v.nameInteraction(), v.nameEditor.Text())
			}
			wantSelected := suggestion
			if !folder {
				wantSelected = "a - Copy"
			}
			if v.nameEditor.SelectedText() != wantSelected || v.panels[0].items[v.nameInteraction().row-1].name != name {
				t.Fatal("duplicate name selection or source anchor was wrong")
			}
			if _, err := os.Stat(filepath.Join(root, suggestion)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("copy was created before Enter")
			}
			send(pressed(key.NameTab), modified(key.NameTab, key.ModShift))
			if v.nameEditor.Text() != suggestion || !router.Source().Focused(&v.nameEditor) {
				t.Fatal("Tab disturbed duplicate editor")
			}
			send(replaceCreate(v, custom), pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			if folder {
				assertBatchFile(t, filepath.Join(root, custom, "nested", "file"), "nested")
				assertBatchFile(t, filepath.Join(root, name, "nested", "file"), "nested")
			} else {
				assertBatchFile(t, filepath.Join(root, custom), "alpha")
				assertBatchFile(t, filepath.Join(root, name), "alpha")
			}
			current, _ := c.read()
			if !reflect.DeepEqual(current, before) || v.nameInteraction() != nil || v.transferPending || v.panels[0].items[v.panels[0].selected].name != custom {
				t.Fatal("duplicate lost copy selection or changed clipboard")
			}
			if v.panels[2].items[v.panels[2].selected].name != "z.txt" || !v.panels[2].marks[v.panels[2].selected] {
				t.Fatal("duplicate changed alias panel selection or marks")
			}
			selectName(t, &v.panels[2], custom)
			if !paste && !reflect.DeepEqual(v.cutClipboard, before) {
				t.Fatal("Ctrl+D cleared cut appearance")
			}
		}
	}
}

func TestDuplicateCancelAndCollisionRetry(t *testing.T) {
	for _, cancel := range []string{"Escape", "click", "retry"} {
		v, root, _ := clipboardView(t)
		c := attachTestFileClipboard(v)
		before, _ := c.write([]string{filepath.Join(root, "z.txt")}, true)
		selectName(t, &v.panels[0], "a.txt")
		send, _ := panelInputFrames(v)
		send(modified("D", key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
		switch cancel {
		case "Escape":
			send(pressed(key.NameEscape))
		case "click":
			v.panels[2].rows[1].Click()
			send()
			if v.active != 2 || v.panels[2].selected != 1 {
				t.Fatal("click cancellation lost navigation")
			}
		case "retry":
			// An arrival after the suggestion must never be overwritten.
			if err := os.WriteFile(filepath.Join(root, "a - Copy.txt"), []byte("external"), 0600); err != nil {
				t.Fatal(err)
			}
			send(pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			if v.nameInteraction() == nil || v.nameEditor.Text() != "a - Copy.txt" || v.nameEditor.ReadOnly || v.panels[0].feedback.Current().Text != "Name already exists" {
				t.Fatal("collision did not retain editable draft")
			}
			assertBatchFile(t, filepath.Join(root, "a - Copy.txt"), "external")
			send(replaceCreate(v, "retry.txt"), pressed(key.NameReturn))
			waitCreated(t, v)
			send()
			assertBatchFile(t, filepath.Join(root, "retry.txt"), "alpha")
		}
		if v.nameInteraction() != nil || v.transferPending {
			t.Fatal("duplicate retained pending state after completion/cancellation")
		}
		current, _ := c.read()
		if !reflect.DeepEqual(current, before) {
			t.Fatal("cancel or collision retry changed clipboard")
		}
		assertBatchFile(t, filepath.Join(root, "a.txt"), "alpha")
		if cancel != "retry" {
			if _, err := os.Stat(filepath.Join(root, "a - Copy.txt")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("cancel created a copy")
			}
		}
	}
}

func TestBatchDuplicateAndSameFolderCutPaste(t *testing.T) {
	for _, shortcut := range []string{"D", "copy", "cut"} {
		v, root, _ := clipboardView(t)
		c := attachTestFileClipboard(v)
		markClipboardItems(t, &v.panels[0])
		paths := []string{filepath.Join(root, "folder"), filepath.Join(root, "a.txt")}
		before, _ := c.write(paths, shortcut == "cut")
		keyName := "V"
		if shortcut == "D" {
			keyName = "D"
		}
		send, _ := panelInputFrames(v)
		send(modified(key.Name(keyName), key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
		if v.nameInteraction() != nil || v.inlineInteraction() != nil || v.transferPending {
			t.Fatal("batch duplicate or same-folder cut retained a choice")
		}
		current, _ := c.read()
		if !reflect.DeepEqual(current, before) || len(v.panels[0].marks) != 2 {
			t.Fatal("batch changed clipboard or source marks")
		}
		if shortcut == "cut" {
			if v.panels[0].feedback.Current().Text != "Already in this folder" {
				t.Fatal("same-folder cut did not explain no-op")
			}
			if _, err := os.Stat(filepath.Join(root, "a - Copy.txt")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("same-folder cut created a duplicate")
			}
		} else {
			assertBatchFile(t, filepath.Join(root, "a - Copy.txt"), "alpha")
			assertBatchFile(t, filepath.Join(root, "folder - Copy", "nested", "file"), "nested")
			selectName(t, &v.panels[2], "folder - Copy")
			if v.panels[0].feedback.Current().Text != "2 copied" {
				t.Fatal("batch duplicate misreported completion")
			}
			// Repeat uses numbered names without replacing previous copies.
			send(modified(key.Name(keyName), key.ModCtrl))
			waitLoaded(t, v, 0)
			send()
			assertBatchFile(t, filepath.Join(root, "a - Copy (2).txt"), "alpha")
		}
		assertBatchFile(t, filepath.Join(root, "a.txt"), "alpha")
		assertBatchFile(t, filepath.Join(root, "folder", "nested", "file"), "nested")
	}
}

func TestDuplicateEditorKeepsUTF8BasenameSelection(t *testing.T) {
	v, root, _ := clipboardView(t)
	attachTestFileClipboard(v)
	name := "résumé.txt"
	if err := os.WriteFile(filepath.Join(root, name), []byte("utf8"), 0600); err != nil {
		t.Fatal(err)
	}
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], name)
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	first, last := v.nameEditor.Selection()
	if first != 0 || last != utf8.RuneCountInString("résumé - Copy") {
		t.Fatalf("wrong UTF8 selection: %d %d", first, last)
	}
	send(pressed(key.NameF2))
	if v.nameEditor.SelectedText() != "résumé - Copy.txt" {
		t.Fatal("F2 did not select whole copy name")
	}
}

func TestArrivingDuplicateDefersBehindEditorOrCancelsAfterLeaving(t *testing.T) {
	for _, leave := range []bool{false, true} {
		v, root, _ := clipboardView(t)
		c := attachTestFileClipboard(v)
		if _, err := c.write([]string{filepath.Join(root, "a.txt")}, false); err != nil {
			t.Fatal(err)
		}
		entered, release := make(chan struct{}), make(chan struct{})
		v.loader.readClipboard = func() (browser.FileClipboard, error) {
			close(entered)
			<-release
			return c.read()
		}
		router, send := clipboardFrames(v)
		send(modified("V", key.ModCtrl))
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("clipboard worker did not start")
		}
		if leave {
			send(pressed(key.NameRightArrow))
		}
		send(modified("D", key.ModAlt))
		v.location.SetText("draft location")
		v.location.SetCaret(1, 4)
		close(release)
		waitLoaded(t, v, 0)
		send()
		first, last := v.location.Selection()
		if !v.locationEditing() || v.nameInteraction() != nil || v.location.Text() != "draft location" || first != 1 || last != 4 || !router.Source().Focused(&v.location) {
			t.Fatal("arriving duplicate disturbed an active editor")
		}
		if (v.pendingDuplicate == nil) != leave || v.transferPending == leave {
			t.Fatal("arriving duplicate did not defer/cancel according to its pane")
		}
		send(pressed(key.NameEscape))
		if (v.nameInteraction() == nil) != leave {
			t.Fatal("duplicate did not resume only in its active pane")
		}
		if !leave {
			send(pressed(key.NameEscape))
		}
		if _, err := os.Stat(filepath.Join(root, "a - Copy.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("deferred/cancelled duplicate created a file")
		}
	}
}

func TestInlineDuplicatePreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless duplicate-name preview")
	}
	v := newPreviewView()
	v.active = 1
	p := &v.panels[1]
	p.selected = 8
	r := &nameSession{kind: nameDuplicate, panel: 1, row: 9, path: p.path,
		previousPosition: p.list.Position, previousReveal: p.reveal}
	var ops op.Ops
	v.beginName(layout.Context{Ops: &ops}, r, "go-mono - Copy.zip", false)
	if err := renderViewPNG(filepath.Join(directory, "inline-duplicate.png"), v); err != nil {
		t.Fatal(err)
	}
}
