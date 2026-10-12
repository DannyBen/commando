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
- `.goreleaser.yml`: Windows x64 ZIP, checksums and license notices.
- `.github/workflows`: Linux/Windows checks and version-tag releases.
- `dist/`: generated executables and release archives. Agent-generated screenshots go in
  `tmp/agent/screenshots/`; temporary binaries go in `tmp/agent/bin/`.
  Keep the `tmp/` root free for user contributions. Private notes stay in `tmp/docs/`.

## Behavior to Preserve

- Foreground interactions have one session owner through interaction.go. Start
  and finish sessions through its controller; route input, cancellation and
  focus through the shared interaction helpers. Keep background operations and
  per-pane loading independent. Submitted name mutations retain their session
  until reconciliation, and stale sessions cannot end a newer interaction.
- Three independent panels retain their own selection and scroll position.
- Ctrl+H cycles each pane independently through All, Hide Hidden, and Hide Hidden
  + dotfiles. Windows hidden attributes and leading-dot names are separate.
  Always exclude entries carrying both hidden and system attributes; retain
  parents and drives. Store each pane's visibility choice with metadata,
  independently of startup paths; missing/unknown choices default to All.
  Keep the complete listing cached and filter its visible projection without
  filesystem reads. Preserve surviving current items, marks and viewport anchors;
  clear marks on entries that become invisible and select the nearest surviving
  row when needed. Keep creation, rename, recycle and transfer updates in both
  the complete cache and visible listing. Choices follow swapped panes.
  Editors, inline interactions, busy panes and drive screens ignore Ctrl+H.
  Report the mode through the existing contextual footer.
- Use browser.CompareEntries for reads, renames and inserted entries. Keep
  parents and folders first; compare files by stem, then final extension, and
  folders by whole name. Ignore case and compare ASCII number runs by value
  without integer overflow. Dotfiles such as .env have no extension. Break
  equivalent numeric names by fewer leading zeros, then exact names for case
  ties. Preserve current-item identity and marks when entries reorder.
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
  Keep messages within five short words, including partial-result counts;
  omit filenames. Preserve specific failure reasons rather than truncating text.
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
- Left/Right switches panels without wrapping. Tab/Shift+Tab cycles panels
  endlessly forward/backward. Browsing and inline interactions share the same
  navigation policy. While editing a location or name, Tab/Shift+Tab retain
  editor text, selection and focus.
- Shift+Left/Right swaps the active pane with its adjacent neighbor without
  wrapping. Move the entire pane model, retaining location, listing, current item,
  marks, viewport, metadata and feedback; focus follows the moved pane. Move
  cached cut-clipboard alias locations with the panes without changing the
  clipboard. Ignore swapping during editors, inline interactions, pending
  transfers or any pane's pending directory work so captured worker indices stay
  valid. Persist the resulting order through the existing state store.
- Ctrl+Left/Right opens the selected folder, parent row or drive in the adjacent
  panel and moves focus there. Preserve the source listing, selection and scroll.
  Files, outer edges and busy panels produce no action. Failed reads retain the
  destination listing and show its footer error.
- On Windows, a bare ASCII drive letter with an optional colon opens its root
  (`d`/`d:` -> `D:\`). Explicit relative paths such as `.\d` retain their meaning.
- Directory reads run off the UI thread through one worker. Keep file-opening
  failures silent unless changing their feedback is part of the requested scope.
- Windows directory notifications trigger quiet, coalesced automatic refreshes
  of open folders through the same worker. Protect foreground sessions and
  pending mutations; defer both dispatch and arriving results when necessary.
  Preserve surviving marks, current-item identity, viewport, focus, filename
  prefixes and operation feedback. Discard stale results after navigation. Keep
  ordinary browsing available during automatic reads, retain failed listings,
  and release notification handles on navigation and shutdown. Ctrl+R remains
  the fallback for unsupported locations; previews bypass directory monitors.
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
  surviving marks, and open moved descendants. Windows IFileOperation is the
  only copy/move/duplicate engine. It owns native progress, cancellation,
  conflicts, merges and error dialogs. Accept its resulting filesystem state
  after cancellation; never perform rollback, cleanup or source deletion after
  the shell operation. Do not retain an internal transfer fallback. Resolve
  SUBST and parent aliases before dispatch. Observe actual per-item results,
  including names changed by Windows and partially completed folder moves.
  Refresh captured open folders after every attempted batch, retaining surviving
  marks, current items and viewport anchors. Follow confirmed moved descendants.
  Linux supports development/previews and refuses actual transfers.
  Categorize requests as browsing, mutation or file-clipboard writes through
  the shared busy policy. Block new mutations and file-clipboard writes until
  reconciliation completes. Keep normal close requests blocked while mutations
  are pending; on destruction, drain active results before saving state and exit.
- Inline actions are temporary rows in the shared file list, separate from its
  entries, marks, current item and persisted state. Use full-contrast action text
  and the normal selection colors. InlineEdge also identifies new-name drafts;
  keep editor placeholders distinct from actionable options. Defer arriving
  choices until an active editor closes, preserving its text, selection and focus.
- F5/F6 captures marked files/folders, otherwise the current item, for direct
  copy/move without reading or changing the file clipboard or cut appearance.
  Start with the inline Copy here / Move here action in the pane on the right,
  falling back to the pane on the left at the right edge. Left/Right and
  Shift+Tab/Tab carry the shared inline action between destination panels,
  skipping the source. Arrows stop at the edges; Tab cycles endlessly through
  destinations using the shared panel navigation policy. Enter on Copy here /
  Move here transfers into the pane's open folder, never its highlighted child.
  Ignore shortcuts in editors or busy source panes; refuse drive lists and busy
  destinations without losing targeting. Escape restores source focus; clicking
  a real row cancels and performs normal selection. Restore visited viewports,
  marks and current items. Confirmation keeps destination focus; completion,
  Windows cancellation and failures preserve subsequent browsing. Reuse the
  worker, native Windows engine and reconciliation; alias destinations retain
  existing duplication/move-no-op rules.
- Ctrl+D duplicates marks, otherwise the current file/folder, without reading
  or changing the clipboard or cut appearance. Same-folder copy/paste uses the
  same flow, resolving SUBST and parent aliases through the worker. One item
  uses the shared inline name editor beneath its source, suggesting an available
  name - Copy.ext; select the basename for files and the full name for folders.
  Enter creates/selects the copy; Escape or another row cancels before copying;
  Tab retains the editor. Existing names retain an editable draft with an error.
  Batches use available names directly, numbering further copies with (2), (3),
  and so on. Dispatch duplicates through the same Windows engine with collision
  renaming, so existing duplicate destinations are preserved. Preserve marks and other panes' current rows,
  updating alias listings through worker-resolved locations. Defer arriving
  name drafts behind editors and cancel if their pane has been left. Same-folder
  cut/paste is a no-op with Already in this folder feedback; retain the clipboard.
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
- Do not add tooltips. App-owned decisions use in-panel UI; Windows transfer
  dialogs are the explicit native-shell exception.
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
