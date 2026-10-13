package app

import "github.com/dannyben/commando/internal/feedback"

// Explicit action classes centralize busy policy for every input route. Browsing
// is available while the worker is busy; file and file-clipboard writes wait.
type actionClass uint8

const (
	actionMutation actionClass = iota // The default is conservative.
	actionClipboard
	actionBrowse
)

func (v *view) actionAllowed(class actionClass) bool {
	if class == actionBrowse {
		return true
	}
	return !v.interaction.operationsHeld() && !v.interaction.mutationsBlocked() && !v.transferPending && !v.mutationsBusy()
}

// Features declare a supported foreground state and action class, rather than
// listing competing editor or modal types at every input entry point.
func (v *view) canStartInteraction(index int, class actionClass) bool {
	return v.interaction.idle() && v.canOfferInteraction(index, class)
}

// Command discovery checks readiness while its palette still owns foreground.
// Dispatch finishes that owner before canStartInteraction admits the action.
func (v *view) canOfferInteraction(index int, class actionClass) bool {
	return index >= 0 && index < len(v.panels) &&
		!v.panels[index].loading && v.pendingNameDraft == nil && v.actionAllowed(class)
}

func (v *view) mutationsBusy() bool {
	return v.loader != nil && v.loader.busy.Load() != 0
}

// The worker count remains held until the UI has reconciled its result. Reads
// and clipboard captures can precede a mutation in the same input event batch.
func (l *directoryLoader) submit(request loadRequest) bool {
	select {
	case <-l.done:
		return false
	default:
	}
	if request.action != actionBrowse && request.action != actionClipboard {
		if !l.busy.CompareAndSwap(0, 1) {
			return false
		}
		request.tracked = true
	} else if request.action == actionClipboard && l.busy.Load() != 0 {
		return false
	}
	select {
	case l.requests <- request:
		return true
	case <-l.done:
	default:
	}
	// A workspace can grow beyond its initial queue capacity. Never block the
	// UI behind a worker waiting for the UI to consume its results.
	if request.tracked {
		l.busy.Add(-1)
	}
	return false
}

func (l *directoryLoader) complete(request loadRequest) {
	if request.tracked {
		l.busy.Add(-1)
	}
}

// A destroyed window cannot discard an active operation's completion. Stop
// accepting queued work, wait for the worker, and reconcile before saving state.
func (v *view) finishOperations() {
	v.finishNative()
	if v.monitor != nil {
		v.monitor.close()
		v.monitor = nil
	}
	if v.loader == nil {
		return
	}
	v.loader.close()
	for {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-v.loader.stopped:
			v.pollLoads()
			return
		}
	}
}

func (v *view) requestClose() bool {
	v.pollLoads()
	if !v.mutationsBusy() && !v.interaction.operationsHeld() {
		return true
	}
	v.showFeedback(v.active, feedback.Info, "File operation in progress")
	return false
}
