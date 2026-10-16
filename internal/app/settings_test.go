package app

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/BurntSushi/toml"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestStartupSettingsApplyBeforeDirectoryResults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("appearance.autocolor.lsh.lightness = 50\nview.show_protected_system_files = true\n"), 0600); err != nil {
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
	if v.settingText(settingLightness) != "50" {
		t.Fatal("cancel did not restore the loaded override")
	}
}

func TestStartupSettingsErrorsSurviveInitialRead(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{
		{errors.Join(settings.ErrRepaired, settings.ErrUnknownKey), "Settings error (auto-fixed)"},
		{errors.Join(settings.ErrRepairFailed, os.ErrPermission), "Settings repair failed"},
		{settings.ErrSyntax, "Settings: invalid TOML"},
		{settings.ErrUnknownKey, "Settings: unknown key"},
		{settings.ErrType, "Settings: invalid value type"},
		{settings.ErrRange, "Settings: value outside range"},
		{settings.ErrValue, "Settings: invalid value"},
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
		if v.panels[0].feedback.Current() != nil || v.settingText(settingLightness) != "36" {
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
			if v.settingsInteraction() != s || s.errorText != "Settings save: access denied" || v.settingText(settingLightness) != "37" {
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
			} else if calls != 1 || v.settingsValues.Overridden(settingLightness) || v.settingText(settingLightness) != "36" {
				t.Fatal("cancel saved or retained failed preview")
			}
			var saved struct {
				Appearance struct {
					Autocolor struct{ Lsh struct{ Lightness float64 } }
				}
			}
			md, err := toml.DecodeFile(path, &saved)
			if err != nil || md.IsDefined("appearance", "autocolor", "lsh", "lightness") != retry || retry && saved.Appearance.Autocolor.Lsh.Lightness != 37 {
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
	if s == nil || v.commandSearch.Text() != "@" || !s.settingsMode || len(s.results) != 14 {
		t.Fatal("@ did not open settings-only palette")
	}
	for i, label := range []string{"Appearance › Auto color", "Appearance › Auto color › Selection", "Appearance › Auto color › Special files", "Appearance › Font", "Appearance › Padding › Horizontal", "Appearance › Padding › Vertical", "Appearance › Theme › Customize", "Appearance › Theme › Generator", "Appearance › Theme › Select", "Application › Data folder", "Feedback › Notifications", "Feedback › Sounds", "Programs › Editors › New", "View › System files"} {
		if got := s.results[i].command.label(); got != label {
			t.Fatalf("settings result %d = %q; want %q", i, got, label)
		}
	}
	for _, result := range s.results {
		if result.command.group == "Settings" || result.command.shortcut() != "" {
			t.Fatal("settings listed as browsing commands")
		}
	}
	send(key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "appearance autocolor"})
	if len(s.results) != 3 || s.results[0].command.label() != "Appearance › Auto color" {
		t.Fatal("settings search did not use existing fuzzy matching")
	}
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != nil || v.settingsInteraction() == nil || len(v.settingsInteraction().fields) != 4 {
		t.Fatal("palette did not open color widget")
	}
	if widget := v.settingsInteraction(); widget.title != "Appearance › Auto color" || widget.fields[0].Label != "Enabled" {
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
	send(pressed(key.NameTab))
	send(pressed(key.NameRightArrow))
	if v.appearance.FilenameColors.filenameLightness != 0.37 {
		t.Fatal("adjustment did not preview live")
	}
	send(pressed(key.NameTab))
	send(pressed(key.NameLeftArrow))
	if v.appearance.FilenameColors.filenameSaturation != 0.99 {
		t.Fatal("Tab did not navigate controls")
	}
	v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 1
	send(pressed(key.NameReturn))
	if v.paletteInteraction() == nil || v.settingsValues.Overridden(settingLightness) || v.settingsValues.Overridden(settingSaturation) || v.appearance.FilenameColors.filenameLightness != 0.36 {
		t.Fatal("Cancel did not restore inherited original values")
	}
	send(pressed(key.NameEscape))
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameTab))
	send(pressed(key.NameRightArrow))
	send(modified(key.NameReturn, key.ModCtrl))
	if !v.interaction.idle() || v.settingText(settingLightness) != "37" {
		t.Fatal("Ctrl+Enter did not commit and close")
	}
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameTab))
	send(pressed(key.NameRightArrow))
	v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 1
	send(pressed(key.NameReturn))
	if v.settingText(settingLightness) != "37" || !v.settingsValues.Overridden(settingLightness) {
		t.Fatal("revert lost prior committed override")
	}
}

