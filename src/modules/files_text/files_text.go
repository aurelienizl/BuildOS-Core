package files_text

import (
	"fmt"

	"buildos/src/config"
	"buildos/src/modules/files"
	"buildos/src/ops"
)

func Apply(entries []config.TextFileEntry, o *ops.Ops) error {
	for _, entry := range entries {
		if entry.Content == nil {
			return fmt.Errorf("file %q requires text content", entry.Name)
		}
		if err := files.Write(entry.FileTarget, o, func() error {
			return o.WriteBytes(entry.Destination, []byte(*entry.Content))
		}); err != nil {
			return err
		}
	}
	return nil
}
