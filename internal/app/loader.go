package app

import (
	"context"
	"errors"
	"github.com/dannyben/commando/internal/diagnostics"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dannyben/commando/internal/browser"
)

type loadRequest struct {
	panel              int
	path, selectName   string
	homeDirectory      string
	favoritesDirectory string // Read favorite aliases independently of filesystem completion.
	favoriteName       string // Exact alias fallback when Enter precedes completion results.
	favoriteRead       *favoriteSession
	favoriteWrite      *nameSession
	archiveFolder      string
	suggestedPath      string // Visible location completion captured at submission.
	successMessage     string
	refresh            bool
	follow             bool // Read exactly this folder; a missing counterpart stays put.
	autoRefresh        bool
	shortcut           bool // Resolve a .lnk before deciding between folder navigation and native opening.
	explicitPath       bool // Resolve pasted files and missing paths within their own ancestry.
	tracked            bool
	action             actionClass
	locations          []string // Captured panel paths for create/rename alias reconciliation.
	rename             *renameRequest
	multiRename        *multiRenameRequest
	create             *createRequest
	duplicate          *duplicateRequest
	removal            *removalRequest
	clipboard          *fileClipboardRequest
	transfer           *transferRequest
	settingsSave       *settingsSaveRequest
	terminal           *terminalRequest
	editor             *editorRequest
	archiveOpen        *archiveOpenRequest
	unpack             *unpackRequest
	fontFamilies       bool
	size               *sizeSession
	folderCacheLoad    bool
	driveSpace         *driveSpaceRequest
	completion         *locationSession
	fileDraft          *fileDraftRequest
	themeLibrary       *settingsSession
	icons              []fileIconKey
}

type loadResult struct {
	driveSpace               browser.DriveSpace
	driveSpaceKnown          bool
	folderSizes              folderSizeCache
	size                     browser.SizeResult
	archiveReconciled        bool
	renamed                  []renameChange
	archiveMoves             []archivePanelMove
	movedNames, movedSources []string
	retainedCopies           int
	progress                 string // Intermediate worker feedback; does not complete the request.
	request                  loadRequest
	directory                browser.Directory
	favorites                []browser.Entry
	created                  browser.Entry
	removed                  browser.RemovalResult
	sameDirectory            []bool
	err                      error
	clipboard                browser.FileClipboard
	transferred              browser.TransferResult
	locations                []string
	clipboardErr             error
	transferMove             bool
	duplicatePlan            *browser.DuplicatePlan
	alreadyHere              bool
	panelUpdates             []transferPanelUpdate
	mutationDirectory        string
	settingsMessage          string
	editorsStarted           int
	externalOpen             bool
	fontFamilies             []string
	pathMissing              bool
	themes                   []themeSnapshot
	icons                    []image.Image
}

type directoryLoader struct {
	readDriveSpace        func(string) (browser.DriveSpace, bool)
	folderCacheDir        string // Directory worker owns the persisted cache.
	folderSizes           folderSizeCache
	calculateSize         func(context.Context, []string) (browser.SizeResult, error)
	removeArchiveSources  func(*browser.ArchiveSources, uintptr) ([]string, error)
	invalidate            func()
	requests              chan loadRequest
	results               chan loadResult
	done                  chan struct{}
	recycle               func(string, []string, uintptr) (browser.RemovalResult, error)
	delete                func(string, []string, uintptr) (browser.RemovalResult, error)
	readClipboard         func() (browser.FileClipboard, error)
	writeClipboard        func([]string, bool) (browser.FileClipboard, error)
	writeArchiveClipboard func(*browser.ArchiveClipboard) (browser.FileClipboard, error)
	completeClipboard     func(browser.FileClipboard, []string) (browser.FileClipboard, error)
	transfer              func(browser.TransferRequest) (browser.TransferResult, error)
	createShortcuts       func([]string, string) (browser.TransferResult, error)
	readDirectory         func(string) (browser.Directory, error)
	renameEntry           func(string, string, string) error
	launchEditor          func(string, []string, string) error
	launchTerminal        func(string, bool, uintptr) error
	folderShortcutTarget  func(string) (string, error)
	readFavorites         func(string) ([]browser.Entry, error)
	createFavorite        func(string, string, string) (browser.Entry, error)
	readFileIcon          func(string, int) image.Image
	readFileTypeIcon      func(string, int, bool) image.Image
	busy                  atomic.Int32
	stopOnce              sync.Once
	stopped               chan struct{}
}

