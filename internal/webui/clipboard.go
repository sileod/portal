package webui

import "bytes"

func init() {
	const toolNeedle = `      <button class="tool" id="newtab" title="New terminal">+</button>`
	const tools = `      <button class="tool" id="newtab" title="New terminal">+</button>
      <button class="tool" id="paste" title="Paste clipboard into active terminal">⎘</button>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(toolNeedle), []byte(tools), 1)

	const bodyNeedle = `</body>`
	const script = `<script>
function activePortalTerminal(){return terms.get(active)||null}
function portalSelection(){const x=activePortalTerminal();return x?x.term.getSelection():''}
async function writePortalClipboard(text){
  if(!text)return false;
  try{
    if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return true}
  }catch{}
  const ta=document.createElement('textarea');
  ta.value=text;ta.setAttribute('readonly','');ta.style.position='fixed';ta.style.opacity='0';ta.style.pointerEvents='none';
  document.body.appendChild(ta);ta.select();ta.setSelectionRange(0,ta.value.length);
  let ok=false;try{ok=document.execCommand('copy')}catch{}
  ta.remove();return ok;
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
document.addEventListener('copy',e=>{
  const text=portalSelection();
  if(!text||!e.clipboardData)return;
  e.clipboardData.setData('text/plain',text);e.preventDefault();
},true);
document.addEventListener('paste',e=>{
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
  x.el.addEventListener('mouseup',()=>{
    const text=x.term.getSelection();if(!text)return;
    writePortalClipboard(text).then(ok=>{if(!ok)showStatus('Clipboard write was blocked by the browser.',true)});
  });
}
const openPortalTerminal=openTerm;
openTerm=function(s){
  const x=openPortalTerminal(s);
  if(!x.portalClipboard){installPortalClipboard(x);x.portalClipboard=true}
  return x;
};
const pastePortalButton=document.querySelector('#paste');
if(pastePortalButton)pastePortalButton.onclick=async()=>{
  if(!activePortalTerminal())return;
  try{
    const text=await readPortalClipboard();pasteIntoActiveTerminal(text);
  }catch{
    showStatus('Clipboard read was blocked. Focus the terminal and use Ctrl/Cmd+V or the browser Paste command.',true);
    activePortalTerminal()?.term.focus();
  }
};
</script></body>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(bodyNeedle), []byte(script), 1)
}
