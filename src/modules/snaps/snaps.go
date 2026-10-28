package snaps

import (
	"fmt"

	"buildos/src/config"
	"buildos/src/ops"
)

func Apply(entries []config.SnapEntry, o *ops.Ops) error {
	for _, e := range entries {
		assertFile, err := o.Fetch(e.Assert)
		if err != nil {
			return fmt.Errorf("failed to fetch snap assertion for %s: %w", e.Name, err)
		}
		snapFile, err := o.Fetch(e.Snap)
		if err != nil {
			return fmt.Errorf("failed to fetch snap file for %s: %w", e.Name, err)
		}
		if err := o.Run("snap", "ack", assertFile); err != nil {
			return fmt.Errorf("failed to acknowledge snap assertion for %s: %w", e.Name, err)
		}
		if err := o.Run("snap", "install", snapFile); err != nil {
			return fmt.Errorf("failed to install snap for %s: %w", e.Name, err)
		}
	}
	return nil
}
