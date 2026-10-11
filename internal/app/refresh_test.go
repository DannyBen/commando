package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func refreshKey() key.Event {
	return key.Event{Name: "R", Modifiers: key.ModCtrl, State: key.Press}
}

func TestRefreshFolderRetainsCurrentItemAndViewport(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 50; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%02d.txt", i)), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root, root}, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameEnd))
	send(pressed(key.NameUpArrow))
	p := &v.panels[0]
	p.metadata = metadataModified
	p.toggleMark(p.selected)
	position := p.list.Position
	otherItems := slices.Clone(v.panels[1].items)
	otherPosition := v.panels[1].list.Position
	selected := p.selected
	path := filepath.Join(root, p.items[p.selected].file.Name)
	if err := os.WriteFile(path, []byte("updated contents"), 0600); err != nil {
		t.Fatal(err)
	}
	send(refreshKey())
	waitLoaded(t, v, 0)
	send()
	if p.selected != selected || p.items[p.selected].file.Bytes != 16 || p.list.Position != position {
		t.Fatalf("refresh lost the current item/viewport or stale metadata: selected=%d position=%+v", p.selected, p.list.Position)
	}
	if p.path != root || p.metadata != metadataModified || len(p.marks) != 0 || p.markRange != nil || p.feedback.Current() != nil {
		t.Fatal("refresh changed location/column or retained marks/progress")
	}
	if v.active != 0 || !slices.Equal(v.panels[1].items, otherItems) || v.panels[1].list.Position != otherPosition {
		t.Fatal("refresh affected another panel")
	}
	// Inserting a folder before the selected file changes its row index.
	if err := os.Mkdir(filepath.Join(root, "new-folder"), 0700); err != nil {
		t.Fatal(err)
	}
	send(refreshKey())
	waitLoaded(t, v, 0)
	send()
	if p.selected != selected+1 || p.items[p.selected].file.Name != "file-48.txt" {
		t.Fatal("refresh followed a row index instead of the selected file")
	}
	send(pressed(key.NameDownArrow))
	if p.items[p.selected].file.Name != "file-49.txt" {
		t.Fatal("refresh lost browsing focus")
	}
}

func TestRefreshMissingSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  int
		want string
	}{{"middle", 2, "c.txt"}, {"last", 3, "b.txt"}, {"parent", 0, ".."}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
				if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			v := newBrowserView([]string{root}, nil)
			defer v.loader.close()
			waitLoaded(t, v, 0)
			p := &v.panels[0]
			p.selected = tc.row
			send, _ := panelInputFrames(v)
			if tc.row != 0 {
				if err := os.Remove(filepath.Join(root, p.items[p.selected].file.Name)); err != nil {
					t.Fatal(err)
				}
			}
			send(refreshKey())
			waitLoaded(t, v, 0)
			send()
			if p.items[p.selected].file.Name != tc.want {
				t.Fatalf("selected %q, want %q", p.items[p.selected].file.Name, tc.want)
			}
		})
	}
}

func TestRefreshDriveListAndEmptyListing(t *testing.T) {
	for _, drives := range []bool{false, true} {
		t.Run(fmt.Sprint(drives), func(t *testing.T) {
			root := t.TempDir()
			files := []browser.Entry{{Name: "old label", Path: root, Directory: true}}
			v := newView([]panel{{path: root, drives: drives, items: []entry{{name: files[0].Name, file: files[0]}}}})
			v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
				want := root
				if drives {
					want = ""
				}
				if path != want {
					t.Errorf("refresh read %q, want %q", path, want)
				}
				return browser.Directory{Path: root, Drives: drives, Entries: files}, nil
			}, nil)
			defer v.loader.close()
			send, _ := panelInputFrames(v)
			if drives {
				files = []browser.Entry{
					{Name: "another", Path: filepath.Join(root, "other"), Directory: true},
					{Name: "new label", Path: root, Directory: true},
				}
				send(refreshKey())
				waitLoaded(t, v, 0)
				send()
				if p := &v.panels[0]; p.selected != 1 || p.items[p.selected].file.Path != root {
					t.Fatal("drive refresh did not follow drive identity through a label change")
				}
			}
			files = nil
			send(refreshKey())
			waitLoaded(t, v, 0)
			send()
			p := &v.panels[0]
			if len(p.items) != 0 || p.selected != -1 || p.path != root || p.drives != drives || p.positionLabel() != "0 / 0" {
				t.Fatal("refresh did not handle an empty location")
			}
			files = []browser.Entry{{Name: "returned.txt"}}
			send(refreshKey())
			waitLoaded(t, v, 0)
			send()
			if p.selected != 0 || len(p.items) != 1 {
				t.Fatal("refresh could not recover from an empty location")
			}
		})
	}
}

