package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestTerminalFolderIsLiteralForCommandPrompt(t *testing.T) {
	for _, name := range []string{"folder with spaces", "%COMSPEC%", "100% ! & (folder)^", "Unicode-שלום"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "terminal-marker"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			command, err := terminalCommand(dir)
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the same startup command without opening a console or
			// waiting for interactive input on the Windows test runner.
			command.SysProcAttr.CreationFlags = 0
			command.SysProcAttr.HideWindow = true
			command.SysProcAttr.CmdLine = strings.Replace(command.SysProcAttr.CmdLine, " /K pushd ", " /C pushd ", 1) + " && if exist terminal-marker (exit /b 0) else (exit /b 7)"
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("terminal did not enter the literal folder: %v, %s", err, output)
			}
		})
	}
}

func TestTerminalConsoleHandles(t *testing.T) {
	const probe = "COMMANDO_TEST_CONSOLE_HANDLES"
	if os.Getenv(probe) == "1" {
		for _, id := range []uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
			handle, err := windows.GetStdHandle(id)
			var mode uint32
			if err != nil || windows.GetConsoleMode(handle, &mode) != nil {
				os.Exit(7)
			}
		}
		os.Exit(0)
	}
	command, err := terminalCommand(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	command.Path, err = os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command.Env = append(command.Env, probe+"=1")
	command.SysProcAttr.CmdLine = windows.EscapeArg(command.Path) + " -test.run=^TestTerminalConsoleHandles$"
	process, err := createConsoleProcess(command)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process.Thread)
	defer windows.CloseHandle(process.Process)
	status, err := windows.WaitForSingleObject(process.Process, 10000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		windows.TerminateProcess(process.Process, 1)
		t.Fatal("console probe did not finish", status, err)
	}
	var exit uint32
	if err := windows.GetExitCodeProcess(process.Process, &exit); err != nil || exit != 0 {
		t.Fatal("new terminal lacks interactive console handles", exit, err)
	}
}

func TestAdminTerminalStartupEntersLiteralFolder(t *testing.T) {
	for _, name := range []string{"folder with spaces", "100! & (folder)^", "Unicode-שלום"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), name)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "terminal-marker"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			resolved, err := ResolveOperationPath(dir)
			if err != nil {
				t.Fatal(err)
			}
			parameters, err := adminTerminalParameters(resolved)
			if err != nil {
				t.Fatal(err)
			}
			command, err := terminalCommand(dir)
			if err != nil {
				t.Fatal(err)
			}
			command.SysProcAttr.CreationFlags = 0
			command.SysProcAttr.HideWindow = true
			command.SysProcAttr.CmdLine = windows.EscapeArg(command.Path) + " " + strings.Replace(parameters, " /K ", " /C ", 1) + " && if exist terminal-marker (exit /b 0) else (exit /b 7)"
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("admin startup command did not enter the folder: %v, %s", err, output)
			}
		})
	}
	if _, err := adminTerminalParameters(`C:\%COMSPEC%`); err != ErrTerminalPathExpansion {
		t.Fatal("admin startup accepted a path subject to environment expansion", err)
	}
}
