package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"strings"
)

func favoriteLocationView(t *testing.T) (*view, string, string) {
	t.Helper()
	v, root := favoriteTestView(t)
	target := t.TempDir()
	for _, name := range []string{"Desktop", "Development"} {
		if _, err := v.loader.createFavorite(v.favoritesDirectory(), name, target); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "Delta"), 0700); err != nil {
		t.Fatal(err)
	}
	return v, root, target
}

func editFavoriteLocation(v *view, send func(...event.Event), query string) {
	n := utf8.RuneCountInString(query)
	if strings.HasSuffix(query, `"`) {
		n--
	}
	send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query}, key.SelectionEvent{Start: n, End: n})
}

func TestFavoriteLocationCompletionBeforeHomeCyclingTabUndoAndEnter(t *testing.T) {
	v, root, target := favoriteLocationView(t)
	if len(v.favorites) != 0 {
		t.Fatal("test must begin without a Home snapshot")
	}
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	editFavoriteLocation(v, send, "De")
	waitLocationCompletion(t, v)
	send()
	s := v.interaction.current.(*locationSession)
	want := []string{"Desktop", "Development", "Delta" + string(filepath.Separator)}
	if !slices.Equal(s.completion.matches, want) || v.location.Text() != "De" || v.panels[0].path != root {
		t.Fatalf("favorites did not join filesystem matches: %v", s.completion.matches)
	}
	send(pressed(key.NameDownArrow), pressed(key.NameDownArrow))
	if s.completion.suffix() != "lta"+string(filepath.Separator) {
		t.Fatal("could not cycle to the filesystem match")
	}
	send(pressed(key.NameDownArrow))
	if s.completion.suffix() != "sktop" {
		t.Fatal("cycling did not wrap to the favorite")
	}
	send(pressed(key.NameTab))
	if !v.locationEditing() || v.location.Text() != target || v.panels[0].path != root {
		t.Fatal("Tab did not expand the alias without navigating")
	}
	send(modified("Z", key.ModCtrl))
	if v.location.Text() != "De" {
		t.Fatal("accepting a favorite broke editor Undo")
	}
	// Native Undo selects the restored edit; completion resumes at the end.
	send(key.SelectionEvent{Start: 2, End: 2})
	waitLocationCompletion(t, v)
	send()
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if v.panels[0].path != target || v.locationEditing() {
		t.Fatalf("Enter did not navigate to the favorite target: path=%q target=%q matches=%v query=%q feedback=%+v", v.panels[0].path, target, s.completion.matches, s.completion.query, v.panels[0].feedback.Current())
	}
}

