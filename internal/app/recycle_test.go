package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestDeleteQueuesStableMarkedOrCurrentSelection(t *testing.T) {
	for _, marked := range []bool{false, true} {
		v := newPreviewView()
		v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
		p := &v.panels[0]
		if !marked {
			p.clearMarks()
		}
		var want []string
		if marked {
			for row, item := range p.items {
				if p.marks[row] {
					want = append(want, item.file.Name)
				}
			}
			p.selected = 0 // Marked items take precedence even on the parent row.
		} else {
			want = []string{p.items[p.selected].file.Name}
		}
		send, _ := panelInputFrames(v)
		send(key.Event{Name: key.NameDeleteForward, State: key.Release}, modified(key.NameDeleteForward, key.ModShift), modified(key.NameDeleteForward, key.ModCtrl))
		if len(v.loader.requests) != 0 {
			t.Fatal("modifier or release queued deletion")
		}
		send(pressed(key.NameDeleteForward), pressed(key.NameDeleteForward))
		if len(v.loader.requests) != 1 || !p.loading || !p.recycling {
			t.Fatal("Delete was not queued once while busy")
		}
		request := <-v.loader.requests
		send(pressed(key.NameDownArrow), pressed(key.NameEscape))
		if request.recycle == nil || !reflect.DeepEqual(request.recycle.names, want) || request.path != p.path {
			t.Fatalf("selection snapshot changed: %+v, want %v", request.recycle, want)
		}
		send(pressed(key.NameRightArrow), pressed(key.NameF2))
		if v.naming != nil {
			t.Fatal("name mutation started during pending recycling")
		}
		v.acceptLoad(loadResult{request: request, recycled: browser.RecycleResult{Names: want}})
		send()
		if p.loading || p.recycling || v.active != 1 || v.panels[1].feedback.Current() != nil {
			t.Fatal("completion retained busy state or stole focus/feedback")
		}
		message := p.feedback.Current()
		if message == nil || message.Kind != feedback.Success {
			t.Fatalf("missing completion feedback: %v", message)
		}
	}
}

func TestRecycleWorkerPartialFailureUpdatesAliasListings(t *testing.T) {
	root, unrelated := t.TempDir(), t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root, unrelated}, nil)
	defer v.loader.close()
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	v.loader.recycle = func(directory string, names []string) (browser.RecycleResult, error) {
		if directory != root || !reflect.DeepEqual(names, []string{"a.txt", "b.txt"}) {
			t.Fatalf("wrong backend request: %q %v", directory, names)
		}
		return browser.RecycleResult{Directory: root, Names: []string{"a.txt"}}, &os.PathError{Op: "recycle", Path: "b.txt", Err: os.ErrPermission}
	}
	p := &v.panels[0]
	for _, name := range []string{"a.txt", "b.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	selectName(t, p, "c.txt")
	selectName(t, &v.panels[1], "c.txt")
	v.panels[1].toggleMark(v.panels[1].selected)
	v.panels[1].path = root + string(filepath.Separator) + "."
	otherItems := slices.Clone(v.panels[2].items)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDeleteForward), pressed(key.NameRightArrow))
	waitLoaded(t, v, 0)
	send()
	if v.active != 1 {
		t.Fatal("completion stole focus")
	}
	for pi := 0; pi < 2; pi++ {
		other := &v.panels[pi]
		if len(other.items) != 3 || other.items[1].file.Name != "b.txt" || other.items[other.selected].file.Name != "c.txt" {
			t.Fatalf("partial completion lost survivors or selection in panel %d", pi)
		}
		if len(other.marks) != 1 || !other.marks[[]int{1, 2}[pi]] {
			t.Fatalf("partial completion lost remaining marks in panel %d", pi)
		}
	}
	if !slices.Equal(v.panels[2].items, otherItems) || v.panels[1].feedback.Current() != nil {
		t.Fatal("completion affected an unrelated panel or feedback")
	}
	message := p.feedback.Current()
	if message == nil || message.Kind != feedback.Error || message.Text != "1 recycled; Access denied" {
		t.Fatalf("wrong partial feedback: %v", message)
	}
}

func TestRecycleRefusalPreservesListingAndMarks(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	send, _ := panelInputFrames(v)
	p := &v.panels[0]
	before, selected, position := slices.Clone(p.items), p.selected, p.list.Position
	marks := make(map[int]bool)
	for row, marked := range p.marks {
		marks[row] = marked
	}
	send(pressed(key.NameDeleteForward))
	request := <-v.loader.requests
	v.acceptLoad(loadResult{request: request, err: &os.PathError{Op: "recycle", Path: "assets", Err: browser.ErrRecycleUnavailable}})
	send()
	if !slices.Equal(p.items, before) || p.selected != selected || p.list.Position != position || !reflect.DeepEqual(p.marks, marks) {
		t.Fatal("refusal changed cached state")
	}
	message := p.feedback.Current()
	if message == nil || message.Kind != feedback.Error || message.Text != "Recycle Bin unavailable" {
		t.Fatalf("wrong refusal feedback: %v", message)
	}
}

