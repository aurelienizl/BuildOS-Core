package exec_cmd

import (
	"fmt"

	"buildos/src/ops"
)

func Apply(commands []string, o *ops.Ops) error {
	for i, command := range commands {
		o.Raw("buildos_log " + ops.Quote(fmt.Sprintf("exec_cmd: command %d", i+1)))
		if err := o.Shell(command); err != nil {
			return err
		}
	}
	return nil
}
