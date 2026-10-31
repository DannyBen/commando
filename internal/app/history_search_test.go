package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func TestHistorySearchConfiguredDelayAndImmediateEnter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delay time.Duration
		enter bool
	}{
		{"zero", 0, false},
		{"short", 50 * time.Millisecond, false},
		{"long", 1500 * time.Millisecond, false},
		{"maximum", 2 * time.Second, false},
		{"Enter", 2 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := resolvedTempDir(t)
			target := filepath.Join(root, "parent", "history-target")
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			v := newBrowserView([]string{root}, nil)
			t.Cleanup(v.finishOperations)
			waitLoaded(t, v, 0)
			if err := v.settingsValues.Set(settings.CatalogueDefault, settingHistoryMatchDelay, settings.Value{Number: float64(tc.delay / time.Millisecond)}); err != nil {
				t.Fatal(err)
			}
			if err := v.settingsValues.Set(settings.CatalogueDefault, settingSaveHistory, settings.Value{Bool: false}); err != nil {
				t.Fatal(err)
			}
			v.visited.remember(target)
			send, advance := panelInputFrames(v)
			send(modified("D", key.ModAlt))
			query := ">historytarget"
			send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query},
				key.SelectionEvent{Start: len(query), End: len(query)})
			p := &v.panels[0]
			if tc.enter {
				send(pressed(key.NameReturn))
				waitLoaded(t, v, 0)
				if p.path != target || v.locationEditing() {
					t.Fatal("Enter waited for the configured delay")
				}
				return
			}
			if tc.delay > 0 {
				advance(tc.delay - time.Millisecond)
				if p.path != root || p.loading {
					t.Fatal("search read a directory before the configured deadline")
				}
				advance(time.Millisecond)
			}
			waitLoaded(t, v, 0)
			if p.path != filepath.Dir(target) || p.items[p.selected].file.Name != filepath.Base(target) {
				t.Fatal("session history did not preview its match at the configured deadline")
			}
		})
	}
}

func TestHistorySearchShowsParentAndEnterOpensFolder(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "go")
	target := filepath.Join(parent, "commando")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(parent, "commander")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.finishOperations)
	waitLoaded(t, v, 0)
	v.visited.remember(target)
	v.visited.remember(other)
	send, advance := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	query := ">gcom"
	send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query},
		key.SelectionEvent{Start: utf8.RuneCountInString(query), End: utf8.RuneCountInString(query)})
	if v.panels[0].path != root || v.panels[0].loading {
		t.Fatal("search read a directory before debounce")
	}
	advance(v.historySearchDelay() - time.Millisecond)
	if v.panels[0].path != root || v.panels[0].loading {
		t.Fatal("search read a directory before its deadline")
	}
	advance(time.Millisecond)
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	if p.path != parent || p.selected < 0 || p.items[p.selected].file.Name != "commando" || v.location.Text() != query || !v.locationEditing() {
		t.Fatalf("parent search state: path=%q selected=%d text=%q editing=%v", p.path, p.selected, v.location.Text(), v.locationEditing())
	}
	send(pressed(key.NameTab))
	if p.items[p.selected].file.Name != "commander" {
		t.Fatal("Tab did not cycle to the next remembered folder")
	}
	send(pressed(key.NameTab))
	if p.items[p.selected].file.Name != "commando" {
		t.Fatal("Tab did not cycle back to the best match")
	}
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if p.path != target || v.locationEditing() {
		t.Fatalf("Enter did not open result: path=%q editing=%v", p.path, v.locationEditing())
	}
}

func TestHistorySearchEscapeKeepsDisplayedParent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "go", "commando")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.finishOperations)
	waitLoaded(t, v, 0)
	v.visited.remember(target)
	send, advance := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	query := ">comm"
	send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query},
		key.SelectionEvent{Start: utf8.RuneCountInString(query), End: utf8.RuneCountInString(query)})
	advance(v.historySearchDelay())
	waitLoaded(t, v, 0)
	send(pressed(key.NameEscape))
	if v.panels[0].path != filepath.Dir(target) || v.locationEditing() {
		t.Fatal("Escape did not leave the displayed parent in place")
	}
}

func TestHistorySearchPrunesMissingFolder(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "go", "commando")
	if err := os.MkdirAll(filepath.Dir(missing), 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.finishOperations)
	waitLoaded(t, v, 0)
	v.visited.remember(missing)
	send, advance := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	query := ">comm"
	send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query},
		key.SelectionEvent{Start: utf8.RuneCountInString(query), End: utf8.RuneCountInString(query)})
	advance(v.historySearchDelay())
	waitLoaded(t, v, 0)
	if len(v.visited) != 1 || v.visited[0] != root {
		t.Fatalf("stale history entry remains: %v", v.visited)
	}
	send()
	// Another saved path can also match the fuzzy query, especially when its
	// Windows temp root contains the letters in "comm". Only the stale target
	// must stay out of subsequent search results.
	if s := v.interaction.current.(*locationSession); samePath(s.searchTarget, missing) {
		t.Fatal("stale entry was selected again")
	}
}
