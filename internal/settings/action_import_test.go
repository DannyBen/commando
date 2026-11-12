package settings

import (
	"reflect"
	"testing"
)

func TestImportActionsFromCapturedWindowsCommands(t *testing.T) {
	paths := []string{`C:\one folder\example`, `C:\שלום & %Entry%\example`}
	for _, tc := range []struct {
		name, command, program, arguments string
		directory                         bool
	}{
		{"Encrypt with Trinity", `"W:\go\trinity\dist\trinity.exe" --encrypt "%1"`, `W:\go\trinity\dist\trinity.exe`, `--encrypt "%Entry%"`, false},
		{"TreeSize Free", `"C:\D\Portable\TreeSize\TreeSizeFree.exe" "%1"`, `C:\D\Portable\TreeSize\TreeSizeFree.exe`, `"%Entry%"`, true},
		{"Codex here...", `explorer.exe "codex://new?path=%1"`, "explorer.exe", `"codex://new?path=%Entry%"`, true},
		{"Named argument", `tool.exe --file="%1" "literal value"`, "tool.exe", `--file="%Entry%" "literal value"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := ImportAction(tc.name, tc.command, tc.directory, []string{"*"})
			if err != nil || row["name"] != tc.name || row["path"] != tc.program || row["args"] != tc.arguments || row["mode"] != ActionSelected {
				t.Fatal("incorrect imported draft", row, err)
			}
			if (row["target"] == ActionFolders) != tc.directory {
				t.Fatal("wrong imported target", row)
			}
			launches, err := ActionArguments(tc.arguments, paths)
			if err != nil || len(launches) != 2 {
				t.Fatal("import lost per-entry behavior", launches, err)
			}
			if tc.program == "explorer.exe" && !reflect.DeepEqual(launches, [][]string{{"codex://new?path=" + paths[0]}, {"codex://new?path=" + paths[1]}}) {
				t.Fatal("URI import changed argument boundaries or path text", launches)
			}
		})
	}
}

func TestImportRejectsUnknownWindowsPlaceholderSemantics(t *testing.T) {
	for _, command := range []string{
		``, `tool.exe`, `tool.exe "%2"`, `tool.exe "%10"`, `tool.exe "%1" %*`,
		`tool.exe "%V"`, `tool.exe "%L"`, `tool.exe "%Entry%"`,
		`tool.exe "%1" "%1"`, `tool.exe "%1`, `"tool.exe"extra "%1"`,
		`C:\Program Files\tool.exe "%1"`, `script.cmd "%1"`,
	} {
		if row, err := ImportAction("Action", command, false, []string{"*"}); err == nil {
			t.Fatal("imported an unsupported command", command, row)
		}
	}
}
