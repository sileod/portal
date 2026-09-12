package webui

import (
	"bytes"
	"testing"
)

func TestTerminalLinksInjected(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`@xterm/addon-fit@0.12.0-beta.301`),
		[]byte(`@xterm/addon-web-links@0.13.0-beta.301`),
		[]byte(`new WebLinksAddon.WebLinksAddon`),
		[]byte(`window.open(uri,'_blank','noopener,noreferrer')`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
	for _, legacy := range [][]byte{
		[]byte(`xterm-addon-fit@0.8.0`),
		[]byte(`xterm-addon-web-links@0.9.0`),
	} {
		if bytes.Contains(IndexHTML, legacy) {
			t.Fatalf("IndexHTML still contains incompatible addon %q", legacy)
		}
	}
}
