package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestCustomActionConsoleLaunch(t *testing.T) {
	const outputKey, consoleKey = "COMMANDO_ACTION_TEST_OUTPUT", "COMMANDO_ACTION_TEST_CONSOLE"
	type report struct {
		Directory string
		Arguments []string
		Error     string
	}
	if output := os.Getenv(outputKey); output != "" {
		directory, err := os.Getwd()
		r := report{Directory: directory, Arguments: os.Args[len(os.Args)-3:]}
		if err != nil {
			r.Error = err.Error()
		}
		if os.Getenv(consoleKey) == "1" {
			for _, id := range []uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
				handle, err := windows.GetStdHandle(id)
				var mode uint32
				if err != nil || windows.GetConsoleMode(handle, &mode) != nil {
					r.Error = "missing interactive console handle"
				}
			}
		}
		data, _ := json.Marshal(r)
		if err := os.WriteFile(output+".tmp", data, 0600); err != nil {
			os.Exit(2)
		}
		if err := os.Rename(output+".tmp", output); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprint(terminal), func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "שלום & 100% folder")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(directory, "result.json")
			t.Setenv(outputKey, output)
			t.Setenv(consoleKey, map[bool]string{false: "0", true: "1"}[terminal])
			want := []string{"a file.txt", "100% ! & ^ literal.txt", `embedded"quote`}
			args := append([]string{"-test.run=^TestCustomActionConsoleLaunch$", "--"}, want...)
			if err := LaunchAction(program, args, directory, terminal); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			var data []byte
			for {
				data, err = os.ReadFile(output)
				if err == nil || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err != nil {
				t.Fatal("action child did not report", err)
			}
			var got report
			if err := json.Unmarshal(data, &got); err != nil || got.Error != "" || !slices.Equal(got.Arguments, want) {
				t.Fatal("action lost arguments, working directory or console input", got, err)
			}
			actual, err := os.Stat(got.Directory)
			if err != nil {
				t.Fatal(err)
			}
			wantDirectory, err := os.Stat(directory)
			if err != nil || !os.SameFile(actual, wantDirectory) {
				t.Fatal("action started in a different directory", got.Directory, directory, err)
			}
		})
	}
}
