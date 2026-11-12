package settings

import (
	"errors"
	"reflect"
	"testing"
)

func TestCustomActionArgumentExecutionAndLiteralPaths(t *testing.T) {
	paths := []string{`C:\one folder\a.txt`, `C:\שלום & %literal%\b.txt`}
	for _, tc := range []struct {
		input string
		want  [][]string
	}{
		{`--wait %Entry%`, [][]string{{"--wait", paths[0]}, {"--wait", paths[1]}}},
		{`--send %Entries%`, [][]string{{"--send", paths[0], paths[1]}}},
		{`"two words" "" "%Entries%"`, [][]string{{"two words", "", paths[0], paths[1]}}},
		{`/K codex`, [][]string{{"/K", "codex"}}},
		{"", [][]string{nil}},
	} {
		got, err := ActionArguments(tc.input, paths)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: got %#v, %v; want %#v", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{`%Entry% %Entries%`, `%Entry% %Entry%`, `%Entries% %Entries%`, `--file=%Entry%`, `--files=%Entries%`, `"unclosed %Entry%`} {
		if _, err := ActionArguments(input, paths); err == nil {
			t.Fatalf("accepted invalid arguments %q", input)
		}
	}
}

func TestCustomActionSettingsRoundTripAndRecovery(t *testing.T) {
	source := `[[programs.actions]]
name = 'Open with archive tool'
target = 'Files'
ext = ['ZIP']
path = 'archive.exe'
args = '--open %Entry%'
[[programs.actions]]
name = 'Codex here'
target = 'Here'
path = 'cmd.exe'
args = '/K codex'
terminal = true
`
	values, err := Parse([]byte(source), CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	rules := values.ActionRules()
	if len(rules) != 2 || !ValidIdentity(rules[0].ID) || !rules[1].Terminal || rules[1].Target != ActionFolders || rules[1].Mode != ActionHere {
		t.Fatal("action defaults or identity lost", rules)
	}
	if !rules[0].Matches("file.zip", false) || rules[0].Matches("file.txt", false) || rules[0].Matches("folder", true) {
		t.Fatal("extension/type matching failed")
	}
	dir := t.TempDir()
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(dir, CatalogueDefault)
	if err != nil || !reflect.DeepEqual(restored.ActionRules(), rules) {
		t.Fatal("round trip changed actions", restored.ActionRules(), err)
	}
	invalid := []string{
		"name=''\npath='tool.exe'", "name='Bad target'\ntarget='Other'\npath='tool.exe'",
		"name='Missing program'", "name='No token'\npath='tool.exe'\nargs=''",
		"name='Mixed tokens'\npath='tool.exe'\nargs='%Entry% %Entries%'", "name='Bad extension'\ntarget='Files'\npath='tool.exe'\next=['*.zip']",
	}
	for _, row := range invalid {
		got, err := Parse([]byte(source+"\n[[programs.actions]]\n"+row), CatalogueDefault)
		if !errors.Is(err, ErrValue) || len(got.ActionRules()) != 2 || !reflect.DeepEqual(got.ActionRules()[0].Extensions, []string{"ZIP"}) {
			t.Fatal("invalid action was not removed while preserving valid actions", row, got.ActionRules(), err)
		}
	}
	// Recovery assigns missing IDs anew; duplicate IDs retain their first owner.
	rows := restored.Collection(ActionCollection)
	rows[1]["id"] = rows[0]["id"]
	restored.SetCollection(ActionCollection, rows)
	if err := CatalogueDefault.NormalizeIdentities(&restored); err == nil || restored.ActionRules()[0].ID == restored.ActionRules()[1].ID {
		t.Fatal("duplicate action identities were not repaired")
	}
}

func TestActionMigrationPreservesIdentityAndInactiveSettings(t *testing.T) {
	values, err := Parse([]byte(`[[programs.actions]]
id = 'explorer'
name = 'Open in Explorer'
target = 'Folders'
ext = ['ZIP']
path = 'explorer.exe'
args = '%File%'
[[programs.actions]]
id = 'here'
name = 'Terminal here'
target = 'Here'
path = 'cmd.exe'
args = '/K'
`), CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	rules := values.ActionRules()
	if rules[0].ID != "explorer" || rules[0].Arguments != EntryToken || rules[0].Mode != ActionSelected || rules[1].ID != "here" || rules[1].Target != ActionFolders || rules[1].Mode != ActionHere {
		t.Fatal("first-pass actions lost their identities or behavior", rules)
	}
	row := values.Collection(ActionCollection)[0]
	row["target"], row["mode"] = ActionEntries, ActionAuto
	if err := NormalizeAction(row); err != nil {
		t.Fatal(err)
	}
	values.SetCollection(ActionCollection, []map[string]any{row})
	rule := values.ActionRules()[0]
	if !rule.Matches("file.txt", false) || !rule.Matches("folder", true) || rule.Extensions[0] != "ZIP" || rule.Mode != ActionAuto {
		t.Fatal("Both restricted entries or discarded inactive values", rule)
	}
	dir := t.TempDir()
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	restored, err := Load(dir, CatalogueDefault)
	if err != nil || !reflect.DeepEqual(restored.ActionRules(), values.ActionRules()) {
		t.Fatal("inactive values did not survive save/reload", restored.ActionRules(), err)
	}
}
