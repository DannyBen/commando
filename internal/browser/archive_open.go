package browser

import (
	"io/fs"
	"os"

	"github.com/dannyben/commando/internal/tempfiles"
)

// OpenArchive opens one read-only, disposable copy with the normal associated
// application. Successful launches retain the copy in system temporary storage:
// the application may still need it after the launcher or Commando exits.
// Shared temporary-file cleanup expires retained copies after seven days.
// There is deliberately no change detection or write-back to the archive.
func OpenArchive(location Location, name string, open func(string) error) error {
	if open == nil {
		return fs.ErrInvalid
	}
	stage, err := stageArchive(location, []string{name}, tempfiles.NewOpen)
	if err != nil {
		return err
	}
	filename := stage.Paths[0]
	retained := false
	defer func() {
		if !retained {
			stage.Close()
		}
	}()
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fs.ErrInvalid
	}
	// On Windows Chmod maps the write bits to FILE_ATTRIBUTE_READONLY.
	if err := os.Chmod(filename, 0400); err != nil {
		return err
	}
	if err := open(filename); err != nil {
		return err
	}
	retained = true
	return nil
}
