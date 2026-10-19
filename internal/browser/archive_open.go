package browser

import (
	"io/fs"
	"os"
)

// OpenArchive opens one read-only, disposable copy with the normal associated
// application. Successful launches retain the copy in system temporary storage:
// the application may still need it after the launcher or Commando exits.
// There is deliberately no change detection or write-back to the archive.
func OpenArchive(location Location, name string, open func(string) error) error {
	if open == nil {
		return fs.ErrInvalid
	}
	stage, err := StageArchive(location, []string{name})
	if err != nil {
		return err
	}
	filename := stage.Paths[0]
	retained := false
	defer func() {
		if !retained {
			// Windows needs the read-only attribute cleared before removal.
			os.Chmod(filename, 0600)
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
