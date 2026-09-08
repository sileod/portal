package webui

import (
	"bytes"
	"testing"
)

func TestLastUsedReadAllAndVersionControls(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`value="last_used"`),
		[]byte(`portalLastUsed`),
		[]byte(`id="readall"`),
		[]byte(`markAllRead()`),
		[]byte(`id="debugVersion"`),
		[]byte(`data.version`),
		[]byte(`hostVersions[host]`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}
