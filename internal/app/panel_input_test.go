package app

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
)

func panelInputFrames(v *view) (send func(...event.Event), advance func(time.Duration)) {
	var router input.Router
	var ops op.Ops
	now := time.Now()
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480)),
		})
		router.Frame(&ops)
	}
	frame()
	frame()
	send = func(events ...event.Event) { router.Queue(events...); frame(); frame() }
	advance = func(d time.Duration) { now = now.Add(d); frame(); frame() }
	return
}

func pressed(name key.Name) key.Event { return key.Event{Name: name, State: key.Press} }

func TestTabSwitchesPanels(t *testing.T) {
	v := newPreviewView()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480)),
		})
		router.Frame(&ops)
	}
	send := func(e event.Event) { router.Queue(e); frame(); frame() }
	frame()
	frame()
	// Give all three panels distinct selections and scroll positions.
	for i := range v.panels {
		send(pressed(key.NameEnd))
		for j := 0; j < i; j++ {
			send(pressed(key.NameUpArrow))
		}
		send(pressed(key.NameRightArrow))
	}
	send(pressed(key.NameLeftArrow))
	send(pressed(key.NameLeftArrow))
	selections := make([]int, len(v.panels))
	positions := make([]layout.Position, len(v.panels))
	for i := range v.panels {
		selections[i], positions[i] = v.panels[i].selected, v.panels[i].list.Position
	}
	for _, tc := range []struct {
		modifiers key.Modifiers
		want      int
	}{{0, 1}, {0, 2}, {0, 0}, {0, 1}, {key.ModShift, 0}, {key.ModShift, 2}, {key.ModShift, 1}, {key.ModShift, 0}} {
		// Mirror Gio's Window: unhandled system Tab invokes widget focus traversal.
		router.WakeupTime()
		router.Queue(input.SystemEvent{Event: key.Event{
			Name: key.NameTab, Modifiers: tc.modifiers, State: key.Press,
		}})
		if _, handled := router.WakeupTime(); !handled {
			t.Fatal("Tab was not consumed")
		}
		frame()
		frame()
		send(key.Event{Name: key.NameTab, Modifiers: tc.modifiers, State: key.Release})
		if v.active != tc.want || !router.Source().Focused(&v.focus) {
			t.Fatalf("Tab changed panel/focus incorrectly: active=%d want=%d", v.active, tc.want)
		}
		for i := range v.panels {
			if v.panels[i].selected != selections[i] || v.panels[i].list.Position != positions[i] {
				t.Fatalf("Tab changed panel %d selection or scroll", i)
			}
		}
	}
	send(pressed(key.NameUpArrow))
	if v.panels[0].selected != selections[0]-1 || v.panels[1].selected != selections[1] {
		t.Fatal("list lost keyboard input after Tab")
	}
	send(key.EditEvent{Text: "c"})
	if v.panels[0].prefix != "c" {
		t.Fatal("test prefix was not entered")
	}
	send(pressed(key.NameTab))
	if v.active != 1 || v.panels[0].prefix != "" {
		t.Fatal("Tab did not clear source prefix like Right")
	}
}

func TestTabKeepsLocationEditorFocus(t *testing.T) {
	v := newPreviewView()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480)),
		})
		router.Frame(&ops)
	}
	send := func(e event.Event) { router.Queue(e); frame(); frame() }
	frame()
	frame()
	send(key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press})
	p := &v.panels[v.active]
	selected, position := p.selected, p.list.Position
	text := v.location.Text()
	start, end := v.location.Selection()
	for _, modifiers := range []key.Modifiers{0, key.ModShift} {
		router.WakeupTime()
		router.Queue(input.SystemEvent{Event: key.Event{
			Name: key.NameTab, Modifiers: modifiers, State: key.Press,
		}})
		if _, handled := router.WakeupTime(); !handled {
			t.Fatal("editor Tab was not consumed")
		}
		frame()
		frame()
		send(key.Event{Name: key.NameTab, Modifiers: modifiers, State: key.Release})
		gotStart, gotEnd := v.location.Selection()
		if !v.locationEditing() || v.active != 0 || !router.Source().Focused(&v.location) ||
			v.location.Text() != text || gotStart != start || gotEnd != end ||
			p.selected != selected || p.list.Position != position {
			t.Fatal("Tab changed editor or panel state/focus")
		}
	}
	send(pressed(key.NameDeleteBackward))
	if v.location.Text() != "" || !v.locationEditing() {
		t.Fatal("path editor lost keyboard input after Tab")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameTab))
	if v.active != 1 || !router.Source().Focused(&v.focus) {
		t.Fatal("Tab did not switch panels after leaving the editor")
	}
}

