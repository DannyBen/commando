package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompareArgumentsKeepPairSeparateAndOrdered(t *testing.T) {
	first, second := `C:\first folder\%SecondFile%.txt`, `C:\second & $(literal).txt`
	args, err := CompareArguments(`--right "%SecondFile%" "two words" --left %FirstFile%`, first, second)
	if err != nil || !reflect.DeepEqual(args, []string{"--right", second, "two words", "--left", first}) {
		t.Fatal("compare parsed file paths as argument syntax or changed order", args, err)
	}
	for _, invalid := range []string{`%FirstFile%`, `%SecondFile%`, `%SelectedFiles%`, `%FirstFile% %FirstFile% %SecondFile%`, `--left=%FirstFile% %SecondFile%`, `"%FirstFile% %SecondFile%`, `%FirstFile% --right=%SecondFile%`} {
		if _, err := CompareArguments(invalid, first, second); err == nil {
			t.Fatal("invalid compare arguments accepted", invalid)
		}
	}
}

func TestCompareRulesReuseMatchingPersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	source := `[[programs.compare]]
ext = ["*"]
path = "fallback.exe"
[[programs.compare]]
ext = [".TXT", "md"]
path = "text.exe"
args = "--left %FirstFile% --right %SecondFile%"
`
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := Load(dir, CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	rules := v.ProgramRules(CompareCollection)
	for file, want := range map[string]int{"one.TXT": 1, "two.md": 1, "three.png": 0, ".env": 0} {
		if got := MatchProgram(rules, file); got != want {
			t.Fatal("wrong compare program", file, got)
		}
	}
	if rules[0].Arguments != FirstFileToken+" "+SecondFileToken {
		t.Fatal("compare rule lost its argument defaults")
	}
	if err := Save(dir, v); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(dir, CatalogueDefault)
	if err != nil || !reflect.DeepEqual(v.Collection(CompareCollection), reloaded.Collection(CompareCollection)) {
		t.Fatal("compare rules did not round-trip", err)
	}
	for _, invalid := range []map[string]any{
		{"ext": []string{"txt"}, "path": "compare.exe", "args": SelectedFilesToken},
		{"ext": []string{"*.txt"}, "path": "compare.exe", "args": FirstFileToken + " " + SecondFileToken},
	} {
		if err := NormalizeProgram(invalid, CompareCollection); err == nil {
			t.Fatal("invalid compare rule accepted by settings widget")
		}
		v.SetCollection(CompareCollection, []map[string]any{invalid})
		if err := Save(dir, v); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir, CatalogueDefault); err == nil {
			t.Fatal("invalid compare rule accepted on load")
		}
	}
}
