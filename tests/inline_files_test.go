package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInlineFileValidationPreservesOutput(t *testing.T) {
	for _, module := range []string{"files_text", "files_base64"} {
		t.Run(module, func(t *testing.T) {
			for _, fields := range []string{
				"destination: /tmp/file",
				"destination: /tmp/file\n    content: null",
				"destination: relative\n    content: ''",
				"destination: /tmp/file\n    content: ''\n    mode: '999'",
				"destination: /tmp/file\n    content: ''\n    source: /tmp/source",
				"destination: /tmp/file\n    content: ''\n    content: ''",
			} {
				input := manifest(t, module+":\n  - "+fields+"\n")
				rejectGeneration(t, input, input)
			}
		})
	}
}

func TestInlineFileOrderAndFragments(t *testing.T) {
	containerOnly(t)
	for _, tc := range []struct{ module, first, second, wantFirst, wantSecond string }{
		{"files_text", "first", "second", "first", "second"},
		{"files_base64", "|\n      aGVs\n      bG8K", "d29ybGQ=", "hello", "world"},
	} {
		t.Run(tc.module, func(t *testing.T) {
			work := t.TempDir()
			dest := filepath.Join(work, "file")
			write(t, filepath.Join(work, "fragment.yaml"), fmt.Sprintf("%s:\n  - destination: %s\n    content: ''\n", tc.module, dest))
			input := filepath.Join(work, "main.yaml")
			write(t, input, fmt.Sprintf(`base:
  include: [fragment.yaml]
%s:
  - destination: %s
    content: %s
exec_cmd:
  - test "$(cat '%s')" = %s
%s:
  - destination: %s
    content: %s
exec_cmd:
  - test "$(cat '%s')" = %s
`, tc.module, dest, tc.first, dest, tc.wantFirst, tc.module, dest, tc.second, dest, tc.wantSecond))
			if out, err := runScript(generate(t, input)); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			info, err := os.Stat(dest)
			if err != nil || info.Size() != 0 || info.Mode().Perm() != 0o644 {
				t.Fatalf("fragment empty/default file: %v, %v", info, err)
			}
		})
	}
}

func TestInlineFileWriteFailureStopsScript(t *testing.T) {
	containerOnly(t)
	for _, module := range []string{"files_text", "files_base64"} {
		t.Run(module, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "not-a-directory")
			write(t, parent, "keep")
			input := manifest(t, fmt.Sprintf("%s:\n  - destination: %s/file\n    content: aGVsbG8=\nexec_cmd: [echo unreachable]\n", module, parent))
			out, err := runScript(generate(t, input))
			if err == nil || !strings.Contains(string(out), "ERROR:") || strings.Contains(string(out), "unreachable") {
				t.Fatalf("%v: %s", err, out)
			}
			if read(t, parent) != "keep" {
				t.Fatal("failed write changed the parent file")
			}
		})
	}
}
