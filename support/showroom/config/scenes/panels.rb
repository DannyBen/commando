# frozen_string_literal: true

# Multi-panel. Because two is never enough.
# Start with the familiar two-panel layout, then bring Downloads into the job.
press 'F1'
press 'Right'
press 'Ctrl+W'
wait 400
screenshot '01-two-panels'

# Show how the same workspace becomes a quieter, icon-free list.
press 'Ctrl+Shift+P'
type '@icons'
press 'Enter'
wait 400
screenshot 'icons-enabled'

press 'Right'
wait 400
screenshot 'icons-disabled'

press 'Ctrl+Enter'
wait 400
screenshot 'two-panels-clean'

press 'Ctrl+Shift+W'
wait 400
screenshot '02-three-panels'

# Narrow a mixed directory to media files and mark the resulting batch.
press '/'
type 'm'
wait 400
screenshot '03-filtering'

type 'p'
wait 400
screenshot '04-filtered-media'

press 'Enter'
press 'Home'
press 'Down'
press 'Insert', times: 2
wait 400
screenshot '05-marked-media'

# Briefly show a fourth panel, then return to the three-panel signature.
press 'Alt+Right'
wait 400
screenshot '06-four-panels'

press 'Ctrl+W'
press 'Left'
wait 400
screenshot '07-three-panels'
