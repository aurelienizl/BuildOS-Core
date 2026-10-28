package base

import (
	"fmt"
	"strings"

	"buildos/src/config"
	"buildos/src/ops"
)

// CheckTarget emits the single OS probe used by every base occurrence.
// The generator calls it before modules, including for configurations without base.
func CheckTarget(o *ops.Ops) {
	o.Raw(`unset ID VERSION_ID
. /etc/os-release
readonly buildos_distribution="${ID:-}"
readonly buildos_release="${VERSION_ID:-}"
case "$buildos_distribution" in
    debian|ubuntu) ;;
    *) buildos_log "ERROR: unsupported distribution: ${buildos_distribution:-unknown}"; exit 1 ;;
esac`)
}

func Apply(cfg config.BaseConfig, o *ops.Ops) error {
	d := strings.ToLower(strings.TrimSpace(cfg.Distribution))
	v := strings.TrimSpace(cfg.Release)
	// Include-only base entries are useful in fragments.
	if d == "" && v == "" {
		return nil
	}
	if d != "debian" && d != "ubuntu" {
		return fmt.Errorf("unsupported distribution: %q", d)
	}
	if v == "" {
		return fmt.Errorf("release is required with distribution")
	}
	o.Raw("if [[ $buildos_distribution != " + ops.Quote(d) + " || $buildos_release != " + ops.Quote(v) + " ]]; then")
	o.Raw("    buildos_log " + ops.Quote("ERROR: expected "+d+" "+v) + "; exit 1")
	o.Raw("fi")
	return nil
}
