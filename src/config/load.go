package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	var c Config
	path, err := filepath.Abs(path)
	if err != nil {
		return Config{}, err
	}
	envPath := filepath.Join(filepath.Dir(path), ".env")
	values, exists, err := readEnv(envPath)
	if err != nil {
		return Config{}, err
	}
	if exists {
		c.Sources = append(c.Sources, envPath)
	}
	if err := c.load(path, make(map[string]bool), values); err != nil {
		return Config{}, err
	}
	return c, nil
}

// load appends one file, then recursively appends its fragments in order.
// Only the active recursion branch is tracked: repeated includes are valid.
func (c *Config) load(path string, active map[string]bool, values map[string]string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if active[canonical] {
		return fmt.Errorf("include cycle at %s", path)
	}
	active[canonical] = true
	defer delete(active, canonical)

	modules, includes, err := readModules(path, values)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for i := range modules {
		modules[i].Source = path
	}
	c.Modules = append(c.Modules, modules...)
	c.Sources = append(c.Sources, path)
	for _, include := range includes {
		if err := c.load(filepath.Join(filepath.Dir(path), include), active, values); err != nil {
			return fmt.Errorf("%s: include %q: %w", path, include, err)
		}
	}
	return nil
}

// readModules parses one document without following any includes.
func readModules(path string, values map[string]string) ([]Module, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := decode(b, &doc); err != nil {
		return nil, nil, err
	}
	if len(doc.Content) != 1 {
		return nil, nil, fmt.Errorf("expected a configuration mapping")
	}
	root := doc.Content[0]
	if root.Kind == yaml.MappingNode && len(root.Content) == 2 && root.Content[0].Value == "buildos" {
		root = root.Content[1]
	}
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("expected a configuration mapping")
	}

	var modules []Module
	var includes []string
	seen := make(map[*yaml.Node]bool)
	versionSeen := false
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, nil, fmt.Errorf("line %d: expected a module name", key.Line)
		}
		if key.Value == "version" {
			if versionSeen {
				return nil, nil, fmt.Errorf("line %d: duplicate version", key.Line)
			}
			if err := validateVersion(value); err != nil {
				return nil, nil, err
			}
			versionSeen = true
			continue
		}
		if value.Tag == "!!null" || value.Kind == yaml.AliasNode {
			return nil, nil, fmt.Errorf("line %d: module %s must have a value (aliases are unsupported)", key.Line, key.Value)
		}
		interpolated, err := interpolate(value, values, seen)
		if err != nil {
			return nil, nil, err
		}
		if key.Value == "base" {
			var base BaseConfig
			if err := Decode(value, &base); err != nil {
				return nil, nil, fmt.Errorf("line %d: base: %w", key.Line, valueError(err, interpolated))
			}
			for _, include := range base.Include {
				if include == "" || filepath.IsAbs(include) {
					return nil, nil, fmt.Errorf("line %d: include must be a non-empty relative path", key.Line)
				}
			}
			includes = append(includes, base.Include...)
		}
		modules = append(modules, Module{Name: key.Value, Value: value, Line: key.Line, Interpolated: interpolated})
	}
	return modules, includes, nil
}

func validateVersion(node *yaml.Node) error {
	var version int
	if node.Tag != "!!int" {
		return fmt.Errorf("line %d: version must be an integer", node.Line)
	}
	if err := Decode(node, &version); err != nil {
		return err
	}
	if version != 0 && version != 1 {
		return fmt.Errorf("line %d: unsupported config version: %d", node.Line, version)
	}
	return nil
}

// Decode retains strict field checking inside each module.
func Decode(node *yaml.Node, out any) error {
	b, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	return decode(b, out)
}

func decode(b []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one YAML document")
	}
	return nil
}
