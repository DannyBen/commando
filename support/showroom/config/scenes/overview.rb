# frozen_string_literal: true

# A working three-panel view for the README header, with independent current
# rows and a marked set of palette, font and icon assets in the active panel.
size width: 960, height: 378

press 'Esc'
press 'Ctrl+Shift+M'
press 'Home'
press 'Down', times: 8

press 'Right'
press 'Ctrl+Shift+M'
press 'Home'
press 'Down', times: 6
press 'Space'
press 'Down', times: 3
press 'Space'
press 'Down'
press 'Space'

press 'Right'
press 'Home'
press 'Down', times: 7
press 'Left'
press 'Up', times: 3
wait 400
screenshot 'overview'
