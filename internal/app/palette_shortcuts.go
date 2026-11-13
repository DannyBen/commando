package app

import (
	"strings"
	"unicode/utf8"

	"github.com/dannyben/commando/internal/fuzzy"
)

func paletteShortcutQuery(query string) (shortcut, bool) {
	// Treat spaces and plus signs as combination separators. A lone plus is
	// still a literal key, as it is in the shortcut editor.
	if strings.TrimSpace(query) != "+" {
		query = strings.Join(strings.Fields(strings.ReplaceAll(query, "+", " ")), "+")
	}
	binding, err := parseBindingOverride(query)
	return binding.Shortcut, err == nil && binding.Mode == bindingAssigned
}

func (c command) matchShortcut(query shortcut) (fuzzy.Result, bool) {
	var match fuzzy.Result
	offset := 0
	for slot := range c.bindings {
		binding := c.effectiveShortcut(slot)
		label := binding.String()
		if label == "" {
			continue
		}
		length := utf8.RuneCountInString(label)
		if binding.Name == query.Name && (query.Modifiers == 0 || binding.Modifiers == query.Modifiers) {
			start := 0
			if query.Modifiers == 0 && binding.Modifiers != 0 {
				start = utf8.RuneCountInString(label[:strings.LastIndexByte(label, '+')+1])
			}
			for i := start; i < length; i++ {
				match.Indices = append(match.Indices, offset+i)
			}
			score := 1.0
			if binding.Modifiers == query.Modifiers {
				score = 2 // Prefer the exact combination to modified versions of a key.
			}
			match.Score = max(match.Score, score)
		}
		offset += length + len(" / ")
	}
	return match, len(match.Indices) != 0
}
