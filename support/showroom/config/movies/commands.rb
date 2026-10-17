# frozen_string_literal: true

shot '01-catalogue', duration: 1.2
16.times do |index|
  shot format('browse-%<index>02d', index: index + 1), duration: 0.3
end
shot '02-search-re', duration: 0.8
shot '03-search-rename', duration: 1.5
shot '04-key-bindings', duration: 2.5
shot '05-execute-rename', duration: 2.0
