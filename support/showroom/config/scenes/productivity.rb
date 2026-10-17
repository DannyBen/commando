# frozen_string_literal: true

# Make it muscle memory: narrow the list, mark a batch, edit a name in place.
press '/'
type '.'
wait 400
screenshot '01-filtered-files'

press 'Enter'
press 'Home'
press 'Down'
press 'Insert', times: 3
wait 400
screenshot '02-marked-batch'

# Keep the draft visible without submitting a filesystem mutation.
press 'F2'
type 'overview'
wait 400
screenshot '03-inline-rename'
