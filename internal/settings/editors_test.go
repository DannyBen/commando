package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEditorArgumentsKeepFilesSeparate(t *testing.T) {
	files := []string{`C:\a folder\one.txt`, `C:\two & $(literal).txt`}
	args, err := EditorArguments(`--wait "two words" "" "C:\folder\\" %SelectedFiles%`, files)
	want := append([]string{"--wait", "two words", "", `C:\folder\`}, files...)
	if err != nil || !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, %v; want %#v", args, err, want)
	}
	for _, invalid := range []string{`"unclosed`, `--file=%SelectedFiles%`, `--wait`, `%SelectedFiles% %SelectedFiles%`} {
		if _, err := EditorArguments(invalid, files); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestEditorGroupingSpecificRulesBeforeFallback(t *testing.T) {
	rules := []EditorRule{
		{[]string{"*"}, "fallback", SelectedFilesToken},
		{[]string{"txt", "md"}, "text", "--wait " + SelectedFilesToken},
		{[]string{"txt"}, "later", SelectedFilesToken},
	}
	files := []string{"a.TXT", "b.md", "c.txt", "d.txt", "picture.png", ".env"}
	got, missing, err := EditorLaunches(rules, files)
	want := []EditorLaunch{{"fallback", files[4:]}, {"text", append([]string{"--wait"}, files[:4]...)}}
	if err != nil || missing != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("launches = %#v, missing %d, %v", got, missing, err)
	}
	_, missing, err = EditorLaunches(rules[1:], files)
	if err != nil || missing != 2 {
		t.Fatalf("unmatched = %d, %v", missing, err)
	}
}

func TestEditorRulesRoundTripAndClone(t *testing.T) {
	dir := t.TempDir()
	source := `appearance.autocolor.salt = 42
[[programs.editors]]
ext = [".TXT", "md", "txt"]
path = "text.exe"
args = "--wait %SelectedFiles%"
[[programs.editors]]
ext = ["*"]
path = "fallback.exe"
`
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := Load(dir, CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	rules := values.EditorRules()
	if len(rules) != 2 || !reflect.DeepEqual(rules[0].Extensions, []string{".TXT", "md", "txt"}) || rules[1].Arguments != SelectedFilesToken {
		t.Fatalf("rules = %#v", rules)
	}
	before := values.Clone()
	rows := values.Collection(EditorCollection)
	rows[0]["path"] = "changed.exe"
	rows[0]["ext"].([]string)[0] = "go"
	values.SetCollection(EditorCollection, rows[:1])
	if before.EditorRules()[0].Program != "text.exe" || before.EditorRules()[0].Extensions[0] != ".TXT" {
		t.Fatal("draft changed snapshot")
	}
	if err := Save(dir, before); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(dir, CatalogueDefault)
	if err != nil || !reflect.DeepEqual(restored.EditorRules(), rules) || restored.Get(CatalogueDefault, "appearance.autocolor.salt").Number != 42 {
		t.Fatalf("round trip = %#v, %v", restored, err)
	}
	restored.SetCollection(EditorCollection, nil)
	if err := Save(dir, restored); err != nil {
		t.Fatal(err)
	}
	empty, err := Load(dir, CatalogueDefault)
	if err != nil || len(empty.EditorRules()) != 0 {
		t.Fatalf("empty collection = %#v, %v", empty, err)
	}
}

func TestEditorRuleValidationRejectsWholeDocument(t *testing.T) {
	for _, source := range []string{
		`[[programs.editors]]
ext = ["txt"]
path = "editor.exe"
unknown = true`,
		`[[programs.editors]]
ext = "txt"
path = "editor.exe"`,
		`[[programs.editors]]
ext = ["txt md"]
path = "editor.exe"`,
		`[[programs.editors]]
ext = ["*.txt"]
path = "editor.exe"`,
		`[[programs.editors]]
ext = ["*", "txt"]
path = "editor.exe"`,
		`[[programs.editors]]
ext = ["txt"]`,
		`[[programs.editors]]
ext = ["txt"]
path = "editor.exe"
args = "--wait"`,
		`[programs.editors]
ext = ["txt"]
path = "editor.exe"`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("appearance.autocolor.salt = 42\n"+source), 0600); err != nil {
			t.Fatal(err)
		}
		values, err := Load(dir, CatalogueDefault)
		if err == nil || values.Overridden("appearance.autocolor.salt") {
			t.Fatalf("accepted partial/invalid rules: %s, %v", source, err)
		}
		if !errors.Is(err, ErrType) && !errors.Is(err, ErrValue) && !errors.Is(err, ErrUnknownKey) {
			t.Fatalf("unclassified error %v", err)
		}
	}
}
