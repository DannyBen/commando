//go:build !windows

package browser

import "os/exec"

func hideGitWindow(*exec.Cmd) {}
