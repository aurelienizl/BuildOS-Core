package files

import (
	"fmt"
	"path"
	"strings"

	"buildos/src/config"
	"buildos/src/ops"
	"buildos/src/util"
)

func Apply(entries []config.FileEntry, o *ops.Ops) error {
	for _, e := range entries {
		if e.Source == "" {
			return fmt.Errorf("file %q requires source", e.Name)
		}
		if err := Write(e.FileTarget, o, func() error {
			local, err := o.Fetch(e.Source)
			if err != nil {
				return err
			}
			if err := o.Sha256Compare(local, e.Checksum); err != nil {
				return err
			}
			return o.Copy(local, e.Destination)
		}); err != nil {
			return err
		}
	}
	return nil
}

// Write shares target validation, ownership, permissions and post-write commands
// between source files and inline content. The callback only supplies the bytes.
func Write(target config.FileTarget, o *ops.Ops, write func() error) error {
	if !path.IsAbs(target.Destination) {
		return fmt.Errorf("file %q requires an absolute destination", target.Name)
	}
	mode, err := util.NormalizeMode(target.Mode)
	if err != nil {
		return err
	}
	if target.Owner == "" {
		target.Owner = "root"
	}
	if target.Group == "" {
		target.Group = "root"
	}
	if err := write(); err != nil {
		return fmt.Errorf("file %q: %w", target.Name, err)
	}
	if err := o.Chown(target.Destination, target.Owner, target.Group); err != nil {
		return err
	}
	if err := o.Chmod(target.Destination, mode); err != nil {
		return err
	}
	if strings.TrimSpace(target.Exec) != "" {
		return o.ShellDir(path.Dir(target.Destination), target.Exec)
	}
	return nil
}