func TestTypeToSelect(t *testing.T) {
	items := []entry{
		{name: "..", file: browser.Entry{Name: "..", Directory: true, Parent: true}},
		{name: "Alpha.txt"}, {name: "alpine.txt"}, {name: "archive.txt"}, {name: "Beta.txt"}, {name: "Ångström.txt"},
	}
	v := newView([]panel{{items: items, selected: 0}, {items: items, selected: 3}})
	send, advance := panelInputFrames(v)
	p := &v.panels[0]
	send(key.EditEvent{Text: "AL"})
	if p.selected != 1 || p.prefix != "AL" {
		t.Fatalf("prefix did not select Alpha: %+v", p)
	}
	send(key.EditEvent{Text: "pine"})
	if p.selected != 2 {
		t.Fatal("continued prefix did not select alpine")
	}
	send(key.EditEvent{Text: "x"})
	if p.selected != 2 || len(p.items) != len(items) {
		t.Fatal("unmatched prefix changed selection or filtered entries")
	}
	send(pressed(key.NameEscape))
	if p.prefix != "" || p.selected != 2 {
		t.Fatal("Escape did not clear only the prefix")
	}
	send(key.EditEvent{Text: "B"})
	if p.selected != 4 {
		t.Fatal("fresh prefix did not select Beta")
	}
	advance(2 * time.Second)
	if p.prefix != "" {
		t.Fatal("prefix did not expire")
	}
	send(key.EditEvent{Text: "a"})
	if p.selected != 1 {
		t.Fatal("fresh prefix did not wrap to Alpha")
	}
	send(pressed(key.NameRightArrow))
	if v.active != 1 || p.prefix != "" || v.panels[1].selected != 3 {
		t.Fatal("switching panels leaked prefix or changed inactive selection")
	}
	send(key.EditEvent{Text: "ån"})
	if v.panels[1].selected != 5 {
		t.Fatal("Unicode prefix did not match case-insensitively")
	}
	send(pressed(key.NameDeleteBackward))
	if v.panels[1].prefix != "" {
		t.Fatal("Backspace did not clear the filename prefix")
	}
	send(pressed(key.NameHome))
	send(key.EditEvent{Text: ".."})
	if v.panels[1].selected != 0 {
		t.Fatal("parent row became a type-to-select match")
	}
}

func TestTypeToSelectRevealsOffscreenEntry(t *testing.T) {
	items := make([]entry, 1000)
	for i := range items {
		items[i] = entry{name: fmt.Sprintf("file-%04d.txt", i)}
	}
	v := newView([]panel{{items: items, selected: 0}})
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "file-0999"})
	p := &v.panels[0]
	if p.selected != 999 || p.list.Position.First == 0 {
		t.Fatal("type-to-select did not reveal its offscreen match")
	}
	send(pressed(key.NamePageUp))
	if p.prefix != "" || p.selected >= 999 {
		t.Fatal("Page Up did not clear the prefix and navigate")
	}
	send(pressed(key.NameHome))
	send(pressed(key.NamePageDown))
	if p.selected <= 0 {
		t.Fatal("Page Down did not navigate")
	}
}

func TestLocationEditing(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"child", "other"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root}, nil)
	v.openFile = nil
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	send, _ := panelInputFrames(v)
	altD := key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press}
	replace := func(text string) {
		send(key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(v.location.Text())}, Text: text})
	}
	send(altD)
	if !v.locationEditing() || v.location.SelectedText() != root {
		t.Fatal("Alt+D did not focus/select the panel path")
	}
	replace("child")
	send(pressed(key.NameLeftArrow))
	if v.active != 0 || !v.locationEditing() {
		t.Fatal("editor arrow switched panels")
	}
	send(pressed(key.NameEscape))
	if v.locationEditing() || v.panels[0].path != root || v.panels[0].loading {
		t.Fatal("Escape navigated or failed to cancel path editing")
	}
	send(pressed(key.NameDownArrow))
	if v.panels[0].selected != 1 {
		t.Fatalf("Escape did not restore list keyboard focus: selected=%d editing=%v", v.panels[0].selected, v.locationEditing())
	}
	send(altD)
	replace("child")
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if v.locationEditing() || v.panels[0].path != filepath.Join(root, "child") || v.panels[1].path != root {
		t.Fatal("relative path submission did not navigate only the current panel")
	}
	send(altD)
	replace(filepath.Join(root, "other"))
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if v.panels[0].path != filepath.Join(root, "other") {
		t.Fatal("absolute path submission failed")
	}
	send(altD)
	replace(filepath.Join(root, "missing"))
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if v.panels[0].path != root || v.panels[0].err != nil || v.panels[1].path != root {
		t.Fatal("missing path did not recover to its parent in only the current panel")
	}
	send(altD)
	replace("")
	send(pressed(key.NameReturn))
	if !v.locationEditing() || v.panels[0].loading {
		t.Fatal("empty path navigated")
	}
	send(pressed(key.NameEscape))
	send(altD)
	v.panels[1].rows[1].Click()
	send()
	send(pressed(key.NameDownArrow))
	if v.locationEditing() || v.active != 1 || v.panels[1].selected != 2 {
		t.Fatal("row click did not cancel editing and restore keyboard focus")
	}
}

