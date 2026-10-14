package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"github.com/BurntSushi/toml"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestStartupSettingsApplyBeforeDirectoryResults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("appearance.autocolor.lightness = 0.5\nview.show_protected_system_files = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	v := newView([]panel{{path: dir, loading: true, selected: -1}})
	v.restoreSettings(values, nil)
	v.acceptLoad(loadResult{request: loadRequest{panel: 0, path: dir}, directory: browser.Directory{Path: dir, Entries: []browser.Entry{
		{Name: "protected.sys", Hidden: true, System: true},
	}}})
	if v.appearance.FilenameColors.filenameLightness != 0.5 || len(v.panels[0].items) != 1 || v.panels[0].items[0].name != "protected.sys" {
		t.Fatal("startup settings were not applied to initial directory results")
	}
	openSettingsGroup(t, v, "appearance.autocolor")
	v.adjustSetting(settingLightness, 1)
	v.finishSettingsWidget(layout.Context{}, false)
	if v.settingText(settingLightness) != "0.5" {
		t.Fatal("cancel did not restore the loaded override")
	}
}

func TestStartupSettingsErrorsSurviveInitialRead(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{
		{settings.ErrSyntax, "Settings: invalid TOML"},
		{settings.ErrUnknownKey, "Settings: unknown key"},
		{settings.ErrType, "Settings: invalid value type"},
		{settings.ErrRange, "Settings: value outside range"},
		{os.ErrPermission, "Settings: access denied"},
		{errors.New("read failure"), "Cannot read settings"},
	} {
		v := newView([]panel{{path: "sample", loading: true}, {path: "sample", loading: true}})
		v.active = 1
		v.restoreSettings(settings.Values{}, fmt.Errorf("startup: %w", tc.err))
		v.acceptLoad(loadResult{request: loadRequest{panel: 1}, directory: browser.Directory{Path: "sample"}})
		message := v.panels[1].feedback.Current()
		if message == nil || message.Kind != feedback.Error || message.Text != tc.message || v.active != 1 {
			t.Fatalf("startup failure lost its contextual feedback: %#v", message)
		}
		if v.panels[0].feedback.Current() != nil || v.settingText(settingLightness) != "0.36" {
			t.Fatal("startup failure affected another pane or changed defaults")
		}
	}
}

func openSettingsGroup(t *testing.T, v *view, group string) {
	t.Helper()
	v.startSettingsWidget(group)
	if v.settingsInteraction() == nil {
		t.Fatalf("could not open %s", group)
	}
}

func TestSettingsSaveFailureRetryAndCancel(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprint("retry=", retry), func(t *testing.T) {
			v := newPreviewView()
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.toml")
			if err := settings.Save(dir, v.settingsValues); err != nil {
				t.Fatal(err)
			}
			calls := 0
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) {
					calls++
					if calls == 1 {
						return "", os.ErrPermission
					}
					return "", settings.Save(dir, values)
				}
			}
			openSettingsGroup(t, v, "appearance.autocolor")
			v.adjustSetting(settingLightness, 1)
			s := v.settingsInteraction()
			v.finishSettingsWidget(layout.Context{}, true)
			if v.settingsInteraction() != s || s.errorText != "Settings save: access denied" || v.settingText(settingLightness) != "0.37" {
				t.Fatal("failed save lost the editable preview or error")
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "" {
				t.Fatal("failed save changed saved settings", err)
			}
			v.finishSettingsWidget(layout.Context{}, retry)
			if !v.interaction.idle() {
				t.Fatal("retry or cancel did not close widget")
			}
			if retry {
				if calls != 2 {
					t.Fatal("retry did not save")
				}
			} else if calls != 1 || v.settingsValues.Overridden(settingLightness) || v.settingText(settingLightness) != "0.36" {
				t.Fatal("cancel saved or retained failed preview")
			}
			var saved struct {
				Appearance struct{ Autocolor struct{ Lightness float64 } }
			}
			md, err := toml.DecodeFile(path, &saved)
			if err != nil || md.IsDefined("appearance", "autocolor", "lightness") != retry || retry && saved.Appearance.Autocolor.Lightness != 0.37 {
				t.Fatalf("saved settings = %#v, %v", saved, err)
			}

		})
	}
}

