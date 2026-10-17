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

- Regular outbound dragging starts after a mouse movement threshold on a file
  or folder. A marked row carries the marked set; an unmarked row carries only
  that item. Suppress the originating click after a drag, retaining normal clicks
  and independent marks. Ctrl+Shift+D and Entry › Arm drag capture marks, otherwise
  the current item, for button-free dragging: the next complete click drops;
  Escape cancels. Ignore an invocation button held initially until it is released.
  Armed drag shows Info through the shared feedback channel with Immediate
  style: full, steady text and the usual semantic sound. Submit that frame before
  entering the native loop; do not wait for an animation.
  An armed click released over Commando's window is ignored and resets click
  tracking; retain arming for the next external click. Regular release is unchanged.
  Both modes use the same native shell source/data object and copy-only effects,
  without accessing the clipboard or deleting sources. Commando does not register
  its panels as drop targets; F5/F6 remains the panel transfer flow. Reuse shared
  native sessions, eligibility, close/mutation blocking and quiet reconciliation.
  Keep the source pinned/rooted until its final COM Release. Native completion
  reports acceptance rather than claiming the destination finished an upload.
- Right-click targets only the clicked file or folder, selects its row and panel,
  and retains independent marks. Alt+/ and the native Menu key use marked items,
  otherwise the current item; command search uses the same handler. Keyboard
  menus anchor beside the revealed current row. Exclude parents, drives and blank
  areas. Use shared foreground readiness and action policy, without enumerating
  competing editors or overlays in the feature. Native shell sessions hold input,
  mutations, clipboard writes and close requests until completion. Run the native
  loop on the window's OLE thread after submitting the frame, keeping Gio's event
  consumer available. Keep the shell host for the window lifetime and build fresh
  menus. Quietly reconcile open folders through the existing worker after shell
  commands, preserving surviving marks, selection, filters and viewports.
- Windows keeps one instance per shared data folder in the login session.
  Acquire ownership before loading state; previews bypass it. Later launches
  ignore supplied paths, request activation and exit without loading or saving
  state. Restore minimized windows, retaining their prior maximized state; leave
  other geometry and UI interactions intact. Hand foreground permission from the
  launching process to the existing process. Keep ownership until operations
  drain and state is saved, and allow recovery after an owner exits or crashes.
  Verify foreground activation on a real Windows desktop before marking complete.
- `/` edits a case-insensitive substring filter for the current folder panel.
  Replace NAME/metadata headings at exactly their existing height while filtering;
  keep the location bar and footer available. Enter finishes query editing without
  opening a result; Enter in browsing opens the current item. Up/Down navigates
  matches during editing, while Left/Right retains native caret movement.
  Tab/Shift+Tab and Alt+1–9 leave query editing and switch panels. Returning to a
  filtered panel restores browsing focus; `/` selects its existing query for editing.
  Escape clears the filter (a browsing filename prefix clears first). Queries stay
  through panel switches and refreshes, and clear on successful folder navigation.
  Filter the cached projection, retaining parents, sorting and surviving selection,
  marks and viewport anchors; entries hidden by a query lose their marks.
- Foreground interactions have one session owner through interaction.go. Start
  and finish sessions through its controller; route input, cancellation and
  focus through the shared interaction helpers. Keep background operations and
  per-pane loading independent. Submitted name mutations retain their session
  until reconciliation, and stale sessions cannot end a newer interaction.
- Backtick/Ctrl+Shift+P opens the command palette through the shared interaction
  controller. Keep its originating pane as the command context and restore
  browsing focus on dismissal. Search uses the Site Command Palette fuzzy
  scoring and best-alignment highlights. Enter dispatches through existing
  browsing handlers. Display group and action with a `›` separator; keep catalogue
  order for a blank query and fuzzy-rank the full label while searching. Unavailable
  actions remain visible and cannot execute.
  Generate Panel › Focus N entries for exactly the live panel count, in numeric
  order within the Panel group, without locations. Show Alt+1–9 for the first
  nine; higher panels remain executable without shortcuts. Both routes use the
  shared panel-focus helper. Preserve useful catalogue entries rather than
  trimming the list for its size; categories and search provide discovery.
  Keep native text editing and Tab focus inside Search; block pointer input
  to the file panels while open. Center the square overlay horizontally below
  the location bars, keeping their height synchronized with text metrics. Home/End
  select the first/last result; modified keys retain native text editing.
  Use the existing surface color for Search. Size the palette for its full
  catalogue's widest label and shortcut, including row padding, borders and
  scrollbar space; keep the width stable while searching. Clamp it to theme bounds,
  and use the full app width below the minimum. Reuse existing theme colors and
  terminal-style lines.
  Wash the file lists and footers with the existing surface color at the theme's
  overlay backdrop opacity; keep location bars and the palette at full contrast.
