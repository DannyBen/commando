package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
)

func TestDeepFilterSearchesEntriesAndKeepsAncestors(t *testing.T) {
	root := resolvedTempDir(t)
	for _, dir := range []string{"section", "portal-dir", "unrelated"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"section/portal.txt", "section/other.txt", "unrelated/other.txt"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.loader.close)
	waitLoaded(t, v, 0)
	v.startDeepSearch(0, "portal")
	deadline := time.Now().Add(5 * time.Second)
	for v.panels[0].deep.searching && time.Now().Before(deadline) {
		v.pollLoads()
		time.Sleep(time.Millisecond)
	}
	p := &v.panels[0]
	if p.deep.searching || len(p.items) != 3 || !hasEntry(p, "section") || !hasEntry(p, "portal-dir") || hasEntry(p, "unrelated") {
		t.Fatalf("root projection did not retain matches and guide folders: %v", visibleNames(p))
	}
	selectName(t, p, "section")
	v.activateSelected()
	waitLoaded(t, v, 0)
	if p.deep == nil || !hasEntry(p, "portal.txt") || hasEntry(p, "other.txt") {
		t.Fatalf("recursive filter did not survive navigation: %v", visibleNames(p))
	}
	v.parentFolder()
	waitLoaded(t, v, 0)
	if p.deep == nil || p.path != root || !hasEntry(p, "section") {
		t.Fatal("returning to the search root canceled the filter")
	}
	selectName(t, p, "section")
	v.activateSelected()
	waitLoaded(t, v, 0)
	v.escape(layout.Context{})
	if p.deep != nil || !hasEntry(p, "other.txt") || p.path != filepath.Join(root, "section") {
		t.Fatal("Escape did not restore the current folder")
	}
}

func TestDeepSearchRootContainsChildren(t *testing.T) {
	root := string(filepath.Separator)
	if !deepContains(root, filepath.Join(root, "folder")) || !deepContains(root, root) {
		t.Fatal("filesystem root did not contain its child")
	}
	if !deepContains(filepath.Join(root, "work"), filepath.Join(root, "work", "folder")) ||
		deepContains(filepath.Join(root, "work"), filepath.Join(root, "workspace")) {
		t.Fatal("non-root containment included a sibling or lost a child")
	}
}

func TestDeepSearchProgressOwnsFooterUntilCompletion(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	d := &deepFilter{root: p.path, query: "absent", matches: make(map[string]bool), searching: true}
	d.started = time.Now().Add(-time.Second)
	d.progress.QuietProgressAt("Searching…", d.started)
	p.deep = d
	frame, owns := d.progress.Frame(time.Now(), v.appearance.Feedback)
	if !owns || frame.Text != "Searching…" {
		t.Fatal("search progress was not visible")
	}
	p.feedback.Progress("Reading folder")
	p.feedback.Complete()
	if d.progress.Current() == nil {
		t.Fatal("folder feedback erased search progress")
	}
	v.acceptDeepSearch(loadResult{request: loadRequest{deepSearch: &deepSearchRequest{filter: d}}, deepDone: true})
	if d.searching || d.progress.Current() != nil {
		t.Fatal("completed search retained progress")
	}
}

func TestBrowsingDuringDeepSearchKeepsProgress(t *testing.T) {
	root := resolvedTempDir(t)
	child := filepath.Join(root, "needle-folder")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.loader.close)
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	d := &deepFilter{root: root, query: "needle", matches: make(map[string]bool), searching: true}
	d.add(child)
	d.progress.QuietProgressAt("Searching…", time.Now().Add(-time.Second))
	d.progress.Frame(time.Now(), v.appearance.Feedback)
	p.deep = d
	p.refilter()
	selectName(t, p, "needle-folder")
	v.activateSelected()
	waitLoaded(t, v, 0)
	if p.deep != d || d.progress.Current() == nil {
		t.Fatal("entering a folder interrupted search progress")
	}
	v.parentFolder()
	waitLoaded(t, v, 0)
	if p.deep != d || d.progress.Current() == nil {
		t.Fatal("backing out to search root interrupted search progress")
	}
}

