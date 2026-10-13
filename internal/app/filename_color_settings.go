package app

// Edit these experiment parameters, then rebuild. Lightness is 0–1; chroma
// controls saturation; the seed is a hue rotation in degrees. Out-of-gamut
// colors lose chroma while retaining lightness and hue.
var defaultFilenameColors = filenameColorSettings{
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
	activeBackgroundLightnessOffset                 float64
	filenameLightness, filenameChroma, hueSeed      float64
	hiddenLightnessOffset, hiddenChromaMultiplier   float64
	dotfileLightnessOffset, dotfileChromaMultiplier float64
}
