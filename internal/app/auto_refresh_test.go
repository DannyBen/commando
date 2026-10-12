package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

// An event source substitutes for Windows; all reads still use the real worker.
func automaticRefreshView(t *testing.T) (*view, string, *directoryWatch) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{root, root}, nil)
	t.Cleanup(v.finishOperations)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	v.monitor = newDirectoryMonitor(func(ctx context.Context, _ string, _ func()) error {
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	v.monitor.sync(v.fileOperationLocations())
	return v, root, v.monitor.watches[directoryWatchKey(root)]
}

func runAutomaticRefresh(v *view, now time.Time) {
	var ops op.Ops
	v.updateAutoRefresh(layout.Context{Ops: &ops, Now: now})
}

func finishAutomaticRefresh(t *testing.T, v *view) {
	t.Helper()
	pending := 0
	for _, p := range v.panels {
		if p.refreshing {
			pending++
		}
	}
	for range pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-time.After(3 * time.Second):
			t.Fatal("automatic read did not finish")
		}
	}
}

func TestAutomaticRefreshCoalescesAndPreservesPanelState(t *testing.T) {
	v, root, watch := automaticRefreshView(t)
	p := &v.panels[0]
	selectName(t, p, "b.txt")
	p.toggleMark(p.selected)
	selectName(t, p, "d.txt")
	p.toggleMark(p.selected)
	selectName(t, p, "b.txt")
	p.list.Position.First, p.list.Position.Offset = p.selected, 7
	p.prefix, p.prefixAt = "b", time.Now()
	p.feedback.Show(feedback.Success, "Copied")
	message := p.feedback.Current()
	v.active = 1
	if err := os.WriteFile(filepath.Join(root, "aa.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "d.txt")); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		v.monitor.changed(watch)
	}
	runAutomaticRefresh(v, time.Now())
	if p.refreshing || p.loading {
		t.Fatal("notification burst was not coalesced")
	}
	runAutomaticRefresh(v, time.Now().Add(directoryChangeDelay))
	if !p.refreshing || p.loading || !v.panels[1].refreshing {
		t.Fatal("shared directory did not refresh both panes independently of loading")
	}
	// Selection changed after dispatch must survive, rather than reverting to b.
	selectName(t, p, "c.txt")
	finishAutomaticRefresh(t, v)
	if p.items[p.selected].file.Name != "c.txt" || v.active != 1 || p.prefix != "b" || p.feedback.Current() != message {
		t.Fatal("automatic refresh disturbed current selection, focus, prefix or feedback")
	}
	if p.items[p.list.Position.First].file.Name != "b.txt" || p.list.Position.Offset != 7 || p.items[p.list.Position.First].file.Bytes != 7 {
		t.Fatal("automatic refresh lost the viewport anchor")
	}
	marked := p.selectedPaths()
	if len(marked) != 1 || filepath.Base(marked[0]) != "b.txt" {
		t.Fatal("automatic refresh lost surviving marks")
	}
	runAutomaticRefresh(v, time.Now().Add(time.Second))
	if p.refreshing || v.panels[1].refreshing {
		t.Fatal("one notification burst scheduled repeated reads")
	}
}

func TestAutomaticRefreshDefersDispatchAndArrivingResults(t *testing.T) {
	for _, blocked := range []string{"editor", "choice", "mutation"} {
		t.Run(blocked, func(t *testing.T) {
			v, _, watch := automaticRefreshView(t)
			p := &v.panels[0]
			v.monitor.changed(watch)
			var ops op.Ops
			gtx := layout.Context{Ops: &ops}
			block := func() {
				switch blocked {
				case "editor":
					v.startLocation(gtx)
				case "choice":
					v.startInlineActions(0, 2, "", "", []string{"Copy here"}, nil, nil)
				case "mutation":
					v.loader.busy.Store(1)
				}
			}
			unblock := func() {
				v.cancelInteraction(gtx)
				v.loader.busy.Store(0)
			}
			block()
			runAutomaticRefresh(v, time.Now().Add(directoryChangeDelay))
			if p.refreshing || !p.refreshNeeded {
				t.Fatal("protected listing was refreshed or lost its pending change")
			}
			unblock()
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			if !p.refreshing {
				t.Fatal("deferred refresh did not resume")
			}
			block()
			before := append([]entry(nil), p.items...)
			finishAutomaticRefresh(t, v)
			if p.refreshing || !p.refreshNeeded || !reflect.DeepEqual(p.items, before) {
				t.Fatal("arriving read disturbed a newly protected listing")
			}
			unblock()
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			finishAutomaticRefresh(t, v)
			if p.refreshNeeded || p.refreshing {
				t.Fatal("refresh did not finish after protection ended")
			}
		})
	}
}

func TestAutomaticRefreshDropsStaleReadsAndRetainsFailures(t *testing.T) {
	v, root, _ := automaticRefreshView(t)
	p := &v.panels[0]
	selectName(t, p, "b.txt")
	p.toggleMark(p.selected)
	before, position, selected := append([]entry(nil), p.items...), p.list.Position, p.selected
	for _, stale := range []bool{false, true} {
		path := root
		var err error = errors.New("cannot read")
		if stale {
			path = filepath.Join(root, "previous-location")
			err = nil
		}
		p.refreshing = true
		v.acceptLoad(loadResult{request: loadRequest{panel: 0, path: path, autoRefresh: true}, err: err,
			directory: browser.Directory{Path: path, Entries: []browser.Entry{{Name: "stale.txt"}}}})
		if p.refreshing || p.selected != selected || p.list.Position != position || !reflect.DeepEqual(p.items, before) || !p.marks[selected] {
			t.Fatal("stale or failed automatic read changed the cached panel")
		}
	}
}

func TestRepeatedAutomaticReadErrorDoesNotRestartItsMessage(t *testing.T) {
	v := newPreviewView()
	p := &v.panels[1]
	start := time.Unix(1000, 0)
	failed := func(err error) {
		v.acceptAutoRefresh(loadResult{request: loadRequest{panel: 1, path: p.path, autoRefresh: true}, err: err})
	}
	failed(&os.PathError{Op: "read", Path: p.path, Err: os.ErrNotExist})
	message := p.feedback.Current()
	p.feedback.Frame(start, v.appearance.Feedback)
	before, _ := p.feedback.Frame(start.Add(350*time.Millisecond), v.appearance.Feedback)
	failed(&os.PathError{Op: "read", Path: p.path, Err: os.ErrNotExist})
	after, _ := p.feedback.Frame(start.Add(350*time.Millisecond), v.appearance.Feedback)
	if p.feedback.Current() != message || before.Text != after.Text || after.Typed || after.Finished {
		t.Fatal("repeated automatic failure restarted typing or audio")
	}
	failed(os.ErrPermission)
	if p.feedback.Current() == message || p.feedback.Current().Text != "Access denied" {
		t.Fatal("different automatic failure was suppressed")
	}
	message = p.feedback.Current()
	v.acceptAutoRefresh(loadResult{request: loadRequest{panel: 1, path: p.path, autoRefresh: true}, directory: browser.Directory{Path: p.path}})
	failed(os.ErrPermission)
	if p.feedback.Current() == message {
		t.Fatal("a new failure after recovery was suppressed")
	}
}

func TestRecycledFolderOpenInAdjacentPanelDoesNotRepeatFeedback(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "kept.txt"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root, child, root}, nil)
	t.Cleanup(v.finishOperations)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	v.monitor = newDirectoryMonitor(func(ctx context.Context, _ string, _ func()) error {
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	v.monitor.sync(v.fileOperationLocations())
	watch := v.monitor.watches[directoryWatchKey(child)]
	recycledPath := filepath.Join(t.TempDir(), "recycled")
	v.loader.recycle = func(directory string, names []string) (browser.RecycleResult, error) {
		// Model the shell recycling by moving the directory out of its old path.
		if err := os.Rename(child, recycledPath); err != nil {
			return browser.RecycleResult{}, err
		}
		return browser.RecycleResult{Directory: directory, Names: names}, nil
	}
	selectName(t, &v.panels[0], "child")
	before := append([]entry(nil), v.panels[1].items...)
	v.recycleSelected()
	waitLoaded(t, v, 0)
	if message := v.panels[0].feedback.Current(); message == nil || message.Text != "Item recycled" {
		t.Fatal("recycle did not complete")
	}
	v.monitor.changed(watch)
	runAutomaticRefresh(v, time.Now().Add(time.Second))
	finishAutomaticRefresh(t, v)
	p := &v.panels[1]
	message := p.feedback.Current()
	if message == nil || message.Text != "Folder not found" || !reflect.DeepEqual(p.items, before) {
		t.Fatal("missing adjacent folder did not retain its listing and report one error")
	}
	for range 20 {
		v.monitor.changed(watch)
		runAutomaticRefresh(v, time.Now().Add(time.Second))
		if p.refreshing || p.refreshNeeded || p.feedback.Current() != message {
			t.Fatal("missing folder kept refreshing or restarting feedback")
		}
	}
	p.feedback.Dismiss()
	v.monitor.changed(watch)
	runAutomaticRefresh(v, time.Now().Add(time.Second))
	if p.feedback.Current() != nil || p.refreshing {
		t.Fatal("dismissed missing-folder message reappeared")
	}
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	v.active = 1
	v.refreshPanel()
	waitLoaded(t, v, 1)
	v.monitor.sync(v.fileOperationLocations())
	fresh := v.monitor.watches[directoryWatchKey(child)]
	if fresh == watch {
		t.Fatal("manual refresh did not re-arm the recovered folder")
	}
	v.monitor.changed(fresh)
	runAutomaticRefresh(v, time.Now().Add(time.Second))
	if !p.refreshing {
		t.Fatal("recovered folder no longer refreshes automatically")
	}
	finishAutomaticRefresh(t, v)
}
