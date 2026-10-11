package app

import "maps"

// Marks belong to the displayed listing and are separate from the current row.
// A range keeps its starting marks so reversing it only removes its own additions.
type selectionRange struct {
	anchor int
	base   map[int]bool
}

func (p *panel) markable(row int) bool {
	return !p.drives && row >= 0 && row < len(p.items) &&
		!p.items[row].file.Parent && p.items[row].name != ".."
}

func (p *panel) toggleMark(row int) {
	p.markRange = nil
	if p.loading || !p.markable(row) {
		return
	}
	if p.marks[row] {
		delete(p.marks, row)
	} else {
		if p.marks == nil {
			p.marks = make(map[int]bool)
		}
		p.marks[row] = true
	}
}

func (p *panel) markTo(row int) {
	if p.loading || row < 0 || row >= len(p.items) || p.selected < 0 {
		return
	}
	if p.markRange == nil {
		p.markRange = &selectionRange{anchor: p.selected, base: maps.Clone(p.marks)}
	}
	p.marks = make(map[int]bool, len(p.markRange.base))
	maps.Copy(p.marks, p.markRange.base)
	for i := min(p.markRange.anchor, row); i <= max(p.markRange.anchor, row); i++ {
		if p.markable(i) {
			p.marks[i] = true
		}
	}
}

func (p *panel) markAll() {
	if p.loading {
		return
	}
	p.markRange = nil
	for i := range p.items {
		if p.markable(i) {
			if p.marks == nil {
				p.marks = make(map[int]bool)
			}
			p.marks[i] = true
		}
	}
}

func (p *panel) clearMarks() {
	p.marks, p.markRange = nil, nil
}

func (p *panel) clickRow(row int, extend, toggle bool) {
	if row < 0 || row >= len(p.items) {
		return
	}
	if extend {
		p.markTo(row)
	} else if toggle {
		p.toggleMark(row)
	} else {
		p.markRange = nil
	}
	p.selected, p.reveal, p.err = row, true, nil
}
