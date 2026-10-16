package app

import (
	"math"
	"testing"
)

func TestAPCAAdjustsColorBeforeUsingNeutral(t *testing.T) {
	for _, tc := range []struct {
		name             string
		text, background uint32
	}{
		{"magenta on light header", 0xbb00aa, 0xefe8fa},
		{"blue on black", 0x003366, 0},
		{"red on white", 0xff0000, 0xffffff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, background := rgb(tc.text), rgb(tc.background)
			if math.Abs(perceptualContrast(original, background)) >= textContrastMinimumLc {
				t.Fatal("fixture already passes")
			}
			adjusted := readableTextColor(original, background)
			if adjusted == rgb(0) || adjusted == rgb(0xffffff) {
				t.Fatal("color was discarded despite a reachable target")
			}
			if score := math.Abs(perceptualContrast(adjusted, background)); score < textContrastMinimumLc {
				t.Fatalf("adjusted color failed APCA: %.3f", score)
			}
			before, after := oklchFromRGB(original), oklchFromRGB(adjusted)
			if hue := math.Min(wrapHue(before.h-after.h), wrapHue(after.h-before.h)); hue > 1.5 {
				t.Fatalf("adjustment changed hue by %.3f degrees", hue)
			}
			// A visibly smaller change must not already satisfy the target.
			closer := before
			closer.l = after.l + (before.l-after.l)*0.1
			if math.Abs(perceptualContrast(closer.rgb(), background)) >= textContrastMinimumLc {
				t.Fatal("adjustment overshot a closer passing color")
			}
			if readableTextColor(adjusted, background) != adjusted {
				t.Fatal("a passing result was adjusted again")
			}
		})
	}
}

func TestAPCAUsesStrongestFallbackWhenTargetIsUnreachable(t *testing.T) {
	for _, tc := range []struct{ background, want uint32 }{
		{0xed009e, 0xffffff},
		{0xe9b200, 0},
	} {
		background := rgb(tc.background)
		black, white := math.Abs(perceptualContrast(rgb(0), background)), math.Abs(perceptualContrast(rgb(0xffffff), background))
		if max(black, white) >= textContrastMinimumLc {
			t.Fatal("fixture has a reachable target")
		}
		got := readableTextColor(rgb(0x334455), background)
		if got != rgb(tc.want) || math.Abs(perceptualContrast(got, background)) != max(black, white) {
			t.Fatalf("background %06x did not get the strongest available contrast: %v", tc.background, got)
		}
	}
}
