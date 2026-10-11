package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrTransferType = errors.New("file and folder types conflict")

type TransferPolicy uint8

const (
	TransferExclusive TransferPolicy = iota
	TransferOverwrite
	TransferSkipExisting
)

// TransferPlan binds a batch decision to inspected sources and destinations.
// Existing directories merge; only regular-file collisions require a choice.
type TransferPlan struct {
	Directory      string
	Conflicts      int
	SingleFile     bool
	Merges         bool
	Subject        Entry
	directoryInput string
	directoryInfo  fs.FileInfo
	groups         []transferGroup
}

type transferGroup struct {
	original string
	node     *transferNode
}

type transferNode struct {
	source, destination string
	info, targetInfo    fs.FileInfo
	children            []*transferNode
	conflict            *FileConflict
	merge               bool
}

func PlanTransfer(paths []string, directory string) (*TransferPlan, error) {
	return planTransfer(paths, directory, false)
}

func planTransfer(paths []string, directory string, duplicate bool) (*TransferPlan, error) {
	if len(paths) == 0 {
		return nil, fs.ErrInvalid
	}
	resolved, err := ResolveOperationPath(directory)
	if err != nil {
		return nil, err
	}
	target, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "paste", Path: directory, Err: fs.ErrInvalid}
	}
	if !os.SameFile(info, info) {
		return nil, ErrPathIdentity
	}
	plan := &TransferPlan{Directory: target, directoryInput: directory, directoryInfo: info}
	names := make(map[string]bool)
	for _, original := range paths {
		if !filepath.IsAbs(original) {
			return nil, &os.PathError{Op: "paste", Path: original, Err: fs.ErrInvalid}
		}
		source, err := ResolveOperationPath(original)
		if err != nil {
			return nil, err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(source))
		if err != nil {
			return nil, err
		}
		source = filepath.Join(parent, filepath.Base(source))
		name := filepath.Base(source)
		if err := ValidateName(name); err != nil {
			return nil, &os.PathError{Op: "paste", Path: source, Err: err}
		}
		destinationName := name
		if duplicate {
			info, err := os.Lstat(source)
			if err != nil {
				return nil, err
			}
			destinationName, err = availableCopyName(target, name, info.IsDir(), names)
			if err != nil {
				return nil, err
			}
		}
		if names[strings.ToUpper(destinationName)] {
			return nil, &os.PathError{Op: "paste", Path: source, Err: fs.ErrExist}
		}
		names[strings.ToUpper(destinationName)] = true
		for _, previous := range plan.groups {
			if pathWithin(previous.node.source, source) || pathWithin(source, previous.node.source) {
				return nil, ErrOverlappingSources
			}
		}
		before := plan.Conflicts
		node, err := plan.inspectNode(source, filepath.Join(target, destinationName))
		if err != nil {
			return nil, err
		}
		if duplicate && node.targetInfo != nil {
			// A name can arrive between suggestion and inspection. Duplicating
			// must never turn that arrival into an overwrite or folder merge.
			return nil, &os.PathError{Op: "copy", Path: node.destination, Err: fs.ErrExist}
		}
		if node.info.IsDir() && pathWithin(source, target) {
			return nil, ErrTransferIntoSelf
		}
		plan.groups = append(plan.groups, transferGroup{original: original, node: node})
		if before == 0 && plan.Conflicts > 0 {
			plan.Subject = transferEntry(node.destination, node.targetInfo)
		}
	}
	plan.SingleFile = len(plan.groups) == 1 && plan.groups[0].node.info.Mode().IsRegular()
	return plan, nil
}

func (plan *TransferPlan) inspectNode(source, destination string) (*transferNode, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return nil, &os.PathError{Op: "paste", Path: source, Err: errors.ErrUnsupported}
	}
	if !os.SameFile(info, info) {
		return nil, ErrPathIdentity
	}
	node := &transferNode{source: source, destination: destination, info: info}
	target, err := os.Lstat(destination)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if !os.SameFile(target, target) {
			return nil, ErrPathIdentity
		}
		if os.SameFile(info, target) {
			return nil, ErrSameTransferFile
		}
		node.targetInfo = target
		switch {
		case info.IsDir() && target.IsDir():
			node.merge, plan.Merges = true, true
		case info.Mode().IsRegular() && target.Mode().IsRegular():
			node.conflict = &FileConflict{Source: source, Destination: destination, Directory: filepath.Dir(destination),
				directoryInput: filepath.Dir(destination), sourcePath: source, sourceInfo: info, destinationInfo: target}
			plan.Conflicts++
		default:
			return nil, &os.PathError{Op: "paste", Path: destination, Err: ErrTransferType}
		}
	}
	// A wholly new destination subtree cannot contain collisions. Leave its
	// copying/verification to transferOne, retaining fast whole-directory moves.
	if node.merge {
		children, err := os.ReadDir(source)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if err := ValidateName(child.Name()); err != nil {
				return nil, err
			}
			nested, err := plan.inspectNode(filepath.Join(source, child.Name()), filepath.Join(destination, child.Name()))
			if err != nil {
				return nil, err
			}
			node.children = append(node.children, nested)
		}
	}
	return node, nil
}

