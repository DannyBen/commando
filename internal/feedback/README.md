# Feedback

Each context owns a `Channel`, which holds one message. Its zero value is ready
to use. Calls belong on the UI thread; worker results are delivered there first.

```go
channel.Show(feedback.Error, "Name already exists")
channel.Show(feedback.Success, "Renamed")
channel.Progress("Reading folder")
channel.ProgressAt("Reading folder", now.Add(delay))
channel.Complete()
channel.Dismiss()
```

`Show(kind, text)` replaces the current message and restarts its animation.
Kinds are `Info`, `Success`, `Warning` and `Error`. `Progress(text)` shows an info
message that also clears on `Complete()`. Completion leaves a result message
alone. `Dismiss()` clears any message; nothing expires on a timer.

`ShowAt(kind, text, notBefore)` schedules a replacement message. Until its
deadline, `Current()` is nil and the channel leaves normal status visible.
`ProgressAt(text, notBefore)` schedules Info feedback conditional on an operation
remaining pending: `Complete()` cancels it before display or clears it afterward.
Dismissal and any replacement also cancel scheduled messages. Animation starts
when the message becomes visible, even if the deadline's frame arrives late.

`Frame(now, animations)` returns a frame and whether the channel owns its
display area. A frame contains visible `Text`, semantic `Kind`, the `Next`
redraw deadline, and a `Started` event on its first visible frame. Schedule `Next`
even when the channel does not own its area; it can be a pending-message deadline.
An empty typing/blink frame still owns the entire area. `Next` is zero when
presentation is steady;
a persistent blinking cursor continues scheduling redraws until dismissal.
`Current()` exposes the complete message for inspection; treat it as read-only.
`Acknowledge()` suppresses a visible message's `Started` notification when its
caller has already announced it; animation and dismissal stay unchanged.

`Animations` holds a master `Enabled` switch and separate profiles for the four
kinds. Typing uses a fixed `CharacterDelay` per Unicode character. `BlinkInterval`
and `Blinks` control binary text pulses; zero blinks disables them. `Cursor`,
`CursorBlinkInterval` and `CursorBlinks` control the typing cursor independently.
A negative cursor blink count keeps blinking until dismissal. Zero character
delay skips typing; disabling animation shows steady text immediately. A
different animation can be selected by kind without changing operation code.

The app chooses the context, message wording, colors and footer layout, schedules
redraws, and calls `Dismiss()` on user input without consuming that input. A new
message replaces the previous one; there is no queue. Normal status and counts
resume when the channel no longer owns the footer.
