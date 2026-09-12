package webui

import "bytes"

func init() {
	const toolNeedle = `      <button class="tool" id="newtab" title="New terminal">+</button>`
	const tools = `      <button class="tool" id="newtab" title="New terminal">+</button>
      <button class="tool" id="copy" title="Copy selected terminal text">⧉</button>
      <button class="tool" id="paste" title="Paste clipboard into active terminal">⎘</button>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(toolNeedle), []byte(tools), 1)

	const bodyNeedle = `</body>`
	const script = `<script>
function activePortalTerminal(){return terms.get(active)||null}
function portalSelection(){const x=activePortalTerminal();return x?x.term.getSelection():''}
let portalPendingCopy='';
async function writePortalClipboard(text){
  if(!text)return false;
  const ta=document.createElement('textarea');
  ta.value=text;ta.setAttribute('readonly','');ta.style.position='fixed';ta.style.opacity='0';ta.style.pointerEvents='none';
  document.body.appendChild(ta);ta.select();ta.setSelectionRange(0,ta.value.length);
  portalPendingCopy=text;
  let fallbackOK=false;try{fallbackOK=document.execCommand('copy')}catch{}finally{portalPendingCopy='';ta.remove()}
  try{
    if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return true}
  }catch{}
  return fallbackOK;
}
async function copyActiveTerminalSelection(){
  const text=portalSelection();
  if(!text){showStatus('Select terminal text first.',true);return false}
  const ok=await writePortalClipboard(text);
  showStatus(ok?'Copied terminal selection.':'Clipboard write was blocked by the browser.',!ok);
  activePortalTerminal()?.term.focus();
  return ok;
}
function pasteIntoActiveTerminal(text){
  const x=activePortalTerminal();
  if(!x)return false;
  x.term.paste(text);x.term.focus();return true;
}
function portalIsMac(){return /Mac|iPhone|iPad|iPod/i.test(navigator.userAgentData?.platform||navigator.platform||'')}
async function readPortalClipboard(){
  if(!navigator.clipboard?.readText)throw new Error('clipboard API unavailable');
  return navigator.clipboard.readText();
}
function promptPortalPaste(){
  const shade=document.createElement('div'),box=document.createElement('div'),label=document.createElement('label'),ta=document.createElement('textarea'),actions=document.createElement('div'),cancel=document.createElement('button'),send=document.createElement('button');
  shade.style.cssText='position:fixed;z-index:100;inset:0;display:grid;place-items:center;padding:20px;background:#0008';
  box.style.cssText='width:min(560px,100%);padding:14px;border:1px solid var(--line);border-radius:6px;background:var(--bg);box-shadow:0 12px 40px #0008';
  label.textContent='Paste clipboard text';label.style.cssText='display:block;margin-bottom:9px';
  ta.style.cssText='display:block;width:100%;height:150px;resize:vertical;padding:9px;border:1px solid var(--line);background:var(--active);color:var(--fg);font:inherit';
  actions.style.cssText='display:flex;justify-content:flex-end;gap:8px;margin-top:10px';
  cancel.className=send.className='panelbutton';cancel.type=send.type='button';cancel.textContent='Cancel';send.textContent='Paste';
  const close=()=>{shade.remove();activePortalTerminal()?.term.focus()};
  cancel.onclick=close;send.onclick=()=>{const text=ta.value;close();pasteIntoActiveTerminal(text)};
  shade.onclick=e=>{if(e.target===shade)close()};ta.onkeydown=e=>{if(e.key==='Escape'){e.preventDefault();close()}else if((e.ctrlKey||e.metaKey)&&e.key==='Enter'){e.preventDefault();send.click()}};
  actions.append(cancel,send);box.append(label,ta,actions);shade.appendChild(box);document.body.appendChild(shade);ta.focus();
}
document.addEventListener('copy',e=>{
  const text=portalPendingCopy||portalSelection();
  if(!text||!e.clipboardData)return;
  e.clipboardData.setData('text/plain',text);e.preventDefault();
},true);
document.addEventListener('paste',e=>{
  if(e.target instanceof HTMLInputElement||e.target instanceof HTMLTextAreaElement)return;
  const x=activePortalTerminal();if(!x)return;
  const text=e.clipboardData?.getData('text/plain');if(typeof text!=='string')return;
  e.preventDefault();e.stopImmediatePropagation();pasteIntoActiveTerminal(text);
},true);
document.addEventListener('keydown',e=>{
  const x=activePortalTerminal();if(!x||e.altKey)return;
  const key=(e.key||'').toLowerCase(),mod=e.ctrlKey||e.metaKey;
  const copyShortcut=mod&&key==='c'&&(e.metaKey||e.shiftKey||x.term.hasSelection());
  if(copyShortcut&&x.term.hasSelection()){
    e.preventDefault();e.stopImmediatePropagation();copyActiveTerminalSelection();
  }else if(portalIsMac()&&e.ctrlKey&&!e.metaKey&&key==='v'){
    e.preventDefault();e.stopImmediatePropagation();
    readPortalClipboard().then(pasteIntoActiveTerminal).catch(()=>{
      showStatus('Clipboard read was blocked. Use Cmd+V or the Paste button.',true);x.term.focus();
    });
  }
},true);
function installPortalClipboard(x){
  x.term.attachCustomKeyEventHandler(e=>{
    if(e.type!=='keydown'||e.altKey)return true;
    const key=(e.key||'').toLowerCase();
    if(key==='c'&&(e.metaKey||e.ctrlKey&&x.term.hasSelection()))return false;
    if(key==='v'&&(e.metaKey||e.ctrlKey))return false;
    return true;
  });
  installPortalMouseSelection(x);
}
function portalTerminalCell(x,e){
  const screen=x.term.element?.querySelector('.xterm-screen');if(!screen)return null;
  const r=screen.getBoundingClientRect();if(!r.width||!r.height)return null;
  const col=Math.max(0,Math.min(x.term.cols-1,Math.floor((e.clientX-r.left)*x.term.cols/r.width)));
  const viewportRow=Math.max(0,Math.min(x.term.rows-1,Math.floor((e.clientY-r.top)*x.term.rows/r.height)));
  return{col,row:x.term.buffer.active.viewportY+viewportRow};
}
function installPortalMouseSelection(x){
  x.term.element.addEventListener('mousedown',down=>{
    if(down.button!==0||down.altKey||x.term.modes.mouseTrackingMode==='none')return;
    const anchor=portalTerminalCell(x,down);if(!anchor)return;
    let moved=false;
    const move=e=>{
      const point=portalTerminalCell(x,e);if(!point)return;
      const a=anchor.row*x.term.cols+anchor.col,b=point.row*x.term.cols+point.col;
      if(a===b)return;
      moved=true;const start=Math.min(a,b),end=Math.max(a,b);
      x.term.select(start%x.term.cols,Math.floor(start/x.term.cols),end-start);
      e.preventDefault();e.stopImmediatePropagation();
    };
    const up=e=>{
      document.removeEventListener('mousemove',move,true);document.removeEventListener('mouseup',up,true);
      if(!moved)x.term.clearSelection();
      e.preventDefault();e.stopImmediatePropagation();
    };
    document.addEventListener('mousemove',move,true);document.addEventListener('mouseup',up,true);
    down.preventDefault();down.stopImmediatePropagation();
  },true);
}
const openPortalTerminal=openTerm;
openTerm=function(s){
  const x=openPortalTerminal(s);
  if(!x.portalClipboard){installPortalClipboard(x);x.portalClipboard=true}
  return x;
};
const pastePortalButton=document.querySelector('#paste');
const copyPortalButton=document.querySelector('#copy');
if(copyPortalButton)copyPortalButton.onclick=copyActiveTerminalSelection;
if(pastePortalButton)pastePortalButton.onclick=async()=>{
  if(!activePortalTerminal())return;
  try{
    const text=await readPortalClipboard();pasteIntoActiveTerminal(text);
  }catch{
    promptPortalPaste();
  }
};
</script></body>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(bodyNeedle), []byte(script), 1)
}
