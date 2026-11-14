package app

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func TestMissingGitDisablesPersistsAndRequiresValidPathToEnable(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v := newView([]panel{{path: root}, {path: root}})
	missing := filepath.Join(root, "missing-git")
	for field, value := range map[string]settings.Value{
		settingGitEnabled: {Bool: true}, settingGitPath: {Text: missing},
	} {
		if err := v.settingsValues.Set(settings.CatalogueDefault, field, value); err != nil {
			t.Fatal(err)
		}
	}
	v.loader = newDirectoryLoader(browser.ReadLocation, nil)
	defer v.loader.close()
	var calls atomic.Int32
	v.loader.readGitStatus = func(ctx context.Context, path, executable string) (map[string]browser.GitStatus, error) {
		calls.Add(1)
		return browser.ReadGitStatus(ctx, path, executable)
	}
	catalogue := bindingSettingsCatalogue()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", catalogue.Save(data, values) }
	}
	for i := range v.panels {
		v.prepareGitRead(&v.panels[i], v.gitOptions())
		v.panels[i].gitStatus = map[string]browser.GitStatus{"old.txt": browser.GitChanged}
	}
	v.submitGitReads(layout.Context{Now: time.Now()})
	waitGitLoaded(t, v, 0)
	// A cancelled read from the other panel can arrive before the save result.
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for v.directSettingsSave != nil {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("automatic Git disable did not save")
		}
	}
	loaded, err := settings.LoadAndRepair(data, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	if v.gitOptions().enabled || loaded.Get(catalogue, settingGitEnabled).Bool || loaded.Get(catalogue, settingGitPath).Text != missing {
		t.Fatal("missing executable did not persist Disabled while retaining its path")
	}
	if message := v.panels[0].feedback.Current(); message == nil || message.Kind != feedback.Error || message.Text != "Git integration disabled" {
		t.Fatal("automatic disable did not explain the settings change in the footer")
	}
	for i := range v.panels {
		if v.panels[i].gitRead != nil || v.panels[i].gitStatus != nil {
			t.Fatal("disabled Git retained pending work or markers on another panel")
		}
	}
	readsBeforeNavigation := calls.Load()
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	v.submitGitReads(layout.Context{Now: time.Now()})
	if calls.Load() != readsBeforeNavigation || v.panels[0].gitRead != nil {
		t.Fatal("directory navigation retried the missing executable")
	}
	v.startSettingsWidget("programs.git")
	s := v.settingsInteraction()
	if v.widgetValues().Get(catalogue, settingGitEnabled).Bool {
		t.Fatal("Git dialog did not show Disabled")
	}
	v.previewSettingValue(settingGitEnabled, settings.Value{Bool: true})
	send, _ := panelInputFrames(v)
	send(modified(key.NameReturn, key.ModCtrl))
	waitSettingsSave(t, v, s)
	if v.settingsInteraction() != s || s.errorText != browser.ErrGitUnavailable.Error() || v.gitOptions().enabled {
		t.Fatal("invalid path did not prevent re-enabling Git in Settings")
	}
}

func TestGitFailureWaitsForSettingsAndDoesNotDisableCorrectedConfiguration(t *testing.T) {
	for _, correction := range []string{"cancel", "path", "installed"} {
		v := newPreviewView()
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
			t.Fatal(err)
		}
		missing := filepath.Join(t.TempDir(), "git.exe")
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitPath, settings.Value{Text: missing}); err != nil {
			t.Fatal(err)
		}
		v.loader = newDirectoryLoader(browser.ReadLocation, nil)
		v.prepareSettingsSave = func(settings.Values) func() (string, error) {
			return func() (string, error) { return "", nil }
		}
		v.prepareGitRead(&v.panels[0], v.gitOptions())
		read := v.panels[0].gitRead
		v.startSettingsWidget("programs.git")
		s := v.settingsInteraction()
		v.acceptGitRead(loadResult{request: loadRequest{gitRead: read}, gitErr: browser.ErrGitUnavailable})
		if !v.gitOptions().enabled || v.settingsInteraction() != s || v.gitDisablePending == nil {
			t.Fatal("background failure changed an open settings transaction")
		}
		send, _ := panelInputFrames(v)
		if correction != "cancel" {
			if correction == "installed" {
				// Validation checks availability; this fixture is never executed.
				if err := os.WriteFile(missing, []byte("fixture"), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				program, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				v.previewSettingValue(settingGitPath, settings.Value{Text: program})
			}
			send(modified(key.NameReturn, key.ModCtrl))
			waitSettingsSave(t, v, s)
		} else {
			send(pressed(key.NameEscape))
		}
		v.submitGitReads(layout.Context{Now: time.Now()})
		if v.gitOptions().enabled != (correction != "cancel") || v.gitDisablePending != nil {
			t.Fatal("pending failure disabled a corrected path or failed to disable the unchanged path", correction)
		}
		v.finishOperations()
	}
}

func TestRepositoryGitErrorsDoNotDisableIntegration(t *testing.T) {
	for _, err := range []error{browser.ErrGitTimeout, browser.ErrGitUntrusted, browser.ErrGitStatus} {
		v := newPreviewView()
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
			t.Fatal(err)
		}
		v.loader = newDirectoryLoader(browser.ReadLocation, nil)
		v.prepareGitRead(&v.panels[0], v.gitOptions())
		v.acceptGitRead(loadResult{request: loadRequest{gitRead: v.panels[0].gitRead}, gitErr: err})
		if !v.gitOptions().enabled || v.gitDisablePending != nil {
			t.Fatal("repository error disabled Git globally", err)
		}
		v.loader.close()
	}
}

