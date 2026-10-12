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
	for _, observation := range observations {
		moved := false
		if request.Move {
			_, err := os.Lstat(observation.source)
			moved = errors.Is(err, fs.ErrNotExist)
		}
		result.Changes = append(result.Changes, TransferredPath{Source: observation.source, Destination: observation.destination, Directory: observation.directory, Moved: moved})
	}
	for i, source := range request.Paths {
		destination := ""
		folder := false
		for _, observation := range observations {
			if sameTransferPath(observation.source, source) {
				destination, folder = observation.destination, observation.directory
				break
			}
		}
		if destination == "" {
			for _, observation := range observations {
				if !pathWithin(source, observation.source) {
					continue
				}
				// A partly transferred folder may have only child completions.
				relative, _ := filepath.Rel(source, observation.source)
				destination = observation.destination
				for relative != "." {
					destination = filepath.Dir(destination)
					relative = filepath.Dir(relative)
				}
				folder = true
				break
			}
		}
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
