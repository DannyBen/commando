package browser

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseGitStatus(t *testing.T) {
	output := " M sub/modified.txt\x00M  sub/staged.txt\x00AM sub/added.txt\x00?? sub/new ü \"file\".txt\x00" +
		"UU sub/conflict.txt\x00AA sub/both-added.txt\x00!! sub/ignored.txt\x00" +
		"R  sub/renamed.txt\x00sub/original.txt\x00?? sub/line\nbreak.txt\x00" +
		" M sub/nested/file.txt\x00?? sibling.txt\x00?? sub/new-folder/deep/file.txt\x00" +
		"?? sub/mixed/new.txt\x00 D sub/mixed/removed.txt\x00?? sub/mixed/another.txt\x00" +
		" D sub/deleted/deep/file.txt\x00"
	want := map[string]GitStatus{
		"modified.txt": GitChanged, "staged.txt": GitChanged, "added.txt": GitNew,
		"new ü \"file\".txt": GitNew, "conflict.txt": GitChanged, "both-added.txt": GitChanged,
		"renamed.txt": GitChanged, "line\nbreak.txt": GitNew,
		"nested": GitChanged, "new-folder": GitNew, "mixed": GitChanged, "deleted": GitChanged,
	}
	got, err := parseGitStatus([]byte(output), "sub/")
	if err != nil || !maps.Equal(got, want) {
		t.Fatalf("status = %#v, %v; want %#v", got, err, want)
	}
	for _, bad := range []string{" M truncated", "M\x00", "xx unknown\x00", "R  target\x00"} {
		if got, err := parseGitStatus([]byte(bad), ""); !errors.Is(err, ErrGitStatus) || got != nil {
			t.Fatalf("malformed status %q = %#v, %v", bad, got, err)
		}
	}
}

func TestGitStatusFolderCase(t *testing.T) {
	got, err := parseGitStatus([]byte(" M Sub/file.txt\x00"), "sub/")
	want := map[string]GitStatus{}
	if runtime.GOOS == "windows" {
		want["file.txt"] = GitChanged
	}
	if err != nil || !maps.Equal(got, want) {
		t.Fatal("folder case matching", got, err)
	}
}

func TestReadGitIgnoredEnvrc(t *testing.T) {
	program, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	for _, source := range []string{"repository", "global", "info/exclude"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			globalConfig := filepath.Join(t.TempDir(), "gitconfig")
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(program, args...)
				cmd.Dir, cmd.Env = root, gitEnvironment()
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, output)
				}
			}
			write := func(path, data string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			run("init", "-q")
			write(filepath.Join(root, ".envrc"), "# ignored fixture\n")
			write(filepath.Join(root, "nested", "deep", ".envrc"), "# ignored fixture\n")
			write(filepath.Join(root, "new.txt"), "new\n")
			want := map[string]GitStatus{"new.txt": GitNew}
			switch source {
			case "repository":
				write(filepath.Join(root, ".gitignore"), ".envrc\n")
				want[".gitignore"] = GitNew
			case "global":
				ignore := filepath.Join(t.TempDir(), "ignore")
				write(ignore, ".envrc\n")
				run("config", "--file", globalConfig, "core.excludesFile", ignore)
			case "info/exclude":
				write(filepath.Join(root, ".git", "info", "exclude"), ".envrc\n")
			}
			got, err := ReadGitStatus(context.Background(), root, program)
			if err != nil || !maps.Equal(got, want) {
				t.Fatalf("%s ignore: status=%#v, %v; want %#v", source, got, err, want)
			}
		})
	}
}

