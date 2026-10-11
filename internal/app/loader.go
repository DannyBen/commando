package app

import "github.com/dannyben/commando/internal/browser"

type loadRequest struct {
	panel            int
	path, selectName string
	refresh          bool
	rename           *renameRequest
	create           *createRequest
}

type loadResult struct {
	request   loadRequest
	directory browser.Directory
	created   browser.Entry
	err       error
}

type directoryLoader struct {
	requests chan loadRequest
	results  chan loadResult
	done     chan struct{}
}

// One worker keeps reads and mutations off the UI thread and avoids concurrent
// filesystem access. Each panel allows only one pending directory read.
func newDirectoryLoader(read func(string) (browser.Directory, error), invalidate func()) *directoryLoader {
	l := &directoryLoader{
		requests: make(chan loadRequest, 3),
		results:  make(chan loadResult, 3),
		done:     make(chan struct{}),
	}
	go func() {
		for {
			select {
			case <-l.done:
				return
			case request := <-l.requests:
				var directory browser.Directory
				var created browser.Entry
				var err error
				if r := request.rename; r != nil {
					err = browser.Rename(request.path, r.oldName, r.newName)
				} else if request.create != nil {
					if request.create.folder {
						created, err = browser.CreateFolder(request.path, request.create.name)
					} else {
						created, err = browser.CreateFile(request.path, request.create.name)
					}
				} else {
					directory, err = read(request.path)
				}
				select {
				case <-l.done:
					return
				case l.results <- loadResult{request: request, directory: directory, created: created, err: err}:
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
