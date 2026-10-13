package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func adjacentKey(direction int) key.Event {
	name := key.NameRightArrow
	if direction < 0 {
		name = key.NameLeftArrow
	}
	return key.Event{Name: name, Modifiers: key.ModCtrl, State: key.Press}
}

func TestAdjacentFolderOpening(t *testing.T) {
	for _, direction := range []int{-1, 1} {
		t.Run(fmt.Sprint(direction), func(t *testing.T) {
			root := t.TempDir()
			for i := 0; i < 30; i++ {
				if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("folder-%02d", i)), 0700); err != nil {
					t.Fatal(err)
				}
			}
			child := filepath.Join(root, "folder-29")
			for _, name := range []string{"a.txt", "b.txt"} {
				if err := os.WriteFile(filepath.Join(child, name), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			v := newBrowserView([]string{root, root, root}, nil)
			defer v.loader.close()
			for i := range v.panels {
				waitLoaded(t, v, i)
			}
			v.active = 1
			send, _ := panelInputFrames(v)
			send(pressed(key.NameEnd))
			source := &v.panels[1]
			selected, position, items := source.selected, source.list.Position, slices.Clone(source.items)
			if position.First == 0 {
				t.Fatal("test source must be scrolled")
			}
			target := 1 + direction
			other := 1 - direction
			otherPosition, otherSelected := v.panels[other].list.Position, v.panels[other].selected
			send(adjacentKey(direction))
			waitLoaded(t, v, target)
			send()
			if v.active != target || v.panels[target].path != child || v.panels[target].err != nil {
				t.Fatalf("folder did not open with focus in adjacent panel: active=%d panel=%+v", v.active, v.panels[target])
			}
			if source.path != root || source.selected != selected || source.list.Position != position || !slices.Equal(source.items, items) {
				t.Fatal("opening adjacent changed source path, selection, scroll or listing")
			}
			if v.panels[other].path != root || v.panels[other].selected != otherSelected || v.panels[other].list.Position != otherPosition {
				t.Fatal("opening adjacent changed the uninvolved panel")
			}
			send(pressed(key.NameDownArrow))
			if v.panels[target].selected != 1 || source.selected != selected {
				t.Fatal("keyboard navigation did not follow focus to the destination")
			}
			send(pressed(adjacentKey(-direction).Name))
			if v.active != 1 || source.selected != selected || source.list.Position != position {
				t.Fatal("returning to source lost selection or scroll")
			}
		})
	}
}

func TestAdjacentOpeningNoAction(t *testing.T) {
	for _, tc := range []struct {
		name      string
		active    int
		direction int
		prepare   func(*view)
	}{
		{"left edge with work pending", 0, -1, func(v *view) { v.panels[2].refreshing = true }},
		{"right edge with work pending", 2, 1, func(v *view) { v.panels[0].loading = true }},
		{"file", 1, 1, func(v *view) { v.panels[1].items[0].file.Directory = false }},
		{"empty", 1, -1, func(v *view) { v.panels[1].items = nil; v.panels[1].selected = -1 }},
		{"invalid selection", 1, 1, func(v *view) { v.panels[1].selected = 4 }},
		{"source busy", 1, 1, func(v *view) { v.panels[1].loading = true }},
		{"destination busy", 1, -1, func(v *view) { v.panels[0].loading = true }},
		{"no loader", 1, 1, func(v *view) { v.loader = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			v := newView([]panel{{path: base}, {path: base}, {path: base}})
			for i := range v.panels {
				v.panels[i].items = []entry{{name: "folder", file: browser.Entry{Name: "folder", Directory: true}}}
			}
			v.active = tc.active
			// A loader without a worker makes unexpected queued reads observable.
			loader := &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
			v.loader = loader
			if tc.prepare != nil {
				tc.prepare(v)
			}
			send, _ := panelInputFrames(v)
			before := v.sessionState(windowState{})
			send(adjacentKey(tc.direction))
			if len(loader.requests) != 0 || !reflect.DeepEqual(v.sessionState(windowState{}), before) {
				t.Fatal("shortcut loaded a directory or moved focus when it should do nothing")
			}
		})
	}
}

func TestAdjacentParentDriveAndReadError(t *testing.T) {
	base := filepath.Join(t.TempDir(), "work")
	root := filepath.VolumeName(base) + string(filepath.Separator)
	for _, tc := range []struct {
		name string
		file browser.Entry
		want string
		err  error
	}{
		{"parent", browser.Entry{Name: "..", Directory: true, Parent: true}, filepath.Dir(base), nil},
		{"drive", browser.Entry{Name: root, Path: root, Directory: true}, root, nil},
		{"unreadable", browser.Entry{Name: "missing", Directory: true}, filepath.Join(base, "missing"), os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newView([]panel{{path: base, items: []entry{{file: tc.file}}},
				{path: base, items: []entry{{name: "retained.txt", file: browser.Entry{Name: "retained.txt"}}}}})
			v.panels[0].drives = tc.name == "drive"
			v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
				if path != tc.want {
					return browser.Directory{}, fmt.Errorf("opened %q, want %q", path, tc.want)
				}
				return browser.Directory{Path: path}, tc.err
			}, nil)
			defer v.loader.close()
			send, _ := panelInputFrames(v)
			before := slices.Clone(v.panels[1].items)
			position := v.panels[1].list.Position
			send(adjacentKey(1))
			waitLoaded(t, v, 1)
			send()
			p := &v.panels[1]
			if v.active != 1 || v.panels[0].path != base {
				t.Fatal("focus did not follow destination or source path changed")
			}
			if tc.err != nil {
				if !errors.Is(p.err, tc.err) || p.path != base || p.selected != 0 || p.list.Position != position || !slices.Equal(p.items, before) {
					t.Fatal("failed read did not retain destination state with its inline error")
				}
			} else if p.err != nil || p.path != tc.want || p.drives {
				t.Fatalf("adjacent navigation failed: path=%q err=%v", p.path, p.err)
			}
		})
	}
}

func TestCtrlArrowsStayInLocationEditor(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	send, _ := panelInputFrames(v)
	send(key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press})
	for _, direction := range []int{-1, 1} {
		send(adjacentKey(direction))
		if !v.locationEditing() || v.active != 0 || len(v.loader.requests) != 0 {
			t.Fatal("Ctrl+arrow in the editor opened a folder or switched panels")
		}
	}
}
