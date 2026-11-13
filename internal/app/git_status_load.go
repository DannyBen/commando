package app

import (
	"context"
	"errors"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/feedback"
)

const gitRefreshInterval = time.Second

// This identity follows its panel through moves. Only the UI uses submitted;
// the worker reads the immutable path, options and cancellation context.
type gitRead struct {
	path      string
	options   gitOptions
	ctx       context.Context
	cancel    context.CancelFunc
	submitted bool
	automatic bool
}

func (p *panel) cancelGitRead() {
	if p.gitRead != nil {
		p.gitRead.cancel()
		p.gitRead = nil
	}
}

func (v *view) prepareGitRead(p *panel, options gitOptions) {
	p.cancelGitRead()
	p.gitOptions = options
	if v.loader == nil || v.loader.ctx == nil || options != v.gitOptions() || !options.enabled || !p.nativeFolder() {
		return
	}
	ctx, cancel := context.WithCancel(v.loader.ctx)
	p.gitRead = &gitRead{ctx: ctx, cancel: cancel, path: p.path, options: options}
}

// Submit after drawing the directory, behind ordinary worker requests. A full
// background queue leaves the read pending for a later frame.
func (v *view) submitGitReads(gtx layout.Context) {
	if v.loader == nil || !v.actionAllowed(actionMutation) || len(v.loader.requests) != 0 {
		return
	}
	for i := range v.panels {
		p := &v.panels[i]
		read := p.gitRead
		if read == nil || read.submitted || p.loading || p.refreshing || p.refreshNeeded {
			continue
		}
		if due := p.gitReadAt.Add(gitRefreshInterval); read.automatic && gtx.Now.Before(due) {
			gtx.Execute(op.InvalidateCmd{At: due})
			continue
		}
		select {
		case <-v.loader.done:
			return
		case v.loader.gitReads <- read:
			read.submitted = true
			p.gitReadAt = gtx.Now
		default:
		}
		return
	}
}

func (l *directoryLoader) runGitRead(read *gitRead) {
	ctx, cancel := context.WithCancel(read.ctx)
	l.gitMu.Lock()
	if len(l.requests) != 0 {
		cancel()
	} else {
		l.gitCancel = cancel
	}
	l.gitMu.Unlock()
	result := loadResult{request: loadRequest{gitRead: read, action: actionBrowse}}
	started := time.Now()
	if ctx.Err() == nil {
		result.gitStatus, result.gitErr = l.readGitStatus(ctx, read.path, read.options.executable)
	}
	if ctx.Err() != nil {
		result.gitStatus, result.gitErr = nil, ctx.Err()
	}
	cancel()
	l.gitMu.Lock()
	l.gitCancel = nil
	l.gitMu.Unlock()
	diagnostics.Record(diagnostics.Browsing, diagnostics.NextID(), "GitStatusCompleted", map[string]any{
		"path": read.path, "error": diagnostics.Error(result.gitErr), "duration_ms": time.Since(started).Seconds() * 1000,
	})
	select {
	case l.results <- result:
		if l.invalidate != nil {
			l.invalidate()
		}
	case <-l.done:
	}
}

func (v *view) acceptGitRead(result loadResult) {
	read := result.request.gitRead
	for i := range v.panels {
		p := &v.panels[i]
		if p.gitRead != read {
			continue
		}
		read.submitted = false
		if read.ctx.Err() != nil || read.options != v.gitOptions() || !p.nativeFolder() || !samePath(p.path, read.path) {
			p.cancelGitRead()
			return
		}
		if errors.Is(result.gitErr, context.Canceled) {
			return // Foreground work won; retry this snapshot when the worker is free.
		}
		p.cancelGitRead()
		if result.gitErr == nil {
			p.gitStatus = gitStatusNames(result.gitStatus)
		} else if !read.automatic && !errors.Is(result.gitErr, browser.ErrGitUnavailable) && !v.interaction.protectsListing(i) {
			v.showFeedback(i, feedback.Error, result.gitErr.Error())
		}
		return
	}
	read.cancel() // Its panel was closed or replaced while the read was pending.
}
