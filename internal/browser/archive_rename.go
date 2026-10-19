package browser

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"
)

var ErrArchiveEncoding = errors.New("unsupported archive name encoding")

func RenameArchive(location Location, oldName, newName string) error {
	adapter := ArchiveFormat(location.Path)
	if adapter == nil || !location.InArchive() || !fs.ValidPath(location.Folder) || !adapter.Capabilities().Rename {
		return fs.ErrInvalid
	}
	if !fs.ValidPath(oldName) || oldName == "." || strings.ContainsAny(oldName, `/\:`) {
		return ErrInvalidName
	}
	if err := ValidateName(newName); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	return adapter.Rename(location, oldName, newName)
}

func (zipAdapter) Rename(location Location, oldName, newName string) error {
	oldPath, newPath := path.Join(location.Folder, oldName), path.Join(location.Folder, newName)
	return rewriteZIP(location.Path, func(reader *zip.Reader, writer *zip.Writer) error {
		found, conflict := false, false
		for _, file := range reader.File {
			if file.Name == oldPath || strings.HasPrefix(file.Name, oldPath+"/") {
				found = true
			} else if file.Name == newPath || strings.HasPrefix(file.Name, newPath+"/") {
				conflict = true
			}
		}
		if !found {
			return fs.ErrNotExist
		}
		if conflict {
			return fs.ErrExist
		}
		for _, file := range reader.File {
			if file.Name == oldPath || strings.HasPrefix(file.Name, oldPath+"/") {

				changed, err := renamedZIPMember(file, newPath+strings.TrimPrefix(file.Name, oldPath), newName)
				if err != nil {
					return err
				}
				if err := writer.Copy(changed); err != nil {
					return err
				}
			} else if err := writer.Copy(file); err != nil {
				return err
			}
		}
		return nil
	})
}

func withoutUnicodePath(extra []byte) []byte {
	var kept []byte
	for len(extra) >= 4 {
		size := 4 + int(binary.LittleEndian.Uint16(extra[2:4]))
		if size > len(extra) {
			break
		}
		if binary.LittleEndian.Uint16(extra[:2]) != 0x7075 {
			kept = append(kept, extra[:size]...)
		}
		extra = extra[size:]
	}
	return append(kept, extra...)
}

// A stale Unicode Path extra field can override the new header name in other
// readers. Both rename and duplicate share this header/encoding treatment.
func renamedZIPMember(file *zip.File, name, replacement string) (*zip.File, error) {
	changed := *file
	changed.Name = name
	changed.Extra = withoutUnicodePath(file.Extra)
	if utf8.ValidString(changed.Name) && utf8.ValidString(changed.Comment) {
		changed.Flags |= 0x800
		changed.NonUTF8 = false
	} else if strings.IndexFunc(replacement, func(r rune) bool { return r > 127 }) >= 0 {
		return nil, ErrArchiveEncoding
	}
	return &changed, nil
}