func TestPaletteSettingsPrefix(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	s := v.paletteInteraction()
	if s == nil || v.commandSearch.Text() != "@" || !s.settingsMode || len(s.results) != 3 {
		t.Fatal("@ did not open settings-only palette")
	}
	for i, label := range []string{"Appearance › Auto color", "Application › Data folder", "View › System files"} {
		if got := s.results[i].command.label(); got != label {
			t.Fatalf("settings result %d = %q; want %q", i, got, label)
		}
	}
	for _, result := range s.results {
		if result.command.group == "Settings" || result.command.binding.Name != "" {
			t.Fatal("settings listed as browsing commands")
		}
	}
	send(key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "appearance autocolor"})
	if len(s.results) != 1 || s.results[0].command.label() != "Appearance › Auto color" {
		t.Fatal("settings search did not use existing fuzzy matching")
	}
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != nil || v.settingsInteraction() == nil || len(v.settingsInteraction().fields) != 3 {
		t.Fatal("palette did not open color widget")
	}
	if widget := v.settingsInteraction(); widget.title != "Appearance › Auto color" || widget.fields[0].Label != "Lightness" {
		t.Fatal("widget did not use its metadata title and option labels")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameEscape))
	send(pressed("`"))
	s = v.paletteInteraction()
	if s.settingsMode || len(s.results) != len(v.commandCatalogue()) {
		t.Fatal("ordinary palette included settings")
	}
	v.commandSearch.SetText("@")
	s.filter("@")
	v.commandSearch.SetText("")
	s.filter("")
	if s.settingsMode || len(s.results) != len(v.commandCatalogue()) {
		t.Fatal("removing prefix did not restore commands")
	}
}

func TestSettingsWidgetLivePreviewRevertAndCommit(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameRightArrow))
	if v.appearance.FilenameColors.filenameLightness != 0.37 {
		t.Fatal("adjustment did not preview live")
	}
	send(pressed(key.NameTab))
	send(pressed(key.NameRightArrow))
	if v.appearance.FilenameColors.filenameChroma != 0.36 {
		t.Fatal("Tab did not navigate controls")
	}
	send(pressed(key.NameEscape))
	if v.paletteInteraction() == nil || v.settingsValues.Overridden(settingLightness) || v.settingsValues.Overridden(settingChroma) || v.appearance.FilenameColors.filenameLightness != 0.36 {
		t.Fatal("Escape did not restore inherited original values")
	}
	send(pressed(key.NameEscape))
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameRightArrow))
	send(modified(key.NameReturn, key.ModCtrl))
	if !v.interaction.idle() || v.settingText(settingLightness) != "0.37" {
		t.Fatal("Ctrl+Enter did not commit and close")
	}
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameEscape))
	if v.settingText(settingLightness) != "0.37" || !v.settingsValues.Overridden(settingLightness) {
		t.Fatal("revert lost prior committed override")
	}
}

func TestSettingsWidgetDirectValueInput(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "2"})
	send(modified(key.NameReturn, key.ModCtrl))
	s := v.settingsInteraction()
	if s == nil || !s.editing || s.errorText == "" || v.settingsValues.Overridden(settingLightness) {
		t.Fatal("invalid number committed")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: "0.5"})
	send(modified(key.NameReturn, key.ModCtrl))
	if !v.interaction.idle() || v.appearance.FilenameColors.filenameLightness != 0.5 {
		t.Fatal("typed number did not save and close")
	}
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "0.2"})
	send(pressed(key.NameEscape))
	if v.paletteInteraction() == nil || v.settingText(settingLightness) != "0.5" {
		t.Fatal("Escape did not cancel unfinished draft")
	}
}

