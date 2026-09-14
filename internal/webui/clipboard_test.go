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
		[]byte(`autoCopyPortalSelection(x)`),
		[]byte(`installPortalViewportProtection(x)`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
	if bytes.Contains(IndexHTML, []byte(`x.el.addEventListener('mouseup'`)) || bytes.Contains(IndexHTML, []byte(`writePortalClipboard(text).then(ok=>`)) {
		t.Fatal("selecting terminal text must not use the selection-destroying fallback copy path")
	}
}

func TestCopyUsesSynchronousFallbackAndModernClipboard(t *testing.T) {
	execCopy := bytes.Index(IndexHTML, []byte(`document.execCommand('copy')`))
	asyncCopy := bytes.Index(IndexHTML, []byte(`navigator.clipboard?.writeText`))
	if execCopy < 0 || asyncCopy < 0 || execCopy > asyncCopy {
		t.Fatal("copy must try the synchronous browser copy event before the async Clipboard API")
	}
	if !bytes.Contains(IndexHTML, []byte(`const text=portalPendingCopy||portalSelection()`)) {
		t.Fatal("the synchronous copy event must use the explicitly requested text")
	}
	if bytes.Contains(IndexHTML, []byte(`if(ok)return true`)) {
		t.Fatal("copy must not trust the deprecated execCommand result without trying the modern Clipboard API")
	}
	if !bytes.Contains(IndexHTML, []byte(`return fallbackOK`)) {
		t.Fatal("copy must retain execCommand as a fallback for restricted Clipboard APIs")
	}
}

func TestSelectionAutoCopyDoesNotRefocusOrUseFallback(t *testing.T) {
	start := bytes.Index(IndexHTML, []byte(`function autoCopyPortalSelection(x){`))
	if start < 0 {
		t.Fatal("auto-copy helper missing")
	}
	end := bytes.Index(IndexHTML[start:], []byte(`async function copyActiveTerminalSelection()`))
	if end < 0 {
		t.Fatal("auto-copy helper boundary missing")
	}
	body := IndexHTML[start : start+end]
	if !bytes.Contains(body, []byte(`navigator.clipboard.writeText(text).catch(()=>{})`)) {
		t.Fatal("selection auto-copy must use only the modern Clipboard API")
	}
	for _, forbidden := range [][]byte{[]byte(`writePortalClipboard`), []byte(`execCommand`), []byte(`term.focus()`)} {
		if bytes.Contains(body, forbidden) {
			t.Fatalf("selection auto-copy must not contain %q", forbidden)
		}
	}
}

func TestStreamingOutputPreservesScrolledViewport(t *testing.T) {
	for _, needle := range [][]byte{
		[]byte(`const portalWrite=x.term.write.bind(x.term)`),
		[]byte(`follow=before.viewportY>=before.baseY`),
		[]byte(`viewportY=before.viewportY`),
		[]byte(`x.term.scrollToLine(Math.min(viewportY,after.baseY))`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("viewport protection missing %q", needle)
		}
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
