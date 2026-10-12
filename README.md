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
- Left/Right switches panels without wrapping. Tab/Shift+Tab cycles forward or
  backward through all panels, wrapping at the edges.
- Shift+Left/Right swaps the active panel with its neighbor without wrapping.
  Focus follows the moved panel, along with its folder, selection, marks, scroll
  and metadata choice. The shortcut is ignored during editing, inline interactions
  or pending directory work. The resulting panel order is saved on close.
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
- Ctrl+R refreshes the active folder or drive list, retaining the current item
  and scroll where possible. If the item disappeared, the nearest row is selected.
  Busy panels and editors ignore the shortcut.

Each panel keeps its own selection and scroll position. Mouse selection, wheel
scrolling and scrollbars are supported. Folders appear first, with names displayed
without a trailing slash. The parent row is `..`. Directory reads run in the
background; a failed read shows an error in the panel footer and retains the listing.

Files sort by name before their final extension, so originals precede longer
variations and copies. Names compare case-insensitively, with numbers in numeric
order (`file2` before `file10`). Folder names compare whole; dotfiles such as
`.env` retain their full name.

## Metadata Column

Ctrl+M cycles the active panel through None, Extension, Size and Modified;
Ctrl+Shift+M cycles backward. Each panel remembers its own choice across restarts,
defaulting to Size. Startup folder arguments preserve those choices.
The header follows the column; None gives filenames the full
width and keeps only the NAME heading. Modified shows local dates as `YYYY-MM-DD`.
Changing the column preserves name ordering, marks, the current row and scrolling.

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
row. Entering a directory or refreshing clears marks; failed reads retain them.
Drives cannot be marked.

## Renaming

F2 edits the current file or folder name inline, even when other items are marked.
For files, the name before the final extension is selected; folders and names
such as `.gitignore` are selected in full. The extension remains editable.
Press F2 again to select the entire filename, including its extension.
Enter applies; Escape or clicking another row cancels. Tab/Shift+Tab keep editor
focus. An existing destination is never replaced; a failed rename shows an error
in the panel footer and keeps the editor open for correction.

Renaming keeps the item selected and preserves marks as the list reorders.
Both the filename and location editors support Ctrl+C, Ctrl+X and Ctrl+V;
Shift+Insert also pastes text.

## Creating Files and Folders

Ctrl+Shift+N adds a draft folder row after the current row; Ctrl+N adds a draft
empty file. Both reuse Rename's inline name editor. The draft stays stationary
while typing. Enter creates the item, sorts it into place and selects it.
Escape removes the draft and restores the previous selection and scroll;
clicking another row cancels and selects that row. Tab/Shift+Tab retain editor
focus. Ctrl+C, Ctrl+X, Ctrl+V and Shift+Insert paste are supported.

Use a single name. Existing entries are never replaced; errors appear in
the panel footer and keep the name editable for correction. Creation preserves
existing marks and updates other panels displaying the same directory. Busy
panels and drive lists ignore the shortcut. Submitted creation finishes before
the editor closes.

## Copying, Cutting and Pasting

F5 copies marked files and folders, or the current item when nothing is marked;
F6 moves them. Both select the panel on the right, or the panel on the left when
there is no panel on the right, and show an inline `Copy here` or `Move here`
action. Left/Right or Shift+Tab/Tab moves the action between destination panels,
skipping the source. Arrows stop at the edges; Tab/Shift+Tab cycles endlessly.
Enter starts the transfer into that panel's
open folder, regardless of its highlighted row, and keeps destination focus.
Escape cancels and restores source focus; clicking a file row cancels and selects
that row. Completion, cancellation and failures from Windows preserve any
subsequent panel navigation. These shortcuts leave the clipboard untouched.

Ctrl+C copies marked files and folders to the clipboard; Ctrl+X cuts them.
With no marks, both use the current item. Ctrl+V pastes into the active panel's
folder, regardless of its highlighted row. Windows shares file selections with
Explorer and other file managers. Inside editors, these shortcuts still edit text.

Cut filenames are dimmed independently of marks. Copy selections remain reusable;
successful moves remove completed items from the cut selection. Failed and
unprocessed items remain available for retry. Replacing the clipboard clears
the cut appearance.

Ctrl+D duplicates marked items, otherwise the current file or folder, without
changing the clipboard. Copying and pasting into the same folder uses the same
duplication behavior. A single item opens an inline name draft beneath its
source, suggesting `name - Copy.ext` (or `folder - Copy`). The basename is
selected for files, the full name for folders. Enter creates and selects the
copy; Escape or clicking another row cancels before copying. Tab retains the
editor. Existing names produce an error and retain the draft.

