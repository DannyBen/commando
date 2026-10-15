package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestPanelBindingsSurviveAbsentPanelsAndRestart(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "panel.focus_3", 0, "Ctrl+Y")
	setTestShortcut(t, v, "panel.focus_3", 1, "Ctrl+Shift+Y")
	v.active = 2
	v.closePanel()
	dir := t.TempDir()
	catalogue := bindingSettingsCatalogue()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(loaded, nil)
	v.active = 0
	send, _ := panelInputFrames(v)
	send(modified("Y", key.ModCtrl))
	if v.active != 0 || len(v.panels) != 2 {
		t.Fatal("absent panel binding changed the workspace")
	}
	for _, c := range v.commandCatalogue() {
		if c.id == "panel.focus_3" {
			t.Fatal("absent panel remained in palette")
		}
	}
	v.active = 1
	v.addPanel(1)
	v.active = 0
	send(modified("3", key.ModAlt))
	if v.active != 0 {
		t.Fatal("replaced panel default remained active")
	}
	send(modified("Y", key.ModCtrl))
	if v.active != 2 {
		t.Fatal("restored panel lost its first binding")
	}
	v.active = 0
	send(modified("Y", key.ModCtrl|key.ModShift))
	if v.active != 2 {
		t.Fatal("restored panel lost its second binding")
	}
	v.settingsValues.Reset("keybindings.panel.focus_3")
	v.active = 0
	send(modified("3", key.ModAlt))
	if v.active != 2 {
		t.Fatal("Reset did not restore panel default")
	}
}

func TestHigherPanelBindingPersistsAndIsEditable(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "panel.focus_12", 0, "Ctrl+Y")
	dir := t.TempDir()
	catalogue := bindingSettingsCatalogue()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(loaded, nil)
	if conflict := findBindingConflict(loaded); conflict != nil {
		t.Fatal(conflict)
	}
	for len(v.panels) < 12 {
		v.addPanel(1)
	}
	v.active = 0
	send, _ := panelInputFrames(v)
	send(modified("Y", key.ModCtrl))
	if v.active != 11 {
		t.Fatal("higher panel shortcut did not execute")
	}
	_, s := openShortcutWidget(t, v, send, "Panel Focus 12")
	if s.bindingCommand.id != "panel.focus_12" || v.settingText(s.fields[0].Key) != "Ctrl+Y" {
		t.Fatal("higher panel could not edit its saved shortcut")
	}
}

func TestGeneratedBindingsShareConflictOwnership(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "panel.focus_3", 0, "Ctrl+Y")
	setTestShortcut(t, v, "settings.appearance.font", 0, "Ctrl+Y")
	if conflict := findBindingConflict(v.settingsValues); conflict == nil {
		t.Fatal("settings and absent panel shortcuts did not conflict")
	}
	v.settingsValues.Reset("keybindings.settings")
	setTestShortcut(t, v, "entry.rename", 1, "Alt+3")
	if conflict := findBindingConflict(v.settingsValues); conflict != nil {
		t.Fatal("rebound panel default was not freed", conflict)
	}
	v.settingsValues.Reset("keybindings.entry")
	setTestShortcut(t, v, "panel.focus_12", 0, "Ctrl+Y")
	if conflict := findBindingConflict(v.settingsValues); conflict == nil {
		t.Fatal("absent higher panel binding did not retain ownership")
	}
}

