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
	ErrFileConflictType = errors.New("conflict is not between regular files")
	ErrSameTransferFile = errors.New("source and destination are the same file")
	ErrConflictChanged  = errors.New("files changed while awaiting a decision")
)

// FileConflict is a read-only snapshot of one regular-file collision. Private
// metadata binds an overwrite decision to the entries that were inspected.
type FileConflict struct {
	Source, Destination, Directory string
	directoryInput, sourcePath     string
	sourceInfo, destinationInfo    fs.FileInfo
}

// DestinationEntry describes the inspected file without another filesystem read.
func (conflict *FileConflict) DestinationEntry() Entry {
	return Entry{Name: filepath.Base(conflict.Destination), Bytes: conflict.destinationInfo.Size(),
		SizeKnown: true, Modified: conflict.destinationInfo.ModTime()}
}

func InspectFileConflict(source, directory string) (*FileConflict, error) {
	resolved, err := ResolveOperationPath(source)
	if err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
	if err != nil {
		return nil, err
	}
	resolved = filepath.Join(parent, filepath.Base(resolved))
	if err := ValidateName(filepath.Base(resolved)); err != nil {
		return nil, err
	}
	target, err := ResolveOperationPath(directory)
	if err != nil {
		return nil, err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return nil, err
	}
	destination := filepath.Join(target, filepath.Base(resolved))
	sourceInfo, err := os.Lstat(resolved)
	if err != nil {
		return nil, err
	}
	destinationInfo, err := os.Lstat(destination)
	if err != nil {
		return nil, err
	}
	if !sourceInfo.Mode().IsRegular() || !destinationInfo.Mode().IsRegular() {
		return nil, ErrFileConflictType
	}
	// Windows FileInfo loads file IDs lazily. Capture them now, before either
	// path can change, so later SameFile checks compare the actual snapshot.
	if !os.SameFile(sourceInfo, sourceInfo) || !os.SameFile(destinationInfo, destinationInfo) {
		return nil, ErrPathIdentity
	}
	if os.SameFile(sourceInfo, destinationInfo) {
		return nil, ErrSameTransferFile
	}
	return &FileConflict{Source: source, Destination: destination, Directory: target,
		directoryInput: directory, sourcePath: resolved, sourceInfo: sourceInfo, destinationInfo: destinationInfo}, nil
}

func OverwriteFile(conflict *FileConflict, move bool) (TransferResult, error) {
	return overwriteFile(conflict, move, copyFileExclusive)
}

func overwriteFile(conflict *FileConflict, move bool, copyFile func(string, string, fs.FileInfo) error) (result TransferResult, err error) {
	if conflict == nil {
		return result, fs.ErrInvalid
	}
	result.Directory = conflict.Directory
	// Re-resolve the presented locations before using the saved backing paths.
	resolved, err := ResolveOperationPath(conflict.Source)
	if err != nil {
		return result, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
	if err != nil {
		return result, err
	}
	resolved = filepath.Join(parent, filepath.Base(resolved))
	target, err := ResolveOperationPath(conflict.directoryInput)
	if err != nil {
		return result, err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return result, err
	}
	if !sameTransferPath(resolved, conflict.sourcePath) || !sameTransferPath(target, conflict.Directory) {
		return result, ErrConflictChanged
	}
	if err := conflict.verify(); err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(conflict.Directory, ".commando-transfer-")
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, removeTransferStage(stage)) }()
	staged := filepath.Join(stage, filepath.Base(conflict.Destination))
	if err := copyFile(conflict.sourcePath, staged, conflict.sourceInfo); err != nil {
		return result, err
	}
	if err := conflict.verify(); err != nil {
		return result, err
	}
	// The only replacement primitive is here, after an explicit decision and
	// a complete staged copy. No truncation or deletion precedes the copy.
	if err := os.Rename(staged, conflict.Destination); err != nil {
		return result, err
	}
	entry := Entry{Name: filepath.Base(conflict.Destination), Bytes: conflict.sourceInfo.Size(), SizeKnown: true, Modified: conflict.sourceInfo.ModTime()}
	result.Items = []TransferredItem{{Source: conflict.Source, Entry: entry, Replaced: true}}
	if move {
		if err := checkConflictFile(conflict.sourcePath, conflict.sourceInfo); err != nil {
			return result, err
		}
		if err := os.Remove(conflict.sourcePath); err != nil {
			return result, err
		}
		result.Items[0].Moved = true
	}
	return result, nil
}

func (conflict *FileConflict) verify() error {
	if err := checkConflictFile(conflict.sourcePath, conflict.sourceInfo); err != nil {
		return err
	}
	return checkConflictFile(conflict.Destination, conflict.destinationInfo)
}

func checkConflictFile(path string, previous fs.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(previous, current) || previous.Size() != current.Size() || !previous.ModTime().Equal(current.ModTime()) || previous.Mode() != current.Mode() {
		return &os.PathError{Op: "overwrite", Path: path, Err: ErrConflictChanged}
	}
	return nil
}

func sameTransferPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
