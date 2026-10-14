package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load accepts dotted keys and table sections alike. It returns only explicit
// overrides, and no partial values on failure. Reading never creates or rewrites
// the data folder or settings file.
func Load(dataDir string, c Catalogue) (Values, error) {
	if dataDir == "" {
		return Values{}, fmt.Errorf("data folder is empty")
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "settings.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return Values{}, nil
	}
	if err != nil {
		return Values{}, err
	}
	var tree map[string]any
	metadata, err := toml.Decode(string(data), &tree)
	if err != nil {
		return Values{}, fmt.Errorf("%w: %w", ErrSyntax, err)
	}
	tables := map[string]bool{}
	for _, d := range c.Settings {
		path := strings.Split(d.Key, ".")
		for depth := 1; depth < len(path); depth++ {
			tables[strings.Join(path[:depth], ".")] = true
		}
	}
	var result Values
	collections := map[string]bool{}
	for _, collection := range c.Collections {
		collections[collection] = true
		parts := strings.Split(collection, ".")
		for depth := 1; depth < len(parts); depth++ {
			tables[strings.Join(parts[:depth], ".")] = true
		}
		raw, exists := lookup(tree, parts)
		if !exists {
			continue
		}
		rows, ok := raw.([]map[string]any)
		if empty, yes := raw.([]any); yes && len(empty) == 0 {
			rows, ok = []map[string]any{}, true
		}
		if !ok {
			return Values{}, fmt.Errorf("%w: %s", ErrType, collection)
		}
		normalized := make([]map[string]any, len(rows))
		for i, row := range rows {
			var item Values
			for name, raw := range row {
				key := collection + "." + name
				d, known := c.Definition(key)
				if !known {
					return Values{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
				}
				scalar, err := validatedScalar(d, raw)
				if err != nil {
					return Values{}, err
				}
				if err := item.Set(c, key, valueFromScalar(scalar)); err != nil {
					return Values{}, err
				}
				if d.Kind == StringList {
					insert(item.overrides, strings.Split(key, "."), scalar)
				}
			}
			for _, d := range c.Settings {
				if strings.HasPrefix(d.Key, collection+".") && !item.Overridden(d.Key) {
					_ = item.Set(c, d.Key, d.Default)
				}
			}
			normalized[i] = item.Item(collection)
			if collection == EditorCollection {
				if err := NormalizeEditor(normalized[i]); err != nil {
					return Values{}, fmt.Errorf("%w: %s: %v", ErrValue, collection, err)
				}
			}
		}
		result.SetCollection(collection, normalized)
	}
	for _, path := range metadata.Keys() {
		for _, part := range path {
			if part == "" || strings.Contains(part, ".") {
				return Values{}, fmt.Errorf("%w: %s", ErrUnknownKey, path)
			}
		}
		key := strings.Join(path, ".")
		inCollection := false
		for collection := range collections {
			if key == collection || strings.HasPrefix(key, collection+".") {
				inCollection = true
				break
			}
		}
		if inCollection {
			continue
		}
		scalar, _ := lookup(tree, path)
		if tables[key] {
			if _, table := scalar.(map[string]any); table {
				continue
			}
			return Values{}, fmt.Errorf("%w: %s", ErrType, path)
		}
		d, known := c.Definition(key)
		if !known {
			return Values{}, fmt.Errorf("%w: %s", ErrUnknownKey, path)
		}
		scalar, err = validatedScalar(d, scalar)
		if err != nil {
			return Values{}, err
		}
		if err := result.Set(c, key, valueFromScalar(scalar)); err != nil {
			return Values{}, err
		}
	}
	return result, nil
}

func validatedScalar(d Definition, raw any) (any, error) {
	valid := false
	switch d.Kind {
	case Boolean:
		_, valid = raw.(bool)
	case Number:
		if n, ok := raw.(int64); ok {
			raw = float64(n)
		}
		_, valid = raw.(float64)
	case String:
		_, valid = raw.(string)
	case StringList:
		if list, ok := raw.([]any); ok {
			stringsList := make([]string, len(list))
			for i, value := range list {
				text, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("%w: %s", ErrType, d.Key)
				}
				stringsList[i] = text
			}
			raw = stringsList
		}
		_, valid = raw.([]string)
	}
	if !valid {
		return nil, fmt.Errorf("%w: %s", ErrType, d.Key)
	}
	return raw, nil
}
