package app

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

type transferPanelUpdate struct {
	needed    bool
	path      string
	directory browser.Directory
	err       error
}

// Read affected open folders on the directory worker, including descendants of
// partial merges. UI reconciliation retains their surviving marks and current row.
func (l *directoryLoader) refreshTransferPanels(result *loadResult) {
	result.panelUpdates = make([]transferPanelUpdate, len(result.locations))
	for pi, original := range result.locations {
		if original == "" {
			continue
		}
		path := original
		// Refresh through an unchanged display alias so parent recovery follows
		// the user's directory tree. Confirmed moves use their new backing path.
		locations := result.request.locations
		if result.request.transfer != nil {
			locations = result.request.transfer.locations
		} else if result.request.removal != nil {
			locations = result.request.removal.locations
		}
		if pi < len(locations) && locations[pi] != "" {
			path = locations[pi]
		}
		// Windows may leave partial changes even when no successful item callback
		// was delivered. Refresh every captured open folder after an attempted batch.
		longest := -1
		for _, change := range result.transferred.Changes {
			if change.Moved && change.Directory && len(change.Source) > longest {
				if relative, ok := movedLocationSuffix(change.Source, original); ok {
					path, longest = filepath.Join(change.Destination, relative), len(change.Source)
				}
			}
		}
		update := transferPanelUpdate{needed: true}
		update.directory, update.err = l.readExistingParent(path)
		if longest >= 0 {
			update.path = path
		}
		result.panelUpdates[pi] = update
	}
}

func transferNameKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}
