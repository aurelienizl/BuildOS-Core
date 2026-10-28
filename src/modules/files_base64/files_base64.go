package files_base64

import (
	"encoding/base64"
	"fmt"

	"buildos/src/config"
	"buildos/src/modules/files"
	"buildos/src/ops"
)

func Apply(entries []config.Base64FileEntry, o *ops.Ops) error {
	for _, entry := range entries {
		if entry.Content == nil {
			return fmt.Errorf("file %q requires base64 content", entry.Name)
		}
		content, err := base64.StdEncoding.Strict().DecodeString(*entry.Content)
		if err != nil {
			return fmt.Errorf("file %q: invalid base64 content: %w", entry.Name, err)
		}
		if err := files.Write(entry.FileTarget, o, func() error {
			return o.WriteBytes(entry.Destination, content)
		}); err != nil {
			return err
		}
	}
	return nil
}
