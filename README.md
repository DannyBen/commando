# Commando

**Work in progress.** Commando is an early preview; features and behavior may change.

![repocard](https://repocard.dannyben.com/svg/commando.svg)

A keyboard-first multi-panel file terminal for Windows.

## Download

[Download from GitHub Releases](https://github.com/DannyBen/commando/releases).

## Usage

Press Backtick or Ctrl+Shift+P to open the command palette and view key bindings.


Press `@` to search settings. Under Programs › Editors, add an editor program,
space-separated extensions (or `*` for a fallback), and arguments containing
`%SelectedFiles%`. F4 edits marked files, otherwise the current file, launching
each matching editor once with its files.

Folder shortcuts (`.lnk`) open in the current panel; other shortcuts use their
normal Windows opening behavior.
