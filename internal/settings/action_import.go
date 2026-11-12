package settings

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ImportAction translates a registered executable command into a reviewable
// draft. Windows %1 names one item; other Shell placeholders need semantics we
// cannot infer from this command alone. Preserve authored quoting verbatim.
func ImportAction(name, command string, directory bool, extensions []string) (map[string]any, error) {
	command = strings.TrimSpace(command)
	words, err := programArguments(command, nil)
	if err != nil || len(words) == 0 || !strings.EqualFold(filepath.Ext(words[0]), ".exe") || strings.Contains(words[0], "%") {
		return nil, fmt.Errorf("Unsupported action program")
	}
	end := strings.IndexAny(command, " \t")
	if strings.HasPrefix(command, `"`) {
		end = strings.IndexByte(command[1:], '"') + 2
		if end < len(command) && command[end] != ' ' && command[end] != '\t' {
			return nil, fmt.Errorf("Unsupported action program")
		}
	}
	if end < 0 || end >= len(command) {
		return nil, fmt.Errorf("Unsupported command arguments")
	}
	arguments := strings.TrimSpace(command[end:])
	for i := 0; i < len(arguments); i++ {
		if arguments[i] != '%' {
			continue
		}
		if i+1 >= len(arguments) || arguments[i+1] != '1' || i+2 < len(arguments) && arguments[i+2] >= '0' && arguments[i+2] <= '9' {
			return nil, fmt.Errorf("Unsupported command arguments")
		}
		i++
	}
	target := ActionFiles
	if directory {
		target = ActionFolders
	}
	row := map[string]any{
		"name": name, "target": target, "mode": ActionSelected, "ext": extensions,
		"path": words[0], "args": strings.ReplaceAll(arguments, "%1", EntryToken), "terminal": false,
	}
	if err := NormalizeAction(row); err != nil {
		return nil, fmt.Errorf("Unsupported command arguments")
	}
	return row, nil
}