func TestRefreshFailureRetainsListingAndMarks(t *testing.T) {
	v := newPreviewView()
	fail := true
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		if fail {
			return browser.Directory{}, os.ErrPermission
		}
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "recovered.txt"}}}, nil
	}, nil)
	defer v.loader.close()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameEnd))
	p := &v.panels[0]
	before := slices.Clone(p.items)
	marks := make(map[int]bool)
	for row, marked := range p.marks {
		marks[row] = marked
	}
	selected, position, path := p.selected, p.list.Position, p.path
	send(refreshKey())
	waitLoaded(t, v, 0)
	send()
	if !slices.Equal(p.items, before) || p.selected != selected || p.list.Position != position || p.path != path || !reflect.DeepEqual(p.marks, marks) {
		t.Fatal("failed refresh changed listing, selection, viewport or marks")
	}
	if message := p.feedback.Current(); message == nil || message.Text != "Access denied" {
		t.Fatal("failed refresh did not retain footer error")
	}
	fail = false
	send(refreshKey())
	waitLoaded(t, v, 0)
	send()
	if p.err != nil || p.feedback.Current() != nil || p.items[p.selected].file.Name != "recovered.txt" || len(p.marks) != 0 {
		t.Fatal("refresh did not recover after a failed read")
	}
}

func TestRefreshWhileLoadingRetainsLatestSelectionAndFocus(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	v := newView([]panel{
		{path: "folder", items: []entry{{name: "a", file: browser.Entry{Name: "a"}}, {name: "b", file: browser.Entry{Name: "b"}}}},
		{items: []entry{{name: "other"}}},
	})
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		close(entered)
		<-release
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "a"}, {Name: "b"}}}, nil
	}, nil)
	defer v.loader.close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	send, _ := panelInputFrames(v)
	send(refreshKey())
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh never started")
	}
	send(refreshKey(), pressed(key.NameDownArrow), pressed(key.NameTab))
	if !v.panels[0].loading || len(v.loader.requests) != 0 || v.active != 1 {
		t.Fatal("refresh blocked navigation or queued a duplicate")
	}
	close(release)
	waitLoaded(t, v, 0)
	send()
	if v.active != 1 || v.panels[0].selected != 1 {
		t.Fatal("refresh completion stole focus or restored an outdated selection")
	}
}

func TestRefreshRoutingAndEditors(t *testing.T) {
	v := newPreviewView()
	// An unconsumed request queue makes accidental reads directly observable.
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	send, _ := panelInputFrames(v)
	send(pressed("R"), key.Event{Name: "R", Modifiers: key.ModCtrl, State: key.Release})
	if len(v.loader.requests) != 0 {
		t.Fatal("plain R or key release refreshed")
	}
	for _, editor := range []string{"location", "rename"} {
		if editor == "location" {
			send(key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press})
		} else {
			send(pressed(key.NameF2))
		}
		active := v.activeEditor()
		if active == nil {
			t.Fatalf("%s editor did not open", editor)
		}
		text := active.Text()
		start, end := active.Selection()
		send(refreshKey())
		gotStart, gotEnd := active.Selection()
		if len(v.loader.requests) != 0 || v.activeEditor() != active || active.Text() != text || gotStart != start || gotEnd != end {
			t.Fatalf("refresh changed %s editor", editor)
		}
		send(pressed(key.NameEscape))
	}
	send(pressed(key.NameTab), refreshKey())
	select {
	case request := <-v.loader.requests:
		if request.panel != 1 || request.path != v.panels[1].path {
			t.Fatal("refresh and panel navigation did not follow event order")
		}
	default:
		t.Fatal("Ctrl+R did not refresh after switching panels")
	}
}
