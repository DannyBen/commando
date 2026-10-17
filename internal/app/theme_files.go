package app

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

const settingThemeSave = "appearance.theme.save"

var themeSaveWidget = settings.Widget{Key: settingThemeSave, Parent: "appearance.theme", Label: "Save", Visible: true}

// Theme documents are complete snapshots, including values equal to defaults.
// Settings persistence deliberately omits defaults and cannot be used here.
func encodeThemeSnapshot(values settings.Values) ([]byte, error) {
	document := map[string]any{}
	c := settings.CatalogueDefault
	for _, d := range c.Settings {
		if !themeSnapshotKey(d.Key) {
			continue
		}
		parts := strings.Split(d.Key, ".")
		table := document
		for _, part := range parts[:len(parts)-1] {
			if table[part] == nil {
				table[part] = map[string]any{}
			}
			table = table[part].(map[string]any)
		}
		value := values.Get(c, d.Key)
		var scalar any = value.Bool
		if d.Kind == settings.Number {
			scalar = value.Number
			if math.Trunc(value.Number) == value.Number && math.Abs(value.Number) <= 1<<53-1 {
				scalar = int64(value.Number)
			}
		}
		table[parts[len(parts)-1]] = scalar
	}
	var data bytes.Buffer
	encoder := toml.NewEncoder(&data)
	encoder.Indent = ""
	err := encoder.Encode(document)
	return data.Bytes(), err
}

func (v *view) saveThemeFile() {
	if v.activeDataFolder == "" || v.loader == nil || !v.canStartInteraction(v.active, actionMutation) {
		return
	}
	data, err := encodeThemeSnapshot(v.settingsValues)
	if err != nil {
		v.showFeedback(v.active, feedback.Error, "Cannot save theme")
		return
	}
	request := loadRequest{panel: v.active, path: filepath.Join(v.activeDataFolder, "themes"),
		fileDraft: &fileDraftRequest{name: "theme.toml", data: data}}
	if v.loader.submit(request) {
		p := &v.panels[v.active]
		p.loading, p.navigating, p.err = true, true, nil
	}
}

// Directory and file reads run on the shared worker. Unusable files never
// replace a palette; other readable themes remain available alongside built-ins.
func readThemeLibrary(directory string) ([]themeSnapshot, error) {
	themes := slices.Clone(builtInThemes)
	files, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return themes, nil
	}
	if err != nil {
		return themes, err
	}
	var problems error
	for _, file := range files {
		if file.IsDir() || !strings.EqualFold(filepath.Ext(file.Name()), ".toml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, file.Name()))
		var values settings.Values
		if err == nil {
			values, err = parseThemeSnapshot(data)
		}
		if err != nil {
			problems = errors.Join(problems, err)
			continue
		}
		name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		themes = append(themes, themeSnapshot{name: name, values: values})
	}
	// Preserve filenames as labels even when they coincide with built-in names.
	for i := range builtInThemes {
		for _, theme := range themes[len(builtInThemes):] {
			if strings.EqualFold(themes[i].name, theme.name) {
				themes[i].name += " (built-in)"
				break
			}
		}
	}
	return themes, problems
}

func (v *view) loadThemeLibrary(s *settingsSession) {
	if v.activeDataFolder == "" || v.loader == nil {
		return
	}
	s.themesLoading = v.loader.submit(loadRequest{panel: s.panel, path: filepath.Join(v.activeDataFolder, "themes"),
		themeLibrary: s, action: actionBrowse})
	if !s.themesLoading {
		s.errorText = "Cannot read themes"
	}
}

func (v *view) acceptThemeLibrary(s *settingsSession, themes []themeSnapshot, err error) {
	if v.settingsInteraction() != s {
		return
	}
	s.themesLoading = false
	if s.editing {
		s.themeLibraryResult = &loadResult{themes: themes, err: err}
		return
	}
	s.themes, s.completion.candidates = themes, nil
	s.themeName = ""
	for _, theme := range themes {
		s.completion.candidates = append(s.completion.candidates, theme.name)
		if s.themeName == "" && theme.matches(v.settingsValues) {
			s.themeName = theme.name
		}
	}
	if err != nil {
		s.errorText = "Some themes could not load"
	}
}

func (v *view) offerThemeLibrary(s *settingsSession) {
	if v.settingsInteraction() != s || s.editing || s.pending || s.themeLibraryResult == nil {
		return
	}
	result := s.themeLibraryResult
	s.themeLibraryResult = nil
	v.acceptThemeLibrary(s, result.themes, result.err)
}
