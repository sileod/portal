package webui

import "bytes"

func init() {
	const fitScript = `<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.js"></script>`
	const scripts = `<script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.8.0/lib/xterm-addon-fit.js"></script>
<script src="https://cdn.jsdelivr.net/npm/xterm-addon-web-links@0.9.0/lib/xterm-addon-web-links.js"></script>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(fitScript), []byte(scripts), 1)

	const addonNeedle = `const fit=new FitAddon.FitAddon();term.loadAddon(fit);term.open(el);`
	const addon = `const fit=new FitAddon.FitAddon();term.loadAddon(fit);const links=new WebLinksAddon.WebLinksAddon((event,uri)=>{const win=window.open(uri,'_blank','noopener,noreferrer');if(win)win.opener=null});term.loadAddon(links);term.open(el);`
	IndexHTML = bytes.Replace(IndexHTML, []byte(addonNeedle), []byte(addon), 1)
}
