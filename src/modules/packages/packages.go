package packages

import (
	"fmt"

	"buildos/src/config"
	"buildos/src/ops"
)

func Apply(cfg config.PackagesConfig, o *ops.Ops) error {
	prefix := append([]string{"apt-get"}, cfg.ExtraArgs...)
	run := func(args ...string) error { return o.Run(append(append([]string(nil), prefix...), args...)...) }

	if len(cfg.Install) > 0 || cfg.Upgrade {
		if err := run("update"); err != nil {
			return fmt.Errorf("failed to update package lists: %w", err)
		}
	}

	if len(cfg.Install) > 0 {
		cmd := append([]string{"install", "-y"}, cfg.Install...)
		if err := run(cmd...); err != nil {
			return fmt.Errorf("failed to install packages: %w", err)
		}
	}
	if len(cfg.Remove) > 0 {
		cmd := append([]string{"remove", "-y"}, cfg.Remove...)
		if err := run(cmd...); err != nil {
			return fmt.Errorf("failed to remove packages: %w", err)
		}
	}
	// Query on the target; obsolete or absent packages should not make purge fail.
	for _, name := range cfg.Purge {
		o.Raw("if buildos_package_status=$(dpkg-query -W -f='${db:Status-Status}' -- " + ops.Quote(name) + " 2>/dev/null) && [[ $buildos_package_status != not-installed ]]; then")
		if err := run("purge", "-y", "--", name); err != nil {
			return err
		}
		o.Raw("fi")
	}
	if cfg.Upgrade {
		if err := run("upgrade", "-y"); err != nil {
			return fmt.Errorf("failed to upgrade packages: %w", err)
		}
	}
	if cfg.Autoremove {
		if err := run("autoremove", "-y"); err != nil {
			return fmt.Errorf("failed to autoremove packages: %w", err)
		}
	}
	if cfg.Clean {
		if err := run("clean"); err != nil {
			return fmt.Errorf("failed to clean packages: %w", err)
		}
	}
	return nil
}
