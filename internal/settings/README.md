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
Widgets share a label column sized from their widest label, with the remaining
width available to values and a gap with an extra text space. Narrow widgets
reserve at least a third for values.

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

Enabled toggles lead their widget. Unfocused toggles underline the active value
and lightly dim the inactive value. Focused toggles use a background on the active
value, with no underline or dimming.

Appearance › Auto color includes an Enabled toggle. Turning it off uses semantic
theme colors while retaining the generated-color parameters for re-enabling.

## Editor rules

`Programs › Editors › New` creates a rule. Saved rules appear by their space-separated extensions
and have Save, Cancel and Delete actions. Delete stages removal until Save;
Cancel restores the rule. Program selection reuses browsing panels: Enter on a
file chooses it and returns to the widget; Enter on a folder opens it. Moving
the highlight or returning with Escape leaves the Program value unchanged.

Enter space-separated extensions in the widget, optionally prefixed with a dot.
The entered extension spelling and order are retained. Matching ignores case
and optional leading dots, and uses the final extension. `*` alone defines a fallback;
explicit matches take precedence. When rules overlap, the first saved rule wins.
Files without an extension use the fallback.

```toml
[[programs.editors]]
ext = ["txt", "md"]
path = 'C:\Program Files\Editor\editor.exe'
args = '--wait %SelectedFiles%'
```

F4 or File › Edit uses marked files, otherwise the current file. Folders are
excluded. Files are grouped by matching rule, with one executable invocation per
rule. Arguments are split using double quotes and Windows backslash-before-quote
rules, without a shell. `%SelectedFiles%` must occur once as a whole argument;
it expands to separate file arguments after parsing. The working folder is the
source panel's folder. Missing rules and launch failures use panel feedback.

Embedded array tables supply one item template for a collection, with defaults
and field metadata handled by the same catalogue as scalar widgets. They do not
install effective rules. User array tables store the collection; empty lists
preserve deletion of all rules. Collection drafts share the settings transaction,
file picker and action bar, with no Reset action.

Escape leaves input editing or selection first. Once any widget change occurs,
it requires Save or Cancel to close, even if an edit is later reverted. Escape
closes an unchanged widget. On a dirty widget, Escape plays the warning cue and
focuses Cancel; Enter then explicitly discards the transaction.
