package app

import "github.com/dannyben/control/internal/browser"

type loadRequest struct {
	panel            int
	path, selectName string
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

// A single worker keeps directory reads off the UI thread and avoids concurrent
// access to the VM's shared folders. Each panel allows only one pending request.
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
				directory, err := read(request.path)
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
