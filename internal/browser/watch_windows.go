package browser

import (
	"context"
	"os"

	"golang.org/x/sys/windows"
)

// WatchDirectory waits for native changes to the directory's immediate entries.
// The first callback establishes a fresh baseline after the watch is armed.
// Cancelling ctx stops the wait and releases both native handles.
func WatchDirectory(ctx context.Context, path string, changed func()) error {
	filter := uint32(windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
		windows.FILE_NOTIFY_CHANGE_ATTRIBUTES | windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE)
	handle, err := windows.FindFirstChangeNotification(path, false, filter)
	if err != nil {
		return err
	}
	defer windows.FindCloseChangeNotification(handle)
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	signalled := make(chan struct{})
	go func() {
		<-ctx.Done()
		windows.SetEvent(stop)
		close(signalled)
	}()
	defer func() {
		cancel()
		<-signalled
		windows.CloseHandle(stop)
	}()
	changed()
	for {
		event, err := windows.WaitForMultipleObjects([]windows.Handle{stop, handle}, false, windows.INFINITE)
		if err != nil {
			return err
		}
		if event == windows.WAIT_OBJECT_0 {
			return ctx.Err()
		}
		if event != windows.WAIT_OBJECT_0+1 {
			continue
		}
		changed()
		// Recycling moves the watched directory out from under its original
		// path. Its notification handle can still refer to the moved directory;
		// never re-arm that handle once the requested location is gone.
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return &os.PathError{Op: "watch", Path: path, Err: os.ErrNotExist}
		}
		if err := windows.FindNextChangeNotification(handle); err != nil {
			return err
		}
	}
}
