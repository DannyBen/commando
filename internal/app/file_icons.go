package app

import (
	"image"
	"path/filepath"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"github.com/dannyben/commando/internal/settings"
)

type fileIconKey struct {
	path string
	size int
}

const fileIconCacheLimit = 512
const fileIconBatchSize = 32

type fileIconEntry struct {
	image  paint.ImageOp
	source image.Image
	used   uint64
}

// Original Windows icons, enabled only for live Windows listings.
func (v *view) withFileIcon(gtx layout.Context, p *panel, item entry, content layout.Widget) layout.Dimensions {
	if !v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool || v.loader == nil || v.loader.readFileIcon == nil {
		return content(gtx)
	}
	size := max(1, min(gtx.Dp(16), v.textHeight(gtx, v.appearance.TextSize)))
	key := fileIconKey{filepath.Join(p.path, item.file.Name), size}
	if item.file.Path != "" {
		key.path = item.file.Path
	}
	if v.fileIcons == nil {
		v.fileIcons = make(map[fileIconKey]fileIconEntry)
	}
	icon, found := v.fileIcons[key]
	if found {
		v.fileIconClock++
		icon.used = v.fileIconClock
		v.fileIcons[key] = icon
	}
	if !found && !item.file.Parent && !v.fileIconPending {
		v.wantFileIcon(key)
	}
	if found && icon.source != nil {
		icon.image = v.adjustedFileIcon(icon, item.file)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			padding := gtx.Dp(max(0, v.appearance.ContentPaddingHorizontal))
			if found && icon.image.Size().X > 0 && !item.file.Parent {
				defer op.Offset(image.Pt(padding, (gtx.Constraints.Min.Y-size)/2)).Push(gtx.Ops).Pop()
				icon.image.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
			}
			return layout.Dimensions{Size: image.Pt(padding+size, gtx.Constraints.Min.Y)}
		}),
		layout.Flexed(1, content),
	)
}

func (v *view) wantFileIcon(key fileIconKey) {
	for _, wanted := range v.fileIconWanted {
		if wanted == key {
			return
		}
	}
	if len(v.fileIconWanted) < fileIconBatchSize {
		v.fileIconWanted = append(v.fileIconWanted, key)
	}
}

// Submit once after laying out the lists, so one worker trip fills multiple
// visible rows without waiting for a repaint between individual icon lookups.
func (v *view) submitFileIcons() {
	if v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool && len(v.fileIconWanted) > 0 && !v.fileIconPending {
		v.fileIconPending = v.loader.submit(loadRequest{action: actionBrowse, icons: append([]fileIconKey(nil), v.fileIconWanted...)})
	}
}

func (v *view) acceptFileIcons(result loadResult) {
	v.fileIconPending = false
	for i, key := range result.request.icons {
		if _, found := v.fileIcons[key]; !found && len(v.fileIcons) >= fileIconCacheLimit {
			var oldest fileIconKey
			var oldestUse uint64 = ^uint64(0)
			for key, entry := range v.fileIcons {
				if entry.used < oldestUse {
					oldest, oldestUse = key, entry.used
				}
			}
			delete(v.fileIcons, oldest)
		}
		var icon paint.ImageOp
		if result.icons[i] != nil {
			icon = paint.NewImageOp(result.icons[i])
		}
		v.fileIconClock++
		v.fileIcons[key] = fileIconEntry{image: icon, source: result.icons[i], used: v.fileIconClock}
	}
}
