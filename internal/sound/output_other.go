//go:build !windows

package sound

// Linux supports development and previews; playback is Windows-only.
type output struct{}

func openOutput() (*output, error) { return &output{}, nil }
func (*output) play([]int16) error { return nil }
func (*output) close()             {}
