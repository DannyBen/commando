package app

import (
	_ "embed"
	"image/color"
	"time"

	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/dannyben/commando/internal/feedback"
)

// Change this asset to choose a different bundled font.
//
//go:embed fonts/JetBrainsMonoNL-Regular.ttf
var themeFont []byte

// Theme holds the appearance choices independently of Gio's widget theme.
// A future configuration loader can supply these values before creating a view.
type Theme struct {
	Colors                  ThemeColors
	Font                    []byte
	TextSize, SmallTextSize unit.Sp
	RowPaddingVertical      unit.Dp // Above and below the measured text height.
	RowPaddingHorizontal    unit.Dp // Left and right of the row contents.
	Feedback                feedback.Animations
}

type ThemeColors struct {
	Surface, Header, Footer, Border, Foreground, Muted color.NRGBA
	Edge, ActiveLocation                               color.NRGBA
	Marked                                             color.NRGBA // Left-edge bar for marked files and folders.
	Filename, Metadata                                 color.NRGBA
	CutFilename, SelectedCutFilename                   color.NRGBA
	Selected, SelectedName, SelectedMetadata           color.NRGBA
	TextSelection, Error                               color.NRGBA
	Info, Success, Warning                             color.NRGBA
	NameRow, EditorText                                color.NRGBA
	InlineSurface, InlineText, InlineEdge              color.NRGBA
	InlineInactiveSelected                             color.NRGBA
	Scrollbar, ScrollbarHover, ScrollbarTrack          color.NRGBA
}

// Each kind starts with the same profile. The theme stores independent copies.
var defaultFeedbackAnimation = feedback.Animation{
	Enabled:        true,
	CharacterDelay: 35 * time.Millisecond,
	BlinkInterval:  160 * time.Millisecond,
	Blinks:         2,
}

// Edit these values to adjust the application's theme, then rebuild.
// Text sizes use scale-independent pixels (sp); colors use RGB hex values.
var defaultTheme = Theme{
	Colors: ThemeColors{
		Surface:                rgb(0xffffff),
		Header:                 rgb(0xdddddd),
		Footer:                 rgb(0xdddddd),
		Border:                 rgb(0xcccccc),
		Foreground:             rgb(0x111111),
		Muted:                  rgb(0x333333),
		Edge:                   rgb(0x0000ff),
		ActiveLocation:         rgb(0x0000ff),
		Marked:                 rgb(0xff8800),
		Filename:               rgb(0x0000ff),
		CutFilename:            rgb(0x8888bb),
		SelectedCutFilename:    rgb(0xb8c9ee),
		Metadata:               rgb(0x0000ff),
		Selected:               rgb(0x0055ff),
		SelectedName:           rgb(0xffffff),
		SelectedMetadata:       rgb(0xffffff),
		TextSelection:          rgb(0xaaaaff),
		NameRow:                rgb(0xffffff), // Surrounding row while editing a name; match Surface to hide its highlight.
		InlineSurface:          rgb(0xf3efff),
		InlineText:             rgb(0x111111),
		InlineEdge:             rgb(0x6633cc),
		InlineInactiveSelected: rgb(0xe2d9f5),
		EditorText:             rgb(0x111111), // Shared by the location and name editors, including selected text.
		Error:                  rgb(0xc00000),
		Info:                   rgb(0x3366cc),
		Success:                rgb(0x008800),
		Warning:                rgb(0xff8800),
		Scrollbar:              rgb(0xcccccc),
		ScrollbarHover:         rgb(0xaaaaaa),
		ScrollbarTrack:         color.NRGBA{}, // Transparent; shows the panel surface.
	},
	Font:                 themeFont,
	TextSize:             15, // File rows and panel paths.
	SmallTextSize:        12, // Column headings and panel footers.
	RowPaddingVertical:   4,  // dp per side; row height grows with the font.
	RowPaddingHorizontal: 14,
	Feedback: feedback.Animations{
		Enabled: true,
		Info:    defaultFeedbackAnimation,
		Success: defaultFeedbackAnimation,
		Warning: defaultFeedbackAnimation,
		Error:   defaultFeedbackAnimation,
	},
}

func (t Theme) materialTheme() *material.Theme {
	theme := material.NewTheme()
	faces := bundledFont(t.Font)
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	theme.Face = faces[0].Font.Typeface
	theme.Fg, theme.Bg = t.Colors.Foreground, t.Colors.Surface
	theme.ContrastBg, theme.ContrastFg = t.Colors.Selected, t.Colors.SelectedName
	return theme
}

func rgb(value uint32) color.NRGBA {
	return color.NRGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 255}
}
