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
		[]byte(`const text=x.term.getSelection();if(!text)return;`),
		[]byte(`writePortalClipboard(text).then(ok=>`),
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

func TestCopyUsesSynchronousFallbackBeforeAsyncClipboard(t *testing.T) {
	execCopy := bytes.Index(IndexHTML, []byte(`document.execCommand('copy')`))
	asyncCopy := bytes.Index(IndexHTML, []byte(`navigator.clipboard?.writeText`))
	if execCopy < 0 || asyncCopy < 0 || execCopy > asyncCopy {
		t.Fatal("copy must try the synchronous browser copy event before the async Clipboard API")
	}
	if !bytes.Contains(IndexHTML, []byte(`const text=portalPendingCopy||portalSelection()`)) {
		t.Fatal("the synchronous copy event must use the explicitly requested text")
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