func TestWatcherGitRefreshCoalescesKeepsMarkersAndStaysQuiet(t *testing.T) {
	v, root, watch := automaticRefreshView(t)
	v.panels = v.panels[:1]
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	p := &v.panels[0]
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	v.loader.readGitStatus = func(ctx context.Context, _, _ string) (map[string]browser.GitStatus, error) {
		switch calls.Add(1) {
		case 1:
			return map[string]browser.GitStatus{"b.txt": browser.GitNew}, nil
		case 2:
			close(entered)
			select {
			case <-release:
				return map[string]browser.GitStatus{"b.txt": browser.GitChanged}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case 3:
			return nil, browser.ErrGitTimeout
		default:
			return map[string]browser.GitStatus{}, nil
		}
	}
	start := time.Now()
	v.prepareGitRead(p, v.gitOptions())
	v.submitGitReads(layout.Context{Now: start})
	waitGitLoaded(t, v, 0)
	selectName(t, p, "b.txt")
	p.toggleMark(p.selected)
	p.list.Position.First, p.list.Position.Offset = p.selected, 7
	p.prefix, p.prefixAt = "b", start
	p.feedback.Show(feedback.Success, "Copied")
	message, selected, position := p.feedback.Current(), p.selected, p.list.Position

	refresh := func(now time.Time) {
		t.Helper()
		for range 20 {
			v.monitor.changed(watch)
		}
		runAutomaticRefresh(v, now)
		finishAutomaticRefresh(t, v)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	// Several completed listing refreshes share one pending scan, retaining the
	// old markers and the original deadline instead of waiting for writes to stop.
	for _, elapsed := range []time.Duration{500, 700, 900} {
		now := start.Add(elapsed * time.Millisecond)
		refresh(now)
		v.submitGitReads(layout.Context{Now: now})
		if calls.Load() != 1 || p.gitRead == nil || p.gitRead.submitted || p.gitStatus[transferNameKey("b.txt")] != browser.GitNew || p.items[p.selected].file.Bytes != 7 {
			t.Fatal("watcher blocked the listing, cleared markers or ran Git too often")
		}
	}
	v.submitGitReads(layout.Context{Now: start.Add(gitRefreshInterval)})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("coalesced Git scan did not start at its deadline")
	}
	if p.loading || p.refreshing || p.gitStatus[transferNameKey("b.txt")] != browser.GitNew {
		t.Fatal("pending Git hid the listing or its previous markers")
	}
	close(release)
	waitGitLoaded(t, v, 0)
	if calls.Load() != 2 || p.gitStatus[transferNameKey("b.txt")] != browser.GitChanged || p.selected != selected || p.list.Position != position || !p.marks[selected] || p.prefix != "b" || p.feedback.Current() != message {
		t.Fatal("automatic Git completion disturbed panel state or did not update markers")
	}
	refresh(start.Add(1500 * time.Millisecond))
	v.submitGitReads(layout.Context{Now: start.Add(1500 * time.Millisecond)})
	if calls.Load() != 2 || p.gitRead == nil || p.gitRead.submitted {
		t.Fatal("next automatic scan ignored the rate limit")
	}
	v.submitGitReads(layout.Context{Now: start.Add(2 * gitRefreshInterval)})
	waitGitLoaded(t, v, 0)
	if calls.Load() != 3 || p.feedback.Current() != message || p.gitStatus[transferNameKey("b.txt")] != browser.GitChanged {
		t.Fatal("automatic Git failure showed feedback or discarded the last status")
	}
	// Ctrl+R bypasses the automatic cooldown and can clear restored/clean markers.
	send, _ := panelInputFrames(v)
	send(refreshKey())
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	if calls.Load() != 4 || len(p.gitStatus) != 0 {
		t.Fatal("manual refresh was throttled or retained stale markers")
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: false}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	refresh(start.Add(3 * gitRefreshInterval))
	v.submitGitReads(layout.Context{Now: start.Add(3 * gitRefreshInterval)})
	if calls.Load() != 4 || p.gitRead != nil {
		t.Fatal("watcher scheduled Git with integration disabled")
	}
}

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
	t.Run("entry", func(t *testing.T) { testGitYieldsToOtherPanel(t, false) })
	t.Run("watcher", func(t *testing.T) { testGitYieldsToOtherPanel(t, true) })
}

func testGitYieldsToOtherPanel(t *testing.T, automatic bool) {
	t.Helper()
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
	var calls atomic.Int32
	v.loader.readGitStatus = func(ctx context.Context, path, executable string) (map[string]browser.GitStatus, error) {
		active.Add(1)
		defer active.Add(-1)
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
		}
		return map[string]browser.GitStatus{"file.txt": browser.GitChanged}, nil
	}
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	v.panels[0].gitRead.automatic = automatic
	v.submitGitReads(layout.Context{Now: time.Now()})
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
	// Allow a full cooldown for automatic work; entry reads retry immediately.
	v.submitGitReads(layout.Context{Now: v.panels[0].gitReadAt.Add(gitRefreshInterval)})
	waitGitLoaded(t, v, 0)
	if calls.Load() < 2 || v.panels[0].gitStatus[transferNameKey("file.txt")] != browser.GitChanged {
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
