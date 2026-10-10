# Project Status

The user authorized an initial native Gio UI evaluation prototype on 2026-10-10.
The current scope is a light-theme window with three sample file panels, real
Gio lists, keyboard file/panel navigation, and ordinary list scrolling.

Confirmed product direction: multi-panel support, a default of two or three
(still to decide), no tabs for now, a modal command palette, and minimal panel
headers/footers. The prototype uses three panels for evaluation.

Do not expand this into filesystem browsing/operations, transfer targeting,
palette interactions, persistence, or other product features without agreeing
on the next scope with the user. Keep sample data distinct from real files.

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
