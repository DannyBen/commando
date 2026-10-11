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

// PlanDuplicate uses the ordinary inspected, exclusive transfer machinery with
// available copy names. It never merges folders or approves replacements.
func PlanDuplicate(paths []string, directory string) (*TransferPlan, error) {
	return planTransfer(paths, directory, true)
}

func availableCopyName(directory, name string, folder bool, reserved map[string]bool) (string, error) {
	ext := ""
	if !folder {
		ext = filepath.Ext(name)
		if ext == name {
			ext = ""
		}
	}
	base := strings.TrimSuffix(name, ext) + " - Copy"
	for number := 1; ; number++ {
		candidate := base + ext
		if number > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", base, number, ext)
		}
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

// DuplicateSubject describes the original entry and its suggested new name for
// the shared inline name editor. Only a single new destination is editable.
func (plan *TransferPlan) DuplicateSubject() (Entry, string) {
	if plan == nil || len(plan.groups) != 1 {
		return Entry{}, ""
	}
	node := plan.groups[0].node
	return transferEntry(node.source, node.info), filepath.Base(node.destination)
}

// DuplicateWithName binds the edited name to the inspected source/directory.
// A name appearing since inspection is an error, never an overwrite choice.
func DuplicateWithName(plan *TransferPlan, name string) (TransferResult, error) {
	if err := ValidateName(name); err != nil {
		return TransferResult{}, err
	}
	if plan == nil || len(plan.groups) != 1 || plan.Conflicts != 0 || plan.Merges {
		return TransferResult{}, fs.ErrInvalid
	}
	destination := filepath.Join(plan.Directory, name)
	if _, err := os.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			err = fs.ErrExist
		}
		return TransferResult{}, err
	}
	copyPlan := *plan
	copyNode := *plan.groups[0].node
	copyNode.destination = destination
	copyPlan.groups = []transferGroup{{original: plan.groups[0].original, node: &copyNode}}
	return ExecuteTransfer(&copyPlan, false, TransferExclusive)
}
