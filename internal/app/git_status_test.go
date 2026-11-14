package app

import (
	"context"
	"image"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestGitExecutableValidationStaysInSettings(t *testing.T) {
	v := newPreviewView()
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitPath, settings.Value{Text: filepath.Join(t.TempDir(), "missing-git")}); err != nil {
		t.Fatal(err)
	}
	v.loader = newDirectoryLoader(browser.ReadDirectory, nil)
	defer v.loader.close()
	saved := false
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { saved = true; return "", nil }
	}
	v.startSettingsWidget("programs.git")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	send(modified(key.NameReturn, key.ModCtrl))
	waitSettingsSave(t, v, s)
	if saved || v.settingsInteraction() != s || s.pending || s.errorText != "Git executable not found" || v.panels[0].feedback.Current() != nil {
		t.Fatal("missing Git did not stay in the editable Settings dialog")
	}
	// Turning integration off must remain possible without a working executable.
	v.previewSettingValue(settingGitEnabled, settings.Value{Bool: false})
	send(modified(key.NameReturn, key.ModCtrl))
	waitSettingsSave(t, v, s)
	if !saved || v.settingsInteraction() != nil || v.gitOptions().enabled {
		t.Fatal("could not disable integration after executable validation failed")
	}
}

func TestGitEdgesKeepRowGeometry(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		v := newPreviewView()
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		p := &v.panels[0]
		p.gitOptions = v.gitOptions()
		item := entry{name: "Ágjpq-long-filename.txt", file: browser.Entry{Name: "Ágjpq-long-filename.txt", SizeKnown: true, Bytes: 1024}}
		for _, selected := range []bool{false, true} {
			var want layout.Dimensions
			var wantConstraints layout.Constraints
			var wantScrollWidth int
			for _, status := range []browser.GitStatus{browser.GitClean, browser.GitNew, browser.GitChanged} {
				p.gitStatus = gitStatusNames(map[string]browser.GitStatus{item.file.Name: status})
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(400, 80))}
				bg, fg, metadata := v.appearance.entryColors(item.file, selected, false)
				var constraints layout.Constraints
				var scroll textMarquee
				got := v.fileRowSurfaceEdges(gtx, gtx.Constraints.Max, bg, v.appearance.Colors.Edge, v.gitEdgeColor(v.fileGitStatus(p, item.file)), func(gtx layout.Context) layout.Dimensions {
					constraints = gtx.Constraints
					return v.rowContents(gtx, item, fg, metadata, metadataSize, v.appearance.entryTextRoles(item.file, selected, false), false, &scroll)
				})
				if status == browser.GitClean {
					want, wantConstraints, wantScrollWidth = got, constraints, scroll.width
				} else if got != want || constraints != wantConstraints || scroll.width != wantScrollWidth {
					t.Fatalf("Git changed row geometry at scale %g: got=%+v want=%+v", scale, got, want)
				}
			}
		}
	}
}

func TestGitEdgePixels(t *testing.T) {
	if os.Getenv("COMMANDO_PREVIEW_GIT") == "" {
		t.Skip("optional rendered Git edge verification")
	}
	const width, height = 160, 40
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Fatal(err)
	}
	defer window.Release()
	v := newPreviewView()
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	p := &v.panels[0]
	p.gitOptions = v.gitOptions()
	file := browser.Entry{Name: "file.txt"}
	for _, scale := range []float32{1, 1.5, 2} {
		for _, selected := range []bool{false, true} {
			capture := func(status browser.GitStatus) *image.RGBA {
				p.gitStatus = gitStatusNames(map[string]browser.GitStatus{file.Name: status})
				var ops op.Ops
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(width, height))}
				bg, _, _ := v.appearance.entryColors(file, selected, false)
				v.fileRowSurfaceEdges(gtx, gtx.Constraints.Max, bg, v.appearance.Colors.Error, v.gitEdgeColor(v.fileGitStatus(p, file)), func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
				if err := window.Frame(&ops); err != nil {
					t.Fatal(err)
				}
				img := image.NewRGBA(image.Rect(0, 0, width, height))
				if err := window.Screenshot(img); err != nil {
					t.Fatal(err)
				}
				return img
			}
			base := capture(browser.GitClean)
			leftWidth := 0
			for leftWidth < width/2 && base.RGBAAt(leftWidth, height/2) != base.RGBAAt(width/2, height/2) {
				leftWidth++
			}
			if leftWidth == 0 {
				t.Fatal("left reference edge is missing")
			}
			for _, status := range []browser.GitStatus{browser.GitNew, browser.GitChanged} {
				marked := capture(status)
				for y := range height {
					for x := range width {
						changed := marked.RGBAAt(x, y) != base.RGBAAt(x, y)
						if changed != (x >= width-leftWidth) {
							t.Fatalf("edge differs from left thickness or row bounds: scale=%g selected=%v status=%v pixel=(%d,%d)", scale, selected, status, x, y)
						}
					}
				}
			}
		}
	}
}

