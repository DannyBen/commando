//go:build !windows

package sound

// Linux supports development and previews; playback is Windows-only.
func runAudio(_ <-chan *voice, stop <-chan struct{}, _ *audioStatistics) error {
	<-stop
	return nil
}
