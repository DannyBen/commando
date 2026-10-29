# frozen_string_literal: true

# Keep the same working view behind the generator throughout the sequence.
press 'Esc'
press 'F1'
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
press 'Up', times: 3

press 'Ctrl+Shift+P'
type '@generator'
press 'Enter'
press(theme == 'dark' ? 'Home' : 'End')
press(theme == 'dark' ? 'Right' : 'Left')
press 'Down'
press 'Enter'
type '42'
press 'Enter'
wait 400
screenshot '01-seed-42'

press 'Right'
wait 400
screenshot '02-seed-43'
press 'Right'
wait 400
screenshot '03-seed-44'

# Open up the palette, then try two more seeds with the same controls.
press 'Down'
press 'Enter'
type '75'
press 'Enter'
press 'Down'
press 'Enter'
type '70'
press 'Enter'
wait 400
screenshot '04-vivid-44'
press 'Up', times: 2
press 'Right'
wait 400
screenshot '05-vivid-45'
press 'Right'
wait 400
screenshot '06-vivid-46'

# Hold the seed fixed and rotate the fully saturated, dynamic palette.
press 'Down'
press 'End'
press 'Down'
press 'End'
press 'Down'
wait 400
screenshot '07-rotation-0'
[80, 160, 240].each_with_index do |rotation, index|
  press 'Ctrl+Right', times: 8
  wait 400
  screenshot format('%<index>02d-rotation-%<rotation>d', index: index + 8, rotation: rotation)
end
