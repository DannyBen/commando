// Package diagnostics provides opt-in production tracing. Group bits are stable
// identifiers, not severity levels. Diagnostics never change operation outcomes.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

type Mask uint32

const (
	FileOperations Mask = 1 << iota
	UI
	Shell
	Browsing
	Storage
	Lifecycle
	Feedback
	All = FileOperations | UI | Shell | Browsing | Storage | Lifecycle | Feedback
)

func (m Mask) Label() string {
	switch m {
	case 0:
		return "OFF"
	case FileOperations:
		return "File operations"
	case UI:
		return "UI"
	case Shell:
		return "Shell"
	case Browsing:
		return "Browsing"
	case Storage:
		return "Storage"
	case Lifecycle:
		return "Lifecycle"
	case Feedback:
		return "Feedback"
	case All:
		return "ALL"
	}
	return ""
}

func (m Mask) String() string {
	if label := m.Label(); label != "" {
		return fmt.Sprintf("%d — %s", m, label)
	}
	return fmt.Sprint(uint32(m))
}

var enabled atomic.Uint32
var ids atomic.Uint64
var log struct {
	sync.Mutex
	file               *os.File
	directory, session string
	sequence           uint64
}

func Enabled(group Mask) bool { return enabled.Load()&uint32(group) != 0 }
func NextID() uint64          { return ids.Add(1) }

// Configure opens the fixed log lazily, and closes it when tracing is disabled.
// A failed reconfiguration leaves the previous configuration intact.
func Configure(directory string, mask Mask) error {
	if mask & ^All != 0 {
		return fmt.Errorf("trace mask must be 0 to %d", All)
	}
	log.Lock()
	defer log.Unlock()
	if mask == 0 {
		enabled.Store(0)
		if log.file != nil {
			writeLocked(0, 0, "SessionEnded", nil)
			err := log.file.Close()
			log.file = nil
			return err
		}
		return nil
	}
	if log.file == nil || directory != log.directory {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(filepath.Join(directory, "trace-log.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if log.file != nil {
			writeLocked(0, 0, "SessionEnded", nil)
			log.file.Close()
		}
		log.file, log.directory = f, directory
		log.session = fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
		log.sequence = 0
		executable, _ := os.Executable()
		build, _ := debug.ReadBuildInfo()
		writeLocked(0, 0, "SessionStarted", map[string]any{"schema": 1, "pid": os.Getpid(), "go": runtime.Version(), "mask": mask, "executable": executable, "build": build})
	}
	if enabled.Load() != uint32(mask) {
		writeLocked(0, 0, "MaskChanged", mask)
	}
	enabled.Store(uint32(mask))
	return nil
}

func Close() error { return Configure("", 0) }

func Record(group Mask, operation uint64, event string, data any) {
	if !Enabled(group) {
		return
	}
	log.Lock()
	defer log.Unlock()
	if log.file != nil && Enabled(group) {
		writeLocked(group, operation, event, data)
	}
}

func writeLocked(group Mask, operation uint64, event string, data any) {
	log.sequence++
	groupLabel := ""
	if group != 0 {
		groupLabel = group.Label()
	}
	// Write failures must not affect shell callbacks or application decisions.
	_ = json.NewEncoder(log.file).Encode(struct {
		Time      time.Time `json:"time"`
		Session   string    `json:"session"`
		Sequence  uint64    `json:"sequence"`
		Group     string    `json:"group,omitempty"`
		Operation uint64    `json:"operation,omitempty"`
		Event     string    `json:"event"`
		Data      any       `json:"data,omitempty"`
	}{time.Now().UTC(), log.session, log.sequence, groupLabel, operation, event, data})
}

func Error(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
