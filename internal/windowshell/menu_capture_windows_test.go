package windowshell

import (
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestRegisteredMenuCommandRequiresAnExactExecutableRegistration(t *testing.T) {
	for _, kind := range []string{"command", "wrong label", "delegate", "drop target", "DDE", "explorer handler", "elevation"} {
		t.Run(kind, func(t *testing.T) {
			class := fmt.Sprintf("CommandoCaptureTest.%d.%d", os.Getpid(), time.Now().UnixNano())
			base := `Software\Classes\` + class
			verbPath := base + `\shell\capture`
			verb, _, err := registry.CreateKey(registry.CURRENT_USER, verbPath, registry.ALL_ACCESS)
			if err != nil {
				t.Fatal(err)
			}
			defer verb.Close()
			t.Cleanup(func() {
				for _, child := range []string{`\shell\capture\command`, `\shell\capture\DropTarget`, `\shell\capture\ddeexec`, `\shell\capture`, `\shell`, ""} {
					if err := registry.DeleteKey(registry.CURRENT_USER, base+child); err != nil && err != registry.ErrNotExist {
						t.Error("remove registry fixture", err)
					}
				}
			})
			label := "&Capture action"
			if kind == "wrong label" {
				label = "Another action"
			}
			if err := verb.SetStringValue("", label); err != nil {
				t.Fatal(err)
			}
			command, _, err := registry.CreateKey(registry.CURRENT_USER, verbPath+`\command`, registry.ALL_ACCESS)
			if err != nil {
				t.Fatal(err)
			}
			defer command.Close()
			const line = `"C:\path with spaces\tool.exe" "%1"`
			if err := command.SetStringValue("", line); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "delegate":
				err = command.SetStringValue("DelegateExecute", "{test}")
			case "drop target", "DDE":
				child := `\DropTarget`
				if kind == "DDE" {
					child = `\ddeexec`
				}
				var key registry.Key
				key, _, err = registry.CreateKey(registry.CURRENT_USER, verbPath+child, registry.ALL_ACCESS)
				if err == nil {
					key.Close()
				}
			case "explorer handler":
				err = verb.SetStringValue("ExplorerCommandHandler", "{test}")
			case "elevation":
				err = verb.SetStringValue("HasLUAShield", "")
			}
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := registeredMenuCommand(class, "capture", "Capture action")
			if kind == "command" {
				if err != nil || !found || got != line {
					t.Fatal("lost direct command", got, found, err)
				}
			} else if err == nil || found {
				t.Fatal("flattened incompatible registration", got, found, err)
			}
		})
	}
}

func TestCaptureRejectsBackgroundAndBatchMenusBeforeShowingThem(t *testing.T) {
	host := &Host{}
	for _, options := range []MenuOptions{{Capture: true}, {Capture: true, Background: true}} {
		if result, err := host.ContextMenu(nil, options); err == nil || result.Captured != nil {
			t.Fatal("capture accepted a non-item menu")
		}
	}
}
