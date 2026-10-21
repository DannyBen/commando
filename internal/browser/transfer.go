package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var ErrTransferUnavailable = errors.New("file transfers require Windows")

// TransferRequest dispatches a selection to the Windows shell. Names is used
// for duplication; ordinary copy/move lets Windows resolve destination names.
type TransferRequest struct {
	ShellClipboard   *uint32  // Expected clipboard sequence; Shell sources are acquired on the STA.
	TemporarySources bool     // Archive staging sources must not enter shell Undo history.
	OpenFolders      []string // Resolved UI locations needing descendant move tracking.
	Paths            []string
	Directory        string
	Names            []string
	Move, Duplicate  bool
	Owner            uintptr
}

type TransferredItem struct {
	Source          string
	Entry           Entry
	Moved, Replaced bool
}

type TransferredPath struct {
	Source, Destination string
	Directory, Moved    bool
}

type TransferResult struct {
	Directory          string
	Items              []TransferredItem
	Changes            []TransferredPath
	Summary            TransferSummary
	Aborted, Attempted bool
	trace              *transferBatchTrace
}

// Transfer has one execution backend. Windows owns progress, conflict/error
// dialogs and cancellation; Commando only observes and reconciles its results.
func Transfer(request TransferRequest) (result TransferResult, err error) {
	trace := beginTransferTrace(request)
	defer func() {
		result.trace = trace
		trace.result(result, err)
	}()
	prepared, err := prepareTransfer(request)
	if err != nil {
		return TransferResult{}, err
	}
	trace.record("Prepared", map[string]any{"source_count": len(prepared.Paths), "request": prepared})
	return transferNative(prepared, request.Paths, trace)
}

// Resolve SUBST and parent aliases without scanning file contents or deciding
// conflicts. The shell receives backing paths so same-volume aliases stay fast.
func prepareTransfer(request TransferRequest) (TransferRequest, error) {
	if request.ShellClipboard != nil {
		if len(request.Paths) != 0 || len(request.Names) != 0 || request.Move || request.Duplicate {
			return request, fs.ErrInvalid
		}
	} else if len(request.Paths) == 0 || len(request.Names) != 0 && len(request.Names) != len(request.Paths) || request.Duplicate && request.Move {
		return request, fs.ErrInvalid
	}
	target, err := ResolveOperationPath(request.Directory)
	if err != nil {
		return request, err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return request, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return request, err
	}
	if !info.IsDir() {
		return request, fs.ErrInvalid
	}
	request.Directory = target
	paths := make([]string, len(request.Paths))
	names := append([]string(nil), request.Names...)
	reserved := make(map[string]bool)
	for i, path := range request.Paths {
		if !filepath.IsAbs(path) {
			return request, fs.ErrInvalid
		}
		resolved, err := ResolveOperationPath(path)
		if err != nil {
			return request, err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
		if err != nil {
			return request, err
		}
		paths[i] = filepath.Join(parent, filepath.Base(resolved))
		if request.Duplicate && len(request.Names) == 0 {
			info, err := os.Lstat(paths[i])
			if err != nil {
				return request, err
			}
			name, err := availableCopyName(target, filepath.Base(paths[i]), info.IsDir(), reserved)
			if err != nil {
				return request, err
			}
			names = append(names, name)
			reserved[strings.ToUpper(name)] = true
		}
	}
	for _, name := range names {
		if err := ValidateName(name); err != nil {
			return request, err
		}
	}
	request.Paths, request.Names = paths, names
	return request, nil
}

func entryFromInfo(path string, info fs.FileInfo) Entry {
	item := Entry{Name: filepath.Base(path), Directory: info.IsDir(), Bytes: info.Size(), SizeKnown: info.Mode().IsRegular(), Modified: info.ModTime(), Created: creationTime(info)}
	setEntryAttributes(&item, info)
	return item
}

func sameTransferPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathWithin(parent, path string) bool {
	if runtime.GOOS == "windows" {
		parent, path = strings.ToUpper(parent), strings.ToUpper(path)
	}
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
