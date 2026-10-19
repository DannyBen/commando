package app

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func writePanelArchive(t *testing.T, filename string, names ...string) {
	t.Helper()
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, name := range names {
		member, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name[len(name)-1] != '/' {
			if _, err := member.Write([]byte(name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func archiveView(t *testing.T) (*view, string) {
	t.Helper()
	root := t.TempDir()
	filename := filepath.Join(root, "sample.zip")
	writePanelArchive(t, filename, "docs/sub/report.txt", "empty/", "a10.txt", "a2.txt")
	v := newBrowserView([]string{root, root}, nil)
	t.Cleanup(v.loader.close)
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	selectName(t, &v.panels[0], "sample.zip")
	v.activateSelected()
	waitLoaded(t, v, 0)
	if v.panels[0].archiveFolder != "." {
		t.Fatal("Enter did not browse the zip", v.panels[0].err)
	}
	return v, filename
}

func TestArchivePanelNavigationAndBrowsing(t *testing.T) {
	v, filename := archiveView(t)
	p := &v.panels[0]
	if p.path != filename || !slices.Equal(visibleNames(p), []string{"..", "docs", "empty", "a2.txt", "a10.txt"}) {
		t.Fatal("zip did not reuse the normal sorted file list", visibleNames(p))
	}
	if filename := os.Getenv("COMMANDO_PREVIEW_ARCHIVE"); filename != "" {
		if err := renderViewPNG(filename, v); err != nil {
			t.Fatal(err)
		}
	}
	send, _ := panelInputFrames(v)
	selectName(t, p, "a2.txt")
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertMarks(t, p, p.selected)
	send(pressed("/"), key.EditEvent{Text: "/"})
	send(key.EditEvent{Text: "a2"}, pressed(key.NameReturn))
	if !slices.Equal(visibleNames(p), []string{"..", "a2.txt"}) || !v.interaction.idle() {
		t.Fatal("archive filtering did not use ordinary panel behavior", visibleNames(p))
	}
	send(pressed(key.NameEscape))
	if p.archiveFolder != "." {
		t.Fatal("filtering discarded the archive boundary")
	}
	send(modified(key.NameF3, key.ModCtrl))
	if !p.sortByMetadata || p.archiveFolder != "." {
		t.Fatal("metadata sorting discarded the archive boundary")
	}
	selectName(t, p, "docs")
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if p.archiveFolder != "docs" || len(p.marks) != 0 {
		t.Fatal("archive folder navigation lost location or kept listing marks")
	}
	selectName(t, p, "sub")
	v.activateSelected()
	waitLoaded(t, v, 0)
	if p.archiveFolder != "docs/sub" || p.locationLabel() != filepath.Join(filename, "docs", "sub") {
		t.Fatal("archive subfolder location is incorrect")
	}
	v.parentFolder()
	waitLoaded(t, v, 0)
	if p.archiveFolder != "docs" || p.items[p.selected].name != "sub" {
		t.Fatal("archive Backspace lost the child selection")
	}
	selectName(t, p, "..")
	v.activateSelected()
	waitLoaded(t, v, 0)
	if p.archiveFolder != "." || p.items[p.selected].name != "docs" {
		t.Fatal("archive parent entry did not return to root")
	}
	v.parentFolder()
	waitLoaded(t, v, 0)
	if p.archiveFolder != "" || p.path != filepath.Dir(filename) || p.items[p.selected].name != "sample.zip" {
		t.Fatal("archive exit did not select the archive in its real folder")
	}
}

func TestArchiveBlocksFilesystemOnlyOperations(t *testing.T) {
	v, filename := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "a2.txt")
	v.shellExecutor = func(shellRequest) shellCompletion {
		t.Error("archive item reached the shell")
		return shellCompletion{}
	}
	v.openFile = func(string) error { t.Error("archive item reached external opening"); return nil }
	blocked := []string{"entry.open", "entry.rename", "entry.copy", "entry.cut", "entry.copy_paths", "entry.paste", "entry.paste_shortcut", "entry.duplicate", "entry.copy_to_panel", "entry.move_to_panel", "entry.copy_left", "entry.copy_right", "entry.recycle", "entry.delete_permanently", "entry.new_file", "entry.new_folder", "entry.context_menu", "entry.properties", "entry.arm_drag", "file.edit", "programs.terminal_here", "programs.terminal_here_admin"}
	for _, c := range v.commandCatalogue() {
		if slices.Contains(blocked, c.id) && c.allowed(v) {
			t.Errorf("unsupported archive command is available: %s", c.id)
		}
	}
	// Pointer/direct routes must be protected too, independently of the catalogue.
	v.activateSelected()
	v.startName(layout.Context{}, nameRename)
	v.startName(layout.Context{}, nameCreateFolder)
	v.removeSelected(layout.Context{}, false)
	v.removeSelected(layout.Context{}, true)
	v.clipboardSelected(false)
	v.pasteFiles()
	v.duplicateSelected()
	v.startPanelTransfer(false)
	v.startContextMenu(layout.Context{}, 0, p.selected, true)
	v.startDrag(layout.Context{}, 0, p.selected, false)
	if p.loading || !v.interaction.idle() || len(v.loader.requests) != 0 || len(p.selectedPaths()) != 0 || v.fileOperationLocations()[0] != "" {
		t.Fatal("an unsupported operation queued work or opened an interaction")
	}
	if v.panelTransferError(0) == "" {
		t.Fatal("archive accepted a filesystem transfer destination")
	}
	if info, err := os.Stat(filename); err != nil || info.IsDir() {
		t.Fatal("archive container changed", err)
	}
}

func TestArchiveRefreshRestoreCloneAndAdjacentNavigation(t *testing.T) {
	v, filename := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "docs")
	v.openAdjacent(1)
	waitLoaded(t, v, 1)
	if v.panels[1].archiveFolder != "docs" || p.archiveFolder != "." {
		t.Fatal("adjacent navigation did not preserve the source archive")
	}
	// Alt+D accepts the displayed archive path without treating it as a host folder.
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 1)
	if v.panels[1].archiveFolder != "docs" || !v.interaction.idle() {
		t.Fatal("location submission lost the archive boundary")
	}
	v.addPanel(1)
	if v.panels[v.active].archiveFolder != "docs" {
		t.Fatal("archive clone lost its location")
	}
	v.closePanel()
	v.reopenPanel()
	if v.panels[v.active].archiveFolder != "docs" {
		t.Fatal("archive close/reopen lost its location")
	}
	state := v.sessionState(windowState{})
	dir := t.TempDir()
	if err := saveState(dir, state); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	locations, err := startingLocations(nil, saved.Panels)
	if err != nil {
		t.Fatal(err)
	}
	restored := newLocationView(locations, state.ActivePanel, nil)
	defer restored.loader.close()
	for i := range restored.panels {
		waitLoaded(t, restored, i)
		if restored.panels[i].archiveFolder != state.Panels[i].ArchiveFolder {
			t.Fatal("restored panel lost its archive location")
		}
	}
	v.active = 0
	// Reacquire the model after insertion can reallocate the panel slice.
	p = &v.panels[0]
	selectName(t, p, "a2.txt")
	p.toggleMark(p.selected)
	writePanelArchive(t, filename, "docs/sub/report.txt", "a2.txt", "new.txt")
	p.refreshNeeded = true
	v.updateAutoRefresh(layout.Context{})
	result := <-v.loader.results
	v.acceptLoad(result)
	if !slices.Contains(visibleNames(p), "new.txt") || !p.marks[p.selected] || p.items[p.selected].name != "a2.txt" {
		t.Fatal("quiet archive refresh lost marks or current item", visibleNames(p))
	}
	v.refreshPanel()
	waitLoaded(t, v, 0)
	if len(p.marks) != 0 || p.items[p.selected].name != "a2.txt" {
		t.Fatal("manual archive refresh did not preserve selection and clear marks")
	}
	if err := os.WriteFile(filename, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := slices.Clone(p.items)
	v.refreshPanel()
	waitLoaded(t, v, 0)
	if p.err == nil || len(p.items) != len(previous) || p.archiveFolder != "." || readError(p.err) != "Invalid archive" {
		t.Fatal("failed archive refresh discarded its listing or location")
	}
}

func TestArchiveQuietRefreshKeepsCaseSensitiveIdentity(t *testing.T) {
	v, filename := archiveView(t)
	writePanelArchive(t, filename, "A.txt", "a.txt", "docs/file.txt")
	v.refreshPanel()
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	selectName(t, p, "A.txt")
	p.toggleMark(p.selected)
	directory, err := browser.ReadArchive(p.location())
	if err != nil {
		t.Fatal(err)
	}
	p.reconcileDirectoryListing(directory)
	if p.items[p.selected].name != "A.txt" || len(p.marks) != 1 || !p.marks[p.selected] {
		t.Fatal("quiet refresh merged case-distinct archive entries")
	}
	previous := slices.Clone(p.items)
	// An arriving refresh for a different folder in the same archive is stale.
	v.acceptAutoRefresh(loadResult{request: loadRequest{panel: 0, path: filename, archiveFolder: "docs", autoRefresh: true}, directory: browser.Directory{Path: filename, ArchiveFolder: "docs"}})
	if !slices.Equal(p.items, previous) || p.archiveFolder != "." {
		t.Fatal("stale archive refresh replaced the current folder")
	}
}

func TestFailedArchiveRestoreKeepsVirtualLocation(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "broken.zip")
	if err := os.WriteFile(filename, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newLocationView([]panelState{{Path: filename, ArchiveFolder: "docs"}}, 0, nil)
	defer v.loader.close()
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	if p.err == nil || p.archiveFolder != "docs" || p.nativeFolder() || writableFolder(v) {
		t.Fatal("failed restore treated the archive file as a real folder")
	}
	v.parentFolder()
	waitLoaded(t, v, 0)
	if p.archiveFolder != "docs" || p.err == nil {
		t.Fatal("failed parent read changed the archive location")
	}
	// An explicit real-folder navigation remains available for recovery.
	v.load(0, filepath.Dir(filename), "broken.zip")
	waitLoaded(t, v, 0)
	if p.archiveFolder != "" || p.err != nil {
		t.Fatal("could not leave a broken restored archive")
	}
}
