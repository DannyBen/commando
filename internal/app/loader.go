package app

import (
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/dannyben/commando/internal/browser"
)

type loadRequest struct {
	panel            int
	path, selectName string
	refresh          bool
	autoRefresh      bool
	tracked          bool
	action           actionClass
	locations        []string // Captured panel paths for create/rename alias reconciliation.
	rename           *renameRequest
	create           *createRequest
	duplicate        *duplicateRequest
	recycle          *recycleRequest
	clipboard        *fileClipboardRequest
	transfer         *transferRequest
	settingsSave     *settingsSaveRequest
	editor           *editorRequest
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
	clipboardErr      error
	transferMove      bool
	duplicatePlan     *browser.DuplicatePlan
	alreadyHere       bool
	panelUpdates      []transferPanelUpdate
	mutationDirectory string
	settingsMessage   string
	editorsStarted    int
}

type directoryLoader struct {
	requests          chan loadRequest
	results           chan loadResult
	done              chan struct{}
	recycle           func(string, []string) (browser.RecycleResult, error)
	readClipboard     func() (browser.FileClipboard, error)
	writeClipboard    func([]string, bool) (browser.FileClipboard, error)
	completeClipboard func(browser.FileClipboard, []string) (browser.FileClipboard, error)
	transfer          func(browser.TransferRequest) (browser.TransferResult, error)
	readDirectory     func(string) (browser.Directory, error)
	launchEditor      func(string, []string, string) error
	busy              atomic.Int32
	stopOnce          sync.Once
	stopped           chan struct{}
}

// One worker keeps reads and mutations off the UI thread and avoids concurrent
// filesystem access. Each panel allows only one pending directory read.
func newDirectoryLoader(read func(string) (browser.Directory, error), invalidate func(), panelCount ...int) *directoryLoader {
	count := 3
	if len(panelCount) > 0 {
		count = max(1, panelCount[0])
	}
	l := &directoryLoader{
		requests:          make(chan loadRequest, 2*count),
		results:           make(chan loadResult, count),
		done:              make(chan struct{}),
		recycle:           browser.Recycle,
		readClipboard:     browser.ReadFileClipboard,
		writeClipboard:    browser.WriteFileClipboard,
		completeClipboard: browser.CompleteFileClipboard,
		transfer:          browser.Transfer,
		stopped:           make(chan struct{}),
		readDirectory:     read,
		launchEditor:      browser.LaunchEditor,
	}
	go func() {
		defer close(l.stopped)
		for {
			select {
			case <-l.done:
				return
			default:
			}
			select {
			case <-l.done:
				return
			case request := <-l.requests:
				if request.editor != nil {
					result := loadResult{request: request}
					for _, launch := range request.editor.launches {
						if err := l.launchEditor(launch.Program, launch.Arguments, request.path); err != nil {
							result.err = err
							break
						}
						result.editorsStarted++
					}
					l.results <- result
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.settingsSave != nil {
					message, err := request.settingsSave.run()
					l.results <- loadResult{request: request, settingsMessage: message, err: err}
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.clipboard != nil || request.transfer != nil || request.duplicate != nil {
					var result loadResult
					if request.duplicate != nil {
						result = l.duplicateOperation(request)
					} else {
						result = l.fileOperation(request)
					}
					l.results <- result
					if invalidate != nil {
						invalidate()
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
						created, err = browser.CreateFileData(mutationPath, request.create.name, request.create.data)
					}
				} else if request.recycle != nil {
					// Capture alias/backing locations before recycling can remove
					// open descendants and make fresh identity resolution fail.
					result.locations = resolveFileLocations(request.recycle.locations)
					// Use the same canonical spelling for the recycled parent and
					// captured panels. Windows temporary paths can contain 8.3
					// names, and aliases may disappear during the operation.
					parent, _ := resolveFileLocation(request.path)
					recycled, err = l.recycle(request.path, request.recycle.names)
					if parent != "" && len(recycled.Names) > 0 {
						recycled.Directory = parent
					}
					sameDirectory = make([]bool, len(request.recycle.locations))
					if len(recycled.Names) > 0 {
						for pi, resolved := range result.locations {
							sameDirectory[pi] = resolved != "" && samePath(filepath.Clean(resolved), recycled.Directory)
						}
					}
				} else {
					directory, err = l.readExistingParent(request.path)
				}
				result.directory, result.created, result.recycled = directory, created, recycled
				result.sameDirectory, result.err = sameDirectory, err
				l.results <- result
				if invalidate != nil {
					invalidate()
				}
			}
		}
	}()
	return l
}

func (l *directoryLoader) close() { l.stopOnce.Do(func() { close(l.done) }) }

// Missing locations can recover at their closest surviving ancestor. Other
// read failures retain their usual feedback rather than changing the location.
// Run this only on the directory worker.
func (l *directoryLoader) readExistingParent(path string) (browser.Directory, error) {
	for {
		directory, err := l.readDirectory(path)
		if !browser.IsMissingDirectory(err) || path == "" {
			return directory, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			// No usable parent remains. Use the default root once, without looping.
			fallback := string(filepath.Separator)
			if runtime.GOOS == "windows" {
				fallback = `C:\`
			}
			return l.readDirectory(fallback)
		}
		path = parent
	}
}
