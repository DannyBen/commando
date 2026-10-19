package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func fixtureRemoveArchiveSources(receipt *browser.ArchiveSources, _ uintptr) ([]string, error) {
	var removed []string
	for _, source := range receipt.Paths() {
		if err := os.RemoveAll(source); err != nil {
			return removed, err
		}
		removed = append(removed, source)
	}
	return removed, nil
}

func TestArchiveMovesOutConfirmWholeRootsAndFollowPanels(t *testing.T) {
	for _, partial := range []bool{false, true} {
		v, filename := archiveView(t)
		if !v.addPanel(1) {
			t.Fatal("clone failed")
		}
		v.active = 0
		v.loadLocation(1, browser.Location{Path: filename, Folder: "docs/sub"}, "report.txt")
		waitLoaded(t, v, 1)
		v.panels[1].toggleMark(v.panels[1].selected)
		destination := resolvedTempDir(t)
		v.load(2, destination, "")
		waitLoaded(t, v, 2)
		for _, name := range []string{"docs", "a2.txt"} {
			selectName(t, &v.panels[0], name)
			v.panels[0].toggleMark(v.panels[0].selected)
		}
		selectName(t, &v.panels[0], "a10.txt")
		v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
			if !request.Move || !request.TemporarySources {
				t.Fatal("staging sources were not moved by shell")
			}
			if partial {
				request.Paths = request.Paths[:1]
			}
			result, err := fixtureTransfer(request)
			if partial {
				result.Aborted = true
				result.Summary.Requested = 2
				result.Summary.Unfinished = 1
			}
			return result, err
		}
		forbidTransferClipboard(t, v)
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF6))
		// The first destination is an archive subfolder; carry the inline action
		// to the normal folder without changing the captured marked selection.
		send(pressed(key.NameRightArrow))
		send(pressed(key.NameReturn))
		waitLoaded(t, v, 2)
		send()
		directory, err := browser.ReadArchive(browser.Location{Path: filename, Folder: "."})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range directory.Entries {
			names = append(names, entry.Name)
		}
		if slices.Contains(names, "docs") || slices.Contains(names, "a2.txt") != partial {
			t.Fatalf("wrong confirmed archive removals partial=%v names=%v active=%d inline=%v feedback=%+v", partial, names, v.active, v.inlineInteraction() != nil, v.panels[2].feedback.Current())
		}
		assertBatchFile(t, filepath.Join(destination, "docs", "sub", "report.txt"), "docs/sub/report.txt")
		if v.panels[1].archiveFolder != "" || !samePath(v.panels[1].path, filepath.Join(destination, "docs", "sub")) || v.panels[1].items[v.panels[1].selected].name != "report.txt" || !v.panels[1].marks[v.panels[1].selected] {
			t.Fatal("open archive descendant did not follow confirmed move", v.panels[1].location())
		}
		if v.transferPending || v.mutationsBusy() {
			t.Fatal("move did not release worker")
		}
	}
}

func TestMoveIntoArchiveAndCutPasteFollowNativeDescendants(t *testing.T) {
	for _, cut := range []bool{false, true} {
		v, filename := archiveView(t)
		canonicalArchive, err := resolveFileLocation(filename)
		if err != nil {
			t.Fatal(err)
		}
		c := attachTestFileClipboard(v)
		source := resolvedTempDir(t)
		writeBatchFiles(t, source, map[string]string{"folder/sub/file.txt": "data", "keep.txt": "keep"})
		v.load(1, source, "folder")
		waitLoaded(t, v, 1)
		v.active = 1
		if !v.addPanel(1) {
			t.Fatal("clone failed")
		}
		v.load(2, filepath.Join(source, "folder", "sub"), "file.txt")
		waitLoaded(t, v, 2)
		v.panels[2].toggleMark(v.panels[2].selected)
		v.active = 1
		v.loader.removeArchiveSources = fixtureRemoveArchiveSources
		send, _ := panelInputFrames(v)
		if cut {
			send(modified("X", key.ModCtrl))
			waitLoaded(t, v, 1)
			send()
			v.active = 0
			v.pasteFiles()
		} else {
			v.transferToNeighbor(-1, true)
		}
		resultPanel := 1
		if cut {
			resultPanel = 0
		}
		waitLoaded(t, v, resultPanel)
		send()
		if archiveMemberContents(t, filename, "folder/sub/file.txt") != "data" {
			t.Fatal("move did not commit ZIP contents")
		}
		if _, err := os.Stat(filepath.Join(source, "folder")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("confirmed source not removed", err)
		}
		if v.panels[2].archiveFolder != "folder/sub" || !samePath(v.panels[2].path, canonicalArchive) || v.panels[2].items[v.panels[2].selected].name != "file.txt" || !v.panels[2].marks[v.panels[2].selected] {
			t.Fatal("native descendant did not follow into ZIP", v.panels[2].location())
		}
		if cut {
			if _, err := c.read(); !errors.Is(err, browser.ErrNoClipboardFiles) {
				t.Fatal("completed native cut remained on clipboard", err)
			}
		}
	}
}

