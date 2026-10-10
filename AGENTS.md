# Project Status

The user authorized an initial native Gio UI evaluation prototype on 2026-10-10.
The user subsequently authorized Todo 17: initial real folder navigation,
then type-to-select, Enter to open files, and Alt+D path entry. File-opening
failures must be silent until their feedback is designed (Todo 19).
The current scope is a light-theme window with three independent directory
panels, type-to-select, Enter to open folders/files, Alt+D path entry,
Backspace to go up, keyboard file/panel navigation,
nonblocking directory loading, inline read errors, and ordinary scrolling.
Fixed sample data remains only for the reproducible headless preview.

Confirmed product direction: multi-panel support, a default of two or three
(still to decide), no tabs for now, a modal command palette, and minimal panel
headers/footers. The prototype uses three panels for evaluation.

Do not expand this into file mutation operations, transfer targeting,
palette interactions, persistence, or other product features without agreeing
on the next scope with the user. Keep preview sample data distinct from the live browser.

Read `docs/project-brief.md` before planning. Keep confirmed requirements,
prototype evidence, suggestions, and open decisions distinct.

# Machine Constraints

Parallel file access under `/vagrant` can hang the VM. Run commands that scan,
test, build, format, generate, render, use git, or walk many files sequentially.

# Documentation Mockups

- Establish a clear product definition and reviewed mockups before application code.
- Use the Todo CLI; every task description must start with a lowercase letter.
- `docs/` is the local PRD and mockup space and must remain ignored by Git.
- Documentation mockups use Slim/Slimview, a light theme, HTML tables for layout, and one shared
  stylesheet with the smallest practical CSS. Do not use flex or grid layouts.
- Use JavaScript only to demonstrate an agreed interaction.
- Keep mockup proposals distinct from confirmed product requirements.
