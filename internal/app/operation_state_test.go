package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestDispatchClassificationBlocksNewWritesAndAllowsBrowsing(t *testing.T) {
	l := &directoryLoader{requests: make(chan loadRequest, 3)}
	if !l.submit(loadRequest{}) {
		t.Fatal("default mutation refused while idle")
	}
	first := <-l.requests
	if l.submit(loadRequest{}) || l.submit(loadRequest{action: actionClipboard}) {
		t.Fatal("busy dispatcher accepted another write")
	}
	if !l.submit(loadRequest{action: actionBrowse}) {
		t.Fatal("busy dispatcher blocked a directory read")
	}
	<-l.requests
	l.complete(first)
	if !l.submit(loadRequest{action: actionClipboard}) {
		t.Fatal("reconciled operation kept clipboard blocked")
	}
}

func TestNativeTransferKeepsMutationGateAndCloseBlockedUntilReconciled(t *testing.T) {
	v, source, _ := clipboardView(t)
	c := attachTestFileClipboard(v)
	_, _ = c.write([]string{filepath.Join(source, "a.txt")}, true)
	entered, release := make(chan browser.TransferRequest, 1), make(chan struct{})
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		entered <- request
		<-release
		return fixtureTransfer(request)
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	v.ownerWindow, v.active = 987, 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	select {
	case request := <-entered:
		if request.Owner != 987 || !request.Move {
			t.Fatal("owner or operation lost at dispatch")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("native dispatch did not start")
	}
	if v.requestClose() || !v.actionAllowed(actionBrowse) || v.actionAllowed(actionMutation) || v.actionAllowed(actionClipboard) {
		t.Fatal("busy policy or close guard incorrect")
	}
	send(pressed(key.NameRightArrow), pressed(key.NameDownArrow))
	selected := v.panels[2].selected
	send(modified("C", key.ModCtrl), modified("X", key.ModCtrl), modified("V", key.ModCtrl), modified("D", key.ModCtrl), pressed(key.NameF2), modified("N", key.ModCtrl), pressed(key.NameDeleteForward))
	if len(v.loader.requests) != 0 || v.nameInteraction() != nil || v.active != 2 || v.panels[2].selected != selected {
		t.Fatal("busy operation allowed writes or blocked browsing")
	}
	close(release)
	var result loadResult
	select {
	case result = <-v.loader.results:
	case <-time.After(3 * time.Second):
		t.Fatal("native completion missing")
	}
	if !v.mutationsBusy() {
		t.Fatal("gate opened before cached state was reconciled")
	}
	v.acceptLoad(result)
	if v.mutationsBusy() || !v.requestClose() {
		t.Fatal("completion did not release busy/close state")
	}
	if _, err := c.read(); err != browser.ErrNoClipboardFiles {
		t.Fatal("confirmed cut not reconciled")
	}
}

func TestShutdownWaitsForActiveOperationAndAcceptsItsResults(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	_, _ = c.write([]string{filepath.Join(source, "folder")}, true)
	entered, release := make(chan struct{}), make(chan struct{})
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		close(entered)
		<-release
		return fixtureTransfer(request)
	}
	v.load(2, filepath.Join(source, "folder", "nested"), "")
	waitLoaded(t, v, 2)
	v.active = 1
	v.pasteFiles()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not start")
	}
	done := make(chan struct{})
	go func() { v.finishOperations(); close(done) }()
	// Wait for the stop request instead of relying on scheduling or a sleep.
	<-v.loader.done
	select {
	case <-done:
		t.Fatal("shutdown abandoned the active operation")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not drain operation results")
	}
	if v.panels[2].path != filepath.Join(target, "folder", "nested") || v.mutationsBusy() {
		t.Fatal("shutdown lost moved-panel or busy reconciliation")
	}
	if _, err := os.Stat(filepath.Join(target, "folder", "nested", "file")); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledShellBatchKeepsPartialStateAndSurvivingMarks(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	markClipboardItems(t, &v.panels[0])
	_, _ = c.write([]string{filepath.Join(source, "folder"), filepath.Join(source, "a.txt")}, true)
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		request.Paths = request.Paths[:1]
		result, err := fixtureTransfer(request)
		result.Aborted = true
		// Windows left an additional partial destination with no completed
		// callback. Commando must display it and accept it without cleaning up.
		if err == nil {
			err = os.WriteFile(filepath.Join(target, "partial.tmp"), []byte("partial"), 0600)
		}
		return result, err
	}
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	selectName(t, &v.panels[1], "partial.tmp")
	selectName(t, &v.panels[0], "a.txt")
	if !v.panels[0].marks[v.panels[0].selected] {
		t.Fatal("cancel lost surviving source mark")
	}
	clipboard, err := c.read()
	if err != nil || len(clipboard.Paths) != 1 || clipboard.Paths[0] != filepath.Join(source, "a.txt") {
		t.Fatal("unfinished cut consumed")
	}
	assertBatchFile(t, filepath.Join(target, "partial.tmp"), "partial")
}

func TestCancelledMergeFollowsConfirmedMovedChildAndRetainsParentCut(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	folder := filepath.Join(source, "folder")
	if err := os.WriteFile(filepath.Join(folder, "remaining"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	v.load(2, filepath.Join(folder, "nested"), "")
	waitLoaded(t, v, 2)
	selectName(t, &v.panels[2], "file")
	v.panels[2].toggleMark(v.panels[2].selected)
	_, _ = c.write([]string{folder}, true)
	v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
		root := filepath.Join(target, "folder")
		child := filepath.Join(root, "actual-child-name")
		if err := os.Mkdir(root, 0700); err != nil {
			return browser.TransferResult{}, err
		}
		if err := os.Rename(filepath.Join(folder, "nested"), child); err != nil {
			return browser.TransferResult{}, err
		}
		return browser.TransferResult{Directory: target, Attempted: true, Aborted: true,
			Summary: browser.TransferSummary{Requested: 1, Partial: 1},
			Items:   []browser.TransferredItem{{Source: folder, Entry: browser.Entry{Name: "folder", Directory: true}}},
			Changes: []browser.TransferredPath{{Source: filepath.Join(folder, "nested"), Destination: child, Directory: true, Moved: true}},
		}, nil
	}
	v.active = 1
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl))
	waitLoaded(t, v, 1)
	send()
	p := &v.panels[2]
	if p.path != filepath.Join(target, "folder", "actual-child-name") || p.items[p.selected].name != "file" || !p.marks[p.selected] || v.active != 1 {
		t.Fatal("cancelled merge lost confirmed descendant, selection, mark or focus")
	}
	clipboard, err := c.read()
	if err != nil || len(clipboard.Paths) != 1 || clipboard.Paths[0] != folder {
		t.Fatal("partly moved parent consumed from cut")
	}
	assertBatchFile(t, filepath.Join(folder, "remaining"), "keep")
}
