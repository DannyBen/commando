package browser

import "path/filepath"

// TransferSummary counts requested source entries, independently of the
// filesystem mappings retained for reconciliation. Each entry counts once.
type TransferSummary struct {
	Requested, Completed, Skipped, Partial, Failed, Unfinished int
	cancelled                                                  bool
	failure                                                    uint32
}

type transferState uint8

const (
	transferPending transferState = iota
	transferCompleted
	transferSkipped
	transferMerged
	transferFailed
	transferCancelled
)

// Informational and decision statuses are not terminal outcomes. In particular,
// PENDING and PENDING_BATCH_COPY can precede the conflict dialog's final result.
func shellTransferState(hr uint32) transferState {
	switch hr {
	case 0, 0x00270008, 0x0027000a, 0x0027000c, 0x0027000e:
		return transferCompleted
	case 0x00270006:
		return transferMerged
	case 0x00270005:
		return transferSkipped
	case 0x80270000, 0x80270001, 0x800704c7, 0x80004004:
		return transferCancelled
	default:
		if int32(hr) < 0 {
			return transferFailed
		}
		return transferPending
	}
}

type transferRootOutcome struct {
	state                                        transferState
	hr                                           uint32
	childCompleted, childSkipped, childCancelled bool
	// Keep only unresolved child failures, so a successful retry clears its
	// earlier error. Successful/skipped children need no individual records.
	childFailures map[string]uint32
}

type transferOutcomes struct {
	roots     map[string]*transferRootOutcome
	cancelled bool
}

func newTransferOutcomes(request TransferRequest) transferOutcomes {
	t := transferOutcomes{roots: make(map[string]*transferRootOutcome)}
	for _, path := range request.Paths {
		t.roots[transferPathKey(path)] = &transferRootOutcome{}
	}
	return t
}

func (t *transferOutcomes) root(source string) (*transferRootOutcome, bool) {
	if source == "" {
		return nil, false
	}
	for path := source; ; path = filepath.Dir(path) {
		if root := t.roots[transferPathKey(path)]; root != nil {
			return root, sameTransferPath(path, source)
		}
		if filepath.Dir(path) == path {
			return nil, false
		}
	}
}

func (t *transferOutcomes) observe(source string, hr uint32) {
	root, exact := t.root(source)
	state := shellTransferState(hr)
	if root == nil || state == transferPending {
		return
	}
	t.cancelled = t.cancelled || state == transferCancelled
	if exact {
		if hr == 0x00270008 {
			// A completed direct move needs no remaining child work.
			root.childSkipped, root.childCancelled, root.childFailures = false, false, nil
		}
		// An observed completion proves this source was transferred, even if
		// another callback for a duplicate attempt later reports a skip/error.
		if root.state != transferCompleted {
			root.state, root.hr = state, hr
		}
		return
	}
	key := transferPathKey(source)
	delete(root.childFailures, key)
	switch state {
	case transferCompleted:
		root.childCompleted = true
	case transferSkipped:
		root.childSkipped = true
	case transferCancelled:
		root.childCancelled = true
	case transferFailed:
		if root.childFailures == nil {
			root.childFailures = make(map[string]uint32)
		}
		root.childFailures[key] = hr
	}
}

func (t *transferOutcomes) summary(interrupted bool) TransferSummary {
	summary := TransferSummary{Requested: len(t.roots), cancelled: t.cancelled}
	interrupted = interrupted || t.cancelled
	for _, root := range t.roots {
		failed := root.state == transferFailed || len(root.childFailures) > 0
		cancelled := root.state == transferCancelled || root.childCancelled
		skipped := root.state == transferSkipped || root.childSkipped
		written := root.state == transferCompleted || root.childCompleted
		finished := root.state == transferCompleted || root.state == transferMerged && !interrupted
		summary.cancelled = summary.cancelled || cancelled
		if root.state == transferFailed && summary.failure == 0 {
			summary.failure = root.hr
		}
		for _, hr := range root.childFailures {
			if summary.failure == 0 {
				summary.failure = hr
			}
		}
		switch {
		case written && (failed || cancelled || skipped || !finished):
			summary.Partial++
		case written:
			summary.Completed++
		case failed:
			summary.Failed++
		case cancelled:
			summary.Unfinished++
		case skipped:
			summary.Skipped++
		case root.state == transferMerged && !interrupted:
			// An empty-folder merge can finish without any child callbacks.
			summary.Completed++
		default:
			summary.Unfinished++
		}
	}
	return summary
}
