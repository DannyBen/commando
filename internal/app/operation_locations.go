package app

import (
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

// Resolve cached panel paths only on the directory worker, before mutations
// can make an alias or an open descendant disappear.
func resolveFileLocations(paths []string) []string {
	locations := make([]string, len(paths))
	for pi, path := range paths {
		if path != "" {
			locations[pi], _ = resolveFileLocation(path)
		}
	}
	return locations
}

// A result must not update a panel which has navigated since submission.
func (v *view) nameResultApplies(result loadResult, panel int) bool {
	p := &v.panels[panel]
	return !p.drives && panel < len(result.request.locations) && panel < len(result.locations) &&
		result.locations[panel] != "" && samePath(p.path, result.request.locations[panel])
}

func resolveFileLocation(path string) (string, error) {
	resolved, err := browser.ResolveOperationPath(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(resolved)
}

func movedLocationSuffix(source, path string) (string, bool) {
	if samePath(source, path) {
		return "", true
	}
	if len(path) > len(source) && samePath(source, path[:len(source)]) && path[len(source)] == filepath.Separator {
		return path[len(source)+1:], true
	}
	return "", false
}
