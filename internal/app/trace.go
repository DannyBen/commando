package app

import (
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func (v *view) applyTraceSettings() {
	// Previews and isolated view tests never open production log files.
	if v.activeDataFolder == "" {
		return
	}
	mask := diagnostics.Mask(v.settingsValues.Get(settings.CatalogueDefault, settingTraceMask).Number)
	if v.traceOverride != nil {
		mask = *v.traceOverride
	}
	if err := diagnostics.Configure(v.activeDataFolder, mask); err != nil {
		if v.traceError != err.Error() {
			v.traceError = err.Error()
			v.showFeedback(v.active, feedback.Error, "Cannot open trace log")
		}
	} else {
		v.traceError = ""
	}
}

func (r loadRequest) traceKind() (diagnostics.Mask, string) {
	switch {
	case r.folderCacheLoad:
		return diagnostics.Storage, "folder_size_cache"
	case r.size != nil:
		return diagnostics.Browsing, "calculate_size"
	case r.favoriteWrite != nil:
		return diagnostics.FileOperations, "favorite"
	case r.unpack != nil:
		return diagnostics.FileOperations, "unpack_zip"
	case r.rename != nil:
		return diagnostics.FileOperations, "rename"
	case r.multiRename != nil:
		return diagnostics.FileOperations, "multi_rename"
	case r.create != nil:
		if len(r.create.packPaths) > 0 {
			return diagnostics.FileOperations, "pack_zip"
		}
		return diagnostics.FileOperations, "create"
	case r.removal != nil:
		if r.removal.permanent {
			return diagnostics.FileOperations, "delete"
		}
		return diagnostics.FileOperations, "recycle"
	case r.duplicate != nil:
		return diagnostics.FileOperations, "duplicate"
	case r.transfer != nil:
		if r.transfer.shortcuts {
			return diagnostics.FileOperations, "paste_shortcut"
		}
		return diagnostics.FileOperations, "transfer"
	case r.clipboard != nil:
		return diagnostics.Shell, "clipboard"
	case r.terminal != nil:
		return diagnostics.Shell, "terminal"
	case r.archiveOpen != nil:
		return diagnostics.Shell, "archive_open"
	case r.editor != nil:
		return diagnostics.Shell, "editor"
	case r.shortcut:
		return diagnostics.Shell, "shortcut"
	case r.settingsSave != nil:
		return diagnostics.Storage, "settings_save"
	case r.themeLibrary != nil:
		return diagnostics.Storage, "theme_library"
	case r.fileDraft != nil:
		return diagnostics.Storage, "file_draft"
	case len(r.icons) > 0:
		return diagnostics.Shell, "icons"
	case r.fontFamilies:
		return diagnostics.UI, "fonts"
	case r.completion != nil:
		return diagnostics.Browsing, "completion"
	default:
		return diagnostics.Browsing, "directory"
	}
}

func (r loadRequest) traceData(kind string) map[string]any {
	data := map[string]any{"kind": kind, "path": r.path, "panel": r.panel, "refresh": r.refresh, "automatic": r.autoRefresh}
	if r.archiveFolder != "" {
		data["archive_folder"] = r.archiveFolder
	}
	if r.rename != nil {
		data["old_name"], data["new_name"] = r.rename.oldName, r.rename.newName
	}
	if r.create != nil {
		data["name"], data["folder"] = r.create.name, r.create.folder
	}
	if r.removal != nil {
		data["names"] = r.removal.names
	}
	return data
}
