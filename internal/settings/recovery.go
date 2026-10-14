package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrRepaired reports successful recovery, rather than a load failure.
var ErrRepaired = errors.New("settings auto-fixed")

var ErrRepairFailed = errors.New("settings repair failed")

// LoadAndRepair preserves invalid but parseable settings in settings.toml.bak
// before rewriting their valid overrides. Syntax and filesystem errors leave
// the original file untouched.
func LoadAndRepair(dataDir string, c Catalogue) (Values, error) {
	if dataDir == "" {
		return Values{}, fmt.Errorf("data folder is empty")
	}
	path := filepath.Join(dataDir, "settings.toml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Values{}, nil
	}
	if err != nil {
		return Values{}, err
	}
	values, validationErr := loadDocument(data, c)
	if validationErr == nil {
		return values, nil
	}
	if errors.Is(validationErr, ErrSyntax) {
		return Values{}, validationErr
	}
	backup, err := os.OpenFile(path+".bak", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err == nil {
		_, err = backup.Write(data)
		if err == nil {
			err = backup.Sync()
		}
		err = errors.Join(err, backup.Close())
	}
	if err != nil {
		return values, fmt.Errorf("%w: cannot back up settings: %w", ErrRepairFailed, err)
	}
	if err := save(dataDir, values, c); err != nil {
		return values, fmt.Errorf("%w: %w", ErrRepairFailed, err)
	}
	return values, fmt.Errorf("%w: %w", ErrRepaired, validationErr)
}
