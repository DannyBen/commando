package browser

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// LaunchAction starts an executable directly. An interactive console uses the
// same native creation path as Terminal here so its standard handles work.
// Shell syntax is available by explicitly configuring a shell as the program.
func LaunchAction(program string, arguments []string, directory string, terminal bool) error {
	if !terminal {
		return LaunchEditor(program, arguments, directory)
	}
	command, err := actionConsoleCommand(program, arguments, directory)
	if err != nil {
		return err
	}
	process, err := createConsoleProcess(command)
	if err != nil {
		return err
	}
	windows.CloseHandle(process.Thread)
	windows.CloseHandle(process.Process)
	return nil
}

func actionConsoleCommand(program string, arguments []string, directory string) (*exec.Cmd, error) {
	command := exec.Command(program, arguments...)
	if command.Err != nil {
		return nil, command.Err
	}
	command.Dir = directory
	line := windows.EscapeArg(command.Path)
	for _, argument := range arguments {
		line += " " + windows.EscapeArg(argument)
	}
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	// Keep the complete environment. createConsoleProcess provides fresh
	// interactive handles rather than os/exec's redirected standard handles.
	return command, nil
}
