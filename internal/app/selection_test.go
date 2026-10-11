package app

import (
	"errors"
	"maps"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/dannyben/commando/internal/browser"
)

func selectionView() *view {
	items := []entry{
		{name: "../", file: browser.Entry{Name: "..", Parent: true, Directory: true}},
		{name: "alpha", file: browser.Entry{Name: "alpha", Directory: true}},
		{name: "alpha beta.txt", file: browser.Entry{Name: "alpha beta.txt"}},
		{name: "charlie.txt", file: browser.Entry{Name: "charlie.txt"}},
		{name: "delta.txt", file: browser.Entry{Name: "delta.txt"}},
		{name: "echo.txt", file: browser.Entry{Name: "echo.txt"}},
	}
	return newView([]panel{{path: "/folder", items: items, selected: 1}, {path: "/other", items: items, selected: 1}})
}

func assertMarks(t *testing.T, p *panel, rows ...int) {
	t.Helper()
	want := make(map[int]bool)
	for _, row := range rows {
		want[row] = true
	}
	if !maps.Equal(p.marks, want) {
		t.Fatalf("marks=%v, want %v", p.marks, want)
	}
}

func modified(name key.Name, modifiers key.Modifiers) key.Event {
	return key.Event{Name: name, Modifiers: modifiers, State: key.Press}
}

func TestMarkKeysAndPanelIndependence(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	p := &v.panels[0]
	send(pressed(insertKey))
	assertMarks(t, p, 1)
	if p.selected != 2 {
		t.Fatal("Insert did not move down")
	}
	send(key.Event{Name: insertKey, State: key.Release})
	assertMarks(t, p, 1)
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, p, 1, 2)
	if p.selected != 2 || p.prefix != "" {
		t.Fatal("Space moved the current row or started filename typing")
	}
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, p, 1)
	send(pressed(key.NameEnd), pressed(insertKey))
	assertMarks(t, p, 1, 5)
	if p.selected != 5 {
		t.Fatal("Insert crossed the last row")
	}
	send(pressed(key.NameRightArrow), pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, &v.panels[1], 1)
	send(pressed(key.NameLeftArrow))
	assertMarks(t, p, 1, 5)
	if p.selected != 5 {
		t.Fatal("panel switch lost current row")
	}
	send(modified("A", key.ModCtrl))
	assertMarks(t, p, 1, 2, 3, 4, 5)
	if status, _ := p.status(); status != "5 marked" {
		t.Fatalf("status=%q", status)
	}
	send(pressed(key.NameEscape))
	assertMarks(t, p)
	assertMarks(t, &v.panels[1], 1)
	send(pressed(key.NameHome), pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, p)
	send(pressed(insertKey))
	assertMarks(t, p)
	if p.selected != 1 {
		t.Fatal("Insert on parent did not advance")
	}
}

func TestRangeSelectionShrinksWithoutLosingEarlierMarks(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	p := &v.panels[0]
	send(pressed(key.NameEnd), pressed(key.NameSpace), key.EditEvent{Text: " "}, pressed(key.NameHome), pressed(key.NameDownArrow))
	send(modified(key.NameDownArrow, key.ModShift), modified(key.NameDownArrow, key.ModShift))
	assertMarks(t, p, 1, 2, 3, 5)
	send(modified(key.NameUpArrow, key.ModShift))
	assertMarks(t, p, 1, 2, 5)
	send(modified(key.NameUpArrow, key.ModShift), modified(key.NameUpArrow, key.ModShift))
	assertMarks(t, p, 1, 5)
	if p.selected != 0 {
		t.Fatal("Shift+Up did not reach the parent row")
	}
	send(pressed(key.NameDownArrow))
	assertMarks(t, p, 1, 5)
}

func TestSpaceInFilenamePrefixAndEscapePriority(t *testing.T) {
	v := selectionView()
	send, advance := panelInputFrames(v)
	p := &v.panels[0]
	send(pressed(key.NameSpace), key.EditEvent{Text: " "}, key.EditEvent{Text: "alpha"})
	send(pressed(key.NameSpace), key.EditEvent{Text: " "}, key.EditEvent{Text: "b"})
	if p.prefix != "alpha b" || p.selected != 2 {
		t.Fatalf("filename with a space: prefix=%q row=%d", p.prefix, p.selected)
	}
	assertMarks(t, p, 1)
	send(pressed(key.NameEscape))
	assertMarks(t, p, 1)
	if p.prefix != "" {
		t.Fatal("Escape did not clear prefix")
	}
	send(pressed(key.NameEscape))
	assertMarks(t, p)
	send(key.EditEvent{Text: "alpha"})
	advance(prefixDelay + time.Millisecond)
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, p, 1)
}

