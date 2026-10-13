# Commando

**Work in progress.** Commando is an early preview; features and behavior may change.

![repocard](https://repocard.dannyben.com/svg/commando.svg)

A keyboard-first multi-panel file terminal for Windows.

## Download

[Download from GitHub Releases](https://github.com/DannyBen/commando/releases).

## Usage

Press Backtick or Ctrl+Shift+P to open the command palette and view key bindings.

Press `/` to filter filenames in the current panel. Matches appear as you type;
Enter returns to browsing and Escape clears the filter. Press `/` again to edit
the query. Filters remain when switching panels and clear when opening another
folder. Files hidden by a filter lose their marks. Ordinary typing still jumps
to a filename prefix.

Alt+Left/Right adds a panel beside the current one. Ctrl+Left/Right opens the
selected folder beside it, creating a panel at an outer edge. Ctrl+W closes a
panel; Ctrl+Shift+W reopens the last closed panel. Alt+1–9 focuses a numbered
panel. At least one panel stays open.

On Windows, adding or closing panels adjusts the window to preserve your
preferred panel width, within the current screen's available space. A manual
window-width resize sets a new preference. Maximized and snapped layouts keep
their window size. Panel count, locations, display choices and preferred width
are saved between sessions.

Ctrl+M / Ctrl+Shift+M cycles the metadata column. Ctrl+F3 toggles sorting by
that column for the current panel: extension ascending, size largest first,
modified newest first. Folders stay ahead of files. Without a metadata column,
the list uses its normal name order. Each panel remembers its sorting choice.
