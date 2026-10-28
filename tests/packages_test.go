package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagePurge(t *testing.T) {
	containerOnly(t)
	script := generate(t, filepath.Join(root, "tests/fixtures/packages-purge.yaml"))
	if out, err := runScript(script); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestPurgeFailureStopsScript(t *testing.T) {
	containerOnly(t)
	bin := t.TempDir()
	write(t, filepath.Join(bin, "dpkg-query"), "#!/bin/bash\nprintf installed\n")
	write(t, filepath.Join(bin, "apt-get"), "#!/bin/bash\nexit 53\n")
	for _, name := range []string{"dpkg-query", "apt-get"} {
		if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := generate(t, manifest(t, "packages:\n  purge: [example]\nexec_cmd: [echo unreachable]\n"))
	out, err := runScript(script, "PATH="+bin+":"+os.Getenv("PATH"))
	if err == nil || err.(*exec.ExitError).ExitCode() != 53 || strings.Contains(string(out), "unreachable") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestServiceRestartAndReload(t *testing.T) {
	containerOnly(t)
	bin := t.TempDir()
	write(t, filepath.Join(bin, "systemctl"), "#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"$TRACE\"\n")
	if err := os.Chmod(filepath.Join(bin, "systemctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "trace")
	script := generate(t, manifest(t, "services:\n  start: [example]\n  restart: [example]\n  reload: [example]\n"))
	if out, err := runScript(script, "PATH="+bin+":"+os.Getenv("PATH"), "TRACE="+trace); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := read(t, trace); got != "start example\nrestart example\nreload example\n" {
		t.Fatalf("service operations: %q", got)
	}
}

func TestPackageExtraArgs(t *testing.T) {
	containerOnly(t)
	bin := t.TempDir()
	write(t, filepath.Join(bin, "apt-get"), "#!/bin/bash\nprintf '%s\\0' \"$@\" >> \"$TRACE\"\nprintf '\\n' >> \"$TRACE\"\n")
	write(t, filepath.Join(bin, "dpkg-query"), "#!/bin/bash\nprintf installed\n")
	for _, name := range []string{"apt-get", "dpkg-query"} {
		if err := os.Chmod(filepath.Join(bin, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	marker := filepath.Join(work, "injected")
	value := "Example::Value=name's $(touch " + marker + ") `false` ; *"
	input := manifest(t, fmt.Sprintf(`packages:
  extra_args: [-o, 'Acquire::http::Proxy=http://127.0.0.1:3142', -o, %q]
  install: [hello]
  remove: [old]
  purge: [obsolete]
  upgrade: true
  autoremove: true
  clean: true
packages:
  clean: true
packages:
  extra_args: []
  clean: true
`, value))
	trace := filepath.Join(work, "trace")
	if out, err := runScript(generate(t, input), "TRACE="+trace, "PATH="+bin+":"+os.Getenv("PATH")); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	prefix := "-o\x00Acquire::http::Proxy=http://127.0.0.1:3142\x00-o\x00" + value + "\x00"
	var want strings.Builder
	for _, args := range []string{
		"update\x00", "install\x00-y\x00hello\x00", "remove\x00-y\x00old\x00",
		"purge\x00-y\x00--\x00obsolete\x00", "upgrade\x00-y\x00", "autoremove\x00-y\x00", "clean\x00",
	} {
		want.WriteString(prefix + args + "\n")
	}
	want.WriteString("clean\x00\nclean\x00\n")
	if got := read(t, trace); got != want.String() {
		t.Fatalf("package arguments: %q; want %q", got, want.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("extra argument executed: %v", err)
	}
}
