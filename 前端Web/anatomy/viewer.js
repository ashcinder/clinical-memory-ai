import * as THREE from './vendor/three.module.js';
import {OrbitControls} from './vendor/OrbitControls.js';
import {GLTFLoader} from './vendor/GLTFLoader.js';
import {MODELS,presetAnchors} from './presets.js';
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const uid=()=>Array.from(crypto.getRandomValues(new Uint8Array(16)),v=>v.toString(16).padStart(2,'0')).join('');
const channels=[
 ['LU','肺经','手太阴肺经'],['LI','大肠经','手阳明大肠经'],['ST','胃经','足阳明胃经'],
 ['SP','脾经','足太阴脾经'],['HT','心经','手少阴心经'],['SI','小肠经','手太阳小肠经'],
 ['BL','膀胱经','足太阳膀胱经'],['KI','肾经','足少阴肾经'],['PC','心包经','手厥阴心包经'],
 ['TE','三焦经','手少阳三焦经'],['GB','胆经','足少阳胆经'],['LR','肝经','足厥阴肝经'],
 ['GV','督脉'],['CV','任脉'],['EX','经外奇穴'],['custom','其他点位']
];
function channelOf(point){
 const name=point.meridian?.trim();
 const match=channels.find(([,short,full])=>name===short||(name&&name===full));
 if(match)return match[0];
 const prefix=point.code?.trim().toUpperCase().match(/^([A-Z]+)(?=[\d-])/)?.[1];
 const code=prefix==='SJ'?'TE':prefix;
 return channels.some(([id])=>id===code)?code:'custom';
}
export async function mountAtlas(root,base=''){
 let dead=false,dirty=false,points=[],revision='',selected='',placing=false,newPending=false,dragging=false,start=null,sex='male',visible=true,saving=false,freeView=false,pickingCenter=false,panMode=false,homeView='front',homeTargetY=1;
 let names=false,channelFilter='';
 const hiddenChannels=new Set();
 const abort=new AbortController(),signal=abort.signal;
 const eye='<path d="M2 12s3.6-6 10-6 10 6 10 6-3.6 6-10 6-10-6-10-6Z"/><circle cx="12" cy="12" r="2.5"/>';
 const icon=paths=>`<svg viewBox="0 0 24 24" aria-hidden="true">${paths}</svg>`;
 const tool=(action,label,paths)=>`<button type="button" data-action="${action}" aria-label="${label}" title="${label}">${icon(paths)}</button>`;
 root.innerHTML=`
 <section class="atlas" aria-label="三维穴位图谱">
  <header class="a-titlebar">
   <div class="a-title"><strong>3D 经络腧穴</strong><span class="a-model-name">男性模型</span></div>
   <div class="a-sex" role="group" aria-label="人体模型"><button data-sex="male" aria-pressed="true">男</button><button data-sex="female" aria-pressed="false">女</button></div>
   <div class="a-header-actions">
    <button data-action="records" aria-label="穴位记录" aria-expanded="false" aria-controls="a-records">${icon('<path d="M5 3h14v18H5zM8 8h8M8 12h8M8 16h5"/>')}<span>穴位记录</span></button>
    ${tool('fullscreen','全屏','<path d="M9 4H4v5m11-5h5v5M4 15v5h5m6 0h5v-5"/>')}
   </div>
  </header>
  <div class="a-message" role="alert" hidden></div>
  <div class="atlas-layout">
   <aside class="a-channels" id="a-channels" aria-label="经络点位">
    <div class="a-rail-head"><strong>经络</strong><button data-action="channels" class="a-rail-close" aria-label="关闭经络面板">×</button></div>
    <div class="a-channel-tools">
     <button data-action="show-all">全显</button><button data-action="hide-all">全隐</button>
    </div>
    <div class="a-channel-list"></div>
   </aside>
   <div class="a-viewer">
    <div class="atlas-stage">
     <div class="a-stage-head">
      <button class="a-channel-trigger" data-action="channels" aria-controls="a-channels" aria-expanded="false">${icon('<path d="M5 5h14M5 12h14M5 19h14"/><circle cx="9" cy="5" r="2"/><circle cx="15" cy="12" r="2"/>')}经络</button>
      <span class="a-orientation">正面</span><span class="a-selection" role="status" hidden></span>
     </div>
     <div class="a-name-layer" aria-label="当前可见穴位名称"><svg class="a-name-lines" aria-hidden="true"></svg></div>
     <div class="a-selected-ring" hidden aria-hidden="true"></div>
     <span class="a-hint">拖动旋转 · 滚轮 / 双指缩放</span>
     <div class="a-model-loading" role="status">正在载入人体模型…</div>
    </div>
    <div class="a-view-tools" aria-label="三维视图工具">
     <div class="a-camera-tools">
      <div class="a-compass" role="group" aria-label="固定视角">
       <button data-view="front" aria-label="正面" title="正面">前</button>
       <button data-view="back" aria-label="背面" title="背面">后</button>
       <button data-view="left" aria-label="左侧" title="左侧">左</button>
       <button data-view="right" aria-label="右侧" title="右侧">右</button>
      </div>
      <div class="a-zoom" role="group" aria-label="缩放">
       ${tool('zoom-out','缩小','<path d="M5 12h14"/>')}
       <button data-action="reset-view" title="恢复完整正面视图">适屏</button>
       ${tool('zoom-in','放大','<path d="M12 5v14M5 12h14"/>')}
      </div>
     </div>
     <div class="a-display-tools">
      <div class="a-layer-tools" role="group" aria-label="点位显示">
       <button data-action="names" aria-pressed="false" title="显示穴位名称">点名</button>
       <button data-action="current-only" aria-pressed="false" title="仅显示当前穴位">仅选</button>
      </div>
      <div class="a-mode-tools" role="group" aria-label="操作模式">
       <button data-action="free-view" aria-pressed="false">自由</button>
       <button data-action="center" aria-pressed="false" disabled>设中心</button>
       <button data-action="pan" aria-pressed="false" disabled>平移</button>
      </div>
     </div>
    </div>
   </div>
   <aside class="atlas-side" id="a-records" aria-label="穴位与记录" hidden>
    <div class="a-side-head"><strong>穴位与记录</strong><button data-action="records" aria-label="关闭穴位记录">×</button></div>
    <div class="a-query-head"><label class="a-query-label" for="a-query">查找穴位</label><button data-action="clear-filter" hidden>查看全部</button></div>
    <input id="a-query" type="search" placeholder="名称、编号或经络">
    <div class="a-list" aria-label="穴位列表"></div><div class="a-editor"></div>
    <p class="a-legacy" hidden></p>
   </aside>
  </div>
  <footer class="a-footer">
   <div class="a-footer-copy"><span class="a-status" role="status">正在载入人体…</span><span class="a-stage-badge">学习示意 · 点位待校核</span></div>
   <div class="a-footer-actions">
    <details class="a-more"><summary aria-label="更多图谱操作" title="更多图谱操作">···</summary><div><button data-action="reload">刷新图谱</button><button data-action="export">导出 JSON</button></div></details>
    <button data-action="add">手动标记</button><button class="a-primary" data-action="save" disabled>保存修改</button>
   </div>
  </footer>
 </section>
 <dialog class="a-fullscreen" aria-label="全屏穴位图谱"></dialog>`;
 const $=s=>root.querySelector(s),stage=$('.atlas-stage'),atlas=$('.atlas'),fullscreen=$('.a-fullscreen'),native=document.body.classList.contains('atlas-standalone');
 const records=$('.atlas-side'),recordToggle=$('.a-header-actions [data-action=records]');
 const channelToggle=$('.a-channel-trigger'),fullscreenToggle=$('[data-action=fullscreen]');
 $('.a-channel-list').innerHTML=channels.map(([id,name,full])=>`
  <div class="a-channel-row">
   <button data-channel="${id}" aria-pressed="false" title="查看${full||name}点位">${name}<small>0</small></button>
   <button data-channel-visibility="${id}" aria-label="显示${name}点位" title="显示${name}点位" aria-pressed="true">${icon(eye+'<path class="a-eye-slash" d="m4 4 16 16"/>')}</button>
  </div>`).join('');
 function setChannels(open){
  atlas.classList.toggle('a-channels-open',open);
  channelToggle.setAttribute('aria-expanded',String(open));
 }
 function setRecords(open){
  records.hidden=!open;
  recordToggle.setAttribute('aria-expanded',String(open));
  if(open)setChannels(false);
 }
 function toggleFullscreen(){
  if(fullscreen.open){fullscreen.close();return;}
  fullscreen.append(atlas);
  fullscreen.showModal();
  fullscreenToggle.setAttribute('aria-label','退出全屏');
  fullscreenToggle.title='退出全屏';
  fullscreenToggle.focus();
 }
 fullscreen.addEventListener('close',()=>{
  root.prepend(atlas);
  fullscreenToggle.setAttribute('aria-label','全屏');
  fullscreenToggle.title='全屏';
  fullscreenToggle.focus({preventScroll:true});
 },{signal});
 if(native)$('[data-action=export]').textContent='复制 JSON';
 const message=e=>{const m=$('.a-message');m.hidden=!e;m.textContent=e||'';};
 const active=()=>points.filter(p=>p.model===MODELS[sex].id&&!p.hidden);
 const current=()=>points.find(p=>p.id===selected&&!p.hidden);
 const shown=p=>!hiddenChannels.has(channelOf(p))&&(visible||p.id===selected);
 function status(){root.dataset.dirty=String(dirty);$('.a-status').textContent=`${active().length} 个穴位 / 点位 · ${dirty?'有未保存修改':'已保存'}`;$('[data-action=save]').disabled=!dirty||saving;}
 function changed(){dirty=true;status();}
 async function request(method='GET',data){const r=await fetch(base+'/api/anatomy',{method,credentials:'same-origin',headers:{'Content-Type':'application/json'},body:data?JSON.stringify(data):undefined,signal});const result=await r.json();if(!r.ok)throw Error(result.detail||'读取失败');return result;}
 const scene=new THREE.Scene(),camera=new THREE.PerspectiveCamera(35,1,.01,100);camera.position.set(0,1.05,3.75);
 let renderer;
 try{renderer=new THREE.WebGLRenderer({antialias:true,alpha:true,preserveDrawingBuffer:true,powerPreference:'low-power'});}catch(e){throw Error('设备未能启用 WebGL，请更新浏览器或 Android System WebView。');}
 renderer.setPixelRatio(Math.min(devicePixelRatio,2));renderer.setClearColor(0,0);renderer.toneMapping=THREE.ACESFilmicToneMapping;renderer.toneMappingExposure=1.05;
 stage.prepend(renderer.domElement);renderer.domElement.setAttribute('aria-label','可旋转缩放的写实人体三维模型');renderer.domElement.tabIndex=0;
 const controls=new OrbitControls(camera,renderer.domElement);controls.target.set(0,1,0);controls.minDistance=.4;controls.maxDistance=5;controls.maxPolarAngle=Math.PI*.95;controls.enablePan=false;
 scene.add(new THREE.HemisphereLight(0xffffff,0xb2b7b9,2));
 const key=new THREE.DirectionalLight(0xffefe3,2.7);key.position.set(-2,3,4);scene.add(key);
 const fill=new THREE.DirectionalLight(0xe2ecff,1.5);fill.position.set(3,2,-3);scene.add(fill);
 const body=new THREE.Group(),marks=new THREE.Group(),bounds=new THREE.Box3();scene.add(body,marks);
 const ray=new THREE.Raycaster(),mouse=new THREE.Vector2(),modelCache=new Map();
 let bodyMeshes=[],loading=false;
 const labelRay=new THREE.Raycaster(),nameNodes=new Map();let labelFrame=null;
 function viewHint(){return panMode?'拖动平移 · 双指缩放':freeView?'拖动旋转 · 双击人体设中心':'拖动旋转 · 滚轮 / 双指缩放';}
 function cameraMode(){controls.enablePan=freeView;controls.zoomToCursor=freeView;controls.minDistance=freeView?.1:.4;controls.maxPolarAngle=freeView?Math.PI-.01:Math.PI*.95;controls.mouseButtons.LEFT=panMode?THREE.MOUSE.PAN:THREE.MOUSE.ROTATE;controls.mouseButtons.MIDDLE=freeView?THREE.MOUSE.PAN:THREE.MOUSE.DOLLY;controls.touches.ONE=panMode?THREE.TOUCH.PAN:THREE.TOUCH.ROTATE;controls.enabled=!pickingCenter;
 $('[data-action=free-view]').setAttribute('aria-pressed',String(freeView));for(const action of ['center','pan'])$('[data-action='+action+']').disabled=!freeView;
 $('[data-action=center]').setAttribute('aria-pressed',String(pickingCenter));$('[data-action=pan]').setAttribute('aria-pressed',String(panMode));stage.classList.toggle('a-pick-center',pickingCenter);$('.a-hint').textContent=pickingCenter?'点击人体表面，设为旋转与缩放中心':viewHint();}
 function setHomeView(view='front'){
  homeView=view;
  if(bounds.isEmpty())return;
  const size=bounds.getSize(new THREE.Vector3()),side=view==='left'||view==='right';
  const slope=Math.tan(THREE.MathUtils.degToRad(camera.fov)/2);
  const distance=Math.max(size.y,(side?size.z:size.x)/camera.aspect)/(1.86*slope)+(side?size.x:size.z)/2;
  bounds.getCenter(controls.target);
  controls.maxDistance=distance/.75;
  camera.position.copy(controls.target);
  if(side)camera.position.x+=view==='left'?distance:-distance;
  else camera.position.z+=view==='back'?-distance:distance;
  controls.update();
  homeTargetY=controls.target.y;render();
 }
 function zoom(factor){
  homeView=null;
  const offset=camera.position.clone().sub(controls.target);
  offset.setLength(THREE.MathUtils.clamp(offset.length()*factor,controls.minDistance,controls.maxDistance));
  camera.position.copy(controls.target).add(offset);
  controls.update();render();
 }
 function resetView(){finishPlacement();pickingCenter=false;panMode=false;cameraMode();setHomeView();}
 function clearNameNodes(){for(const {button,line} of nameNodes.values()){button.remove();line.remove();}nameNodes.clear();}
 function unobscured(point){const distance=camera.position.distanceTo(point);labelRay.set(camera.position,point.clone().sub(camera.position).normalize());labelRay.far=Math.max(0,distance-.003);return labelRay.intersectObjects(bodyMeshes,false).length===0;}
 function layoutNames(){
  labelFrame=null;if(dead)return;
  const layer=$('.a-name-layer'),svg=$('.a-name-lines');layer.hidden=loading;
  if(layer.hidden){clearNameNodes();return;}
  const w=stage.clientWidth,h=stage.clientHeight,canvasH=renderer.domElement.clientHeight;
  if(!w||!h||!canvasH)return;
  svg.setAttribute('viewBox',`0 0 ${w} ${h}`);
  if(!visible||!names){layoutCurrentName(layer,svg,w,h,canvasH);return;}
  const columns=[[],[]],used=new Set(),byID=new Map(active().map(p=>[p.id,p]));
  for(const mesh of marks.children){if(!mesh.visible)continue;const p=byID.get(mesh.userData.id);if(!p)continue;const v=mesh.position.clone().project(camera);if(v.z< -1||v.z>1||v.x< -1||v.x>1||v.y< -1||v.y>1||!unobscured(mesh.position))continue;
   const x=(v.x+1)*w/2,y=(-v.y+1)*canvasH/2;
   const side=Math.abs(x-w/2)<12?(columns[0].length<=columns[1].length?0:1):x<w/2?0:1;
   columns[side].push({p,x,y});used.add(p.id);
  }
  for(const [id,node] of nameNodes)if(!used.has(id)){node.button.remove();node.line.remove();nameNodes.delete(id);}
  const width=Math.min(70,Math.max(42,w*.115)),top=62,bottom=h-48;
  for(let side=0;side<2;side++){
   const entries=columns[side].sort((a,b)=>a.y-b.y),step=Math.min(22,(bottom-top)/Math.max(1,entries.length));
   let previous=top-step;
   for(const e of entries){e.labelY=Math.max(top,e.y,previous+step);previous=e.labelY;}
   if(entries.length&&entries.at(-1).labelY>bottom){entries.at(-1).labelY=bottom;for(let i=entries.length-2;i>=0;i--)entries[i].labelY=Math.min(entries[i].labelY,entries[i+1].labelY-step);}
   for(const e of entries){const node=nameNode(e.p,layer,svg);node.button.classList.remove('a-focus-callout');node.line.style.display='';
    const x=side===0?7:w-7-width,endX=side===0?x+width+2:x-2;
    node.button.textContent=e.p.name;node.button.title=[e.p.name,e.p.side,e.p.code].filter(Boolean).join(' · ');node.button.setAttribute('aria-label','查看穴位：'+node.button.title);node.button.setAttribute('aria-pressed',String(e.p.id===selected));node.button.style.left=x+'px';node.button.style.top=e.labelY+'px';node.button.style.width=width+'px';
    node.line.setAttribute('points',`${e.x},${e.y} ${endX+(side===0?7:-7)},${e.labelY} ${endX},${e.labelY}`);node.line.classList.toggle('is-selected',e.p.id===selected);
   }
  }
 }
 function nameNode(p,layer,svg){let node=nameNodes.get(p.id);if(!node){const button=document.createElement('button'),line=document.createElementNS('http://www.w3.org/2000/svg','polyline');button.type='button';button.className='a-callout';button.dataset.point=p.id;line.classList.add('a-callout-line');svg.append(line);layer.append(button);node={button,line};nameNodes.set(p.id,node);}return node;}
 function layoutCurrentName(layer,svg,w,h,canvasH){
  const p=current();if(!p||!shown(p)){clearNameNodes();return;}
  for(const [id,node] of nameNodes)if(id!==p.id){node.button.remove();node.line.remove();nameNodes.delete(id);}
  const point=new THREE.Vector3(...p.position),v=point.clone().project(camera),inFrame=v.z>=-1&&v.z<=1&&Math.abs(v.x)<=1&&Math.abs(v.y)<=1;
  const exposed=inFrame&&unobscured(point),x=(v.x+1)*w/2,y=(-v.y+1)*canvasH/2;
  const node=nameNode(p,layer,svg),width=Math.min(130,Math.max(82,w*.19));
  // Retain the label side near the middle to avoid jitter while orbiting.
  if(node.focusSide===undefined)node.focusSide=inFrame?x>=w/2:false;
  if(inFrame){if(x<width+34)node.focusSide=false;else if(x>w-width-34)node.focusSide=true;}
  const leftSide=node.focusSide;
  const left=leftSide?9:w-9-width,labelY=exposed?THREE.MathUtils.clamp(y,64,h-55):68;
  const reason=!inFrame?'视野外':!exposed?'背侧 · 旋转查看':'';
  node.button.classList.add('a-focus-callout');node.button.setAttribute('aria-pressed','true');
  node.button.innerHTML=`<span class="a-focus-name">${esc(p.name)}${p.side?'<em>'+esc(p.side)+'</em>':''}</span><small>${esc(p.code||'自定义')}${reason?' · '+reason:''}</small>`;
  node.button.title=[p.name,p.side,p.code,reason].filter(Boolean).join(' · ');node.button.setAttribute('aria-label','当前穴位：'+node.button.title);
  node.button.style.left=left+'px';node.button.style.top=labelY+'px';node.button.style.width=width+'px';
  node.line.classList.add('is-selected');node.line.style.display=exposed?'':'none';
  const endX=leftSide?left+width+3:left-3;
  if(exposed)node.line.setAttribute('points',`${x},${y} ${endX+(leftSide?10:-10)},${labelY} ${endX},${labelY}`);else node.line.removeAttribute('points');
 }
 function render(){if(dead||!stage.clientWidth||!stage.clientHeight)return;
  // Keep the selected surface dot small and legible at every zoom distance.
  camera.updateMatrixWorld();for(const m of marks.children)if(m.userData.id===selected){const depth=-m.position.clone().applyMatrix4(camera.matrixWorldInverse).z;const unitsPerPixel=2*Math.max(.001,depth)*Math.tan(THREE.MathUtils.degToRad(camera.fov)/2)/Math.max(1,renderer.domElement.clientHeight);m.scale.setScalar(unitsPerPixel*3/.0065);}
  renderer.render(scene,camera);if(labelFrame===null)labelFrame=requestAnimationFrame(layoutNames);
  const p=current(),ring=$('.a-selected-ring'),summary=$('.a-selection');
  ring.hidden=true;summary.hidden=(!p&&visible)||loading;
  if(!p||loading){if(!p&&!visible)summary.textContent='仅显示当前穴位 · 请从列表选择';return;}
  const point=new THREE.Vector3(...p.position),v=point.clone().project(camera);
  const inFrame=v.z>=-1&&v.z<=1&&Math.abs(v.x)<=1&&Math.abs(v.y)<=1,exposed=inFrame&&unobscured(point);
  summary.textContent=`已选：${p.name}${p.side?' · '+p.side:''}${p.code?' · '+p.code:''}${!inFrame?' · 当前视野外':!exposed?' · 人体背侧，旋转查看':''}`;
  if(!shown(p)){summary.textContent=p.name+' · 所属经络已隐藏';return;}
  if(!exposed)return;
  ring.hidden=false;ring.style.left=(v.x+1)*renderer.domElement.clientWidth/2+'px';ring.style.top=(-v.y+1)*renderer.domElement.clientHeight/2+'px';
 }

 function clearMarks(){for(const m of [...marks.children]){marks.remove(m);m.geometry.dispose();m.material.dispose();}}
 function updateChannelTools(modelPoints){
  const counts=new Map();
  for(const p of modelPoints){const id=channelOf(p);counts.set(id,(counts.get(id)||0)+1);}
  if(channelFilter&&!counts.has(channelFilter))channelFilter='';
  for(const [id,name] of channels){
   const b=$('[data-channel='+id+']'),toggle=$('[data-channel-visibility='+id+']');
   const count=counts.get(id)||0,on=!hiddenChannels.has(id);
   b.querySelector('small').textContent=count;
   b.disabled=toggle.disabled=!count;
   b.setAttribute('aria-pressed',String(channelFilter===id));
   toggle.setAttribute('aria-pressed',String(on));
   toggle.title=(on?'隐藏':'显示')+name+'点位';
   toggle.setAttribute('aria-label',toggle.title);
  }
 }
 function updateList(modelPoints=active()){
  const group=channels.find(([id])=>id===channelFilter);
  $('.a-query-label').textContent=group?'查找'+group[1]+'点位':'查找穴位';
  $('[data-action=clear-filter]').hidden=!group;
  const q=$('#a-query').value.trim().toLowerCase();
  const filtered=modelPoints.filter(p=>(!channelFilter||channelOf(p)===channelFilter)&&(!q||`${p.name} ${p.code} ${p.side} ${p.meridian}`.toLowerCase().includes(q)));
  $('.a-list').innerHTML=filtered.map(p=>`<button type="button" data-point="${esc(p.id)}" aria-pressed="${p.id===selected}"><span>${esc(p.name)}${p.side?' · '+esc(p.side):''}</span><small>${esc(p.code||'自定义')}</small></button>`).join('')||'<p class="a-empty">没有匹配的穴位。</p>';
 }
 function rebuild(){
  clearMarks();
  const modelPoints=active();
  for(const p of modelPoints){
   const picked=p.id===selected,custom=!p.id.startsWith('preset-');
   const m=new THREE.Mesh(new THREE.SphereGeometry(custom?.008:.0065,16,12),new THREE.MeshStandardMaterial({
    color:picked?0xffcb51:custom?0x6abed8:0x72b9a2,emissive:picked?0x88571a:0x103730,roughness:.5
   }));
   m.position.fromArray(p.position);m.userData.id=p.id;m.visible=shown(p);marks.add(m);
  }
  updateChannelTools(modelPoints);updateList(modelPoints);render();
 }
 function updateVisibility(){
  const modelPoints=active(),byID=new Map(modelPoints.map(p=>[p.id,p]));
  for(const m of marks.children){const p=byID.get(m.userData.id);m.visible=!!p&&shown(p);}
  updateChannelTools(modelPoints);render();
 }
 function finishPlacement(){pickingCenter=false;placing=false;newPending=false;dragging=false;controls.enabled=true;stage.classList.remove('a-editing');$('[data-action=add]').textContent='手动标记';cameraMode();}
 function editor(){finishPlacement();const p=current();if(!p){$('.a-editor').innerHTML='<p class="a-empty">选择穴位查看记录，或手动标记新点位。</p>';return;}
 $('.a-editor').innerHTML=`<h2>${esc(p.name)}</h2><p class="a-note">${MODELS[sex].label}模型 · ${p.id.startsWith('preset-')?'预置示意穴位':'手动标记'} · 位置待校核</p>${[['name','名称'],['code','编号'],['side','侧别'],['meridian','经络'],['source','资料来源']].map(([k,l])=>`<label for="a-${k}">${l}</label><input id="a-${k}" data-field="${k}" value="${esc(p[k])}" maxlength="${k==='source'?1000:k==='name'||k==='meridian'?80:k==='side'?20:40}">`).join('')}<label for="a-description">穴位记录</label><textarea id="a-description" data-field="description" placeholder="记录定位依据、学习笔记和需要核对的内容" maxlength="4000">${esc(p.description)}</textarea><p class="a-coords">位置 ${p.position.map(n=>n.toFixed(3)).join(', ')}</p><div class="a-row"><button data-action="move">调整位置</button><button data-action="find">查找笔记</button><button class="a-danger" data-action="delete">删除点位</button></div>`;
 }
 function select(id){
  selected=id;
  const p=current();
  if(p){hiddenChannels.delete(channelOf(p));if(channelFilter&&channelFilter!==channelOf(p))channelFilter='';}
  editor();setRecords(true);rebuild();
 }
 // Project rough display anchors onto each loaded model independently. These are not medical calibrations.
 function generatePresets(targetSex){return presetAnchors(targetSex).map(p=>{let x=p.x*(targetSex==='female'?.9:1),y=p.y;const z=p.face<0?-1:1;ray.set(new THREE.Vector3(x,y,z*4),new THREE.Vector3(0,0,-z));let hit=ray.intersectObjects(bodyMeshes,false)[0];if(!hit){let nearest=null,d=Infinity;for(const mesh of bodyMeshes){if(mesh.name!=='Body')continue;const pos=mesh.geometry.attributes.position;for(let i=0;i<pos.count;i++){const v=new THREE.Vector3().fromBufferAttribute(pos,i).applyMatrix4(mesh.matrixWorld);const delta=(v.x-x)**2+(v.y-y)**2;if(delta<d){d=delta;nearest=v;}}}if(nearest){x=nearest.x;y=nearest.y;ray.set(new THREE.Vector3(x,y,z*4),new THREE.Vector3(0,0,-z));hit=ray.intersectObjects(bodyMeshes,false)[0];}}
 const position=hit?hit.point.clone().addScaledVector(new THREE.Vector3(0,0,z),.005).toArray():[x,y,0];return {id:p.id,model:p.model,name:p.name,code:p.code,side:p.side,meridian:p.meridian,description:p.description,source:p.source,position};});}
 async function switchModel(next){if(loading)return;loading=true;$('.a-model-loading').hidden=false;$('.a-model-loading').textContent='正在载入'+MODELS[next].label+'人体…';finishPlacement();selected='';
 try{let model=modelCache.get(next);if(!model){const gltf=await new GLTFLoader().loadAsync(new URL('./models/'+MODELS[next].file,import.meta.url).href);model=gltf.scene;modelCache.set(next,model);model.traverse(o=>{if(o.isMesh){o.frustumCulled=false;if(o.material?.name==='Skin'){o.material.roughness=.62;o.material.metalness=0;}}});}if(dead)return;body.clear();body.add(model);body.updateMatrixWorld(true);bounds.setFromObject(body);bodyMeshes=[];model.traverse(o=>{if(o.isMesh)bodyMeshes.push(o);});sex=next;
 const existing=new Set(points.map(p=>p.id));for(const p of generatePresets(sex))if(!existing.has(p.id))points.push(p);
 for(const b of root.querySelectorAll('[data-sex]'))b.setAttribute('aria-pressed',String(b.dataset.sex===sex));$('.a-model-name').textContent=MODELS[sex].label+'模型';setHomeView();editor();status();
 }finally{loading=false;if(!dead){$('.a-model-loading').hidden=true;rebuild();}}}
 function hit(e,objects){const r=renderer.domElement.getBoundingClientRect();mouse.set((e.clientX-r.left)/r.width*2-1,-(e.clientY-r.top)/r.height*2+1);ray.setFromCamera(mouse,camera);return ray.intersectObjects(objects,false)[0];}
 function chooseCenter(e){const h=hit(e,bodyMeshes);if(!h){message('请点击人体表面选择中心');return;}homeView=null;controls.target.copy(h.point);pickingCenter=false;cameraMode();controls.update();$('.a-hint').textContent='旋转中心已更新 · '+viewHint();message('');render();}
 function pointOnSurface(e){const h=hit(e,bodyMeshes);if(!h)return null;const n=h.face.normal.clone().transformDirection(h.object.matrixWorld);return h.point.clone().addScaledVector(n,.005).toArray().map(v=>Math.round(v*10000)/10000);}
 function move(e){const p=current(),pos=pointOnSurface(e);if(p&&pos){p.position=pos;changed();rebuild();const c=$('.a-coords');if(c)c.textContent='位置 '+p.position.map(n=>n.toFixed(3)).join(', ');}}
 renderer.domElement.addEventListener('pointerdown',e=>{start=e.button===0&&e.isPrimary?[e.clientX,e.clientY]:null;if(pickingCenter)return;if(placing){controls.enabled=false;dragging=true;renderer.domElement.setPointerCapture(e.pointerId);if(!newPending)move(e);}},{signal});
 renderer.domElement.addEventListener('pointermove',e=>{if(dragging&&!newPending)move(e);},{signal});
 renderer.domElement.addEventListener('pointerup',e=>{if(pickingCenter){if(e.button===0&&start&&Math.hypot(e.clientX-start[0],e.clientY-start[1])<5)chooseCenter(e);start=null;return;}if(dragging){if(newPending){const pos=pointOnSurface(e);if(pos){const p={id:uid(),model:MODELS[sex].id,name:'新标记',code:'',side:'',meridian:'',description:'',source:'',position:pos};points.push(p);changed();select(p.id);$('.a-editor').scrollIntoView({block:'nearest',behavior:'smooth'});$('#a-name').focus({preventScroll:true});}else message('请点击人体表面放置点位');}else{controls.enabled=true;dragging=false;}return;}if(e.button===0&&start&&Math.hypot(e.clientX-start[0],e.clientY-start[1])<5){const h=hit(e,[...marks.children.filter(m=>m.visible),...bodyMeshes]);if(h?.object.userData.id)select(h.object.userData.id);}start=null;},{signal});
 renderer.domElement.addEventListener('pointercancel',()=>{start=null;dragging=false;controls.enabled=!pickingCenter;},{signal});
 renderer.domElement.addEventListener('dblclick',e=>{if(freeView&&!placing)chooseCenter(e);},{signal});
 controls.addEventListener('start',()=>{homeView=null;});
 controls.addEventListener('change',()=>{
  const angle=Math.atan2(camera.position.x-controls.target.x,camera.position.z-controls.target.z);
  const direction=Math.abs(angle)<Math.PI/4?'front':Math.abs(angle)>3*Math.PI/4?'back':angle>0?'left':'right';
  $('.a-orientation').textContent={front:'正面',back:'背面',left:'左侧',right:'右侧'}[direction];
  for(const b of root.querySelectorAll('[data-view]'))b.setAttribute('aria-pressed',String(b.dataset.view===direction));
  render();
 });
 const observer=new ResizeObserver(()=>{
  const w=stage.clientWidth,h=stage.clientHeight;
  if(!w||!h)return;
  renderer.setSize(w,h,false);camera.aspect=w/h;camera.updateProjectionMatrix();
  if(homeView)setHomeView(homeView);else render();
 });
 observer.observe(stage);

 root.addEventListener('input',e=>{if(e.target.id==='a-query'){updateList();return;}const p=current(),field=e.target.dataset.field;if(p&&field){p[field]=e.target.value;changed();if(field==='name')$('.a-editor h2').textContent=p.name;updateVisibility();updateList();}},{signal});
 const warn=()=>!dirty||confirm('有未保存的图谱修改，确定放弃吗？');
 async function load(){const data=await request();if(dead)return;points=data.points;revision=data.revision;dirty=false;selected='';const old=points.filter(p=>!p.model).length;$('.a-legacy').hidden=!old;$('.a-legacy').textContent=`已保留 ${old} 个旧版模型标记，可导出查看；旧坐标不会套用到新人体。`;await switchModel(sex);status();message('');}
 root.addEventListener('click',async e=>{const b=e.target.closest('button');if(!b||saving)return;try{
 if(b.dataset.sex){if(b.dataset.sex!==sex)await switchModel(b.dataset.sex);return;}
 if(b.dataset.channel){
  channelFilter=channelFilter===b.dataset.channel?'':b.dataset.channel;
  $('#a-query').value='';updateChannelTools(active());updateList();setRecords(true);return;
 }
 if(b.dataset.channelVisibility){
  const id=b.dataset.channelVisibility;
  if(hiddenChannels.has(id))hiddenChannels.delete(id);else hiddenChannels.add(id);
  updateVisibility();return;
 }
 if(b.dataset.point){select(b.dataset.point);return;}
 if(b.dataset.view){finishPlacement();setHomeView(b.dataset.view);return;}
 const a=b.dataset.action;
 if(a==='fullscreen'){toggleFullscreen();return;}
 if(a==='records'){setRecords(records.hidden);if(records.hidden)recordToggle.focus();return;}
 if(a==='channels'){
  const open=!atlas.classList.contains('a-channels-open');if(open)setRecords(false);
  setChannels(open);if(!open)channelToggle.focus();return;
 }
 if(a==='clear-filter'){channelFilter='';$('#a-query').value='';updateChannelTools(active());updateList();$('#a-query').focus();return;}
 if(a==='names'){names=!names;b.setAttribute('aria-pressed',String(names));render();return;}
 if(a==='current-only'){visible=!visible;b.setAttribute('aria-pressed',String(!visible));updateVisibility();return;}
 if(a==='show-all'||a==='hide-all'){
  hiddenChannels.clear();
  if(a==='hide-all')for(const [id] of channels)hiddenChannels.add(id);
  visible=true;$('[data-action=current-only]').setAttribute('aria-pressed','false');updateVisibility();return;
 }
 if(a==='zoom-in'||a==='zoom-out'){zoom(a==='zoom-in'?.8:1.25);return;}
 if(a==='reset-view'){resetView();return;}
 if(a==='free-view'){finishPlacement();freeView=!freeView;panMode=false;pickingCenter=false;if(!freeView){const offset=camera.position.clone().sub(controls.target);controls.target.set(0,homeTargetY,0);camera.position.copy(controls.target).add(offset);}cameraMode();controls.update();render();return;}
 if(a==='center'&&freeView){const next=!pickingCenter;finishPlacement();pickingCenter=next;cameraMode();return;}
 if(a==='pan'&&freeView){const next=!panMode;finishPlacement();panMode=next;cameraMode();return;}

 if(a==='reload'||a==='export')b.closest('details').open=false;
 if(a==='reload'&&!loading&&warn())await load();
 if(a==='add'){if(loading)return;if(placing){editor();if(current())setRecords(true);render();return;}if(points.length>=500)throw Error('最多 500 个标注');finishPlacement();setRecords(false);setChannels(false);placing=true;newPending=true;selected='';rebuild();stage.classList.add('a-editing');b.textContent='取消标记';$('.a-hint').textContent='点击人体表面添加点位';stage.scrollIntoView({block:'nearest'});render();}
 if(a==='save'){saving=true;status();try{const data=await request('PUT',{revision,points});revision=data.revision;dirty=false;message('');rebuild();}finally{saving=false;status();}}
 if(a==='move'&&current()){finishPlacement();setRecords(false);placing=true;hiddenChannels.delete(channelOf(current()));stage.classList.add('a-editing');$('[data-action=add]').textContent='结束调整';rebuild();$('.a-hint').textContent='点击或拖动人体表面调整位置';stage.scrollIntoView({block:'nearest'});}
 if(a==='delete'&&current()&&confirm('删除这个点位及其记录？保存后生效。')){if(selected.startsWith('preset-'))current().hidden=true;else points=points.filter(p=>p.id!==selected);selected='';changed();editor();rebuild();}
 if(a==='export'){const value=JSON.stringify({format:'linji-anatomy-v2',clinical_validated:false,revision,points},null,2);if(native){if(navigator.clipboard&&isSecureContext)await navigator.clipboard.writeText(value);else{const text=document.createElement('textarea');text.value=value;root.append(text);text.select();const ok=document.execCommand('copy');text.remove();if(!ok)throw Error('当前设备不支持复制，请在网页端导出');}$('.a-status').textContent='图谱 JSON 已复制';}else{const url=URL.createObjectURL(new Blob([value],{type:'application/json'}));const link=document.createElement('a');link.href=url;link.download='穴位图谱.json';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}}
 if(a==='find'){const p=current();if(p){if(!warn())return;dirty=false;status();const q=[p.name,p.code].filter(Boolean).join(' ');if(!native)root.dispatchEvent(new CustomEvent('anatomy-search',{detail:q,bubbles:true}));else location.href=base+'/?anatomy_q='+encodeURIComponent(q)+'#search';}}
 }catch(err){if(!dead)message(err.message);}},{signal});
 window.addEventListener('beforeunload',e=>{if(dirty){e.preventDefault();e.returnValue='';}},{signal});
 function dispose(){dead=true;if(fullscreen.open)fullscreen.close();if(labelFrame!==null)cancelAnimationFrame(labelFrame);clearNameNodes();abort.abort();observer.disconnect();controls.dispose();clearMarks();for(const model of modelCache.values())model.traverse(o=>{o.geometry?.dispose();for(const m of (Array.isArray(o.material)?o.material:[o.material]))if(m){for(const v of Object.values(m))if(v?.isTexture)v.dispose();m.dispose();}});renderer.dispose();renderer.forceContextLoss();}
 try{await load();}catch(e){dispose();throw e;}
 return {hasChanges:()=>dirty,dispose};
}
