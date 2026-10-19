package browser

import (
	"archive/zip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var ErrArchiveChanged = errors.New("archive changed during rewrite")
var ErrArchiveRecovery = errors.New("archive recovery needed")

// rewriteZIP keeps the original untouched while edit builds a sibling file.
// Existing compressed members can be copied directly; no unpack/repack cycle is
// needed. Validation checks the new ZIP structure, not unchanged payload CRCs.
func rewriteZIP(filename string, edit func(*zip.Reader, *zip.Writer) error) error {
	return rewriteZIPWith(filename, edit, replaceArchiveFile)
}

func rewriteZIPWith(filename string, edit func(*zip.Reader, *zip.Writer) error, replace func(string, string) error) error {
	resolved, err := ResolveOperationPath(filename)
	if err != nil {
		return err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return err
	}
	source, err := os.OpenFile(resolved, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer source.Close()
	before, err := source.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fs.ErrInvalid
	}
	reader, err := zip.NewReader(source, before.Size())
	if err != nil {
		return err
	}
	if err := checkZIPStructure(reader, before.Size()); err != nil {
		return err
	}
	// Prefix-bearing/self-extracting archives require a separate preservation
	// policy. Do not silently discard their executable or other leading bytes.
	var signature [4]byte
	if _, err := source.ReadAt(signature[:], 0); err != nil {
		return err
	}
	if string(signature[:]) != "PK\x03\x04" && string(signature[:]) != "PK\x05\x06" {
		return errors.ErrUnsupported
	}
	temporary, err := os.CreateTemp(filepath.Dir(resolved), ".commando-zip-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if err := temporary.Chmod(before.Mode().Perm()); err != nil {
		return err
	}
	writer := zip.NewWriter(temporary)
	if err := writer.SetComment(reader.Comment); err != nil {
		return err
	}
	if err := edit(reader, writer); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	check, err := zip.OpenReader(temporary.Name())
	if err != nil {
		if check != nil {
			check.Close()
		}
		return err
	}
	size, statErr := os.Stat(temporary.Name())
	if statErr != nil {
		check.Close()
		return statErr
	}
	err = checkZIPStructure(&check.Reader, size.Size())
	closeErr := check.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !os.SameFile(before, current) || before.Size() != current.Size() || !before.ModTime().Equal(current.ModTime()) {
		return ErrArchiveChanged
	}
	// Windows cannot replace every open file. Release our own reader first;
	// another program holding the archive may still cause a normal failure.
	if err := source.Close(); err != nil {
		return err
	}
	return replace(temporary.Name(), resolved)
}

func DeleteArchive(location Location, names []string) (RemovalResult, error) {
	adapter := ArchiveFormat(location.Path)
	if adapter == nil || !location.InArchive() || !fs.ValidPath(location.Folder) || !adapter.Capabilities().Delete || len(names) == 0 {
		return RemovalResult{}, fs.ErrInvalid
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `/\:`) || seen[name] {
			return RemovalResult{}, ErrInvalidName
		}
		seen[name] = true
	}
	return adapter.Delete(location, names)
}

func (zipAdapter) Delete(location Location, names []string) (RemovalResult, error) {
	err := rewriteZIP(location.Path, func(reader *zip.Reader, writer *zip.Writer) error {
		removed := make([]bool, len(reader.File))
		selected := make(map[string]bool, len(names))
		for _, name := range names {
			selected[name] = false
		}
		prefix := ""
		if location.Folder != "." {
			prefix = location.Folder + "/"
		}
		for i, file := range reader.File {
			if !strings.HasPrefix(file.Name, prefix) {
				continue
			}
			child, _, _ := strings.Cut(strings.TrimPrefix(file.Name, prefix), "/")
			if _, wanted := selected[child]; wanted {
				removed[i], selected[child] = true, true
			}
		}
		for _, found := range selected {
			if !found {
				return fs.ErrNotExist
			}
		}
		for i, file := range reader.File {
			if !removed[i] {
				if err := writer.Copy(file); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return RemovalResult{}, err
	}
	return RemovalResult{Directory: location.Path, Names: slices.Clone(names)}, nil
}

func checkZIPStructure(reader *zip.Reader, size int64) error {
	for _, file := range reader.File {
		offset, err := file.DataOffset()
		if err != nil {
			return err
		}
		if offset < 0 || offset > size || file.CompressedSize64 > uint64(size-offset) {
			return ErrInvalidArchive
		}
	}
	return nil
}
