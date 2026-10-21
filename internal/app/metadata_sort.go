package app

import (
	"cmp"
	"strings"
	"time"

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
	if !p.metadataSortActive() ||
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
		now := time.Now()
		first, firstKnown := p.entrySize(a, now)
		second, secondKnown := p.entrySize(b, now)
		order = compareKnown(firstKnown, secondKnown)
		if order == 0 && firstKnown {
			order = cmp.Compare(second, first)
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

func (p *panel) metadataSortActive() bool {
	return p.sortByMetadata && p.metadata != metadataNone && !p.drives
}

func (p *panel) metadataSortHeading() string {
	heading := p.metadata.heading()
	if !p.metadataSortActive() {
		return heading
	}
	if p.metadata == metadataExtension {
		return "↑ " + heading
	}
	return "↓ " + heading
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
