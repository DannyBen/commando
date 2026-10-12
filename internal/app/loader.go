package app

import (
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

type loadRequest struct {
	panel            int
	path, selectName string
	refresh          bool
	locations        []string // Captured panel paths for create/rename alias reconciliation.
	rename           *renameRequest
	create           *createRequest
	duplicate        *duplicateRequest
	recycle          *recycleRequest
	clipboard        *fileClipboardRequest
	transfer         *transferRequest
}

type loadResult struct {
	request           loadRequest
	directory         browser.Directory
	created           browser.Entry
	recycled          browser.RecycleResult
	sameDirectory     []bool
	err               error
	clipboard         browser.FileClipboard
	transferred       browser.TransferResult
	locations         []string
	sourceParents     []string
	clipboardErr      error
	transferMove      bool
	conflictPlan      *browser.TransferPlan
	duplicatePlan     *browser.TransferPlan
	alreadyHere       bool
	panelUpdates      []transferPanelUpdate
	mutationDirectory string
}

type directoryLoader struct {
	requests          chan loadRequest
	results           chan loadResult
	done              chan struct{}
	recycle           func(string, []string) (browser.RecycleResult, error)
	readClipboard     func() (browser.FileClipboard, error)
	writeClipboard    func([]string, bool) (browser.FileClipboard, error)
	completeClipboard func(browser.FileClipboard, []string) (browser.FileClipboard, error)
	planTransfer      func([]string, string) (*browser.TransferPlan, error)
	executeTransfer   func(*browser.TransferPlan, bool, browser.TransferPolicy) (browser.TransferResult, error)
	readDirectory     func(string) (browser.Directory, error)
}

// One worker keeps reads and mutations off the UI thread and avoids concurrent
// filesystem access. Each panel allows only one pending directory read.
func newDirectoryLoader(read func(string) (browser.Directory, error), invalidate func()) *directoryLoader {
	l := &directoryLoader{
		requests:          make(chan loadRequest, 3),
		results:           make(chan loadResult, 3),
		done:              make(chan struct{}),
		recycle:           browser.Recycle,
		readClipboard:     browser.ReadFileClipboard,
		writeClipboard:    browser.WriteFileClipboard,
		completeClipboard: browser.CompleteFileClipboard,
		planTransfer:      browser.PlanTransfer,
		executeTransfer:   browser.ExecuteTransfer,
		readDirectory:     read,
	}
	go func() {
		for {
			select {
			case <-l.done:
				return
			case request := <-l.requests:
				if request.clipboard != nil || request.transfer != nil || request.duplicate != nil {
					var result loadResult
					if request.duplicate != nil {
						result = l.duplicateOperation(request)
					} else {
						result = l.fileOperation(request)
					}
					select {
					case <-l.done:
						return
					case l.results <- result:
						if invalidate != nil {
							invalidate()
						}
					}
					continue
				}
				result := loadResult{request: request}
				mutationPath := request.path
				var resolveErr error
				if request.rename != nil || request.create != nil {
					result.locations = resolveFileLocations(request.locations)
					mutationPath, resolveErr = resolveFileLocation(request.path)
					result.mutationDirectory = mutationPath
				}
				var directory browser.Directory
				var created browser.Entry
				var recycled browser.RecycleResult
				var sameDirectory []bool
				var err error
				if resolveErr != nil {
					err = resolveErr
				} else if r := request.rename; r != nil {
					err = browser.Rename(mutationPath, r.oldName, r.newName)
				} else if request.create != nil {
					if request.create.folder {
						created, err = browser.CreateFolder(mutationPath, request.create.name)
					} else {
						created, err = browser.CreateFile(mutationPath, request.create.name)
					}
				} else if request.recycle != nil {
					recycled, err = l.recycle(request.path, request.recycle.names)
					sameDirectory = make([]bool, len(request.recycle.locations))
					if len(recycled.Names) > 0 {
						for pi, path := range request.recycle.locations {
							if path == "" {
								continue
							}
							resolved, resolveErr := browser.ResolveOperationPath(path)
							sameDirectory[pi] = resolveErr == nil && samePath(filepath.Clean(resolved), recycled.Directory)
						}
					}
				} else {
					directory, err = read(request.path)
				}
				result.directory, result.created, result.recycled = directory, created, recycled
				result.sameDirectory, result.err = sameDirectory, err
				select {
				case <-l.done:
					return
				case l.results <- result:
					if invalidate != nil {
						invalidate()
					}
				}
			}
		}
	}()
	return l
}

func (l *directoryLoader) close() { close(l.done) }
