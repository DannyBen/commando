package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestThemeFileSnapshotContainsDefaultsAndExcludesPreferences(t *testing.T) {
	values := settings.Values{}
	_ = values.Set(settings.CatalogueDefault, settingContrastGuard, settings.Value{Bool: false})
	_ = values.Set(settings.CatalogueDefault, settingPaddingRows, settings.Value{Number: 5})
	_ = values.Set(settings.CatalogueDefault, settingThemeGenerator+".seed", settings.Value{Number: 99})
	data, err := encodeThemeSnapshot(values)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseThemeSnapshot(data)
	if err != nil || !(themeSnapshot{values: parsed}).matches(values) {
		t.Fatal("theme file did not preserve the complete palette", err)
	}
	for _, key := range []string{settingContrastGuard, settingPaddingRows, settingThemeGenerator + ".seed"} {
		if parsed.Overridden(key) {
			t.Fatal("theme included an application preference", key)
		}
	}
	for _, key := range []string{settingAutoColorEnabled, settingSalt, settingHiddenSaturation, settingDotfileSaturation} {
		if !parsed.Overridden(key) {
			t.Fatal("snapshot omitted a default-valued color setting", key)
		}
	}
}

func TestThemeSaveUsesDataFolderDraftAndCollisionRetry(t *testing.T) {
	v, _, other := clipboardView(t)
	v.active, v.activeDataFolder = 1, resolvedTempDir(t)
	v.settingsValues = builtInThemes[1].values.Clone()
	v.applySettings()
	captured := themeSnapshot{values: v.settingsValues.Clone()}
	router, send := clipboardFrames(v)
	found := false
	for _, cmd := range v.settingsCatalogue() {
		if cmd.label() == "Appearance › Theme › Save" {
			if !cmd.allowed(v) {
				t.Fatal("theme save unavailable")
			}
			cmd.execute(v)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing theme save command")
	}
	waitLoaded(t, v, 1)
	send()
	directory := filepath.Join(v.activeDataFolder, "themes")
	if v.panels[1].path != directory || v.nameInteraction() == nil || v.nameEditor.Text() != "theme.toml" || v.nameEditor.SelectedText() != "theme" || !router.Source().Focused(&v.nameEditor) {
		t.Fatal("Save did not navigate and offer the selected default name")
	}
	path := filepath.Join(directory, "theme.toml")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("theme written before confirmation")
	}
	if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	// The contents belong to the captured draft, independently of later settings.
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingLightness, settings.Value{Number: 12})
	send(pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	if n := v.nameInteraction(); n == nil || n.pending || !errors.Is(n.err, os.ErrExist) || v.nameEditor.Text() != "theme.toml" {
		t.Fatal("collision did not retain the editable name")
	}
	if data, _ := os.ReadFile(path); string(data) != "existing" {
		t.Fatal("Save overwrote the existing theme")
	}
	send(replaceCreate(v, "Midnight.toml"), pressed(key.NameReturn))
	waitCreated(t, v)
	send()
	data, err := os.ReadFile(filepath.Join(directory, "Midnight.toml"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseThemeSnapshot(data)
	if err != nil || !captured.matches(parsed) {
		t.Fatal("retry lost the captured theme", err)
	}
	p := &v.panels[1]
	if v.nameInteraction() != nil || p.items[p.selected].name != "Midnight.toml" || v.mutationsBusy() || v.transferPending || other == p.path {
		t.Fatal("creation did not reconcile the selected file")
	}
	// A second save still proposes theme.toml despite its existing collision.
	v.saveThemeFile()
	waitLoaded(t, v, 1)
	send()
	if v.nameEditor.Text() != "theme.toml" {
		t.Fatal("Save generated a numbered filename")
	}
	send(pressed(key.NameEscape))
	if v.nameInteraction() != nil {
		t.Fatal("Escape did not cancel the draft")
	}
	if data, _ := os.ReadFile(path); string(data) != "existing" {
		t.Fatal("cancellation modified the existing file")
	}
}

func TestThemeSaveFolderFailurePreservesListing(t *testing.T) {
	v, root, _ := clipboardView(t)
	v.activeDataFolder = resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(v.activeDataFolder, "themes"), []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	before := slices.Clone(v.panels[0].items)
	v.saveThemeFile()
	waitLoaded(t, v, 0)
	if v.panels[0].path != root || !reflect.DeepEqual(before, v.panels[0].items) || v.nameInteraction() != nil || v.pendingNameDraft != nil || v.mutationsBusy() {
		t.Fatal("folder failure changed the listing or left an unfinished save")
	}
	if message := v.panels[0].feedback.Current(); message == nil {
		t.Fatal("folder failure was silent")
	}
}

func waitThemeLibrary(t *testing.T, v *view) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for s := v.settingsInteraction(); s != nil && s.pending; s = v.settingsInteraction() {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("theme library did not finish loading")
		}
	}
}

