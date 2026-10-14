package settings

import (
	"reflect"
	"slices"
	"strings"
)

// defaultTree restores tables and collection item templates from the typed
// catalogue. Templates describe each item; they do not install collection items.
func defaultTree(c Catalogue) map[string]any {
	tree := make(map[string]any)
	for _, d := range c.Settings {
		var value any
		switch d.Kind {
		case Boolean:
			value = d.Default.Bool
		case Number:
			value = d.Default.Number
		case String:
			value = d.Default.Text
		case StringList:
			value = strings.Fields(d.Default.Text)
		}
		insert(tree, strings.Split(d.Key, "."), value)
	}
	collections := slices.Clone(c.Collections)
	slices.SortFunc(collections, func(a, b string) int { return strings.Count(b, ".") - strings.Count(a, ".") })
	for _, key := range collections {
		path := strings.Split(key, ".")
		raw, _ := lookup(tree, path)
		if item, ok := raw.(map[string]any); ok {
			insert(tree, path, []map[string]any{item})
		}
	}
	return tree
}

// Filtering works on a copy so previews, Cancel and failed-save retries retain
// their transaction state. Empty collection items retain their membership.
func (v Values) withoutDefaults(c Catalogue) Values {
	filtered := v.Clone()
	omitDefaults(filtered.overrides, defaultTree(c))
	return filtered
}

func omitDefaults(tree, defaults map[string]any) {
	for key, value := range tree {
		baseline, known := defaults[key]
		switch value := value.(type) {
		case map[string]any:
			children, _ := baseline.(map[string]any)
			omitDefaults(value, children)
			if len(value) == 0 {
				delete(tree, key)
			}
		case []map[string]any:
			templates, ok := baseline.([]map[string]any)
			if !ok || len(templates) != 1 {
				continue
			}
			if len(value) == 0 {
				delete(tree, key)
				continue
			}
			for _, item := range value {
				omitDefaults(item, templates[0])
			}
		case []string:
			if baseline, ok := baseline.([]string); ok && slices.Equal(value, baseline) {
				delete(tree, key)
			}
		default:
			if known && reflect.DeepEqual(value, baseline) {
				delete(tree, key)
			}
		}
	}
}
