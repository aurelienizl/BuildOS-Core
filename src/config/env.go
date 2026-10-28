package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// readEnv deliberately supports literal, single-line assignments only.
// It never sources the file or consults the process environment.
func readEnv(path string) (map[string]string, bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// A broken symlink exists and must not be mistaken for an absent .env.
		if _, linkErr := os.Lstat(path); os.IsNotExist(linkErr) {
			return nil, false, nil
		}
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%s: .env must be a regular file", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	values := make(map[string]string)
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.ContainsRune(line, 0) {
			return nil, false, fmt.Errorf("%s:%d: NUL bytes are unsupported", path, i+1)
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || !envName.MatchString(name) {
			return nil, false, fmt.Errorf("%s:%d: expected NAME=value", path, i+1)
		}
		if _, exists := values[name]; exists {
			return nil, false, fmt.Errorf("%s:%d: duplicate variable %s", path, i+1, name)
		}
		value = strings.TrimSpace(value)
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			if len(value) < 2 || value[len(value)-1] != value[0] {
				return nil, false, fmt.Errorf("%s:%d: unmatched quotes for %s", path, i+1, name)
			}
			value = value[1 : len(value)-1]
		}
		values[name] = value
	}
	return values, true, nil
}

// interpolate visits values only, preserving keys, YAML types and source lines.
// Alias targets are visited at their definition, never expanded a second time.
func interpolate(node *yaml.Node, values map[string]string, seen map[*yaml.Node]bool) (used bool, err error) {
	if node.Kind == yaml.AliasNode {
		return seen[node.Alias], nil
	}
	defer func() { seen[node] = used }()
	changed := false
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		value, used, err := expandEnv(node.Value, values)
		if err != nil {
			return false, fmt.Errorf("line %d: %w", node.Line, err)
		}
		node.Value = value
		return used, nil
	}
	start, step := 0, 1
	if node.Kind == yaml.MappingNode {
		start, step = 1, 2
	}
	for i := start; i < len(node.Content); i += step {
		used, err := interpolate(node.Content[i], values, seen)
		if err != nil {
			return false, err
		}
		changed = changed || used
	}
	return changed, nil
}

func expandEnv(s string, values map[string]string) (string, bool, error) {
	var out strings.Builder
	used := false
	for {
		i := strings.Index(s, "${env:")
		if i < 0 {
			out.WriteString(s)
			return out.String(), used, nil
		}
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			return "", false, fmt.Errorf("unclosed env placeholder")
		}
		end += i
		name := s[i+6 : end]
		if !envName.MatchString(name) {
			return "", false, fmt.Errorf("invalid env placeholder name")
		}
		if i > 0 && s[i-1] == '$' {
			out.WriteString(s[:i-1])
			out.WriteString(s[i : end+1])
		} else {
			value, ok := values[name]
			if !ok {
				return "", false, fmt.Errorf("undefined env variable %s", name)
			}
			out.WriteString(s[:i])
			out.WriteString(value)
			used = true
		}
		s = s[end+1:]
	}
}

// YAML decoding and module validators may echo a value in an error. Suppress
// those details for interpolated modules rather than trying to redact secrets.
func valueError(err error, interpolated bool) error {
	if err != nil && interpolated {
		return fmt.Errorf("invalid module fields or values (interpolated values omitted)")
	}
	return err
}
