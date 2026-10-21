package app

import (
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

// A passive presentation of otherwise transient theme states. It never owns
// input or writes to panel selection, marks, filters, feedback or the clipboard.
type panelPreview struct {
	textSelection bool
	messages      []feedback.Kind
	rows          []previewFileRow
}

func (v *view) settingsPanelPreview(index int) panelPreview {
	s := v.settingsWidget()
	if s == nil || len(s.fields) == 0 || index != 0 {
		return panelPreview{}
	}
	group := s.fields[0].Widget
	if group == settingThemeGenerator || group == settingThemeSelect || group == "appearance.contrast_guard" || isThemeWidget(s) {
		return panelPreview{textSelection: true, rows: themePreviewRows(), messages: []feedback.Kind{feedback.Info, feedback.Success, feedback.Warning, feedback.Error}}
	}
	switch group {
	case "appearance.autocolor":
		return panelPreview{rows: autoColorPreviewRows()}
	case "appearance.autocolor.selection":
		return panelPreview{rows: selectionPreviewRows()}
	case "appearance.autocolor.special_files":
		return panelPreview{rows: specialFilePreviewRows()}
	}
	return panelPreview{}
}

// Row states describe visual samples independently of real browsing state.
// Several samples may be selected to compare their full row backgrounds.
type previewFileRow struct {
	file                  browser.Entry
	selected, marked, cut bool
}

func previewFile(name string) browser.Entry {
	return browser.Entry{Name: name, Bytes: 1024, SizeKnown: true,
		Modified: time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC),
		Created:  time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)}
}

func themePreviewRows() []previewFileRow {
	folder := previewFile("documents")
	folder.Directory = true
	hidden := previewFile("hidden-notes.txt")
	hidden.Hidden = true
	return []previewFileRow{
		{file: folder},
		{file: previewFile("notes.txt"), marked: true},
		{file: previewFile("photo.jpg"), selected: true},
		{file: previewFile("cut-archive.zip"), selected: true, marked: true, cut: true},
		{file: hidden, cut: true},
		{file: previewFile(".config"), marked: true},
	}
}

func autoColorPreviewRows() []previewFileRow {
	var rows []previewFileRow
	for _, name := range []string{"notes.txt", "photo.jpg", "archive.zip"} {
		rows = append(rows, previewFileRow{file: previewFile(name)}, previewFileRow{file: previewFile("selected-" + name), selected: true})
	}
	return rows
}

func selectionPreviewRows() []previewFileRow {
	return []previewFileRow{
		{file: previewFile("notes.txt"), selected: true},
		{file: previewFile("photo.jpg"), selected: true},
		{file: previewFile("archive.zip"), selected: true},
		{file: previewFile("source.go"), selected: true},
		{file: previewFile("cut-notes.txt"), cut: true},
		{file: previewFile("cut-photo.jpg"), cut: true},
		{file: previewFile("uncut-notes.txt")},
		{file: previewFile("uncut-photo.jpg")},
	}
}

func specialFilePreviewRows() []previewFileRow {
	var rows []previewFileRow
	for _, spec := range []struct {
		name   string
		hidden bool
	}{
		{"normal-file.txt", false}, {"hidden-file.txt", true}, {".dotfile.txt", false}, {".hidden-dotfile.txt", true}, {"normal-noext", false}, {".dotfile-noext", false},
	} {
		file := previewFile(spec.name)
		file.Hidden = spec.hidden
		rows = append(rows, previewFileRow{file: file})
	}
	return rows
}

func (preview panelPreview) layoutRows(v *view, gtx layout.Context, mode metadataColumn, height int) layout.Dimensions {
	size := gtx.Constraints.Max
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	for i, row := range preview.rows {
		if i*height >= size.Y {
			break
		}
		stack := op.Offset(image.Pt(0, i*height)).Push(gtx.Ops)
		rowGTX := gtx
		rowGTX.Constraints = layout.Exact(image.Pt(size.X, height))
		bg, name, metadata := v.appearance.entryColors(row.file, row.selected, row.cut)
		edge := color.NRGBA{}
		if row.marked {
			edge = v.appearance.Colors.Edge
		}
		v.fileRowSurface(rowGTX, rowGTX.Constraints.Max, bg, edge, func(gtx layout.Context) layout.Dimensions {
			item := entry{name: row.file.Name, size: formatSize(row.file.Bytes), file: row.file}
			if row.file.Directory {
				item.size = "—"
			}
			return v.withRowIcon(gtx, &panel{}, item, row.cut, true, func(gtx layout.Context) layout.Dimensions {
				return v.rowContents(gtx, item, name, metadata, mode, v.appearance.entryTextRoles(row.file, row.selected, row.cut), false)
			})
		})
		stack.Pop()
	}
	return layout.Dimensions{Size: size}
}

func (v *view) themePreviewSelection(gtx layout.Context) layout.Dimensions {
	c := v.appearance.Colors
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, "/ ", v.appearance.SmallTextSize, c.EditorText, text.Start)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			const sample = "Example selection"
			foreground := v.resolvedTextOn(c.EditorText, c.Surface)
			background := v.resolvedTextSelection(foreground, c.Surface)
			size := image.Pt(min(gtx.Constraints.Max.X, v.textWidth(gtx, sample, v.appearance.SmallTextSize)), v.textHeight(gtx, v.appearance.SmallTextSize))
			return v.fileRowSurface(gtx, size, background, color.NRGBA{}, func(gtx layout.Context) layout.Dimensions {
				return v.rawLabel(gtx, sample, v.appearance.SmallTextSize, foreground, text.Start)
			})
		}),
	)
}

func (preview panelPreview) footer(v *view, gtx layout.Context, height int) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(preview.messages))
	for i, kind := range preview.messages {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			label := map[feedback.Kind]string{feedback.Error: "Error", feedback.Success: "Success", feedback.Warning: "Warning", feedback.Info: "Info"}[kind]
			background, foreground := v.notificationColors(kind)
			// Keep normal outer margins, with only a small gap between samples.
			inset := layout.Inset{Left: 2, Right: 2}
			if i == 0 {
				inset.Left = max(0, v.appearance.ContentPaddingHorizontal)
			}
			if i == len(preview.messages)-1 {
				inset.Right = max(0, v.appearance.ContentPaddingHorizontal)
			}
			return v.bandWithInset(gtx, height, background, inset, func(gtx layout.Context) layout.Dimensions {
				return v.feedbackLabel(gtx, label, foreground)
			}, false)
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}