Batches duplicate directly with available copy names; subsequent names use
`name - Copy (2).ext`, and so on. Copies never replace existing entries. Cutting
and pasting into the same folder does nothing and shows "Already in this folder".

Copy, move and duplication use Windows file operations. Windows owns their
progress, cancellation, conflict choices, folder merging and error dialogs.
Commando accepts the files and folders Windows leaves after cancellation,
refreshes its panels, and retains unfinished cut items for retry.

Other filesystem mutations and file-clipboard writes wait until the running
operation has finished and its results are reflected in the panels. Normal close
requests keep Commando open during that work. Selection, scrolling and panel
navigation remain available where the native Windows UI permits them.

Linux supports development and headless previews; file transfers require Windows.

## Recycling Files and Folders

Delete sends marked files and folders to the Windows Recycle Bin, or the current
item when nothing is marked. Parent rows and drives are excluded. Shift+Delete
does not perform permanent deletion; Delete inside an editor still edits text.

SUBST drives use their backing paths for recycling. Windows Restore returns
items to those backing locations, making them visible through the aliases again.
If recycling is unavailable, the operation is refused; there is no permanent-delete
fallback. A batch stops at the first failure and reports any items already recycled.
Failed and unprocessed items retain their marks. Other panels showing the same
directory update without losing their surviving current items or marks.

Recycling runs in the background and uses the panel footer for progress and
results. Busy panels ignore Delete. New name mutations wait for recycling to
finish. Recycling is unavailable on the Linux development build.

## Feedback

Messages take over the panel footer, replacing its status and counts. They type
at a fixed speed, blink fully off/on twice, then remain until the next key press
or mouse click. That input still performs its normal action. Progress messages
also clear when their operation finishes.

Info is blue, success green, warnings orange and errors red. Feedback preserves
editor text, selection and focus.

`defaultTheme.Feedback` in [`internal/app/theme.go`](internal/app/theme.go)
holds independent `Info`, `Success`, `Warning` and `Error` animation profiles.
All start with a 35 ms `CharacterDelay`, 160 ms `BlinkInterval` and 2 `Blinks`.
Set its `Enabled` to false to disable animation globally, or disable an individual
profile. The corresponding theme colors define each kind's foreground.

The toolkit-independent [`internal/feedback`](internal/feedback/README.md) package
owns messages and animation; the app renders them in the contextual footer.

## Appearance and Saved State

Colors, text sizes, the bundled font and row padding are defined in
[`internal/app/theme.go`](internal/app/theme.go). Edit `defaultTheme` and rebuild
to adjust the appearance. Rows grow with the font height and center their text.
`Marked` controls the mark bars independently of filename colors.
`Metadata` and `SelectedMetadata` control the optional column's text colors.
`CutFilename` and `SelectedCutFilename` control cut filenames in ordinary and
selected rows.
`NameRow` controls the surrounding row during inline name editing; matching
`Surface` hides the row highlight. `EditorText` controls text in both the filename and
location editors; `TextSelection` controls their shared selection background.

Window position, size, maximized state, panel locations, metadata columns and the
active panel are saved on close. Windows stores state in
`%LOCALAPPDATA%\Commando\state.json`.

## Development

`op check` formats, vets and tests the app. `op preview` renders
`tmp/agent/screenshots/preview.png` through the same Gio layout with fixed sample entries,
without reading or writing saved state.

Filesystem transfer tests use disposable directories. For Windows file-clipboard
integration on a desktop, set `COMMANDO_TEST_CLIPBOARD=1` and run
`go test ./internal/browser -run WindowsFileClipboard`.
This optional test replaces the system clipboard. Explorer interoperability
still requires desktop testing. Linux development uses an in-process file clipboard.

For native transfer tests on a Windows desktop, set
`COMMANDO_TEST_SHELL_TRANSFERS=1` and run
`go test ./internal/browser -run WindowsShell`. These tests show real
Windows transfer UI and use disposable directories. Optionally set
`COMMANDO_TEST_TRANSFER_VOLUME` to a folder on another volume and
`COMMANDO_TEST_SUBST_ROOT` to an existing SUBST root to exercise those paths.
Drive mappings are left unchanged.

For Linux environments using Wayland/OpenGL without Vulkan or X11 support:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
```

The bundled [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) font
license is in [`internal/app/fonts/OFL.txt`](internal/app/fonts/OFL.txt) and
embedded as a Windows resource in `commando.exe`.
