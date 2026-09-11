package webui

import (
	"bytes"
	"testing"
)

func TestXtermAllowsPlainDragSelectionWithMouseReporting(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`cdn.jsdelivr.net/npm/@xterm/xterm@6.1.0-beta.303/css/xterm.css`),
		[]byte(`cdn.jsdelivr.net/npm/@xterm/xterm@6.1.0-beta.303/lib/xterm.js`),
		[]byte(`mouseEventsRequireAlt:true`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
	if bytes.Contains(IndexHTML, []byte(`xterm@5.3.0`)) || bytes.Contains(IndexHTML, []byte(`cdnjs.cloudflare.com/ajax/libs/xterm/5.4.0`)) {
		t.Fatal("legacy xterm builds must not be served")
	}
}