func TestNormalizeDriveShortcut(t *testing.T) {
	for letter := byte('A'); letter <= 'Z'; letter++ {
		want := string(letter) + `:\`
		for _, input := range []string{string(letter), string(letter + 'a' - 'A'), string(letter) + ":", string(letter+'a'-'A') + ":"} {
			if got := normalizeDriveShortcut(input); got != want {
				t.Errorf("%q resolved to %q, want %q", input, got, want)
			}
		}
	}
	for _, path := range []string{"", ".", "..", "1", "1:", ":", "é", " d", "d ", "d::", "docs", `D:\`, `D:/`, `D:\Projects`, `D:Projects`, `\Projects`, `.\d`, "./d", `\\server\share`} {
		if got := normalizeDriveShortcut(path); got != path {
			t.Errorf("ordinary path %q changed to %q", path, got)
		}
	}
}

func TestLocationDriveShortcutSubmission(t *testing.T) {
	base := filepath.Join(t.TempDir(), "work")
	for _, tc := range []struct {
		input, windows, other string
		failed                bool
	}{
		{"d", `D:\`, filepath.Join(base, "d"), false},
		{"D", `D:\`, filepath.Join(base, "D"), false},
		{"d:", `D:\`, filepath.Join(base, "d:"), false},
		{"D:", `D:\`, filepath.Join(base, "D:"), false},
		{"./d", filepath.Join(base, "d"), filepath.Join(base, "d"), false},
		{`.\d`, filepath.Join(base, "d"), filepath.Join(base, `.\d`), false},
		{filepath.Join(base, "docs"), filepath.Join(base, "docs"), filepath.Join(base, "docs"), false},
		{string(filepath.Separator) + "Projects", filepath.VolumeName(base) + `\Projects`, "/Projects", false},
		{"z:", `Z:\`, filepath.Join(base, "z:"), true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			want := tc.other
			if runtime.GOOS == "windows" {
				want = tc.windows
			}
			v := newView([]panel{{path: base}, {path: base}})
			v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
				if path != want {
					return browser.Directory{}, fmt.Errorf("submitted %q, want %q", path, want)
				}
				if tc.failed {
					return browser.Directory{}, os.ErrPermission
				}
				return browser.Directory{Path: path}, nil
			}, nil)
			defer v.loader.close()
			send, _ := panelInputFrames(v)
			send(key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press})
			send(key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(v.location.Text())}, Text: tc.input})
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			p := &v.panels[0]
			if tc.failed {
				if p.path != base || !errors.Is(p.err, os.ErrPermission) {
					t.Fatalf("failed navigation changed the folder or error: path=%q err=%v", p.path, p.err)
				}
			} else if p.path != want || p.err != nil {
				t.Fatalf("submission failed: path=%q want=%q err=%v", p.path, want, p.err)
			}
			if v.locationEditing() || v.panels[1].path != base {
				t.Fatal("submission left editor open or changed another panel")
			}
		})
	}
}

func TestFileOpeningIsNonblockingAndSilent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hello world.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, root}, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	opened := make(chan string, 1)
	release, finished := make(chan struct{}), make(chan struct{})
	v.openFile = func(path string) error {
		defer close(finished)
		opened <- path
		<-release
		return errors.New("no associated application")
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDownArrow))
	send(pressed(key.NameReturn))
	select {
	case path := <-opened:
		if path != filepath.Join(root, "hello world.txt") {
			t.Fatalf("wrong file path %q", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Enter did not open the selected file")
	}
	send(pressed(key.NameRightArrow))
	if v.active != 1 || v.panels[0].loading {
		t.Fatal("file opening blocked navigation")
	}
	close(release)
	released = true
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("opener did not finish")
	}
	send()
	if v.panels[0].err != nil || v.panels[0].path != root || v.panels[0].selected != 1 {
		t.Fatal("file-opening failure changed panel state or showed an error")
	}
}

func TestBackspaceGoesUpWithActivePrefix(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "note.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{child}, nil)
	defer v.loader.close()
	waitLoaded(t, v, 0)
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "no"})
	if v.panels[0].prefix != "no" {
		t.Fatal("prefix not active")
	}
	send(pressed(key.NameDeleteBackward))
	waitLoaded(t, v, 0)
	send()
	p := &v.panels[0]
	if p.path != root || p.prefix != "" || p.items[p.selected].file.Name != "child" {
		t.Fatal("Backspace did not immediately go up and reselect the folder")
	}
	// In the path editor, Backspace still deletes text instead of navigating.
	send(key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press})
	send(pressed(key.NameDeleteBackward))
	if !v.locationEditing() || v.location.Text() != "" || p.path != root || p.loading {
		t.Fatal("Backspace in the path editor navigated instead of editing text")
	}
}
