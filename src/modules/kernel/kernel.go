package kernel

import (
	"fmt"
	"regexp"
	"sort"

	"buildos/src/config"
	"buildos/src/ops"
)

const sysctlFile = "/etc/sysctl.d/99-buildos.conf"

var sysctlKeyR = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func Apply(cfg config.KernelConfig, o *ops.Ops) error {
	if len(cfg.Sysctl) == 0 {
		return nil
	}
	lines := make([]string, 0, len(cfg.Sysctl))
	for k, v := range cfg.Sysctl {
		if err := ValidateSysctlKey(k); err != nil {
			return fmt.Errorf("invalid sysctl key %s: %w", k, err)
		}
		lines = append(lines, fmt.Sprintf("%s = %v", k, v))
	}
	sort.Strings(lines)
	return o.WriteFile(sysctlFile, lines, "# buildos managed")
}

func ValidateSysctlKey(k string) error {
	if !sysctlKeyR.MatchString(k) {
		return fmt.Errorf("invalid sysctl key: %s", k)
	}
	return nil
}
