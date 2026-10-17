//go:build !windows

package browser

import "image"

func NewFileIconReader() func(string, int) image.Image { return nil }

func NewFileTypeIconReader() func(string, int, bool) image.Image { return nil }
