package app

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func newBrowserView(paths []string, invalidate func()) *view {
	locations := make([]panelState, len(paths))
	for i, path := range paths {
		locations[i].Path = path
	}
	v := newLocationView(locations, 0, invalidate)
	v.loader.readFileIcon = nil
	// These navigation fixtures also exercise the optional parent row.
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingParentEntry, settings.Value{Bool: true})
	v.applySettings()
	return v
}

func navigationFrames(v *view) (frame func(), press func(key.Name)) {
	var router input.Router
	var ops op.Ops
	frame = func() {
		ops.Reset()
		v.layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(800, 480)),
		})
		router.Frame(&ops)
	}
	frame()
	frame()
	press = func(name key.Name) {
		router.Queue(key.Event{Name: name, State: key.Press})
		frame()
		frame()
	}
	return
}

func waitLoaded(t *testing.T, v *view, index int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for v.panels[index].loading {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("directory load did not complete")
		}
	}
}

func selectName(t *testing.T, p *panel, name string) {
	t.Helper()
	for i, item := range p.items {
		if item.file.Name == name {
			p.selected, p.reveal = i, true
			return
		}
	}
	t.Fatalf("entry %q missing from %s", name, p.path)
}

func TestRealFolderNavigation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	names := []string{"child", "empty", "gone", "zz-folder"}
	for i := 0; i < 25; i++ {
		names = append(names, fmt.Sprintf("folder-%02d", i))
	}
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "child", "data.bin"), make([]byte, 2048), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, root, root}, nil)
	v.openFile = nil
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	frame, press := navigationFrames(v)
	p := &v.panels[0]
	if p.itemCount() != 30 {
		t.Fatalf("item count=%d", p.itemCount())
	}
	press(key.NameReturn)
	waitLoaded(t, v, 0)
	frame()
	if p.path != filepath.Join(root, "child") || p.itemCount() != 1 || p.items[1].size != "2.0 KB" {
		t.Fatalf("Enter did not load real folder/files: path=%s items=%+v", p.path, p.items)
	}
	if v.panels[1].path != root || v.panels[2].path != root {
		t.Fatal("navigation changed another panel's directory")
	}
	press(key.NameDeleteBackward)
	waitLoaded(t, v, 0)
	frame()
	if p.path != root || p.items[p.selected].file.Name != "child" {
		t.Fatal("Backspace did not select the folder just left")
	}
	selectName(t, p, "zz-folder")
	frame()
	press(key.NameReturn)
	waitLoaded(t, v, 0)
	frame()
	if p.itemCount() != 0 || len(p.items) != 1 || !p.items[0].file.Parent {
		t.Fatal("empty folder did not retain its parent navigation row")
	}
	press(key.NameReturn)
	waitLoaded(t, v, 0)
	frame()
	if p.path != root || p.items[p.selected].file.Name != "zz-folder" || p.list.Position.First == 0 {
		t.Fatal("Enter on parent did not restore and reveal the folder just left")
	}
	position := p.list.Position
	press(key.NameRightArrow)
	press(key.NameEnd)
	press(key.NameReturn)
	if v.panels[1].loading || v.panels[1].path != root {
		t.Fatal("Enter on an ordinary file navigated or loaded a folder")
	}
	press(key.NameLeftArrow)
	if p.items[p.selected].file.Name != "zz-folder" || p.list.Position != position {
		t.Fatal("switching panels lost directory selection/scroll")
	}
	if err := os.Remove(filepath.Join(root, "gone")); err != nil {
		t.Fatal(err)
	}
	selectName(t, p, "gone")
	frame()
	press(key.NameReturn)
	waitLoaded(t, v, 0)
	frame()
	if p.path != root || p.err != nil || p.selected != 1 {
		t.Fatal("disappeared child did not recover to its surviving parent")
	}
	for _, item := range p.items {
		if item.file.Name == "gone" {
			t.Fatal("recovered listing retained the disappeared folder")
		}
	}
	press(key.NameDownArrow)
	if p.err != nil {
		t.Fatal("continued navigation did not clear the old error")
	}
}

