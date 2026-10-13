package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func visibilityView(t *testing.T) (*view, *atomic.Int32) {
	t.Helper()
	files := []browser.Entry{
		{Name: "..", Directory: true, Parent: true},
		{Name: ".dotdir", Directory: true},
		{Name: "hidden-dir", Directory: true, Hidden: true},
		{Name: "visible-dir", Directory: true},
		{Name: "$RECYCLE.BIN", Directory: true, Hidden: true, System: true},
		{Name: ".env"},
		{Name: "hidden-file.txt", Hidden: true},
		{Name: "protected.sys", Hidden: true, System: true},
		{Name: "system-only.sys", System: true},
		{Name: "visible.txt"},
	}
	slices.SortFunc(files, browser.CompareEntries)
	root := t.TempDir()
	v := newView([]panel{{path: root, selected: -1}, {path: root, selected: -1}, {path: root, selected: -1}})
	reads := new(atomic.Int32)
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		reads.Add(1)
		return browser.Directory{Path: path, Entries: slices.Clone(files)}, nil
	}, nil)
	t.Cleanup(v.loader.close)
	for pi := range v.panels {
		v.load(pi, root, "")
		waitLoaded(t, v, pi)
	}
	return v, reads
}

func visibleNames(p *panel) []string {
	names := make([]string, len(p.items))
	for i, item := range p.items {
		names[i] = item.name
	}
	return names
}

