package app

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

const helpCommandID = "application.toggle_help"

//go:embed help.toml
var helpTOML string

type helpSpan struct{ text, command, settingsQuery string }
type helpTip struct{ lines [][]helpSpan }
type helpCatalogue map[string]helpTip

var defaultHelp = mustHelpCatalogue()

func mustHelpCatalogue() helpCatalogue {
	catalogue, err := parseHelpCatalogue(helpTOML, defaultCommands())
	if err != nil {
		panic(err)
	}
	for _, id := range []string{"help", "browsing.introduction"} {
		if _, ok := catalogue[id]; !ok {
			panic("missing help entry: " + id)
		}
	}
	return catalogue
}

func (c helpCatalogue) resolve(ids ...string) (string, helpTip) {
	for _, id := range ids {
		if tip, ok := c[id]; ok {
			return id, tip
		}
	}
	return "", helpTip{}
}

// User-created instances share explanations without losing the option of an
// exact entry. Coverage requires the family, not an arbitrary sample instance.
func paletteHelpID(id string) string {
	if _, ok := panelFocusNumber(id); ok {
		return "palette.panel.focus"
	}
	if _, ok := editorRuleBindingID(id); ok {
		return "palette.settings.programs.editors.rules"
	}
	return "palette." + id
}

func parseHelpCatalogue(source string, commands []command) (helpCatalogue, error) {
	var entries map[string]struct{ Lines []string }
	metadata, err := toml.Decode(source, &entries)
	if err != nil {
		return nil, err
	}
	if len(metadata.Undecoded()) > 0 {
		return nil, fmt.Errorf("unknown help field: %s", metadata.Undecoded()[0])
	}
	known := make(map[string]bool, len(commands))
	for _, c := range commands {
		known[c.id] = true
	}
	catalogue := make(helpCatalogue, len(entries))
	for id, entry := range entries {
		if strings.TrimSpace(id) != id || id == "" || len(entry.Lines) < 1 || len(entry.Lines) > 2 {
			return nil, fmt.Errorf("invalid help entry: %q", id)
		}
		var tip helpTip
		for _, line := range entry.Lines {
			if strings.TrimSpace(line) == "" || strings.ContainsAny(line, "\r\n\t") {
				return nil, fmt.Errorf("invalid help line: %s", id)
			}
			spans, err := parseHelpLine(line, known)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			tip.lines = append(tip.lines, spans)
		}
		catalogue[id] = tip
	}
	return catalogue, nil
}

func parseHelpLine(line string, commands map[string]bool) ([]helpSpan, error) {
	var spans []helpSpan
	for line != "" {
		marker := "[key:"
		start := strings.Index(line, marker)
		if settings := strings.Index(line, "[settings:"); settings >= 0 && (start < 0 || settings < start) {
			start, marker = settings, "[settings:"
		}
		if start < 0 {
			spans = append(spans, helpSpan{text: line})
			break
		}
		plain, token := line[:start], line[start+len(marker):]
		if plain != "" {
			spans = append(spans, helpSpan{text: plain})
		}
		id, rest, closed := strings.Cut(token, "]")
		if marker == "[settings:" {
			if !closed || id == "" || strings.ContainsAny(id, " @[]\t\r\n") {
				return nil, fmt.Errorf("invalid help settings query: %q", token)
			}
			spans = append(spans, helpSpan{text: "@" + id, settingsQuery: id})
		} else if !closed || !commands[id] {
			return nil, fmt.Errorf("invalid help binding: %q", token)
		} else {
			spans = append(spans, helpSpan{command: id})
		}
		line = rest
	}
	return spans, nil
}

func (v *view) helpCommand() command {
	for _, c := range commands {
		if c.id == helpCommandID {
			return v.resolveCommandBindings(c)
		}
	}
	panic("missing help command")
}

func (v *view) resolveHelpLine(line []helpSpan) []helpSpan {
	resolved := make([]helpSpan, len(line))
	for i, span := range line {
		if span.command != "" {
			span.text = "[unassigned]"
			for _, c := range commands {
				if c.id != span.command {
					continue
				}
				c = v.resolveCommandBindings(c)
				for slot := range c.bindings {
					if binding := c.effectiveShortcut(slot).String(); binding != "" {
						span.text = "[" + binding + "]"
						break
					}
				}
				break
			}
		}
		resolved[i] = span
	}
	return resolved
}