func TestSettingsPathPanelSelection(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@application"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil {
		t.Fatal("storage widget missing")
	}
	send(pressed(key.NameRightArrow))
	if !s.selecting || v.active != 1 || v.settingText(settingDataFolder) != v.panels[1].path {
		t.Fatal("arrow did not start live folder selection")
	}
	before := v.settingText(settingDataFolder)
	send(pressed(key.NameDownArrow))
	if v.settingText(settingDataFolder) != before {
		t.Fatal("highlighting child changed folder path")
	}
	send(pressed(key.NameRightArrow))
	if v.active != 2 || v.settingText(settingDataFolder) != v.panels[2].path {
		t.Fatal("second selector panel did not update path")
	}
	send(pressed(key.NameEscape))
	if s.selecting || v.settingsInteraction() != s || v.settingText(settingDataFolder) != v.panels[2].path {
		t.Fatal("Escape did not return to widget with draft")
	}
	send(pressed(key.NameEscape))
	if v.paletteInteraction() == nil || v.settingsValues.Overridden(settingDataFolder) {
		t.Fatal("second Escape did not revert path")
	}
	send(pressed(key.NameEscape))
	send(key.EditEvent{Text: "@application"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameUpArrow))
	if v.settingsInteraction().selecting {
		t.Fatal("Up unexpectedly started folder selection")
	}
	send(pressed(key.NameRightArrow))
	chosen := v.panels[v.active].path
	send(modified(key.NameReturn, key.ModCtrl))
	if !v.interaction.idle() || v.settingText(settingDataFolder) != chosen {
		t.Fatal("Ctrl+Enter from panels did not save")
	}
}

func TestSettingsPickerDirectoryReadsAndChildOwnership(t *testing.T) {
	v := newPreviewView()
	openSettingsGroup(t, v, "application.data_folder")
	s := v.settingsInteraction()
	s.selecting = true
	v.updateSettingsSelection()
	before := v.settingText(settingDataFolder)
	v.acceptLoad(loadResult{request: loadRequest{panel: 0, path: "/missing"}, err: errors.New("failed")})
	v.updateSettingsSelection()
	if v.settingText(settingDataFolder) != before {
		t.Fatal("failed read changed path")
	}
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1), results: make(chan loadResult, 1)}
	expected := filepath.Join(v.panels[0].path, v.panels[0].items[v.panels[0].selected].file.Name)
	v.activateSelected()
	select {
	case req := <-v.loader.requests:
		v.acceptLoad(loadResult{request: req, directory: browser.Directory{Path: expected}})
	default:
		t.Fatal("chooser blocked folder navigation")
	}
	v.updateSettingsSelection()
	if v.settingText(settingDataFolder) != expected {
		t.Fatal("successful navigation did not update path")
	}
	v.startLocation(layout.Context{})
	if !v.locationEditing() || v.settingsWidget() != s || v.actionAllowed(actionMutation) {
		t.Fatal("location editor lost widget or mutation protection")
	}
	if v.interaction.finish(s) {
		t.Fatal("stale parent ended child editor")
	}
	v.escape(layout.Context{})
	if v.settingsInteraction() != s || !s.selecting {
		t.Fatal("cancelling editor lost selection mode")
	}
	v.escape(layout.Context{})
	if s.selecting {
		t.Fatal("Escape did not return to widget")
	}
	v.panels[0].drives = true
	s.selecting = true
	v.updateSettingsSelection()
	if v.settingText(settingDataFolder) != expected {
		t.Fatal("drive screen supplied a path")
	}
}

func TestSettingsPathTextEditorKeepsCaretKeys(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@application"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Text: `D:\my folder`})
	send(pressed(key.NameLeftArrow))
	s := v.settingsInteraction()
	if s.selecting || v.active != 0 {
		t.Fatal("caret movement entered panels")
	}
	send(modified(key.NameReturn, key.ModCtrl))
	if v.settingText(settingDataFolder) != `D:\my folder` || !v.interaction.idle() {
		t.Fatal("typed path was not saved")
	}
}

func TestSettingsWidgetPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if directory == "" {
		t.Skip("optional headless widget preview")
	}
	v := newPreviewView()
	v.startPalette(layout.Context{})
	v.commandSearch.SetText("@")
	v.paletteInteraction().filter("@")
	if err := renderViewPNG(filepath.Join(directory, "settings-palette.png"), v); err != nil {
		t.Fatal(err)
	}
	v.stopPalette(layout.Context{})
	openSettingsGroup(t, v, "appearance.autocolor")
	if err := renderViewPNG(filepath.Join(directory, "settings-color-widget.png"), v); err != nil {
		t.Fatal(err)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	openSettingsGroup(t, v, "view.show_protected_system_files")
	if err := renderViewPNG(filepath.Join(directory, "settings-boolean-widget.png"), v); err != nil {
		t.Fatal(err)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	openSettingsGroup(t, v, "application.data_folder")
	s := v.settingsInteraction()
	s.selecting = true
	v.switchPanelTo(1)
	v.updateSettingsSelection()
	if err := renderViewPNG(filepath.Join(directory, "settings-file-widget.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsDockBlocksOnlyItsOwnPointerArea(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@application"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameRightArrow))
	before := v.panels[1].selected
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(400, 375)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(400, 375)})
	if v.panels[1].selected != before {
		t.Fatal("widget click reached underlying file row")
	}
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(300, 110)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(300, 110)})
	if v.panels[1].selected == before || !v.settingsSelecting() {
		t.Fatal("dock blocked ordinary panel selection")
	}
}

func TestSettingsSingleFieldNavigationStaysSilent(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@system"})
	send(pressed(key.NameReturn))
	if s := v.settingsInteraction(); s == nil || s.title != "View › System files" || s.fields[0].Label != "Show protected system entries" {
		t.Fatal("single-setting widget conflated its title and option label")
	}
	recorded := &recordedSounds{}
	v.sounds = recorded
	send(pressed(key.NameUpArrow))
	send(pressed(key.NameUpArrow))
	if len(recorded.played) != 0 {
		t.Fatal("unchanged field navigation played sound")
	}
}

func TestSettingsAdjustmentsTickOnlyWhenValueChanges(t *testing.T) {
	for _, tc := range []struct {
		name, query, field string
		initial            settings.Value
		keys               []key.Event
	}{
		{"slider", "@autocolor", settingLightness, settings.Value{Number: 1}, []key.Event{
			pressed(key.NameRightArrow), pressed(key.NameLeftArrow), pressed(key.NameRightArrow), pressed(key.NameRightArrow),
		}},
		{"boolean", "@system", settingProtectedSystem, settings.Value{Bool: false}, []key.Event{
			pressed(key.NameRightArrow), pressed(key.NameLeftArrow), pressed(key.NameLeftArrow), pressed(key.NameRightArrow),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newPreviewView()
			if err := v.settingsValues.Set(settings.CatalogueDefault, tc.field, tc.initial); err != nil {
				t.Fatal(err)
			}
			send, _ := panelInputFrames(v)
			send(key.EditEvent{Text: tc.query})
			send(pressed(key.NameReturn))
			recorded := &recordedSounds{}
			v.sounds = recorded
			for _, ke := range tc.keys {
				send(ke)
				ke.State = key.Release
				send(ke)
			}
			if len(recorded.played) != 2 {
				t.Fatalf("expected two changes to tick, got %v", recorded.played)
			}
			for _, cue := range recorded.played {
				if cue != sound.Navigation {
					t.Fatalf("adjustment cue: %v", cue)
				}
			}
		})
	}
}

func TestSettingsEscapeRestoresPalettePosition(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameDownArrow))
	send(pressed(key.NameDownArrow))
	source := v.paletteInteraction()
	selected, position, query := source.selected, source.list.Position, v.commandSearch.Text()
	send(pressed(key.NameReturn))
	send(pressed(key.NameEscape))
	if v.paletteInteraction() != source || source.selected != selected || source.list.Position != position || v.commandSearch.Text() != query {
		t.Fatal("Escape lost palette query, selection or viewport")
	}
	send(pressed(key.NameReturn))
	if v.settingsInteraction() == nil || v.settingsInteraction().fields[0].Key != settingProtectedSystem {
		t.Fatal("restored selection opened wrong setting")
	}
}

