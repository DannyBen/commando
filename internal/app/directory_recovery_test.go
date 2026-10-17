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

func recoveryView(t *testing.T) (*view, string, string, *directoryWatch) {
	t.Helper()
	root := t.TempDir()
	child := filepath.Join(root, "ancestor", "child")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "same.txt"), filepath.Join(root, ".hidden"), filepath.Join(child, "same.txt")} {
		if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := newBrowserView([]string{child, child, root}, nil)
	t.Cleanup(v.finishOperations)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	v.monitor = newDirectoryMonitor(func(ctx context.Context, _ string, _ func()) error {
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	v.monitor.sync(v.fileOperationLocations())
	return v, root, child, v.monitor.watches[directoryWatchKey(child)]
}

func TestLiveMissingFoldersRecoverWithoutLeakingStateToParent(t *testing.T) {
	for _, change := range []string{"delete", "rename-ancestor", "replace-with-file"} {
		t.Run(change, func(t *testing.T) {
			v, root, child, watch := recoveryView(t)
			v.active = 2
			for pi := 0; pi < 2; pi++ {
				p := &v.panels[pi]
				p.metadata = []metadataColumn{metadataModified, metadataNone}[pi]
				p.visibility = visibilityHideHiddenDotfiles
				selectName(t, p, "same.txt")
				p.toggleMark(p.selected)
				p.list.Position.First, p.list.Position.Offset = p.selected, 8
				p.prefix, p.prefixAt = "same", time.Now()
				p.feedback.Show(feedback.Success, "Copied")
			}
			messages := []*feedback.Message{v.panels[0].feedback.Current(), v.panels[1].feedback.Current()}
			want := filepath.Dir(child)
			switch change {
			case "rename-ancestor":
				if err := os.Rename(filepath.Dir(child), filepath.Join(t.TempDir(), "moved")); err != nil {
					t.Fatal(err)
				}
				want = root
			default:
				if err := os.RemoveAll(child); err != nil {
					t.Fatal(err)
				}
				if change == "replace-with-file" {
					if err := os.WriteFile(child, []byte("replacement"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			v.monitor.changed(watch)
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			finishAutomaticRefresh(t, v)
			for pi := 0; pi < 2; pi++ {
				p := &v.panels[pi]
				if p.path != want || p.drives || p.err != nil || p.loading || p.refreshing || p.refreshNeeded ||
					len(p.marks) != 0 || p.selected != 0 || p.list.Position.First != 0 || p.list.Position.Offset != 0 ||
					p.prefix != "" || p.feedback.Current() != messages[pi] || p.visibility != visibilityHideHiddenDotfiles ||
					p.metadata != []metadataColumn{metadataModified, metadataNone}[pi] {
					t.Fatalf("panel %d did not cleanly recover to %q: %+v", pi, want, p)
				}
				for _, item := range p.items {
					if item.file.Name == ".hidden" {
						t.Fatal("recovery ignored the panel's visibility preference")
					}
				}
			}
			if v.active != 2 || v.sessionState(windowState{}).Panels[0].Path != want {
				t.Fatal("recovery stole focus or persisted the vanished path")
			}
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			v.monitor.changed(watch)
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			if v.panels[0].refreshing || v.panels[1].refreshing {
				t.Fatal("retired folder notification restarted recovery")
			}
		})
	}
}

func TestMissingFolderRecoveryDefersForForegroundAndPendingWork(t *testing.T) {
	for _, protection := range []string{"location", "palette", "inline", "mutation", "arriving-result"} {
		t.Run(protection, func(t *testing.T) {
			v, root, child, watch := recoveryView(t)
			before := append([]entry(nil), v.panels[0].items...)
			if err := os.RemoveAll(filepath.Dir(child)); err != nil {
				t.Fatal(err)
			}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops}
			var arriving loadResult
			if protection == "arriving-result" {
				v.monitor.changed(watch)
				runAutomaticRefresh(v, time.Now().Add(time.Second))
				select {
				case arriving = <-v.loader.results:
				case <-time.After(3 * time.Second):
					t.Fatal("recovery read did not arrive")
				}
			}
			switch protection {
			case "location", "arriving-result":
				v.startLocation(gtx)
				v.location.SetText("draft path")
				v.location.SetCaret(2, 7)
			case "palette":
				v.startPalette(gtx)
			case "inline":
				v.startInlineActions(0, 1, "", "", []string{"Choose"}, nil, nil)
			case "mutation":
				v.loader.busy.Store(1)
			}
			if protection == "arriving-result" {
				v.acceptLoad(arriving)
				finishAutomaticRefresh(t, v)
			} else {
				v.monitor.changed(watch)
				runAutomaticRefresh(v, time.Now().Add(time.Second))
				finishAutomaticRefresh(t, v)
			}
			if v.panels[0].path != child || !v.panels[0].refreshNeeded || !reflect.DeepEqual(v.panels[0].items, before) {
				t.Fatal("recovery disturbed a protected listing or lost its pending update")
			}
			if v.locationEditing() {
				first, last := v.location.Selection()
				if v.location.Text() != "draft path" || first != 2 || last != 7 {
					t.Fatal("arriving recovery changed the location draft or selection")
				}
			}
			v.cancelInteraction(gtx)
			v.loader.busy.Store(0)
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			finishAutomaticRefresh(t, v)
			if v.panels[0].path != root || v.panels[0].refreshNeeded || v.active != 0 {
				t.Fatal("deferred recovery did not resume safely")
			}
		})
	}
}

func TestMissingFolderRecoveryDiscardsResultAfterNavigation(t *testing.T) {
	v, root, child, _ := recoveryView(t)
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	before := append([]entry(nil), v.panels[0].items...)
	v.acceptAutoRefresh(loadResult{request: loadRequest{panel: 0, path: child, autoRefresh: true},
		directory: browser.Directory{Path: filepath.Dir(child)}})
	if v.panels[0].path != root || !reflect.DeepEqual(v.panels[0].items, before) {
		t.Fatal("stale recovery replaced the user's newer directory")
	}
}

func TestRefreshRecoversMissingLocationAndClearsOldIdentity(t *testing.T) {
	v, root, child, _ := recoveryView(t)
	p := &v.panels[0]
	selectName(t, p, "same.txt")
	p.toggleMark(p.selected)
	if err := os.RemoveAll(filepath.Dir(child)); err != nil {
		t.Fatal(err)
	}
	v.refreshPanel()
	waitLoaded(t, v, 0)
	if p.path != root || p.err != nil || len(p.marks) != 0 || p.selected != 1 || p.feedback.Current() != nil {
		t.Fatal("manual refresh treated the surviving parent as the old listing")
	}
}

func TestParentRecoveryStopsAtPermissionErrorsAndHandlesUnavailableRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	child := filepath.Join(root, "missing", "nested")
	for _, permission := range []bool{false, true} {
		var reads []string
		l := &directoryLoader{readDirectory: func(path string) (browser.Directory, error) {
			reads = append(reads, path)
			if path == child {
				return browser.Directory{}, os.ErrNotExist
			}
			if permission {
				return browser.Directory{}, os.ErrPermission
			}
			if len(reads) == 4 {
				return browser.Directory{Path: path}, nil
			}
			return browser.Directory{}, os.ErrNotExist
		}}
		directory, err := l.readExistingParent(child)
		if permission {
			if !errors.Is(err, os.ErrPermission) || len(reads) != 2 {
				t.Fatal("parent recovery bypassed a permission failure")
			}
		} else if err != nil || directory.Drives || len(reads) != 4 || directory.Path != reads[3] {
			t.Fatal("unavailable root did not try the default root")
		}
	}
}