- A variable number of independent panels retain their own selection and scroll
  position. Alt+Left/Right inserts a panel beside the current pane, cloning its
  location, selection, viewport and display preferences with independent caches
  and no marks. Ctrl+W closes the current pane, retaining at least one; focus goes
  to its right successor or left neighbor. Ctrl+Shift+W restores closed panes in
  reverse order, including marks and viewport, then quietly reconciles changes.
  Alt+1–9 focuses a numbered pane; bare digits remain filename-prefix input.
  Editors, foreground sessions, pending directory work and file operations block
  insertion/removal so captured worker indices remain valid. Move clipboard alias
  cache slots with inserted/removed/restored panes without changing the clipboard.
  Closed-pane history lasts for the session. Persist live pane count/preferences.
  Windows panel-count changes preserve a separately saved base width, grow/shrink
  within the current monitor's work area and retain relative horizontal alignment.
  Only manual width resizing learns a new base; height-only changes, automatic
  sizing, clamping, snap/maximize/restore and DPI changes do not. Leave snapped or
  maximized geometry under OS control. Keep base width separate from pane models
  so future fixed width presets can build on it.
- Feedback › Sounds contains Profile and Master volume. Profiles ascend through
  Off, Errors only, Warnings and errors, Notifications only, All except navigation,
  and All sounds. Defaults preserve all cues at 50% master volume. Each cue in
  `internal/sound/sounds.toml` declares a positive numeric level and synthesis
  parameters; profiles use cumulative numeric thresholds, with 0 silent and -1
  including every configured level. Keep profile labels and order in settings
  metadata, independently of the semantic event-to-cue map. Preview sound setting
  changes through the same gate and master volume, with one cue; Off and zero
  volume remain silent. Stop active audio when profile or volume changes.
- Choice and autocomplete steppers use Home/End for the first/last available
  choice. Keep native caret movement while editing and leave the action bar
  unchanged. Repeated endpoint presses and key releases stay silent.
- Keyboard navigation plays the quiet semantic Navigation cue, when allowed by
  the sound profile, when the current
  row or active panel changes, including palette results and inline choices.
  Keep navigation at an edge, key releases and native text editing silent.
- Escape uses the shared cancellation helper and plays one single-note Cancel
  cue. Preserve submitted name sessions; ignored Escape and key releases stay
  silent. A cancellation result keeps its animation without a second tone.
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
  None, Extension, Size, Modified, Created, skipping disabled columns.
  View › Metadata columns controls the four actual columns; None is
  always available. Created defaults to off, the others to on. Disabling a
  current column advances affected live and closed panels to the next enabled
  choice; Cancel restores their original columns. Save each panel's choice in the
  shared state store, independently of startup paths. Missing or unknown choices
  default to Size. Keep the header synchronized
  with the column. None reserves no metadata width. Preserve listing order,
  marks, current row and scroll. Handle browsing keys in event order.
- Ctrl+F3 toggles Sort by metadata independently for each panel; the command
  palette uses the same browsing handler. Persist the choice independently of
  startup paths, defaulting to off. None uses name order without clearing the
  choice. Extension sorts ascending; Size, Modified and Created sort descending,
  with unknown values last and natural-name ties. Keep parents first, then folders,
  then files. Folders use name order for Extension/Size and date then name for
  Modified/Created. Creation time is unknown when the platform does not provide
  it; do not substitute modification or inode-change time. Reorder cached
  projections without reads, retaining current items,
  marks and viewport anchors. Apply the same comparator after reads, filtering,
  renames, creation and transfers. Editors, inline interactions, busy panels
  and drive lists ignore the toggle; choices follow swapped panes.
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
  Reveal text with fixed character timing, without typing sounds. Play the
  semantic message tone once at the start, and keep the cursor blinking with
  binary off/on states until interaction; never expire messages on a timer.
  Dismissing input must still perform its normal
  action, and message-producing actions must retain their result. Progress also
  clears on completion. Route operation feedback through internal/feedback.
  Keep semantic colors and independent per-kind animation profiles in theme.go;
  all kinds currently use the same timing. Preserve editor text, selection and
  focus. Observe native Windows editor keys without consuming them.
  Directory navigation plays its own immediate cue. Schedule Reading folder
  through the channel's delayed progress interface using FeedbackProgressDelay
  in theme.go (500 ms by default). Pending messages leave normal status visible;
  completion, dismissal or replacement cancels them. Once visible, delayed
  progress uses normal Info sound, animation and edge styling. Manual refresh
  uses the same delayed progress; automatic refresh stays quiet.
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
  Files open their containing folder in the adjacent panel and select the file.
  At outer edges, create a pane on that side before opening it. Busy
  panels produce no action. Failed reads retain the
  destination listing and show its footer error.
