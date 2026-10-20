// Package windowshell hosts native file dragging and shell context menus.
// Hosts and their methods belong to the native window's OLE STA thread.
package windowshell

import (
	"fmt"
	"image"
	"path/filepath"
	"strings"
	"time"
)

func siblingPaths(paths []string, allowRoot bool) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files selected")
	}
	cleaned := make([]string, len(paths))
	for i, path := range paths {
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
			return nil, fmt.Errorf("shell paths must be absolute")
		}
		cleaned[i] = filepath.Clean(path)
		if filepath.Dir(cleaned[i]) == cleaned[i] && !(allowRoot && len(paths) == 1) {
			return nil, fmt.Errorf("select files or folders, not a drive root")
		}
		if i > 0 && !strings.EqualFold(filepath.Dir(cleaned[0]), filepath.Dir(cleaned[i])) {
			return nil, fmt.Errorf("selected items must share a parent")
		}
	}
	return cleaned, nil
}

// Result describes the native API outcome, not verification of filesystem
// changes made by a destination application or shell extension.
type Result struct {
	Action       string
	Cancelled    bool
	Effect       uint32
	Commands     uint32
	MenuVersion  int
	MenuMessages uint32
	ThreadID     uint32
	Timings      MenuTimings
}

type MenuOptions struct {
	Background  bool // Request the single folder's background menu, including drive roots.
	Extended    bool
	ClientPoint *image.Point // nil uses the cursor captured before preparation.
}

type MenuTimings struct {
	Resolve  time.Duration
	Object   time.Duration
	Populate time.Duration
	Ready    time.Duration
}