func TestNavigationWhileLoading(t *testing.T) {
	entered, release, notified := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	released := false
	v := newPreviewView()
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		close(entered)
		<-release
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "result.txt", Bytes: 3, SizeKnown: true}}}, nil
	}, func() { notified <- struct{}{} })
	defer v.loader.close()
	defer func() {
		if !released {
			close(release)
		}
	}()
	v.panels[0].items[3].file = browser.Entry{Name: "commando", Directory: true}
	_, press := navigationFrames(v)
	press(key.NameReturn)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("background load never started")
	}
	press(key.NameReturn) // Ignore repeated activation while the panel is loading.
	press(key.NameRightArrow)
	press(key.NameDownArrow)
	if v.active != 1 || v.panels[1].selected != 6 || !v.panels[0].loading || len(v.loader.requests) != 0 {
		t.Fatal("pending directory read blocked navigation or queued a duplicate request")
	}
	close(release)
	released = true
	waitLoaded(t, v, 0)
	select {
	case <-notified:
	case <-time.After(3 * time.Second):
		t.Fatal("completed load did not invalidate the window")
	}
	if v.active != 1 || v.panels[0].items[0].name != "result.txt" {
		t.Fatal("completed load stole panel focus or failed to update its own panel")
	}
}

func TestEmptyRootCanOpenEmptyDriveList(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	v := newView([]panel{{path: root, selected: -1}})
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		if path != "" {
			t.Errorf("unexpected folder read %q", path)
		}
		return browser.Directory{Drives: true}, nil
	}, nil)
	defer v.loader.close()
	frame, press := navigationFrames(v)
	press(key.NameDeleteBackward)
	waitLoaded(t, v, 0)
	frame()
	if !v.panels[0].drives || v.panels[0].selected != -1 {
		t.Fatal("empty root did not enter the empty drive list")
	}
	for _, name := range []key.Name{key.NameUpArrow, key.NameDownArrow, key.NameReturn, key.NameDeleteBackward} {
		press(name)
	}
	if v.panels[0].selected != -1 || v.panels[0].loading {
		t.Fatal("empty drive-list navigation changed selection or queued a read")
	}
}

func TestDriveListNavigation(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	unavailable := filepath.Join(t.TempDir(), "unavailable-drive")
	entries := []browser.Entry{
		{Name: "Unavailable", Path: unavailable, Directory: true},
		{Name: "System", Path: root, Directory: true},
	}
	v := newView([]panel{{path: root, selected: -1}, {path: root, selected: -1}})
	lists := 0
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		switch path {
		case "":
			lists++
			return browser.Directory{Drives: true, Entries: entries}, nil
		case root:
			return browser.Directory{Path: root}, nil
		case unavailable:
			return browser.Directory{}, os.ErrPermission
		default:
			return browser.Directory{}, fmt.Errorf("unexpected location %q", path)
		}
	}, nil)
	defer v.loader.close()
	send, _ := panelInputFrames(v)
	p := &v.panels[0]
	send(pressed(key.NameDeleteBackward))
	waitLoaded(t, v, 0)
	send()
	if !p.drives || p.locationLabel() != "Drives" || p.selected != 1 || p.items[1].name != "System" {
		t.Fatal("root Backspace did not show drives and retain the current drive selection")
	}
	if v.panels[1].drives || v.panels[1].path != root {
		t.Fatal("drive list changed another panel")
	}
	if status, failed := p.status(); status != "2 drives" || failed {
		t.Fatalf("drive status=%q failed=%v", status, failed)
	}
	send(pressed(key.NameDeleteBackward))
	if p.loading || lists != 1 {
		t.Fatal("Backspace left or reloaded the top-level drive list")
	}
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameLeftArrow))
	if !p.drives || p.selected != 1 {
		t.Fatal("switching panels lost drive-list selection")
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if p.drives || p.path != root || p.selected != -1 {
		t.Fatal("Enter did not open the selected drive root")
	}
	send(pressed(key.NameDeleteBackward))
	waitLoaded(t, v, 0)
	send()
	if lists != 2 || !p.drives || p.selected != 1 {
		t.Fatal("returning to drives did not refresh the list and reselect the drive left")
	}
	send(key.EditEvent{Text: "un"})
	if p.selected != 0 {
		t.Fatal("type-to-select did not work in the drive list")
	}
	before := slices.Clone(p.items)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if !p.drives || p.path != root || p.selected != 0 || !slices.Equal(p.items, before) || !errors.Is(p.err, os.ErrPermission) {
		t.Fatal("unreadable drive did not retain the list/selection with the existing inline error")
	}
	send(key.Event{Name: "D", Modifiers: key.ModAlt, State: key.Press})
	if !v.locationEditing() || v.location.SelectedText() != unavailable {
		t.Fatal("Alt+D in the drive list did not use the selected drive path")
	}
	send(pressed(key.NameEscape))
	send(key.EditEvent{Text: "sys"})
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if p.drives || p.path != root || p.err != nil {
		t.Fatal("failed drive opening prevented subsequent drive navigation")
	}
}

