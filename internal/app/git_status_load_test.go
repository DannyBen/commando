package app

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestGitNeverHoldsDirectoryOrNextNavigation(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	v := newView([]panel{{path: root}})
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "child", Directory: true}, {Name: "file.txt"}}}, nil
	}, nil)
	defer v.loader.close()
	entered := make(chan struct{})
	v.loader.readGitStatus = func(ctx context.Context, path, executable string) (map[string]browser.GitStatus, error) {
		if path == root {
			close(entered)
			<-ctx.Done() // No timer: navigation must actively interrupt this read.
			return map[string]browser.GitStatus{"stale.txt": browser.GitChanged}, nil
		}
		return map[string]browser.GitStatus{"file.txt": browser.GitNew}, nil
	}
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	if p.loading || len(p.items) != 2 || p.gitRead == nil || p.gitRead.submitted {
		t.Fatal("directory was not published before starting Git")
	}
	send, _ := panelInputFrames(v)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("deferred Git read did not start")
	}
	selectName(t, p, "child")
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if p.loading || p.path != child || p.gitStatus[transferNameKey("stale.txt")] != browser.GitClean {
		t.Fatal("navigation waited for Git or accepted stale badges")
	}
	p.toggleMark(0)
	selected, position := p.selected, p.list.Position
	waitGitLoaded(t, v, 0)
	if p.gitStatus[transferNameKey("file.txt")] != browser.GitNew || p.gitStatus[transferNameKey("stale.txt")] != browser.GitClean ||
		p.selected != selected || p.list.Position != position || !p.marks[0] {
		t.Fatal("badge completion changed selection, viewport or marks, or returned an old snapshot")
	}
}

func TestGitYieldsToOtherPanelAndRetriesWithoutParallelReads(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	v := newView([]panel{{path: root}, {path: other}})
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	var active atomic.Int32
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		if active.Load() != 0 {
			t.Error("directory and Git reads ran concurrently")
		}
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "file.txt"}}}, nil
	}, nil)
	defer v.loader.close()
	entered := make(chan struct{})
	calls := 0
	v.loader.readGitStatus = func(ctx context.Context, path, executable string) (map[string]browser.GitStatus, error) {
		active.Add(1)
		defer active.Add(-1)
		calls++
		if calls == 1 {
			close(entered)
			<-ctx.Done()
		}
		return map[string]browser.GitStatus{"file.txt": browser.GitChanged}, nil
	}
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	v.submitGitReads()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Git read did not start")
	}
	v.load(1, other, "")
	waitLoaded(t, v, 1)
	if v.panels[0].gitStatus != nil || v.panels[1].path != other {
		t.Fatal("interrupted scan returned badges or held another panel's listing")
	}
	waitGitLoaded(t, v, 0)
	if calls != 2 || v.panels[0].gitStatus[transferNameKey("file.txt")] != browser.GitChanged {
		t.Fatal("interrupted scan did not retry when browsing finished")
	}
}

func TestGitReadIdentityFollowsMovesAndRejectsClosedPanels(t *testing.T) {
	v := newView([]panel{{path: "first"}, {path: "second"}})
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	read := &gitRead{ctx: ctx, cancel: cancel, path: "first", options: v.gitOptions()}
	v.panels[0].gitRead, v.panels[0].gitOptions = read, read.options
	clone := v.panels[0].clone()
	if clone.gitRead != nil || read.ctx.Err() != nil {
		t.Fatal("clone retained the pending read or cancelled its source")
	}
	v.panels[0], v.panels[1] = v.panels[1], v.panels[0]
	v.acceptLoad(loadResult{request: loadRequest{gitRead: read}, gitStatus: map[string]browser.GitStatus{"first.txt": browser.GitNew}})
	if v.panels[0].gitStatus != nil || v.panels[1].gitStatus[transferNameKey("first.txt")] != browser.GitNew {
		t.Fatal("Git completion used an obsolete panel index")
	}
	ctx, cancel = context.WithCancel(context.Background())
	read = &gitRead{ctx: ctx, cancel: cancel, path: "first", options: v.gitOptions()}
	v.panels[1].gitRead = read
	v.active = 1
	v.closePanel()
	v.acceptLoad(loadResult{request: loadRequest{gitRead: read}, gitStatus: map[string]browser.GitStatus{"late.txt": browser.GitChanged}})
	if read.ctx.Err() == nil || v.panels[0].gitStatus != nil || v.closedPanels[0].panel.gitStatus[transferNameKey("late.txt")] != browser.GitClean {
		t.Fatal("late result updated a closed panel or its successor")
	}
}
