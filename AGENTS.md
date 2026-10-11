# Commando File Manager

Commando is a native Gio file manager. Windows is the target platform;
Linux supports development and headless previews. Use `commando` for the
module/executable and `Commando` for the window title and data-folder name.

## Repository Layout

- `main.go`: application entrypoint.
- `internal/app`: UI, keyboard input, navigation, window integration and state.
- `internal/browser`: directory reads, drive enumeration and file opening.
- `internal/feedback`: contextual message lifecycle and animation, without Gio.
- `internal/app/theme.go`: colors, typography, font and row padding.
- `scripts/build-windows` and `scripts/commando.rc`: Windows build and resources.
- `dist/`: generated executable only. Agent-generated screenshots go in
  `tmp/agent/screenshots/`; temporary binaries go in `tmp/agent/bin/`.
  Keep the `tmp/` root free for user contributions. Private notes stay in `tmp/docs/`.

## Behavior to Preserve

- Three independent panels retain their own selection and scroll position.
- Ctrl+M/Ctrl+Shift+M cycles the optional metadata column forward/backward:
  None, Extension, Size, Modified. Save and restore each panel's choice in the
  shared state store, independently of startup paths. Missing or unknown choices
  default to Size. Keep the header synchronized
  with the column. None reserves no metadata width. Preserve listing order,
  marks, current row and scroll. Handle browsing keys in event order.
- File marks are separate from the current row, local to each panel, and not
  persisted. Preserve them during navigation within a listing and failed reads;
  clear them on successful listing changes. Exclude parent rows and drives.
  Keep mark bars independent of filename colors. Enter opens only the current row.
- F2 renames only the current file or folder inline. Select the basename for
  files and the whole name for folders; F2 in the editor selects the full name.
  Enter applies; Escape or another row
  cancels the draft. Keep Tab in the editor. Never replace an existing entry.
  Run the mutation through the directory worker; failures use the panel footer
  and retain the editor. Preserve marks and selection after reordering.
- Feedback owns the entire contextual panel footer, including the count area.
  Use fixed character timing, binary off/on blinks, then hold until interaction;
  never expire messages on a timer. Dismissing input must still perform its normal
  action, and message-producing actions must retain their result. Progress also
  clears on completion. Route operation feedback through internal/feedback.
  Keep semantic colors and independent per-kind animation profiles in theme.go;
  all kinds currently use the same timing. Preserve editor text, selection and
  focus. Observe native Windows editor keys without consuming them.
- Location and name editors share EditorText and TextSelection colors.
  Support Shift+Insert paste through the native Insert bridge on Windows and
  through Gio key events elsewhere; keep browsing marks separate from editors.
- Left/Right and Shift+Tab/Tab switch panels without wrapping. While editing a
  location, Tab/Shift+Tab retain editor text, selection and focus.
- Ctrl+Left/Right opens the selected folder, parent row or drive in the adjacent
  panel and moves focus there. Preserve the source listing, selection and scroll.
  Files, outer edges and busy panels produce no action. Failed reads retain the
  destination listing and show its footer error.
