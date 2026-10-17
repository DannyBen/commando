//go:build !windows

package browser

func LaunchTerminal(directory string, admin bool, owner uintptr) error {
	return ErrTerminalUnavailable
}
