package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func dataFolderValues(t *testing.T, target string) settings.Values {
	t.Helper()
	values := settings.Values{}
	if err := values.Set(settings.CatalogueDefault, settingDataFolder, settings.Value{Text: target}); err != nil {
		t.Fatal(err)
	}
	return values
}

func TestDataFolderCopyAndManualRestart(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "existing"}[existing], func(t *testing.T) {
			base, target := t.TempDir(), t.TempDir()
			store := dataFolderStore{base: base, active: base, chosen: base}
			values := dataFolderValues(t, target)
			_ = values.Set(settings.CatalogueDefault, settingLightness, settings.Value{Number: 0.45})
			state := sessionState{Panels: []panelState{{Path: "current panel"}}, PanelWidth: 321}
			wantState := state
			if existing {
				other := dataFolderValues(t, t.TempDir()) // Must not redirect a second time.
				_ = other.Set(settings.CatalogueDefault, settingLightness, settings.Value{Number: 0.7})
				if err := settings.Save(target, other); err != nil {
					t.Fatal(err)
				}
				wantState = sessionState{Panels: []panelState{{Path: "destination panel"}}}
				if err := saveState(target, wantState); err != nil {
					t.Fatal(err)
				}
			}
			write := func(dir, name, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(base, "shell-menu.jsonl", "source diagnostic")
			write(base, "transfer-trace-test.jsonl", "transfer diagnostic")
			write(base, "trace-log.jsonl", "diagnostic")
			write(base, "unrelated.txt", "leave here")
			write(target, "shell-menu.jsonl", "existing diagnostic")
			message, err := store.save(values, state)
			if err != nil || message != "Restart Commando" || store.active != base {
				t.Fatalf("save: %q, %v, active=%s", message, err, store.active)
			}
			assertBatchFile(t, filepath.Join(target, "shell-menu.jsonl"), "existing diagnostic")
			assertBatchFile(t, filepath.Join(base, "shell-menu.jsonl"), "source diagnostic")
			assertBatchFile(t, filepath.Join(target, "transfer-trace-test.jsonl"), "transfer diagnostic")
			assertBatchFile(t, filepath.Join(target, "trace-log.jsonl"), "diagnostic")
			for _, name := range []string{"unrelated.txt", dataFolderPointer} {
				if _, err := os.Lstat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("copied %s: %v", name, err)
				}
			}
			activeState, err := loadState(base)
			if err != nil || !reflect.DeepEqual(activeState, state) {
				t.Fatalf("source state: %+v, %v", activeState, err)
			}
			resolved, err := resolveDataFolder(base)
			if err != nil || resolved != target {
				t.Fatalf("startup folder: %s, %v", resolved, err)
			}
			loaded, err := settings.Load(resolved, settings.CatalogueDefault)
			if err != nil {
				t.Fatal(err)
			}
			wantLightness := 0.45
			if existing {
				wantLightness = 0.7
			}
			if got := loaded.Get(settings.CatalogueDefault, settingLightness).Number; got != wantLightness {
				t.Fatalf("loaded lightness %g", got)
			}
			restarted := dataFolderStore{base: base, active: resolved, chosen: resolved}
			restarted.restoreLocation(&loaded)
			if got := loaded.Get(settings.CatalogueDefault, settingDataFolder).Text; got != target {
				t.Fatalf("destination redirected to %s", got)
			}
			gotState, err := loadState(resolved)
			if err != nil || !reflect.DeepEqual(gotState, wantState) {
				t.Fatalf("destination state: %+v, %v", gotState, err)
			}
			// Later saves belong to the running source session, preserving the copy.
			_ = values.Set(settings.CatalogueDefault, settingLightness, settings.Value{Number: 0.5})
			if _, err := store.save(values, state); err != nil {
				t.Fatal(err)
			}
			loaded, err = settings.Load(target, settings.CatalogueDefault)
			if err != nil || loaded.Get(settings.CatalogueDefault, settingLightness).Number != wantLightness {
				t.Fatal("later save overwrote destination", err)
			}
		})
	}
}

