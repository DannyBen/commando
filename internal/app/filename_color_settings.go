package app

// Fallback colors for themes without an applied settings snapshot. Lightness
// is 0–1; saturation is the fraction of usable chroma at each lightness and hue.
var defaultFilenameColors = filenameColorSettings{
	enabled:                         true,
	filenameLightness:               0.36,
	filenameSaturation:              1.0,
	salt:                            0,
	activeBackgroundLightnessOffset: 0.10, // Positive softens the row on the current white surface; use a negative offset for a dark theme.
	cutLightnessOffset:              0.20,
	hiddenLightnessOffset:           0.06,
	hiddenSaturationMultiplier:      0.65,
	dotfileLightnessOffset:          0.03,
	dotfileSaturationMultiplier:     0.80,
}

type filenameColorSettings struct {
	enabled                                             bool
	activeBackgroundLightnessOffset, cutLightnessOffset float64
	filenameLightness, filenameSaturation, salt         float64
	hiddenLightnessOffset, hiddenSaturationMultiplier   float64
	dotfileLightnessOffset, dotfileSaturationMultiplier float64
}