func ExecuteTransfer(plan *TransferPlan, move bool, policy TransferPolicy) (TransferResult, error) {
	return executeTransfer(plan, move, policy, transferOne)
}

func executeTransfer(plan *TransferPlan, move bool, policy TransferPolicy, perform func(string, string, bool) (bool, bool, error)) (TransferResult, error) {
	result := TransferResult{}
	if plan == nil || policy > TransferSkipExisting {
		return result, fs.ErrInvalid
	}
	result.Directory = plan.Directory
	if policy == TransferExclusive && plan.Conflicts > 0 {
		return result, fs.ErrExist
	}
	if err := plan.verify(); err != nil {
		return result, err
	}
	for _, group := range plan.groups {
		node := group.node
		changed, moved, err := executeTransferNode(node, move, policy, perform, &result)
		if changed {
			info, _ := os.Stat(node.destination)
			if info == nil {
				info = node.info
			}
			result.Items = append(result.Items, TransferredItem{Source: group.original,
				Entry: transferEntry(node.destination, info), Moved: moved, Replaced: node.targetInfo != nil})
		}
		if err != nil {
			return result, &os.PathError{Op: "paste", Path: node.destination, Err: err}
		}
	}
	return result, nil
}

func executeTransferNode(node *transferNode, move bool, policy TransferPolicy, perform func(string, string, bool) (bool, bool, error), result *TransferResult) (changed, moved bool, err error) {
	if node.conflict != nil && policy == TransferSkipExisting {
		result.Skipped++
		return false, false, nil
	}
	if err := checkTransferSnapshot(node.source, node.info, false); err != nil {
		return false, false, err
	}
	if node.merge {
		if err := checkTransferSnapshot(node.destination, node.targetInfo, true); err != nil {
			return false, false, err
		}
		allMoved := true
		for _, child := range node.children {
			childChanged, childMoved, childErr := executeTransferNode(child, move, policy, perform, result)
			changed = changed || childChanged
			allMoved = allMoved && childMoved
			if childErr != nil {
				return changed, false, childErr
			}
		}
		if move && allMoved {
			// Only remove our now-empty directory. New or skipped children survive.
			if err := checkTransferSnapshot(node.source, node.info, true); err != nil {
				return changed, false, err
			}
			if err := os.Remove(node.source); err != nil {
				return changed, false, err
			}
			changed, moved = true, true
		}
	} else if node.conflict != nil {
		replacement, overwriteErr := OverwriteFile(node.conflict, move)
		changed = len(replacement.Items) > 0
		if changed {
			moved = replacement.Items[0].Moved
		}
		err = overwriteErr
	} else {
		changed, moved, err = perform(node.source, node.destination, move)
	}
	if changed {
		result.Changes = append(result.Changes, TransferredPath{Source: node.source, Destination: node.destination,
			Directory: node.info.IsDir(), Moved: moved})
	}
	return changed, moved, err
}

func (plan *TransferPlan) verify() error {
	resolved, err := ResolveOperationPath(plan.directoryInput)
	if err != nil {
		return err
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return err
	}
	if !sameTransferPath(resolved, plan.Directory) {
		return ErrConflictChanged
	}
	if err := checkTransferSnapshot(plan.Directory, plan.directoryInfo, true); err != nil {
		return err
	}
	for _, group := range plan.groups {
		resolved, err := ResolveOperationPath(group.original)
		if err != nil {
			return err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
		if err != nil {
			return err
		}
		if !sameTransferPath(filepath.Join(parent, filepath.Base(resolved)), group.node.source) {
			return ErrConflictChanged
		}
		if err := verifyTransferNode(group.node); err != nil {
			return err
		}
	}
	return nil
}

func verifyTransferNode(node *transferNode) error {
	if err := checkTransferSnapshot(node.source, node.info, false); err != nil {
		return err
	}
	if node.targetInfo != nil {
		if err := checkTransferSnapshot(node.destination, node.targetInfo, node.merge); err != nil {
			return err
		}
	} else if _, err := os.Lstat(node.destination); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return ErrConflictChanged
	}
	for _, child := range node.children {
		if err := verifyTransferNode(child); err != nil {
			return err
		}
	}
	return nil
}

func checkTransferSnapshot(path string, previous fs.FileInfo, identityOnly bool) error {
	current, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &os.PathError{Op: "paste", Path: path, Err: ErrConflictChanged}
		}
		return err
	}
	if !os.SameFile(previous, current) || previous.Mode() != current.Mode() ||
		!identityOnly && (previous.Size() != current.Size() || !previous.ModTime().Equal(current.ModTime())) {
		return &os.PathError{Op: "paste", Path: path, Err: ErrConflictChanged}
	}
	return nil
}

func transferEntry(path string, info fs.FileInfo) Entry {
	return Entry{Name: filepath.Base(path), Directory: info.IsDir(), Bytes: info.Size(),
		SizeKnown: info.Mode().IsRegular(), Modified: info.ModTime()}
}