func TestSaltStepperAndWholeNumberInput(t *testing.T) {
	v := newPreviewView()
	openSettingsGroup(t, v, "appearance.autocolor")
	v.settingsInteraction().selected = 3
	send, _ := panelInputFrames(v)
	send(pressed(key.NameLeftArrow))
	if v.appearance.FilenameColors.salt != 0 {
		t.Fatal("salt stepped below zero")
	}
	send(pressed(key.NameRightArrow))
	if v.appearance.FilenameColors.salt != 1 {
		t.Fatal("salt did not preview one integer step")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: "0.5"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if !s.editing || s.errorText != "Enter a whole number" || v.appearance.FilenameColors.salt != 1 {
		t.Fatal("fractional salt changed the preview or left the editor")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "1000"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameRightArrow))
	if v.appearance.FilenameColors.salt != 1001 {
		t.Fatal("salt did not accept and step beyond 360")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.FilenameColors.salt != 0 {
		t.Fatal("Cancel did not restore the original salt")
	}
}

func TestSettingsWidgetDirectValueInput(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameTab))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "200"})
	send(modified(key.NameReturn, key.ModCtrl))
	s := v.settingsInteraction()
	if s == nil || !s.editing || s.errorText == "" || v.settingsValues.Overridden(settingLightness) {
		t.Fatal("invalid number committed")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "50"})
	send(modified(key.NameReturn, key.ModCtrl))
	if !v.interaction.idle() || v.appearance.FilenameColors.filenameLightness != 0.5 {
		t.Fatal("typed number did not save and close")
	}
	send(key.EditEvent{Text: "@"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameTab))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "20"})
	send(pressed(key.NameEscape))
	if v.settingsInteraction() == nil || v.settingsInteraction().editing || v.paletteInteraction() != nil || v.settingText(settingLightness) != "50" {
		t.Fatal("Escape did not leave the input and discard unfinished text")
	}
	send(pressed(key.NameEscape))
	if v.settingsInteraction() == nil {
		t.Fatal("second Escape closed a dirty widget")
	}
	v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 1
	send(pressed(key.NameReturn))
	if v.paletteInteraction() == nil {
		t.Fatal("Cancel did not return to the palette")
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
	v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 1
	send(pressed(key.NameReturn))
	if v.paletteInteraction() == nil || v.settingsValues.Overridden(settingDataFolder) {
		t.Fatal("Cancel did not revert path")
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
	v.settingsInteraction().selected = 1
	if err := renderViewPNG(filepath.Join(directory, "settings-color-widget-unfocused-toggle.png"), v); err != nil {
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
		{"slider", "@autocolor", settingLightness, settings.Value{Number: 100}, []key.Event{
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
			if tc.field == settingLightness {
				send(pressed(key.NameTab))
			}
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
	send(pressed(key.NameEnd))
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
	if s.selected != 3 || s.actionsFocused {
		t.Fatal("Tab did not visit Salt")
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
	send(pressed(key.NameTab))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "200"})
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(240, 405)}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(240, 405)})
	s := v.settingsInteraction()
	if s == nil || !s.editing || s.actionsFocused || s.errorText == "" {
		t.Fatal("failed Save did not return focus to invalid input")
	}
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "50"})
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != s || s.editing || v.settingText(settingLightness) != "50" {
		t.Fatal("correcting input did not preview within widget")
	}
}

