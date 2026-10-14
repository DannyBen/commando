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
different menu name; leaf `label` names its option. Fields belong to their nearest
titled ancestor. Parent widgets hold basic controls; titled children hold more
specific controls. Aim for four or five fields per widget. The settings
palette sorts by the full displayed label; metadata order determines field order.

Labels and titles use sentence case. Widget headings include their category path.
Widgets share a label column sized from their widest label, with the remaining
width available to values and a gap with an extra text space. Narrow widgets
reserve at least a third for values.

`visible = false` hides a node and its descendants from settings UI while keeping
their values, validation and persistence. The application binds paths to effects
in Go.

Overrides form a nested value tree. Save filters a copy against built-in defaults
and uses the TOML library to rewrite `settings.toml` with only differing values.
Empty tables and empty collections are omitted. Each collection item is filtered
against its template; item order and membership remain intact, including items
whose fields all match defaults. Loading supplies omitted fields from defaults.
Reset removes an override; cancellation restores the previous transaction tree.
Previews bypass storage.

Startup reads and validates user overrides before applying them. Missing files
use defaults. Syntax errors, unknown paths, incorrect types and out-of-range
values reject the entire file and use defaults, leaving the file untouched.
The contextual footer reports the failure; stderr includes diagnostic details.

Enabled toggles lead their widget. Unfocused toggles underline the active value
and lightly dim the inactive value. Focused toggles use a background on the active
value, with no underline or dimming.

Appearance › Auto color includes an Enabled toggle. Turning it off uses semantic
theme colors while retaining the generated-color parameters for re-enabling.
Its Selection widget adjusts background lightness relative to file lightness.
Special files adjusts lightness offsets and chroma multipliers for hidden
attributes and dot-prefixed names; both modifiers apply when an entry has both.
Each widget resets only its own fields.

Appearance › Font provides a text field for an installed font family, two text
sizes and an application-wide weight (400 regular, 700 bold). Changes preview
live; Save persists them, Cancel restores the previous values, and Reset restores
the bundled family and default sizes/weight. Unavailable or blank families use
the bundled font. Weight uses the closest available face; the bundled font
currently includes only regular. Row padding is independent of font settings.

Feedback › Sounds includes an Enabled toggle, on by default. It applies live to
all application cues and stops queued/current audio when disabled. Individual cue
volumes stay unchanged. Save, Cancel and Reset use the shared settings transaction.

Feedback › Notifications includes an Animation toggle, on by default. Turning it
off shows complete, steady messages without a typing or blinking cursor. Semantic
sounds, delayed progress, message replacement and dismissal retain their behavior.

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
install effective rules. User array tables store the collection; missing or empty collections contain
no items. Collection drafts share the settings transaction,
file picker and action bar, with no Reset action.

Escape leaves input editing or selection first. Once any widget change occurs,
it requires Save or Cancel to close, even if an edit is later reverted. Escape
closes an unchanged widget. On a dirty widget, Escape plays the warning cue and
focuses Cancel; Enter then explicitly discards the transaction.
