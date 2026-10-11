# Project Status

The user authorized an initial native Gio UI evaluation prototype on 2026-10-10.
The user subsequently authorized Todo 17: initial real folder navigation,
then type-to-select, Enter to open files, and Alt+D path entry. File-opening
failures must be silent until their feedback is designed (Todo 19).
Todo 25 authorizes Backspace at a filesystem root to show drives in the same
panel; Enter opens a drive and returning reselects the drive just left.
The current scope is a light-theme window with three independent directory
panels, type-to-select, Enter to open folders/files, Alt+D path entry,
Backspace to go up, keyboard file/panel navigation,
nonblocking directory loading, inline read errors, and ordinary scrolling.
Fixed sample data remains only for the reproducible headless preview.

Todo 32: Ctrl+Left/Right opens the selected folder (including a parent row or
drive) in the adjacent panel and moves focus there, preserving the source
listing/selection/scroll. No wrapping for v0.1.0. At an outer edge, on a file,
or while either involved panel is loading, do nothing. Read errors appear in
the destination and retain its previous listing, with focus there.

Todo 35: in the Windows location editor, bare ASCII drive letters with an
optional colon always open the drive root (`d`/`d:` -> `D:\`). Explicit relative
paths such as `.\d` retain their meaning. Other operating systems keep ordinary
relative-path behavior; unavailable drives use the existing inline read error.

The user authorized Todos 15 and 30 on 2026-10-11: save window position, normal
size and maximized state, panel locations (including Drives), and the active
panel on close; restore on startup. Explicit startup paths override saved
locations. Use one shared `dataDir`, defaulting to the user's application data
folder (`%LOCALAPPDATA%\CONTROL` on Windows), for state and future settings/data.
User-configurable location and its bootstrap pointer are future scope; no
settings UI or relocation is authorized yet. Previews must bypass persistence.

Confirmed product direction: multi-panel support, a default of two or three
(still to decide), no tabs for now, a modal command palette, and minimal panel
headers/footers. The prototype uses three panels for evaluation.

Do not expand this into file mutation operations, transfer targeting,
palette interactions, additional persisted state, or other product features without agreeing
on the next scope with the user. Keep preview sample data distinct from the live browser.

Read `docs/project-brief.md` before planning. Keep confirmed requirements,
prototype evidence, suggestions, and open decisions distinct.

# Backlog and v0.1.0

Classify open Todo items with simple category tags: `+visual`, `+keys`,
`+selection`, `+actions`, `+feedback`, `+settings`, `+palette`, or `+release`.
Use at least one category; use more when the task crosses categories. Existing
tags such as `+plan`, `+discuss`, and `+maybe` describe status or uncertainty.
`+v1` marks v0.1.0 scope, `+now` marks current work, and `+done` marks completion.
Milestone membership does not make an item current work automatically.

The agreed v0.1.0 target is three functional, satisfying panels: one theme
defined in code, keyboard navigation and selection, opening/external editing,
copy/move, rename, new folder, Recycle Bin deletion, and clear operation
feedback. Adjacent-panel opening and pane swapping follow the core actions.
Settings, command palette, icons, decorative additions, archives, and panel
count/arrangement controls are outside this milestone. Keep tasks broad at
first; define the detailed behavior as each task becomes current work.

# Machine Constraints

Parallel file access under `/vagrant` can hang the VM. Run commands that scan,
test, build, format, generate, render, use git, or walk many files sequentially.

# Documentation and UI Evaluation

- Agree on the next product scope before implementing it. Evaluate UI changes
  directly in the native Gio prototype and its headless preview.
- Use the Todo CLI; every task description must start with a lowercase letter.
- `docs/` is the local PRD and discussion space and must remain ignored by Git.
- Keep UI proposals and prototype evidence distinct from confirmed product requirements.
