//go:build !windows

package browser

func shortcutData(string) ([]byte, error) { return nil, ErrTransferUnavailable }
