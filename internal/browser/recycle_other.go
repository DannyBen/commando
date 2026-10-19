//go:build !windows

package browser

func recyclePaths([]string, uintptr) (removalBatch, error) {
	return removalBatch{}, ErrRecycleUnavailable
}

func deletePaths([]string, uintptr) (removalBatch, error) {
	return removalBatch{}, ErrDeleteUnavailable
}

func deleteVerifiedPaths([]string, uintptr, func(string) error) (removalBatch, error) {
	return removalBatch{}, ErrDeleteUnavailable
}
