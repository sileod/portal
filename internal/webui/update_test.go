package webui

import (
	"bytes"
	"testing"
)

func TestUpdateControlsInjected(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`id="portalUpdateHosts"`),
		[]byte(`portal_update_`),
		[]byte(`raw.githubusercontent.com/sileod/portal/main/install.sh`),
		[]byte(`action:'create'`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}