func TestReadGitStatus(t *testing.T) {
	program, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	root := filepath.Join(t.TempDir(), "repo with spaces ü")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "no-config"))
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(program, append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=" + filepath.Join(root, "no-hooks"), "-c", "commit.gpgsign=false"}, args...)...)
		command.Dir, command.Env = root, gitEnvironment()
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	write := func(name, data string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	run("config", "core.autocrlf", "false")
	write(".gitignore", "*.tmp\n")
	write(".gitattributes", "*.txt text eol=crlf\n")
	for _, name := range []string{"modified.txt", "staged.txt", "clean.txt", "sub/nested ü.txt", "removed/deep/file.txt"} {
		write(name, "original\r\n")
	}
	run("add", ".")
	run("commit", "-qm", "fixture")
	write("modified.txt", "changed\r\n")
	write("staged.txt", "staged change\r\n")
	write("added.txt", "staged new\r\n")
	run("add", "staged.txt", "added.txt")
	write("new ü.txt", "untracked\r\n")
	write("ignored.tmp", "ignored\n")
	write("sub/nested ü.txt", "nested change\r\n")
	write("only-new/deep/file.txt", "new\n")
	if err := os.Remove(filepath.Join(root, "removed", "deep", "file.txt")); err != nil {
		t.Fatal(err)
	}
	write("local-only", "ignored by info/exclude\n")
	write(".git/info/exclude", "local-only\n")
	write("global-only", "ignored by core.excludesFile\n")
	globalIgnore := filepath.Join(t.TempDir(), "excludes")
	if err := os.WriteFile(globalIgnore, []byte("global-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("config", "core.excludesFile", globalIgnore)
	// Changing only a timestamp must not turn a clean CRLF file into a change.
	info, err := os.Stat(filepath.Join(root, "clean.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "clean.txt"), info.ModTime(), info.ModTime().Add(-1e9)); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".git", "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]GitStatus{"modified.txt": GitChanged, "staged.txt": GitChanged, "added.txt": GitNew, "new ü.txt": GitNew, "sub": GitChanged, "only-new": GitNew, "removed": GitChanged}
	// An inherited override must not redirect the query away from the panel.
	t.Setenv("GIT_INDEX_FILE", filepath.Join(root, "wrong-index"))
	for _, executable := range []string{program, ""} {
		got, err := ReadGitStatus(context.Background(), root, executable)
		if err != nil || !maps.Equal(got, want) {
			t.Fatalf("status(%q) = %#v, %v; want %#v", executable, got, err, want)
		}
	}
	after, err := os.ReadFile(indexPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("status changed the Git index", err)
	}
	got, err := ReadGitStatus(context.Background(), filepath.Join(root, "sub"), program)
	if err != nil || !maps.Equal(got, map[string]GitStatus{"nested ü.txt": GitChanged}) {
		t.Fatal("subfolder status", got, err)
	}
	for folder, status := range map[string]GitStatus{"only-new": GitNew, "removed": GitChanged} {
		got, err := ReadGitStatus(context.Background(), filepath.Join(root, folder), program)
		if err != nil || !maps.Equal(got, map[string]GitStatus{"deep": status}) {
			t.Fatal("descendant status while drilling into folder", folder, got, err)
		}
	}
	linked := filepath.Join(t.TempDir(), "linked worktree")
	run("worktree", "add", "-qb", "linked", linked)
	if err := os.WriteFile(filepath.Join(linked, "new.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = ReadGitStatus(context.Background(), linked, program)
	if err != nil || !maps.Equal(got, map[string]GitStatus{"new.txt": GitNew}) {
		t.Fatal("linked worktree status", got, err)
	}
	got, err = ReadGitStatus(context.Background(), t.TempDir(), program)
	if err != nil || got != nil {
		t.Fatal("non-repository was not quiet", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadGitStatus(ctx, root, program); !errors.Is(err, ErrGitTimeout) {
		t.Fatal("cancelled query", err)
	}
}

func TestGitUnavailableAndOutputLimit(t *testing.T) {
	root, missing := t.TempDir(), filepath.Join(t.TempDir(), "missing-git")
	if got, err := ReadGitStatus(context.Background(), root, missing); err != nil || got != nil {
		t.Fatal("looked for Git outside a repository", got, err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadGitStatus(context.Background(), root, missing); !errors.Is(err, ErrGitUnavailable) {
		t.Fatal("missing executable", err)
	}
	var output gitOutput
	if _, err := output.Write(make([]byte, (8<<20)+1)); !errors.Is(err, errGitOutputLimit) || len(output.Bytes()) != 0 {
		t.Fatal("unbounded Git output", err)
	}
}

func TestGitWorktreeMarkers(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "child")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if hasGitWorktree(nested) {
		t.Fatal("ordinary folder treated as a repository")
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !hasGitWorktree(nested) {
		t.Fatal("linked-worktree marker in ancestor was missed")
	}
}
