Changelog
========================================

v0.3.0 - 2026-10-16
----------------------------------------

- Add preliminary settings model
- Reafacor settings as TOML
- Organize settings
- Allow choosing data location
- Expose settings in command palette
- File editors and edit command
- Allow disabling auto color
- Sound settings (on/off)
- Notification animation settings (on/off)
- Fix regression: Error on invalid Alt+D folder
- Avoid saving default settings in user file
- Folder .lnk files open in app
- Fix filtering commit sound
- Accept environment variables in location bar
- Additional autocolor settings
- Font settings
- Update gio to fix fallback glyphs
- Autocomplete font family
- Switch to Victor Mono as default font
- Fix Ctrl+Backspace in input fields
- Ctrl+Shift+C to copy paths
- Shift+Insert to paste
- Change autocolor from hue rotation to salted random
- Normalize padding values
- Settings > Appearance > Padding
- Change default padding
- Application > Open data folder
- Refactor command dispatch and model dual key bindings
- Allow overriding key bindings
- Ctrl+E to edit command palette shortcut
- Allow shortcut config for special keys
- Allow adding shortcuts to settings widgets
- Fix reset button sound
- Allow adding shortcuts to program rules
- Refactor sound map internals
- Alt+D accepts filenames
- Add theme settings
- Update color controls
- Add swatch to Hue control
- Remove selected file edge color
- Refactor theme editor
- Ctrl+Left/Right in stepper fields for larger steps
- Notify on opening Data Folder
- Add theme generator
- Implement APCA contrast for selected text
- Refactor theme generator controls
- Live theme generator preview
- Update theme generator values
- Refactor APCA contrast guard
- Show file selection in theme preview
- Allow different APCA target per UI component
- Implement APCA guard for text selection
- Support PgUp/PgDn/Home/End in input steppers
- Fix No Command Found padding


v0.2.0 - 2026-10-13
----------------------------------------

- Add command palette
- Add message sounds
- Update message sounds
- Fix footer info on dir navigation
- Add ESC sound
- Handle gone dirs
- Fix premature filename ellipsis
- Detach file extension when extension meta is active
- Add formula based color per extension
- Ctrl+F3 to sort by metadata column
- Allow adding/removing panels
- Mention panel focus commands in palette
- Prevent stale selection highlight during adjacent folder loading
- Narrower command palette
- Use slash to filter
- Force single instance
- Add Windows context menu
- Add drag outside and arm-drag
- Ignore in-app arm-drag drops
- Pasting of image data stream creates image file


v0.1.0 - 2026-10-12
----------------------------------------

- Initial list interaction
- Fix Alt+D bell
- Show drive list
- Update theme
- Do not require colon when changing drives
- Ctrl+Right/Left opens dir in adjacent panel
- Tab/Shift+Tab move between panels
- Rename to commando
- Allow selecting multiple files
- Remove trailing slash from dirs
- Allow choosing meta column
- Add F2 to rename
- Implement footer message channels
- Remember meta column state
- Ctrl+R to refresh panel
- Ctrl+N/Ctrl+Shift+N to create new file/folder
- Del recycles
- Implement Ctrl+C/V/X
- Implement Overwrite/Cancel interaction model
- Implement bulk copy/move
- Cancel panel operation on defocus
- Ctrl+D to duplicate files
- Fix sort to exclude extension
- Consolidate file transfers and harden move safety
- Use native Windows operations for file ops
- F5/F6 to copy/move to panel
- Shift+Right/Left to move panels
- Ctrl+H to cycle through 3 hidden visibility modes
- Centralize foreground interaction state and lifecycle
- Auto refresh panels on change
- Update vulnerable deps
- Prepare initial public release
- Fix initial cross-platform CI checks
- Fix Windows validation and simplify public repository


