//go:build !windows

package browser

import "image"

func NewFileIconReader() func(string, int) image.Image { return nil }
