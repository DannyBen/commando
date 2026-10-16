package app

import (
	"strconv"
	"strings"
	"time"
)

// These directories and files are illustrative, not read from the host machine.
func samplePanels() []panel {
	panels := []panel{
		{path: `C:\Work\Projects`, selected: 3, items: []entry{
			{name: "..", size: "—"}, {name: "assets", size: "—"}, {name: "caretaker", size: "—"},
			{name: "commando", size: "—"}, {name: "docs", size: "—"}, {name: "experiments", size: "—"},
			{name: "fontview", size: "—"}, {name: "moxel", size: "—"}, {name: "scripts", size: "—"},
			{name: "shirei-grid", size: "—"}, {name: "todo", size: "—"}, {name: "trinity", size: "—"},
			{name: "AGENTS.md", size: "3.2 KB"}, {name: "CHANGELOG.md", size: "8.7 KB"},
			{name: "README.md", size: "2.4 KB"}, {name: "architecture-notes.md", size: "6.1 KB"},
			{name: "build.ps1", size: "1.8 KB"}, {name: "config.yml", size: "840 B"},
			{name: "design-review-2026-10.md", size: "12.4 KB"}, {name: "go.mod", size: "1.1 KB"},
			{name: "go.sum", size: "4.8 KB"}, {name: "notes.txt", size: "1.2 KB"},
			{name: "op.conf", size: "920 B"}, {name: "workspace.code-workspace", size: "480 B"},
		}},
		{path: `D:\Downloads`, selected: 5, items: []entry{
			{name: "..", size: "—"}, {name: "documents", size: "—"}, {name: "fonts", size: "—"},
			{name: "images", size: "—"}, {name: "installers", size: "—"},
			{name: "commando-reference.png", size: "824 KB"}, {name: "data-export.csv", size: "2.8 MB"},
			{name: "design-reference.pdf", size: "4.2 MB"}, {name: "go-mono.zip", size: "1.3 MB"},
			{name: "invoice-2026-10.pdf", size: "128 KB"}, {name: "meeting-notes.txt", size: "3.1 KB"},
			{name: "mockup-light.png", size: "640 KB"}, {name: "project-assets.zip", size: "18.6 MB"},
			{name: "release-notes.txt", size: "6.4 KB"}, {name: "sample-directory-list.csv", size: "92 KB"},
			{name: "screenshot-2026-10-08.png", size: "1.2 MB"},
			{name: "screenshot-2026-10-09.png", size: "1.4 MB"},
			{name: "screenshot-2026-10-10.png", size: "980 KB"},
			{name: "source-export-2026-10.zip", size: "24.8 MB"},
			{name: "test-archive.zip", size: "8.6 MB"}, {name: "theme-colors.json", size: "2.1 KB"},
			{name: "trinity.exe", size: "16.2 MB"}, {name: "ui-reference.svg", size: "14.8 KB"},
			{name: "workspace-backup.zip", size: "32.4 MB"},
			{name: "workspace-preview-final-revision.png", size: "1.8 MB"},
			{name: "workspace-preview-previous.png", size: "1.6 MB"},
			{name: "workspace-sample.yml", size: "4.8 KB"}, {name: "zip-format-notes.txt", size: "2.2 KB"},
		}},
		{path: `E:\Archive\2026`, selected: 4, items: []entry{
			{name: "..", size: "—"}, {name: "07-july", size: "—"}, {name: "08-august", size: "—"},
			{name: "09-september", size: "—"}, {name: "10-october", size: "—"},
			{name: "assets-2026-08.zip", size: "42.8 MB"}, {name: "assets-2026-09.zip", size: "48.2 MB"},
			{name: "caretaker-0.1.0.zip", size: "6.4 MB"}, {name: "commando-mockups.zip", size: "2.8 MB"},
			{name: "documents-2026-09.zip", size: "12.6 MB"}, {name: "font-collection.zip", size: "8.4 MB"},
			{name: "moxel-0.1.0.zip", size: "3.2 MB"}, {name: "project-notes.zip", size: "1.6 MB"},
			{name: "release-checklist.md", size: "3.1 KB"}, {name: "source-backup-2026-09.zip", size: "84.6 MB"},
			{name: "source-backup-2026-10.zip", size: "92.4 MB"}, {name: "todo-0.2.0.zip", size: "4.8 MB"},
			{name: "trinity-backup-2026-10.zip", size: "18.2 MB"},
			{name: "weekly-backup-2026-10-03.zip", size: "128 MB"},
			{name: "weekly-backup-2026-10-10.zip", size: "136 MB"},
			{name: "workspace-index.txt", size: "8.2 KB"},
		}},
	}
	for pi := range panels {
		for row := range panels[pi].items {
			item := &panels[pi].items[row]
			item.file.Name = item.name
			item.file.Directory = item.size == "—"
			item.file.Parent = item.name == ".."
			if !item.file.Directory {
				value, unit, _ := strings.Cut(item.size, " ")
				if bytes, err := strconv.ParseFloat(value, 64); err == nil {
					switch unit {
					case "KB":
						bytes *= 1024
					case "MB":
						bytes *= 1024 * 1024
					}
					item.file.Bytes, item.file.SizeKnown = int64(bytes), true
				}
			}
			if !item.file.Parent {
				item.file.Modified = time.Date(2026, time.October, 1+row%10, 12, 0, 0, 0, time.UTC)
				item.file.Created = time.Date(2026, time.September, 1+row%10, 12, 0, 0, 0, time.UTC)
			}
		}
	}
	return panels
}
