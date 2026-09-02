package webui

import (
	"bytes"
	"testing"
)

func TestClipboardControlsInjected(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`id="copy"`),
		[]byte(`id="paste"`),
		[]byte(`addEventListener('copy'`),
		[]byte(`addEventListener('paste'`),
		[]byte(`x.term.hasSelection()`),
		[]byte(`x.term.paste(text)`),
		[]byte(`navigator.clipboard?.writeText`),
		[]byte(`navigator.clipboard?.readText`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}
