//go:build !windows

package app

func localCompletionPath(string) bool { return true }
