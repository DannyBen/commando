# Feedback

Each context owns a `Channel`, which holds one message. Its zero value is ready
to use. Calls belong on the UI thread; worker results are delivered there first.

```go
channel.Show(feedback.Error, "Name already exists")
channel.Show(feedback.Success, "Renamed")
channel.Progress("Reading folder")
channel.Complete()
channel.Dismiss()
```

`Show(kind, text)` replaces the current message and restarts its animation.
Kinds are `Info`, `Success`, `Warning` and `Error`. `Progress(text)` shows an info
message that also clears on `Complete()`. Completion leaves a result message
alone. `Dismiss()` clears any message; nothing expires on a timer.

`Frame(now, animations)` returns a frame and whether the channel owns its
display area. A frame contains visible `Text`, semantic `Kind`, and the `Next`
redraw deadline. An empty typing/blink frame still owns the entire area.
Once animation finishes, `Next` is zero and the complete message stays visible.
`Current()` exposes the complete message for inspection; treat it as read-only.

`Animations` holds a master `Enabled` switch and separate profiles for the four
kinds. Each profile has `Enabled`, `CharacterDelay`, `BlinkInterval` and `Blinks`.
Typing uses a fixed interval per Unicode character. Blinks switch completely
off/on. Zero character delay skips typing; zero blinks skips blinking; disabling
animation shows steady text immediately. A different animation can be selected
by kind without changing operation code.

The app chooses the context, message wording, colors and footer layout, schedules
redraws, and calls `Dismiss()` on user input without consuming that input. A new
message replaces the previous one; there is no queue. Normal status and counts
resume when the channel no longer owns the footer.
