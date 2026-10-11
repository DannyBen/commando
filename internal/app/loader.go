package app

import "github.com/dannyben/commando/internal/browser"

type loadRequest struct {
	panel            int
	path, selectName string
	refresh          bool
	rename           *renameRequest
}

type loadResult struct {
	request   loadRequest
	directory browser.Directory
	err       error
}

type directoryLoader struct {
	requests chan loadRequest
	results  chan loadResult
	done     chan struct{}
}

// One worker keeps reads and renames off the UI thread and avoids concurrent
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
				var err error
				if r := request.rename; r != nil {
					err = browser.Rename(request.path, r.oldName, r.newName)
				} else {
					directory, err = read(request.path)
				}
				select {
				case <-l.done:
					return
				case l.results <- loadResult{request: request, directory: directory, err: err}:
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
