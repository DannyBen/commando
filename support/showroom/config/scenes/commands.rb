# frozen_string_literal: true

density 2

# A working three-panel backdrop, with the current file away from the edges.
press 'Esc'
press 'F1'
press 'Ctrl+Shift+M'
press 'Home'
press 'Down', times: 8
press 'Right'
press 'Ctrl+Shift+M'
press 'Home'
press 'Down', times: 7

# Tour the catalogue quickly enough to convey its breadth.
press 'Ctrl+Shift+P'
wait 400
screenshot '01-catalogue'
16.times do |index|
  press 'Down', times: 3
  wait 100
  screenshot format('browse-%<index>02d', index: index + 1)
end

type 're'
wait 400
screenshot '02-search-re'
type 'name'
wait 400
screenshot '03-search-rename'
press 'Ctrl+E'
wait 400
screenshot '04-key-bindings'

# Return to the result and execute it through the same palette.
press 'Esc'
press 'Enter'
wait 400
screenshot '05-execute-rename'