func TestSettingsEscapeLeavesInputBeforeWidget(t *testing.T) {
	for _, tc := range []struct {
		group string
		field int
		text  string
	}{
		{"appearance.autocolor", 1, "invalid number"},
		{"application.data_folder", 0, "unfinished folder"},
		{settings.EditorCollection, 0, "*.txt"},
		{settings.EditorCollection, 1, "unfinished program"},
		{settings.EditorCollection, 2, "unfinished arguments"},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.group, tc.field), func(t *testing.T) {
			v := newPreviewView()
			send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			if tc.group == settings.EditorCollection {
				v.startCollectionWidget(tc.group, -1, "New")
			} else {
				v.startSettingsWidget(tc.group)
			}
			s := v.settingsInteraction()
			s.selected = tc.field
			field := s.fields[s.selected].Key
			original := v.settingText(field)
			send(pressed(key.NameReturn))
			send(key.EditEvent{Range: key.Range{End: len([]rune(original))}, Text: tc.text})
			s.errorText = "Draft error"
			recorded := &recordedSounds{}
			v.sounds = recorded
			send(pressed(key.NameEscape))
			if v.settingsInteraction() != s || s.editing || s.errorText != "" || v.settingText(field) != original || !router.Source().Focused(&v.focus) {
				t.Fatal("Escape lost widget, retained draft, or failed to restore focus")
			}
			if len(recorded.played) != 1 || recorded.played[0] != sound.Cancel {
				t.Fatal("Escape did not play one cancellation cue")
			}
			send(key.Event{Name: key.NameEscape, State: key.Release})
			if len(recorded.played) != 1 {
				t.Fatal("Escape release played a cue")
			}
			send(pressed(key.NameReturn))
			if !s.editing || s.editor.Text() != original {
				t.Fatal("reopening input retained discarded text")
			}
			send(pressed(key.NameEscape))
			send(pressed(key.NameEscape))
			if v.settingsInteraction() != s || !s.dirty {
				t.Fatal("second Escape closed dirty widget")
			}
			v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 1
			send(pressed(key.NameReturn))
			if !v.interaction.idle() {
				t.Fatal("Cancel did not close dirty widget")
			}
		})
	}
}

func TestSettingsCancelActionClosesWhileEditing(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	v.startSettingsWidget("appearance.autocolor")
	send(pressed(key.NameTab))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: 4}, Text: "80"})
	v.activateSettingsAction(layout.Context{}, 1)
	if !v.interaction.idle() || v.settingText(settingLightness) != "36" {
		t.Fatal("Cancel only left editing instead of cancelling widget")
	}
}

func TestDirtySettingsEscapeWarnsAndFocusesCancel(t *testing.T) {
	for _, kind := range []string{"adjust", "toggle", "reset", "delete"} {
		t.Run(kind, func(t *testing.T) {
			v := newPreviewView()
			send, _ := panelInputFrames(v)
			switch kind {
			case "adjust":
				v.startSettingsWidget("appearance.autocolor")
				v.adjustSetting(settingLightness, 1)
				v.adjustSetting(settingLightness, -1)
			case "toggle":
				v.startSettingsWidget("view.show_protected_system_files")
				v.adjustSetting(settingProtectedSystem, 0)
			case "reset":
				_ = v.settingsValues.Set(settings.CatalogueDefault, settingDataFolder, settings.Value{Text: "chosen"})
				v.startSettingsWidget("application.data_folder")
				v.activateSettingsAction(layout.Context{}, 2)
			case "delete":
				setEditorRule(t, v, "txt", "editor.exe")
				v.startCollectionWidget(settings.EditorCollection, 0, "txt")
				v.activateSettingsAction(layout.Context{}, 2)
			}
			s := v.settingsInteraction()
			if !s.dirty {
				t.Fatal("change did not mark widget dirty")
			}
			recorded := &recordedSounds{}
			v.sounds = recorded
			send(pressed(key.NameEscape))
			if v.settingsInteraction() != s || !s.actionsFocused || s.action != 1 || len(recorded.played) != 1 || recorded.played[0] != sound.MessageWarning {
				t.Fatal("dirty Escape did not preserve widget, warn once, and focus Cancel")
			}
			send(key.Event{Name: key.NameEscape, State: key.Release})
			if len(recorded.played) != 1 {
				t.Fatal("Escape release repeated the warning")
			}
			send(pressed(key.NameReturn))
			if !v.interaction.idle() {
				t.Fatal("Cancel did not discard dirty widget")
			}
		})
	}
}

