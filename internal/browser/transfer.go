package browser

import (
	"errors"
	"io/fs"
	"os"
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
	Source string
	Entry  Entry
	Moved  bool
}

type TransferResult struct {
	Directory string
	Items     []TransferredItem
}

// Transfer preflights the whole batch, then transfers in clipboard order. Every
// destination is created exclusively, including if a collision appears after
// preflight. A runtime failure stops the batch and retains confirmed results.
func Transfer(paths []string, directory string, move bool) (TransferResult, error) {
	return transfer(paths, directory, move, transferOne)
}

func transfer(paths []string, directory string, move bool, perform func(string, string, bool) (bool, bool, error)) (TransferResult, error) {
	var result TransferResult
	resolved, err := ResolveOperationPath(directory)
	if err != nil {
		return result, err
	}
	target, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return result, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, &os.PathError{Op: "paste", Path: directory, Err: fs.ErrInvalid}
	}
	result.Directory = target
	sources := make([]string, 0, len(paths))
	names := make(map[string]bool)
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return result, &os.PathError{Op: "paste", Path: path, Err: fs.ErrInvalid}
		}
		source, err := ResolveOperationPath(path)
		if err != nil {
			return result, err
		}
		// Resolve aliases in the parent, preserving the final link as an entry.
		parent, err := filepath.EvalSymlinks(filepath.Dir(source))
		if err != nil {
			return result, err
		}
		source = filepath.Join(parent, filepath.Base(source))
		name := filepath.Base(source)
		if err := ValidateName(name); err != nil {
			return result, &os.PathError{Op: "paste", Path: source, Err: err}
		}
		key := strings.ToUpper(name)
		if names[key] {
			return result, &os.PathError{Op: "paste", Path: name, Err: fs.ErrExist}
		}
		names[key] = true
		if _, err := os.Lstat(filepath.Join(target, name)); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				err = fs.ErrExist
			}
			return result, &os.PathError{Op: "paste", Path: name, Err: err}
		}
		info, err := os.Lstat(source)
		if err != nil {
			return result, err
		}
		if info.IsDir() && pathWithin(source, target) {
			return result, &os.PathError{Op: "paste", Path: source, Err: ErrTransferIntoSelf}
		}
		for _, previous := range sources {
			if pathWithin(previous, source) || pathWithin(source, previous) {
				return result, ErrOverlappingSources
			}
		}
		sources = append(sources, source)
	}
	for index, source := range sources {
		name := filepath.Base(source)
		destination := filepath.Join(target, name)
		created, moved, err := perform(source, destination, move)
		if created {
			entry := Entry{Name: name}
			if info, statErr := os.Stat(destination); statErr == nil {
				entry.Directory, entry.Bytes, entry.SizeKnown, entry.Modified = info.IsDir(), info.Size(), info.Mode().IsRegular(), info.ModTime()
			}
			result.Items = append(result.Items, TransferredItem{Source: paths[index], Entry: entry, Moved: moved})
		}
		if err != nil {
			return result, &os.PathError{Op: "paste", Path: name, Err: err}
		}
	}
	return result, nil
}

func pathWithin(parent, path string) bool {
	if runtime.GOOS == "windows" {
		parent, path = strings.ToUpper(parent), strings.ToUpper(path)
	}
	rel, err := filepath.Rel(parent, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func transferOne(source, destination string, move bool) (created, moved bool, err error) {
	if move {
		if err := renameExclusive(source, destination); err == nil {
			return true, true, nil
		} else if !crossDevice(err) {
			return false, false, err
		}
	}
	return copyTransfer(source, destination, move)
}

func copyTransfer(source, destination string, move bool) (created, moved bool, err error) {
	// Publish only a complete copy. The temporary tree is ours; existing
	// destination entries are never opened, merged, removed or overwritten.
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".commando-transfer-")
	if err != nil {
		return false, false, err
	}
	defer func() { err = errors.Join(err, removeTransferStage(stage)) }()
	staged := filepath.Join(stage, filepath.Base(destination))
	var manifest []sourceEntry
	if err := copyTree(source, staged, &manifest); err != nil {
		return false, false, err
	}
	if err := renameExclusive(staged, destination); err != nil {
		return false, false, err
	}
	if move {
		if err := removeCopiedSource(manifest); err != nil {
			return true, false, err
		}
	}
	return true, move, nil
}

// Copied read-only attributes must not prevent cleanup of our private staging
// tree. WalkDir does not follow links; never chmod a link's source target.
func removeTransferStage(stage string) error {
	if err := filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return os.Chmod(path, 0700)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(stage)
}

type sourceEntry struct {
	path string
	info fs.FileInfo
}

func copyTree(source, destination string, manifest *[]sourceEntry) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	*manifest = append(*manifest, sourceEntry{source, info})
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(target, destination)
	case info.IsDir():
		if err := os.Mkdir(destination, 0700); err != nil {
			return err
		}
		children, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := copyTree(filepath.Join(source, child.Name()), filepath.Join(destination, child.Name()), manifest); err != nil {
				return err
			}
		}
		if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(destination, info.ModTime(), info.ModTime())
	case info.Mode().IsRegular():
		return copyFileExclusive(source, destination, info)
	default:
		return &os.PathError{Op: "copy", Path: source, Err: errors.ErrUnsupported}
	}
}

// Cross-volume moves copy first. Verify the complete source snapshot before
// deleting anything, then remove only recorded entries, never RemoveAll(source).
// Newly added children survive and make the final directory removal fail.
func removeCopiedSource(manifest []sourceEntry) error {
	for _, entry := range manifest {
		if err := verifySourceEntry(entry); err != nil {
			return err
		}
	}
	for i := len(manifest) - 1; i >= 0; i-- {
		entry := manifest[i]
		if !entry.info.IsDir() {
			if err := verifySourceEntry(entry); err != nil {
				return err
			}
		}
		if err := os.Remove(entry.path); err != nil {
			return err
		}
	}
	return nil
}

func verifySourceEntry(entry sourceEntry) error {
	current, err := os.Lstat(entry.path)
	if err != nil {
		return err
	}
	if !os.SameFile(entry.info, current) || entry.info.Size() != current.Size() || !entry.info.ModTime().Equal(current.ModTime()) {
		return &os.PathError{Op: "move", Path: entry.path, Err: ErrSourceChanged}
	}
	return nil
}
