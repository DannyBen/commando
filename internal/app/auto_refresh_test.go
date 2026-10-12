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
