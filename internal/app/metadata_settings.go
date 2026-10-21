package app

import "github.com/dannyben/commando/internal/settings"

const settingMetadataColumns = "view.metadata_columns"

func (p *panel) setMetadataColumn(column metadataColumn) {
	if column == metadataNone && p.metadata != metadataNone {
		p.lastMetadata = p.metadata
	}
	p.metadata = column
}

func (v *view) toggleMetadataColumn() {
	if !v.canStartInteraction(v.active, actionMutation) {
		return
	}
	p := &v.panels[v.active]
	column := metadataNone
	if p.metadata == metadataNone {
		column = p.lastMetadata
		if column == metadataNone || !v.metadataColumnEnabled(column) {
			column = metadataSize
			if !v.metadataColumnEnabled(column) {
				column = v.nextMetadataColumn(metadataNone, 1)
			}
		}
	}
	if column == p.metadata {
		return
	}
	p.setMetadataColumn(column)
	if p.sortByMetadata {
		p.resort()
	}
}

func (v *view) metadataColumnEnabled(column metadataColumn) bool {
	return column == metadataNone || v.settingsValues.Get(settings.CatalogueDefault, settingMetadataColumns+"."+column.stateName()).Bool
}

func (v *view) nextMetadataColumn(column metadataColumn, direction int) metadataColumn {
	for range metadataColumnCount {
		column = column.cycle(direction)
		if v.metadataColumnEnabled(column) {
			return column
		}
	}
	return metadataNone
}

func (v *view) previewMetadataColumns(s *settingsSession) {
	// Panel membership is fixed while the settings widget owns the interaction.
	var panels []*panel
	for i := range v.panels {
		panels = append(panels, &v.panels[i])
	}
	for i := range v.closedPanels {
		panels = append(panels, &v.closedPanels[i].panel)
	}
	columns := make([]metadataColumn, len(panels))
	remembered := make([]metadataColumn, len(panels))
	for i, p := range panels {
		columns[i] = p.metadata
		remembered[i] = p.lastMetadata
	}
	s.afterCancel = func() {
		for i, p := range panels {
			p.lastMetadata = remembered[i]
			if p.metadata != columns[i] {
				p.metadata = columns[i]
				if p.sortByMetadata {
					p.resort()
				}
			}
		}
	}
}
