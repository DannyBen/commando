package browser

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type GitStatus uint8

const (
	GitClean GitStatus = iota
	GitNew
	GitChanged
)

var (
	ErrGitUnavailable = errors.New("Git executable not found")
	ErrGitTimeout     = errors.New("Git status timed out")
	ErrGitStatus      = errors.New("Cannot read Git status")
	ErrGitUntrusted   = errors.New("Git repository not trusted")
	errGitOutputLimit = errors.New("Git status too large")
)

// ReadGitStatus returns immediate entries, summarizing descendant changes for
// folders. Git owns ignore rules and comparisons; no shell or mutation is invoked.
func ReadGitStatus(ctx context.Context, directory, executable string) (map[string]GitStatus, error) {
	if !hasGitWorktree(directory) {
		return nil, nil
	}
	program, err := FindGitExecutable(executable)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	run := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, program, append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false"}, args...)...)
		command.Dir = directory
		command.Env = gitEnvironment()
		command.WaitDelay = 100 * time.Millisecond
		hideGitWindow(command)
		var output, stderr gitOutput
		command.Stdout, command.Stderr = &output, &stderr
		err := command.Run()
		switch {
		case ctx.Err() != nil:
			return nil, ErrGitTimeout
		case errors.Is(err, errGitOutputLimit):
			return nil, errGitOutputLimit
		case errors.Is(err, os.ErrNotExist), errors.Is(err, exec.ErrNotFound):
			return nil, ErrGitUnavailable
		case err != nil:
			message := stderr.String()
			if strings.Contains(message, "not a git repository") || strings.Contains(message, "must be run in a work tree") {
				return nil, nil
			}
			if strings.Contains(message, "dubious ownership") {
				return nil, ErrGitUntrusted
			}
			return nil, ErrGitStatus
		}
		return output.Bytes(), nil
	}
	// Porcelain paths are relative to the repository root. The prefix lets us
	// select immediate children even when browsing a subfolder or linked worktree.
	prefix, err := run("rev-parse", "--show-prefix")
	if err != nil || prefix == nil {
		return nil, err
	}
	output, err := run("status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--", ".")
	if err != nil {
		return nil, err
	}
	return parseGitStatus(output, strings.TrimSuffix(strings.TrimSuffix(string(prefix), "\n"), "\r"))
}

// FindGitExecutable also serves Settings validation, without needing a repository.
func FindGitExecutable(executable string) (string, error) {
	if executable == "" {
		executable = "git"
	}
	program, err := exec.LookPath(executable)
	if err != nil {
		return "", ErrGitUnavailable
	}
	program, err = filepath.Abs(program)
	if err != nil {
		return "", ErrGitUnavailable
	}
	return program, nil
}

func hasGitWorktree(directory string) bool {
	// A panel may reach a repository through a folder symlink or junction.
	directory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return false
	}
	for {
		info, err := os.Stat(filepath.Join(directory, ".git"))
		if err == nil {
			if info.IsDir() {
				head, err := os.Stat(filepath.Join(directory, ".git", "HEAD"))
				return err == nil && head.Mode().IsRegular()
			}
			return info.Mode().IsRegular() // Linked worktrees use a file.
		}
		parent := filepath.Dir(directory)
		if !os.IsNotExist(err) || parent == directory {
			return false
		}
		directory = parent
	}
}

func parseGitStatus(output []byte, prefix string) (map[string]GitStatus, error) {
	statuses := make(map[string]GitStatus)
	for len(output) > 0 {
		end := bytes.IndexByte(output, 0)
		if end < 3 || output[2] != ' ' {
			return nil, ErrGitStatus
		}
		x, y, path := output[0], output[1], string(output[3:end])
		if !strings.ContainsRune(" MADRCUT?!", rune(x)) || !strings.ContainsRune(" MADRCUT?!", rune(y)) {
			return nil, ErrGitStatus
		}
		output = output[end+1:]
		// Defensive support if an older Git still emits a rename/copy record.
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			end = bytes.IndexByte(output, 0)
			if end < 0 {
				return nil, ErrGitStatus
			}
			output = output[end+1:]
		}
		relative, found := gitRelativePath(path, prefix)
		name, _, _ := strings.Cut(relative, "/")
		if !found || name == "" || x == '!' || x == ' ' && y == ' ' {
			continue
		}
		status := GitChanged
		conflict := x == 'U' || y == 'U' || x == y && (x == 'A' || x == 'D')
		if x == '?' && y == '?' || x == 'A' && !conflict {
			status = GitNew
		}
		// Changed wins over new, independent of Git's output order. This also
		// covers folders whose only changes are deleted descendants.
		statuses[name] = max(statuses[name], status)
	}
	return statuses, nil
}

func gitRelativePath(path, prefix string) (string, bool) {
	for prefix != "" {
		folder, rest, _ := strings.Cut(prefix, "/")
		name, tail, found := strings.Cut(path, "/")
		matches := name == folder
		if runtime.GOOS == "windows" {
			matches = strings.EqualFold(name, folder)
		}
		if !found || !matches {
			return "", false
		}
		prefix, path = rest, tail
	}
	return path, true
}

// Ignore inherited repository overrides: Commando's current panel determines
// the repository, even when Commando was started from a Git hook or helper.
func gitEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(name) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_PREFIX":
			continue
		}
		env = append(env, value)
	}
	return append(env, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1")
}

type gitOutput struct{ buffer bytes.Buffer }

func (b *gitOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *gitOutput) String() string { return b.buffer.String() }

func (b *gitOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 8<<20 {
		return 0, errGitOutputLimit
	}
	return b.buffer.Write(data)
}
