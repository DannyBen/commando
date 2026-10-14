package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		normalized, err := loadCollection(c, collection, raw)
		if err != nil {
			return Values{}, err
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

// Collection items reuse the catalogue's scalar, table and collection rules.
func loadCollection(c Catalogue, key string, raw any) ([]map[string]any, error) {
	rows, ok := raw.([]map[string]any)
	if empty, yes := raw.([]any); yes && len(empty) == 0 {
		rows, ok = []map[string]any{}, true
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrType, key)
	}
	result := make([]map[string]any, len(rows))
	for i, row := range rows {
		var item Values
		if err := loadItem(c, key, row, &item); err != nil {
			return nil, err
		}
		for _, d := range c.Settings {
			if !strings.HasPrefix(d.Key, key+".") {
				continue
			}
			nested := false
			for _, collection := range c.Collections {
				if strings.HasPrefix(collection, key+".") && strings.HasPrefix(d.Key, collection+".") {
					nested = true
					break
				}
			}
			if !nested && !item.Overridden(d.Key) {
				_ = item.Set(c, d.Key, d.Default)
			}
		}
		result[i] = item.Item(key)
		if key == EditorCollection {
			if err := NormalizeEditor(result[i]); err != nil {
				return nil, fmt.Errorf("%w: %s: %v", ErrValue, key, err)
			}
		}
	}
	return result, nil
}

func loadItem(c Catalogue, prefix string, row map[string]any, item *Values) error {
	for name, raw := range row {
		key := prefix + "." + name
		if name == "" || strings.Contains(name, ".") {
			return fmt.Errorf("%w: %s", ErrUnknownKey, key)
		}
		if slices.Contains(c.Collections, key) {
			rows, err := loadCollection(c, key, raw)
			if err != nil {
				return err
			}
			item.SetCollection(key, rows)
			continue
		}
		if d, known := c.Definition(key); known {
			scalar, err := validatedScalar(d, raw)
			if err != nil {
				return err
			}
			if err := item.Set(c, key, valueFromScalar(scalar)); err != nil {
				return err
			}
			if d.Kind == StringList {
				insert(item.overrides, strings.Split(key, "."), scalar)
			}
			continue
		}
		group := false
		for _, d := range c.Settings {
			if strings.HasPrefix(d.Key, key+".") {
				group = true
				break
			}
		}
		if !group {
			return fmt.Errorf("%w: %s", ErrUnknownKey, key)
		}
		children, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %s", ErrType, key)
		}
		if err := loadItem(c, key, children, item); err != nil {
			return err
		}
	}
	return nil
}
