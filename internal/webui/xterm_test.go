package webui

import (
	"bytes"
	"testing"
)

func TestXtermAllowsPlainAndAltDragSelectionWithMouseReporting(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`cdn.jsdelivr.net/npm/@xterm/xterm@6.0.0/css/xterm.css`),
		[]byte(`cdn.jsdelivr.net/npm/@xterm/xterm@6.0.0/lib/xterm.js`),
		[]byte(`installPortalMouseSelection(x)`),
		[]byte(`x.term.modes.mouseTrackingMode==='none'`),
		[]byte(`down.button!==0||x.term.modes.mouseTrackingMode==='none'`),
		[]byte(`for(const x of terms.values())ensurePortalTerminalControls(x)`),
		[]byte(`x.term.select(`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
	if bytes.Contains(IndexHTML, []byte(`xterm@5.3.0`)) || bytes.Contains(IndexHTML, []byte(`6.1.0-beta`)) {
		t.Fatal("legacy xterm builds must not be served")
	}
}