func TestGitFolderBadges(t *testing.T) {
	v := newPreviewView()
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	p := &v.panels[0]
	p.gitOptions = v.gitOptions()
	p.gitStatus = gitStatusNames(map[string]browser.GitStatus{"assets": browser.GitNew, "src": browser.GitChanged})
	for _, tc := range []struct {
		file browser.Entry
		want browser.GitStatus
	}{
		{browser.Entry{Name: "assets", Directory: true}, browser.GitNew},
		{browser.Entry{Name: "src", Directory: true}, browser.GitChanged},
		{browser.Entry{Name: "clean", Directory: true}, browser.GitClean},
		{browser.Entry{Name: "src", Directory: true, Parent: true}, browser.GitClean},
	} {
		if got := v.fileGitStatus(p, tc.file); got != tc.want {
			t.Fatalf("folder badge for %+v = %v; want %v", tc.file, got, tc.want)
		}
	}
}

func TestGitSettingsAndRefresh(t *testing.T) {
	root := t.TempDir()
	v := newView([]panel{{path: root}, {path: root}})
	v.applySettings()
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "new.txt"}, {Name: "changed.txt"}}}, nil
	}, nil)
	defer v.loader.close()
	calls := 0
	statuses := map[string]browser.GitStatus{"new.txt": browser.GitNew, "changed.txt": browser.GitChanged}
	var gitErr error
	v.loader.readGitStatus = func(ctx context.Context, path, executable string) (map[string]browser.GitStatus, error) {
		calls++
		if path != root || executable != "custom git.exe" {
			t.Errorf("Git query = %q, %q", path, executable)
		}
		return maps.Clone(statuses), gitErr
	}
	v.load(0, root, "")
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	if calls != 0 || v.panels[0].gitStatus != nil {
		t.Fatal("Git ran with integration disabled")
	}
	send, _ := panelInputFrames(v)
	v.startSettingsWidget("programs.git")
	send()
	send(pressed(key.NameLeftArrow))
	if v.gitOptions().enabled || !v.widgetValues().Get(settings.CatalogueDefault, settingGitEnabled).Bool {
		t.Fatal("Git settings applied before Save or draft did not change")
	}
	send(pressed(key.NameEscape))
	if v.gitOptions().enabled {
		t.Fatal("Cancel enabled integration")
	}
	v.startSettingsWidget("programs.git")
	send()
	send(pressed(key.NameLeftArrow))
	v.previewSettingValue(settingGitPath, settings.Value{Text: "custom git.exe"})
	send(modified(key.NameReturn, key.ModCtrl))
	if !v.gitOptions().enabled || v.gitOptions().executable != "custom git.exe" || calls != 0 {
		t.Fatal("Save did not apply Git settings, or launched an unrequested scan")
	}
	send(refreshKey())
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	if calls != 1 || !maps.Equal(v.panels[0].gitStatus, gitStatusNames(statuses)) || v.panels[1].gitStatus != nil {
		t.Fatal("Ctrl+R did not update only the active panel", calls)
	}
	p := &v.panels[0]
	p.toggleMark(0)
	clone := p.clone()
	clone.gitStatus[transferNameKey("new.txt")] = browser.GitChanged
	if p.gitStatus[transferNameKey("new.txt")] != browser.GitNew {
		t.Fatal("cloned panel shares Git status storage")
	}
	// Automatic refresh publishes the listing before its optional Git read.
	p.refreshing = true
	v.loader.submit(loadRequest{panel: 0, path: root, autoRefresh: true, action: actionBrowse})
	v.acceptLoad(<-v.loader.results)
	if calls != 1 || !maps.Equal(p.gitStatus, gitStatusNames(statuses)) || !p.marks[0] || p.gitRead == nil || !p.gitRead.automatic {
		t.Fatalf("automatic refresh did not defer Git or changed independent panel metadata: calls=%d status=%v marks=%v read=%+v pending=%v", calls, p.gitStatus, p.marks, p.gitRead, p.refreshNeeded)
	}
	statuses = map[string]browser.GitStatus{}
	send(refreshKey())
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	if calls != 2 || len(p.gitStatus) != 0 {
		t.Fatal("Ctrl+R retained committed/restored file badges")
	}
	statuses = map[string]browser.GitStatus{"new.txt": browser.GitNew}
	send(refreshKey())
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	gitErr = browser.ErrGitUnavailable
	send(refreshKey())
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	if p.gitStatus != nil || len(p.items) != 2 || p.err != nil || v.gitOptions().enabled || p.feedback.Current() == nil || p.feedback.Current().Text != "Git integration disabled" {
		t.Fatal("missing Git did not disable integration or disturbed the listing")
	}
	gitErr = nil
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	send(refreshKey())
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	// Captured results from an older configuration cannot restore its badges.
	ctx, cancel := context.WithCancel(v.loader.ctx)
	read := &gitRead{ctx: ctx, cancel: cancel, path: root, options: v.gitOptions()}
	p.gitRead = read
	stale := loadResult{request: loadRequest{gitRead: read}, gitStatus: statuses}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: false}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	if p.gitStatus != nil {
		t.Fatal("disabling integration retained cached badges")
	}
	v.acceptLoad(stale)
	if p.gitStatus != nil {
		t.Fatal("stale Git result survived a settings change")
	}
}