func TestArchiveCutCompletionRetainsAliasedPanelPaths(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[0]
	p.path, p.archiveFolder = filepath.Join(t.TempDir(), "alias.zip"), "."
	p.clearMarks()
	p.selected = 1
	canonical := filepath.Join(t.TempDir(), "physical.zip")
	clipboard := browser.FileClipboard{Move: true, Archive: &browser.ArchiveClipboard{
		Move: true, Location: browser.Location{Path: canonical, Folder: "."}, Names: p.selectedNames(),
	}}
	v.acceptFileOperation(loadResult{
		request: loadRequest{panel: 0, transfer: &transferRequest{
			locations: []string{""}, archiveLocations: []browser.Location{p.location()},
		}},
		archiveReconciled: true, locations: []string{canonical}, clipboard: clipboard,
	})
	if !v.cutEntry(0, p.selected) {
		t.Fatal("partial archive Cut lost its captured alias mapping")
	}
}

func TestArchiveCutPartialCompletionRetainsReusableSelection(t *testing.T) {
	v, filename := archiveView(t)
	c := attachTestFileClipboard(v)
	for _, name := range []string{"docs", "a2.txt"} {
		selectName(t, &v.panels[0], name)
		v.panels[0].toggleMark(v.panels[0].selected)
	}
	destination := resolvedTempDir(t)
	v.load(1, destination, "")
	waitLoaded(t, v, 1)
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		request.Paths = request.Paths[:1]
		result, err := fixtureTransfer(request)
		result.Aborted = true
		result.Summary.Requested = 2
		result.Summary.Unfinished = 1
		return result, err
	}
	send, _ := panelInputFrames(v)
	send(modified("X", key.ModCtrl))
	waitLoaded(t, v, 0)
	send()
	captured, err := c.read()
	if err != nil || !captured.Move || captured.Archive == nil || len(captured.Paths) != 0 || !v.cutEntry(0, v.panels[0].selected) {
		t.Fatal("archive Cut did not capture/highlight references", captured, err)
	}
	send(pressed(key.NameTab), modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	remaining, err := c.read()
	if err != nil || !remaining.Move || remaining.Archive == nil || !slices.Equal(remaining.Archive.Names, []string{"a2.txt"}) {
		t.Fatal("partial cut did not retain surviving reference", remaining, err)
	}
	selectName(t, &v.panels[0], "a2.txt")
	if !v.cutEntry(0, v.panels[0].selected) {
		t.Fatal("remaining archive Cut lost highlighting")
	}
	v.loader.transfer = fixtureTransfer
	v.pasteFiles()
	waitLoaded(t, v, 1)
	send()
	if _, err := c.read(); !errors.Is(err, browser.ErrNoClipboardFiles) {
		t.Fatal("completed archive cut remained", err)
	}
	directory, err := browser.ReadArchive(browser.Location{Path: filename, Folder: "."})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range directory.Entries {
		if entry.Name == "a2.txt" || entry.Name == "docs" {
			t.Fatal("moved archive source survived", entry.Name)
		}
	}
}

