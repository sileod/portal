package webui

import "bytes"

func init() {
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css`),
		[]byte(`https://cdn.jsdelivr.net/npm/@xterm/xterm@6.0.0/css/xterm.css`), 1)
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.js`),
		[]byte(`https://cdn.jsdelivr.net/npm/@xterm/xterm@6.0.0/lib/xterm.js`), 1)
}
