# CONTROL

A native Gio prototype for a keyboard-oriented, multi-panel file manager.
The light-theme window defaults to 800 × 480 and browses three independent directories, with
one file list per panel and compact path and status bars. It bundles JetBrains
Mono.

Colors, text sizes, and the bundled font are defined in
[`internal/app/theme.go`](internal/app/theme.go). Edit `defaultTheme` and rebuild
to adjust the appearance.
Header/footer and scrollbar colors are independent. `RowPaddingVertical` adds
space above and below each line; rows grow with the measured font height and
keep the text vertically centered. `RowPaddingHorizontal` controls side spacing.

Open `dist/control.exe` on Windows, or run `op run` on a graphical Linux desktop.

- Up/Down selects a file; Left/Right switches panels.
- Type a filename prefix to select it, without filtering the list. Matching is
  case-insensitive; the footer shows the prefix. Escape or a one-second pause
  clears it. Backspace always goes to the parent when the file list is focused.
- Enter opens folders or opens files in their associated application. File-opening
  failures are silent for now.
- Alt+D edits the active panel path. Enter navigates; Escape cancels. Relative
  paths resolve from the current folder.
- On Windows, `d`, `d:`, and `D:\` open the drive root. Use `.\d` for a relative
  folder named `d`; `\Projects` starts at the current drive's root.
- Backspace at a drive root shows the drive list in that panel. Enter opens
  the selected drive; returning selects the drive you just left. Backspace
  in the drive list stays there.
- Going up selects the folder you just left.
- Each panel keeps its own selection and scroll position.
- Home/End and Page Up/Down navigate longer lists.
- Mouse selection, wheel scrolling, and scrollbars also work.
- Tab/Shift+Tab are reserved. Alt+Enter fullscreen switching is disabled.

The window position, size, maximized state, panel locations, and active panel
are saved on close and restored at startup. On Windows, state is stored in
`%LOCALAPPDATA%\CONTROL\state.json`. Panels start at home on the first run.
Supply up to three folders with `op run /path/one /path/two /path/three` to
override saved locations; omitted panels use the first supplied folder.
Folders appear first, followed by files, in name order. Directory reads
run in the background; a failed read shows an inline error and keeps the listing.

This is a browsing prototype. Copy/move, tabs, and the modal command palette
are not implemented.
Three panels is the prototype choice; the product's default of two or three
remains open. See [the project brief](docs/project-brief.md).

`op check` formats, vets, and tests the app. `op build-windows` builds the Windows
amd64 GUI executable with its placeholder icon. `op preview` renders
`tmp/preview.png` through the same Gio layout using fixed sample entries,
without reading or writing saved state.

For this VM's Wayland/OpenGL build:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
op build-windows
```

The bundled [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) font
license is in `internal/app/fonts/OFL.txt` and embedded as a Windows resource
in `control.exe`. The `dist/` folder contains only the executable.