func TestMarkMouseModifiersAndKeyboardFocus(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	// Header + headings occupy 68px; the default rows are 28px high.
	click := func(row int, modifiers key.Modifiers) {
		t.Helper()
		position := f32.Pt(60, float32(68+row*28+14))
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: position, Modifiers: modifiers})
		send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: position, Modifiers: modifiers})
	}
	p := &v.panels[0]
	click(2, key.ModCtrl)
	assertMarks(t, p, 2)
	click(4, key.ModShift)
	assertMarks(t, p, 2, 3, 4)
	click(3, key.ModShift)
	assertMarks(t, p, 2, 3)
	click(1, 0)
	assertMarks(t, p, 2, 3)
	send(pressed(insertKey))
	assertMarks(t, p, 1, 2, 3)
	if p.selected != 2 {
		t.Fatal("row click did not restore keyboard focus")
	}
}

func TestMarksFollowSuccessfulListingChanges(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	p := &v.panels[0]
	p.loading = true
	send(modified("A", key.ModCtrl), modified(key.NameDownArrow, key.ModShift), pressed(insertKey))
	assertMarks(t, p, 1)
	v.acceptLoad(loadResult{request: loadRequest{panel: 0, path: "/missing"}, err: errors.New("read failed")})
	assertMarks(t, p, 1)
	v.acceptLoad(loadResult{request: loadRequest{panel: 0, path: "/next"}, directory: browser.Directory{Path: "/next", Entries: []browser.Entry{{Name: "new.txt"}}}})
	assertMarks(t, p)
	if p.markRange != nil {
		t.Fatal("new listing retained a range anchor")
	}
	p.drives = true
	send(modified("A", key.ModCtrl), pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, p)
}

func TestSelectionKeysStayInLocationEditor(t *testing.T) {
	v := selectionView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	send, advance := panelInputFrames(v)
	send(pressed(key.NameSpace), key.EditEvent{Text: " "}, modified("D", key.ModAlt))
	p := &v.panels[0]
	v.nativeKeys.push(pressed(insertKey))
	advance(0)
	send(modified("A", key.ModCtrl))
	if v.location.SelectedText() != v.location.Text() {
		t.Fatal("Ctrl+A did not select editor text")
	}
	send(pressed(key.NameSpace), key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(v.location.Text())}, Text: " "})
	assertMarks(t, p, 1)
	if !v.editing || p.selected != 1 || v.location.Text() != " " {
		t.Fatalf("marking shortcuts leaked out of the location editor: editing=%v current=%d text=%q", v.editing, p.selected, v.location.Text())
	}
	send(pressed(key.NameEscape))
	assertMarks(t, p, 1)
	v.nativeKeys.push(modified(insertKey, key.ModCtrl))
	v.nativeKeys.push(key.Event{Name: insertKey, State: key.Release})
	advance(0)
	assertMarks(t, p, 1)
	v.nativeKeys.push(pressed(insertKey))
	advance(0)
	assertMarks(t, p)
	if p.selected != 2 {
		t.Fatal("native Insert did not mark and advance after leaving editor")
	}
}

func TestEnterOpensOnlyCurrentFile(t *testing.T) {
	v := selectionView()
	opened := make(chan string, 2)
	v.openFile = func(path string) error { opened <- path; return nil }
	send, _ := panelInputFrames(v)
	send(modified("A", key.ModCtrl), pressed(key.NameDownArrow), pressed(key.NameReturn))
	select {
	case path := <-opened:
		if path != filepath.Join("/folder", "alpha beta.txt") {
			t.Fatalf("opened %q", path)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter did not open current file")
	}
	assertMarks(t, &v.panels[0], 1, 2, 3, 4, 5)
	if len(opened) != 0 {
		t.Fatal("Enter opened marked files")
	}
}
