// Command faketool is the fake odin and odinfmt executable used by Woden's
// command tests. Package faketool installs two copies of it, named odin and
// odinfmt.
//
// Each invocation appends one JSON line to the file named by
// WODEN_FAKE_TOOL_LOG (when set) and exits with the code named by
// WODEN_FAKE_TOOL_EXIT (default 0). The fake tools print nothing, so the tests
// can treat any output as coming from Woden itself.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// record is one logged invocation. It mirrors faketool.Record; the two must
// stay in sync.
type record struct {
	Tool string   `json:"tool"`
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
	Exit int      `json:"exit"`
}

func main() {
	exitCode := 0
	if raw := os.Getenv("WODEN_FAKE_TOOL_EXIT"); raw != "" {
		if code, err := strconv.Atoi(raw); err == nil {
			exitCode = code
		}
	}
	dir, _ := os.Getwd()
	name := filepath.Base(os.Args[0])
	invocation := record{
		Tool: strings.TrimSuffix(name, filepath.Ext(name)),
		Args: os.Args[1:],
		Dir:  dir,
		Exit: exitCode,
	}
	if path := os.Getenv("WODEN_FAKE_TOOL_LOG"); path != "" {
		if file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			if data, err := json.Marshal(invocation); err == nil {
				_, _ = file.Write(append(data, '\n'))
			}
			_ = file.Close()
		}
	}
	os.Exit(exitCode)
}
