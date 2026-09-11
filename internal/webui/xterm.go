package webui

import "bytes"

func init() {
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css`),
		[]byte(`https://cdn.jsdelivr.net/npm/@xterm/xterm@6.1.0-beta.303/css/xterm.css`), 1)
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.js`),
		[]byte(`https://cdn.jsdelivr.net/npm/@xterm/xterm@6.1.0-beta.303/lib/xterm.js`), 1)
	IndexHTML = bytes.Replace(IndexHTML,
		[]byte(`scrollback:10000,theme:`),
		[]byte(`scrollback:10000,mouseEventsRequireAlt:true,theme:`), 1)
}
