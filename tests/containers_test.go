package tests

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainerExec(t *testing.T) {
	containerOnly(t)
	command := `docker run --rm -e 'MESSAGE=hello "world"' example:latest`
	for _, tc := range []struct {
		name, command, want string
		badChecksum         bool
		loadStatus, status  int
	}{
		{name: "quoted-command", command: command, want: "load\nrun\nload\nafter\n"},
		{name: "empty", want: "load\nload\nafter\n"},
		{name: "whitespace", command: " \t\n", want: "load\nload\nafter\n"},
		{name: "checksum-failure", command: command, badChecksum: true, status: 1},
		{name: "load-failure", command: command, loadStatus: 42, status: 42, want: "load\n"},
		{name: "exec-failure", command: "exit 23", status: 23, want: "load\n"},
		{name: "semicolon-failure", command: `false; echo unreachable >> "$TRACE"`, status: 1, want: "load\n"},
		{name: "pipeline-failure", command: `false | cat; echo unreachable >> "$TRACE"`, status: 1, want: "load\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			source, trace := filepath.Join(work, "archive"), filepath.Join(work, "trace")
			write(t, source, "archive content")
			checksum := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("archive content")))
			firstChecksum := checksum
			if tc.badChecksum {
				firstChecksum = "sha256:" + strings.Repeat("0", 64)
			}
			docker := filepath.Join(work, "docker")
			write(t, docker, `#!/bin/bash
set -Eeuo pipefail
case "$1" in
load)
    test "$#" = 3
    test "$2" = -i
    test "$(cat "$3")" = 'archive content'
    echo load >> "$TRACE"
    exit "$LOAD_STATUS"
    ;;
run)
    test "$#" = 5
    test "$2" = --rm
    test "$3" = -e
    test "$4" = 'MESSAGE=hello "world"'
    test "$5" = example:latest
    echo run >> "$TRACE"
    ;;
*) exit 99 ;;
esac
`)
			if err := os.Chmod(docker, 0o700); err != nil {
				t.Fatal(err)
			}
			input := manifest(t, fmt.Sprintf(`containers:
  - name: first
    source: %q
    checksum: %s
    exec: %q
  - name: second
    source: %q
    checksum: %s
exec_cmd:
  - echo after >> "$TRACE"
`, source, firstChecksum, tc.command, source, checksum))
			script := generate(t, input)
			if _, err := os.Stat(trace); !os.IsNotExist(err) {
				t.Fatalf("generation executed a command: %v", err)
			}
			write(t, trace, "")
			out, err := runScript(script, "PATH="+work+":"+os.Getenv("PATH"), "TRACE="+trace, fmt.Sprintf("LOAD_STATUS=%d", tc.loadStatus))
			if tc.status == 0 {
				if err != nil {
					t.Fatalf("application failed: %v: %s", err, out)
				}
			} else {
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != tc.status {
					t.Fatalf("expected exit %d: %v: %s", tc.status, err, out)
				}
				if !strings.Contains(string(out), "ERROR:") || strings.Contains(string(out), "Configuration completed") {
					t.Fatalf("incorrect failure log: %s", out)
				}
			}
			if got := read(t, trace); got != tc.want {
				t.Fatalf("execution order: %q; want %q", got, tc.want)
			}
		})
	}
}