func TestDataFolderResetAndFailedCopy(t *testing.T) {
	base, active, target := t.TempDir(), t.TempDir(), t.TempDir()
	if err := saveDataFolderPointer(base, active); err != nil {
		t.Fatal(err)
	}
	store := dataFolderStore{base: base, active: active, chosen: active}
	if err := os.Mkdir(filepath.Join(active, "shell-menu.jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.save(dataFolderValues(t, target), sessionState{}); err == nil {
		t.Fatal("copy of non-file succeeded")
	}
	if got, err := resolveDataFolder(base); err != nil || got != active || store.chosen != active {
		t.Fatal("failed copy switched folders", got, err)
	}
	if err := os.Remove(filepath.Join(active, "shell-menu.jsonl")); err != nil {
		t.Fatal(err)
	}
	if message, err := store.save(settings.Values{}, sessionState{}); err != nil || message != "Restart Commando" {
		t.Fatal("reset failed", message, err)
	}
	if got, err := resolveDataFolder(base); err != nil || got != base || store.active != active {
		t.Fatal("reset changed running location", got, err)
	}
	values := dataFolderValues(t, active)
	reset := dataFolderStore{base: base, active: base, chosen: base}
	reset.restoreLocation(&values)
	if values.Overridden(settingDataFolder) {
		t.Fatal("default location retains stale override")
	}
}

func TestDataFolderUnreadablePointerCanBeReset(t *testing.T) {
	for _, path := range []string{"relative", filepath.Join(t.TempDir(), "missing")} {
		base := t.TempDir()
		if err := saveDataFolderPointer(base, path); err != nil {
			t.Fatal(err)
		}
		active, err := resolveDataFolder(base)
		if err == nil || active != base {
			t.Fatal("bad pointer did not report fallback", active, err)
		}
		store := dataFolderStore{base: base, active: active}
		if message, err := store.save(settings.Values{}, sessionState{}); err != nil || message != "" {
			t.Fatal("cannot reset bad pointer", message, err)
		}
		if active, err := resolveDataFolder(base); err != nil || active != base {
			t.Fatal("bad pointer was retained", active, err)
		}
	}
}

func TestDataFolderInvalidPathAndAliases(t *testing.T) {
	base := t.TempDir()
	store := dataFolderStore{base: base, active: base, chosen: base}
	if _, err := store.save(dataFolderValues(t, "relative-folder"), sessionState{}); !errors.Is(err, errDataFolderPath) {
		t.Fatal("relative folder accepted", err)
	}
	if _, err := os.Stat(filepath.Join(base, "settings.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid choice saved settings")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(base, alias); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if message, err := store.save(dataFolderValues(t, alias), sessionState{}); err != nil || message != "" {
		t.Fatal("same folder required restart", message, err)
	}
	if got, err := resolveDataFolder(base); err != nil || got != base {
		t.Fatal("alias switched startup folder", got, err)
	}
	// An existing destination link wins and its target is never written.
	source, destination := filepath.Join(base, "source"), filepath.Join(base, "destination")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, destination); err != nil {
		t.Fatal(err)
	}
	if err := copyDataFile(source, destination); err != nil {
		t.Fatal(err)
	}
	assertBatchFile(t, source, "keep")
	if info, err := os.Lstat(destination); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("existing link replaced", err)
	}
}

func TestSettingsSaveWorkerKeepsSessionUntilCompletion(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "retry"}[fail], func(t *testing.T) {
			v := newPreviewView()
			v.loader = newDirectoryLoader(func(string) (browser.Directory, error) { return browser.Directory{}, nil }, nil)
			t.Cleanup(v.finishOperations)
			entered, release := make(chan settings.Values, 1), make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) {
					entered <- values
					<-release
					if fail {
						return "", os.ErrPermission
					}
					return "Restart Commando", nil
				}
			}
			openSettingsGroup(t, v, "appearance.autocolor")
			v.adjustSetting(settingLightness, 1)
			s := v.settingsInteraction()
			v.finishSettingsWidget(layout.Context{}, true)
			select {
			case values := <-entered:
				if values.Get(settings.CatalogueDefault, settingLightness).Number != 37 {
					t.Fatal("save lost snapshot")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("worker did not save")
			}
			v.activateSettingsAction(layout.Context{}, 2)
			v.escape(layout.Context{})
			if !s.pending || v.settingsInteraction() != s || v.settingText(settingLightness) != "37" || v.requestClose() || !v.interaction.panelsBlocked() {
				t.Fatal("pending save released or changed interaction")
			}
			close(release)
			select {
			case result := <-v.loader.results:
				v.acceptLoad(result)
			case <-time.After(3 * time.Second):
				t.Fatal("worker result missing")
			}
			if s.pending || v.mutationsBusy() {
				t.Fatal("completion did not release worker")
			}
			if fail {
				if v.settingsInteraction() != s || s.errorText != "Settings save: access denied" {
					t.Fatal("failed save lost draft")
				}
				v.finishSettingsWidget(layout.Context{}, true)
				select {
				case result := <-v.loader.results:
					v.acceptLoad(result)
				case <-time.After(3 * time.Second):
					t.Fatal("retry missing")
				}
				v.finishSettingsWidget(layout.Context{}, false)
			} else if message := v.panels[s.panel].feedback.Current(); message == nil || message.Text != "Restart Commando" || !v.interaction.idle() {
				t.Fatal("restart message missing")
			}
		})
	}
}