func TestThemeSelectLoadsFilesCyclesAndSavesIndependentSnapshot(t *testing.T) {
	v, _, _ := clipboardView(t)
	v.activeDataFolder = resolvedTempDir(t)
	directory := filepath.Join(v.activeDataFolder, "themes")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	custom := builtInThemes[1].values.Clone()
	_ = custom.Set(settings.CatalogueDefault, settingLightness, settings.Value{Number: 62.1234})
	data, err := encodeThemeSnapshot(custom)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "Midnight.toml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(v.activeDataFolder, values) }
	}
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingContrastGuard, settings.Value{Bool: false})
	before := v.settingsValues.Clone()
	v.startSettingsWidget(settingThemeSelect)
	waitThemeLibrary(t, v)
	if got := v.settingsInteraction().completion.candidates; !reflect.DeepEqual(got, []string{"Built-in Light", "Built-in Dark", "Midnight"}) {
		t.Fatal("theme file missing or extension displayed", got)
	}
	for range 3 {
		v.adjustSetting(settingThemeSelect, 1)
	}
	if v.settingText(settingThemeSelect) != "Midnight" || !(themeSnapshot{values: custom}).matches(v.settingsValues) || v.settingsValues.Get(settings.CatalogueDefault, settingContrastGuard).Bool {
		t.Fatal("file did not preview while retaining preferences")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if !reflect.DeepEqual(v.settingsValues, before) {
		t.Fatal("Cancel did not restore original settings")
	}
	v.startSettingsWidget(settingThemeSelect)
	waitThemeLibrary(t, v)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: len(v.settingsInteraction().editor.Text())}, Text: "Mid"}, key.SelectionEvent{Start: 3, End: 3})
	if v.settingsInteraction().completion.suffix() != "night" {
		t.Fatal("theme filename did not autocomplete")
	}
	send(pressed(key.NameTab), pressed(key.NameReturn))
	v.finishSettingsWidget(layout.Context{}, true)
	waitThemeLibrary(t, v)
	loaded, err := settings.Load(v.activeDataFolder, settings.CatalogueDefault)
	if err != nil || !(themeSnapshot{values: custom}).matches(loaded) {
		t.Fatal("selected file palette was not persisted", err)
	}
	if saved, _ := os.ReadFile(path); !bytes.Equal(saved, data) {
		t.Fatal("selecting a theme modified its source file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(loaded, nil)
	if !(themeSnapshot{values: custom}).matches(v.settingsValues) {
		t.Fatal("theme depended on its deleted source")
	}
	v.startSettingsWidget(settingThemeSelect)
	waitThemeLibrary(t, v)
	if slices.Contains(v.settingsInteraction().completion.candidates, "Midnight") {
		t.Fatal("selector retained a deleted source")
	}
}

func TestThemeLibraryMissingDirectoryAndInvalidFiles(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "themes")
	themes, err := readThemeLibrary(directory)
	if err != nil || len(themes) != 2 {
		t.Fatal("missing theme folder prevented built-in selection", err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Select created a theme folder")
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	valid, err := encodeThemeSnapshot(builtInThemes[0].values)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"Shared.TOML": valid, "broken.toml": []byte("invalid = ["), "partial.toml": []byte("[appearance.autocolor]\nenabled = true"), "notes.txt": []byte("ignore")} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	themes, err = readThemeLibrary(directory)
	if err == nil || len(themes) != 3 || themes[2].name != "Shared" {
		t.Fatal("invalid files hid valid themes or entered the selector", err)
	}
}

func TestThemeLibraryNamesAndStaleCompletion(t *testing.T) {
	directory := t.TempDir()
	data, err := encodeThemeSnapshot(builtInThemes[1].values)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Built-in Light.toml", "Current theme.toml"} {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	themes, err := readThemeLibrary(directory)
	if err != nil {
		t.Fatal(err)
	}
	v := newPreviewView()
	v.startSettingsWidget(settingThemeSelect)
	s := v.settingsInteraction()
	v.acceptThemeLibrary(s, themes, nil)
	for _, name := range []string{"Built-in Light (built-in)", "Built-in Light", "Current theme"} {
		if !slices.Contains(s.completion.candidates, name) {
			t.Fatal("theme filename collided with a built-in label", name)
		}
	}
	if err := v.selectTheme("Current theme"); err != nil || !builtInThemes[1].matches(v.settingsValues) {
		t.Fatal("reserved display label prevented selecting a theme file", err)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	v.startSettingsWidget(settingThemeSelect)
	current := v.settingsInteraction()
	v.acceptThemeLibrary(s, themes, errors.New("late failure"))
	if current.pending || current.errorText != "" || len(current.completion.candidates) != 2 {
		t.Fatal("stale library result changed a newer widget")
	}
}

func TestThemeFilePreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional theme file previews")
	}
	for _, mode := range []string{"save", "select"} {
		v, _, _ := clipboardView(t)
		v.activeDataFolder = resolvedTempDir(t)
		v.settingsValues = builtInThemes[1].values.Clone()
		v.applySettings()
		if mode == "save" {
			v.saveThemeFile()
			waitLoaded(t, v, v.active)
			v.offerNameDraft(layout.Context{})
		} else {
			directory := filepath.Join(v.activeDataFolder, "themes")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			data, err := encodeThemeSnapshot(v.settingsValues)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "Midnight.toml"), data, 0600); err != nil {
				t.Fatal(err)
			}
			v.startSettingsWidget(settingThemeSelect)
			waitThemeLibrary(t, v)
			if err := v.selectTheme("Midnight"); err != nil {
				t.Fatal(err)
			}
			v.applySettings()
		}
		if err := renderViewPNG(filepath.Join(dir, "theme-file-"+mode+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
