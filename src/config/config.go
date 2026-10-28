package config

import "gopkg.in/yaml.v3"

// Module preserves each occurrence, including repeated module names.
type Module struct {
	Name         string
	Value        *yaml.Node
	Source       string
	Line         int
	Interpolated bool
}

func (m Module) ValidationError(err error) error { return valueError(err, m.Interpolated) }

type Config struct {
	Modules []Module
	Sources []string
}

type BaseConfig struct {
	Include      []string `yaml:"include"`
	Distribution string   `yaml:"distribution"` // e.g. "ubuntu", "debian"
	Release      string   `yaml:"release"`      // e.g. "24.04"
}

type PackagesConfig struct {
	ExtraArgs  []string `yaml:"extra_args"` // Extra apt-get arguments, scoped to this occurrence.
	Install    []string `yaml:"install"`    // Package names to install
	Purge      []string `yaml:"purge"`      // Remove installed packages and remaining configuration.
	Remove     []string `yaml:"remove"`     // Package names to remove
	Autoremove bool     `yaml:"autoremove"` // Whether to run autoremove after package changes
	Upgrade    bool     `yaml:"upgrade"`    // Whether to upgrade all packages to the latest version
	Clean      bool     `yaml:"clean"`      // Whether to clean apt cache after package changes (e.g. apt-get clean)
}

type ServicesConfig struct {
	Enable  []string `yaml:"enable"`  // Service names to enable
	Disable []string `yaml:"disable"` // Service names to disable
	Start   []string `yaml:"start"`   // Service names to start
	Restart []string `yaml:"restart"` // Service names to restart
	Reload  []string `yaml:"reload"`  // Service names to reload
	Stop    []string `yaml:"stop"`    // Service names to stop
	Mask    []string `yaml:"mask"`    // Service names to mask
	UnMask  []string `yaml:"unmask"`  // Service names to unmask
}

type KernelConfig struct {
	Sysctl map[string]any `yaml:"sysctl"` // Sysctl key-value pairs
}

// FileTarget is shared by files copied from a source and inline files.
type FileTarget struct {
	Name        string `yaml:"name"`
	Destination string `yaml:"destination"`
	Owner       string `yaml:"owner"`
	Group       string `yaml:"group"`
	Mode        any    `yaml:"mode"`
	Exec        string `yaml:"exec"`
}

type FileEntry struct {
	FileTarget `yaml:",inline"`
	Source     string `yaml:"source"`
	Checksum   string `yaml:"checksum"`
}

type Base64FileEntry struct {
	FileTarget `yaml:",inline"`
	// A pointer distinguishes an empty file from a missing required field.
	Content *string `yaml:"content"`
}

type TextFileEntry struct {
	FileTarget `yaml:",inline"`
	// A pointer distinguishes an empty file from missing or null content.
	Content *string `yaml:"content"`
}

type SnapEntry struct {
	Name   string `yaml:"name"`   // User defined
	Assert string `yaml:"assert"` // Assert file path or URL
	Snap   string `yaml:"snap"`   // Snap file path or URL
}

type ContainerEntry struct {
	Name     string `yaml:"name"`     // User defined
	Source   string `yaml:"source"`   // Container .tar.gz path or URL
	Checksum string `yaml:"checksum"` // Mandatory SHA256 checksum of the archive
	Exec     string `yaml:"exec"`     // Optional shell command after loading the image
}

type UserEntry struct {
	Username     string   `yaml:"username"`      // User name
	PasswordHash string   `yaml:"password_hash"` // Mandatory password hash (e.g. from mkpasswd --method=SHA-512)
	Shell        string   `yaml:"shell"`         // Default shell
	Groups       []string `yaml:"groups"`        // User groups
	Sudo         bool     `yaml:"sudo"`          // User is a sudoer
}
