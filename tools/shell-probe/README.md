# Windows shell probe

A separate Gio executable for testing native outbound file dragging and the
traditional Windows shell context menu. Commando's main UI is unchanged.

Build with `op build-shell-probe`, then run
`tmp/agent/bin/commando-shell-probe.exe` on Windows. No arguments are needed.
Every launch creates a new disposable test folder with `source`, `destination`
and `session.log`. The test folder remains after closing for inspection.
“Open test folder” opens it in Explorer.

## Desktop checks

1. Open `destination` in Explorer. Drag `alpha.txt` from the demo into it while
   holding Ctrl. Check that the contents copied and the original remains.
2. Ctrl+click multiple rows, then drag a marked row. Check that the entire batch
   arrives, including a folder's contents and names containing spaces or Unicode.
3. Hold Shift during a drag to request a move. Check both source and destination;
   the demo refreshes after the native drag returns. It never deletes sources
   based solely on the reported drag effect.
4. Drag `image.png` onto an application or browser file-upload target that
   normally accepts Explorer file drops. Check that the target reads the file.
5. Begin a drag and press Escape. Check that no files transferred and the demo
   still responds to another drag.
6. Right-click a file, folder and marked batch. Try Open, Properties, an available
   Open with or Send to submenu, and an installed extension command. Check that
   commands act on the intended items. Shift+right-click requests extended verbs;
   Shift+F10 opens the menu from the keyboard.
7. Cancel menus with Escape and an outside click, then reopen them. Check for
   freezes, disappearing selection, broken submenus or a window that stops
   responding. Try Refresh after commands that alter the source files.

Use the disposable samples for commands that rename, delete or move files.
Ctrl+A marks all rows; Space toggles the current mark; Escape clears marks.
Clicking or dragging a marked row retains the marked batch; an unmarked row
becomes the sole subject. Up/Down changes the current row; Ctrl+R refreshes.

The menu is the classic shell menu, corresponding to “Show more options” in
Windows 11, rather than Explorer's compact menu. Available entries depend on
installed applications and shell extensions. This demo accepts no inbound drops.

`session.log` records requests, native OS thread IDs, errors, drag effects,
menu command counts, interface versions and forwarded menu messages. API
success is not proof that a destination read the files or a command completed;
inspect the desktop result as well. Effect `0x1` is copy, `0x2` is move and
`0x0` is no transfer. Shell extensions may finish asynchronously; Refresh
checks the source listing again.

## Development

The reusable native boundary is `internal/windowshell`; the demo file list is
`internal/shellprobe`. Native shell operations execute through `Window.Run` on
Gio's owning window thread. A separate caller keeps Gio's event consumer running
during the shell's nested menu and drag loops. OLE stays initialized until native
window destruction, with a subclass forwarding shell menu messages.

Run `op check` for portable behavior and Gio event-routing tests. On Windows,
`go test ./internal/windowshell` also checks shell objects for single and
multiple Unicode filenames. Desktop interoperability still requires the checks
above.

For a Linux headless layout preview:

```sh
EGL_PLATFORM=surfaceless GOFLAGS='-tags=novulkan,nox11' \
  go run ./tools/shell-probe --png tmp/agent/screenshots/shell-probe.png
```
