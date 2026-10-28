package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvSecretsAndFragments(t *testing.T) {
	containerOnly(t)
	work := t.TempDir()
	fragments := filepath.Join(work, "fragments")
	if err := os.Mkdir(fragments, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(work, "injected")
	secret := "name's \"quotes\" # $HOME ${HOME} ${env:UNDEFINED} $(touch " + marker + ") `false` ; * \\n : Café"
	hash := "$6$salt$example-hash"
	write(t, filepath.Join(work, ".env"), " # comment\r\n VALUE = \""+secret+"\"\r\nHASH='"+hash+"'\r\nEMPTY=\r\nPLAIN= command-value \r\nFRAGMENT=fragments/users.yaml\r\nUNQUOTED=value#literal\r\nPADDED='  padded  '\r\n")
	// Only the main config's .env is read, even for nested fragments.
	write(t, filepath.Join(fragments, ".env"), "HASH=wrong\nVALUE=wrong\n")
	write(t, filepath.Join(fragments, "users.yaml"), "base: {include: [nested.yaml]}\nusers:\n  - username: buildos_env\n    password_hash: '${env:HASH}'\n")
	write(t, filepath.Join(fragments, "nested.yaml"), fmt.Sprintf("files_text:\n  - destination: %s/nested\n    content: '${env:VALUE}'\n", work))
	input := filepath.Join(work, "config.yaml")
	write(t, input, fmt.Sprintf(`buildos:
  base:
    include: ['${env:FRAGMENT}']
  files_text:
    - destination: %s/main
      mode: '600'
      content: |
        password=${env:VALUE}
        hash=${env:HASH}
        twice=${env:VALUE}
        literal=$${env:MISSING}
        shell=$HOME ${HOME} $(false)
        empty=[${env:EMPTY}]
        unquoted=${env:UNQUOTED}
        padded=${env:PADDED}
    - destination: %s/empty
      content: '${env:EMPTY}'
    - destination: %s/alias
      content: &secret '${env:VALUE}'
    - destination: %s/alias-copy
      content: *secret
  exec_cmd:
    - test '${env:PLAIN}' = command-value; test -n "${HOME}"
`, work, work, work, work))
	script := generate(t, input)
	// The script is self-contained after generation.
	if err := os.Remove(filepath.Join(work, ".env")); err != nil {
		t.Fatal(err)
	}
	want := "password=" + secret + "\nhash=" + hash + "\ntwice=" + secret + "\nliteral=${env:MISSING}\nshell=$HOME ${HOME} $(false)\nempty=[]\nunquoted=value#literal\npadded=  padded  \n"
	for i := 0; i < 2; i++ {
		out, err := runScript(script)
		if err != nil {
			t.Fatalf("application: %v: %s", err, out)
		}
		if strings.Contains(string(out), secret) || strings.Contains(string(out), hash) {
			t.Fatal("operation log exposed a secret")
		}
		for name, expected := range map[string]string{"main": want, "nested": secret, "empty": "", "alias": secret, "alias-copy": secret} {
			if read(t, filepath.Join(work, name)) != expected {
				t.Fatalf("%s: interpolated bytes differ", name)
			}
		}
		shadow, err := exec.Command("getent", "shadow", "buildos_env").Output()
		if err != nil || !strings.HasPrefix(string(shadow), "buildos_env:"+hash+":") {
			t.Fatalf("password hash differs: %v", err)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("secret was executed: %v", err)
	}
}

func TestEnvValidationPreservesOutput(t *testing.T) {
	for _, tc := range []struct{ name, env, value, message string }{
		{"missing-file", "", "${env:NAME}", "undefined env variable NAME"},
		{"missing-variable", "OTHER=present", "${env:NAME}", "undefined env variable NAME"},
		{"empty-name", "NAME=secret", "${env:}", "invalid env placeholder"},
		{"invalid-name", "NAME=secret", "${env:BAD-NAME}", "invalid env placeholder"},
		{"unsupported-default", "NAME=secret", "${env:NAME:-fallback}", "invalid env placeholder"},
		{"unclosed", "NAME=secret", "${env:NAME", "unclosed env placeholder"},
		{"duplicate", "NAME=secret\nNAME=other", "plain", "duplicate variable NAME"},
		{"invalid-assignment", "a secret without equals", "plain", "expected NAME=value"},
		{"invalid-key", "BAD-NAME=secret", "plain", "expected NAME=value"},
		{"export", "export NAME=secret", "plain", "expected NAME=value"},
		{"single-quote", "NAME='secret", "plain", "unmatched quotes"},
		{"double-quote", "NAME=\"secret", "plain", "unmatched quotes"},
		{"multiline", "NAME='secret\nmore'", "plain", "unmatched quotes"},
		{"inline-comment", "NAME='secret' # comment", "plain", "unmatched quotes"},
		{"nul", "NAME=secret\x00", "plain", "NUL bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := manifest(t, "files_text:\n  - destination: /tmp/file\n    content: '"+tc.value+"'\n")
			envPath := filepath.Join(filepath.Dir(input), ".env")
			context := input
			if tc.env != "" {
				write(t, envPath, tc.env)
				if tc.value == "plain" {
					context = envPath + ":"
				}
			}
			t.Setenv("NAME", "process-secret")
			out := rejectGeneration(t, input, context)
			if !strings.Contains(out, tc.message) || strings.Contains(out, "secret") {
				t.Fatalf("incorrect or sensitive diagnostic: %s", out)
			}
			if tc.value != "plain" && !strings.Contains(out, "line 3") {
				t.Fatalf("missing placeholder line: %s", out)
			}
		})
	}
}

func TestEnvModuleDiagnosticsOmitValues(t *testing.T) {
	for _, content := range []string{
		"containers: [{name: app, checksum: '${env:SECRET}'}]",
		"files: [{source: /tmp/source, destination: /tmp/dest, checksum: '${env:SECRET}'}]",
		"users: [{username: '${env:SECRET}', password_hash: present}]",
		"packages: {upgrade: '${env:SECRET}'}",
		"files_text: [{destination: /tmp/file, content: '', mode: '${env:SECRET}'}]",
		"base: {include: '${env:SECRET}'}",
		"base: {include: ['/${env:SECRET}']}",
		"files_text: [{destination: /tmp/file, content: '${env:SECRET}', extra: invalid}]",
	} {
		input := manifest(t, content+"\n")
		write(t, filepath.Join(filepath.Dir(input), ".env"), "SECRET='very-sensitive-$-value'\n")
		out := rejectGeneration(t, input, input)
		if strings.Contains(out, "very-sensitive") || !strings.Contains(out, "line 1") && !strings.Contains(out, ":1:") {
			t.Fatalf("incorrect or sensitive module diagnostic: %s", out)
		}
	}
}

func TestEnvDiscoveryBesideConfigSymlink(t *testing.T) {
	work := t.TempDir()
	target := manifest(t, "files_text: [{destination: /tmp/file, content: '${env:VALUE}'}]\n")
	write(t, filepath.Join(filepath.Dir(target), ".env"), "VALUE=target-value\n")
	alias := filepath.Join(work, "config.yaml")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(work, ".env"), "VALUE=alias-value\n")
	script := read(t, generate(t, alias))
	if !strings.Contains(script, "alias-value") || strings.Contains(script, "target-value") {
		t.Fatal(".env was not resolved beside the provided config path")
	}
}

func TestEnvLiteralEscapeWithoutEnv(t *testing.T) {
	input := manifest(t, "files_text: [{destination: /tmp/file, content: '$${env:UNDEFINED} ${HOME} $HOME'}]\n")
	script := read(t, generate(t, input))
	if !strings.Contains(script, "${env:UNDEFINED} ${HOME} $HOME") || strings.Contains(script, "$${env:") {
		t.Fatal("escaped placeholder or existing shell syntax changed")
	}
}

func TestEnvReadFailure(t *testing.T) {
	for _, name := range []string{"directory", "broken-symlink"} {
		t.Run(name, func(t *testing.T) {
			input := manifest(t, "base: {}\n")
			envPath := filepath.Join(filepath.Dir(input), ".env")
			var err error
			if name == "directory" {
				err = os.Mkdir(envPath, 0o700)
			} else {
				err = os.Symlink("missing-env", envPath)
			}
			if err != nil {
				t.Fatal(err)
			}
			rejectGeneration(t, input, envPath)
		})
	}
}