func TestUnchangedWidgetAndInputCanCloseWithEscape(t *testing.T) {
	for _, edit := range []bool{false, true} {
		v := newPreviewView()
		send, _ := panelInputFrames(v)
		v.startSettingsWidget("appearance.autocolor")
		s := v.settingsInteraction()
		s.selected = 1
		if edit {
			send(pressed(key.NameReturn))
			send(pressed(key.NameEscape))
			if v.settingsInteraction() != s || s.editing || s.dirty {
				t.Fatal("entering and leaving unchanged input made widget dirty")
			}
		}
		send(pressed(key.NameEscape))
		if !v.interaction.idle() {
			t.Fatal("Escape did not close unchanged widget")
		}
	}
}

func TestDirtySettingsFailedSaveStillNeedsExplicitCancel(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	v.startSettingsWidget("appearance.autocolor")
	v.adjustSetting(settingLightness, 1)
	v.prepareSettingsSave = func(settings.Values) func() (string, error) {
		return func() (string, error) { return "", os.ErrPermission }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	s := v.settingsInteraction()
	send(pressed(key.NameEscape))
	if v.settingsInteraction() != s || s.errorText != "Settings save: access denied" {
		t.Fatal("Escape discarded failed save or its error")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if !v.interaction.idle() || v.settingText(settingLightness) != "36" {
		t.Fatal("Cancel failed after save error")
	}
}

func TestAutoColorEnabledLiveCancelAndPersistence(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingLightness, settings.Value{Number: 50})
	v.applySettings()
	p := &v.panels[v.active]
	selected, marked, position := p.selected, len(p.marks), p.list.Position
	if !v.appearance.FilenameColors.enabled {
		t.Fatal("auto color did not default to enabled")
	}
	send, _ := panelInputFrames(v)
	v.startSettingsWidget("appearance.autocolor")
	s := v.settingsInteraction()
	for i, field := range s.fields {
		if field.Key == settingAutoColorEnabled {
			s.selected = i
		}
	}
	send(pressed(key.NameSpace))
	if v.appearance.FilenameColors.enabled || !s.dirty || p.selected != selected || len(p.marks) != marked || p.list.Position.First != position.First {
		t.Fatal("toggle did not apply live or changed listing state")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if !v.appearance.FilenameColors.enabled || v.settingsValues.Overridden(settingAutoColorEnabled) {
		t.Fatal("Cancel did not restore enabled default")
	}
	v.startSettingsWidget("appearance.autocolor")
	v.adjustSetting(settingAutoColorEnabled, 0)
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || loaded.Get(settings.CatalogueDefault, settingAutoColorEnabled).Bool || loaded.Get(settings.CatalogueDefault, settingLightness).Number != 50 {
		t.Fatalf("saved toggle/color preferences = %#v, %v", loaded, err)
	}
	restored := newPreviewView()
	restored.restoreSettings(loaded, nil)
	if restored.appearance.FilenameColors.enabled {
		t.Fatal("startup did not apply disabled auto color")
	}
	restored.startSettingsWidget("appearance.autocolor")
	restored.activateSettingsAction(layout.Context{}, 2)
	if !restored.appearance.FilenameColors.enabled {
		t.Fatal("Reset did not restore enabled default")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.appearance.FilenameColors.enabled {
		t.Fatal("Cancel did not undo Reset")
	}
}

func TestAutoColorDisabledPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional disabled auto color preview")
	}
	v := newPreviewView()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingAutoColorEnabled, settings.Value{Bool: false})
	v.applySettings()
	v.startSettingsWidget("appearance.autocolor")
	if err := renderViewPNG(filepath.Join(dir, "settings-autocolor-disabled.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestCutLightnessSettingsPreviewCancelAndPersistence(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget("appearance.autocolor.selection")
	s := v.settingsInteraction()
	if len(s.fields) != 2 || s.fields[0].Label != "Background lightness offset" || s.fields[1].Label != "Cut entries lightness offset" {
		t.Fatal("Selection does not expose the two lightness offsets")
	}
	v.adjustSetting(settingCutLightness, 1)
	if v.appearance.FilenameColors.cutLightnessOffset != 0.21 || !s.dirty {
		t.Fatal("cut offset did not preview")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.FilenameColors.cutLightnessOffset != 0.20 {
		t.Fatal("Cancel did not restore cut offset")
	}
	_ = v.setWidgetValue(settingAutoColorEnabled, settings.Value{Bool: false})
	v.startSettingsWidget("appearance.autocolor.selection")
	_ = v.setWidgetValue(settingCutLightness, settings.Value{Number: -10})
	v.applySettings()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.appearance.FilenameColors.enabled || restored.appearance.FilenameColors.cutLightnessOffset != -0.10 {
		t.Fatal("disabled Auto color and cut offset did not persist independently")
	}
	_, normal, _ := restored.appearance.entryColors(browser.Entry{Name: "notes.txt"}, false, false)
	_, cut, _ := restored.appearance.entryColors(browser.Entry{Name: "notes.txt"}, false, true)
	if cut == normal {
		t.Fatal("cut offset stopped working with Auto color disabled")
	}
}

func TestSettingsCtrlArrowsUseLargerNumericSteps(t *testing.T) {
	for _, tc := range []struct {
		name, widget, key    string
		row                  int
		initial, right, left float64
	}{
		{"points", "appearance.autocolor", settingLightness, 1, 36, 46, 36},
		{"unbounded", "appearance.autocolor", settingSalt, 3, 0, 10, 0},
		{"font size", "appearance.font", settingFontTextSize, 1, 15, 25, 15},
		{"upper limit", colorPrefix + "background", colorPrefix + "background.lightness", 1, 98, 100, 90},
		{"hue wrap", colorPrefix + "background", colorPrefix + "background.hue", 3, 355, 5, 355},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newPreviewView()
			_ = v.settingsValues.Set(settings.CatalogueDefault, tc.key, settings.Value{Number: tc.initial})
			v.startSettingsWidget(tc.widget)
			s := v.settingsInteraction()
			s.selected = tc.row
			send, _ := panelInputFrames(v)
			send(modified(key.NameRightArrow, key.ModCtrl))
			if got := v.settingsValues.Get(settings.CatalogueDefault, tc.key).Number; got != tc.right || !s.dirty {
				t.Fatalf("Ctrl+Right = %g; want %g", got, tc.right)
			}
			send(modified(key.NameLeftArrow, key.ModCtrl))
			if got := v.settingsValues.Get(settings.CatalogueDefault, tc.key).Number; got != tc.left {
				t.Fatalf("Ctrl+Left = %g; want %g", got, tc.left)
			}
		})
	}
}

func TestSettingsCtrlArrowsKeepTextEditingAndChoices(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	send, _ := panelInputFrames(v)
	s := v.settingsInteraction()
	send(modified(key.NameRightArrow, key.ModCtrl))
	if v.settingsInteraction() != s || themeElementKey(s) != colorPrefix+"background" || s.dirty {
		t.Fatal("Ctrl+Right cycled Element")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameReturn))
	text := s.editor.Text()
	s.editor.SetCaret(len(text), len(text))
	send()
	send(modified(key.NameLeftArrow, key.ModCtrl))
	start, end := s.editor.Selection()
	if !s.editing || s.editor.Text() != text || start >= len(text) || start != end || s.dirty {
		t.Fatal("Ctrl+Left did not retain native text editing")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	v.startSettingsWidget("appearance.autocolor")
	s = v.settingsInteraction()
	send(modified(key.NameLeftArrow, key.ModCtrl))
	if s.dirty || !v.settingsValues.Get(settings.CatalogueDefault, settingAutoColorEnabled).Bool {
		t.Fatal("Ctrl+Left changed a toggle")
	}
}
