package gcintegration

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/format"
)

//go:embed bootstrap.json
var bootstrapPatch []byte

// patchBootstrap keeps the normal cmd vendor tree, but copies the library into
// the root of dist's separate bootstrap module and rewrites its imports there.
// Nested vendor directories do not provide module dependencies in that module.
func patchBootstrap(source []byte) ([]byte, error) {
	var edits []struct{ Old, New string }
	if err := json.Unmarshal(bootstrapPatch, &edits); err != nil {
		return nil, err
	}
	for _, edit := range edits {
		if bytes.Count(source, []byte(edit.Old)) != 1 {
			return nil, fmt.Errorf("unsupported pinned bootstrap source anchor %q", edit.Old)
		}
		source = bytes.Replace(source, []byte(edit.Old), []byte(edit.New), 1)
	}
	return format.Source(source)
}
