package containers

import (
	"fmt"
	"strings"

	"buildos/src/config"
	"buildos/src/ops"
)

func Apply(entries []config.ContainerEntry, o *ops.Ops) error {
	for _, e := range entries {
		if e.Name == "" || e.Source == "" || e.Checksum == "" {
			return fmt.Errorf("container entry must have name, source, and checksum: %+v", e)
		}
		path, err := o.Fetch(e.Source)
		if err != nil {
			return fmt.Errorf("failed to fetch container %s: %w", e.Name, err)
		}
		if err := o.Sha256Compare(path, e.Checksum); err != nil {
			return fmt.Errorf("checksum verification failed for container %s: %w", e.Name, err)
		}
		var cmd = []string{"docker", "load", "-i", path}
		if err := o.Run(cmd...); err != nil {
			return fmt.Errorf("failed to load container %s: %w", e.Name, err)
		}
		if strings.TrimSpace(e.Exec) != "" {
			if err := o.Shell(e.Exec); err != nil {
				return fmt.Errorf("failed to execute command for container %s: %w", e.Name, err)
			}
		}
	}
	return nil
}
