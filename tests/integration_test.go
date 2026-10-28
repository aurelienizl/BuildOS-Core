package tests

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidConfigurationsPreserveOutput(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(root, "tests/fixtures/invalid/*.yaml"))
	if err != nil || len(paths) == 0 {
		t.Fatal("missing invalid cases", err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			rejectGeneration(t, path, path)
		})
	}
}

func TestGenerationDoesNotExecute(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")
	generate(t, manifest(t, fmt.Sprintf("exec_cmd:\n  - touch '%s'\n", marker)))
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("generation executed a command: %v", err)
	}
}

func TestRefuseOverwritingInputs(t *testing.T) {
	dir := t.TempDir()
	fragment := filepath.Join(dir, "fragment.yaml")
	write(t, fragment, "exec_cmd: [echo fragment]\n")
	input := filepath.Join(dir, "main.yaml")
	write(t, input, "base:\n  include: [fragment.yaml]\n")
	alias := filepath.Join(dir, "alias.yaml")
	if err := os.Symlink(fragment, alias); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(dir, ".env")
	write(t, envPath, "PASSWORD=secret\n")
	envAlias, envHardLink := filepath.Join(dir, "env-alias"), filepath.Join(dir, "env-hard-link")
	if err := os.Symlink(envPath, envAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(envPath, envHardLink); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{input, fragment, alias, envPath, envAlias, envHardLink} {
		before := read(t, output)
		if out, err := exec.Command(binary, "--config", input, "--output", output).CombinedOutput(); err == nil {
			t.Fatalf("overwrote input: %s", out)
		}
		if read(t, output) != before {
			t.Fatal("input changed")
		}
	}
}

func TestCLIRequiresNamedFlags(t *testing.T) {
	for _, args := range [][]string{{}, {"--config", "x"}, {"--output", "x"}, {"--dry-run", "x"}, {"x"}} {
		if out, err := exec.Command(binary, args...).CombinedOutput(); err == nil {
			t.Fatalf("accepted %v: %s", args, out)
		}
	}
}

func TestOrderedModulesAndFragments(t *testing.T) {
	containerOnly(t)
	trace := filepath.Join(t.TempDir(), "trace")
	script := generate(t, filepath.Join(root, "tests/fixtures/ordered.yaml"))
	// A command double records package calls between real exec_cmd entries.
	bin := t.TempDir()
	apt := filepath.Join(bin, "apt-get")
	write(t, apt, "#!/bin/bash\nprintf 'apt-%s\\n' \"$*\" >> \"$TRACE\"\n")
	if err := os.Chmod(apt, 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := runScript(script, "TRACE="+trace, "PATH="+bin+":"+os.Getenv("PATH"))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := "main-1\nmain-2\napt-clean\nmain-3\napt-clean\nfirst\nnested\nsecond\nsecond\n"
	if got := read(t, trace); got != want {
		t.Fatalf("order:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(string(out), "Configuration completed") {
		t.Fatalf("missing completion log: %s", out)
	}
}

func TestFailFast(t *testing.T) {
	containerOnly(t)
	for _, tc := range []struct{ name, command string }{{"exit-status", "exit 23"}, {"semicolon", "false; echo unreachable"}, {"pipeline", "false | cat"}} {
		t.Run(tc.name, func(t *testing.T) {
			command := tc.command
			content := read(t, filepath.Join(root, "tests/fixtures/fail-fast.yaml"))
			content = strings.Replace(content, "exit 23", command, 1)
			// Keep includes relative to a copied main file.
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "fragments"), 0o700); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(dir, "fragments/second.yaml"), read(t, filepath.Join(root, "tests/fixtures/fragments/second.yaml")))
			input := filepath.Join(dir, "main.yaml")
			write(t, input, content)
			trace := filepath.Join(dir, "trace")
			out, err := runScript(generate(t, input), "TRACE="+trace)
			if err == nil {
				t.Fatalf("expected failure: %s", out)
			}
			if command == "exit 23" && err.(*exec.ExitError).ExitCode() != 23 {
				t.Fatalf("lost exit status: %v", err)
			}
			if got := read(t, trace); got != "before\n" {
				t.Fatalf("continued after failure: %q", got)
			}
			if !strings.Contains(string(out), "ERROR:") || !strings.Contains(string(out), "exec_cmd") || strings.Contains(string(out), "unreachable") || strings.Contains(string(out), "Configuration completed") {
				t.Fatalf("incorrect failure log: %s", out)
			}
		})
	}
}

