package browser

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrTransferIntoSelf   = errors.New("cannot transfer a folder into itself")
	ErrOverlappingSources = errors.New("overlapping transfer sources")
	ErrSourceChanged      = errors.New("source changed during transfer")
)

type TransferredItem struct {
	Source   string
	Entry    Entry
	Moved    bool
	Replaced bool
}

type TransferResult struct {
	Directory string
	Items     []TransferredItem
	Changes   []TransferredPath
	Skipped   int
}

// TransferredPath records whole entries and confirmed child changes during
// partial moves and folder merges, so open descendants can follow completed moves.
type TransferredPath struct {
	Source, Destination string
	Directory, Moved    bool
}

// Transfer uses the same inspected pipeline as transfers awaiting a conflict
// decision. Without an overwrite policy, file collisions refuse the whole batch.
func Transfer(paths []string, directory string, move bool) (TransferResult, error) {
	return transfer(paths, directory, move, transferOne)
}

func transfer(paths []string, directory string, move bool, perform transferPerformer) (TransferResult, error) {
	plan, err := PlanTransfer(paths, directory)
	if err != nil {
		return TransferResult{}, err
	}
	return executeTransfer(plan, move, TransferExclusive, perform)
}

func pathWithin(parent, path string) bool {
	if runtime.GOOS == "windows" {
		parent, path = strings.ToUpper(parent), strings.ToUpper(path)
	}
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
