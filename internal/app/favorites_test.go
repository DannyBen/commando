package app

import (
	"errors"
	"image"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func favoriteTestView(t *testing.T) (*view, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	v := newLocationView([]panelState{{Path: root}}, 0, nil, t.TempDir())
	t.Cleanup(v.finishOperations)
	waitLoaded(t, v, 0)
	v.loader.createFavorite = func(directory, name, target string) (browser.Entry, error) {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return browser.Entry{}, err
		}
		return browser.CreateFileData(directory, name+".lnk", []byte(target))
	}
	v.loader.folderShortcutTarget = func(path string) (string, error) {
		data, err := os.ReadFile(path)
		return string(data), err
	}
	v.loader.readFavorites = func(directory string) ([]browser.Entry, error) {
		listing, err := browser.ReadDirectory(directory)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var entries []browser.Entry
		for _, file := range listing.Entries {
			if file.Parent || file.Directory || !strings.EqualFold(filepath.Ext(file.Name), ".lnk") {
				continue
			}
			file.Favorite, file.Path = true, filepath.Join(directory, file.Name)
			file.Target, err = v.loader.folderShortcutTarget(file.Path)
			if err != nil {
				return nil, err
			}
			entries = append(entries, file)
		}
		return entries, nil
	}
	return v, root
}

func TestFavoriteNamingCancelSaveOpenAndReveal(t *testing.T) {
	v, root := favoriteTestView(t)
	p := &v.panels[0]
	p.setFilter("child")
	p.marks = map[int]bool{p.selected: true}
	send, _ := panelInputFrames(v)
	selected, marks, position := p.selected, maps.Clone(p.marks), p.list.Position
	send(modified("B", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	if n := v.nameInteraction(); n == nil || n.kind != nameFavorite || !p.drives {
		t.Fatal("Favorite did not open inline naming on Home")
	}
	send(pressed(key.NameEscape))
	if p.drives || p.path != root || p.filter != "child" || p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position != position {
		t.Fatal("cancellation did not restore the complete source panel")
	}
	if _, err := os.Stat(v.favoritesDirectory()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("cancelling naming created storage")
	}
	send(modified("B", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	p.rows[1].Click() // Drive row below the naming draft; it is not a source-folder row.
	send()
	if !v.interaction.idle() || p.drives || p.path != root || p.selected != selected || !maps.Equal(p.marks, marks) {
		t.Fatal("cancelling through a Home row clicked through to the restored folder")
	}
	send(pressed("`"))
	if v.paletteInteraction() == nil {
		t.Fatalf("palette did not open after Home cancellation: owner=%T", v.interaction.current)
	}
	send(replacePalette(v, "Folder Favorite"), pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if n := v.nameInteraction(); n == nil || n.kind != nameFavorite {
		t.Fatalf("palette did not enter favorite naming: owner=%T home=%v loading=%v feedback=%+v", v.interaction.current, p.drives, p.loading, p.feedback.Current())
	}
	v.nameEditor.SetText("Work")
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	create := v.loader.createFavorite
	v.loader.createFavorite = func(directory, name, target string) (browser.Entry, error) {
		<-gate
		return create(directory, name, target)
	}
	send(pressed(key.NameReturn))
	if n := v.nameInteraction(); n == nil || !n.pending || v.requestClose() {
		t.Fatal("submitted favorite did not hold mutation ownership")
	}
	release()
	v.acceptLoad(<-v.loader.results)
	send()
	if !v.interaction.idle() || !p.drives || len(p.items) < 2 || p.items[0].file.Name != "Work.lnk" || p.selected != 0 || p.metadata.displayName(p.items[0]) != "Work" {
		t.Fatal("saved favorite was not selected before drives", p.items)
	}
	data, err := os.ReadFile(filepath.Join(v.favoritesDirectory(), "Work.lnk"))
	if err != nil || string(data) != root {
		t.Fatal("favorite did not save the captured source folder", string(data), err)
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if p.drives || p.path != root {
		t.Fatal("opening a favorite did not navigate inside Commando")
	}
	send(modified("B", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	if !v.interaction.idle() || !p.drives || p.selected != 0 {
		t.Fatal("favoriting an existing target did not reveal its shortcut")
	}
}

func TestFavoriteCancelledReadCannotReplaceNewOwner(t *testing.T) {
	v := selectionView()
	v.activeDataFolder = t.TempDir()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, _ := panelInputFrames(v)
	send(modified("B", key.ModCtrl))
	request := <-v.loader.requests
	send(pressed(key.NameEscape), pressed("`"))
	owner := v.interaction.current
	v.acceptLoad(loadResult{request: request, directory: browser.Directory{Drives: true}})
	if v.interaction.current != owner || v.panels[0].drives || v.panels[0].loading {
		t.Fatal("cancelled Home lookup replaced a newer interaction")
	}
}

func TestFavoritesReviewPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_FAVORITES")
	if dir == "" {
		t.Skip("optional Favorites review previews")
	}
	v := newPreviewView()
	v.active = 1
	home := browser.Directory{Drives: true, Entries: []browser.Entry{
		{Name: "Desktop.lnk", Path: `C:\Commando\favorites\Desktop.lnk`, Favorite: true, Target: `C:\Users\Danny\Desktop`},
		{Name: "Projects.lnk", Path: `C:\Commando\favorites\Projects.lnk`, Favorite: true, Target: `C:\Work\Projects`},
		{Name: "C:\\", Path: "C:\\", Directory: true},
		{Name: "D:\\", Path: "D:\\", Directory: true},
	}}
	v.rememberFavorites(home)
	v.acceptLoad(loadResult{request: loadRequest{panel: 1}, directory: home})
	if err := renderViewPNG(filepath.Join(dir, "favorites-home.png"), v); err != nil {
		t.Fatal(err)
	}
	s := &favoriteSession{panel: 1, target: `C:\Work\Projects`, previous: v.panels[1]}
	v.beginName(layout.Context{}, &nameSession{kind: nameFavorite, panel: 1, row: 2, favorite: s}, "Commando", true)
	if err := renderSizedViewPNG(filepath.Join(dir, "favorites-name.png"), v, image.Pt(800, 480)); err != nil {
		t.Fatal(err)
	}
}
