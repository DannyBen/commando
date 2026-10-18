package app

import (
	"embed"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

// All upright weights share one family. Regular is the default face.
//
//go:embed fonts/VictorMono-*.ttf
var themeFontFiles embed.FS

var themeFont = bundledFonts(themeFontFiles)

// Shared pointer feedback strength; independent of theme colors and selection.
const hoverOpacity = 0.08

type contrastRole uint8

const (
	contrastPrimary contrastRole = iota
	contrastSecondary
	contrastTextSelection
	contrastDimmedFile
)

type contrastAdjustment uint8

const (
	adjustForeground contrastAdjustment = iota
	adjustBackground
)

type contrastRule struct {
	targetLc float64
	adjust   contrastAdjustment
	// Background adjustments stop this many LSH points from the editor surface.
	minLightnessGap float64
}

// Display policy for generated and customized palettes alike. Text selection
// keeps the editor's resolved foreground and adjusts only its own background.
var themeContrastRules = map[contrastRole]contrastRule{
	contrastPrimary:       {targetLc: 75, adjust: adjustForeground},
	contrastDimmedFile:    {targetLc: 30, adjust: adjustForeground},
	contrastSecondary:     {targetLc: 60, adjust: adjustForeground},
	contrastTextSelection: {targetLc: 60, adjust: adjustBackground, minLightnessGap: 8},
}

// Theme holds the appearance choices independently of Gio's widget theme.
// A future configuration loader can supply these values before creating a view.
type Theme struct {
	FilenameColors          *filenameColorSettings
	SettingsWidth           unit.Dp
	Colors                  ThemeColors
	Font                    []font.FontFace
	FontFamily              string
	FontWeight              font.Weight
	BoldWeight              font.Weight
	TextSize, SmallTextSize unit.Sp
	// Padding is measured in dp per side. Border thicknesses remain separate
	// layout rules.
	RowPaddingVertical               unit.Dp
	HeaderFooterPaddingVertical      unit.Dp
	LocationPaddingVertical          unit.Dp
	SearchPaddingVertical            unit.Dp
	ContentPaddingHorizontal         unit.Dp
	OverlayMinWidth, OverlayMaxWidth unit.Dp
	OverlayBackdropOpacity           float32 // Surface-color wash; 0 disables it, 1 is opaque.
	Feedback                         feedback.Animations
	FeedbackProgressDelay            time.Duration // Delay before showing read or file-operation progress.
}

type ThemeColors struct {
	Surface, Header, Footer, Border, Foreground, Muted color.NRGBA
	Edge, ActiveLocation                               color.NRGBA
	Filename, Metadata                                 color.NRGBA
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
	Enabled:             true,
	CharacterDelay:      15 * time.Millisecond,
	BlinkInterval:       160 * time.Millisecond,
	Blinks:              0, // Set to 2 to restore the text pulse (160 ms per state).
	Cursor:              "█",
	CursorBlinkInterval: 350 * time.Millisecond,
	CursorBlinks:        -1, // Keep blinking until the message is dismissed.
}

// Appearance defaults. Theme colors come from the LSH settings;
// user preferences are applied through the shared settings snapshot.
var defaultTheme = Theme{
	SettingsWidth:               480,
	Colors:                      themeColorsFromSettings(settings.Values{}),
	Font:                        themeFont,
	FontFamily:                  "Victor Mono",
	FontWeight:                  font.SemiBold,
	BoldWeight:                  font.SemiBold,
	TextSize:                    15, // File rows and panel paths.
	SmallTextSize:               12, // Column headings and panel footers.
	RowPaddingVertical:          2,  // dp per side; row height grows with the font.
	HeaderFooterPaddingVertical: 6,
	LocationPaddingVertical:     6,
	SearchPaddingVertical:       8,
	ContentPaddingHorizontal:    14,
	OverlayMinWidth:             480,
	OverlayMaxWidth:             720,
	OverlayBackdropOpacity:      0.15,
	FeedbackProgressDelay:       500 * time.Millisecond,
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
	theme.Shaper, theme.Face = fontShaper(t.Font, t.FontFamily)
	theme.Fg, theme.Bg = t.Colors.Foreground, t.Colors.Surface
	theme.ContrastBg, theme.ContrastFg = t.Colors.Selected, t.Colors.SelectedName
	return theme
}

func rgb(value uint32) color.NRGBA {
	return color.NRGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 255}
}
