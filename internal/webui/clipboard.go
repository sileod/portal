package webui

import "bytes"

func init() {
	const toolNeedle = `      <button class="tool" id="newtab" title="New terminal">+</button>`
	const tools = `      <button class="tool" id="newtab" title="New terminal">+</button>
      <button class="tool" id="paste" title="Paste clipboard into active terminal">⎘</button>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(toolNeedle), []byte(tools), 1)

	const bodyNeedle = `</body>`
	const script = `<script>
function pasteIntoActiveTerminal(text){
  const x=terms.get(active);
  if(!x)return false;
  x.term.paste(text);
  x.term.focus();
  return true;
}
document.addEventListener('paste',e=>{
  const x=terms.get(active);
  if(!x)return;
  const text=e.clipboardData?.getData('text/plain');
  if(typeof text!=='string')return;
  e.preventDefault();
  e.stopImmediatePropagation();
  pasteIntoActiveTerminal(text);
},true);
const pastePortalButton=document.querySelector('#paste');
if(pastePortalButton)pastePortalButton.onclick=async()=>{
  if(!terms.get(active))return;
  try{
    if(!navigator.clipboard?.readText)throw new Error('clipboard API unavailable');
    const text=await navigator.clipboard.readText();
    pasteIntoActiveTerminal(text);
  }catch(e){
    showStatus('Clipboard access was blocked. Focus the terminal and use Ctrl/Cmd+V or the browser paste command.',true);
    terms.get(active)?.term.focus();
  }
};
</script></body>`
	IndexHTML = bytes.Replace(IndexHTML, []byte(bodyNeedle), []byte(script), 1)
}
