# CONTROL

A native Gio UI study for a keyboard-oriented, multi-panel file manager.
The current 800 × 480 light-theme window shows three sample directories, with
one file list per panel and compact path and status bars. It bundles JetBrains
Mono, with blue filenames and golden-yellow sizes.

Open `dist/control.exe` on Windows, or run `op run` on a graphical Linux desktop.

- Up/Down selects a file; Left/Right switches panels.
- Each panel keeps its own selection and scroll position.
- Home/End and Page Up/Down navigate longer lists.
- Mouse selection, wheel scrolling, and scrollbars also work.

This is a UI evaluation prototype. Entries are sample data; filesystem
operations, tabs, and the modal command palette are not implemented.
Three panels is the prototype choice; the product's default of two or three
remains open. See [the project brief](docs/project-brief.md).

`op check` formats, vets, and tests the app. `op build-windows` builds the Windows
amd64 GUI executable with its placeholder icon. `op preview` renders
`dist/preview.png` through the same Gio layout.

For this VM's Wayland/OpenGL build:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
op build-windows
```

The bundled [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) font
license is in `internal/app/fonts/OFL.txt` and copied into `dist/` for Windows.
