//go:build !windows

package browser

func recyclePaths([]string) (int, error) {
	return 0, ErrRecycleUnavailable
}

func deletePaths([]string) (int, error) {
	return 0, ErrDeleteUnavailable
}
