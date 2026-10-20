package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/dannyben/commando/internal/browser"
)

type metadataColumn uint8

const (
	metadataNone metadataColumn = iota
	metadataExtension
	metadataSize
	metadataModified
	metadataCreated
	metadataColumnCount
)

func (m metadataColumn) stateName() string {
	switch m {
	case metadataNone:
		return "none"
	case metadataExtension:
		return "extension"
	case metadataModified:
		return "modified"
	case metadataCreated:
		return "created"
	default:
		return "size"
	}
}

func metadataFromState(name string) metadataColumn {
	switch name {
	case "none":
		return metadataNone
	case "extension":
		return metadataExtension
	case "modified":
		return metadataModified
	case "created":
		return metadataCreated
	default:
		// Missing or unknown choices retain the first-run Size default.
		return metadataSize
	}
}

func (m metadataColumn) cycle(direction int) metadataColumn {
	return metadataColumn((int(m) + direction + int(metadataColumnCount)) % int(metadataColumnCount))
}

func (m metadataColumn) heading() string {
	switch m {
	case metadataExtension:
		return "EXTENSION"
	case metadataSize:
		return "SIZE"
	case metadataModified:
		return "MODIFIED"
	case metadataCreated:
		return "CREATED"
	default:
		return ""
	}
}

// Keep the complete name in the entry; only the browsing label omits an
// extension that is already visible in the trailing metadata.
func (m metadataColumn) displayName(item entry) string {
	if item.file.Favorite {
		return strings.TrimSuffix(item.name, filepath.Ext(item.name))
	}
	if m == metadataExtension {
		if extension := m.value(item); extension != "" {
			return strings.TrimSuffix(item.name, "."+extension)
		}
	}
	return item.name
}

func (m metadataColumn) value(item entry) string {
	if item.file.Parent || item.name == ".." || item.file.Path != "" {
		return ""
	}
	switch m {
	case metadataExtension:
		if item.file.Directory {
			return ""
		}
		name := item.file.Name
		if name == "" {
			name = item.name
		}
		ext := filepath.Ext(name)
		// A leading dot alone is not a filename extension.
		if ext == name {
			return ""
		}
		return strings.TrimPrefix(ext, ".")
	case metadataSize:
		return item.size
	case metadataModified, metadataCreated:
		date := m.date(item.file)
		if date.IsZero() {
			return "?"
		}
		return date.Local().Format("2006-01-02")
	default:
		return ""
	}
}

func (m metadataColumn) date(file browser.Entry) time.Time {
	if m == metadataCreated {
		return file.Created
	}
	return file.Modified
}
