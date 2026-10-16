package settings

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

func TestSaveOverrides(t *testing.T) {
	c := CatalogueDefault
	c.Settings = append(append([]Definition(nil), c.Settings...), Definition{Key: "files.editor", Kind: String})
	var v Values
	for key, value := range map[string]Value{
		"view.show_protected_system_files":   {Bool: false}, // Explicit defaults are omitted on Save.
		"appearance.autocolor.lsh.lightness": {Number: 50},
		"application.data_folder.path":       {Text: "C:\\folder: #1\\\"quoted\"\nnext\tline"},
		"files.editor":                       {Text: "null"},
	} {
		if err := v.Set(c, key, value); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := Save(dir, v); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"appearance":  map[string]any{"autocolor": map[string]any{"lsh": map[string]any{"lightness": int64(50)}}},
		"files":       map[string]any{"editor": "null"},
		"application": map[string]any{"data_folder": map[string]any{"path": "C:\\folder: #1\\\"quoted\"\nnext\tline"}},
	}
	assertSavedTOML(t, dir, want)
	// Reset drops an override; saving replaces the previous document rather
	// than retaining stale keys or expanding inherited defaults.
	v.Reset("appearance.autocolor.lsh.lightness")
	if err := Save(dir, v); err != nil {
		t.Fatal(err)
	}
	delete(want, "appearance")
	assertSavedTOML(t, dir, want)
	if err := Save(dir, Values{}); err != nil {
		t.Fatal(err)
	}
	assertSavedTOML(t, dir, map[string]any{})
}

func TestSaveFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "settings.toml")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep")
	if err := os.WriteFile(marker, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, Values{}); err == nil {
		t.Fatal("replaced an inaccessible destination")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "original" {
		t.Fatalf("destination changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "settings.toml" {
		t.Fatalf("temporary file left behind: %v, %v", entries, err)
	}
	if err := Save("", Values{}); err == nil {
		t.Fatal("accepted empty data folder")
	}
}

func assertSavedTOML(t *testing.T, dir string, want map[string]any) {
	t.Helper()
	var got map[string]any
	_, err := toml.DecodeFile(filepath.Join(dir, "settings.toml"), &got)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("saved TOML = %#v, %v; want %#v", got, err, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected data files: %v, %v", entries, err)
	}
}

func TestSaveReturningToDefaultsRemovesKeysAndEmptyTables(t *testing.T) {
	var values Values
	dir := t.TempDir()
	for key, value := range map[string]Value{
		"feedback.notifications.animation":    {Bool: false},
		"appearance.autocolor.lsh.lightness":  {Number: 50},
		"application.data_folder.path":        {Text: "chosen folder"},
		"appearance.autocolor.lsh.saturation": {Number: 20},
	} {
		if err := values.Set(CatalogueDefault, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]Value{
		"feedback.notifications.animation":   {Bool: true},
		"appearance.autocolor.lsh.lightness": {Number: 36},
		"application.data_folder.path":       {Text: ""},
	} {
		if err := values.Set(CatalogueDefault, key, value); err != nil {
			t.Fatal(err)
		}
	}
	before := values.Clone()
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	assertSavedTOML(t, dir, map[string]any{"appearance": map[string]any{"autocolor": map[string]any{"lsh": map[string]any{"saturation": int64(20)}}}})
	if !reflect.DeepEqual(values, before) {
		t.Fatal("Save changed the caller's transaction")
	}
	restored, err := Load(dir, CatalogueDefault)
	if err != nil || !restored.Get(CatalogueDefault, "feedback.notifications.animation").Bool || restored.Get(CatalogueDefault, "appearance.autocolor.lsh.lightness").Number != 36 || restored.Get(CatalogueDefault, "appearance.autocolor.lsh.saturation").Number != 20 {
		t.Fatal("filtered settings did not reload correctly", err)
	}
}

func TestSaveCleansExistingExplicitDefaultsOnUnrelatedEdit(t *testing.T) {
	dir := t.TempDir()
	source := `feedback.notifications.animation = true
feedback.sounds.profile = -1
feedback.sounds.master_volume = 50
appearance.autocolor.lsh.lightness = 36
appearance.autocolor.salt = 0
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := Load(dir, CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	if err := values.Set(CatalogueDefault, "view.show_protected_system_files", Value{Bool: true}); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	assertSavedTOML(t, dir, map[string]any{"view": map[string]any{"show_protected_system_files": true}})
}

func TestSaveCollectionDefaultsRecursivelyPreservesItems(t *testing.T) {
	// An arbitrary collection with nested tables and collections exercises the
	// same persistence path as application settings, without editor-specific keys.
	c := Catalogue{Collections: []string{"profiles", "profiles.rules"}, Settings: []Definition{
		{Key: "profiles.name", Kind: String, Default: Value{Text: ""}},
		{Key: "profiles.options.enabled", Kind: Boolean, Default: Value{Bool: true}},
		{Key: "profiles.options.count", Kind: Number, Default: Value{Number: 2}},
		{Key: "profiles.rules.token", Kind: String, Default: Value{Text: "all"}},
		{Key: "profiles.rules.include", Kind: StringList, Default: Value{Text: "x y"}},
	}}
	defaults := map[string]any{"name": "", "options": map[string]any{"enabled": true, "count": float64(2)}}
	var values Values
	values.SetCollection("profiles", []map[string]any{
		defaults,
		{"name": "kept", "options": map[string]any{"enabled": true, "count": float64(5)}, "rules": []map[string]any{
			{"token": "all", "include": []string{"x", "y"}},
			{"token": "custom", "include": []string{"y", "x"}},
			{"token": "all", "include": []string{"x", "y"}},
		}},
		defaults,
	})
	dir := t.TempDir()
	before := values.Clone()
	if err := save(dir, values, c); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if _, err := toml.DecodeFile(filepath.Join(dir, "settings.toml"), &document); err != nil {
		t.Fatal(err)
	}
	items, ok := document["profiles"].([]map[string]any)
	if !ok || len(items) != 3 || len(items[0]) != 0 || len(items[2]) != 0 {
		t.Fatalf("collection lost empty items or order: %#v", document)
	}
	options := items[1]["options"].(map[string]any)
	if len(options) != 1 || options["count"] != int64(5) {
		t.Fatalf("nested table defaults remain: %#v", options)
	}
	rules := items[1]["rules"].([]map[string]any)
	if len(rules) != 3 || len(rules[0]) != 0 || len(rules[2]) != 0 || rules[1]["token"] != "custom" || !reflect.DeepEqual(rules[1]["include"], []any{"y", "x"}) {
		t.Fatalf("nested collection was not filtered generically: %#v", rules)
	}
	restored, err := Load(dir, c)
	if err != nil || !reflect.DeepEqual(restored.Collection("profiles"), before.Collection("profiles")) {
		t.Fatalf("collection defaults did not round trip: %#v, %v", restored.Collection("profiles"), err)
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("collection filtering changed transaction data")
	}
}

func TestSaveEmptyCollectionsAndDefaultStringLists(t *testing.T) {
	c := Catalogue{Collections: []string{"items"}, Settings: []Definition{
		{Key: "items.value", Kind: String},
		{Key: "options.names", Kind: StringList, Default: Value{Text: "a b"}},
		{Key: "options.empty", Kind: StringList},
	}}
	var values Values
	values.SetCollection("items", nil)
	_ = values.Set(c, "options.names", Value{Text: "a b"})
	_ = values.Set(c, "options.empty", Value{Text: ""})
	dir := t.TempDir()
	if err := save(dir, values, c); err != nil {
		t.Fatal(err)
	}
	assertSavedTOML(t, dir, map[string]any{})
}

func TestFilteredSaveFailureKeepsTransaction(t *testing.T) {
	var values Values
	_ = values.Set(CatalogueDefault, "feedback.notifications.animation", Value{Bool: true})
	values.SetCollection(EditorCollection, []map[string]any{{"ext": []string{"txt"}, "path": "editor.exe", "args": SelectedFilesToken}})
	before := values.Clone()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "settings.toml"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, values); err == nil {
		t.Fatal("inaccessible destination accepted")
	}
	if !reflect.DeepEqual(values, before) {
		t.Fatal("failed save modified transaction values")
	}
}

func TestColorPrecisionStaysConciseThroughNormalizationAndSaveLoad(t *testing.T) {
	for _, tc := range []struct {
		key         string
		input, want float64
	}{
		{"appearance.theme.colors.alt_background.hue", 90.11250000000001, 90.1125},
		{"appearance.theme.colors.background.hue", 80.92790000000002, 80.9279},
		{"appearance.theme.colors.background.hue", -10.1234567, 349.8765},
		{"appearance.theme.colors.background.hue", 359.9999999, 0},
		{"appearance.autocolor.lsh.saturation", 70.594642289, 70.5946},
		{"appearance.autocolor.selection.lightness_points", -16.1234567, -16.1235},
	} {
		t.Run(fmt.Sprintf("%s/%g", tc.key, tc.input), func(t *testing.T) {
			source := fmt.Sprintf("%s = %.17g\n", tc.key, tc.input)
			values, err := Parse([]byte(source), CatalogueDefault)
			if err != nil || values.Get(CatalogueDefault, tc.key).Number != tc.want {
				t.Fatal("committed value not rounded after normalization", err, values.Get(CatalogueDefault, tc.key))
			}
			dir := t.TempDir()
			var previous string
			for attempt := 0; attempt < 3; attempt++ {
				if err := Save(dir, values); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
				if err != nil {
					t.Fatal(err)
				}
				if regexp.MustCompile(`\.\d{5}`).Match(data) {
					t.Fatalf("excess fractional digits: %s", data)
				}
				if attempt > 0 && string(data) != previous {
					t.Fatal("save/load changed serialized precision")
				}
				previous = string(data)
				values, err = Load(dir, CatalogueDefault)
				if err != nil || values.Get(CatalogueDefault, tc.key).Number != tc.want {
					t.Fatal("round trip lost rounded value", err)
				}
			}
		})
	}
	var values Values
	const fontSize = 18.123456789
	if err := values.Set(CatalogueDefault, "appearance.font.text_size", Value{Number: fontSize}); err != nil {
		t.Fatal(err)
	}
	if values.Get(CatalogueDefault, "appearance.font.text_size").Number != fontSize {
		t.Fatal("color precision affected an unrelated setting")
	}
}
