package browser

import (
	"archive/zip"
	"io/fs"
	"path"
	"strings"
)

func (reference *ArchiveClipboard) Remove(names []string) error {
	if err := reference.check(); err != nil {
		return err
	}
	allowed := make(map[string]bool, len(reference.Names))
	for _, name := range reference.Names {
		allowed[name] = true
	}
	for _, name := range names {
		if !allowed[name] {
			return fs.ErrInvalid
		}
	}
	if !reference.Location.Capabilities().Delete {
		return fs.ErrInvalid
	}
	_, err := ArchiveFormat(reference.Location.Path).Delete(reference.Location, names, &reference.Stamp)
	return err
}

func MoveArchive(reference *ArchiveClipboard, target Location) error {
	if err := reference.check(); err != nil {
		return err
	}
	if !target.InArchive() || !fs.ValidPath(target.Folder) || !target.Capabilities().Add || !reference.Location.Capabilities().Delete {
		return fs.ErrInvalid
	}
	stamp, err := readArchiveClipboardStamp(target.Path)
	if err != nil {
		return err
	}
	if stamp.Identity != reference.Stamp.Identity {
		return fs.ErrInvalid
	}
	if reference.Location.Folder == target.Folder {
		return nil
	}
	return ArchiveFormat(reference.Location.Path).Move(reference.Location, reference.Names, target, &reference.Stamp)
}

// Moving within one ZIP rewrites member names once, with no extraction or
// intermediate copy. A collision or an invalid subtree leaves it unchanged.
func (zipAdapter) Move(source Location, names []string, target Location, expected *archiveClipboardStamp) error {
	return rewriteZIP(source.Path, func(reader *zip.Reader, writer *zip.Writer) error {
		stamp, err := readArchiveClipboardStamp(source.Path)
		if err != nil {
			return err
		}
		if stamp != *expected {
			return ErrArchiveClipboardChanged
		}
		if source.Folder == target.Folder {
			return nil
		}
		prefix := ""
		if source.Folder != "." {
			prefix = source.Folder + "/"
		}
		roots, found := map[string]string{}, map[string]bool{}
		for _, name := range names {
			oldPath, newPath := path.Join(source.Folder, name), path.Join(target.Folder, name)
			if target.Folder == oldPath || strings.HasPrefix(target.Folder, oldPath+"/") {
				return fs.ErrInvalid
			}
			roots[oldPath] = newPath
		}
		targetFound := target.Folder == "."
		targetPrefix := ""
		if target.Folder != "." {
			targetPrefix = target.Folder + "/"
		}
		selected := make(map[string]bool, len(names))
		for _, name := range names {
			selected[name] = true
		}
		for _, member := range reader.File {
			clean := strings.TrimSuffix(member.Name, "/")
			if clean == target.Folder && member.FileInfo().IsDir() || strings.HasPrefix(clean, target.Folder+"/") {
				targetFound = true
			}
			if strings.HasPrefix(member.Name, targetPrefix) {
				child, _, _ := strings.Cut(strings.TrimPrefix(member.Name, targetPrefix), "/")
				if selected[child] {
					return fs.ErrExist
				}
			}
			if strings.HasPrefix(member.Name, prefix) {
				child, _, _ := strings.Cut(strings.TrimPrefix(member.Name, prefix), "/")
				oldPath := prefix + child
				if _, ok := roots[oldPath]; ok {
					found[oldPath] = true
				}
			}
		}
		if !targetFound {
			return fs.ErrNotExist
		}
		for oldPath := range roots {
			if !found[oldPath] {
				return fs.ErrNotExist
			}
		}
		for _, member := range reader.File {
			changed := member
			if strings.HasPrefix(member.Name, prefix) {
				child, _, _ := strings.Cut(strings.TrimPrefix(member.Name, prefix), "/")
				oldPath := prefix + child
				if newPath, ok := roots[oldPath]; ok {
					changed, err = renamedZIPMember(member, newPath+strings.TrimPrefix(member.Name, oldPath), child)
					if err != nil {
						return err
					}
				}
			}
			if err := writer.Copy(changed); err != nil {
				return err
			}
		}
		return nil
	})
}
