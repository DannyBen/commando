package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func newFolderKey() key.Event { return modified("N", key.ModCtrl|key.ModShift) }

func replaceCreate(v *view, text string) key.EditEvent {
	return key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(v.nameEditor.Text())}, Text: text}
}

func waitCreated(t *testing.T, v *view) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for v.nameInteraction() != nil && v.nameInteraction().pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-timer.C:
			t.Fatal("folder creation did not complete")
		}
	}
}

func TestInlineCreateDraftAndCancel(t *testing.T) {
	v := newPreviewView()
	router, send := clipboardFrames(v)
	p := &v.panels[0]
	before, selected, position := slices.Clone(p.items), p.selected, p.list.Position
	marks := make(map[int]bool)
	for row, marked := range p.marks {
		marks[row] = marked
	}
	send(pressed("N"), key.Event{Name: "N", State: key.Release, Modifiers: key.ModCtrl | key.ModShift})
	if v.nameInteraction() != nil {
		t.Fatal("creation opened without shortcut press")
	}
	send(newFolderKey())
	if v.nameInteraction() == nil || !router.Source().Focused(&v.nameEditor) || v.nameEditor.Text() != "" {
		t.Fatal("shortcut did not focus an empty inline editor")
	}
	draft := v.nameInteraction().row
	if draft != selected+1 || v.storedRow(0, draft) != -1 || !slices.Equal(p.items, before) || !reflect.DeepEqual(p.marks, marks) {
		t.Fatal("draft was not a separate visual row after the current item")
	}
	send(replaceCreate(v, "draft"), modified(key.NameTab, key.ModShift), pressed(key.NameTab))
	send(refreshKey(), modified("D", key.ModAlt), newFolderKey())
	if v.activeEditor() != &v.nameEditor || v.nameEditor.Text() != "draft" || !router.Source().Focused(&v.nameEditor) || v.nameInteraction().row != draft {
		t.Fatal("shortcut, typing or Tab moved the draft/editor")
	}
	send(pressed(key.NameF2))
	if v.nameEditor.SelectedText() != "draft" {
		t.Fatal("F2 did not select the entire draft")
	}
	send(pressed(key.NameEscape))
	if v.nameInteraction() != nil || !router.Source().Focused(&v.focus) || !slices.Equal(p.items, before) || !reflect.DeepEqual(p.marks, marks) || p.selected != selected || p.list.Position != position {
		t.Fatal("cancellation did not restore listing, marks, selection, scroll or focus")
	}
	send(pressed(key.NameRightArrow))
	if v.active != 1 {
		t.Fatal("cancellation did not restore keyboard navigation")
	}
}

func TestClickCancelsInlineCreateDraft(t *testing.T) {
	for _, otherPanel := range []bool{false, true} {
		v := newPreviewView()
		send, _ := panelInputFrames(v)
		originalPosition := v.panels[0].list.Position
		send(newFolderKey())
		send(replaceCreate(v, "draft"))
		if otherPanel {
			point := f32.Pt(330, 110)
			send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: point})
			send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
			if v.active != 1 || v.panels[1].selected != 1 || v.panels[0].list.Position != originalPosition {
				t.Fatal("click did not cancel and switch to the other panel")
			}
		} else {
			v.panels[0].rows[6].Click() // Display row 6 is stored row 5 below the draft.
			send()
			if v.panels[0].selected != 5 {
				t.Fatal("click below the draft selected the wrong stored row")
			}
		}
		if v.nameInteraction() != nil {
			t.Fatal("row click did not cancel the draft")
		}
	}
}

func TestCreateFolderReplacesStaleCachedEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "old"), 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, root}, nil)
	defer v.loader.close()
	for pi := range v.panels {
		waitLoaded(t, v, pi)
		selectName(t, &v.panels[pi], "old")
		v.panels[pi].toggleMark(v.panels[pi].selected)
	}
	if err := os.Remove(filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(newFolderKey(), replaceCreate(v, "old"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	for pi := range v.panels {
		p := &v.panels[pi]
		if len(p.items) != 2 || p.items[p.selected].file.Name != "old" || len(p.marks) != 0 {
			t.Fatal("stale entry was duplicated or retained an obsolete mark/current item")
		}
	}
}

func TestRealCreateFolderPreservesMarksAndUpdatesPanels(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	for _, name := range []string{"a.txt", "z.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root, other}, nil)
	defer v.loader.close()
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	for pi := 0; pi < 2; pi++ {
		selectName(t, &v.panels[pi], "z.txt")
		v.panels[pi].toggleMark(v.panels[pi].selected)
	}
	v.panels[0].metadata = metadataModified
	otherItems := slices.Clone(v.panels[2].items)
	send, _ := panelInputFrames(v)
	send(newFolderKey())
	send(replaceCreate(v, "Å folder"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	p := &v.panels[0]
	if v.nameInteraction() != nil || v.active != 0 || p.items[p.selected].name != "Å folder" || p.metadata != metadataModified || p.marks[p.selected] {
		t.Fatal("created folder was not selected and unmarked, or form stayed open")
	}
	if p.selected != 1 || !p.items[p.selected].file.Directory || p.items[p.selected].file.Modified.IsZero() {
		t.Fatal("folder was not inserted in directory order with metadata")
	}
	for pi := 0; pi < 2; pi++ {
		marked := &v.panels[pi]
		if len(marked.marks) != 1 {
			t.Fatal("creation lost existing marks")
		}
		for row := range marked.marks {
			if marked.items[row].name != "z.txt" {
				t.Fatal("creation moved a mark to the wrong item")
			}
		}
	}
	if v.panels[1].items[v.panels[1].selected].name != "z.txt" || !slices.Equal(v.panels[2].items, otherItems) {
		t.Fatal("creation changed another panel's current item or unrelated listing")
	}
	if info, err := os.Stat(filepath.Join(root, "Å folder")); err != nil || !info.IsDir() {
		t.Fatalf("folder was not created: %v", err)
	}
	if message := p.feedback.Current(); message == nil || message.Kind != feedback.Success || message.Text != "Folder created" {
		t.Fatal("creation did not leave success feedback")
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if p.path != filepath.Join(root, "Å folder") {
		t.Fatal("browsing focus was not restored to the new folder")
	}
}

func TestCreateFolderErrorsRetainDraftAndAllowRetry(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root}, nil)
	defer v.loader.close()
	waitLoaded(t, v, 0)
	before := slices.Clone(v.panels[0].items)
	send, _ := panelInputFrames(v)
	send(newFolderKey())
	for _, tc := range []struct {
		name, message string
		err           error
	}{{"../outside", "Invalid folder name", browser.ErrInvalidName}, {"existing", "Name already exists", fs.ErrExist}} {
		send(replaceCreate(v, tc.name))
		send(pressed(key.NameReturn))
		waitCreated(t, v)
		send()
		if v.nameInteraction() == nil || !errors.Is(v.nameInteraction().err, tc.err) || v.nameEditor.ReadOnly || v.nameEditor.Text() != tc.name || !slices.Equal(v.panels[0].items, before) {
			t.Fatalf("failed creation did not retain editable draft: %q", tc.name)
		}
		if message := v.panels[0].feedback.Current(); message == nil || message.Text != tc.message {
			t.Fatalf("missing creation error: %q", tc.message)
		}
	}
	send(replaceCreate(v, "new"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if v.nameInteraction() != nil || v.panels[0].items[v.panels[0].selected].name != "new" {
		t.Fatal("retry failed")
	}
	if data, err := os.ReadFile(filepath.Join(root, "existing")); err != nil || string(data) != "keep" {
		t.Fatal("creation replaced the existing file")
	}
}

func TestCreateFolderEmptyDirectoryAndUnavailableContexts(t *testing.T) {
	for _, p := range []panel{{loading: true, path: "folder"}, {drives: true, path: "root"}, {}} {
		v := newView([]panel{p})
		send, _ := panelInputFrames(v)
		send(newFolderKey())
		if v.nameInteraction() != nil {
			t.Fatal("creation opened for a busy panel, drive list or unknown location")
		}
	}
	v := newView([]panel{{path: t.TempDir(), selected: -1}})
	send, _ := panelInputFrames(v)
	send(newFolderKey())
	if v.nameInteraction() == nil {
		t.Fatal("empty directory could not create a folder")
	}
	// Editor shortcuts keep their normal semantics and never open another editor.
	send(pressed(key.NameEscape))
	send(modified("D", key.ModAlt))
	send(newFolderKey())
	if v.nameInteraction() != nil || !v.locationEditing() {
		t.Fatal("creation interrupted a location editor")
	}
	v = newPreviewView()
	send, _ = panelInputFrames(v)
	send(pressed(key.NameF2))
	send(newFolderKey())
	if v.nameInteraction() == nil || v.nameInteraction().kind != nameRename {
		t.Fatal("creation interrupted a filename editor")
	}
}

func TestInlineFileCreationAndCollision(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, root}, nil)
	defer v.loader.close()
	for pi := range v.panels {
		waitLoaded(t, v, pi)
		selectName(t, &v.panels[pi], "existing.txt")
		v.panels[pi].toggleMark(v.panels[pi].selected)
	}
	send, _ := panelInputFrames(v)
	send(modified("N", key.ModCtrl))
	if v.nameInteraction() == nil || v.nameInteraction().kind != nameCreateFile {
		t.Fatal("Ctrl+N did not open a file draft")
	}
	send(replaceCreate(v, "existing.txt"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if v.nameInteraction() == nil || !errors.Is(v.nameInteraction().err, fs.ErrExist) || v.nameEditor.ReadOnly {
		t.Fatal("file collision did not retain an editable draft")
	}
	if data, err := os.ReadFile(filepath.Join(root, "existing.txt")); err != nil || string(data) != "keep" {
		t.Fatal("creation changed the existing file")
	}
	send(replaceCreate(v, "a-new.txt"))
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	p := &v.panels[0]
	item := p.items[p.selected]
	if v.nameInteraction() != nil || item.name != "a-new.txt" || item.file.Directory || !item.file.SizeKnown || item.size != "0 B" || p.selected != 1 {
		t.Fatal("new file was not selected with fresh metadata in sorted order")
	}
	if data, err := os.ReadFile(filepath.Join(root, "a-new.txt")); err != nil || len(data) != 0 {
		t.Fatal("new file was not empty")
	}
	for pi := range v.panels {
		for row := range v.panels[pi].marks {
			if v.panels[pi].items[row].name != "existing.txt" {
				t.Fatal("creation changed mark identity")
			}
		}
		if len(v.panels[pi].marks) != 1 {
			t.Fatal("creation lost an existing mark")
		}
	}
	if v.panels[1].items[v.panels[1].selected].name != "existing.txt" {
		t.Fatal("creation changed another panel's current file")
	}
	if message := p.feedback.Current(); message == nil || message.Text != "File created" {
		t.Fatal("file creation did not retain success feedback")
	}
}

func TestInlineDraftAtEndRestoresScrollAndSelection(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameEnd))
	p := &v.panels[0]
	selected, position := p.selected, p.list.Position
	send(modified("N", key.ModCtrl))
	if v.nameInteraction() == nil || v.nameInteraction().row != len(p.items) {
		t.Fatal("draft was not appended after the last current item")
	}
	if p.list.Position.First == position.First && p.list.Position.Offset == position.Offset {
		t.Fatal("offscreen draft was not revealed")
	}
	send(replaceCreate(v, "draft.txt"))
	send(pressed(key.NameEscape))
	if v.nameInteraction() != nil || p.selected != selected || p.list.Position != position {
		t.Fatalf("cancellation did not restore viewport: got=%+v want=%+v", p.list.Position, position)
	}
}

func TestScrollingAwayCancelsInlineDraft(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(newFolderKey())
	send(replaceCreate(v, "draft"))
	point := f32.Pt(200, 250)
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: point, Scroll: f32.Pt(0, 1000)})
	send()
	send()
	if v.nameInteraction() != nil {
		t.Fatal("scrolling left an invisible draft editor")
	}
}

func TestCreateFolderFollowsPanelEventOrder(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameTab), newFolderKey())
	if v.nameInteraction() == nil || v.nameInteraction().panel != 1 || v.nameInteraction().path != v.panels[1].path {
		t.Fatal("creation did not use the panel selected by the preceding key")
	}
}

func TestPendingCreateFolderCannotCancelOrDuplicate(t *testing.T) {
	root := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	v := newView([]panel{{path: root, selected: -1}, {path: root}})
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		close(entered)
		<-release
		return browser.ReadDirectory(path)
	}, nil)
	defer v.loader.close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	v.load(1, root, "")
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	send, _ := panelInputFrames(v)
	send(newFolderKey())
	send(replaceCreate(v, "queued"))
	send(pressed(key.NameReturn))
	send(pressed(key.NameEscape), pressed(key.NameReturn), pressed(key.NameTab), refreshKey())
	if v.nameInteraction() == nil || !v.nameInteraction().pending || !v.nameEditor.ReadOnly || len(v.loader.requests) != 1 || v.active != 0 {
		t.Fatal("pending operation was cancelled, duplicated or allowed browsing")
	}
	close(release)
	waitCreated(t, v)
	send()
	if v.nameInteraction() != nil || v.panels[0].items[v.panels[0].selected].name != "queued" {
		t.Fatal("queued creation did not complete")
	}
}
