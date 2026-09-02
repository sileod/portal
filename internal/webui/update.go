package webui

import "bytes"

func init() {
	const panelNeedle = `<h2>Pending scheduled messages</h2>`
	const panel = `<h2>Update Portal</h2>
        <div style="color:var(--muted);margin-bottom:8px">Update Portal on a host using the standard installer. tmux sessions keep running; that host may disconnect briefly.</div>
        <div id="portalUpdateHosts" style="border:1px solid var(--line);background:var(--tab)"></div>
        <h2>Pending scheduled messages</h2>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(panelNeedle), []byte(panel), 1)

	const bodyNeedle = `</body>`
	const script = `<script>
function renderPortalUpdateHosts(){
  const root=document.querySelector('#portalUpdateHosts');if(!root)return;
  root.replaceChildren();
  if(!hosts.length){const e=document.createElement('div');e.style.cssText='padding:12px;color:var(--muted)';e.textContent='No connected hosts.';root.appendChild(e);return}
  for(const host of hosts){
    const row=document.createElement('div'),name=document.createElement('span'),button=document.createElement('button');
    row.style.cssText='display:flex;align-items:center;justify-content:space-between;gap:12px;padding:9px 10px;border-bottom:1px solid var(--line)';
    name.textContent=host;button.className='panelbutton';button.type='button';button.textContent='Update';
    button.onclick=async()=>{
      if(!confirm('Update Portal on '+host+'?\n\nTerminal sessions keep running, but Portal on that host will reconnect.'))return;
      const session='portal_update_'+Date.now().toString(36);
      const installer='https://raw.githubusercontent.com/sileod/portal/main/install.sh';
      const command='tmp=$(mktemp); if curl -fsSL '+installer+' -o "$tmp" && sh "$tmp"; then rm -f "$tmp"; tmux kill-session -t '+session+'; else rc=$?; rm -f "$tmp"; echo "[portal update failed; session left open for inspection]"; exit $rc; fi';
      button.disabled=true;button.textContent='Starting…';
      const ok=await sessionAction({action:'create',host,name:session,command},'Update '+host);
      if(ok)showStatus('Update started on '+host+'. It may disconnect briefly.');
      setTimeout(()=>{button.disabled=false;button.textContent='Update'},3000);
    };
    row.append(name,button);root.appendChild(row);
  }
  const rows=root.children;if(rows.length)rows[rows.length-1].style.borderBottom='0';
}
const portalPanelUpdater=setInterval(()=>{if(active==='__portal__')renderPortalUpdateHosts()},1500);
renderPortalUpdateHosts();
</script></body>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(bodyNeedle), []byte(script), 1)
}