func TestMoveWithinAndBetweenZIPsAndRetainedSources(t *testing.T) {
	for _, same := range []bool{false, true} {
		v, filename := archiveView(t)
		target := filename
		if !same {
			target = filepath.Join(t.TempDir(), "target.zip")
			writePanelArchive(t, target, "empty/")
		}
		v.loadLocation(1, browser.Location{Path: target, Folder: "empty"}, "")
		waitLoaded(t, v, 1)
		selectName(t, &v.panels[0], "a2.txt")
		forbidTransferClipboard(t, v)
		v.transferToNeighbor(1, true)
		waitLoaded(t, v, 0)
		if archiveMemberContents(t, target, "empty/a2.txt") != "a2.txt" {
			t.Fatal("archive move missing destination")
		}
		if slices.Contains(visibleNames(&v.panels[0]), "a2.txt") {
			t.Fatal("archive source not removed")
		}
	}
	v, filename := archiveView(t)
	source := resolvedTempDir(t)
	writeBatchFiles(t, source, map[string]string{"new.txt": "data"})
	v.load(1, source, "new.txt")
	waitLoaded(t, v, 1)
	v.active = 1
	v.loader.removeArchiveSources = func(*browser.ArchiveSources, uintptr) ([]string, error) { return nil, browser.ErrArchiveSourceChanged }
	v.transferToNeighbor(-1, true)
	waitLoaded(t, v, 1)
	if archiveMemberContents(t, filename, "new.txt") != "data" {
		t.Fatal("source-removal failure rolled back destination")
	}
	assertBatchFile(t, filepath.Join(source, "new.txt"), "data")
	if message := v.panels[1].feedback.Current(); message == nil || message.Text != "Copied; Source changed; retry" {
		t.Fatal("source retention misreported", message)
	}
}

func TestArchiveMoveRetainsPartialFoldersAndChangedSources(t *testing.T) {
	for _, changed := range []bool{false, true} {
		v, filename := archiveView(t)
		selectName(t, &v.panels[0], "docs")
		destination := resolvedTempDir(t)
		v.load(1, destination, "")
		waitLoaded(t, v, 1)
		v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
			if changed {
				result, err := fixtureTransfer(request)
				// A concurrent archive update must veto removal after native success.
				writePanelArchive(t, filename, "docs/sub/report.txt", "new.txt")
				return result, err
			}
			target := filepath.Join(destination, "docs", "sub")
			if err := os.MkdirAll(target, 0700); err != nil {
				return browser.TransferResult{}, err
			}
			if err := os.Rename(filepath.Join(request.Paths[0], "sub", "report.txt"), filepath.Join(target, "report.txt")); err != nil {
				return browser.TransferResult{}, err
			}
			return browser.TransferResult{Directory: destination, Attempted: true, Aborted: true,
				Items:   []browser.TransferredItem{{Source: request.Paths[0], Entry: browser.Entry{Name: "docs", Directory: true}}},
				Summary: browser.TransferSummary{Requested: 1, Partial: 1}}, nil
		}
		v.transferToNeighbor(1, true)
		waitLoaded(t, v, 0)
		if archiveMemberContents(t, filename, "docs/sub/report.txt") != "docs/sub/report.txt" {
			t.Fatal("unfinished move removed archive contents")
		}
		assertBatchFile(t, filepath.Join(destination, "docs", "sub", "report.txt"), "docs/sub/report.txt")
		if changed {
			if message := v.panels[0].feedback.Current(); message == nil || message.Text != "Copied; Archive changed; copy again" {
				t.Fatal("changed-source retention not reported", message)
			}
		}
	}
}

func TestArchivePartialMoveFeedbackKeepsSpecificReason(t *testing.T) {
	v := newPreviewView()
	v.showTransferResult(loadResult{request: loadRequest{panel: 0}, transferMove: true, retainedCopies: 1,
		transferred: browser.TransferResult{Summary: browser.TransferSummary{Completed: 1}}, err: browser.ErrArchiveSourceChanged}, false)
	if message := v.panels[0].feedback.Current(); message == nil || message.Text != "1 transferred; Source changed; retry" {
		t.Fatal("partial move lost count/reason", message)
	}
}
