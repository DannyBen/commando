package browser

import (
	"archive/zip"
	"io/fs"
	"path"
	"slices"
	"strings"
)

func PlanArchiveDuplicate(location Location, names []string) (*DuplicatePlan, error) {
	if !location.Capabilities().Add {
		return nil, fs.ErrInvalid
	}
	reference, err := CaptureArchiveClipboard(location, names)
	if err != nil {
		return nil, err
	}
	directory, err := ReadArchive(reference.Location)
	if err != nil {
		return nil, err
	}
	entries := map[string]Entry{}
	reserved := map[string]bool{}
	for _, entry := range directory.Entries {
		entries[entry.Name], reserved[entry.Name] = entry, true
	}
	plan := &DuplicatePlan{Directory: reference.Location.Path, Archive: reference}
	for _, name := range reference.Names {
		entry, ok := entries[name]
		if !ok || entry.Parent {
			return nil, fs.ErrNotExist
		}
		for number := 1; ; number++ {
			candidate := copyName(name, entry.Directory, number)
			if reserved[candidate] {
				continue
			}
			plan.Names = append(plan.Names, candidate)
			reserved[candidate] = true
			break
		}
		plan.subject = entry
	}
	return plan, nil
}

func DuplicateArchive(plan *DuplicatePlan, name string) (Entry, error) {
	if plan == nil || plan.Archive == nil || len(plan.Names) != len(plan.Archive.Names) {
		return Entry{}, fs.ErrInvalid
	}
	if err := plan.Archive.check(); err != nil {
		return Entry{}, err
	}
	location := plan.Archive.Location
	adapter := ArchiveFormat(location.Path)
	if adapter == nil || !location.Capabilities().Add {
		return Entry{}, fs.ErrInvalid
	}
	destinations := slices.Clone(plan.Names)
	if len(destinations) == 1 {
		destinations[0] = name
	}
	seen := map[string]bool{}
	for _, destination := range destinations {
		if err := ValidateName(destination); err != nil {
			return Entry{}, err
		}
		if seen[destination] {
			return Entry{}, fs.ErrExist
		}
		seen[destination] = true
	}
	if err := adapter.Duplicate(location, plan.Archive.Names, destinations); err != nil {
		return Entry{}, err
	}
	if len(destinations) != 1 {
		return Entry{}, nil
	}
	created := plan.subject
	created.Name = destinations[0]
	return created, nil
}

func (zipAdapter) Duplicate(location Location, names, destinations []string) error {
	return rewriteZIP(location.Path, func(reader *zip.Reader, writer *zip.Writer) error {
		prefix := ""
		if location.Folder != "." {
			prefix = location.Folder + "/"
		}
		replacements := map[string]string{}
		targets := map[string]bool{}
		found := map[string]bool{}
		for i, name := range names {
			replacements[name], targets[destinations[i]] = destinations[i], true
		}
		for _, member := range reader.File {
			if !strings.HasPrefix(member.Name, prefix) {
				continue
			}
			child, _, _ := strings.Cut(strings.TrimPrefix(member.Name, prefix), "/")
			if targets[child] {
				return fs.ErrExist
			}
			if _, wanted := replacements[child]; wanted {
				found[child] = true
			}
		}
		for _, name := range names {
			if !found[name] {
				return fs.ErrNotExist
			}
		}
		for _, member := range reader.File {
			if err := writer.Copy(member); err != nil {
				return err
			}
		}
		for _, member := range reader.File {
			if !strings.HasPrefix(member.Name, prefix) {
				continue
			}
			child, _, _ := strings.Cut(strings.TrimPrefix(member.Name, prefix), "/")
			replacement, wanted := replacements[child]
			if !wanted {
				continue
			}
			name := path.Join(location.Folder, replacement) + strings.TrimPrefix(member.Name, prefix+child)
			changed, err := renamedZIPMember(member, name, replacement)
			if err != nil {
				return err
			}
			if err := writer.Copy(changed); err != nil {
				return err
			}
		}
		return nil
	})
}
