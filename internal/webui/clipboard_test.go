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
		[]byte(`installPortalClipboard(x)`),
		[]byte(`term.attachCustomKeyEventHandler`),
		[]byte(`x.el.addEventListener('mouseup'`),
		[]byte(`portalIsMac()`),
		[]byte(`e.ctrlKey&&!e.metaKey&&key==='v'`),
		[]byte(`navigator.clipboard?.writeText`),
		[]byte(`navigator.clipboard?.readText`),
		[]byte(`promptPortalPaste()`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}

func TestBackgroundRefreshDoesNotRefocusTerminal(t *testing.T) {
	if !bytes.Contains(IndexHTML, []byte(`activate(active,false)`)) {
		t.Fatal("background refresh must not refocus and clear the active terminal selection")
	}
}
