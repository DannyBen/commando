package settings

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Save encodes the nested override tree through TOML. Inherited defaults and
// UI metadata are never written to the user document.
func Save(dataDir string, v Values) error {
	if dataDir == "" {
		return fmt.Errorf("data folder is empty")
	}
	var data bytes.Buffer
	if err := toml.NewEncoder(&data).Encode(v.overrides); err != nil {
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
