package webui

import (
	"bytes"
	"testing"
)

func TestSessionDeepLinkAndOverview(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`new URLSearchParams(location.search)`),
		[]byte(`launchParams.get('host')`),
		[]byte(`launchParams.get('session')`),
		[]byte(`className='tab'+(isPreferred(s)?' requested':'')`),
		[]byte(`id="sessionOverview"`),
		[]byte(`tmux attach-session -t `),
		[]byte(`writePortalClipboard(command)`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}

func TestUnexpectedTerminalDisconnectReloadsWhenPortalReturns(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`function reloadWhenPortalReturns()`),
		[]byte(`if(r.ok||r.status===401){location.reload();return}`),
		[]byte(`if(!x.closing)reloadWhenPortalReturns()`),
		[]byte(`x.closing=true;x.ws.close()`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing reconnect behavior %q", needle)
		}
	}
}
