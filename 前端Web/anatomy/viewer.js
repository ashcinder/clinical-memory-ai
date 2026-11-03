import * as THREE from './vendor/three.module.js';
import {OrbitControls} from './vendor/OrbitControls.js';
import {GLTFLoader} from './vendor/GLTFLoader.js';
import {MODELS,presetAnchors} from './presets.js';
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const uid=()=>Array.from(crypto.getRandomValues(new Uint8Array(16)),v=>v.toString(16).padStart(2,'0')).join('');
export async function mountAtlas(root,base=''){
 let dead=false,dirty=false,points=[],revision='',selected='',placing=false,newPending=false,dragging=false,start=null,sex='male',visible=true,saving=false,freeView=false,pickingCenter=false,panMode=false,homeView='front',homeTargetY=1;
 const abort=new AbortController(),signal=abort.signal;
 root.innerHTML=`<section class="atlas"><div class="atlas-head"><div class="a-sex" role="group" aria-label="人体模型"><button data-sex="male" aria-pressed="true">男性</button><button data-sex="female" aria-pressed="false">女性</button></div><span class="a-status" role="status">正在载入人体…</span><button data-action="reload">刷新</button><button data-action="export">导出 JSON</button><button data-action="add">手动标记</button><button class="a-primary" data-action="save" disabled>保存修改</button></div><div class="a-message" role="alert" hidden></div><div class="atlas-layout"><div class="atlas-stage"><div class="a-camera-bar"><div class="a-views"><button data-view="front">正面</button><button data-view="back">背面</button><button data-view="left">侧面</button><button data-action="reset-view">复位</button><button data-action="free-view" aria-pressed="false">自由视角</button><button data-action="center" aria-pressed="false" disabled>选择中心</button><button data-action="pan" aria-pressed="false" disabled>平移</button><label class="a-visibility a-all-points"><input type="checkbox" id="a-show" aria-label="仅显示当前穴位"><span class="a-switch-track" aria-hidden="true"></span><span>仅当前穴位</span></label></div><div class="a-selection" role="status" aria-live="polite" hidden></div></div><span class="a-orientation">正面</span><span class="a-stage-badge">穴位位置待校核 · 不用于临床操作定位</span><span class="a-hint">拖动旋转 · 滚轮 / 双指缩放 · 点击穴位查看</span><div class="a-name-layer" aria-label="当前可见穴位名称"><svg class="a-name-lines" aria-hidden="true"></svg></div><div class="a-selected-ring" hidden aria-hidden="true"></div><div class="a-model-loading" role="status">正在载入写实人体…</div></div><aside class="atlas-side"><div class="a-side-head"><strong>穴位与记录</strong><small class="a-model-name">男性模型</small></div><label for="a-query">查找穴位</label><input id="a-query" type="search" placeholder="名称、编号、侧别或经络"><div class="a-list" aria-label="穴位列表"></div><div class="a-editor"></div><p class="a-note">男女模型分别保存点位。修改后请保存。</p><p class="a-legacy" hidden></p></aside></div></section>`;
 const $=s=>root.querySelector(s),stage=$('.atlas-stage'),native=root.id==='atlas-root';
 if(native)$('[data-action=export]').textContent='复制 JSON';
 const message=e=>{const m=$('.a-message');m.hidden=!e;m.textContent=e||'';};
 const active=()=>points.filter(p=>p.model===MODELS[sex].id&&!p.hidden);
 const current=()=>points.find(p=>p.id===selected&&!p.hidden);
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
 const body=new THREE.Group(),marks=new THREE.Group();scene.add(body,marks);
 const ray=new THREE.Raycaster(),mouse=new THREE.Vector2(),modelCache=new Map();
 let bodyMeshes=[],loading=false;
 const labelRay=new THREE.Raycaster(),nameNodes=new Map();let labelFrame=null,canvasTop=100;
 function viewHint(){return freeView?'自由视角 · 拖动旋转 / 平移 · 右键平移 · 双指移动缩放 · 双击人体设中心':'拖动旋转 · 滚轮 / 双指缩放 · 点击穴位查看';}
 function cameraMode(){controls.enablePan=freeView;controls.zoomToCursor=freeView;controls.minDistance=freeView?.1:.4;controls.maxPolarAngle=freeView?Math.PI-.01:Math.PI*.95;controls.mouseButtons.LEFT=panMode?THREE.MOUSE.PAN:THREE.MOUSE.ROTATE;controls.mouseButtons.MIDDLE=freeView?THREE.MOUSE.PAN:THREE.MOUSE.DOLLY;controls.touches.ONE=panMode?THREE.TOUCH.PAN:THREE.TOUCH.ROTATE;controls.enabled=!pickingCenter;
 $('[data-action=free-view]').setAttribute('aria-pressed',String(freeView));for(const action of ['center','pan'])$('[data-action='+action+']').disabled=!freeView;
 $('[data-action=center]').setAttribute('aria-pressed',String(pickingCenter));$('[data-action=pan]').setAttribute('aria-pressed',String(panMode));stage.classList.toggle('a-pick-center',pickingCenter);$('.a-hint').textContent=pickingCenter?'点击人体表面，设为旋转与缩放中心':viewHint();}
 function setHomeView(view='front'){
  homeView=view;controls.target.set(0,1,0);camera.position.set(view==='left'?3.75:0,1.05,view==='back'?-3.75:view==='left'?0:3.75);controls.update();
  // Align the crown near the top of the model viewport, including wide-screen layouts.
  const height=renderer.domElement.clientHeight;
  for(let i=0;i<2&&height>0;i++){camera.updateMatrixWorld();const crown=new THREE.Vector3(0,2.02,0).project(camera),gap=(1-crown.y)*height/2;
   const shift=Math.max(0,gap-24)*2*camera.position.distanceTo(controls.target)*Math.tan(THREE.MathUtils.degToRad(camera.fov)/2)/height;
   camera.position.y-=shift;controls.target.y-=shift;controls.update();}
  homeTargetY=controls.target.y;render();
 }
 function resetView(){finishPlacement();pickingCenter=false;panMode=false;cameraMode();setHomeView();}
 function clearNameNodes(){for(const {button,line} of nameNodes.values()){button.remove();line.remove();}nameNodes.clear();}
 function unobscured(point){const distance=camera.position.distanceTo(point);labelRay.set(camera.position,point.clone().sub(camera.position).normalize());labelRay.far=Math.max(0,distance-.003);return labelRay.intersectObjects(bodyMeshes,false).length===0;}
 function layoutNames(){
  labelFrame=null;if(dead)return;
  const layer=$('.a-name-layer'),svg=$('.a-name-lines');layer.hidden=loading;
  if(layer.hidden){clearNameNodes();return;}
  const w=stage.clientWidth,h=stage.clientHeight,canvasH=renderer.domElement.clientHeight;
  svg.setAttribute('viewBox',`0 0 ${w} ${h}`);
  if(!visible){layoutCurrentName(layer,svg,w,h,canvasH);return;}
  const columns=[[],[]],used=new Set(),byID=new Map(active().map(p=>[p.id,p]));
  for(const mesh of marks.children){if(!mesh.visible)continue;const p=byID.get(mesh.userData.id);if(!p)continue;const v=mesh.position.clone().project(camera);if(v.z< -1||v.z>1||v.x< -1||v.x>1||v.y< -1||v.y>1||!unobscured(mesh.position))continue;
   const x=(v.x+1)*w/2,y=canvasTop+(-v.y+1)*canvasH/2;
   const side=Math.abs(x-w/2)<12?(columns[0].length<=columns[1].length?0:1):x<w/2?0:1;
   columns[side].push({p,x,y});used.add(p.id);
  }
  for(const [id,node] of nameNodes)if(!used.has(id)){node.button.remove();node.line.remove();nameNodes.delete(id);}
  const width=Math.min(86,Math.max(48,w*.135)),top=canvasTop+8,bottom=h-86;
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
  const p=current();for(const [id,node] of nameNodes)if(id!==p?.id){node.button.remove();node.line.remove();nameNodes.delete(id);}if(!p)return;
  const point=new THREE.Vector3(...p.position),v=point.clone().project(camera),inFrame=v.z>=-1&&v.z<=1&&Math.abs(v.x)<=1&&Math.abs(v.y)<=1;
  const exposed=inFrame&&unobscured(point),x=(v.x+1)*w/2,y=canvasTop+(-v.y+1)*canvasH/2;
  const node=nameNode(p,layer,svg),width=Math.min(148,Math.max(104,w*.19));
  // Retain the label side near the middle to avoid jitter while orbiting.
  if(node.focusSide===undefined)node.focusSide=inFrame?x>=w/2:false;
  if(inFrame){if(x<width+34)node.focusSide=false;else if(x>w-width-34)node.focusSide=true;}
  const leftSide=node.focusSide;
  const left=leftSide?12:w-12-width,labelY=exposed?y:canvasTop+32;
  const reason=!inFrame?'视野外':!exposed?'背侧 · 旋转查看':'';
  node.button.classList.add('a-focus-callout');node.button.setAttribute('aria-pressed','true');
  node.button.innerHTML=`<span class="a-focus-name">${esc(p.name)}${p.side?'<em>'+esc(p.side)+'</em>':''}</span><small>${esc(p.code||'自定义')}${reason?' · '+reason:''}</small>`;
  node.button.title=[p.name,p.side,p.code,reason].filter(Boolean).join(' · ');node.button.setAttribute('aria-label','当前穴位：'+node.button.title);
  node.button.style.left=left+'px';node.button.style.top=labelY+'px';node.button.style.width=width+'px';
  node.line.classList.add('is-selected');node.line.style.display=exposed?'':'none';
  const endX=leftSide?left+width+3:left-3;
  if(exposed)node.line.setAttribute('points',`${x},${y} ${endX+(leftSide?10:-10)},${labelY} ${endX},${labelY}`);else node.line.removeAttribute('points');
 }
 function render(){if(dead)return;
  // Keep the selected surface dot small and legible at every zoom distance.
  camera.updateMatrixWorld();for(const m of marks.children)if(m.userData.id===selected){const depth=-m.position.clone().applyMatrix4(camera.matrixWorldInverse).z;const unitsPerPixel=2*Math.max(.001,depth)*Math.tan(THREE.MathUtils.degToRad(camera.fov)/2)/Math.max(1,renderer.domElement.clientHeight);m.scale.setScalar(unitsPerPixel*3/.0065);}
  renderer.render(scene,camera);if(labelFrame===null)labelFrame=requestAnimationFrame(layoutNames);
  const p=current(),ring=$('.a-selected-ring'),summary=$('.a-selection');
  ring.hidden=true;summary.hidden=(!p&&visible)||loading;
  if(!p||loading){if(!p&&!visible)summary.textContent='仅显示当前穴位 · 请从列表选择';return;}
  const point=new THREE.Vector3(...p.position),v=point.clone().project(camera);
  const inFrame=v.z>=-1&&v.z<=1&&Math.abs(v.x)<=1&&Math.abs(v.y)<=1,exposed=inFrame&&unobscured(point);
  summary.textContent=`已选：${p.name}${p.side?' · '+p.side:''}${p.code?' · '+p.code:''}${!inFrame?' · 当前视野外':!exposed?' · 人体背侧，旋转查看':''}`;
  if(!exposed)return;
  ring.hidden=false;ring.style.left=(v.x+1)*renderer.domElement.clientWidth/2+'px';ring.style.top=canvasTop+(-v.y+1)*renderer.domElement.clientHeight/2+'px';
 }

 function clearMarks(){for(const m of [...marks.children]){marks.remove(m);m.geometry.dispose();m.material.dispose();}}
 function rebuild(){clearMarks();const q=$('#a-query').value.trim().toLowerCase();const filtered=active().filter(p=>!q||`${p.name} ${p.code} ${p.side} ${p.meridian}`.toLowerCase().includes(q));for(const p of active()){const picked=p.id===selected,custom=!p.id.startsWith('preset-');const m=new THREE.Mesh(new THREE.SphereGeometry(picked ? .0065 : custom ? .0075 : .0065,16,12),new THREE.MeshStandardMaterial({color:picked?0xb97736:custom?0x315cab:0x237b69,emissive:picked?0x3b2009:0x042018,roughness:.5}));m.position.fromArray(p.position);m.userData.id=p.id;m.visible=visible||picked;marks.add(m);}
 $('.a-list').innerHTML=filtered.map(p=>`<button type="button" data-point="${esc(p.id)}" aria-pressed="${p.id===selected}"><span>${esc(p.name)}${p.side?' · '+esc(p.side):''}</span><small>${esc(p.code||'自定义')}</small></button>`).join('')||'<p class="a-empty">没有匹配的穴位，可手动标记。</p>';render();}
 function finishPlacement(){pickingCenter=false;placing=false;newPending=false;dragging=false;controls.enabled=true;stage.classList.remove('a-editing');$('[data-action=add]').textContent='手动标记';cameraMode();}
 function editor(){finishPlacement();const p=current();if(!p){$('.a-editor').innerHTML='<p class="a-empty">选择穴位查看记录，或手动标记新点位。</p>';return;}
 $('.a-editor').innerHTML=`<h2>${esc(p.name)}</h2><p class="a-note">${MODELS[sex].label}模型 · ${p.id.startsWith('preset-')?'预置示意穴位':'手动标记'} · 位置待校核</p>${[['name','名称'],['code','编号'],['side','侧别'],['meridian','经络'],['source','资料来源']].map(([k,l])=>`<label for="a-${k}">${l}</label><input id="a-${k}" data-field="${k}" value="${esc(p[k])}" maxlength="${k==='source'?1000:k==='name'||k==='meridian'?80:k==='side'?20:40}">`).join('')}<label for="a-description">穴位记录</label><textarea id="a-description" data-field="description" placeholder="记录定位依据、学习笔记和需要核对的内容" maxlength="4000">${esc(p.description)}</textarea><p class="a-coords">位置 ${p.position.map(n=>n.toFixed(3)).join(', ')}</p><div class="a-row"><button data-action="move">调整位置</button><button data-action="find">查找笔记</button><button class="a-danger" data-action="delete">删除点位</button></div>`;
 }
 function select(id){selected=id;editor();rebuild();}
 // Project rough display anchors onto each loaded model independently. These are not medical calibrations.
 function generatePresets(targetSex){return presetAnchors(targetSex).map(p=>{let x=p.x*(targetSex==='female'?.9:1),y=p.y;const z=p.face<0?-1:1;ray.set(new THREE.Vector3(x,y,z*4),new THREE.Vector3(0,0,-z));let hit=ray.intersectObjects(bodyMeshes,false)[0];if(!hit){let nearest=null,d=Infinity;for(const mesh of bodyMeshes){if(mesh.name!=='Body')continue;const pos=mesh.geometry.attributes.position;for(let i=0;i<pos.count;i++){const v=new THREE.Vector3().fromBufferAttribute(pos,i).applyMatrix4(mesh.matrixWorld);const delta=(v.x-x)**2+(v.y-y)**2;if(delta<d){d=delta;nearest=v;}}}if(nearest){x=nearest.x;y=nearest.y;ray.set(new THREE.Vector3(x,y,z*4),new THREE.Vector3(0,0,-z));hit=ray.intersectObjects(bodyMeshes,false)[0];}}
 const position=hit?hit.point.clone().addScaledVector(new THREE.Vector3(0,0,z),.005).toArray():[x,y,0];return {id:p.id,model:p.model,name:p.name,code:p.code,side:p.side,meridian:p.meridian,description:p.description,source:p.source,position};});}
 async function switchModel(next){if(loading)return;loading=true;$('.a-model-loading').hidden=false;$('.a-model-loading').textContent='正在载入'+MODELS[next].label+'人体…';finishPlacement();selected='';
 try{let model=modelCache.get(next);if(!model){const gltf=await new GLTFLoader().loadAsync(new URL('./models/'+MODELS[next].file,import.meta.url).href);model=gltf.scene;modelCache.set(next,model);model.traverse(o=>{if(o.isMesh){o.frustumCulled=false;if(o.material?.name==='Skin'){o.material.roughness=.62;o.material.metalness=0;}}});}if(dead)return;body.clear();body.add(model);body.updateMatrixWorld(true);bodyMeshes=[];model.traverse(o=>{if(o.isMesh)bodyMeshes.push(o);});sex=next;
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
 controls.addEventListener('change',()=>{const angle=Math.atan2(camera.position.x-controls.target.x,camera.position.z-controls.target.z);$('.a-orientation').textContent=Math.abs(angle)<.65?'正面':Math.abs(angle)>2.5?'背面':'侧面';render();});
 const observer=new ResizeObserver(()=>{const views=$('.a-views'),bar=$('.a-camera-bar'),bottom=12+views.offsetHeight;canvasTop=bar.offsetHeight+8;stage.style.setProperty('--a-controls-bottom',bottom+'px');const w=stage.clientWidth,h=Math.max(160,stage.clientHeight-canvasTop-78);renderer.domElement.style.top=canvasTop+'px';renderer.setSize(w,h);camera.aspect=w/h;
 // Keep the full-width body in frame when the desktop panel becomes tall and narrow.
 camera.fov=THREE.MathUtils.radToDeg(2*Math.atan(Math.tan(THREE.MathUtils.degToRad(35)/2)*Math.max(1,.72/camera.aspect)));camera.updateProjectionMatrix();if(homeView)setHomeView(homeView);else render();});observer.observe(stage);observer.observe($('.a-camera-bar'));

 root.addEventListener('input',e=>{if(e.target.id==='a-query'){rebuild();return;}if(e.target.id==='a-show'){visible=!e.target.checked;for(const m of marks.children)m.visible=visible||m.userData.id===selected;render();return;}const p=current();if(p&&e.target.dataset.field){p[e.target.dataset.field]=e.target.value;changed();if(e.target.dataset.field==='name')$('.a-editor h2').textContent=p.name;render();}},{signal});
 const warn=()=>!dirty||confirm('有未保存的图谱修改，确定放弃吗？');
 async function load(){const data=await request();if(dead)return;points=data.points;revision=data.revision;dirty=false;selected='';const old=points.filter(p=>!p.model).length;$('.a-legacy').hidden=!old;$('.a-legacy').textContent=`已保留 ${old} 个旧版模型标记，可导出查看；旧坐标不会套用到新人体。`;await switchModel(sex);status();message('');}
 root.addEventListener('click',async e=>{const b=e.target.closest('button');if(!b||saving)return;try{
 if(b.dataset.sex){if(b.dataset.sex!==sex)await switchModel(b.dataset.sex);return;}
 if(b.dataset.point){select(b.dataset.point);return;}
 if(b.dataset.view){finishPlacement();setHomeView(b.dataset.view);return;}
 const a=b.dataset.action;
 if(a==='reset-view'){resetView();return;}
 if(a==='free-view'){finishPlacement();freeView=!freeView;panMode=false;pickingCenter=false;if(!freeView){const offset=camera.position.clone().sub(controls.target);controls.target.set(0,homeTargetY,0);camera.position.copy(controls.target).add(offset);}cameraMode();controls.update();render();return;}
 if(a==='center'&&freeView){const next=!pickingCenter;finishPlacement();pickingCenter=next;cameraMode();return;}
 if(a==='pan'&&freeView){const next=!panMode;finishPlacement();panMode=next;cameraMode();return;}

 if(a==='reload'&&warn())await load();
 if(a==='add'){if(loading)return;if(placing&&newPending){finishPlacement();return;}if(points.length>=500)throw Error('最多 500 个标注');finishPlacement();placing=true;newPending=true;selected='';rebuild();stage.classList.add('a-editing');b.textContent='取消标记';$('.a-hint').textContent='点击人体表面添加点位，然后填写穴位记录';stage.scrollIntoView({block:'nearest',behavior:'smooth'});render();}
 if(a==='save'){saving=true;status();try{const data=await request('PUT',{revision,points});revision=data.revision;dirty=false;message('');rebuild();}finally{saving=false;status();}}
 if(a==='move'&&current()){placing=!placing;newPending=false;stage.classList.toggle('a-editing',placing);b.textContent=placing?'结束调整':'调整位置';rebuild();$('.a-hint').textContent=placing?'点击或拖动人体表面调整位置；结束后恢复旋转':viewHint();stage.scrollIntoView({block:'nearest',behavior:'smooth'});}
 if(a==='delete'&&current()&&confirm('删除这个点位及其记录？保存后生效。')){if(selected.startsWith('preset-'))current().hidden=true;else points=points.filter(p=>p.id!==selected);selected='';changed();editor();rebuild();}
 if(a==='export'){const value=JSON.stringify({format:'linji-anatomy-v2',clinical_validated:false,revision,points},null,2);if(native){if(navigator.clipboard&&isSecureContext)await navigator.clipboard.writeText(value);else{const text=document.createElement('textarea');text.value=value;root.append(text);text.select();const ok=document.execCommand('copy');text.remove();if(!ok)throw Error('当前设备不支持复制，请在网页端导出');}$('.a-status').textContent='图谱 JSON 已复制';}else{const url=URL.createObjectURL(new Blob([value],{type:'application/json'}));const link=document.createElement('a');link.href=url;link.download='穴位图谱.json';link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}}
 if(a==='find'){const p=current();if(p){if(!warn())return;dirty=false;status();const q=p.name+' '+p.code;if(!native)root.dispatchEvent(new CustomEvent('anatomy-search',{detail:q}));else location.href=base+'/?anatomy_q='+encodeURIComponent(q)+'#search';}}
 }catch(err){if(!dead)message(err.message);}},{signal});
 window.addEventListener('beforeunload',e=>{if(dirty){e.preventDefault();e.returnValue='';}},{signal});
 function dispose(){dead=true;if(labelFrame!==null)cancelAnimationFrame(labelFrame);clearNameNodes();abort.abort();observer.disconnect();controls.dispose();clearMarks();for(const model of modelCache.values())model.traverse(o=>{o.geometry?.dispose();for(const m of (Array.isArray(o.material)?o.material:[o.material]))if(m){for(const v of Object.values(m))if(v?.isTexture)v.dispose();m.dispose();}});renderer.dispose();renderer.forceContextLoss();}
 try{await load();}catch(e){dispose();throw e;}
 return {hasChanges:()=>dirty,dispose};
}
