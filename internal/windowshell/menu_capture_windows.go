package windowshell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	getMenuString = user32.NewProc("GetMenuStringW")
	assocQuery    = windows.NewLazySystemDLL("shlwapi.dll").NewProc("AssocQueryStringW")
)

func captureMenuCommand(menu *object, hm, command uintptr, paths []string) (*CapturedCommand, error) {
	unsupported := fmt.Errorf("Cannot import this action")
	if len(paths) != 1 {
		return nil, unsupported
	}
	var verb [4096]uint16
	if hr := menu.call(5, command-1, 4, 0, uintptr(unsafe.Pointer(&verb[0])), uintptr(len(verb))); hr != 0 {
		return nil, unsupported
	}
	name := windows.UTF16ToString(verb[:])
	if name == "" || strings.ContainsAny(name, `\/`) {
		return nil, unsupported
	}
	var label [4096]uint16
	getMenuString.Call(hm, command, uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)), 0) // MF_BYCOMMAND includes submenus.
	title := cleanMenuLabel(windows.UTF16ToString(label[:]))
	if title == "" {
		return nil, unsupported
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		return nil, err
	}
	var classes []string
	if info.IsDir() {
		classes = []string{"Directory", "Folder"}
	} else {
		ext := strings.ToLower(filepath.Ext(paths[0]))
		if ext != "" {
			if progID := associationClass(ext); progID != "" {
				classes = append(classes, progID)
			}
			classes = append(classes, ext, `SystemFileAssociations\`+ext)
		}
		classes = append(classes, "*")
	}
	classes = append(classes, "AllFilesystemObjects")
	captured := &CapturedCommand{Name: title, Directory: info.IsDir(), Extensions: []string{"*"}}
	if !info.IsDir() && filepath.Ext(paths[0]) != "" {
		captured.Extensions = []string{strings.TrimPrefix(strings.ToLower(filepath.Ext(paths[0])), ".")}
	}
	for _, class := range classes {
		line, found, err := registeredMenuCommand(class, name, title)
		if err != nil {
			return nil, unsupported
		}
		if !found {
			continue
		}
		if captured.Command != "" && captured.Command != line {
			return nil, unsupported // Conflicting registrations cannot identify this handler reliably.
		}
		captured.Command = line
		if class == "*" || class == "AllFilesystemObjects" {
			captured.Extensions = []string{"*"}
		}
	}
	if captured.Command == "" {
		return nil, unsupported
	}
	return captured, nil
}

func cleanMenuLabel(label string) string {
	label, _, _ = strings.Cut(label, "\t")
	label = strings.NewReplacer("&&", "\x00", "&", "").Replace(label)
	return strings.TrimSpace(strings.ReplaceAll(label, "\x00", "&"))
}

// Query the effective association, including a user's chosen ProgID. NOFIXUPS
// keeps lookup observational; no associations or registrations are changed.
func associationClass(extension string) string {
	ext, _ := windows.UTF16PtrFromString(extension)
	var buffer [4096]uint16
	size := uint32(len(buffer))
	hr, _, _ := assocQuery.Call(0x120, 20, uintptr(unsafe.Pointer(ext)), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)))
	if uint32(hr) != 0 {
		return ""
	}
	return windows.UTF16ToString(buffer[:])
}

// Resolve only an exact verb key. COM and DDE registrations can override a
// visible command string, so they must not be flattened into a program action.
func registeredMenuCommand(class, verb, title string) (string, bool, error) {
	if strings.EqualFold(verb, "runas") || strings.EqualFold(verb, "runasuser") {
		return "", false, fmt.Errorf("Shell handler required")
	}
	base := class + `\shell\` + verb
	key, err := registry.OpenKey(registry.CLASSES_ROOT, base, registry.QUERY_VALUE)
	if err != nil {
		return "", false, nil
	}
	defer key.Close()
	for _, value := range []string{"ExplorerCommandHandler", "HasLUAShield"} {
		if _, _, err := key.GetValue(value, nil); err == nil {
			return "", false, fmt.Errorf("Shell handler required")
		}
	}
	label, _, _ := key.GetStringValue("MUIVerb")
	if label == "" {
		label, _, _ = key.GetStringValue("")
	}
	if label != "" && !strings.EqualFold(cleanMenuLabel(label), title) {
		return "", false, fmt.Errorf("Menu label does not match")
	}
	for _, subkey := range []string{`\DropTarget`, `\ddeexec`} {
		if child, err := registry.OpenKey(registry.CLASSES_ROOT, base+subkey, registry.QUERY_VALUE); err == nil {
			child.Close()
			return "", false, fmt.Errorf("Shell handler required")
		}
	}
	command, err := registry.OpenKey(registry.CLASSES_ROOT, base+`\command`, registry.QUERY_VALUE)
	if err != nil {
		return "", false, nil
	}
	defer command.Close()
	if _, _, err := command.GetValue("DelegateExecute", nil); err == nil {
		return "", false, fmt.Errorf("Shell handler required")
	}
	line, kind, err := command.GetStringValue("")
	if err == nil && kind == registry.EXPAND_SZ {
		line, err = registry.ExpandString(line)
	}
	return line, line != "" && err == nil, err
}
