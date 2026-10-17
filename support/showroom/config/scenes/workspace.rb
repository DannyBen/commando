# frozen_string_literal: true

# Paths and initial panels come from showroom.yml. Each theme starts afresh.
screenshot '01-workspace'

press 'Right'
press 'Ctrl+W'
wait 400
screenshot '02-two-panels'

press 'Ctrl+Shift+W'
wait 400
screenshot '03-three-panels'

press 'Ctrl+Shift+P'
type 'panel'
wait 400
screenshot '04-command-palette'
