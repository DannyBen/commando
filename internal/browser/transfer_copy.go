package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type transferOutcome struct {
	created, moved bool
	changes        []TransferredPath
}

type transferPerformer func(string, string, bool) (transferOutcome, error)

func transferOne(source, destination string, move bool) (transferOutcome, error) {
	if move {
		if err := renameExclusive(source, destination); err == nil {
			return transferOutcome{created: true, moved: true}, nil
		} else if !crossDevice(err) {
			return transferOutcome{}, err
		}
	}
	return copyTransfer(source, destination, move)
}

func copyTransfer(source, destination string, move bool) (transferOutcome, error) {
	return stagedTransfer(source, destination, move, copyTree, os.Remove)
}

// Publish a complete exclusive copy, then delete only verified source entries.
// The injected primitives let tests exercise changes during copying/deletion.
func stagedTransfer(source, destination string, move bool, copy func(string, string, *[]sourceEntry) error, remove func(string) error) (outcome transferOutcome, err error) {
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".commando-transfer-")
	if err != nil {
		return outcome, err
	}
	defer func() { err = errors.Join(err, removeTransferStage(stage)) }()
	staged := filepath.Join(stage, filepath.Base(destination))
	var manifest []sourceEntry
	if err := copy(source, staged, &manifest); err != nil {
		return outcome, err
	}
	if err := renameExclusive(staged, destination); err != nil {
		return outcome, err
	}
	outcome.created = true
	if move {
		outcome.changes, err = removeCopiedEntries(manifest, source, destination, remove)
		outcome.moved = err == nil
		if outcome.moved {
			// A complete move is represented by its root, just like rename.
			outcome.changes = nil
		}
		return outcome, err
	}
	return outcome, nil
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
	// Windows loads file IDs lazily. Freeze each ID before copying, including
	// directories and final links, while the inspected path still names it.
	if !os.SameFile(info, info) {
		return ErrPathIdentity
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
func removeCopiedEntries(manifest []sourceEntry, source, destination string, remove func(string) error) ([]TransferredPath, error) {
	var changes []TransferredPath
	for _, entry := range manifest {
		if err := verifySourceEntry(entry); err != nil {
			return changes, err
		}
	}
	for i := len(manifest) - 1; i >= 0; i-- {
		entry := manifest[i]
		// Removing children changes directory size/mtime. Recheck identity and
		// mode anyway so a replacement directory is never accepted as ours.
		if err := verifySourceSnapshot(entry, entry.info.IsDir()); err != nil {
			return changes, err
		}
		if err := remove(entry.path); err != nil {
			return changes, err
		}
		if source != "" {
			relative, _ := filepath.Rel(source, entry.path)
			changes = append(changes, TransferredPath{Source: entry.path, Destination: filepath.Join(destination, relative), Directory: entry.info.IsDir(), Moved: true})
		}
	}
	return changes, nil
}

func verifySourceEntry(entry sourceEntry) error {
	return verifySourceSnapshot(entry, false)
}

func verifySourceSnapshot(entry sourceEntry, identityOnly bool) error {
	current, err := os.Lstat(entry.path)
	if err != nil {
		return err
	}
	if !os.SameFile(entry.info, current) || entry.info.Mode() != current.Mode() || !identityOnly && (entry.info.Size() != current.Size() || !entry.info.ModTime().Equal(current.ModTime())) {
		return &os.PathError{Op: "move", Path: entry.path, Err: ErrSourceChanged}
	}
	return nil
}
