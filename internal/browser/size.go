package browser

import (
	"context"
	"io/fs"
	"path/filepath"
)

type SizeResult struct {
	Bytes, Files int64
}

// CalculateSize sums regular file lengths without following directory links.
// A failed or cancelled walk never returns a partial result as a complete total.
func CalculateSize(ctx context.Context, paths []string) (SizeResult, error) {
	var total SizeResult
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return SizeResult{}, err
		}
		err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if sizeTraversalLink(info) {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if info.Mode().IsRegular() {
				total.Bytes += info.Size()
				total.Files++
			}
			return nil
		})
		if err != nil {
			return SizeResult{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return SizeResult{}, err
	}
	return total, nil
}
