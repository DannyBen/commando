# Showroom

Generate Commando showcase screenshots, video masters, MP4 previews and GIFs
from scripted scenes. Configuration lives in `config/`; generated files stay in
the locally ignored `out/` directory. Run `showroom publish` after capturing and
encoding to copy the overview screenshots, MP4 previews, and JPEG posters into
`../../site/assets/`. The site owns its logo and social image separately.

## Dependencies

- Linux with a C compiler, `pkg-config`, EGL/OpenGL ES support, and Wayland and
  xkbcommon development libraries. Mesa supports headless rendering; no desktop
  session is required. The project's Ubuntu CI installs `libegl-dev`,
  `libwayland-dev` and `libxkbcommon-dev`.
- Go matching the version in [go.mod](../../go.mod), for screenshot capture.
- Ruby and the `docopt_ng` gem declared in [Gemfile](Gemfile).
- FFmpeg with the `ffv1` and `libx264` encoders, plus `ffprobe`, for video assembly
  and GIF conversion.
- [Gifski](https://github.com/ImageOptim/gifski), for GIF conversion.

Tests also require the `minitest` gem.
External executables must be available on `PATH`.

## Usage

```sh
showroom --help
showroom shot overview
showroom shot panels
showroom shot themes
showroom shot commands
showroom vid panels
showroom vid themes
showroom vid commands
showroom publish
```
