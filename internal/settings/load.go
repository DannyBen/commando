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
// overrides, and no partial values on failure. It never modifies files.
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
	values, err := loadDocument(data, c)
	if err != nil {
		return Values{}, err
	}
	return values, nil
}

func loadDocument(data []byte, c Catalogue) (Values, error) {
	var tree map[string]any
	if _, err := toml.Decode(string(data), &tree); err != nil {
		return Values{}, fmt.Errorf("%w: %w", ErrSyntax, err)
	}
	var result Values
	err := loadItem(c, "", tree, &result)
	if c.ValidateOverrides != nil {
		err = errors.Join(err, c.ValidateOverrides(&result))
	}
	return result, err
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
	result := make([]map[string]any, 0, len(rows))
	var failures []error
	for _, row := range rows {
		var item Values
		if err := loadItem(c, key, row, &item); err != nil {
			failures = append(failures, err)
			// Unknown fields can be removed safely. Invalid rule values must
			// not silently become defaults that change how programs launch.
			if errors.Is(err, ErrType) || errors.Is(err, ErrRange) || errors.Is(err, ErrValue) {
				continue
			}
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
		normalized := item.Item(key)
		if key == EditorCollection {
			if err := NormalizeEditor(normalized); err != nil {
				failures = append(failures, fmt.Errorf("%w: %s: %v", ErrValue, key, err))
				continue
			}
		}
		result = append(result, normalized)
	}
	return result, errors.Join(failures...)
}

// loadItem retains valid leaves while collecting validation errors. Strict
// loading discards its result; startup recovery can save the surviving values.
func loadItem(c Catalogue, prefix string, row map[string]any, item *Values) error {
	var failures []error
	for name, raw := range row {
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if name == "" || strings.Contains(name, ".") {
			failures = append(failures, fmt.Errorf("%w: %s", ErrUnknownKey, key))
			continue
		}
		if slices.Contains(c.Collections, key) {
			rows, err := loadCollection(c, key, raw)
			if err != nil {
				failures = append(failures, err)
			}
			if rows != nil {
				item.SetCollection(key, rows)
			}
			continue
		}
		if d, known := c.Definition(key); known {
			scalar, err := validatedScalar(d, raw)
			if err == nil {
				err = item.Set(c, key, valueFromScalar(scalar))
			}
			if err != nil {
				failures = append(failures, err)
				continue
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
			failures = append(failures, fmt.Errorf("%w: %s", ErrUnknownKey, key))
			continue
		}
		children, ok := raw.(map[string]any)
		if !ok {
			failures = append(failures, fmt.Errorf("%w: %s", ErrType, key))
			continue
		}
		if err := loadItem(c, key, children, item); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