func TestFavoriteLocationLiteralPrecedenceAndExplicitAcceptance(t *testing.T) {
	for _, scenario := range []string{"exact alias", "literal prefix", "literal alias", "Tab accepts favorite", "cycle to local folder", "cycle to same-name file"} {
		t.Run(scenario, func(t *testing.T) {
			v, root, target := favoriteLocationView(t)
			query, want := "De", target
			switch scenario {
			case "exact alias":
				query = "desktop"
			case "literal prefix":
				want = filepath.Join(root, "De")
				if err := os.Mkdir(want, 0700); err != nil {
					t.Fatal(err)
				}
			case "literal alias", "Tab accepts favorite", "cycle to local folder":
				local := filepath.Join(root, "Desktop")
				if err := os.Mkdir(local, 0700); err != nil {
					t.Fatal(err)
				}
				if scenario == "literal alias" {
					query, want = "Desktop", local
				}
				if scenario == "cycle to local folder" {
					want = local
				}
			case "cycle to same-name file":
				want = root
				if err := os.WriteFile(filepath.Join(root, "Desktop"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			send, _ := panelInputFrames(v)
			send(modified("D", key.ModAlt))
			editFavoriteLocation(v, send, query)
			waitLocationCompletion(t, v)
			send()
			if scenario == "Tab accepts favorite" {
				send(pressed(key.NameTab))
			}
			if scenario == "cycle to local folder" || scenario == "cycle to same-name file" {
				s := v.interaction.current.(*locationSession)
				// Locate the filesystem candidate after the two favorite aliases.
				for s.completion.selected < len(s.completion.matches)-1 {
					send(pressed(key.NameDownArrow))
				}
				send(pressed(key.NameTab))
			}
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			if v.panels[0].path != want {
				t.Fatalf("path=%q want=%q", v.panels[0].path, want)
			}
			if scenario == "cycle to same-name file" && v.panels[0].items[v.panels[0].selected].file.Name != "Desktop" {
				t.Fatal("same-name local file was treated as a favorite")
			}
		})
	}
}

func TestFavoriteLocationQuotesAndExplicitPaths(t *testing.T) {
	v, root, target := favoriteLocationView(t)
	if _, err := v.loader.createFavorite(v.favoritesDirectory(), "Desk space", target); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	editFavoriteLocation(v, send, filepath.Join(root, "De"))
	waitLocationCompletion(t, v)
	s := v.interaction.current.(*locationSession)
	if !slices.Equal(s.completion.matches, []string{filepath.Join(root, "Delta") + string(filepath.Separator)}) {
		t.Fatal("aliases appeared inside an explicit path", s.completion.matches)
	}
	editFavoriteLocation(v, send, `"Desk s"`)
	waitLocationCompletion(t, v)
	send(pressed(key.NameTab))
	if v.location.Text() != `"`+target+`"` {
		t.Fatal("favorite completion lost quotes", v.location.Text())
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if v.panels[0].path != target {
		t.Fatal("quoted alias did not navigate to its target")
	}
}

func TestFavoriteLocationEnterBeforeLookupFinishes(t *testing.T) {
	v, _, target := favoriteLocationView(t)
	read := v.loader.readFavorites
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	v.loader.readFavorites = func(directory string) ([]browser.Entry, error) { <-gate; return read(directory) }
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	editFavoriteLocation(v, send, "Desktop")
	send(pressed(key.NameReturn))
	if !v.panels[0].loading {
		t.Fatal("Enter did not submit navigation while lookup was pending")
	}
	release()
	waitLoaded(t, v, 0)
	if v.panels[0].path != target {
		t.Fatal("exact alias depended on completion timing")
	}
}

func TestFavoriteLocationFreshSessionsQuietFailuresAndMissingStore(t *testing.T) {
	v, root := favoriteTestView(t)
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	waitLocationCompletion(t, v)
	if _, err := os.Stat(v.favoritesDirectory()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("completion created an empty favorites store")
	}
	send(pressed(key.NameEscape))
	if _, err := v.loader.createFavorite(v.favoritesDirectory(), "Child", filepath.Join(root, "child")); err != nil {
		t.Fatal(err)
	}
	send(modified("D", key.ModAlt))
	editFavoriteLocation(v, send, "Ch")
	waitLocationCompletion(t, v)
	if v.interaction.current.(*locationSession).completion.suffix() != "ild" {
		t.Fatal("a new session retained the old favorite snapshot")
	}
	send(pressed(key.NameEscape))
	v.loader.readFavorites = func(string) ([]browser.Entry, error) { return nil, fs.ErrPermission }
	send(modified("D", key.ModAlt))
	editFavoriteLocation(v, send, "ch")
	waitLocationCompletion(t, v)
	if v.interaction.current.(*locationSession).completion.suffix() != "ild"+string(filepath.Separator) || v.panels[0].feedback.Current() != nil {
		t.Fatal("favorite lookup failure broke normal completion or showed feedback")
	}
}

func TestFavoriteLocationOldLookupCannotChangeNewSession(t *testing.T) {
	v := newPreviewView()
	v.activeDataFolder = t.TempDir()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 8)}
	v.startLocation(layout.Context{})
	v.updateLocationCompletion()
	request := <-v.loader.requests
	v.stopLocation(layout.Context{})
	v.startLocation(layout.Context{})
	current := v.interaction.current.(*locationSession)
	v.acceptLoad(loadResult{request: request, favorites: []browser.Entry{{Name: "Desktop.lnk", Target: "stale"}}})
	if len(current.favorites) != 0 || current.favoritesLoaded {
		t.Fatal("old favorite lookup populated a new editor")
	}
}

func TestFavoriteLocationPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_FAVORITES")
	if dir == "" {
		t.Skip("optional favorite completion preview")
	}
	v := newPreviewView()
	v.active = 1
	v.startLocation(layout.Context{})
	s := v.interaction.current.(*locationSession)
	s.favoritesLoaded = true
	s.favorites = []browser.Entry{{Name: "Desktop.lnk", Target: `C:\Users\Danny\Desktop`, Favorite: true}}
	plan, _ := completionLocation(v.panels[1].locationPath(), "De")
	s.completionPath, s.completionLoaded = plan.directory, true
	s.completionEntries = []browser.Entry{{Name: "Delta", Directory: true}}
	v.location.SetText("De")
	v.location.SetCaret(2, 2)
	if err := renderViewPNG(filepath.Join(dir, "favorite-location-completion.png"), v); err != nil {
		t.Fatal(err)
	}
}
