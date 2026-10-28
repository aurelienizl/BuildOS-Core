// Package ops emits shell operations; it never executes target commands.
package ops

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var sha256Re = regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`)

type Ops struct {
	script    strings.Builder
	downloads map[string]string
}

func New() *Ops {
	o := &Ops{downloads: make(map[string]string)}
	o.Raw(`#!/bin/bash
set -Eeuo pipefail
if (( EUID != 0 )); then
    printf '%s\n' '[buildos] ERROR: must run as root' >&2
    exit 1
fi
buildos_step='initialization'
buildos_log() { printf '[buildos] %s\n' "$*" >&2; }
trap 'buildos_status=$?; buildos_log "ERROR: $buildos_step (line $LINENO, exit $buildos_status)"; exit "$buildos_status"' ERR
buildos_tmp=$(mktemp -d)
trap 'rm -rf -- "$buildos_tmp"' EXIT
export DEBIAN_FRONTEND=noninteractive
`)
	return o
}

// Quote treats configuration values as literal shell arguments.
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func (o *Ops) argument(s string) string {
	if expression, ok := o.downloads[s]; ok {
		return expression
	}
	return Quote(s)
}

func (o *Ops) Raw(s string)   { o.script.WriteString(s); o.script.WriteByte('\n') }
func (o *Ops) String() string { return o.script.String() }
func (o *Ops) Step(label string) {
	o.Raw("\nbuildos_step=" + Quote(label))
	o.Raw(`buildos_log "$buildos_step"`)
}

func (o *Ops) Run(cmd ...string) error {
	if len(cmd) == 0 {
		return nil
	}
	// Do not print arguments: password hashes and other values can be sensitive.
	o.Raw("buildos_log " + Quote("RUN: "+cmd[0]))
	args := make([]string, len(cmd))
	for i, arg := range cmd {
		args[i] = o.argument(arg)
	}
	o.Raw(strings.Join(args, " "))
	return nil
}

func (o *Ops) Shell(s string) error { return o.ShellDir("", s) }
func (o *Ops) ShellDir(dir, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("shell command must not be empty")
	}
	// Each entry has its own strict shell, so a failed command or pipeline stops it.
	if dir != "" {
		o.Raw("(\ncd -- " + Quote(dir))
	}
	err := o.Run("bash", "-Eeuo", "pipefail", "-c", s)
	if dir != "" {
		o.Raw(")")
	}
	return err
}

func (o *Ops) MkdirAll(path string) error { return o.Run("mkdir", "-p", "--", path) }
func (o *Ops) WriteFile(path string, lines []string, header string) error {
	if err := o.MkdirAll(filepath.Dir(path)); err != nil {
		return err
	}
	var content string
	if header != "" {
		content = header + "\n"
	}
	for _, line := range lines {
		content += line + "\n"
	}
	o.Raw("printf '%s' " + Quote(content) + " > " + Quote(path))
	return nil
}

// WriteBytes emits decoded text literally and binary data as octal escapes.
// Both forms preserve trailing newlines and require only Bash's printf builtin.
func (o *Ops) WriteBytes(path string, content []byte) error {
	if err := o.MkdirAll(filepath.Dir(path)); err != nil {
		return err
	}
	format, value := "%s", string(content)
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		var escaped strings.Builder
		for _, b := range content {
			fmt.Fprintf(&escaped, "\\%03o", b)
		}
		format, value = "%b", escaped.String()
	}
	o.Raw("buildos_log " + Quote("WRITE: "+path))
	// Limit access while writing a new file; the shared file handler sets its final mode.
	o.Raw("(umask 077; printf " + Quote(format) + " " + Quote(value) + " > " + Quote(path) + ")")
	return nil
}

func (o *Ops) Fetch(source string) (string, error) {
	if source == "" {
		return "", fmt.Errorf("source must not be empty")
	}
	if !strings.HasPrefix(source, "https://") && !strings.HasPrefix(source, "http://") {
		return source, nil
	}
	// An internal token lets Run quote the runtime temporary path correctly.
	token := fmt.Sprintf("\x00download-%d", len(o.downloads)+1)
	o.downloads[token] = fmt.Sprintf(`"$buildos_tmp/resource-%d"`, len(o.downloads)+1)
	if err := o.Run("wget", "--timeout=30", "--tries=1", "-O", token, "--", source); err != nil {
		return "", err
	}
	return token, nil
}

func (o *Ops) Copy(src, dst string) error {
	if src == dst {
		return nil
	}
	if err := o.MkdirAll(filepath.Dir(dst)); err != nil {
		return err
	}
	return o.Run("cp", "--", src, dst)
}
func (o *Ops) Chown(path, owner, group string) error {
	return o.Run("chown", owner+":"+group, "--", path)
}
func (o *Ops) Chmod(path, mode string) error { return o.Run("chmod", mode, "--", path) }
func (o *Ops) Sha256Compare(path, checksum string) error {
	if !sha256Re.MatchString(checksum) {
		return fmt.Errorf("invalid checksum format: %s", checksum)
	}
	expected := strings.ToLower(strings.TrimPrefix(checksum, "sha256:"))
	o.Raw("buildos_log 'RUN: sha256sum'")
	o.Raw("buildos_checksum=$(sha256sum < " + o.argument(path) + ")")
	o.Raw("[[ ${buildos_checksum%% *} == " + Quote(expected) + " ]]")
	return nil
}
