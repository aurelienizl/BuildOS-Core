package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary, root string

func TestMain(m *testing.M) {
	if os.Getenv("BUILDOS_CONTAINER_TESTS") != "1" {
		fmt.Fprintln(os.Stderr, "Run make test: tests must run inside disposable containers")
		os.Exit(1)
	}
	root = os.Getenv("BUILDOS_ROOT")
	binary = os.Getenv("BUILDOS_BINARY")
	if root == "" || binary == "" {
		panic("missing test image configuration")
	}
	os.Exit(m.Run())
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func generate(t *testing.T, config string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "script.sh")
	cmd := exec.Command(binary, "--config", config, "--output", output)
	// Includes must work independently of the generator's working directory.
	cmd.Dir = t.TempDir()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generation: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", "-n", output).CombinedOutput(); err != nil {
		t.Fatalf("invalid Bash: %v\n%s", err, out)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("script permissions: %v, %v", info, err)
	}
	return output
}
func manifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	write(t, path, content)
	return path
}
func containerOnly(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Fatal("container execution tests require root")
	}
}
func runScript(script string, env ...string) ([]byte, error) {
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

// rejectGeneration checks the shared failure contract, including atomic output.
func rejectGeneration(t *testing.T, input, context string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "script.sh")
	write(t, output, "existing script")
	out, err := exec.Command(binary, "--config", input, "--output", output).CombinedOutput()
	if err == nil || !strings.Contains(string(out), context) {
		t.Fatalf("expected generation failure with %s context: %v: %s", context, err, out)
	}
	if read(t, output) != "existing script" {
		t.Fatal("failed generation changed existing output")
	}
	return string(out)
}