func TestRootRequired(t *testing.T) {
	script := generate(t, manifest(t, "exec_cmd: [echo should-not-run]\n"))
	if os.Geteuid() != 0 {
		out, err := runScript(script)
		if err == nil || !strings.Contains(string(out), "must run as root") {
			t.Fatalf("%v: %s", err, out)
		}
		return
	}
	containerOnly(t)
	// Put a readable copy outside Go's private temporary directories.
	dir, err := os.MkdirTemp("/tmp", "buildos-nonroot-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "script.sh")
	write(t, path, read(t, script))
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("su", "-s", "/bin/bash", "nobody", "-c", "bash "+path).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "must run as root") || strings.Contains(string(out), "should-not-run") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestChecksumFailure(t *testing.T) {
	containerOnly(t)
	work := t.TempDir()
	source, dest := filepath.Join(work, "source"), filepath.Join(work, "dest")
	write(t, source, "wrong content")
	write(t, dest, "existing")
	input := manifest(t, fmt.Sprintf("files:\n  - source: %s\n    destination: %s\n    checksum: sha256:%064d\nexec_cmd: [echo unreachable]\n", source, dest, 0))
	out, err := runScript(generate(t, input))
	if err == nil || strings.Contains(string(out), "unreachable") || !strings.Contains(string(out), "ERROR:") {
		t.Fatalf("%v: %s", err, out)
	}
	if read(t, dest) != "existing" || read(t, source) != "wrong content" {
		t.Fatal("checksum failure changed files")
	}
}

func TestTargetDistributionCheckedAtRuntime(t *testing.T) {
	containerOnly(t)
	input := manifest(t, "base:\n  distribution: debian\n  release: '0'\nexec_cmd: [echo unreachable]\n")
	out, err := runScript(generate(t, input))
	if err == nil || !strings.Contains(string(out), "expected debian 0") || strings.Contains(string(out), "unreachable") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestDownloadsAreDistinctAndCleaned(t *testing.T) {
	containerOnly(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, r.URL.Path) }))
	defer server.Close()
	work := t.TempDir()
	bin := filepath.Join(work, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	// Snap/docker are not normally available in containers. Exercise their real
	// generated invocations against doubles that inspect the downloaded content.
	write(t, filepath.Join(bin, "snap"), "#!/bin/bash\nset -eu\ncase $1 in\nack) test \"$(cat \"$2\")\" = /assert ;;\ninstall) test \"$(cat \"$2\")\" = /snap ;;\nesac\nprintf '%s\\n' \"$2\" >> \"$TRACE\"\n")
	write(t, filepath.Join(bin, "docker"), "#!/bin/bash\nset -eu\ntest \"$1 $2\" = 'load -i'\ntest \"$(cat \"$3\")\" = /archive\nprintf '%s\\n' \"$3\" >> \"$TRACE\"\n")
	for _, name := range []string{"snap", "docker"} {
		if err := os.Chmod(filepath.Join(bin, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	input := manifest(t, fmt.Sprintf("snaps:\n  - assert: %s/assert\n    snap: %s/snap\ncontainers:\n  - name: test\n    source: %s/archive\n    checksum: sha256:%x\n", server.URL, server.URL, server.URL, sha256.Sum256([]byte("/archive"))))
	trace := filepath.Join(work, "trace")
	out, err := runScript(generate(t, input), "TRACE="+trace, "PATH="+bin+":"+os.Getenv("PATH"))
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	paths := strings.Fields(read(t, trace))
	if len(paths) != 3 || paths[0] == paths[1] || paths[1] == paths[2] {
		t.Fatalf("downloads collide: %v", paths)
	}
	for _, path := range paths {
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatalf("temporary directory remains: %s (%v)", path, err)
		}
	}
}

func TestModuleCommandFailure(t *testing.T) {
	containerOnly(t)
	bin := t.TempDir()
	apt := filepath.Join(bin, "apt-get")
	write(t, apt, "#!/bin/bash\nexit 42\n")
	if err := os.Chmod(apt, 0o700); err != nil {
		t.Fatal(err)
	}
	input := manifest(t, "packages:\n  install: [example]\nexec_cmd: [echo unreachable]\n")
	out, err := runScript(generate(t, input), "PATH="+bin+":"+os.Getenv("PATH"))
	if err == nil || err.(*exec.ExitError).ExitCode() != 42 || !strings.Contains(string(out), "packages") || !strings.Contains(string(out), "ERROR:") || strings.Contains(string(out), "unreachable") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestLiteralArguments(t *testing.T) {
	containerOnly(t)
	bin := t.TempDir()
	apt := filepath.Join(bin, "apt-get")
	write(t, apt, "#!/bin/bash\nprintf '%s\\n' \"$@\" >> \"$TRACE\"\n")
	if err := os.Chmod(apt, 0o700); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	marker := filepath.Join(work, "injected")
	value := "name's $(touch " + marker + ") `false` ; *"
	// Double-quoted YAML still leaves shell metacharacters literal.
	input := manifest(t, fmt.Sprintf("packages:\n  install: [%q]\n", value))
	trace := filepath.Join(work, "trace")
	out, err := runScript(generate(t, input), "TRACE="+trace, "PATH="+bin+":"+os.Getenv("PATH"))
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := read(t, trace); got != "update\ninstall\n-y\n"+value+"\n" {
		t.Fatalf("arguments changed: %q", got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("argument executed: %v", err)
	}
}

func TestDownloadFailureCleansTemporaryFiles(t *testing.T) {
	containerOnly(t)
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	tmp := t.TempDir()
	input := manifest(t, fmt.Sprintf("snaps:\n  - assert: %s/missing\n    snap: %s/snap\nexec_cmd: [echo unreachable]\n", server.URL, server.URL))
	out, err := runScript(generate(t, input), "TMPDIR="+tmp)
	if err == nil || !strings.Contains(string(out), "ERROR:") || strings.Contains(string(out), "unreachable") {
		t.Fatalf("%v: %s", err, out)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary files remain: %v, %v", entries, err)
	}
}

func TestSymlinkIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "main.yaml")
	write(t, input, "base:\n  include: [alias.yaml]\n")
	if err := os.Symlink(input, filepath.Join(dir, "alias.yaml")); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(binary, "--config", input, "--output", filepath.Join(dir, "script.sh")).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "include cycle") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestBaseProbeAndValidation(t *testing.T) {
	containerOnly(t)
	original := read(t, "/etc/os-release")
	defer write(t, "/etc/os-release", original)
	for _, tc := range []struct {
		name, osRelease, config string
		success                 bool
	}{
		{"matching-repeated-base", "ID=debian\nVERSION_ID=12\n", "base: {distribution: debian, release: '12'}\nexec_cmd: [echo reached]\nbase: {distribution: debian, release: '12'}\n", true},
		{"ubuntu-without-base", "ID=ubuntu\nVERSION_ID=24.04\n", "exec_cmd: [echo reached]\n", true},
		{"unsupported-without-base", "ID=fedora\nVERSION_ID=40\n", "exec_cmd: [echo reached]\n", false},
		{"missing-id", "VERSION_ID=12\n", "exec_cmd: [echo reached]\n", false},
		{"release-mismatch", "ID=debian\nVERSION_ID=12\n", "base: {distribution: debian, release: '13'}\nexec_cmd: [echo reached]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace := filepath.Join(t.TempDir(), "probe")
			// os-release is a sourced shell file. Record how often it is read.
			write(t, "/etc/os-release", tc.osRelease+"printf 'probe\\n' >> \"$PROBE_TRACE\"\n")
			out, err := runScript(generate(t, manifest(t, tc.config)), "PROBE_TRACE="+trace, "ID=ubuntu", "VERSION_ID=24.04")
			if (err == nil) != tc.success {
				t.Fatalf("success=%v: %v: %s", tc.success, err, out)
			}
			if read(t, trace) != "probe\n" {
				t.Fatal("OS information was read more than once")
			}
			if strings.Contains(string(out), "reached") != tc.success {
				t.Fatalf("incorrect module execution: %s", out)
			}
			if !tc.success && !strings.Contains(string(out), "ERROR:") {
				t.Fatalf("missing error log: %s", out)
			}
		})
	}
}

func TestFragmentPathsRelativeToIncludingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(target, "main.yaml")
	write(t, main, "base:\n  include: [fragment.yaml]\n")
	alias := filepath.Join(dir, "alias.yaml")
	if err := os.Symlink(main, alias); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "fragment.yaml"), "exec_cmd: [echo sibling-of-alias]\n")
	script := generate(t, alias)
	if !strings.Contains(read(t, script), "sibling-of-alias") {
		t.Fatal("fragment was not resolved relative to the including path")
	}
}
