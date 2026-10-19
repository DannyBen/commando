package app

import (
	"path"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

func (l *directoryLoader) archiveRenameOperation(request loadRequest) loadResult {
	result := loadResult{request: request}
	locations := request.rename.archiveLocations
	paths := make([]string, len(locations))
	for i, location := range locations {
		paths[i] = location.Path
	}
	result.locations = resolveFileLocations(paths)
	original, err := resolveFileLocation(request.path)
	if err != nil {
		result.err = err
		return result
	}
	result.mutationDirectory = original
	result.err = browser.RenameArchive(browser.Location{Path: original, Folder: request.archiveFolder}, request.rename.oldName, request.rename.newName)
	if result.err == nil {
		l.refreshArchivePanels(&result, locations, original)
	}
	return result
}

func renamedArchiveFolder(folder string, request loadRequest) string {
	oldPath := path.Join(request.archiveFolder, request.rename.oldName)
	if folder == oldPath || strings.HasPrefix(folder, oldPath+"/") {
		return path.Join(request.archiveFolder, request.rename.newName) + strings.TrimPrefix(folder, oldPath)
	}
	return folder
}