func TestPaletteActionEndsDeepSearch(t *testing.T) {
	v := newPreviewView()
	v.panels[0].deep = &deepFilter{root: v.panels[0].path, query: "needle", matches: make(map[string]bool), cancel: func() {}}
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(replacePalette(v, "Panel Follow me"), pressed(key.NameReturn))
	if v.panels[0].deep != nil {
		t.Fatal("palette action retained the deep search")
	}
}

func TestLocationSubmissionEndsDeepSearch(t *testing.T) {
	v, root, _ := clipboardView(t)
	p := &v.panels[0]
	p.deep = &deepFilter{root: root, query: "needle", matches: make(map[string]bool), cancel: func() {}}
	v.startLocation(layout.Context{})
	v.location.SetText(filepath.Join(root, "folder"))
	v.submitLocation(layout.Context{})
	if p.deep != nil {
		t.Fatal("location submission retained the deep search")
	}
}

func TestAsteriskOpensDeferredDeepFilter(t *testing.T) {
	v := newPreviewView()
	v.loader = newDirectoryLoader(nil, nil, len(v.panels))
	t.Cleanup(v.loader.close)
	send, _ := panelInputFrames(v)
	before := len(v.panels[0].items)
	send(key.EditEvent{Text: "*"})
	if s := v.filterInteraction(); s == nil || !s.deep || len(v.panels[0].items) != before {
		t.Fatal("asterisk did not open a deferred deep filter")
	}
}

func TestSearchFromPalette(t *testing.T) {
	v := newPreviewView()
	v.loader = newDirectoryLoader(nil, nil, len(v.panels))
	t.Cleanup(v.loader.close)
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(replacePalette(v, "Search"))
	s := v.paletteInteraction()
	if s == nil || len(s.results) == 0 || s.results[0].command.id != "view.deep_filter" || s.results[0].command.shortcut() != "* / F3" {
		t.Fatal("Search and its asterisk binding are missing from the palette")
	}
	send(pressed(key.NameReturn))
	if session := v.filterInteraction(); session == nil || !session.deep {
		t.Fatal("palette Search did not open the deep filter")
	}
}

func TestF3OpensDeepSearch(t *testing.T) {
	v := newPreviewView()
	v.loader = newDirectoryLoader(nil, nil, len(v.panels))
	t.Cleanup(v.loader.close)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF3))
	if s := v.filterInteraction(); s == nil || !s.deep {
		t.Fatal("F3 did not open Search")
	}
}

func TestDeepFilterContinuesAcrossWorkerBatches(t *testing.T) {
	root := resolvedTempDir(t)
	for i := range 20 {
		dir := filepath.Join(root, fmt.Sprintf("branch-%02d", i))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if i == 19 {
			if err := os.WriteFile(filepath.Join(dir, "needle.txt"), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.loader.close)
	waitLoaded(t, v, 0)
	v.startDeepSearch(0, "needle")
	deadline := time.Now().Add(5 * time.Second)
	for v.panels[0].deep.searching && time.Now().Before(deadline) {
		v.pollLoads()
		v.resumeDeepSearches()
		time.Sleep(time.Millisecond)
	}
	if v.panels[0].deep.searching || !hasEntry(&v.panels[0], "branch-19") || hasEntry(&v.panels[0], "branch-00") {
		t.Fatal("later search batch did not appear in the panel")
	}
}

func TestEmptyDeepFilterUsesEmptyFilterMessage(t *testing.T) {
	p := panel{deep: &deepFilter{}}
	if got := p.emptyFolderLabel(); got != "No matching entries" {
		t.Fatalf("empty search label = %q", got)
	}
	p.deep.searching = true
	if got := p.emptyFolderLabel(); got != "Searching…" {
		t.Fatalf("pending search label = %q", got)
	}
}

func hasEntry(p *panel, name string) bool {
	for _, item := range p.items {
		if item.file.Name == name {
			return true
		}
	}
	return false
}
