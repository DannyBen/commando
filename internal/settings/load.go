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
	for _, path := range metadata.Keys() {
		for _, part := range path {
			if part == "" || strings.Contains(part, ".") {
				return Values{}, fmt.Errorf("%w: %s", ErrUnknownKey, path)
			}
		}
		key := strings.Join(path, ".")
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
		// TOML users may write an integer for a numeric setting, e.g. hue = 20.
		if n, ok := scalar.(int64); ok && d.Kind == Number {
			scalar = float64(n)
		}
		validType := false
		switch d.Kind {
		case Boolean:
			_, validType = scalar.(bool)
		case Number:
			_, validType = scalar.(float64)
		case String:
			_, validType = scalar.(string)
		}
		if !validType {
			return Values{}, fmt.Errorf("%w: %s", ErrType, path)
		}
		if err := result.Set(c, key, valueFromScalar(scalar)); err != nil {
			return Values{}, err
		}
	}
	return result, nil
}
