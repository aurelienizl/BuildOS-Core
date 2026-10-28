package users

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"buildos/src/config"
	"buildos/src/ops"
)

var nameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)

func Apply(entries []config.UserEntry, o *ops.Ops) error {
	for _, u := range entries {
		if !nameRE.MatchString(u.Username) {
			return fmt.Errorf("invalid username: %q", u.Username)
		}
		if u.PasswordHash == "" {
			return fmt.Errorf("user %s requires password_hash", u.Username)
		}
		shell := u.Shell
		if shell == "" {
			shell = "bash"
		}
		if !strings.Contains(shell, "/") {
			shell = path.Join("/bin", shell)
		}
		if !path.IsAbs(shell) {
			return fmt.Errorf("user %s: shell must be a name or absolute path", u.Username)
		}
		o.Raw("test -x " + ops.Quote(shell))
		o.Raw("if id -u -- " + ops.Quote(u.Username) + " >/dev/null 2>&1; then")
		if err := o.Run("usermod", "-p", u.PasswordHash, "-s", shell, u.Username); err != nil {
			return err
		}
		o.Raw("else")
		if err := o.Run("useradd", "-m", "-p", u.PasswordHash, "-s", shell, u.Username); err != nil {
			return err
		}
		o.Raw("fi")
		groups := append([]string(nil), u.Groups...)
		if u.Sudo {
			groups = append(groups, "sudo")
		}
		if len(groups) > 0 {
			if err := o.Run("usermod", "-aG", strings.Join(groups, ","), u.Username); err != nil {
				return err
			}
		}
	}
	return nil
}
