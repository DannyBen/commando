# Settings

`settings.toml` contains embedded defaults. `settings-ui.toml` describes matching
value paths, including labels, descriptions, palette groups, controls and numeric
constraints. Both files are parsed and validated once when the package starts.

Value types come from defaults. Controls are independent: `input`, `toggle`,
`stepper`, `stepper-progress`, `folder` or `file`. Without an explicit control,
booleans use toggles and other values use inputs. Steppers require a positive
step; progress steppers also require a finite minimum and maximum.

Parent labels name palette categories. A `title` declares a widget on a group
or a single setting and supplies its heading. `palette` can give that widget a
different menu name; leaf `label` names its option. Group widgets collect their
descendant fields, and nested widget declarations are rejected. The settings
palette sorts by the full displayed label; metadata order determines field order.

Labels and titles use sentence case. Widget headings include their category path.

`visible = false` hides a node and its descendants from settings UI while keeping
their values, validation and persistence. The application binds paths to effects
in Go.

Overrides form a nested value tree. Save uses the TOML library to write only
explicit overrides to `settings.toml` in the shared data folder. Reset removes
an override; cancellation restores the previous tree. Previews bypass storage.

Startup reads and validates user overrides before applying them. Missing files
use defaults. Syntax errors, unknown paths, incorrect types and out-of-range
values reject the entire file and use defaults, leaving the file untouched.
The contextual footer reports the failure; stderr includes diagnostic details.
