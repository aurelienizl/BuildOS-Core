package services

import (
	"fmt"

	"buildos/src/config"
	"buildos/src/ops"
)

func Apply(cfg config.ServicesConfig, o *ops.Ops) error {
	for _, s := range cfg.Mask {
		if err := o.Run("systemctl", "mask", s); err != nil {
			return fmt.Errorf("failed to mask service %s: %w", s, err)
		}
	}
	for _, s := range cfg.UnMask {
		if err := o.Run("systemctl", "unmask", s); err != nil {
			return fmt.Errorf("failed to unmask service %s: %w", s, err)
		}
	}
	for _, s := range cfg.Enable {
		if err := o.Run("systemctl", "enable", s); err != nil {
			return fmt.Errorf("failed to enable service %s: %w", s, err)
		}
	}
	for _, s := range cfg.Disable {
		if err := o.Run("systemctl", "disable", s); err != nil {
			return fmt.Errorf("failed to disable service %s: %w", s, err)
		}
	}
	for _, s := range cfg.Start {
		if err := o.Run("systemctl", "start", s); err != nil {
			return fmt.Errorf("failed to start service %s: %w", s, err)
		}
	}
	for _, s := range cfg.Stop {
		if err := o.Run("systemctl", "stop", s); err != nil {
			return fmt.Errorf("failed to stop service %s: %w", s, err)
		}
	}

	for _, service := range cfg.Restart {
		if err := o.Run("systemctl", "restart", service); err != nil {
			return err
		}
	}
	for _, service := range cfg.Reload {
		if err := o.Run("systemctl", "reload", service); err != nil {
			return err
		}
	}

	return nil
}
