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
	if bytes.Contains(IndexHTML, []byte(`addEventListener('mouseup'`)) {
		t.Fatal("selecting terminal text must not trigger clipboard work or clear the selection")
	}
}

func TestBackgroundRefreshDoesNotReactivateTerminal(t *testing.T) {
	if bytes.Contains(IndexHTML, []byte(`activate(active,false)`)) {
		t.Fatal("background refresh must not hide and reactivate the terminal because that clears its selection")
	}
	if !bytes.Contains(IndexHTML, []byte(`if(x)x.s=currentSession`)) {
		t.Fatal("background refresh must update the active session in place")
	}
}
