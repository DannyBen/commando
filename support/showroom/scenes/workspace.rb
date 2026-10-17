# frozen_string_literal: true

# Paths and initial panels come from config/. Each theme starts afresh.
screenshot '01-workspace'

press 'Down', times: 2
press 'Alt+Left'
wait 400
screenshot '02-three-panels'

press 'Ctrl+Shift+P'
type 'panel'
wait 400
screenshot '03-command-palette'