func TestSettingsResetToDefaultsIsReversible(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingDataFolder, settings.Value{Text: "chosen folder"})
	send(key.EditEvent{Text: "@application"})
	source := v.paletteInteraction()
	send(pressed(key.NameReturn))
	send(modified(key.NameTab, key.ModShift))
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || !s.actionsFocused || s.action != 2 || v.settingsValues.Overridden(settingDataFolder) {
		t.Fatal("Reset did not preview inherited default")
	}
	send(pressed(key.NameLeftArrow))
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != source || v.settingText(settingDataFolder) != "chosen folder" {
		t.Fatal("Cancel did not undo reset and restore palette")
	}
	send(pressed(key.NameReturn))
	send(modified(key.NameTab, key.ModShift))
	send(pressed(key.NameReturn))
	send(pressed(key.NameLeftArrow))
	send(pressed(key.NameLeftArrow))
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || v.settingsValues.Overridden(settingDataFolder) {
		t.Fatal("Save did not commit removal of override")
	}
}

func TestSettingsBooleanChoicesUseArrowDirection(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@system"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameLeftArrow))
	if !v.settingsValues.Get(settings.CatalogueDefault, settingProtectedSystem).Bool {
		t.Fatal("Left did not select ON")
	}
	send(pressed(key.NameLeftArrow))
	if !v.settingsValues.Get(settings.CatalogueDefault, settingProtectedSystem).Bool {
		t.Fatal("Left toggled away from ON")
	}
	send(pressed(key.NameRightArrow))
	if v.settingsValues.Get(settings.CatalogueDefault, settingProtectedSystem).Bool {
		t.Fatal("Right did not select OFF")
	}
	send(pressed(key.NameReturn))
	if !v.settingsValues.Get(settings.CatalogueDefault, settingProtectedSystem).Bool {
		t.Fatal("Enter did not toggle")
	}
}

func TestSettingsActionBarKeyboardFocus(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	send(pressed(key.NameTab))
	send(pressed(key.NameTab))
	if s.selected != 2 || s.actionsFocused {
		t.Fatal("Tab did not visit all fields")
	}
	send(pressed(key.NameTab))
	if !s.actionsFocused || s.action != 0 {
		t.Fatal("Tab did not reach Save")
	}
	recorded := &recordedSounds{}
	v.sounds = recorded
	send(pressed(key.NameLeftArrow))
	if s.action != 0 || len(recorded.played) != 0 {
		t.Fatal("left edge changed action or played cue")
	}
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameRightArrow))
	if s.action != 2 {
		t.Fatal("arrows did not reach Reset")
	}
	send(pressed(key.NameTab))
	if s.actionsFocused || s.selected != 0 {
		t.Fatal("Tab did not cycle back to first field")
	}
	send(modified(key.NameTab, key.ModShift))
	if !s.actionsFocused || s.action != 2 {
		t.Fatal("Shift+Tab did not cycle back to Reset")
	}
}

func TestSettingsActionBarMouse(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingDataFolder, settings.Value{Text: "chosen folder"})
	send(key.EditEvent{Text: "@application"})
	send(pressed(key.NameReturn))
	click := func(x float32) {
		send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, 405)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, 405)})
	}
	click(550)
	if v.settingsInteraction() == nil || v.settingsValues.Overridden(settingDataFolder) {
		t.Fatal("Reset click did not preview defaults")
	}
	click(400)
	if v.paletteInteraction() == nil || v.settingText(settingDataFolder) != "chosen folder" {
		t.Fatal("Cancel click did not undo reset")
	}
	send(pressed(key.NameReturn))
	click(550)
	click(240)
	if !v.interaction.idle() || v.settingsValues.Overridden(settingDataFolder) {
		t.Fatal("Save click did not commit reset")
	}
}

func TestSettingsSaveClickKeepsInvalidDraftEditable(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "2"})
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(240, 405)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(240, 405)})
	s := v.settingsInteraction()
	if s == nil || !s.editing || s.actionsFocused || s.errorText == "" {
		t.Fatal("failed Save did not return focus to invalid input")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: "0.5"})
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != s || s.editing || v.settingText(settingLightness) != "0.5" {
		t.Fatal("correcting input did not preview within widget")
	}
}
