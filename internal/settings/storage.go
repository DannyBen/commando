package settings

import (
	"bytes"
	"fmt"
	"github.com/dannyben/commando/internal/diagnostics"
	"math"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Save rewrites the user document with non-default values only. Collections
// use the same recursive filtering against their item templates.
func Save(dataDir string, v Values) error {
	return save(dataDir, v, CatalogueDefault)
}

func (c Catalogue) Save(dataDir string, v Values) error {
	return save(dataDir, v, c)
}

func save(dataDir string, v Values, c Catalogue) (resultErr error) {
	defer func() {
		diagnostics.Record(diagnostics.Storage, 0, "SettingsSave", map[string]any{"directory": dataDir, "error": diagnostics.Error(resultErr)})
	}()
	if dataDir == "" {
		return fmt.Errorf("data folder is empty")
	}
	v = v.Clone()
	if err := c.NormalizeIdentities(&v); err != nil {
		return err
	}
	if c.ValidateOverrides != nil {
		if err := c.ValidateOverrides(&v); err != nil {
			return err
		}
	}
	var data bytes.Buffer
	encoder := toml.NewEncoder(&data)
	encoder.Indent = ""
	document := v.withoutDefaults(c).overrides
	wholeNumbers(document)
	if err := encoder.Encode(document); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dataDir, ".settings-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dataDir, "settings.toml"))
}

// Integer literals make human-edited values compact. Fractional values retain
// their precision; the transaction itself remains a floating-point value store.
func wholeNumbers(tree map[string]any) {
	for key, value := range tree {
		switch value := value.(type) {
		case map[string]any:
			wholeNumbers(value)
		case []map[string]any:
			for _, row := range value {
				wholeNumbers(row)
			}
		case float64:
			if math.Trunc(value) == value && math.Abs(value) <= 1<<53-1 {
				tree[key] = int64(value)
			}
		}
	}
}
