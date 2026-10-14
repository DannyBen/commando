package browser

import "os/exec"

// LaunchEditor starts a configured executable directly, without a shell. Reap
// it independently: file browsing must not wait for the editor to close.
func LaunchEditor(program string, arguments []string, directory string) error {
	command := exec.Command(program, arguments...)
	command.Dir = directory
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
