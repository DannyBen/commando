Changelog
========================================

v0.6.3 - 2026-10-20
----------------------------------------

- Multi rename with expressions
- Example inserter for multi-rename expression
- Update multi rename examples
- Fix footer premature ellipsis


v0.6.2 - 2026-10-20
----------------------------------------

- Right click on empty space open Windows menu
- Disable location bar scrolling when palette is open
- Show ellipsis on commands that need further input
- Up/Down selects the next match during type-to-select
- Add references to related @settings in tips
- Favorites
- Delete/rename/copy favorites
- Autocomplete favorites in location bar
- Remove meta column from home screen
- Add sound to panel resizing
- Fix default key binding overrides saved in user settings


v0.6.1 - 2026-10-20
----------------------------------------

- Help footer
- Add keyboard hints to some tips
- Add help for commands
- Add interaction tips


v0.6.0 - 2026-10-19
----------------------------------------

- +/- to resize panels
- Shift+Home/End to select files
- Allow viewing zip files in panels
- Organize and normalize Theme settings
- Deletable files in ZIP files
- Rename in ZIP
- Copy into ZIP
- Open files in ZIP
- Copy files from ZIP
- Duplicate files in ZIP
- Move files in/out of ZIP
- Improve temp file cleanup
- Copy path from inside ZIP
- Entry > Pack to ZIP
- Entry > Unpack ZIP
- Allow showing drive properties


v0.5.0 - 2026-10-18
----------------------------------------

- Notify on failed open
- Navigation > Drives
- Appearance > Icons
- Add icon recoloring setting
- Fix possible theme selector hang
- Apply color offsets to hidden and dot files
- Show icons in settings overlays and draft floats
- Preserve custom keybindings when new defaults conflict
- Feedback > Toggle sounds
- Add toggle commands for metadata and boolean settings
- Programs > Terminal here
- Application > Toggle full screen
- Entry > Properties
- Show confirmation modal for key reassignment
- Guards > Recycle/Delete confirmation
- Orange icon
- Fix deletion outcomes and footer notifications
- Debug > Trace mask
- Remember Current theme in Theme > Select
- Internal mouse control surface
- Add mouse over and click shading
- Fix key reassign button on Alt+Enter assignment
- Show native retry dialog on locked deletions
- Double click to open entries
- Clickable settings buttons
- Steppers respond to click and mouse wheel
- Double click a command palette action executes
- Click on empty panel area selects the panel
- Double click location bar to edit
- Add Empty folder marker to empty panels
- Icon for parent entry
- Entry > Paste shortcut


v0.4.0 - 2026-10-17
----------------------------------------

- Appearance > Theme > Select
- Auto select theme on clean startup
- Reduce theme generator autocolor influence
- Improve preview for autocolor settings
- Update APCA thresholds for naturally dimmed items
- Appearance > Contrast guard
- Appearance > Theme > Save
- View > Metadata columns
- Sound profiles
- Home/End to navigate autocompelte steppers
- Fix sound and visual palette opening issues
- Fix focus steal
- Allow bold weight for dirs
- Autocomplete location bar
- Use []/{} to copy/move entries left/right
- Update icon
- View > Parent entry
- Ctrl+Left/Right on a file clones the panel also
- Ctrl+N/Ctrl+Shift+N now allow deep creation


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