func TestGoToDrivesFromNestedFolder(t *testing.T) {
	for _, route := range []string{"keyboard", "palette"} {
		t.Run(route, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "folder")
			root := filepath.VolumeName(path) + string(filepath.Separator)
			v := newView([]panel{{path: path}, {path: path}})
			v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
				if path != "" {
					return browser.Directory{}, fmt.Errorf("unexpected location %q", path)
				}
				return browser.Directory{Drives: true, Entries: []browser.Entry{
					{Name: "Other", Path: filepath.Join(root, "other"), Directory: true},
					{Name: "System", Path: root, Directory: true},
				}}, nil
			}, nil)
			defer v.finishOperations()
			send, _ := panelInputFrames(v)
			p := &v.panels[0]
			p.loading = true
			send(modified(key.NameHome, key.ModCtrl))
			if len(v.loader.requests) != 0 {
				t.Fatal("Ctrl+Home queued navigation in a busy panel")
			}
			p.loading = false
			send(modified("D", key.ModAlt), modified(key.NameHome, key.ModCtrl))
			if !v.locationEditing() || p.loading || p.drives {
				t.Fatal("Ctrl+Home navigated while editing a location")
			}
			send(pressed(key.NameEscape))
			p.filter, p.prefix = "query", "prefix"
			p.marks = map[int]bool{0: true}
			if route == "keyboard" {
				send(modified(key.NameHome, key.ModCtrl))
			} else {
				send(pressed("`"), key.EditEvent{Text: "Navigation Drives"})
				s := v.paletteInteraction()
				if len(s.results) == 0 || s.results[0].command.id != "navigation.drives" {
					t.Fatal("Drives command not found in palette")
				}
				send(pressed(key.NameReturn))
			}
			waitLoaded(t, v, 0)
			send()
			if !p.drives || p.selected != 1 || p.filter != "" || p.prefix != "" || len(p.marks) != 0 {
				t.Fatal("Drives did not select the current drive and clear listing state")
			}
			if v.active != 0 || v.panels[1].drives || v.panels[1].path != path {
				t.Fatal("Drives navigation changed another panel or focus")
			}
			send(modified(key.NameHome, key.ModCtrl))
			if p.loading || p.selected != 1 {
				t.Fatal("Ctrl+Home reloaded or changed the drive list")
			}
		})
	}
}

func TestLargeListKeepsOnlyVisibleRowWidgets(t *testing.T) {
	items := make([]entry, 10000)
	for i := range items {
		items[i] = entry{name: fmt.Sprintf("file-%05d.txt", i), size: "1 KB"}
	}
	v := newView([]panel{{items: items}})
	_, press := navigationFrames(v)
	press(key.NameEnd)
	if v.panels[0].selected != 9999 || v.panels[0].list.Position.First == 0 {
		t.Fatal("large list did not reveal its last entry")
	}
	if count := len(v.panels[0].rows); count > 30 {
		t.Fatalf("large listing retained %d row widgets", count)
	}
}
