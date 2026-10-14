package settings

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

const EditorCollection = "programs.editors"
const SelectedFilesToken = "%SelectedFiles%"

type EditorRule struct {
	Extensions []string
	Program    string
	Arguments  string
}

func (v Values) EditorRules() []EditorRule {
	var rules []EditorRule
	for _, row := range v.Collection(EditorCollection) {
		ext, _ := row["ext"].([]string)
		program, _ := row["path"].(string)
		args, _ := row["args"].(string)
		rules = append(rules, EditorRule{ext, program, args})
	}
	return rules
}

// NormalizeEditor validates a complete draft before it becomes a launch rule.
func NormalizeEditor(row map[string]any) error {
	ext, _ := row["ext"].([]string)
	if len(ext) == 0 {
		return fmt.Errorf("Enter extensions or *")
	}
	seen := map[string]bool{}
	var normalized []string
	for _, name := range ext {
		name = strings.ToLower(strings.TrimPrefix(name, "."))
		if name == "" || strings.IndexFunc(name, unicode.IsSpace) >= 0 || name != "*" && strings.ContainsAny(name, "*?./\\,;:%\"<>|\x00") {
			return fmt.Errorf("Use extensions or *")
		}
		if !seen[name] {
			normalized = append(normalized, name)
			seen[name] = true
		}
	}
	if seen["*"] && len(normalized) != 1 {
		return fmt.Errorf("Use * on its own")
	}
	program, _ := row["path"].(string)
	program = strings.TrimSpace(program)
	if program == "" {
		return fmt.Errorf("Choose an editor program")
	}
	args, _ := row["args"].(string)
	if _, err := EditorArguments(args, []string{"file"}); err != nil {
		return err
	}
	row["path"] = program
	return nil
}

// EditorArguments uses double quotes and Windows backslash-before-quote rules.
// Expansion happens after splitting, so file paths are never parsed as syntax.
func EditorArguments(input string, files []string) ([]string, error) {
	var words []string
	var word strings.Builder
	quoted, started := false, false
	for i := 0; i < len(input); {
		ch := input[i]
		if !quoted && (ch == ' ' || ch == '\t') {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			i++
			continue
		}
		if ch == '\\' {
			end := i
			for end < len(input) && input[end] == '\\' {
				end++
			}
			count := end - i
			if end < len(input) && input[end] == '"' {
				word.WriteString(strings.Repeat("\\", count/2))
				if count%2 != 0 {
					word.WriteByte('"')
				} else {
					quoted = !quoted
				}
				i = end + 1
			} else {
				word.WriteString(input[i:end])
				i = end
			}
			started = true
			continue
		}
		if ch == '"' {
			quoted = !quoted
			started = true
			i++
			continue
		}
		word.WriteByte(ch)
		started = true
		i++
	}
	if quoted {
		return nil, fmt.Errorf("Close the argument quotes")
	}
	if started {
		words = append(words, word.String())
	}
	var result []string
	tokens := 0
	for _, arg := range words {
		if arg == SelectedFilesToken {
			result = append(result, files...)
			tokens++
			continue
		}
		if strings.Contains(arg, SelectedFilesToken) {
			return nil, fmt.Errorf("File token must stand alone")
		}
		result = append(result, arg)
	}
	if tokens != 1 {
		return nil, fmt.Errorf("Use one %%SelectedFiles%% token")
	}
	return result, nil
}

type EditorLaunch struct {
	Program   string
	Arguments []string
}

// EditorLaunches groups files by the first specific rule; the first wildcard
// rule is used only when no specific rule matches, regardless of its position.
func EditorLaunches(rules []EditorRule, files []string) ([]EditorLaunch, int, error) {
	grouped := make([][]string, len(rules))
	unmatched := 0
	for _, file := range files {
		extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(filepath.Base(file)), "."))
		if strings.HasPrefix(filepath.Base(file), ".") && strings.Count(filepath.Base(file), ".") == 1 {
			extension = ""
		}
		match, fallback := -1, -1
		for i, rule := range rules {
			for _, ext := range rule.Extensions {
				if ext == "*" && fallback < 0 {
					fallback = i
				}
				if strings.EqualFold(strings.TrimPrefix(ext, "."), extension) && match < 0 {
					match = i
				}
			}
		}
		if match < 0 {
			match = fallback
		}
		if match < 0 {
			unmatched++
			continue
		}
		grouped[match] = append(grouped[match], file)
	}
	var launches []EditorLaunch
	for i, files := range grouped {
		if len(files) == 0 {
			continue
		}
		args, err := EditorArguments(rules[i].Arguments, files)
		if err != nil {
			return nil, unmatched, err
		}
		launches = append(launches, EditorLaunch{rules[i].Program, args})
	}
	return launches, unmatched, nil
}
