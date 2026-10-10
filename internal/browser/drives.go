package browser

// The empty location represents the drive list, rather than a physical folder.
func ReadLocation(path string) (Directory, error) {
	if path == "" {
		return ReadDrives()
	}
	return ReadDirectory(path)
}

// Drive letters are listed alphabetically without probing the volumes.
func driveEntries(mask uint32) []Entry {
	var entries []Entry
	for letter := 0; letter < 26; letter++ {
		if mask&(1<<letter) == 0 {
			continue
		}
		root := string(rune('A'+letter)) + ":\\"
		entries = append(entries, Entry{Name: root, Path: root, Directory: true})
	}
	return entries
}
