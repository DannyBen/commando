package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func metadataSortFiles() []browser.Entry {
	old, recent := time.Unix(1000, 0), time.Unix(2000, 0)
	return []browser.Entry{
		{Name: "..", Directory: true, Parent: true},
		{Name: "folder2", Directory: true, Modified: old},
		{Name: "folder10", Directory: true, Modified: recent},
		{Name: "folder1", Directory: true, Modified: recent},
		{Name: "unknown-folder", Directory: true},
		{Name: "readme", Bytes: 1, SizeKnown: true, Modified: old},
		{Name: ".env", SizeKnown: true, Modified: recent},
		{Name: ".config.json", Bytes: 20, SizeKnown: true, Modified: old},
		{Name: "b.TXT", Bytes: 300, SizeKnown: true, Modified: recent},
		{Name: "a.txt", Bytes: 300, SizeKnown: true, Modified: recent},
		{Name: "archive.tar.gz"},
		{Name: "z.bin", SizeKnown: true},
	}
}

func sortedMetadataPanel(mode metadataColumn, enabled bool) panel {
	p := panel{path: filepath.Clean("/sample"), metadata: mode, sortByMetadata: enabled, selected: -1}
	files := metadataSortFiles()
	slices.SortFunc(files, browser.CompareEntries)
	p.reconcileDirectoryListing(browser.Directory{Path: p.path, Entries: files})
	return p
}

func TestMetadataSortOrders(t *testing.T) {
	folders := []string{"..", "folder1", "folder2", "folder10", "unknown-folder"}
	nameFiles := []string{".config.json", ".env", "a.txt", "archive.tar.gz", "b.TXT", "readme", "z.bin"}
	for _, tc := range []struct {
		name string
		mode metadataColumn
		want []string
	}{
		{"none", metadataNone, append(slices.Clone(folders), nameFiles...)},
		{"extension", metadataExtension, append(slices.Clone(folders), ".env", "readme", "z.bin", "archive.tar.gz", ".config.json", "a.txt", "b.TXT")},
		{"size", metadataSize, append(slices.Clone(folders), "a.txt", "b.TXT", ".config.json", "readme", ".env", "z.bin", "archive.tar.gz")},
		{"modified", metadataModified, []string{"..", "folder1", "folder10", "folder2", "unknown-folder", ".env", "a.txt", "b.TXT", ".config.json", "readme", "archive.tar.gz", "z.bin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sortedMetadataPanel(tc.mode, true)
			if got := visibleNames(&p); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			p.sortByMetadata = false
			p.resort()
			if got := visibleNames(&p); !slices.Equal(got, append(slices.Clone(folders), nameFiles...)) {
				t.Fatalf("disabled sort did not restore name order: %v", got)
			}
		})
	}
}

func TestMetadataSortPreservesBrowsingAndFilters(t *testing.T) {
	v := newView([]panel{sortedMetadataPanel(metadataSize, false), sortedMetadataPanel(metadataSize, false)})
	p := &v.panels[0]
	selectName(t, p, "readme")
	p.toggleMark(p.selected)
	p.list.Position.First, p.list.Position.Offset = p.selected, 7
	p.prefix, p.err, p.reveal = "r", errors.New("retained error"), false
	beforeCache, err := slices.Clone(p.entries), p.err
	v.browseKey(layout.Context{}, modified(key.NameF3, key.ModCtrl))
	if !p.sortByMetadata || v.panels[1].sortByMetadata || v.active != 0 ||
		p.items[p.selected].name != "readme" || !p.marks[p.selected] || len(p.marks) != 1 ||
		p.items[p.list.Position.First].name != "readme" || p.list.Position.Offset != 7 ||
		p.prefix != "r" || p.err != err || p.reveal || !slices.Equal(p.entries, beforeCache) {
		t.Fatal("sorting changed panel context, browsing state, marks, viewport, or complete cache")
	}
	// Filtering chooses the nearest remaining item in the active sort order.
	selectName(t, p, ".env")
	p.toggleMark(p.selected)
	p.list.Position.First = p.selected
	p.visibility = visibilityHideHiddenDotfiles
	p.refilter()
	if p.items[p.selected].name != "z.bin" || p.items[p.list.Position.First].name != "z.bin" || len(p.marks) != 1 {
		t.Fatal("filtering used name order or retained marks on invisible entries")
	}
}