- On Windows, a bare ASCII drive letter with an optional colon opens its root
  (`d`/`d:` -> `D:\`). Explicit relative paths such as `.\d` retain their meaning.
- Opening a Windows .lnk shortcut to a filesystem folder navigates in the current
  panel. Resolve it on activation through the shared worker, with no shortcut edits
  or resolver dialogs. Other targets retain native opening. Keep the shortcut a
  file for listing, marking, rename, copy, move and recycle operations.
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
- Alt+D accepts quoted folder or file paths. Files open their containing folder
  and select the file; missing paths open their nearest existing ancestor with
  Path not found warning. Permission/read failures and unavailable ancestry
  retain the current listing, selection, marks and viewport.
- Missing open/restored folders recover through existing parents, then fall back to `C:\`
  (filesystem root on Linux). Use the existing directory worker and notification
  refresh flow; the Windows watcher also checks existence once per second.
  Preserve focus, panel preferences and operation feedback; clear listing-specific
  marks, prefixes and viewport when recovery changes the location. Respect active
  editors and pending operations, and discard stale results after navigation.
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
  Creation accepts relative paths with `/` or `\`, creating missing parent
  directories before exclusively creating the final file or folder. Validate
  every component before creating anything. Failures may leave created parents;
  retain the editable path for retry. On success, open the final item's parent
  and select it; changing folders clears listing-specific marks as usual.
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

- F4 and File › Edit use marked files, otherwise the current file; exclude
  folders, parents and drives. Programs › Editors provides New and saved-rule
  widgets named by their space-separated extensions, with Extensions, Program
  and Arg fields. Retain entered extension spelling/order; normalize only matching.
  Extension matching ignores case and optional leading dots; `*` alone
  is fallback. Specific rules precede fallback; overlapping rules use saved order.
  Group files by matching rule and invoke each program once. `%SelectedFiles%`
  expands after argument parsing into separate paths; never invoke a shell.
  Reuse settings sessions, file picking, persistence and the shared worker.
  File pickers change the value only on Enter over a file, returning to the widget;
  folders open normally, and Escape returns without changing the file value.
  Saved rules replace Reset with Delete, staged until Save and reversible through
  Cancel; New has no Reset/Delete. Separate launches per file remain future work.

- Editable colors use LSH: lightness and saturation are 0–100; hue is a
  360-degree wheel. Saturation is the percentage of usable sRGB OKLCH chroma
  at the chosen lightness and hue. UI, clipboard and saved values share units.
  Color offsets use points; multipliers remain dimensionless. Non-file
  selections retain normal text when readable on their background, otherwise
  use the better-contrasting black or white through shared theme resolution.
  File-color selections retain their existing inversion behavior.

## Changes and Documentation

- Do not edit any README file unless the user explicitly requests a README update.
- Discuss new interaction models with the user before implementing them.
- Do not add tooltips. Name editing and transfer targeting use in-panel UI;
  command search uses a modal overlay. Windows transfer dialogs and context
  menus remain native-shell exceptions.
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

Use `op loc` for occasional project-size reviews and when a change appears
unexpectedly large. It reports production Go and test lines separately;
investigate growth that is disproportionate to the feature's scope.

Use `op check` for formatting, vet and tests, `op build-windows` for the Windows
executable, and `op preview` for the headless Gio preview. Tests must check behavior
rather than mirror implementation. A successful cross-build does not establish
native Windows behavior; distinguish automated checks from desktop testing.
Build only the main dist/commando.exe. If it cannot be written, report that it
was not built; do not create an alternative executable.

On Linux environments using Wayland/OpenGL without Vulkan or X11 support:

```sh
GOFLAGS='-tags=novulkan,nox11' op check
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' op preview
op build-windows
```

On `/vagrant` shared folders, run scans, formatting, tests, builds, rendering and
Git commands sequentially; parallel file access can hang the VM.
