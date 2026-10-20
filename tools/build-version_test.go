package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestBuildVersionFromGit(t *testing.T) {
	t.Chdir(t.TempDir())
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Version Test", "GIT_AUTHOR_EMAIL=version@example.invalid", "GIT_COMMITTER_NAME=Version Test", "GIT_COMMITTER_EMAIL=version@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	wantVersion := func(want string) {
		t.Helper()
		if got := buildVersion(); got != want {
			t.Fatalf("buildVersion() = %q, want %q", got, want)
		}
	}
	git("init", "--quiet")
	git("-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "Initial")
	wantVersion("0.0.0-dev")
	git("tag", "v1.2.3")
	wantVersion("1.2.3")
	if err := os.WriteFile("changed.txt", []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	wantVersion("1.2.3-dirty")
	git("add", "changed.txt")
	git("-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Change")
	wantVersion("1.2.4-dev.1")
	git("-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "Another")
	wantVersion("1.2.4-dev.2")
	git("tag", "v1.2.4")
	wantVersion("1.2.4")
}

func TestNextPatch(t *testing.T) {
	tests := []struct {
		version string
		want    string
		wantOK  bool
	}{
		{version: "0.1.0", want: "0.1.1", wantOK: true},
		{version: "12.34.56", want: "12.34.57", wantOK: true},
		{version: "1.2", wantOK: false},
		{version: "1.2.beta", wantOK: false},
	}
	for _, test := range tests {
		got, ok := nextPatch(test.version)
		if got != test.want || ok != test.wantOK {
			t.Errorf("nextPatch(%q) = (%q, %t), want (%q, %t)", test.version, got, ok, test.want, test.wantOK)
		}
	}
}

func TestIsVersionTag(t *testing.T) {
	for _, tag := range []string{"v0.1.0", "v12.34.56"} {
		if !isVersionTag(tag) {
			t.Errorf("isVersionTag(%q) = false, want true", tag)
		}
	}
	for _, tag := range []string{"0.1.0", "v1.2", "latest", "v1.2.3-rc1"} {
		if isVersionTag(tag) {
			t.Errorf("isVersionTag(%q) = true, want false", tag)
		}
	}
}