func TestVisibilityCycleFiltersFilesAndFoldersWithoutReads(t *testing.T) {
	v, reads := visibilityView(t)
	router, send := clipboardFrames(v)
	initialReads := reads.Load()
	all := []string{"..", ".dotdir", "hidden-dir", "visible-dir", ".env", "hidden-file.txt", "system-only.sys", "visible.txt"}
	if !slices.Equal(visibleNames(&v.panels[0]), all) {
		t.Fatal("All did not exclude only protected hidden-system entries")
	}
	for _, tc := range []struct {
		mode  visibilityMode
		names []string
	}{
		{visibilityHideHidden, []string{"..", ".dotdir", "visible-dir", ".env", "system-only.sys", "visible.txt"}},
		{visibilityHideHiddenDotfiles, []string{"..", "visible-dir", "system-only.sys", "visible.txt"}},
		{visibilityAll, all},
	} {
		send(modified("H", key.ModCtrl))
		p := &v.panels[0]
		if p.visibility != tc.mode || !slices.Equal(visibleNames(p), tc.names) || v.active != 0 || !router.Source().Focused(&v.focus) {
			t.Fatalf("incorrect visibility cycle: mode=%v names=%v", p.visibility, visibleNames(p))
		}
		if p.feedback.Current() == nil || p.feedback.Current().Text != tc.mode.label() {
			t.Fatal("mode change did not use the panel footer")
		}
		if !slices.Equal(visibleNames(&v.panels[1]), all) || !slices.Equal(visibleNames(&v.panels[2]), all) || reads.Load() != initialReads {
			t.Fatal("filtering changed another pane or reread the filesystem")
		}
	}
	send(modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	if v.panels[0].visibility != visibilityHideHiddenDotfiles || reads.Load() != initialReads {
		t.Fatal("same-frame mode changes were lost or performed directory reads")
	}
}

func TestVisibilityPreservesVisibleIdentityAndDropsInvisibleMarks(t *testing.T) {
	v, _ := visibilityView(t)
	p := &v.panels[0]
	for _, name := range []string{".env", "hidden-file.txt", "visible.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	selectName(t, p, "visible.txt")
	selectName(t, &v.panels[1], "hidden-file.txt")
	v.panels[1].toggleMark(v.panels[1].selected)
	send, _ := panelInputFrames(v)
	send(modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	if p.items[p.selected].name != "visible.txt" || len(p.marks) != 1 || !p.marks[p.selected] || p.markRange != nil {
		t.Fatal("filter lost current item, kept invisible marks or kept an old mark range")
	}
	if !slices.Equal(p.selectedPaths(), []string{filepath.Join(p.path, "visible.txt")}) {
		t.Fatal("file operations still included an invisible marked item")
	}
	send(modified("H", key.ModCtrl))
	if len(p.marks) != 1 || p.items[p.selected].name != "visible.txt" || len(v.panels[1].marks) != 1 {
		t.Fatal("showing entries resurrected marks or changed another pane")
	}
}

func TestVisibilitySelectsNearestSurvivorAndKeepsViewportAnchor(t *testing.T) {
	v, _ := visibilityView(t)
	p := &v.panels[0]
	selectName(t, p, "hidden-file.txt")
	p.list.Position.First, p.list.Position.Offset = p.selected, 7
	p.visibility = visibilityHideHidden
	p.ensureEntries()
	p.refilter()
	if p.items[p.selected].name != "system-only.sys" || p.items[p.list.Position.First].name != "system-only.sys" || p.list.Position.Offset != 0 {
		t.Fatal("hidden current/viewport anchor did not choose the next surviving row")
	}
	selectName(t, p, "visible.txt")
	p.list.Position.First, p.list.Position.Offset = p.selected, 5
	p.visibility = visibilityAll
	p.refilter()
	if p.items[p.selected].name != "visible.txt" || p.items[p.list.Position.First].name != "visible.txt" || p.list.Position.Offset != 5 {
		t.Fatal("surviving current item or viewport anchor was not retained")
	}
	p.entries, p.items = []browser.Entry{{Name: ".only"}}, []entry{{name: ".only", file: browser.Entry{Name: ".only"}}}
	p.selected, p.visibility = 0, visibilityHideHiddenDotfiles
	p.refilter()
	if len(p.items) != 0 || p.selected != -1 || p.list.Position.First != 0 {
		t.Fatal("empty filtered root retained a selection or invalid viewport")
	}
}

func TestVisibilityIgnoresBusyDrivesAndEditors(t *testing.T) {
	for _, state := range []string{"busy", "drives", "location", "rename", "inline"} {
		t.Run(state, func(t *testing.T) {
			v, _ := visibilityView(t)
			router, send := clipboardFrames(v)
			switch state {
			case "busy":
				v.panels[0].loading = true
			case "drives":
				v.panels[0].drives = true
			case "location":
				send(modified("D", key.ModAlt))
			case "rename":
				selectName(t, &v.panels[0], "visible.txt")
				send(pressed(key.NameF2))
			case "inline":
				v.startInlineActions(0, 1, "", "", []string{"Copy here"}, nil, nil)
			}
			editor := v.activeEditor()
			text, first, last := "", 0, 0
			if editor != nil {
				text = editor.Text()
				first, last = editor.Selection()
			}
			send(modified("H", key.ModCtrl))
			if v.panels[0].visibility != visibilityAll {
				t.Fatal("busy/drive/editor context changed visibility")
			}
			if editor != nil {
				gotFirst, gotLast := editor.Selection()
				if v.activeEditor() != editor || editor.Text() != text || gotFirst != first || gotLast != last || !router.Source().Focused(editor) {
					t.Fatal("Ctrl+H disturbed editor state")
				}
			}
		})
	}
}

func TestVisibilityPersistsPerPaneAndFollowsSwaps(t *testing.T) {
	v, _ := visibilityView(t)
	send, _ := panelInputFrames(v)
	send(modified("H", key.ModCtrl), pressed(key.NameTab), modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	send(modified(key.NameRightArrow, key.ModShift))
	if v.active != 2 || v.panels[2].visibility != visibilityHideHiddenDotfiles || v.panels[1].visibility != visibilityAll {
		t.Fatal("visibility did not travel with its pane")
	}
	dataDir := t.TempDir()
	if err := saveState(dataDir, v.sessionState(windowState{Width: 800, Height: 480})); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hide-hidden", "all", "hide-hidden-dotfiles"}
	for i, state := range saved.Panels {
		if state.Visibility != want[i] {
			t.Fatal("state store did not retain independent visibility choices")
		}
	}
	locations, err := startingLocations([]string{t.TempDir()}, saved.Panels)
	if err != nil {
		t.Fatal(err)
	}
	for i, location := range locations {
		if location.Visibility != want[i] {
			t.Fatal("startup path overrides lost visibility choices")
		}
	}
	restored := newLocationView(locations, saved.ActivePanel, nil)
	t.Cleanup(restored.loader.close)
	for i := range restored.panels {
		waitLoaded(t, restored, i)
		if restored.panels[i].visibility.stateName() != want[i] {
			t.Fatal("restored pane did not apply its saved mode")
		}
	}
	for _, unknown := range []string{"", "unknown"} {
		if visibilityFromState(unknown) != visibilityAll {
			t.Fatal("missing/unknown visibility did not default to All")
		}
	}
}

func TestVisibilityCacheTracksMutationsAndFailedReads(t *testing.T) {
	root := t.TempDir()
	writeBatchFiles(t, root, map[string]string{"visible.txt": "keep", ".old": "dot"})
	v := newBrowserView([]string{root, root}, nil)
	t.Cleanup(v.loader.close)
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	send, _ := panelInputFrames(v)
	send(modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	selectName(t, &v.panels[0], "visible.txt")
	send(pressed(key.NameF2))
	send(replaceRename(v, ".renamed"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	if slices.Contains(visibleNames(&v.panels[0]), ".renamed") || slices.Contains(visibleNames(&v.panels[0]), "visible.txt") {
		t.Fatal("renaming to a dotfile left a stale visible row")
	}
	send(modified("N", key.ModCtrl))
	send(replaceCreate(v, ".created"), pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if slices.Contains(visibleNames(&v.panels[0]), ".created") || !slices.Contains(visibleNames(&v.panels[1]), ".created") {
		t.Fatal("creation ignored pane-specific filtering")
	}
	send(modified("H", key.ModCtrl))
	if !slices.Contains(visibleNames(&v.panels[0]), ".created") || !slices.Contains(visibleNames(&v.panels[0]), ".renamed") || slices.Contains(visibleNames(&v.panels[0]), "visible.txt") {
		t.Fatal("showing entries used a stale mutation cache")
	}
	v.loader.recycle = func(path string, names []string) (browser.RecycleResult, error) {
		if err := os.Remove(filepath.Join(path, names[0])); err != nil {
			return browser.RecycleResult{}, err
		}
		return browser.RecycleResult{Directory: path, Names: names}, nil
	}
	selectName(t, &v.panels[0], ".created")
	send(pressed(key.NameDeleteForward))
	waitLoaded(t, v, 0)
	send()
	send(modified("H", key.ModCtrl), modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	if slices.Contains(visibleNames(&v.panels[0]), ".created") || slices.Contains(visibleNames(&v.panels[1]), ".created") {
		t.Fatal("visibility cycling resurrected a recycled entry")
	}
	before := slices.Clone(v.panels[0].entries)
	v.acceptLoad(loadResult{request: loadRequest{panel: 0}, err: fs.ErrPermission})
	if !reflect.DeepEqual(v.panels[0].entries, before) {
		t.Fatal("failed read discarded the complete listing")
	}
	send(modified("H", key.ModCtrl), modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	if !slices.Contains(visibleNames(&v.panels[0]), ".renamed") {
		t.Fatal("failed read prevented redisplaying cached entries")
	}
	if _, err := os.Stat(filepath.Join(root, ".created")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("recycle fixture did not remove its file")
	}
}

func TestVisibilityRenameInAnotherPaneRevealsCachedHiddenEntry(t *testing.T) {
	root := t.TempDir()
	writeBatchFiles(t, root, map[string]string{".secret": "hidden", "stay.txt": "keep"})
	v := newBrowserView([]string{root, root}, nil)
	t.Cleanup(v.loader.close)
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	send, _ := panelInputFrames(v)
	send(modified("H", key.ModCtrl), modified("H", key.ModCtrl))
	selectName(t, &v.panels[0], "stay.txt")
	v.panels[0].toggleMark(v.panels[0].selected)
	send(pressed(key.NameTab))
	selectName(t, &v.panels[1], ".secret")
	send(pressed(key.NameF2))
	send(replaceRename(v, "revealed.txt"), pressed(key.NameReturn))
	waitRenamed(t, v)
	send()
	p := &v.panels[0]
	if !slices.Contains(visibleNames(p), "revealed.txt") || p.items[p.selected].name != "stay.txt" || !p.marks[p.selected] {
		t.Fatal("rename in another pane failed to update the filtered listing or retained selection/mark")
	}
	send(pressed(key.NameTab), modified("H", key.ModCtrl))
	if slices.Contains(visibleNames(p), ".secret") {
		t.Fatal("revealing all entries restored a stale name")
	}
}

func TestVisibilityTransferRefreshKeepsFilteredEntriesInCache(t *testing.T) {
	v, source, target := clipboardView(t)
	writeBatchFiles(t, source, map[string]string{".copied": "hidden"})
	v.load(0, source, "")
	waitLoaded(t, v, 0)
	selectName(t, &v.panels[0], ".copied")
	v.panels[1].visibility = visibilityHideHiddenDotfiles
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF5), pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	send()
	if slices.Contains(visibleNames(&v.panels[1]), ".copied") {
		t.Fatal("transfer refresh exposed an entry hidden by the destination's mode")
	}
	send(modified("H", key.ModCtrl))
	if !slices.Contains(visibleNames(&v.panels[1]), ".copied") {
		t.Fatal("visibility change lost an entry from the transfer refresh cache")
	}
	assertBatchFile(t, filepath.Join(target, ".copied"), "hidden")
}

func TestVisibilityPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless visibility preview")
	}
	v := newPreviewView()
	for i := range v.panels {
		p := &v.panels[i]
		p.ensureEntries()
		p.entries = append(p.entries,
			browser.Entry{Name: ".env", Bytes: 512, SizeKnown: true},
			browser.Entry{Name: "hidden.txt", Hidden: true, Bytes: 1024, SizeKnown: true},
			browser.Entry{Name: "$RECYCLE.BIN", Directory: true, Hidden: true, System: true})
		slices.SortFunc(p.entries, browser.CompareEntries)
		p.visibility = visibilityMode(i)
		p.refilter()
		v.showFeedback(i, feedback.Info, p.visibility.label())
		p.feedback.Frame(time.Now().Add(-time.Minute), v.appearance.Feedback)
	}
	if err := renderViewPNG(filepath.Join(directory, "visibility-modes.png"), v); err != nil {
		t.Fatal(err)
	}
}
