package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/key"
)

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
	advance(historySearchDelay - time.Millisecond)
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
	advance(historySearchDelay)
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
	advance(historySearchDelay)
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
