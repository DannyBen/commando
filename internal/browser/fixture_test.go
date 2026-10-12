package browser

import (
	"path/filepath"
	"testing"
)

// Match the physical paths supplied to native shell callbacks, including when
// Windows' temporary directory environment uses an 8.3 spelling.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
