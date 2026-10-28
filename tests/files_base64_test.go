package tests

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBase64Files(t *testing.T) {
	containerOnly(t)
	work := t.TempDir()
	dest := filepath.Join(work, "subdir/file's data")
	payload := append([]byte("$(echo do-not-execute) `false` %s \\c ' \"\r\n"), bytes.Repeat([]byte{0, 1, 255, 128, '\n'}, 256)...)
	entries := map[string]any{"files_base64": []map[string]any{{
		"name": "binary", "destination": dest,
		"content": base64.StdEncoding.EncodeToString(payload),
		"owner":   "nobody", "group": "nogroup", "mode": "640",
		"exec": "test -s \"file's data\"; pwd > cwd",
	}}}
	data, err := yaml.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	script := generate(t, manifest(t, string(data)))
	if strings.Contains(read(t, script), "base64 --decode") {
		t.Fatal("base64 decoding must happen at generation")
	}
	for i := 0; i < 2; i++ {
		if out, err := runScript(script); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		actual, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(actual, payload) {
			t.Fatalf("bytes differ: %v", err)
		}
	}
	if got := strings.TrimSpace(read(t, filepath.Join(work, "subdir/cwd"))); got != filepath.Dir(dest) {
		t.Fatalf("file exec directory: %q", got)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode: %v, %v", info, err)
	}
	// Check ownership using the target's own user database.
	check := fmt.Sprintf("exec_cmd:\n  - test \"$(stat -c '%%U:%%G' %q)\" = nobody:nogroup\n", dest)
	if out, err := runScript(generate(t, manifest(t, check))); err != nil {
		t.Fatalf("ownership: %v: %s", err, out)
	}
}
