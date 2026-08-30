package webui

import (
	"bytes"
	"testing"
)

func TestClipboardControlsInjected(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`id="paste"`),
		[]byte(`addEventListener('paste'`),
		[]byte(`x.term.paste(text)`),
		[]byte(`navigator.clipboard?.readText`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}
