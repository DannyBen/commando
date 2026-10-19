package browser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SourcesInDirectory compares backing directories, including SUBST and parent
// aliases, while keeping a final symlink as the entry being copied.
func SourcesInDirectory(paths []string, directory string) (bool, error) {
	resolved, err := ResolveOperationPath(directory)
	if err != nil {
		return false, err
	}
	target, err := os.Stat(resolved)
	if err != nil {
		return false, err
	}
	if !target.IsDir() || len(paths) == 0 {
		return false, fs.ErrInvalid
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return false, fs.ErrInvalid
		}
		resolved, err := ResolveOperationPath(path)
		if err != nil {
			return false, err
		}
		parent, err := os.Stat(filepath.Dir(resolved))
		if err != nil {
			return false, err
		}
		if !os.SameFile(parent, target) {
			return false, nil
		}
		if _, err := os.Lstat(resolved); err != nil {
			return false, err
		}
	}
	return true, nil
}

// DuplicatePlan retains the selection and name suggestions for the inline
// editor. Filesystem copies use the native backend; archives use their adapter.
type DuplicatePlan struct {
	Archive   *ArchiveClipboard // Deferred in-archive duplication; never native paths.
	Directory string
	Paths     []string
	Names     []string
	subject   Entry
}

func PlanDuplicate(paths []string, directory string) (*DuplicatePlan, error) {
	request, err := prepareTransfer(TransferRequest{Paths: paths, Directory: directory, Duplicate: true})
	if err != nil {
		return nil, err
	}
	plan := &DuplicatePlan{Directory: directory, Paths: append([]string(nil), paths...), Names: request.Names}
	if len(paths) == 1 {
		info, err := os.Lstat(request.Paths[0])
		if err != nil {
			return nil, err
		}
		plan.subject = entryFromInfo(request.Paths[0], info)
	}
	return plan, nil
}

func availableCopyName(directory, name string, folder bool, reserved map[string]bool) (string, error) {
	for number := 1; ; number++ {
		candidate := copyName(name, folder, number)
		if reserved[strings.ToUpper(candidate)] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(directory, candidate)); errors.Is(err, fs.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
}

func (plan *DuplicatePlan) DuplicateSubject() (Entry, string) {
	if plan == nil || len(plan.Names) != 1 {
		return Entry{}, ""
	}
	return plan.subject, plan.Names[0]
}

func (plan *DuplicatePlan) Request(name string, owner uintptr) (TransferRequest, error) {
	if plan == nil || plan.Archive != nil || len(plan.Paths) != 1 {
		return TransferRequest{}, fs.ErrInvalid
	}
	if err := ValidateName(name); err != nil {
		return TransferRequest{}, err
	}
	if _, err := os.Lstat(filepath.Join(plan.Directory, name)); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			err = fs.ErrExist
		}
		return TransferRequest{}, err
	}
	return TransferRequest{Paths: plan.Paths, Directory: plan.Directory, Names: []string{name}, Duplicate: true, Owner: owner}, nil
}

func copyName(name string, folder bool, number int) string {
	ext := ""
	if !folder {
		ext = filepath.Ext(name)
		if ext == name {
			ext = ""
		}
	}
	base := strings.TrimSuffix(name, ext) + " - Copy"
	if number > 1 {
		return fmt.Sprintf("%s (%d)%s", base, number, ext)
	}
	return base + ext
}