func TestGitNavigationStartupAndShutdown(t *testing.T) {
	values := settings.Values{}
	if err := values.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	if err := values.Set(settings.CatalogueDefault, settingGitPath, settings.Value{Text: filepath.Join(t.TempDir(), "missing-git")}); err != nil {
		t.Fatal(err)
	}
	v := newConfiguredLocationView([]panelState{{Path: t.TempDir()}}, 0, nil, values)
	defer v.loader.close()
	waitLoaded(t, v, 0)
	waitGitLoaded(t, v, 0)
	if !v.panels[0].gitOptions.enabled || v.panels[0].feedback.Current() != nil {
		t.Fatal("startup lost saved Git settings or showed missing-program feedback")
	}
	entered := make(chan struct{})
	v.loader.readGitStatus = func(ctx context.Context, path, executable string) (map[string]browser.GitStatus, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	v.load(0, t.TempDir(), "")
	waitLoaded(t, v, 0)
	v.submitGitReads(layout.Context{Now: time.Now()})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("directory entry did not request Git status")
	}
	v.loader.close()
	select {
	case <-v.loader.stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel Git work")
	}
}

func TestGitPreviewPNG(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_GIT")
	if dir == "" {
		t.Skip("optional Git preview")
	}
	v := newPreviewView()
	v.panels = v.panels[:1]
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingGitEnabled, settings.Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	p := &v.panels[0]
	p.items = append(p.items[:2:2], p.items[12:]...)
	p.selected = 3
	p.gitOptions = v.gitOptions()
	p.gitStatus = gitStatusNames(map[string]browser.GitStatus{"assets": browser.GitNew, "caretaker": browser.GitChanged, "CHANGELOG.md": browser.GitChanged, "README.md": browser.GitChanged, "architecture-notes.md": browser.GitNew, "config.yml": browser.GitNew})
	if err := renderViewPNG(filepath.Join(dir, "git-status.png"), v); err != nil {
		t.Fatal(err)
	}
	v.startSettingsWidget("programs.git")
	if err := renderViewPNG(filepath.Join(dir, "git-settings.png"), v); err != nil {
		t.Fatal(err)
	}
}

func waitGitLoaded(t *testing.T, v *view, index int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for v.panels[index].gitRead != nil {
		v.submitGitReads(layout.Context{Now: time.Now()})
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("Git status did not complete")
		}
	}
}
