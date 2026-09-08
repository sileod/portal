package webui

import "bytes"

func init() {
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css`),
		[]byte(`https://cdnjs.cloudflare.com/ajax/libs/xterm/5.4.0/xterm.css`), 1)
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.js`),
		[]byte(`https://cdnjs.cloudflare.com/ajax/libs/xterm/5.4.0/xterm.js`), 1)
}
