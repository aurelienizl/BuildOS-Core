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

	"buildos/src/config"
)

// The runner starts a new container for each tests/profile/<module>.yaml.
func TestModule(t *testing.T) {
	containerOnly(t)
	name := os.Getenv("BUILDOS_MODULE")
	if name == "" {
		t.Skip("the container runner selects one module per container")
	}
	work := t.TempDir()
	profile := read(t, filepath.Join(root, "tests/profile", name+".yaml"))
	content := "hello\n"
	url := ""
	var verify func()
	command := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
		return string(out)
	}
	switch name {
	case "base":
		osRelease := command("bash", "-c", ". /etc/os-release; printf '%s %s' \"$ID\" \"$VERSION_ID\"")
		fields := strings.Fields(osRelease)
		if len(fields) != 2 {
			t.Fatalf("invalid os-release: %q", osRelease)
		}
		profile = strings.NewReplacer("__DISTRIBUTION__", fields[0], "__RELEASE__", fields[1]).Replace(profile)
		verify = func() {} // Successful application proves the actual OS matched.
	case "packages":
		if _, err := exec.LookPath("hello"); err == nil {
			t.Fatal("package already installed before profile application")
		}
		verify = func() {
			if command("dpkg-query", "-W", "-f=${Status}", "hello") != "install ok installed" {
				t.Fatal("hello is not installed")
			}
			if !strings.Contains(command("hello"), "Hello") {
				t.Fatal("installed application failed")
			}
		}
	case "services":
		// systemctl supports persistent enable/mask operations without a running PID 1.
		unit := "[Unit]\nDescription=BuildOS test\n[Service]\nExecStart=/bin/true\n[Install]\nWantedBy=multi-user.target\n"
		for _, service := range []string{"buildos-test", "buildos-masked"} {
			write(t, "/lib/systemd/system/"+service+".service", unit)
		}
		verify = func() {
			if strings.TrimSpace(command("systemctl", "is-enabled", "buildos-test.service")) != "enabled" {
				t.Fatal("service was not enabled")
			}
			target, err := os.Readlink("/etc/systemd/system/buildos-masked.service")
			if err != nil || target != "/dev/null" {
				t.Fatalf("service was not masked: %q, %v", target, err)
			}
		}
	case "kernel":
		verify = func() {
			if got := read(t, "/etc/sysctl.d/99-buildos.conf"); got != "# buildos managed\nnet.ipv4.ip_forward = 1\nvm.swappiness = 10\n" {
				t.Fatalf("kernel configuration: %q", got)
			}
		}
	case "files", "files_base64", "files_text":
		write(t, filepath.Join(work, "source"), content)
		if name == "files_text" {
			content = "hello 'world' \"quoted\" $HOME $(touch " + work + "/injected) `false` %s \\c\nCafé — BuildOS\n\n"
			write(t, filepath.Join(work, "empty"), "must be truncated")
		}
		verify = func() {
			dest := filepath.Join(work, "output/file")
			if got := read(t, dest); got != content {
				t.Fatalf("file bytes: %q; want %q", got, content)
			}
			if strings.TrimSpace(command("stat", "-c", "%U:%G %a", dest)) != "nobody:nogroup 640" {
				t.Fatal("incorrect ownership or permissions")
			}
			if read(t, filepath.Join(work, "output/cwd")) != filepath.Join(work, "output")+"\n" {
				t.Fatal("post-write command ran in the wrong directory")
			}
			if name == "files" && read(t, filepath.Join(work, "source")) != content {
				t.Fatal("source changed")
			}
			if name == "files_text" {
				if read(t, filepath.Join(work, "empty")) != "" || read(t, filepath.Join(work, "no-newline")) != "hello" {
					t.Fatal("empty content or trailing newline changed")
				}
				if strings.TrimSpace(command("stat", "-c", "%U:%G %a", filepath.Join(work, "empty"))) != "root:root 644" {
					t.Fatal("incorrect default file attributes")
				}
				if _, err := os.Stat(filepath.Join(work, "injected")); !os.IsNotExist(err) {
					t.Fatal("file content was executed")
				}
			}
		}
	case "users":
		if err := exec.Command("id", "buildos_test").Run(); err == nil {
			t.Fatal("user exists before applying profile")
		}
		verify = func() {
			if !strings.HasSuffix(strings.TrimSpace(command("getent", "passwd", "buildos_test")), ":/home/buildos_test:/bin/sh") {
				t.Fatal("user home or shell differs")
			}
			groups := " " + strings.TrimSpace(command("id", "-nG", "buildos_test")) + " "
			if !strings.Contains(groups, " adm ") || !strings.Contains(groups, " sudo ") {
				t.Fatal("missing user groups")
			}
			if !strings.HasPrefix(command("getent", "shadow", "buildos_test"), "buildos_test:!:") {
				t.Fatal("password was not set")
			}
			if info, err := os.Stat("/home/buildos_test"); err != nil || !info.IsDir() {
				t.Fatal("home directory missing")
			}
		}
	case "exec_cmd":
		verify = func() {
			if read(t, filepath.Join(work, "output")) != content {
				t.Fatal("command did not write expected output")
			}
		}
	case "snaps", "containers":
		// Daemon integration is intentionally a command-contract test. Neither
		// snapd nor a nested Docker daemon is available in an ordinary container.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, r.URL.Path)
		}))
		defer server.Close()
		url = server.URL
		content = "/archive"
		bin := filepath.Join(work, "bin")
		if err := os.Mkdir(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		tool, body, want := "docker", `test "$1 $2" = 'load -i'
test "$(cat "$3")" = /archive
printf '%s\n' "$*" >> "$TRACE"
`, "load -i "
		if name == "snaps" {
			tool, body, want = "snap", `case "$1" in
ack) test "$(cat "$2")" = /assert ;;
install) test "$(cat "$2")" = /snap ;;
*) exit 1 ;;
esac
printf '%s\n' "$*" >> "$TRACE"
`, "ack "
		}
		path := filepath.Join(bin, tool)
		write(t, path, "#!/bin/bash\nset -euo pipefail\n"+body)
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
		t.Setenv("TRACE", filepath.Join(work, "trace"))
		verify = func() {
			trace := read(t, filepath.Join(work, "trace"))
			if !strings.HasPrefix(trace, want) || (name == "snaps" && !strings.Contains(trace, "\ninstall ")) {
				t.Fatalf("missing module invocations: %q", trace)
			}
			for _, line := range strings.Split(strings.TrimSpace(trace), "\n") {
				args := strings.Fields(line)
				if _, err := os.Stat(args[len(args)-1]); !os.IsNotExist(err) {
					t.Fatal("download was not cleaned up")
				}
			}
		}
	default:
		t.Fatalf("module %q has no verifier", name)
	}
	profile = strings.NewReplacer("__WORK__", work, "__URL__", url, "__CHECKSUM__", fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))).Replace(profile)
	input := manifest(t, profile)
	cfg, err := config.Load(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Modules) == 0 {
		t.Fatal("empty module profile")
	}
	for _, module := range cfg.Modules {
		if module.Name != name {
			t.Fatalf("profile for %s also configures %s", name, module.Name)
		}
	}
	script := generate(t, input)
	// Verify first application and rerun, within this module's container only.
	for i := 0; i < 2; i++ {
		if out, err := runScript(script); err != nil {
			t.Fatalf("application %d: %v\n%s", i+1, err, out)
		}
		verify()
	}
}
