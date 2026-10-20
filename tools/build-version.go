package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	fmt.Println(buildVersion())
}

func buildVersion() string {
	version := cleanBuildVersion()
	if repositoryDirty() {
		version += "-dirty"
	}
	return version
}

func cleanBuildVersion() string {
	tag, err := gitOutput("describe", "--tags", "--match", "v*", "--exact-match")
	if err == nil && isVersionTag(tag) {
		return strings.TrimPrefix(tag, "v")
	}

	baseTag, err := gitOutput("describe", "--tags", "--match", "v*", "--abbrev=0")
	if err != nil || !isVersionTag(baseTag) {
		return "0.0.0-dev"
	}

	count, err := gitOutput("rev-list", "--count", baseTag+"..HEAD")
	if err != nil {
		return strings.TrimPrefix(baseTag, "v") + "-dev"
	}

	next, ok := nextPatch(strings.TrimPrefix(baseTag, "v"))
	if !ok {
		return strings.TrimPrefix(baseTag, "v") + "-dev." + count
	}

	return next + "-dev." + count
}

func repositoryDirty() bool {
	status, err := gitOutput("status", "--porcelain", "--untracked-files=normal")
	return err == nil && status != ""
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isVersionTag(tag string) bool {
	if !strings.HasPrefix(tag, "v") {
		return false
	}
	_, ok := nextPatch(strings.TrimPrefix(tag, "v"))
	return ok
}

func nextPatch(version string) (string, bool) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", false
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1), true
}
