package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func ExecuteTransfer(plan *TransferPlan, move bool, policy TransferPolicy) (TransferResult, error) {
	return executeTransfer(plan, move, policy, transferOne)
}

func executeTransfer(plan *TransferPlan, move bool, policy TransferPolicy, perform transferPerformer) (TransferResult, error) {
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

func executeTransferNode(node *transferNode, move bool, policy TransferPolicy, perform transferPerformer, result *TransferResult) (changed, moved bool, err error) {
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
		var outcome transferOutcome
		outcome, err = perform(node.source, node.destination, move)
		changed, moved = outcome.created, outcome.moved
		result.Changes = append(result.Changes, outcome.changes...)
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
