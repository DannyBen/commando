package app

// These directories and files are illustrative, not read from the host machine.
func samplePanels() []panel {
	return []panel{
		{path: `C:\Work\Projects`, selected: 3, items: []entry{
			{"../", "—"}, {"assets/", "—"}, {"caretaker/", "—"},
			{"control/", "—"}, {"docs/", "—"}, {"experiments/", "—"},
			{"fontview/", "—"}, {"moxel/", "—"}, {"scripts/", "—"},
			{"shirei-grid/", "—"}, {"todo/", "—"}, {"trinity/", "—"},
			{"AGENTS.md", "3.2 KB"}, {"CHANGELOG.md", "8.7 KB"},
			{"README.md", "2.4 KB"}, {"architecture-notes.md", "6.1 KB"},
			{"build.ps1", "1.8 KB"}, {"config.yml", "840 B"},
			{"design-review-2026-10.md", "12.4 KB"}, {"go.mod", "1.1 KB"},
			{"go.sum", "4.8 KB"}, {"notes.txt", "1.2 KB"},
			{"op.conf", "920 B"}, {"workspace.code-workspace", "480 B"},
		}},
		{path: `D:\Downloads`, selected: 5, items: []entry{
			{"../", "—"}, {"documents/", "—"}, {"fonts/", "—"},
			{"images/", "—"}, {"installers/", "—"},
			{"control-reference.png", "824 KB"}, {"data-export.csv", "2.8 MB"},
			{"design-reference.pdf", "4.2 MB"}, {"go-mono.zip", "1.3 MB"},
			{"invoice-2026-10.pdf", "128 KB"}, {"meeting-notes.txt", "3.1 KB"},
			{"mockup-light.png", "640 KB"}, {"project-assets.zip", "18.6 MB"},
			{"release-notes.txt", "6.4 KB"}, {"sample-directory-list.csv", "92 KB"},
			{"screenshot-2026-10-08.png", "1.2 MB"},
			{"screenshot-2026-10-09.png", "1.4 MB"},
			{"screenshot-2026-10-10.png", "980 KB"},
			{"source-export-2026-10.zip", "24.8 MB"},
			{"test-archive.zip", "8.6 MB"}, {"theme-colors.json", "2.1 KB"},
			{"trinity.exe", "16.2 MB"}, {"ui-reference.svg", "14.8 KB"},
			{"workspace-backup.zip", "32.4 MB"},
			{"workspace-preview-final-revision.png", "1.8 MB"},
			{"workspace-preview-previous.png", "1.6 MB"},
			{"workspace-sample.yml", "4.8 KB"}, {"zip-format-notes.txt", "2.2 KB"},
		}},
		{path: `E:\Archive\2026`, selected: 4, items: []entry{
			{"../", "—"}, {"07-july/", "—"}, {"08-august/", "—"},
			{"09-september/", "—"}, {"10-october/", "—"},
			{"assets-2026-08.zip", "42.8 MB"}, {"assets-2026-09.zip", "48.2 MB"},
			{"caretaker-0.1.0.zip", "6.4 MB"}, {"control-mockups.zip", "2.8 MB"},
			{"documents-2026-09.zip", "12.6 MB"}, {"font-collection.zip", "8.4 MB"},
			{"moxel-0.1.0.zip", "3.2 MB"}, {"project-notes.zip", "1.6 MB"},
			{"release-checklist.md", "3.1 KB"}, {"source-backup-2026-09.zip", "84.6 MB"},
			{"source-backup-2026-10.zip", "92.4 MB"}, {"todo-0.2.0.zip", "4.8 MB"},
			{"trinity-backup-2026-10.zip", "18.2 MB"},
			{"weekly-backup-2026-10-03.zip", "128 MB"},
			{"weekly-backup-2026-10-10.zip", "136 MB"},
			{"workspace-index.txt", "8.2 KB"},
		}},
	}
}
