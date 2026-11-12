package settings

import (
	"fmt"
	"strings"
)

const ActionCollection = "programs.actions"
const EntryToken, EntriesToken = "%Entry%", "%Entries%"
const (
	ActionFiles    = "Files"
	ActionFolders  = "Folders"
	ActionEntries  = "Both"
	ActionSelected = "Selected"
	ActionHere     = "Here"
	ActionAuto     = "Auto"
)

type ActionRule struct {
	ID, Name, Target, Mode, Program, Arguments string
	Extensions                                 []string
	Terminal                                   bool
}

func (v Values) ActionRules() []ActionRule {
	var rules []ActionRule
	for _, row := range v.Collection(ActionCollection) {
		id, _ := row["id"].(string)
		name, _ := row["name"].(string)
		target, _ := row["target"].(string)
		mode, _ := row["mode"].(string)
		if mode == "" {
			mode = ActionSelected
		}
		program, _ := row["path"].(string)
		args, _ := row["args"].(string)
		ext, _ := row["ext"].([]string)
		terminal, _ := row["terminal"].(bool)
		rules = append(rules, ActionRule{id, name, target, mode, program, args, ext, terminal})
	}
	return rules
}

func NormalizeAction(row map[string]any) error {
	migrateAction(row)
	name, _ := row["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("Enter an action name")
	}
	target, _ := row["target"].(string)
	switch target {
	case ActionFiles, ActionFolders, ActionEntries:
	default:
		return fmt.Errorf("Choose an action target")
	}
	mode, _ := row["mode"].(string)
	if mode == "" {
		mode = ActionSelected
	}
	switch mode {
	case ActionSelected, ActionHere, ActionAuto:
	default:
		return fmt.Errorf("Choose a folder mode")
	}
	// Retain inactive extensions without imposing restrictions on folders or Both.
	ext := row["ext"]
	if target != ActionFiles {
		ext = []string{"*"}
	}
	program := map[string]any{"ext": ext, "path": row["path"], "args": SelectedFilesToken}
	if err := NormalizeEditor(program); err != nil {
		return err
	}
	args, _ := row["args"].(string)
	if !(target == ActionFolders && mode == ActionHere) && !strings.Contains(args, EntryToken) && !strings.Contains(args, EntriesToken) {
		return fmt.Errorf("Use %s or %s", EntryToken, EntriesToken)
	}
	if _, err := ActionArguments(args, []string{"file"}); err != nil {
		return err
	}
	row["name"], row["path"], row["mode"] = name, program["path"], mode
	return nil
}

// Keep first-pass actions usable without changing their IDs or bindings.
// Conversion happens in memory; the ordinary Save flow persists it.
func migrateAction(row map[string]any) {
	if row["target"] == "Here" {
		row["target"], row["mode"] = ActionFolders, ActionHere
	}
	if args, ok := row["args"].(string); ok {
		row["args"] = strings.NewReplacer("%File%", EntryToken, "%Files%", EntriesToken).Replace(args)
	}
}

// ActionArguments infers batching from a whole argument token. Parsing precedes
// substitution, so paths never become argument syntax. Tokenless commands run
// once and are reserved for actions targeting the current folder.
func ActionArguments(input string, paths []string) ([][]string, error) {
	each, all := strings.Contains(input, EntryToken), strings.Contains(input, EntriesToken)
	if each && all {
		return nil, fmt.Errorf("Use %s or %s, not both", EntryToken, EntriesToken)
	}
	if each {
		var result [][]string
		for _, path := range paths {
			args, err := programArguments(input, map[string][]string{EntryToken: {path}})
			if err != nil {
				return nil, err
			}
			result = append(result, args)
		}
		return result, nil
	}
	replacements := map[string][]string{}
	if all {
		replacements[EntriesToken] = paths
	}
	args, err := programArguments(input, replacements)
	if err != nil {
		return nil, err
	}
	return [][]string{args}, nil
}

func (r ActionRule) Matches(name string, directory bool) bool {
	if r.Target == ActionEntries {
		return true
	}
	if directory {
		return r.Target == ActionFolders
	}
	return r.Target == ActionFiles &&
		MatchProgram([]EditorRule{{Extensions: r.Extensions}}, name) >= 0
}