func TestMetadataSortKeysAndPalette(t *testing.T) {
	v := newView([]panel{sortedMetadataPanel(metadataSize, false), sortedMetadataPanel(metadataSize, false)})
	send, _ := panelInputFrames(v)
	send(modified(key.NameF3, key.ModCtrl), modified("M", key.ModCtrl), pressed(key.NameHome), pressed(key.NameDownArrow))
	if !v.panels[0].sortByMetadata || v.panels[0].metadata != metadataModified || v.panels[0].items[v.panels[0].selected].name != "folder1" {
		t.Fatal("sorting/column/navigation keys were not handled in event order")
	}
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != metadataNone || !v.panels[0].sortByMetadata {
		t.Fatal("None cleared the sort preference")
	}
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != metadataExtension || !slices.Equal(visibleNames(&v.panels[0])[5:7], []string{".env", "readme"}) {
		t.Fatal("showing metadata did not resume sorting")
	}
	send(key.Event{Name: key.NameF3, Modifiers: key.ModCtrl, State: key.Release}, pressed(key.NameF3), modified(key.NameF3, key.ModCtrl|key.ModShift))
	if !v.panels[0].sortByMetadata {
		t.Fatal("key release or another modifier toggled sorting")
	}
	send(pressed(key.NameTab), pressed("`"))
	send(key.EditEvent{Text: "Sort by metadata"}, pressed(key.NameReturn))
	if v.active != 1 || !v.panels[1].sortByMetadata || !v.interaction.idle() {
		t.Fatal("palette sort did not use its originating panel and browsing handler")
	}
	send(modified("D", key.ModAlt))
	text := v.location.Text()
	send(modified(key.NameF3, key.ModCtrl))
	if !v.locationEditing() || text != v.location.Text() || !v.panels[1].sortByMetadata {
		t.Fatal("sort shortcut escaped the location editor")
	}
	for _, configure := range []func(*panel){
		func(p *panel) { p.loading = true }, func(p *panel) { p.drives = true },
	} {
		busy := newView([]panel{sortedMetadataPanel(metadataSize, false)})
		configure(&busy.panels[0])
		busy.browseKey(layout.Context{}, modified(key.NameF3, key.ModCtrl))
		if busy.panels[0].sortByMetadata {
			t.Fatal("sort shortcut changed a busy panel or drive list")
		}
	}
}

func TestMetadataSortUpdatesAndRefresh(t *testing.T) {
	p := sortedMetadataPanel(metadataSize, true)
	selectName(t, &p, "readme")
	p.toggleMark(p.selected)
	p.insertCreatedEntry(browser.Entry{Name: "new.txt", Bytes: 500, SizeKnown: true}, false)
	if p.items[5].name != "new.txt" || p.items[p.selected].name != "readme" || !p.marks[p.selected] {
		t.Fatal("insertion lost sort order, current item, or marks")
	}
	p.applyTransferredEntry(browser.TransferredItem{Replaced: true, Entry: browser.Entry{Name: "readme", Bytes: 1000, SizeKnown: true}})
	if p.items[5].name != "readme" || p.items[p.selected].name != "readme" || !p.marks[p.selected] {
		t.Fatal("replacement did not reorder updated metadata while preserving identity")
	}
	v := newView([]panel{p})
	v.panels[0].sortByMetadata = true
	updated := slices.Clone(p.entries)
	for i := range updated {
		if updated[i].Name == "new.txt" {
			updated[i].Bytes = 2000
		}
	}
	v.acceptLoad(loadResult{request: loadRequest{panel: 0, path: p.path, refresh: true}, directory: browser.Directory{Path: p.path, Entries: updated}})
	if v.panels[0].items[5].name != "new.txt" || v.panels[0].items[v.panels[0].selected].name != "readme" {
		t.Fatal("manual refresh lost sort order or current identity")
	}
}

func TestMetadataSortStateAndPanelSwap(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	saved := []panelState{{Path: first, Metadata: "size", SortByMetadata: true}, {Path: second, Metadata: "modified"}}
	locations, err := startingLocations([]string{second}, saved)
	if err != nil || !locations[0].SortByMetadata || locations[1].SortByMetadata {
		t.Fatalf("startup paths lost independent sort choices: %v %v", locations, err)
	}
	v := newRestoredView(saved, 0, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	send, _ := panelInputFrames(v)
	send(modified(key.NameRightArrow, key.ModShift))
	state := v.sessionState(windowState{})
	if state.Panels[0].SortByMetadata || !state.Panels[1].SortByMetadata || state.Panels[1].Path != first {
		t.Fatal("swapping panes did not carry their sort preference")
	}
	dataDir := t.TempDir()
	if err := saveState(dataDir, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadState(dataDir)
	if err != nil || loaded.Panels[0].SortByMetadata || !loaded.Panels[1].SortByMetadata {
		t.Fatal("sort choices did not survive persistence")
	}
	// Earlier state files have no preference and therefore start with sorting off.
	if err := os.WriteFile(filepath.Join(dataDir, "state.json"), []byte(`{"panels":[{"path":"/sample","metadata":"size"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadState(dataDir)
	if err != nil || loaded.Panels[0].SortByMetadata {
		t.Fatal("missing sort choice did not default to off")
	}
}

func TestMetadataSortPreview(t *testing.T) {
	path := os.Getenv("COMMANDO_PREVIEW_SORT")
	if path == "" {
		t.Skip("optional headless metadata sorting preview")
	}
	v := newPreviewView()
	for i, mode := range []metadataColumn{metadataExtension, metadataSize, metadataModified} {
		v.panels[i].metadata, v.panels[i].sortByMetadata = mode, true
		v.panels[i].resort()
	}
	v.active = 1
	if err := renderViewPNG(path, v); err != nil {
		t.Fatal(err)
	}
}
