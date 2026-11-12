package app

import (
	"errors"
	"image"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/windowshell"
)

func TestActionImportFromSettingsThroughMouseMenuToReviewedSave(t *testing.T) {
	v := menuTestView()
	before := v.settingsValues.Clone()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "@actions new from context menu"})
	palette := v.paletteInteraction()
	if len(palette.results) == 0 || palette.results[0].command.id != actionImportCommand || !palette.results[0].command.allowed(v) {
		t.Fatal("import command is not discoverable", palette.results)
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || !s.importing || !s.selecting || v.actionAllowed(actionMutation) {
		t.Fatal("capture drawer did not retain a protected settings session")
	}
	// Browsing and switching panels must retain the drawer and leave its draft untouched.
	send(pressed(key.NameTab), pressed(key.NameDownArrow))
	if v.active != 1 || v.settingsInteraction() != s || !reflect.DeepEqual(before, v.settingsValues.Clone()) || s.dirty {
		t.Fatal("panel browsing ended capture or changed settings")
	}
	p := &v.panels[1]
	marks := maps.Clone(p.marks)
	point := f32.Pt(285, 110)
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonSecondary, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	native, ok := v.interaction.current.(*nativeSession)
	if !ok || !native.request.options.Capture || len(native.request.paths) != 1 || native.request.paths[0] != p.entryPath(1) || !maps.Equal(p.marks, marks) {
		t.Fatal("capture menu did not snapshot only the example item", v.interaction.current)
	}
	if v.interaction.suspended != s || v.requestClose() {
		t.Fatal("native menu lost settings owner or shutdown protection")
	}
	captured := &windowshell.CapturedCommand{Name: "Codex here...", Command: `explorer.exe "codex://new?path=%1"`, Directory: true, Extensions: []string{"*"}}
	if !v.completeNative(shellCompletion{session: native, result: windowshell.Result{Captured: captured}}) {
		t.Fatal("native completion rejected its owner")
	}
	if v.settingsInteraction() != s || s.importing || s.selecting || !s.dirty || s.title != "Programs › Actions › New" || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatal("capture did not hand off to an unsaved New Action draft", s)
	}
	if v.settingText(settings.ActionCollection+".path") != "explorer.exe" || v.settingText(settings.ActionCollection+".args") != `"codex://new?path=%Entry%"` || s.fields[2].Key != settings.ActionCollection+".mode" {
		t.Fatal("regular editor did not receive imported values")
	}
	for i := range v.panels {
		if v.panels[i].refreshNeeded {
			t.Fatal("capture-only completion triggered execution reconciliation")
		}
	}
	send(modified(key.NameReturn, key.ModCtrl))
	rules := v.settingsValues.ActionRules()
	if len(rules) != 1 || rules[0].Name != captured.Name || !settings.ValidIdentity(rules[0].ID) || rules[0].Target != settings.ActionFolders {
		t.Fatal("reviewed action was not saved", rules)
	}
}

func TestActionImportCancellationFailuresAndStaleResults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    windowshell.Result
		err       error
		wantError bool
	}{
		{"cancelled menu", windowshell.Result{Cancelled: true}, nil, false},
		{"unsupported handler", windowshell.Result{}, errors.New("no command"), true},
		{"unsupported arguments", windowshell.Result{Captured: &windowshell.CapturedCommand{Name: "Unsupported", Command: `tool.exe "%1" %*`, Extensions: []string{"*"}}}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := menuTestView()
			before := v.settingsValues.Clone()
			v.startActionImport()
			s := v.settingsInteraction()
			send, _ := panelInputFrames(v)
			send(modified("/", key.ModAlt))
			native, ok := v.interaction.current.(*nativeSession)
			if !ok || !native.request.options.Capture {
				t.Fatal("keyboard context menu did not capture")
			}
			completion := shellCompletion{session: native, result: tc.result, err: tc.err}
			v.completeNative(completion)
			if v.settingsInteraction() != s || !s.importing || (s.errorText != "") != tc.wantError || s.dirty {
				t.Fatal("failed capture lost its retryable drawer", s)
			}
			send(modified(key.NameReturn, key.ModCtrl))
			if v.settingsInteraction() != s || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
				t.Fatal("Ctrl+Enter saved an uncaptured action")
			}
			send(pressed(key.NameEscape))
			if v.settingsInteraction() != nil || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
				t.Fatal("Escape did not discard capture")
			}
			v.startActionImport()
			fresh := v.settingsInteraction()
			if v.completeNative(completion) || v.settingsInteraction() != fresh {
				t.Fatal("stale capture changed a newer session")
			}
		})
	}
}

func TestActionImportBusyAndUnavailableTargets(t *testing.T) {
	v := menuTestView()
	v.startActionImport()
	s := v.settingsInteraction()
	for _, block := range []func(){
		func() { v.transferPending = true },
		func() { v.panels[0].loading = true },
		func() { v.panels[0].drives = true },
	} {
		block()
		v.startContextMenu(layout.Context{}, 0, 1, true)
		if v.settingsInteraction() != s {
			t.Fatal("import bypassed unavailable target or busy policy")
		}
		v.transferPending, v.panels[0].loading, v.panels[0].drives = false, false, false
	}
	v.startBackgroundContextMenu(layout.Context{}, 0)
	if v.settingsInteraction() != s {
		t.Fatal("background menu replaced capture drawer")
	}
}

func TestActionImportPreviews(t *testing.T) {
	if os.Getenv("COMMANDO_ACTION_IMPORT_PREVIEWS") != "1" {
		t.Skip("set COMMANDO_ACTION_IMPORT_PREVIEWS=1 to render import drawers")
	}
	dir := filepath.Join("..", "..", "tmp", "agent", "screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range []image.Point{image.Pt(960, 540), image.Pt(800, 220)} {
		v := menuTestView()
		v.startActionImport()
		name := "action-import"
		if size.Y == 220 {
			name += "-short"
		}
		if err := renderSizedViewPNG(filepath.Join(dir, name+".png"), v, size); err != nil {
			t.Fatal(err)
		}
	}
	dark := menuTestView()
	dark.settingsValues = builtInThemes[1].values.Clone()
	dark.applySettings()
	dark.startActionImport()
	if err := renderSizedViewPNG(filepath.Join(dir, "action-import-dark.png"), dark, image.Pt(960, 540)); err != nil {
		t.Fatal(err)
	}
	v := menuTestView()
	v.startActionImport()
	v.settingsInteraction().errorText = "Cannot import this action"
	if err := renderSizedViewPNG(filepath.Join(dir, "action-import-unsupported.png"), v, image.Pt(960, 540)); err != nil {
		t.Fatal(err)
	}
	v.completeActionImport(shellCompletion{session: &nativeSession{panel: v.active}, result: windowshell.Result{
		Captured: &windowshell.CapturedCommand{Name: "Codex here...", Command: `explorer.exe "codex://new?path=%1"`, Directory: true, Extensions: []string{"*"}},
	}})
	if err := renderSizedViewPNG(filepath.Join(dir, "action-import-review.png"), v, image.Pt(960, 540)); err != nil {
		t.Fatal(err)
	}
}
