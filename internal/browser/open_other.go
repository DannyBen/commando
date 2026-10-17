//go:build !windows

package browser

import (
	"os/exec"
	"runtime"
)

// OpenFile asks the desktop to open a file. The caller owns failure feedback.
func OpenFile(path string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	return exec.Command(command, path).Run()
}
