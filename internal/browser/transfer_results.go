package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Observations describe completed shell callbacks, not instructions to mutate
// files. Cancellation accepts the filesystem as Windows left it.
type transferObservation struct {
	source, destination string
	directory           bool
}

// Classify native Windows completion statuses separately from COM so the
// success/skip boundary is exercised by development-platform tests as well.
func shellTransferCompleted(hr uint32) bool {
	state := shellTransferState(hr)
	return state == transferCompleted || state == transferMerged
}

func transferResults(request TransferRequest, originals []string, observations []transferObservation, aborted bool, summary TransferSummary) TransferResult {
	result := TransferResult{Directory: request.Directory, Attempted: true, Aborted: aborted || summary.cancelled, Summary: summary}
	// Index completions once, including the first inferred destination for a
	// partially transferred root. Large selections must not rescan every
	// observation for each completed or skipped source.
	roots := make(map[string]bool, len(request.Paths))
	for _, source := range request.Paths {
		roots[transferPathKey(source)] = true
	}
	exact := make(map[string]transferObservation, len(observations))
	inferred := make(map[string]transferObservation)
	for _, observation := range observations {
		key := transferPathKey(observation.source)
		if _, exists := exact[key]; !exists {
			exact[key] = observation
		}
		path, destination := observation.source, observation.destination
		for filepath.Dir(path) != path {
			path, destination = filepath.Dir(path), filepath.Dir(destination)
			key := transferPathKey(path)
			if _, exists := inferred[key]; roots[key] && !exists {
				inferred[key] = transferObservation{destination: destination, directory: true}
			}
		}
		moved := false
		if request.Move {
			_, err := os.Lstat(observation.source)
			moved = errors.Is(err, fs.ErrNotExist)
		}
		result.Changes = append(result.Changes, TransferredPath{Source: observation.source, Destination: observation.destination, Directory: observation.directory, Moved: moved})
	}
	for i, source := range request.Paths {
		observation, ok := exact[transferPathKey(source)]
		if !ok {
			observation = inferred[transferPathKey(source)]
		}
		destination, folder := observation.destination, observation.directory
		if destination == "" {
			continue
		}
		entry := Entry{Name: filepath.Base(destination), Directory: folder}
		if info, err := os.Lstat(destination); err == nil {
			entry = entryFromInfo(destination, info)
		}
		moved := false
		if request.Move {
			_, err := os.Lstat(source)
			moved = errors.Is(err, fs.ErrNotExist)
		}
		result.Items = append(result.Items, TransferredItem{Source: originals[i], Entry: entry, Moved: moved})
	}
	return result
}
