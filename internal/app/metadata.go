package app

import (
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
)

type metadataColumn uint8

const (
	metadataNone metadataColumn = iota
	metadataExtension
	metadataSize
	metadataModified
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
	default:
		return ""
	}
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
	case metadataModified:
		if item.file.Modified.IsZero() {
			return "?"
		}
		return item.file.Modified.Local().Format("2006-01-02")
	default:
		return ""
	}
}

// Measure a bounded column using the configured font, including its heading.
// None reserves no width. Long extensions elide within the extension column.
func (v *view) metadataWidth(gtx layout.Context, mode metadataColumn) int {
	if mode == metadataNone {
		return 0
	}
	sample := "1023.9 MB"
	switch mode {
	case metadataExtension:
		sample = "xxxxxx"
	case metadataModified:
		sample = "2000-12-31"
	}
	gtx.Constraints = layout.Constraints{Max: gtx.Constraints.Max}
	gtx.Constraints.Max.X = 1_000_000
	gtx.Constraints.Max.Y = 1_000_000
	macro := op.Record(gtx.Ops)
	value := v.label(gtx, sample, v.appearance.TextSize, v.appearance.Colors.Metadata, text.End)
	heading := v.label(gtx, mode.heading(), v.appearance.SmallTextSize, v.appearance.Colors.Metadata, text.End)
	macro.Stop()
	width := max(value.Size.X, heading.Size.X) + gtx.Dp(8)
	if mode == metadataSize {
		width = max(width, gtx.Dp(84))
	}
	return width
}
