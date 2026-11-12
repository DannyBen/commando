//go:build !windows

package browser

func LaunchAction(program string, arguments []string, directory string, terminal bool) error {
	if terminal {
		return ErrTerminalUnavailable
	}
	return LaunchEditor(program, arguments, directory)
}
