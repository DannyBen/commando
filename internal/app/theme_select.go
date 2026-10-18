package app

import (
	"embed"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/dannyben/commando/internal/settings"
)

const settingThemeSelect = "appearance.theme.select"

var themeSelectWidget = settings.Widget{Key: settingThemeSelect, Parent: "appearance.theme", Label: "Select", Title: "Select", Visible: true}

//go:embed themes/*.toml
var themeFiles embed.FS

// Snapshots are sources for settings values, never persistent theme identities.
type themeSnapshot struct {
	name   string
	values settings.Values
}

var builtInThemes = mustBuiltInThemes()

func themeSnapshotKey(key string) bool {
	return strings.HasPrefix(key, themeSettingPrefix) || strings.HasPrefix(key, "appearance.autocolor.")
}

func parseThemeSnapshot(data []byte) (settings.Values, error) {
	c := settings.CatalogueDefault
	values, err := settings.Parse(data, c)
	if err != nil {
		return settings.Values{}, err
	}
	for _, d := range c.Settings {
		included := themeSnapshotKey(d.Key)
		if included != values.Overridden(d.Key) {
			return settings.Values{}, fmt.Errorf("invalid theme setting: %s", d.Key)
		}
	}
	for _, key := range c.Collections {
		if len(values.Collection(key)) != 0 {
			return settings.Values{}, fmt.Errorf("unexpected theme collection: %s", key)
		}
	}
	return values, nil
}

func mustBuiltInThemes() []themeSnapshot {
	var themes []themeSnapshot
	for _, spec := range []struct{ name, file string }{{"Built-in Light", "light.toml"}, {"Built-in Dark", "dark.toml"}} {
		data, err := themeFiles.ReadFile("themes/" + spec.file)
		if err != nil {
			panic(err)
		}
		values, err := parseThemeSnapshot(data)
		if err != nil {
			panic(fmt.Errorf("%s: %w", spec.name, err))
		}
		themes = append(themes, themeSnapshot{spec.name, values})
	}
	return themes
}

func (t themeSnapshot) matches(values settings.Values) bool {
	for _, d := range settings.CatalogueDefault.Settings {
		if !themeSnapshotKey(d.Key) {
			continue
		}
		a, b := t.values.Get(settings.CatalogueDefault, d.Key), values.Get(settings.CatalogueDefault, d.Key)
		// Cyclic normalization can leave insignificant floating-point tails.
		if d.Kind == settings.Number {
			if math.Abs(a.Number-b.Number) > 1e-9 {
				return false
			}
		} else if a != b {
			return false
		}
	}
	return true
}

func (v *view) selectTheme(name string) error {
	s := v.settingsInteraction()
	if s == nil || len(s.fields) == 0 || s.fields[0].Key != settingThemeSelect || s.pending {
		return fmt.Errorf("theme selector unavailable")
	}
	name = strings.TrimSpace(name)
	for _, theme := range s.themes {
		if !strings.EqualFold(name, theme.name) {
			continue
		}
		values := v.widgetValues().Clone()
		for _, d := range settings.CatalogueDefault.Settings {
			if themeSnapshotKey(d.Key) {
				if err := values.Set(settings.CatalogueDefault, d.Key, theme.values.Get(settings.CatalogueDefault, d.Key)); err != nil {
					return err
				}
			}
		}
		s.themeName, s.dirty, s.errorText = theme.name, true, ""
		*v.widgetValues() = values
		return nil
	}

	return fmt.Errorf("unknown theme: %s", name)
}

// Reuse the transaction's opening snapshot as an ordinary session-only choice.
// Check the opening appearance against the library, even after live cycling.
func (s *settingsSession) setThemeChoices(themes []themeSnapshot, current settings.Values) {
	s.themes = slices.Clone(themes)
	if !slices.ContainsFunc(themes, func(t themeSnapshot) bool { return t.matches(s.before) }) {
		// Keep a saved file with this name accessible alongside the opening snapshot.
		for i, t := range s.themes {
			if !strings.EqualFold(t.name, "Current theme") {
				continue
			}
			name := "Current theme (saved)"
			for slices.ContainsFunc(s.themes, func(t themeSnapshot) bool { return strings.EqualFold(t.name, name) }) {
				name += " (saved)"
			}
			s.themes[i].name = name
		}
		s.themes = append(s.themes, themeSnapshot{name: "Current theme", values: s.before})
	}
	s.completion.candidates = nil
	s.themeName = ""
	for _, t := range s.themes {
		s.completion.candidates = append(s.completion.candidates, t.name)
		if s.themeName == "" && t.matches(current) {
			s.themeName = t.name
		}
	}
}
