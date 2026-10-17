package app

import (
	"errors"
	"reflect"
	"testing"

	"gioui.org/io/key"
)

func TestPropertiesUsesSelectionAndSharedNativeSession(t *testing.T) {
	for _, marked := range []bool{false, true} {
		for _, palette := range []bool{false, true} {
			v := menuTestView()
			p := &v.panels[v.active]
			if !marked {
				p.clearMarks()
				p.selected = 1
			}
			want := p.selectedPaths()
			send, _ := panelInputFrames(v)
			if palette {
				send(pressed("`"))
				send(replacePalette(v, "Entry Properties"), pressed(key.NameReturn))
			} else {
				send(pressed(key.NameF12))
			}
			s, ok := v.interaction.current.(*nativeSession)
			if !ok || s.request.action != shellProperties || !reflect.DeepEqual(s.request.paths, want) || s.request.options.ClientPoint == nil {
				t.Fatalf("marked=%v palette=%v: wrong properties session: %+v", marked, palette, s)
			}
			if v.requestClose() || v.actionAllowed(actionMutation) {
				t.Fatal("properties bypassed the native busy policy")
			}
			p.clearMarks()
			if !reflect.DeepEqual(s.request.paths, want) {
				t.Fatal("selection changes altered captured paths")
			}
			anchor := p.selectionAnchor
			v.shellExecutor = func(request shellRequest) shellCompletion {
				if request.options.ClientPoint == nil || *request.options.ClientPoint != anchor {
					t.Error("properties did not use the revealed current-row anchor")
				}
				return shellCompletion{}
			}
			v.dispatchNative(func() {})
			result := <-v.shellResults
			result.err = errors.New("native failure")
			v.completeNative(result)
			if !v.interaction.idle() || p.feedback.Current() == nil || p.feedback.Current().Text != "Cannot open properties" {
				t.Fatal("properties failure did not release session with specific feedback")
			}
		}
	}
}

func TestPropertiesIgnoresUnavailableTargets(t *testing.T) {
	for _, mode := range []string{"parent", "drives", "loading", "busy", "editor", "unavailable shell"} {
		t.Run(mode, func(t *testing.T) {
			v := menuTestView()
			p := &v.panels[0]
			p.clearMarks()
			p.selected = 1
			send, _ := panelInputFrames(v)
			switch mode {
			case "parent":
				p.selected = 0
			case "drives":
				p.drives = true
			case "loading":
				p.loading = true
			case "busy":
				v.transferPending = true
			case "editor":
				send(modified("D", key.ModAlt))
			case "unavailable shell":
				v.shellExecutor = nil
			}
			send(pressed(key.NameF12))
			if _, ok := v.interaction.current.(*nativeSession); ok {
				t.Fatal("unavailable properties started a session")
			}
		})
	}
}
