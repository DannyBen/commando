package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestSimultaneousInstanceLaunchesChooseOneOwner(t *testing.T) {
	name := `Local\` + instanceName(t.TempDir())
	type result struct {
		release func()
		err     error
	}
	results := make(chan result, 8)
	for range cap(results) {
		go func() {
			release, err := claimInstance(name)
			results <- result{release: release, err: err}
		}()
	}
	winners := 0
	for range cap(results) {
		r := <-results
		if r.err != nil {
			t.Error(r.err)
		}
		if r.release != nil {
			winners++
			t.Cleanup(r.release)
		}
	}
	if winners != 1 {
		t.Fatalf("simultaneous launches chose %d owners", winners)
	}
}

func TestInstanceLockExcludesOtherOwnersAndReleases(t *testing.T) {
	name := `Local\` + instanceName(t.TempDir())
	release, err := claimInstance(name)
	if err != nil || release == nil {
		t.Fatalf("first owner: %v", err)
	}
	t.Cleanup(release)
	other, err := claimInstance(name)
	if other != nil {
		other()
	}
	if err != nil || other != nil {
		t.Fatalf("second owner was not excluded: %v", err)
	}
	release()
	release() // Startup failures and normal shutdown may both call cleanup.
	other, err = claimInstance(name)
	if err != nil || other == nil {
		t.Fatalf("new owner after release: %v", err)
	}
	other()
}

func TestInstanceLockRecoversAfterProcessCrash(t *testing.T) {
	name := `Local\` + instanceName(t.TempDir())
	// Keep a non-owning handle open, so recovery exercises WAIT_ABANDONED
	// rather than just creating a new mutex after the old object disappears.
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	mutex, err := windows.CreateMutex(nil, false, ptr)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(mutex)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstanceCrashHelper$")
	child.Env = append(os.Environ(), "COMMANDO_TEST_INSTANCE="+name)
	child.Stderr = os.Stderr
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	in, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Process.Kill(); child.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "ready\n" {
		child.Process.Kill()
		child.Wait()
		t.Fatalf("child owner not ready: %q %v", line, err)
	}
	other, err := claimInstance(name)
	if other != nil {
		other()
	}
	if err != nil || other != nil {
		t.Fatalf("live child was not exclusive: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	child.Wait()
	release, err := claimInstance(name)
	if err != nil || release == nil {
		t.Fatalf("crashed owner prevented restart: %v", err)
	}
	release()
}

func TestInstanceCrashHelper(t *testing.T) {
	name := os.Getenv("COMMANDO_TEST_INSTANCE")
	if name == "" {
		t.Skip("subprocess helper")
	}
	release, err := claimInstance(name)
	if err != nil || release == nil {
		t.Fatalf("child claim: %v", err)
	}
	fmt.Println("ready")
	io.Copy(io.Discard, os.Stdin)
	release()
}

func TestInstanceIdentityFollowsDataFolder(t *testing.T) {
	if instanceName(`C:\Users\Me\AppData\Local\Commando`) != instanceName(`c:\users\me\appdata\local\commando\.`) {
		t.Fatal("case or path spelling split instance ownership")
	}
	if instanceName(`C:\Users\Me\AppData\Local\Commando`) == instanceName(`C:\Users\Other\AppData\Local\Commando`) {
		t.Fatal("different users share instance ownership")
	}
}
