package webui

import (
	"bytes"
	"testing"
)

func TestXtermIncludesTmuxMouseSelectionFix(t *testing.T) {
	if bytes.Contains(IndexHTML, []byte(`xterm@5.3.0`)) {
		t.Fatal("xterm 5.3 clears tmux mouse selections on mouseup")
	}
	for _, needle := range [][]byte{
		[]byte(`cdnjs.cloudflare.com/ajax/libs/xterm/5.4.0/xterm.css`),
		[]byte(`cdnjs.cloudflare.com/ajax/libs/xterm/5.4.0/xterm.js`),
	} {
		if !bytes.Contains(IndexHTML, needle) {
			t.Fatalf("IndexHTML missing %q", needle)
		}
	}
}
