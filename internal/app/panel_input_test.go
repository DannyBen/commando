package app

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/control/internal/browser"
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

func TestTypeToSelect(t *testing.T) {
	items := []entry{
		{name: "../", file: browser.Entry{Name: "..", Directory: true, Parent: true}},
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
	if v.panels[1].prefix != "å" {
		t.Fatal("Backspace did not remove one Unicode character")
	}
	send(pressed(key.NameDeleteBackward))
	if v.panels[1].prefix != "" || v.panels[1].selected != 5 {
		t.Fatal("prefix deletion changed folders or selection")
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
	if !v.editing || v.location.SelectedText() != root {
		t.Fatal("Alt+D did not focus/select the panel path")
	}
	replace("child")
	send(pressed(key.NameLeftArrow))
	if v.active != 0 || !v.editing {
		t.Fatal("editor arrow switched panels")
	}
	send(pressed(key.NameEscape))
	if v.editing || v.panels[0].path != root || v.panels[0].loading {
		t.Fatal("Escape navigated or failed to cancel path editing")
	}
	send(pressed(key.NameDownArrow))
	if v.panels[0].selected != 1 {
		t.Fatalf("Escape did not restore list keyboard focus: selected=%d editing=%v", v.panels[0].selected, v.editing)
	}
	send(altD)
	replace("child")
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if v.editing || v.panels[0].path != filepath.Join(root, "child") || v.panels[1].path != root {
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
	before := v.panels[0].path
	send(altD)
	replace(filepath.Join(root, "missing"))
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if v.panels[0].path != before || v.panels[0].err == nil {
		t.Fatal("invalid path did not retain the listing and existing directory error behavior")
	}
	send(altD)
	replace("")
	send(pressed(key.NameReturn))
	if !v.editing || v.panels[0].loading {
		t.Fatal("empty path navigated")
	}
	send(pressed(key.NameEscape))
	send(altD)
	v.panels[1].rows[1].Click()
	send()
	send(pressed(key.NameDownArrow))
	if v.editing || v.active != 1 || v.panels[1].selected != 2 {
		t.Fatal("row click did not cancel editing and restore keyboard focus")
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