func TestRecycleCompletionPreservesLocationEditor(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
	send, _ := panelInputFrames(v)
	v.panels[0].clearMarks()
	send(pressed(key.NameDeleteForward))
	request := <-v.loader.requests
	send(modified("D", key.ModAlt))
	v.location.SetText("draft path")
	v.location.SetCaret(2, 6)
	v.acceptLoad(loadResult{request: request, recycled: browser.RecycleResult{Names: request.recycle.names}})
	send()
	first, last := v.location.Selection()
	if !v.editing || v.location.Text() != "draft path" || first != 2 || last != 6 {
		t.Fatal("completion disturbed location editing")
	}
}

func TestRecycledRowsRetainCurrentIdentityAndViewport(t *testing.T) {
	files := []entry{
		{name: "..", file: browser.Entry{Name: "..", Parent: true}},
		{name: "a", file: browser.Entry{Name: "a"}}, {name: "b", file: browser.Entry{Name: "b"}},
		{name: "c", file: browser.Entry{Name: "c"}}, {name: "d", file: browser.Entry{Name: "d"}},
	}
	for _, tc := range []struct {
		selected int
		removed  []string
		want     string
	}{
		{3, []string{"a", "b"}, "c"}, {2, []string{"b"}, "c"}, {4, []string{"d"}, "c"},
		{1, []string{"a", "b", "c", "d"}, ".."},
	} {
		p := panel{items: slices.Clone(files), selected: tc.selected, marks: map[int]bool{4: true}}
		p.list.Position = layout.Position{First: 3, Offset: 7}
		p.removeEntries(tc.removed)
		if p.items[p.selected].file.Name != tc.want {
			t.Fatalf("selected %q, want %q", p.items[p.selected].file.Name, tc.want)
		}
		if p.list.Position.Offset != 7 {
			t.Fatal("removal lost the viewport offset")
		}
	}
	p := panel{items: slices.Clone(files), selected: 1}
	p.list.Position = layout.Position{First: 3, Offset: 7}
	p.removeEntries([]string{"b"})
	if p.list.Position.First != 2 || p.reveal {
		t.Fatal("removing a row jumped to a surviving offscreen selection")
	}
	p = panel{items: files[1:2], selected: 0}
	p.removeEntries([]string{"a"})
	if len(p.items) != 0 || p.selected != -1 {
		t.Fatal("empty listing retained a current row")
	}
}

func TestRecycleResultIgnoresNavigatedAliasSnapshot(t *testing.T) {
	v := newPreviewView()
	before := slices.Clone(v.panels[1].items)
	request := loadRequest{panel: 0, path: v.panels[0].path, recycle: &recycleRequest{locations: []string{v.panels[0].path, "previous path"}}}
	v.acceptLoad(loadResult{request: request, recycled: browser.RecycleResult{Names: []string{v.panels[1].items[1].file.Name}}, sameDirectory: []bool{true, true}})
	if !slices.Equal(v.panels[1].items, before) {
		t.Fatal("completion changed a panel that navigated away")
	}
}

func TestDeleteSkipsNavigationRowsAndStaysInEditors(t *testing.T) {
	for _, state := range []string{"parent", "drives", "empty", "busy", "location", "rename", "create"} {
		t.Run(state, func(t *testing.T) {
			v := newPreviewView()
			v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3)}
			v.panels[0].clearMarks()
			send, _ := panelInputFrames(v)
			p := &v.panels[0]
			switch state {
			case "parent":
				p.selected = 0
			case "drives":
				p.drives = true
			case "empty":
				p.items, p.selected = nil, -1
			case "busy":
				p.loading = true
			case "location":
				send(modified("D", key.ModAlt))
			case "rename":
				send(pressed(key.NameF2))
			case "create":
				send(newFolderKey())
			}
			editor := v.activeEditor()
			if editor != nil {
				editor.SetText("abcd")
				editor.SetCaret(1, 1)
				send()
			}
			send(pressed(key.NameDeleteForward))
			if len(v.loader.requests) != 0 || p.feedback.Current() != nil {
				t.Fatal("ignored Delete queued recycling or showed feedback")
			}
			if editor != nil && (v.activeEditor() != editor || editor.Text() != "acd") {
				t.Fatal("Delete stopped editing text")
			}
		})
	}
}

func TestRecycleErrorDetails(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{browser.ErrRecycleUnconfirmed, "Cannot confirm recycling"}, {os.ErrNotExist, "Item not found"},
		{browser.ErrDriveMappingCycle, "Backing path unavailable"}, {errors.New("other"), "Cannot recycle"},
	} {
		if got := recycleError(tc.err); got != tc.want {
			t.Fatalf("%q, want %q", got, tc.want)
		}
	}
}