// One worker keeps reads and mutations off the UI thread and avoids concurrent
// filesystem access. Each panel allows only one pending directory read.
func newDirectoryLoader(read func(string) (browser.Directory, error), invalidate func(), panelCount ...int) *directoryLoader {
	count := 3
	if len(panelCount) > 0 {
		count = max(1, panelCount[0])
	}
	l := &directoryLoader{
		readDriveSpace:        browser.ReadFixedDriveSpace,
		calculateSize:         browser.CalculateSize,
		removeArchiveSources:  browser.RemoveArchiveSources,
		requests:              make(chan loadRequest, 2*count),
		results:               make(chan loadResult, count),
		done:                  make(chan struct{}),
		recycle:               browser.Recycle,
		delete:                browser.Delete,
		readClipboard:         browser.ReadFileClipboard,
		writeClipboard:        browser.WriteFileClipboard,
		writeArchiveClipboard: browser.WriteArchiveClipboard,
		invalidate:            invalidate,
		completeClipboard:     browser.CompleteFileClipboard,
		transfer:              browser.Transfer,
		createShortcuts:       browser.CreateShortcuts,
		stopped:               make(chan struct{}),
		readDirectory:         read,
		renameEntry:           browser.Rename,
		launchEditor:          browser.LaunchEditor,
		launchTerminal:        browser.LaunchTerminal,
		folderShortcutTarget:  browser.FolderShortcutTarget,
		readFavorites:         browser.ReadFavorites,
		createFavorite:        browser.CreateFavorite,
	}
	go func() {
		defer close(l.stopped)
		for {
			select {
			case <-l.done:
				return
			default:
			}
			select {
			case <-l.done:
				return
			case request := <-l.requests:
				group, name := request.traceKind()
				id := diagnostics.NextID()
				started := time.Now()
				if diagnostics.Enabled(group) {
					diagnostics.Record(group, id, "WorkerRequested", request.traceData(name))
				}
				publish := func(result loadResult) {
					diagnostics.Record(group, id, "WorkerCompleted", map[string]any{"kind": name, "error": diagnostics.Error(result.err), "duration_ms": time.Since(started).Seconds() * 1000, "entries": len(result.directory.Entries)})
					l.results <- result
				}
				if request.driveSpace != nil {
					for _, root := range request.driveSpace.roots {
						select {
						case <-l.done:
							return
						default:
						}
						space, known := l.readDriveSpace(root)
						completed := request
						completed.path = root
						publish(loadResult{request: completed, driveSpace: space, driveSpaceKnown: known})
						if invalidate != nil {
							invalidate()
						}
					}
					continue
				}
				if request.folderCacheLoad {
					l.folderCacheDir = request.path
					cache, err := loadFolderSizes(request.path, time.Now())
					l.folderSizes = cache
					diagnostics.Record(diagnostics.Storage, id, "FolderSizeCacheLoaded", map[string]any{"error": diagnostics.Error(err), "entries": len(cache)})
					publish(loadResult{request: request, folderSizes: cache.clone()})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if s := request.size; s != nil {
					size, cache, err := l.calculateCachedSize(s)
					publish(loadResult{request: request, size: size, folderSizes: cache, err: err})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.homeDirectory != "" {
					result := loadResult{request: request}
					if request.favoriteWrite != nil {
						result.created, result.err = l.createFavorite(request.homeDirectory, request.path, request.favoriteWrite.favorite.target)
					}
					if result.err == nil {
						result.directory, result.err = l.readHome(request.homeDirectory)
					}
					publish(result)
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if len(request.icons) > 0 {
					icons := make([]image.Image, len(request.icons))
					for i, key := range request.icons {
						if key.sample {
							if l.readFileTypeIcon != nil {
								icons[i] = l.readFileTypeIcon(key.path, key.size, key.directory)
							}
						} else if l.readFileIcon != nil {
							icons[i] = l.readFileIcon(key.path, key.size)
						}
					}
					publish(loadResult{request: request, icons: icons})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.completion != nil {
					result := loadResult{request: request}
					if request.favoritesDirectory != "" {
						result.favorites, result.err = l.readFavorites(request.favoritesDirectory)
					} else if localCompletionPath(request.path) {
						result.directory, result.err = l.readDirectory(request.path)
					}
					publish(result)
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.themeLibrary != nil {
					themes, err := readThemeLibrary(request.path)
					publish(loadResult{request: request, themes: themes, err: err})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.fontFamilies {
					publish(loadResult{request: request, fontFamilies: readFontFamilies()})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.archiveOpen != nil {
					var err error
					for _, name := range request.archiveOpen.names {
						err = errors.Join(err, browser.OpenArchive(browser.Location{Path: request.path, Folder: request.archiveFolder}, name, request.archiveOpen.open))
					}
					publish(loadResult{request: request, err: err})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.terminal != nil {
					result := loadResult{request: request, err: l.launchTerminal(request.path, request.terminal.admin, request.terminal.owner)}
					publish(result)
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.editor != nil {
					result := loadResult{request: request}
					for _, launch := range request.editor.launches {
						if err := l.launchEditor(launch.Program, launch.Arguments, request.path); err != nil {
							result.err = err
							break
						}
						result.editorsStarted++
					}
					publish(result)
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.settingsSave != nil {
					message, err := request.settingsSave.run()
					publish(loadResult{request: request, settingsMessage: message, err: err})
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				if request.multiRename != nil || request.unpack != nil || request.clipboard != nil || request.transfer != nil || request.duplicate != nil || (request.removal != nil || request.rename != nil || request.create != nil) && request.archiveFolder != "" {
					var result loadResult
					if request.multiRename != nil {
						result = l.multiRenameOperation(request)
					} else if request.unpack != nil {
						result = l.unpackOperation(request)
					} else if request.rename != nil {
						result = l.archiveRenameOperation(request)
					} else if request.create != nil {
						result = l.archiveCreateOperation(request)
					} else if request.removal != nil {
						result = l.archiveRemovalOperation(request)
					} else if request.duplicate != nil {
						result = l.duplicateOperation(request)
					} else {
						result = l.fileOperation(request)
					}
					publish(result)
					if invalidate != nil {
						invalidate()
					}
					continue
				}
				result := loadResult{request: request}
				mutationPath := request.path
				var resolveErr error
				if request.rename != nil || request.create != nil {
					result.locations = resolveFileLocations(request.locations)
					mutationPath, resolveErr = resolveFileLocation(request.path)
					result.mutationDirectory = mutationPath
				}
				var directory browser.Directory
				var created browser.Entry
				var removed browser.RemovalResult
				var sameDirectory []bool
				var err error
				if resolveErr != nil {
					err = resolveErr
				} else if r := request.rename; r != nil {
					err = browser.Rename(mutationPath, r.oldName, r.newName)
				} else if request.create != nil {
					if len(request.create.packPaths) > 0 {
						created, err = browser.PackZIP(mutationPath, request.create.name, request.create.packPaths)
					} else {
						result.mutationDirectory, created, err = browser.CreatePath(mutationPath, request.create.name, request.create.folder, request.create.data)
					}
					if filepath.Dir(request.create.name) != "." {
						l.refreshTransferPanels(&result)
						if err == nil {
							// Navigate through the original spelling, preserving directory aliases.
							directory, err = l.readDirectory(filepath.Join(request.path, filepath.Dir(request.create.name)))
						}
					}
				} else if request.removal != nil {
					// Capture alias/backing locations before removal can delete
					// open descendants and make fresh identity resolution fail.
					result.locations = resolveFileLocations(request.removal.locations)
					// Use the same canonical spelling for the removed parent and
					// captured panels. Windows temporary paths can contain 8.3
					// names, and aliases may disappear during the operation.
					parent, _ := resolveFileLocation(request.path)
					if request.removal.permanent {
						removed, err = l.delete(request.path, request.removal.names, request.removal.owner)
					} else {
						removed, err = l.recycle(request.path, request.removal.names, request.removal.owner)
					}
					if removed.Incomplete {
						// A skipped directory may already have lost some children.
						l.refreshTransferPanels(&result)
					}
					if parent != "" && len(removed.Names) > 0 {
						removed.Directory = parent
					}
					sameDirectory = make([]bool, len(request.removal.locations))
					if len(removed.Names) > 0 {
						for pi, resolved := range result.locations {
							sameDirectory[pi] = resolved != "" && samePath(filepath.Clean(resolved), removed.Directory)
						}
					}
				} else if request.fileDraft != nil {
					err = os.MkdirAll(request.path, 0700)
					if err == nil {
						directory, err = l.readDirectory(request.path)
					}
				} else if request.shortcut {
					target, resolveErr := l.folderShortcutTarget(request.path)
					if resolveErr != nil || target == "" {
						result.externalOpen = true
					} else {
						directory, err = l.readExistingParent(target)
					}
				} else if request.archiveFolder != "" {
					directory, err = browser.ReadArchive(browser.Location{Path: request.path, Folder: request.archiveFolder})
				} else if request.follow {
					directory, err = l.readDirectory(request.path)
				} else if request.explicitPath {
					if request.favoriteName != "" && request.favoritesDirectory != "" {
						if favorites, favoriteErr := l.readFavorites(request.favoritesDirectory); favoriteErr == nil {
							request.suggestedPath = favoriteCompletionTarget(favorites, request.favoriteName)
						}
					}
					directory, result.request.selectName, result.pathMissing, err = l.readExplicitPath(request.path, request.suggestedPath)
				} else {
					directory, err = l.readExistingParent(request.path)
				}
				result.directory, result.created, result.removed = directory, created, removed
				result.sameDirectory, result.err = sameDirectory, err
				publish(result)
				if invalidate != nil {
					invalidate()
				}
			}
		}
	}()
	return l
}

func (l *directoryLoader) close() { l.stopOnce.Do(func() { close(l.done) }) }

// Missing locations can recover at their closest surviving ancestor. Other
// read failures retain their usual feedback rather than changing the location.
// Run this only on the directory worker.
func (l *directoryLoader) readExistingParent(path string) (browser.Directory, error) {
	for {
		directory, err := l.readDirectory(path)
		if !browser.IsMissingDirectory(err) || path == "" {
			return directory, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			// No usable parent remains. Use the default root once, without looping.
			fallback := string(filepath.Separator)
			if runtime.GOOS == "windows" {
				fallback = `C:\`
			}
			return l.readDirectory(fallback)
		}
		path = parent
	}
}

// Explicit locations may name files or incomplete paths. Stay within the given
// ancestry, stopping at permission errors or an unavailable root. All reads and
// file probing stay on the worker, independently of UI input and rendering.
func (l *directoryLoader) readExplicitPath(path, suggestedPath string) (browser.Directory, string, bool, error) {
	if location, ok := browser.ResolveArchiveLocation(path); ok {
		directory, err := browser.ReadArchive(location)
		if browser.IsMissingDirectory(err) && location.Folder != "." {
			parent, name := location.Parent()
			if containing, parentErr := browser.ReadArchive(parent); parentErr == nil {
				for _, entry := range containing.Entries {
					if entry.Name == name && !entry.Directory {
						return containing, name, false, nil
					}
				}
			}
		}
		return directory, "", false, err
	}
	directory, err := l.readDirectory(path)
	if !browser.IsMissingDirectory(err) {
		return directory, "", false, err
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !browser.IsMissingDirectory(statErr) {
		return browser.Directory{}, "", false, statErr
	}
	// Existing literal paths always win. Try only the captured, visible
	// completion, and only for a missing literal; permission/read errors above
	// retain their normal behavior. A vanished suggestion keeps parent recovery.
	if browser.IsMissingDirectory(statErr) && suggestedPath != "" {
		if _, suggestionErr := os.Stat(suggestedPath); suggestionErr == nil {
			return l.readExplicitPath(suggestedPath, "")
		} else if !browser.IsMissingDirectory(suggestionErr) {
			return browser.Directory{}, "", false, suggestionErr
		}
	}
	selected := ""
	if statErr == nil && !info.IsDir() {
		selected = filepath.Base(path)
	}
	missing := selected == ""
	for browser.IsMissingDirectory(err) {
		parent := filepath.Dir(path)
		if parent == path {
			return browser.Directory{}, "", false, err
		}
		path = parent
		directory, err = l.readDirectory(path)
		if err == nil {
			return directory, selected, missing, nil
		}
		selected, missing = "", true
	}
	return browser.Directory{}, "", false, err
}
