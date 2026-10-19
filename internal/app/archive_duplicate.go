package app

import "github.com/dannyben/commando/internal/browser"

func (l *directoryLoader) archiveDuplicateOperation(request loadRequest, result loadResult) loadResult {
	var plan *browser.DuplicatePlan
	var name string
	var locations []browser.Location
	if request.duplicate != nil {
		plan, name, locations = request.duplicate.plan, request.duplicate.name, request.duplicate.archiveLocations
	} else {
		selection := request.transfer.archive
		var err error
		plan, err = browser.PlanArchiveDuplicate(selection.location, selection.names)
		if err != nil {
			result.err = err
			return result
		}
		if len(plan.Names) == 1 {
			result.duplicatePlan = plan
			return result
		}
		locations = request.transfer.archiveLocations
	}
	paths := make([]string, len(locations))
	for i, location := range locations {
		paths[i] = location.Path
	}
	result.locations = resolveFileLocations(paths)
	result.created, result.err = browser.DuplicateArchive(plan, name)
	if result.err != nil {
		return result
	}
	count := len(plan.Names)
	result.transferred = browser.TransferResult{Directory: plan.Directory, Attempted: true, Summary: browser.TransferSummary{Requested: count, Completed: count}}
	l.refreshArchivePanels(&result, locations, plan.Directory)
	return result
}
