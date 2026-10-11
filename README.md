# Commando File Manager

A lightweight native file manager with three independent directory panels and
keyboard navigation. Built with Gio, with a compact monospace interface and
bundled JetBrains Mono.

## Build and Run

The commands below use Go and Opcode (`op`). The Windows build also requires
MinGW-w64's `x86_64-w64-mingw32-windres` resource compiler.

```sh
op build-windows
```

Run `dist/commando.exe` on Windows. On a graphical Linux desktop, use `op run`.
Supply up to three folder paths to choose the starting locations; omitted panels
use the first supplied folder. Without arguments, saved locations are restored,
or panels start at home.

## Keyboard Navigation

- Up/Down selects a file; Home/End and Page Up/Down navigate longer lists.
- Left/Right or Shift+Tab/Tab switches panels without wrapping.
- Ctrl+Left/Right opens the selected folder in the adjacent panel and moves
  focus there, preserving the source panel. The outer edges do not wrap.
- Type a filename prefix to select it without filtering. Matching is
  case-insensitive; Escape or a one-second pause clears the prefix.
- Enter opens a folder or opens a file in its associated application.
- Alt+D edits the active location. Enter navigates; Escape cancels.
  Tab/Shift+Tab keep focus in the location editor while editing.
- Relative paths resolve from the current folder. On Windows, `d`, `d:`, and
  `D:\` open the drive root; `.\d` opens a relative folder named `d`, and
  `\Projects` starts at the current drive's root.
- Backspace goes to the parent folder and selects the folder just left.
  At a drive root, it shows drives in the same panel. Enter opens a drive;
  returning selects the drive just left.

Each panel keeps its own selection and scroll position. Mouse selection, wheel
scrolling and scrollbars are supported. Folders appear first, with names displayed
without a trailing slash. The parent row is `..`. Directory reads run in the
background; a failed read shows an inline error and retains the listing.

## Marking Files

Marks use a left-edge bar and remain separate from the current-row highlight.

- Insert toggles a mark and moves down; Space toggles without moving.
- Shift+Up/Down extends a range; reversing direction shrinks it.
- Ctrl+A marks all files and folders, excluding the parent row.
- Escape clears a typing prefix first, otherwise it clears marks.
- Ctrl+click toggles a mark; Shift+click extends a range. Ordinary clicks and
  navigation preserve marks. Space remains part of filename typing while a
  prefix is active.

Each panel retains its marks when switching panels. Enter opens only the current
row. Entering a directory clears marks; failed reads retain them. Drives cannot
be marked.

## Appearance and Saved State

Colors, text sizes, the bundled font and row padding are defined in
[`internal/app/theme.go`](internal/app/theme.go). Edit `defaultTheme` and rebuild
to adjust the appearance. Rows grow with the font height and center their text.
`Marked` controls the mark bars independently of filename colors.

Window position, size, maximized state, panel locations and the active panel are
saved on close. Windows stores state in `%LOCALAPPDATA%\Commando\state.json`.

## Development

`op check` formats, vets and tests the app. `op preview` renders
`tmp/preview.png` through the same Gio layout with fixed sample entries,
without reading or writing saved state.

For Linux environments using Wayland/OpenGL without Vulkan or X11 support:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
```

The bundled [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) font
license is in [`internal/app/fonts/OFL.txt`](internal/app/fonts/OFL.txt) and
embedded as a Windows resource in `commando.exe`.
