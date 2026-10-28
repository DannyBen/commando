package app

import (
	"testing"
	"time"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestDateDisplayRelativeAndCalendarBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		age      time.Duration
		relative string
		calendar string
	}{
		{0, "just now", "12:00"},
		{5 * time.Minute, "5m ago", "11:55"},
		{2 * time.Hour, "2h ago", "10:00"},
		{23*time.Hour + 59*time.Minute, "23h ago", now.Add(-23*time.Hour - 59*time.Minute).Format("15:04")},
		{24 * time.Hour, "1d ago", "2026-10-27"},
		{60 * 24 * time.Hour, "2mo ago", now.Add(-60 * 24 * time.Hour).Format("2006-01-02")},
	} {
		date := now.Add(-tc.age)
		if got := formatDate(date, now, "Time ago"); got != tc.relative {
			t.Errorf("age %v: relative = %q, want %q", tc.age, got, tc.relative)
		}
		if got := formatDate(date, now, "Calendar date"); got != tc.calendar {
			t.Errorf("age %v: calendar = %q, want %q", tc.age, got, tc.calendar)
		}
	}
	if got := formatDate(now.Add(time.Hour), now, "Calendar date"); got != "2026-10-28" {
		t.Fatal("future timestamp shown as a past time", got)
	}
	if got := formatDate(time.Time{}, now, "Time ago"); got != "?" {
		t.Fatal("unknown timestamp lost its placeholder", got)
	}
}

func TestDateDisplaySettingAppliesToRowsAndFooter(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)
	item := entry{file: browser.Entry{Name: "notes.txt", Modified: now.Add(-2 * time.Hour), Created: now.Add(-48 * time.Hour)}}
	v := newPreviewView()
	if v.dateDisplay() != "Time ago" {
		t.Fatal("time ago is not the default")
	}
	p := &v.panels[0]
	p.items, p.selected, p.metadata = []entry{item}, 0, metadataCreated
	p.clearMarks()
	if got := metadataModified.valueAt(item, v.dateDisplay(), now); got != "2h ago" {
		t.Fatal("modified row did not use relative time", got)
	}
	if got := metadataCreated.valueAt(item, v.dateDisplay(), now); got != "2d ago" {
		t.Fatal("created row did not use relative time", got)
	}
	if got, _ := p.statusAt(now, v.dateDisplay()); got != "2h ago" {
		t.Fatal("footer did not use relative time", got)
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingDateDisplay, settings.Value{Text: "Calendar date"}); err != nil {
		t.Fatal(err)
	}
	if got := metadataModified.valueAt(item, v.dateDisplay(), now); got != "10:00" {
		t.Fatal("modified row did not use calendar time", got)
	}
	if got := metadataCreated.valueAt(item, v.dateDisplay(), now); got != "2026-10-26" {
		t.Fatal("created row did not use calendar date", got)
	}
	if got, _ := p.statusAt(now, v.dateDisplay()); got != "10:00" {
		t.Fatal("footer did not use calendar time", got)
	}
}
