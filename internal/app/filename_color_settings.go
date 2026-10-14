package app

// Fallback colors for themes without an applied settings snapshot. Lightness
// is 0–1; out-of-gamut colors lose chroma while retaining lightness and hue.
var defaultFilenameColors = filenameColorSettings{
	enabled:                         true,
	filenameLightness:               0.36,
	filenameChroma:                  0.35,
	hueSeed:                         20,
	activeBackgroundLightnessOffset: 0.10, // Positive softens the row on the current white surface; use a negative offset for a dark theme.
	hiddenLightnessOffset:           0.06,
	hiddenChromaMultiplier:          0.65,
	dotfileLightnessOffset:          0.03,
	dotfileChromaMultiplier:         0.80,
}

type filenameColorSettings struct {
	enabled                                         bool
	activeBackgroundLightnessOffset                 float64
	filenameLightness, filenameChroma, hueSeed      float64
	hiddenLightnessOffset, hiddenChromaMultiplier   float64
	dotfileLightnessOffset, dotfileChromaMultiplier float64
}
