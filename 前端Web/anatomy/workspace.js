import {mountAtlas} from './viewer.js';
import {mountLibrary} from './library.js';
export async function mountWorkspace(root,base=''){
 root.innerHTML='<nav class="model-switch" aria-label="三维资料"><button type="button" data-tab="local" aria-pressed="true">穴位图谱</button><button type="button" data-tab="library" aria-pressed="false">医学模型库 <span>在线</span></button></nav><div class="model-local"></div><div class="model-online" hidden></div>';
 const local=root.querySelector('.model-local'),online=root.querySelector('.model-online');let atlas,library,dead=false,busy=false;
 const observer=new MutationObserver(()=>root.dataset.dirty=String(atlas?.hasChanges()||local.dataset.dirty==='true'));observer.observe(local,{attributes:true,attributeFilter:['data-dirty']});
 async function select(tab){if(busy||dead)return;busy=true;const remote=tab==='library';local.hidden=remote;online.hidden=!remote;root.querySelectorAll('[data-tab]').forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.tab===tab)));try{if(remote&&!library)library=mountLibrary(online,base);if(!remote&&!atlas)atlas=await mountAtlas(local,base);if(!remote)window.dispatchEvent(new Event('resize'));}catch(e){(remote?online:local).textContent='载入失败：'+e.message;}finally{busy=false;}}
 root.querySelectorAll('[data-tab]').forEach(b=>b.onclick=()=>select(b.dataset.tab));
 await select(new URLSearchParams(location.search).get('model_library')==='1'?'library':'local');
 return {hasChanges:()=>atlas?.hasChanges()||false,dispose(){dead=true;observer.disconnect();atlas?.dispose();library?.dispose();}};
}
