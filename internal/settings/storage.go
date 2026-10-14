package settings

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Save rewrites the user document with non-default values only. Collections
// use the same recursive filtering against their item templates.
func Save(dataDir string, v Values) error {
	return save(dataDir, v, CatalogueDefault)
}

func save(dataDir string, v Values, c Catalogue) error {
	if dataDir == "" {
		return fmt.Errorf("data folder is empty")
	}
	var data bytes.Buffer
	encoder := toml.NewEncoder(&data)
	encoder.Indent = ""
	if err := encoder.Encode(v.withoutDefaults(c).overrides); err != nil {
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
