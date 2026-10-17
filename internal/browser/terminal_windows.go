package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The folder is data in the child's environment, never text in a shell command.
// cmd expands it once; delayed expansion stays off so %, ! and metacharacters
// in names remain literal. pushd also supports UNC folders via a temporary drive.
func terminalCommand(directory string) (*exec.Cmd, error) {
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("terminal folder is not a directory")
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	program := filepath.Join(system, "cmd.exe")
	command := exec.Command(program)
	command.Dir = system
	command.Env = append(os.Environ(), terminalDirectoryVariable+"="+directory)
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_CONSOLE,
		CmdLine:       windows.EscapeArg(program) + ` /D /E:ON /V:OFF /K pushd "%` + terminalDirectoryVariable + `%"`,
	}
	return command, nil
}

func startTerminal(directory string) error {
	command, err := terminalCommand(directory)
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

// os/exec installs redirected standard handles even with CREATE_NEW_CONSOLE.
// Leave STARTF_USESTDHANDLES unset so Windows supplies interactive console I/O.
func createConsoleProcess(command *exec.Cmd) (windows.ProcessInformation, error) {
	var process windows.ProcessInformation
	program, err := windows.UTF16PtrFromString(command.Path)
	if err != nil {
		return process, err
	}
	line, err := windows.UTF16PtrFromString(command.SysProcAttr.CmdLine)
	if err != nil {
		return process, err
	}
	directory, err := windows.UTF16PtrFromString(command.Dir)
	if err != nil {
		return process, err
	}
	environment := utf16.Encode([]rune(strings.Join(command.Environ(), "\x00") + "\x00\x00"))
	startup := windows.StartupInfo{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	err = windows.CreateProcess(program, line, nil, nil, false,
		windows.CREATE_NEW_CONSOLE|windows.CREATE_UNICODE_ENVIRONMENT,
		&environment[0], directory, &startup, &process)
	runtime.KeepAlive(environment)
	return process, err
}

// Elevate the system command prompt directly. Resolve SUBST in the caller
// context, before elevation loses access to its drive aliases.
func LaunchTerminal(directory string, admin bool, owner uintptr) error {
	if !admin {
		return startTerminal(directory)
	}
	directory, err := ResolveOperationPath(directory)
	if err != nil {
		return err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	arguments, err := adminTerminalParameters(directory)
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(filepath.Join(system, "cmd.exe"))
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(arguments)
	if err != nil {
		return err
	}
	workingDirectory, err := windows.UTF16PtrFromString(system)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
		return err
	}
	defer windows.CoUninitialize()
	const noAsync, noUI = 0x100, 0x400
	info := shellExecuteInfo{mask: noAsync | noUI, window: windows.Handle(owner), verb: verb, file: file, parameters: parameters, directory: workingDirectory, show: windows.SW_SHOWNORMAL}
	info.size = uint32(unsafe.Sizeof(info))
	result, _, callErr := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	if result == 0 {
		if callErr == windows.ERROR_CANCELLED {
			return ErrTerminalCancelled
		}
		return callErr
	}
	return nil
}

// Percent expansion cannot be disabled by /V:OFF or quoting. Refuse it rather
// than launching the terminal in a different folder after variable expansion.
func adminTerminalParameters(directory string) (string, error) {
	if strings.Contains(directory, "%") {
		return "", ErrTerminalPathExpansion
	}
	return `/D /E:ON /V:OFF /K pushd "` + directory + `"`, nil
}
