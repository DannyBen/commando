package app

import (
	"cmp"
	"strings"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

// Metadata sorting is a panel preference. The complete cache retains its
// natural-name order; visible projections and inserted rows use this comparator.
func (p *panel) compareEntries(a, b browser.Entry) int {
	if p.drives && a.Favorite != b.Favorite {
		if a.Favorite {
			return -1
		}
		return 1
	}
	if !p.sortByMetadata || p.metadata == metadataNone || p.drives ||
		a.Parent || b.Parent || a.Directory != b.Directory {
		return browser.CompareEntries(a, b)
	}
	order := 0
	switch p.metadata {
	case metadataExtension:
		if !a.Directory {
			order = strings.Compare(strings.ToLower(metadataExtension.value(entry{file: a})),
				strings.ToLower(metadataExtension.value(entry{file: b})))
		}
	case metadataSize:
		if !a.Directory {
			order = compareKnown(a.SizeKnown, b.SizeKnown)
			if order == 0 && a.SizeKnown {
				order = cmp.Compare(b.Bytes, a.Bytes)
			}
		}
	case metadataModified, metadataCreated:
		first, second := p.metadata.date(a), p.metadata.date(b)
		order = compareKnown(!first.IsZero(), !second.IsZero())
		if order == 0 {
			order = second.Compare(first)
		}
	}
	if order != 0 {
		return order
	}
	return browser.CompareEntries(a, b)
}

func compareKnown(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return -1
	}
	return 1
}

func (v *view) toggleMetadataSort() {
	p := &v.panels[v.active]
	if !v.interaction.idle() || !folderReady(v) || v.pendingNameDraft != nil && v.pendingNameDraft.request.panel == v.active {
		return
	}
	p.sortByMetadata = !p.sortByMetadata
	p.resort()
	message := "Metadata sorting disabled"
	if p.sortByMetadata {
		message = "Metadata sorting enabled"
	}
	v.showFeedback(v.active, feedback.Info, message)
}

// Reordering is not navigation or a read result. Retain transient browsing
// state as well as the identities/marks/viewport preserved by reconciliation.
func (p *panel) resort() {
	p.ensureEntries()
	prefix, prefixAt, err, reveal := p.prefix, p.prefixAt, p.err, p.reveal
	p.refilter()
	p.prefix, p.prefixAt, p.err, p.reveal = prefix, prefixAt, err, reveal
}
