package browser

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func hideGitWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
