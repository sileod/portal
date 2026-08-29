package webui

import "bytes"

func init() {
	const panelNeedle = `<h2>Pending scheduled messages</h2>`
	const panel = `<h2>Portal</h2>
        <div style="display:flex;align-items:center;justify-content:space-between;gap:18px;padding:12px 0;border-top:1px solid var(--line);border-bottom:1px solid var(--line)">
          <span style="color:var(--muted)">Stop public Portal access. tmux sessions and scheduled actions keep running.</span>
          <button class="panelbutton" id="stopPortal" type="button" style="color:var(--danger);white-space:nowrap">Stop Portal</button>
        </div>
        <h2>Pending scheduled messages</h2>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(panelNeedle), []byte(panel), 1)

	const bodyNeedle = `</body>`
	const script = `<script>
const stopPortalButton=document.querySelector('#stopPortal');
if(stopPortalButton)stopPortalButton.onclick=async()=>{
  if(!confirm('Stop Portal public access?\n\nYour tmux sessions and scheduled actions will keep running.'))return;
  stopPortalButton.disabled=true;stopPortalButton.textContent='Stopping…';
  try{
    const r=await fetch('/api/session',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'shutdown'})});
    if(!r.ok)throw new Error((await r.text()).trim()||('HTTP '+r.status));
    document.body.innerHTML='<div style="height:100vh;display:grid;place-items:center;font:14px ui-monospace,SFMono-Regular,Menlo,monospace">🌀 Portal stopped. tmux sessions are still running.</div>';
  }catch(e){stopPortalButton.disabled=false;stopPortalButton.textContent='Stop Portal';alert('Could not stop Portal: '+e.message)}
};
</script></body>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(bodyNeedle), []byte(script), 1)
}
