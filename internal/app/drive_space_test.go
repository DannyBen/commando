package app

import (
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func driveSpaceHome() browser.Directory {
	return browser.Directory{Drives: true, Entries: []browser.Entry{
		{Name: "Favorite.lnk", Path: "favorite", Directory: true, Favorite: true},
		{Name: `C:\`, Path: `C:\`, Directory: true},
		{Name: `N:\`, Path: `N:\`, Directory: true},
	}}
}

func TestDriveSpaceHomeAppearsBeforeQueryAndRefreshesExplicitly(t *testing.T) {
	v := newView([]panel{{path: "folder"}})
	home := driveSpaceHome()
	started, release := make(chan string, 4), make(chan struct{})
	calls := 0
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		if path == "" {
			return home, nil
		}
		return browser.Directory{Path: path}, nil
	}, nil)
	v.loader.readDriveSpace = func(root string) (browser.DriveSpace, bool) {
		started <- root
		<-release
		calls++
		return browser.DriveSpace{Free: int64(calls) * 1024, Total: 10 * 1024}, root == `C:\`
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		v.finishOperations()
	})
	v.showDrives()
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	if !p.drives || len(p.items) != 3 || p.loading || !v.interaction.idle() {
		t.Fatal("Home was held behind space metadata")
	}
	select {
	case <-started:
		t.Fatal("query ran before Home layout")
	default:
	}
	if got := p.displayEntry(p.items[1]).size; got != "" {
		t.Fatal("pending space fabricated")
	}
	send, _ := panelInputFrames(v)
	select {
	case root := <-started:
		if root != `C:\` {
			t.Fatalf("queried favorite: %q", root)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("query not started")
	}
	if p.loading || v.mutationsBusy() {
		t.Fatal("metadata claimed a directory load or mutation")
	}
	close(release)
	for range 2 {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-time.After(5 * time.Second):
			t.Fatal("metadata did not arrive")
		}
	}
	if got := p.displayEntry(p.items[1]).size; got != "1.0 KB / 10.0 KB" {
		t.Fatalf("row=%q", got)
	}
	if got := p.displayEntry(p.items[2]).size; got != "" {
		t.Fatal("unsupported drive had metadata")
	}
	p.selected = 1
	p.metadata = metadataExtension
	if got, _ := p.status(); got != "9.0 KB used" {
		t.Fatalf("footer=%q", got)
	}
	v.showFeedback(0, feedback.Error, "Access denied")
	if got, _ := p.status(); got != "Access denied" {
		t.Fatal("drive space covered notification")
	}
	p.feedback.Dismiss()
	p.selected = 0
	if got, _ := p.status(); got != "" {
		t.Fatal("favorite had used space")
	}
	// Automatic favorite reconciliation preserves the snapshot and performs no query.
	token := p.driveSpace
	v.acceptAutoRefresh(loadResult{request: loadRequest{homeDirectory: "favorites"}, directory: home})
	if p.driveSpace != token || len(p.driveSpaces) != 1 {
		t.Fatal("automatic Home refresh reset snapshot")
	}
	select {
	case root := <-started:
		if root != `N:\` {
			t.Fatal(root)
		}
	default:
	}
	// A normal Refresh creates a new snapshot without changing the folder column preference.
	send(modified("R", key.ModCtrl))
	waitLoaded(t, v, 0)
	if p.driveSpace == token || len(p.driveSpaces) != 0 || p.metadata != metadataExtension {
		t.Fatal("explicit Refresh retained snapshot or changed column preference")
	}
	v.submitDriveSpaces()
	for range 2 {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-time.After(5 * time.Second):
			t.Fatal("refresh did not query")
		}
	}
	if got := p.displayEntry(p.items[1]).size; got != "3.0 KB / 10.0 KB" {
		t.Fatalf("refresh reused snapshot=%q", got)
	}
	// Entering Home again also gets a fresh snapshot, and ordinary folders never show it.
	v.acceptLoad(loadResult{directory: browser.Directory{Path: "other"}})
	if p.driveSpace != nil || len(p.driveSpaces) != 0 {
		t.Fatal("space leaked into folder view")
	}
	v.acceptLoad(loadResult{directory: home})
	if p.driveSpace == nil || p.driveSpace == token {
		t.Fatal("Home re-entry reused request")
	}
}

func TestDriveSpaceLateResultsFollowPanelAndIgnoreRefreshOrClone(t *testing.T) {
	v := selectionView()
	home := driveSpaceHome()
	v.acceptLoad(loadResult{directory: home})
	original := v.panels[0].driveSpace
	result := loadResult{request: loadRequest{driveSpace: original, path: `C:\`}, driveSpace: browser.DriveSpace{Free: 5, Total: 20}, driveSpaceKnown: true}
	v.insertPanel(0, v.panels[0].clone(), "", "")
	v.acceptLoad(result)
	if len(v.panels[0].driveSpaces) != 0 || v.panels[1].driveSpaces[`C:\`].Total != 20 {
		t.Fatal("result leaked into clone or lost moved owner")
	}
	// Refresh replaces ownership; results captured before it cannot overwrite the new snapshot.
	v.acceptLoad(loadResult{request: loadRequest{panel: 1}, directory: home})
	v.acceptLoad(result)
	if len(v.panels[1].driveSpaces) != 0 {
		t.Fatal("old result overwrote refreshed Home")
	}
	v.acceptLoad(loadResult{request: loadRequest{panel: 1}, directory: browser.Directory{Path: "elsewhere"}})
	v.acceptLoad(result)
	if len(v.panels[1].driveSpaces) != 0 {
		t.Fatal("late result changed a folder")
	}
	v.active = 1
	v.closePanel()
	before := slices.Clone(v.panels[0].items)
	v.acceptLoad(result)
	if !slices.EqualFunc(before, v.panels[0].items, func(a, b entry) bool { return a.name == b.name }) {
		t.Fatal("closed owner affected surviving panel")
	}
}

func TestDriveSpaceCloneKeepsIndependentSnapshot(t *testing.T) {
	p := panel{driveSpaces: map[string]browser.DriveSpace{`C:\`: {Free: 10, Total: 20}}, driveSpace: &driveSpaceRequest{submitted: true}}
	clone := p.clone()
	clone.driveSpaces[`C:\`] = browser.DriveSpace{Free: 2, Total: 20}
	if clone.driveSpace != nil || p.driveSpaces[`C:\`].Free != 10 {
		t.Fatal("clone shared pending ownership or mutable snapshot")
	}
}
