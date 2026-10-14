# Settings

`settings.toml` contains embedded defaults. `settings-ui.toml` describes matching
value paths, including labels, descriptions, palette groups, controls and numeric
constraints. Both files are parsed and validated once when the package starts.

Value types come from defaults. Controls are independent: `input`, `toggle`,
`stepper`, `stepper-progress`, `folder` or `file`. Without an explicit control,
booleans use toggles and other values use inputs. Steppers require a positive
step; progress steppers also require a finite minimum and maximum.

Parent sections in the UI file label palette groups. Leaf sections describe
settings controls; each field belongs to its immediate parent group. Default-value
order determines group and field order.
The application binds setting paths to their effects in Go.

Overrides form a nested value tree. Save uses the TOML library to write only
explicit overrides to `settings.toml` in the shared data folder. Reset removes
an override; cancellation restores the previous tree. Previews bypass storage.
