# Commando File Manager

Commando is a native Gio file manager. Windows is the target platform;
Linux supports development and headless previews. Use `commando` for the
module/executable and `Commando` for the window title and data-folder name.

## Repository Layout

- `main.go`: application entrypoint.
- `internal/app`: UI, keyboard input, navigation, window integration and state.
- `internal/browser`: directory reads, drive enumeration and file opening.
- `internal/app/theme.go`: colors, typography, font and row padding.
- `scripts/build-windows` and `scripts/commando.rc`: Windows build and resources.
- `dist/`: generated executable only; previews belong in `tmp/`.

## Behavior to Preserve

- Three independent panels retain their own selection and scroll position.
- Left/Right and Shift+Tab/Tab switch panels without wrapping. While editing a
  location, Tab/Shift+Tab retain editor text, selection and focus.
- Ctrl+Left/Right opens the selected folder, parent row or drive in the adjacent
  panel and moves focus there. Preserve the source listing, selection and scroll.
  Files, outer edges and busy panels produce no action. Failed reads retain the
  destination listing and show its inline error.
- On Windows, a bare ASCII drive letter with an optional colon opens its root
  (`d`/`d:` -> `D:\`). Explicit relative paths such as `.\d` retain their meaning.
- Directory reads run off the UI thread through one worker. Keep file-opening
  failures silent unless changing their feedback is part of the requested scope.
- Resolve one shared `dataDir` for state and other application data. Windows uses
  `%LOCALAPPDATA%\Commando`. Previews use fixed sample entries and bypass persistence.

## Changes and Documentation

Keep changes within the requested scope. Preserve existing behavior unless the
change calls for adjusting it. Keep public documentation focused on usage,
architecture and development conventions; exclude local work-item identifiers,
conversation history, plans and progress reports.

Optional private notes may exist in ignored `tmp/docs/`. Consult them if present,
but do not require them to build or understand the repository. Keep local
planning and progress in those notes or the local Todo list, if available.

## Validation

Use `op check` for formatting, vet and tests, `op build-windows` for the Windows
executable, and `op preview` for the headless Gio preview. Tests must check behavior
rather than mirror implementation. A successful cross-build does not establish
native Windows behavior; distinguish automated checks from desktop testing.

On Linux environments using Wayland/OpenGL without Vulkan or X11 support:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
op build-windows
```

On `/vagrant` shared folders, run scans, formatting, tests, builds, rendering and
Git commands sequentially; parallel file access can hang the VM.
