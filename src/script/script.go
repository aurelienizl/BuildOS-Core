// Package script connects ordered configuration modules to shell emitters.
package script

import (
	"fmt"

	"buildos/src/config"
	"buildos/src/modules/base"
	"buildos/src/modules/containers"
	"buildos/src/modules/exec_cmd"
	"buildos/src/modules/files"
	"buildos/src/modules/files_base64"
	"buildos/src/modules/files_text"
	"buildos/src/modules/kernel"
	"buildos/src/modules/packages"
	"buildos/src/modules/services"
	"buildos/src/modules/snaps"
	"buildos/src/modules/users"
	"buildos/src/ops"

	"gopkg.in/yaml.v3"
)

type emitter func(*yaml.Node, *ops.Ops) error

func module[T any](apply func(T, *ops.Ops) error) emitter {
	return func(node *yaml.Node, o *ops.Ops) error {
		var cfg T
		if err := config.Decode(node, &cfg); err != nil {
			return err
		}
		return apply(cfg, o)
	}
}

var modules = map[string]emitter{
	"base":         module(base.Apply),
	"packages":     module(packages.Apply),
	"services":     module(services.Apply),
	"kernel":       module(kernel.Apply),
	"files":        module(files.Apply),
	"files_base64": module(files_base64.Apply),
	"files_text":   module(files_text.Apply),
	"snaps":        module(snaps.Apply),
	"containers":   module(containers.Apply),
	"users":        module(users.Apply),
	"exec_cmd":     module(exec_cmd.Apply),
}

func Generate(cfg config.Config) (string, error) {
	o := ops.New()
	base.CheckTarget(o)
	for i, m := range cfg.Modules {
		label := fmt.Sprintf("%s:%d: %s (#%d)", m.Source, m.Line, m.Name, i+1)
		emit, ok := modules[m.Name]
		if !ok {
			return "", fmt.Errorf("%s: unknown module", label)
		}
		o.Step(label)
		if err := emit(m.Value, o); err != nil {
			return "", fmt.Errorf("%s: %w", label, m.ValidationError(err))
		}
	}
	o.Raw("buildos_log 'Configuration completed'")
	return o.String(), nil
}