- On Windows, a bare ASCII drive letter with an optional colon opens its root
  (`d`/`d:` -> `D:\`). Explicit relative paths such as `.\d` retain their meaning.
- Directory reads run off the UI thread through one worker. Keep file-opening
  failures silent unless changing their feedback is part of the requested scope.
- Ctrl+R refreshes only the active folder or drive list, retaining the current
  item by name (drive by path) and the viewport where possible. Missing items
  select the nearest remaining row; empty lists select none. Busy panels and
  editors ignore refresh. Failed reads retain listing, selection, scroll and
  marks with footer feedback; successful reads clear marks. Completion must
  preserve subsequent selection changes and never steal panel focus.
- Ctrl+Shift+N/Ctrl+N creates a folder/empty file through a temporary inline row
  and the directory worker. Reuse the same name editor as Rename. Put the draft
  after the current row and keep it stationary while typing. Enter submits;
  Escape removes the draft and restores selection/scroll; another row cancels
  and selects that row. Tab retains editor focus. Busy panels, drives and other
  editors ignore these shortcuts.
  Never reuse an existing entry. Errors use the panel footer and retain the draft.
  Select the created item and preserve existing marks; update other panels
  showing the directory without changing their current item.
- Resolve one shared `dataDir` for state and other application data. Windows uses
  `%LOCALAPPDATA%\Commando`. Previews use fixed sample entries and bypass persistence.
- Ctrl+C/Ctrl+X puts marked files/folders, otherwise the current item, on the
  Windows file clipboard; preserve ordinary text clipboard keys inside editors.
  Ctrl+V transfers into the active folder, never its highlighted child. Read the
  current system clipboard through the single directory worker. Copy selections
  stay reusable; remove only confirmed moved items from cut selections and never
  replace a newer clipboard. Cut filename colors belong to the shared file list
  and remain independent of marks. Preserve panel focus, current-item identity,
  surviving marks, and open moved descendants. Preflight the whole batch for
  file conflicts before any mutation; recursively merge existing real folders
  and preserve destination-only entries. Refuse mismatched file/folder types and
  final-link collisions before the batch starts. One regular-file transfer
  offers Overwrite (default), Cancel. Folder contents and batches offer Overwrite
  all (default), Skip existing, Cancel, with one decision covering the batch.
  Keep shared inline rows beneath the first conflicting target file, or its
  top-level folder for nested conflicts. Omit headings and repeated filenames;
  keep the subject and choices visible together. Ensure the inspected subject
  exists in the cached listing before attaching actions. Explain ordinary
  conflicts through Warning feedback: File exists for a single file, a count
  for batches, without filenames. Up/Down selects, Enter applies, Escape
  or clicking an ordinary row cancels; clicking an option selects it. Closing
  actions with a choice or Escape selects and reveals their subject
  file. An ordinary-row click selects the clicked row instead. Keep the
  choice anchored while Left/Right or Tab/Shift+Tab switches panels. Retain the
  presented clipboard snapshot, stage the replacement fully and recheck file
  identity and metadata before replacing. Overwritten files are not recycled.
  Verify the full inspected decision before running it; changed entries require
  a fresh choice. Never overwrite a late arrival absent from the approved plan.
  Skipped files stay at the source. Remove only verified, now-empty merged
  source folders, and keep partly moved folders in the cut selection. Stop at
  runtime failure with partial results in the existing footer. Refresh affected
  open folders through the worker, retaining surviving marks, current items and
  viewport anchors; follow confirmed moved descendants into merged destinations.
  Transfer progress and cancellation require a separate agreed interaction model.
- Inline actions are temporary rows in the shared file list, separate from its
  entries, marks, current item and persisted state. Use full-contrast action text
  and the normal selection colors. InlineEdge also identifies new-name drafts;
  keep editor placeholders distinct from actionable options. Defer arriving
  choices until an active editor closes, preserving its text, selection and focus.
- Delete recycles marked entries, otherwise the current file/folder, through the
  single worker. Exclude parents and drives; keep Delete in editors and ignore
  modifier variants. Resolve SUBST backing paths freshly and verify identity.
  Use Windows IFileOperation with RECYCLEONDELETE and a pre-delete recycle veto;
  never fall back to permanent deletion or a legacy undo-if-possible API. Only
  confirmed recycled entries leave cached listings. Stop at the first failure
  and report partial success in the existing footer. Preserve surviving marks,
  current-item identity, scroll and focus in all panels showing the directory,
  including aliases. Hold new name mutations while recycling is pending.

## Changes and Documentation

- Discuss new interaction models with the user before implementing them.
- All UI models must be reusable, with either a general purpose (such as a modal
  or message channel) or a semantic purpose (such as a file list or search bar).
  Extend the appropriate shared model instead of adding an operation-specific UI.

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
