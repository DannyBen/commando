//go:build !windows

package browser

import (
	"io/fs"
	"time"
)

// Standard FileInfo has no portable creation timestamp. Do not substitute
// modification time or Unix inode-change time for a file's creation date.
func creationTime(fs.FileInfo) time.Time { return time.Time{} }
