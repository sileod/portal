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
