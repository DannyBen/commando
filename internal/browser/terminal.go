package browser

import "errors"

var ErrTerminalCancelled = errors.New("terminal launch cancelled")
var ErrTerminalUnavailable = errors.New("terminal launching requires Windows")
var ErrTerminalPathExpansion = errors.New("terminal folder contains percent expansion")

const terminalDirectoryVariable = "COMMANDO_TERMINAL_DIRECTORY"
