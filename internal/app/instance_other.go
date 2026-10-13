//go:build !windows

package app

// Linux is a development host; previews bypass instance ownership entirely.
type instanceGuard struct{}

func startInstance(string) (*instanceGuard, bool, error) { return nil, true, nil }
func (*instanceGuard) close()                            {}
