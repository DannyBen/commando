//go:build !windows

package browser

// Unix development builds expose the filesystem root. Mount discovery is not
// part of the Windows drive-list feature.
func ReadDrives() (Directory, error) {
	return Directory{Drives: true, Entries: []Entry{{Name: "/", Path: "/", Directory: true}}}, nil
}
