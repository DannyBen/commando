package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dannyben/commando/internal/settings"
)

var errInitialThemeSave = errors.New("cannot save initial theme")

// An existing document, even an empty or invalid one, owns the appearance.
// Persisting the first palette makes initialization independent of later system
// theme changes. Previews and secondary launches bypass this startup path.
func loadStartupSettings(dataDir string, prefersDark func() bool) (settings.Values, error) {
	c := bindingSettingsCatalogue()
	if dataDir == "" {
		return settings.LoadAndRepair(dataDir, c)
	}
	if _, err := os.Lstat(filepath.Join(dataDir, "settings.toml")); !errors.Is(err, os.ErrNotExist) {
		return settings.LoadAndRepair(dataDir, c)
	}
	theme := builtInThemes[0]
	if prefersDark() {
		theme = builtInThemes[1]
	}
	values := theme.values.Clone()
	if err := c.Save(dataDir, values); err != nil {
		return values, fmt.Errorf("%w: %w", errInitialThemeSave, err)
	}
	return values, nil
}