func TestSettingsShortcutWidgetSaveDispatchAndReset(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil, len(v.panels))
	defer v.loader.close()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", bindingSettingsCatalogue().Save(dir, values) }
	}
	send, _ := panelInputFrames(v)
	p, s := openShortcutWidget(t, v, send, "@font")
	if s.bindingCommand.id != "settings.appearance.font" {
		t.Fatal("wrong settings widget", s.bindingCommand.id)
	}
	send(pressed(key.NameReturn))
	send(modified("Y", key.ModCtrl))
	send(modified(key.NameReturn, key.ModCtrl))
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for s.pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("save timed out")
		}
	}
	send()
	if v.paletteInteraction() != p || p.results[p.selected].command.shortcut() != "Ctrl+Y" || !p.settingsMode {
		t.Fatalf("save did not restore settings palette and effective shortcut: palette=%v shortcut=%q mode=%v error=%q", v.paletteInteraction() == p, p.results[p.selected].command.shortcut(), p.settingsMode, s.errorText)
	}
	send(pressed(key.NameEscape))
	loaded, err := settings.LoadAndRepair(dir, bindingSettingsCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(loaded, nil)
	v.active = 1
	send(modified("Y", key.ModCtrl))
	s = v.settingsInteraction()
	if s == nil || s.panel != 1 || s.bindingCommand != nil || s.fields[0].Key != settingFontFamily || v.paletteInteraction() != nil {
		t.Fatal("shortcut did not open font widget directly in active panel")
	}
	send(pressed(key.NameEscape))
	v.settingsValues.Reset("keybindings.settings.appearance.font")
	send(modified("Y", key.ModCtrl))
	if !v.interaction.idle() {
		t.Fatal("reset retained settings binding")
	}
}

func TestEditorsNewAndSavedRulesAreSeparatelyBindable(t *testing.T) {
	v := newPreviewView()
	v.settingsValues.SetCollection(settings.EditorCollection, []map[string]any{{"ext": []string{"txt"}, "program": "editor", "args": "%SelectedFiles%"}})
	if err := settings.CatalogueDefault.NormalizeIdentities(&v.settingsValues); err != nil {
		t.Fatal(err)
	}
	var newCommand, saved command
	for _, c := range v.settingsCatalogue() {
		if c.name == "New" {
			newCommand = c
		}
		if c.name == "txt" {
			saved = c
		}
	}
	if !v.canEditShortcuts(newCommand) || !v.canEditShortcuts(saved) || saved.id == newCommand.id {
		t.Fatal("collection shortcut eligibility is wrong")
	}
	setTestShortcut(t, v, newCommand.id, 0, "Ctrl+Y")
	send, _ := panelInputFrames(v)
	send(modified("Y", key.ModCtrl))
	s := v.settingsInteraction()
	if s == nil || s.collection != settings.EditorCollection || s.item != -1 {
		t.Fatal("New shortcut did not open a fresh editor rule")
	}
}

func TestPanelBindingFromFilterRetainsQueryAndNativeEditing(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "panel.focus_2", 0, "Ctrl+Y")
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "/"})
	send(key.EditEvent{Text: "go"})
	send(modified("2", key.ModAlt))
	if v.active != 0 || v.filterInteraction() == nil {
		t.Fatal("removed panel shortcut still left filter")
	}
	send(modified("Y", key.ModCtrl))
	if v.active != 1 || v.filterInteraction() != nil || v.panels[0].filter != "go" {
		t.Fatal("custom panel shortcut lost query or failed to leave filter")
	}
	v.active = 0
	setTestShortcut(t, v, "panel.focus_2", 0, "Ctrl+A")
	send(key.EditEvent{Text: "/"})
	send(modified("A", key.ModCtrl))
	if v.active != 0 || v.filterInteraction() == nil || v.filterEditor.SelectedText() != "go" {
		t.Fatal("panel shortcut stole native text selection")
	}
}

func TestGeneratedBindingLoadRejectsInvalidPanelIdentities(t *testing.T) {
	for _, id := range []string{"focus_0", "focus_01", "focus_-1", "focus_12.extra", "focus_9999999999999999999999999999"} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("[keybindings.panel."+id+"]\nshortcut_1 = 'Ctrl+Y'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := settings.Load(dir, bindingSettingsCatalogue()); err == nil {
				t.Fatal("invalid panel identity accepted")
			}
		})
	}
}

func TestModifiedEnterPanelBindingFromFilter(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "panel.focus_2", 0, "Ctrl+Enter")
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "/"})
	send(modified(key.NameEnter, key.ModCtrl))
	if v.active != 1 || v.filterInteraction() != nil {
		t.Fatal("modified keypad Enter finished editing without focusing its panel")
	}
}