func TestShutdownFinishesSettingsSaveQueuedBehindRead(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprint(direct), func(t *testing.T) {
			v := newPreviewView()
			reading, release := make(chan struct{}), make(chan struct{})
			v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
				close(reading)
				<-release
				return browser.Directory{Path: path}, nil
			}, nil)
			if !v.loader.submit(loadRequest{path: v.panels[0].path, action: actionBrowse}) {
				t.Fatal("read not queued")
			}
			<-reading
			saved := make(chan struct{})
			v.prepareSettingsSave = func(settings.Values) func() (string, error) {
				return func() (string, error) { close(saved); return "Restart Commando", nil }
			}
			if direct {
				v.toggleBooleanSetting(settingSoundEnabled)
			} else {
				openSettingsGroup(t, v, "application.data_folder")
				v.finishSettingsWidget(layout.Context{}, true)
			}
			done := make(chan struct{})
			go func() { v.finishOperations(); close(done) }()
			close(release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("shutdown did not drain queued save")
			}
			select {
			case <-saved:
			default:
				t.Fatal("shutdown discarded settings save")
			}
			if !v.interaction.idle() || v.mutationsBusy() {
				t.Fatal("shutdown lost save reconciliation")
			}
		})
	}
}

func TestSettingsSaveRestoresBrowsingKeyboardFocus(t *testing.T) {
	for _, mode := range []string{"panel selection", "typed path", "action bar", "mouse"} {
		t.Run(mode, func(t *testing.T) {
			v := newPreviewView()
			v.loader = &directoryLoader{requests: make(chan loadRequest, 1), results: make(chan loadResult, 1)}
			v.prepareSettingsSave = func(settings.Values) func() (string, error) {
				return func() (string, error) { return "Restart Commando", nil }
			}
			send, _ := panelInputFrames(v)
			send(key.EditEvent{Text: "@application"})
			send(pressed(key.NameReturn))
			switch mode {
			case "panel selection":
				send(pressed(key.NameRightArrow))
			case "typed path":
				send(pressed(key.NameReturn))
				send(key.EditEvent{Range: key.Range{}, Text: `D:\Portable\Commando`})
			case "action bar":
				send(pressed(key.NameTab))
			}
			switch mode {
			case "mouse":
				send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(240, 405)},
					pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(240, 405)})
			case "action bar":
				send(pressed(key.NameReturn))
			default:
				send(modified(key.NameReturn, key.ModCtrl))
			}
			request := <-v.loader.requests
			send() // Render while save is pending, before the worker result arrives.
			v.loader.results <- loadResult{request: request, settingsMessage: "Restart Commando"}
			send()
			if !v.interaction.idle() {
				t.Fatal("save kept settings interaction")
			}
			send(pressed(key.NameHome))
			before := v.panels[v.active].selected
			send(pressed(key.NameDownArrow))
			if v.panels[v.active].selected != before+1 {
				t.Fatal("Down did not navigate after settings save")
			}
			send(pressed(key.NameUpArrow))
			if v.panels[v.active].selected != before {
				t.Fatal("Up did not navigate after settings save")
			}
			send(pressed("`"))
			if v.paletteInteraction() == nil {
				t.Fatal("Backtick did not open palette after settings save")
			}
		})
	}
}
