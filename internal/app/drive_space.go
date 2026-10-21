package app

import "github.com/dannyben/commando/internal/browser"

// The token follows its panel model, rather than capturing a movable index.
// Only the UI accesses submitted and the panel's results; the worker reads roots.
type driveSpaceRequest struct {
	roots     []string
	submitted bool
}

func (p *panel) prepareDriveSpaces() {
	p.driveSpaces = nil
	p.driveSpace = nil
	if !p.drives {
		return
	}
	p.driveSpaces = make(map[string]browser.DriveSpace)
	request := &driveSpaceRequest{}
	for _, file := range p.entries {
		if !file.Favorite && file.Path != "" {
			request.roots = append(request.roots, file.Path)
		}
	}
	if len(request.roots) > 0 {
		p.driveSpace = request
	}
}

// Submit after Home has been laid out; a slow query cannot hold its first frame.
// Failed submissions retry on a later frame without duplicating accepted work.
func (v *view) submitDriveSpaces() {
	if v.loader == nil {
		return
	}
	for i := range v.panels {
		p := &v.panels[i]
		request := p.driveSpace
		if !p.drives || p.loading || request == nil || request.submitted {
			continue
		}
		if v.loader.submit(loadRequest{driveSpace: request, action: actionBrowse}) {
			request.submitted = true
		}
	}
}

func (v *view) acceptDriveSpace(result loadResult) {
	for i := range v.panels {
		p := &v.panels[i]
		if !p.drives || p.driveSpace != result.request.driveSpace {
			continue
		}
		if result.driveSpaceKnown {
			p.driveSpaces[result.request.path] = result.driveSpace
		} else {
			delete(p.driveSpaces, result.request.path)
		}
	}
}

func (p *panel) driveSpaceFor(file browser.Entry) (browser.DriveSpace, bool) {
	if !p.drives || file.Favorite || file.Path == "" {
		return browser.DriveSpace{}, false
	}
	space, ok := p.driveSpaces[file.Path]
	return space, ok
}

func (p *panel) driveSpaceLabel(file browser.Entry) string {
	if space, ok := p.driveSpaceFor(file); ok {
		return formatSize(space.Free) + " / " + formatSize(space.Total)
	}
	return ""
}
