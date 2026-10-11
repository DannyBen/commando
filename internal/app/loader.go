package app

import (
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

type loadRequest struct {
	panel            int
	path, selectName string
	refresh          bool
	rename           *renameRequest
	create           *createRequest
	recycle          *recycleRequest
	clipboard        *fileClipboardRequest
	transfer         *transferRequest
}

type loadResult struct {
	request       loadRequest
	directory     browser.Directory
	created       browser.Entry
	recycled      browser.RecycleResult
	sameDirectory []bool
	err           error
	clipboard     browser.FileClipboard
	transferred   browser.TransferResult
	locations     []string
	sourceParents []string
	clipboardErr  error
	transferMove  bool
}

type directoryLoader struct {
	requests          chan loadRequest
	results           chan loadResult
	done              chan struct{}
	recycle           func(string, []string) (browser.RecycleResult, error)
	readClipboard     func() (browser.FileClipboard, error)
	writeClipboard    func([]string, bool) (browser.FileClipboard, error)
	completeClipboard func(browser.FileClipboard, []string) (browser.FileClipboard, error)
	transfer          func([]string, string, bool) (browser.TransferResult, error)
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
		transfer:          browser.Transfer,
	}
	go func() {
		for {
			select {
			case <-l.done:
				return
			case request := <-l.requests:
				if request.clipboard != nil || request.transfer != nil {
					result := l.fileOperation(request)
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
				var directory browser.Directory
				var created browser.Entry
				var recycled browser.RecycleResult
				var sameDirectory []bool
				var err error
				if r := request.rename; r != nil {
					err = browser.Rename(request.path, r.oldName, r.newName)
				} else if request.create != nil {
					if request.create.folder {
						created, err = browser.CreateFolder(request.path, request.create.name)
					} else {
						created, err = browser.CreateFile(request.path, request.create.name)
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
				select {
				case <-l.done:
					return
				case l.results <- loadResult{request: request, directory: directory, created: created, recycled: recycled, sameDirectory: sameDirectory, err: err}:
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
