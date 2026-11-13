package app

import (
	"image/color"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

type gitOptions struct {
	enabled    bool
	executable string
}

func (v *view) gitOptions() gitOptions {
	c := settings.CatalogueDefault
	return gitOptions{v.settingsValues.Get(c, settingGitEnabled).Bool, v.settingsValues.Get(c, settingGitPath).Text}
}

func gitStatusNames(statuses map[string]browser.GitStatus) map[string]browser.GitStatus {
	if statuses == nil {
		return nil
	}
	names := make(map[string]browser.GitStatus, len(statuses))
	for name, status := range statuses {
		key := transferNameKey(name)
		names[key] = max(names[key], status)
	}
	return names
}

// Keep Git metadata independent of names, selection, marks and file-icon caches.
func (v *view) fileGitStatus(p *panel, file browser.Entry) browser.GitStatus {
	if p.gitStatus == nil || !p.gitOptions.enabled || p.gitOptions != v.gitOptions() || p.deep != nil || file.Parent {
		return browser.GitClean
	}
	return p.gitStatus[transferNameKey(file.Name)]
}

func (v *view) gitEdgeColor(status browser.GitStatus) color.NRGBA {
	switch status {
	case browser.GitNew:
		return v.appearance.Colors.Success
	case browser.GitChanged:
		return v.appearance.Colors.Warning
	default:
		return color.NRGBA{}
	}
}
