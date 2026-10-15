package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func TestBindingOverridesPersistDispatchAndReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	document := "[keybindings.view.next_metadata_column]\nshortcut_1 = 'Ctrl+Y'\nshortcut_2 = 'Ctrl+Shift+Y'\n"
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	catalogue := bindingSettingsCatalogue()
	values, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	// Saving another setting must retain both shortcut overrides.
	if err := values.Set(catalogue, settingLightness, settings.Value{Number: 0.42}); err != nil {
		t.Fatal(err)
	}
	store := dataFolderStore{base: dir, active: dir, chosen: dir}
	if _, err := store.save(values, sessionState{}); err != nil {
		t.Fatal(err)
	}
	values, err = settings.Load(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v := newPreviewView()
	v.restoreSettings(values, nil)
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != before {
		t.Fatal("old shortcut still executed after replacement")
	}
	send(modified("Y", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("replacement shortcut was not delivered or executed")
	}
	send(modified("Y", key.ModCtrl|key.ModShift))
	if v.panels[0].metadata != before.cycle(1).cycle(1) {
		t.Fatal("second shortcut was not delivered or executed")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Next metadata column"})
	if s := v.paletteInteraction(); len(s.results) == 0 || s.results[0].command.shortcut() != "Ctrl+Y / Ctrl+Shift+Y" {
		t.Fatal("palette did not show effective shortcuts")
	}
	send(pressed(key.NameEscape))
	values.Reset(bindingSettingKey("view.next_metadata_column", 0))
	values.Reset(bindingSettingKey("view.next_metadata_column", 1))
	if err := catalogue.Save(dir, values); err != nil {
		t.Fatal(err)
	}
	reset, err := settings.Load(dir, catalogue)
	if err != nil || reset.Overridden(bindingSettingKey("view.next_metadata_column", 0)) {
		t.Fatal("reset did not remove the override", err)
	}
	v.restoreSettings(reset, nil)
	before = v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("reset did not restore the default shortcut")
	}
}

func TestExplicitlyUnassignedBindingSurvivesRestart(t *testing.T) {
	catalogue := bindingSettingsCatalogue()
	var values settings.Values
	setting := bindingSettingKey("view.next_metadata_column", 0)
	if err := values.Set(catalogue, setting, settings.Value{Text: "none"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := catalogue.Save(dir, values); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(dir, catalogue)
	if err != nil || !loaded.Overridden(setting) || loaded.Get(catalogue, setting).Text != "none" {
		t.Fatal("explicit unassignment was lost", err)
	}
	v := newPreviewView()
	v.restoreSettings(loaded, nil)
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != before {
		t.Fatal("unassigned default still executed")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Next metadata column"})
	s := v.paletteInteraction()
	if s.results[0].command.shortcut() != "" {
		t.Fatal("unassigned shortcut remained visible")
	}
	send(pressed(key.NameReturn))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("unassigning a shortcut disabled palette execution")
	}
}

func TestCustomBrowsingPasteBindingLeavesTextFieldPasteFixed(t *testing.T) {
	v := newPreviewView()
	catalogue := bindingSettingsCatalogue()
	for field, text := range map[string]string{
		bindingSettingKey("entry.paste", 0):               "none",
		bindingSettingKey("view.next_metadata_column", 0): "Ctrl+V",
	} {
		if err := v.settingsValues.Set(catalogue, field, settings.Value{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalogue.ValidateOverrides(&v.settingsValues); err != nil {
		t.Fatal(err)
	}
	router, send := clipboardFrames(v)
	before := v.panels[0].metadata
	send(modified("V", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("Ctrl+V did not execute its new browsing action")
	}
	send(modified("D", key.ModAlt))
	v.location.SetText("")
	send()
	send(modified("V", key.ModCtrl))
	send(clipboardData("pasted text"))
	if v.location.Text() != "pasted text" || !router.Source().Focused(&v.location) || v.panels[0].metadata != before.cycle(1) {
		t.Fatal("browsing binding intercepted text-field paste or changed focus")
	}
}

func TestInvalidBindingsUseSettingsRecovery(t *testing.T) {
	for _, invalid := range []string{
		"[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+'",
		"[keybindings.entry.rename]\nshortcut_1 = 42",
		"[keybindings.selection.clear_filter_or_marks]\nshortcut_1 = 'none'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+C'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+Y'\nshortcut_2 = 'Ctrl+Y'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Up'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Alt+1'",
		"[keybindings.entry.rename]\nshortcut_1 = 'x'",
	} {
		t.Run(invalid, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.toml")
			original := "appearance.autocolor.lightness = 0.42\n" + invalid + "\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			catalogue := bindingSettingsCatalogue()
			values, err := settings.LoadAndRepair(dir, catalogue)
			if !errors.Is(err, settings.ErrRepaired) || values.Get(catalogue, settingLightness).Number != 0.42 {
				t.Fatal("invalid binding was not repaired or lost another setting", err)
			}
			backup, err := os.ReadFile(path + ".bak")
			if err != nil || string(backup) != original {
				t.Fatal("recovery lost the original settings document", err)
			}
			if _, err := settings.Load(dir, catalogue); err != nil {
				t.Fatal("repaired settings did not load", err)
			}
		})
	}
}

func TestBindingSwapLoadsAsOneSet(t *testing.T) {
	dir := t.TempDir()
	document := "[keybindings.entry.copy]\nshortcut_1 = 'Ctrl+X'\n[keybindings.entry.cut]\nshortcut_1 = 'Ctrl+C'\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := settings.LoadAndRepair(dir, bindingSettingsCatalogue())
	if err != nil {
		t.Fatal("valid swap was rejected while only one side was loaded", err)
	}
	v := newPreviewView()
	v.restoreSettings(values, nil)
	for _, c := range v.commandCatalogue() {
		if c.id == "entry.copy" && c.shortcut() != "Ctrl+X" || c.id == "entry.cut" && c.shortcut() != "Ctrl+C" {
			t.Fatal("swap lost a binding", c.id, c.shortcut())
		}
	}
}

func TestConflictingSaveDoesNotChangeFileOrCaller(t *testing.T) {
	catalogue := bindingSettingsCatalogue()
	var values settings.Values
	field := bindingSettingKey("entry.rename", 0)
	if err := values.Set(catalogue, field, settings.Value{Text: "Ctrl+C"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte("# existing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := catalogue.Save(dir, values); !errors.Is(err, settings.ErrValue) {
		t.Fatal("conflicting bindings were saved", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "# existing") || !values.Overridden(field) {
		t.Fatal("failed save changed the file or caller", err)
	}
}

func TestAltBindingUpdatesWindowsInputAndResets(t *testing.T) {
	v := newPreviewView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	catalogue := bindingSettingsCatalogue()
	field := bindingSettingKey("view.next_metadata_column", 0)
	if err := v.settingsValues.Set(catalogue, field, settings.Value{Text: "Alt+Y"}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("Y", key.ModAlt))
	if v.panels[0].metadata != before.cycle(1) || !v.nativeKeys.suppressesAltText('y') || !v.nativeKeys.suppressesAltText('Y') || v.nativeKeys.suppressesAltText(' ') {
		t.Fatal("Alt binding lost its action, case handling or native character suppression")
	}
	v.settingsValues.Reset(field)
	v.applySettings()
	if v.nativeKeys.suppressesAltText('y') {
		t.Fatal("reset retained old Alt character suppression")
	}
}
