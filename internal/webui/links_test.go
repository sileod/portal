package webui

import (
	"bytes"
	"testing"
)

func TestTerminalLinksInjected(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`xterm-addon-web-links@0.9.0`),
		[]byte(`new WebLinksAddon.WebLinksAddon`),
		[]byte(`window.open(uri,'_blank','noopener,noreferrer')`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}
