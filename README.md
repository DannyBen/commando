# Commando

**Work in progress.** Commando is an early preview; features and behavior may change.

![repocard](https://repocard.dannyben.com/svg/commando.svg)

A keyboard-first file manager for Windows with three independent panels.

## Download

Download the Windows x64 ZIP from [GitHub Releases](https://github.com/DannyBen/commando/releases),
extract it, and run `commando.exe`.

Or install with [eget](https://github.com/zyedidia/eget):

```powershell
eget DannyBen/commando --pre-release
```

## Basic Shortcuts

| Key | Action |
| --- | --- |
| Tab / Shift+Tab | Cycle panels |
| Enter / Backspace | Open an item / go up |
| Alt+D | Edit the current location |
| Insert / Space | Mark an item |
| F5 / F6 | Copy / move; choose a destination panel and press Enter |
| F2 | Rename |
| Ctrl+N / Ctrl+Shift+N | Create a file / folder |
| Ctrl+C / Ctrl+X / Ctrl+V | Copy / cut / paste |
| Delete | Send marked or current items to the Recycle Bin |
| Ctrl+H | Cycle hidden-file visibility |

For building from source and contributing, see [CONTRIBUTING.md](CONTRIBUTING.md).

Licensed under [MIT](LICENSE). The bundled JetBrains Mono font uses the
[SIL Open Font License](internal/app/fonts/OFL.txt).
