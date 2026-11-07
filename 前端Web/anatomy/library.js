import * as THREE from './vendor/three.module.js';
import {OrbitControls} from './vendor/OrbitControls.js';
import {GLTFLoader} from './vendor/GLTFLoader.js';

const icons = {
 back: '<path d="m15 5-7 7 7 7"/>',
 expand: '<path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/>',
 close: '<path d="m6 6 12 12M18 6 6 18"/>',
 models: '<rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/>',
 parts: '<path d="m12 3 9 5-9 5-9-5 9-5Zm-9 9 9 5 9-5M3 16l9 5 9-5"/>',
 isolate: '<path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/><circle cx="12" cy="12" r="3"/>',
 hide: '<path d="M3 3l18 18M10 6a11 11 0 0 1 11 6 12 12 0 0 1-3 3M6 7a12 12 0 0 0-3 5 11 11 0 0 0 12 6"/>',
 pan: '<path d="M12 2v20M2 12h20M9 5l3-3 3 3m-6 14 3 3 3-3M5 9l-3 3 3 3m14-6 3 3-3 3"/>',
 capture: '<path d="M4 7h4l2-3h4l2 3h4v13H4z"/><circle cx="12" cy="13" r="3"/>',
 find: '<path d="M5 3h11l3 3v15H5zM8 10h8m-8 4h8m-8 4h5"/>',
 info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5m0-9v.1"/>',
 eye: '<path d="M2 12s4-6 10-6 10 6 10 6-4 6-10 6-10-6-10-6Z"/><circle cx="12" cy="12" r="2.4"/>',
 up: '<path d="m5 15 7-7 7 7"/>',
 down: '<path d="m5 9 7 7 7-7"/>',
 left: '<path d="m15 5-7 7 7 7"/>',
 right: '<path d="m9 5 7 7-7 7"/>',
};
const glyph = name => '<svg viewBox="0 0 24 24" aria-hidden="true">' + icons[name] + '</svg>';
const shortName = {skeleton:'骨骼', heart:'心脏', airways:'气道', skull:'颅骨', knee:'膝部', shoulder:'肌群', kidneys:'双肾'};
const minViewScale = .75;
const perspectiveFov = 33;
const perspectiveSlope = Math.tan(THREE.MathUtils.degToRad(perspectiveFov / 2));

function disposeModel(model) {
 model?.traverse(node => {
  if (!node.isMesh) return;
  node.geometry.dispose();
  node.material.dispose();
 });
}

export function mountLibrary(root, base = '') {
 root.innerHTML = `<section class="medical-library" aria-label="医学三维工作台">
  <header class="ml-bar">
   <button class="ml-back" type="button" aria-label="返回穴位图谱"></button>
   <div class="ml-heading"><h3 class="ml-title">人体解剖</h3></div>
   <span class="ml-category"></span><span class="ml-live" role="status">载入目录</span>
   <button class="ml-expand" type="button" aria-label="进入全屏" title="全屏"></button>
   <button class="ml-exit" type="button" aria-label="退出全屏" hidden></button>
  </header>
  <div class="ml-workbench">
   <aside class="ml-side" aria-label="模型和结构">
    <div class="ml-panel-tabs" role="group" aria-label="侧栏内容">
     <button type="button" data-panel="models">模型</button><button type="button" data-panel="parts">结构</button>
    </div>
    <div class="ml-catalog" hidden>
     <form class="ml-search"><label class="ml-sr" for="ml-keyword">查找模型或结构</label>
      <input id="ml-keyword" type="search" maxlength="100" placeholder="名称或 FMA 编号">
     </form>
     <div class="ml-topics" aria-label="解剖系统"></div>
     <p class="ml-count" role="status"></p>
     <div class="ml-results" aria-label="模型列表"></div>
    </div>
    <div class="ml-parts">
     <div class="ml-parts-head"><span>结构显隐</span><span class="ml-part-count"></span></div>
     <div class="ml-visibility" role="group" aria-label="批量显隐">
      <button type="button" data-action="show-all" disabled>全显</button><button type="button" data-action="hide-all" disabled>全隐</button>
     </div>
     <div class="ml-part-list"></div>
    </div>
    <div class="ml-info" hidden>
     <strong>模型来源</strong><p class="ml-summary"></p><p class="ml-credit"></p>
     <a class="ml-source" rel="noopener noreferrer">数据来源 ↗</a>
     <a class="ml-license" rel="noopener noreferrer">CC BY 4.0 ↗</a>
    </div>
   </aside>
   <div class="ml-stage">
    <div class="ml-quick" role="group" aria-label="快速切换模型"></div>
    <div class="ml-viewport" aria-label="可旋转与缩放的三维模型"></div>
    <div class="ml-message" role="status"><p>正在读取解剖目录…</p><button type="button" data-action="reload" hidden>重新载入</button></div>
    <div class="ml-stage-footer">
     <div class="ml-stage-copy">
      <div class="ml-stage-readout">
       <strong class="ml-selected" role="status">模型总览</strong><small class="ml-selected-id"></small>
      </div>
      <p class="ml-hint">拖动旋转 · 滚轮或双指缩放 · 点选结构</p>
     </div>
     <div class="ml-camera" role="group" aria-label="视角与缩放">
      <div class="ml-compass" role="group" aria-label="固定视角">
       <button type="button" data-view="front" aria-label="正面" disabled>前</button>
       <button type="button" data-view="back" aria-label="背面" disabled>后</button>
       <button type="button" data-view="left" aria-label="左侧" disabled>左</button>
       <button type="button" data-view="right" aria-label="右侧" disabled>右</button>
       <button type="button" data-action="projection" aria-label="切换正交与透视" aria-pressed="true" disabled>正交</button>
      </div>
      <div class="ml-zoom" role="group" aria-label="缩放与复位">
       <button type="button" data-action="zoom-out" aria-label="缩小" disabled>−</button>
       <button type="button" data-view="reset" title="恢复完整正面视图" disabled>适屏</button>
       <button type="button" data-action="zoom-in" aria-label="放大" disabled>＋</button>
       <details class="ml-adjust">
        <summary>微调</summary>
        <div class="ml-orbit" role="group" aria-label="微调旋转">
         <button type="button" data-orbit="up" aria-label="向上旋转" disabled></button>
         <button type="button" data-orbit="left" aria-label="向左旋转" disabled></button>
         <span aria-hidden="true"></span>
         <button type="button" data-orbit="right" aria-label="向右旋转" disabled></button>
         <button type="button" data-orbit="down" aria-label="向下旋转" disabled></button>
        </div>
       </details>
      </div>
     </div>
    </div>
   </div>
   <div class="ml-tools" role="group" aria-label="模型工具">
    <button type="button" data-tool="models">目录</button>
    <button type="button" data-tool="parts">结构</button>
    <button type="button" data-tool="isolate" aria-pressed="false" disabled>单独看</button>
    <button type="button" data-tool="hide" disabled>速隐</button>
    <button type="button" data-tool="pan" aria-pressed="false" disabled>平移</button>
    <button type="button" data-tool="capture" disabled>截图</button>
    <button type="button" data-tool="find" disabled>笔记</button>
    <button type="button" data-tool="info" disabled>来源</button>
   </div>
  </div>
 </section><dialog class="ml-fullscreen" aria-label="医学模型全屏"></dialog>
 <dialog class="ml-capture" aria-label="模型截图预览">
  <header><strong>模型截图</strong><button type="button" aria-label="关闭截图预览">关闭</button></header>
  <img alt="当前三维模型截图"><a download>保存 PNG</a>
 </dialog>`;
 const $ = selector => root.querySelector(selector);
 const shell = $('.medical-library');
 const stage = $('.ml-viewport');
 const dialog = $('.ml-fullscreen');
 const capture = $('.ml-capture');
 const input = $('#ml-keyword');
 const notice = $('.ml-message');
 const reload = $('[data-action=reload]');
 const standalone = document.body.classList.contains('atlas-standalone');
 const compact = matchMedia('(max-width: 900px)');
 const events = new AbortController();
 if (!standalone) root.querySelectorAll('.ml-info a').forEach(link => link.target = '_blank');
 capture.querySelector('a').hidden = standalone;

 $('.ml-back').innerHTML = glyph('back');
 $('.ml-expand').innerHTML = glyph('expand');
 $('.ml-exit').innerHTML = glyph('close');
 root.querySelectorAll('[data-orbit]').forEach(button => button.innerHTML = glyph(button.dataset.orbit));
 root.querySelectorAll('[data-tool]').forEach(button => button.insertAdjacentHTML('afterbegin', glyph(button.dataset.tool)));

 let catalog, active, category = '', selected = '', panel = '';
 let isolated = false, panning = false, dead = false;
 let renderer, scene, camera, controls, observer, model, pending;
 let meshes = new Map(), hiddenParts = new Set();
 let view = 'front', orthographic = true, halfHeight = 1;
 let orthographicDistance = 1, maxPerspectiveDistance = Infinity;
 const loader = new GLTFLoader();
 const directions = {
  front: new THREE.Vector3(0, 0, 1), back: new THREE.Vector3(0, 0, -1),
  left: new THREE.Vector3(1, 0, 0), right: new THREE.Vector3(-1, 0, 0),
 };
 const viewerButtons = '[data-view], [data-orbit], [data-action=projection], [data-action=zoom-in], [data-action=zoom-out], [data-action=show-all], [data-action=hide-all], [data-tool=pan], [data-tool=capture]';

 function button(label, onClick) {
  const item = document.createElement('button');
  item.type = 'button';
  item.textContent = label;
  item.onclick = onClick;
  return item;
 }

 function setPanel(mode) {
  panel = mode;
  shell.dataset.panel = mode;
  $('.ml-side').hidden = !mode;
  $('.ml-catalog').hidden = mode !== 'models';
  $('.ml-parts').hidden = mode !== 'parts';
  $('.ml-info').hidden = mode !== 'info';
  root.querySelectorAll('[data-panel], [data-tool=models], [data-tool=parts], [data-tool=info]').forEach(item => {
   item.setAttribute('aria-pressed', String((item.dataset.panel || item.dataset.tool) === mode));
  });
 }

 function status(label, message = '', retry = false) {
  $('.ml-live').textContent = label;
  notice.hidden = !message;
  notice.querySelector('p').textContent = message;
  reload.hidden = !retry;
  root.dataset.ready = String(label === '模型已就绪');
 }

 function render() {
  if (renderer && stage.clientWidth && stage.clientHeight) renderer.render(scene, camera);
 }

 function updateProjection() {
  const aspect = (stage.clientWidth || 1) / (stage.clientHeight || 1);
  if (orthographic) {
   camera.left = -halfHeight * aspect;
   camera.right = halfHeight * aspect;
   camera.top = halfHeight;
   camera.bottom = -halfHeight;
  } else camera.aspect = aspect;
  camera.updateProjectionMatrix();
 }

 function frameTarget() {
  return isolated && meshes.get(selected) || model;
 }

 function fit(direction = view, target = frameTarget()) {
  if (!target || !controls || !stage.clientWidth || !stage.clientHeight) return;
  const bounds = new THREE.Box3().setFromObject(target);
  const sphere = bounds.getBoundingSphere(new THREE.Sphere());
  const size = bounds.getSize(new THREE.Vector3());
  const side = direction === 'left' || direction === 'right';
  const width = side ? size.z : size.x;
  const depth = side ? size.x : size.z;
  const aspect = stage.clientWidth / stage.clientHeight;
  // Leave a small margin on the limiting axis so the full model stays visible.
  halfHeight = .53 * Math.max(size.y, width / aspect);
  camera.zoom = 1;
  updateProjection();
  const perspectiveDistance = halfHeight / perspectiveSlope + depth / 2;
  orthographicDistance = sphere.radius * 3 + 1;
  const distance = orthographic ? orthographicDistance : perspectiveDistance;
  maxPerspectiveDistance = perspectiveDistance / minViewScale;
  controls.target.copy(sphere.center);
  camera.position.copy(sphere.center).addScaledVector(directions[direction], distance);
  controls.minDistance = Math.max(.035, sphere.radius * .25);
  controls.maxDistance = orthographic ? Infinity : maxPerspectiveDistance;
  controls.minZoom = minViewScale;
  controls.maxZoom = 5;
  controls.update();
  view = direction;
  root.querySelectorAll('.ml-compass [data-view]').forEach(item => {
   item.setAttribute('aria-pressed', String(item.dataset.view === view));
  });
  render();
 }

 function changeProjection() {
  const offset = camera.position.clone().sub(controls.target);
  const span = orthographic ? halfHeight / camera.zoom : offset.length() * perspectiveSlope;
  orthographic = !orthographic;
  camera = orthographic ? new THREE.OrthographicCamera(-1, 1, 1, -1, .015, 100) :
   new THREE.PerspectiveCamera(perspectiveFov, 1, .015, 100);
  if (orthographic) {
   camera.zoom = THREE.MathUtils.clamp(halfHeight / span, controls.minZoom, controls.maxZoom);
   offset.setLength(orthographicDistance);
  } else offset.setLength(span / perspectiveSlope);
  camera.position.copy(controls.target).add(offset);
  controls.object = camera;
  controls.maxDistance = orthographic ? Infinity : maxPerspectiveDistance;
  const toggle = $('[data-action=projection]');
  toggle.textContent = orthographic ? '正交' : '透视';
  toggle.setAttribute('aria-pressed', String(orthographic));
  updateProjection();
  controls.update();
  render();
 }

 function initViewer() {
  if (renderer) return;
  renderer = new THREE.WebGLRenderer({antialias:true, alpha:true, powerPreference:'low-power'});
  renderer.setPixelRatio(Math.min(devicePixelRatio, 1.75));
  renderer.setClearColor(0x000000, 0);
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.12;
  renderer.domElement.setAttribute('aria-label', '三维解剖图。也可从结构清单选择部件。');
  stage.append(renderer.domElement);
  scene = new THREE.Scene();
  camera = new THREE.OrthographicCamera(-1, 1, 1, -1, .015, 100);
  scene.add(new THREE.HemisphereLight(0xf6fbff, 0x958779, 2.0));
  const key = new THREE.DirectionalLight(0xfff4e3, 2.35);
  key.position.set(-3, 5, 5);
  const rim = new THREE.DirectionalLight(0xe6effc, 1.5);
  rim.position.set(4, 3, -5);
  scene.add(key, rim);
  controls = new OrbitControls(camera, renderer.domElement);
  controls.enableDamping = false;
  controls.addEventListener('change', () => {
   const direction = camera.position.clone().sub(controls.target).normalize();
   root.querySelectorAll('.ml-compass [data-view]').forEach(item => {
    item.setAttribute('aria-pressed', String(direction.dot(directions[item.dataset.view]) > .998));
   });
   render();
  });
  const resize = () => {
   const w = stage.clientWidth, h = stage.clientHeight;
   if (!w || !h) return;
   renderer.setSize(w, h, false);
   updateProjection();
   if (model) fit();
   else render();
  };
  observer = new ResizeObserver(resize);
  observer.observe(stage);
  resize();

  const raycaster = new THREE.Raycaster();
  let start;
  renderer.domElement.addEventListener('pointerdown', event => {
   start = {x:event.clientX, y:event.clientY, id:event.pointerId};
  });
  renderer.domElement.addEventListener('pointerup', event => {
   if (!start || start.id !== event.pointerId || !model || panning) return;
   if (Math.hypot(event.clientX - start.x, event.clientY - start.y) > 6) return;
   const rect = renderer.domElement.getBoundingClientRect();
   raycaster.setFromCamera({
    x: 2 * (event.clientX - rect.left) / rect.width - 1,
    y: 1 - 2 * (event.clientY - rect.top) / rect.height,
   }, camera);
   const hit = raycaster.intersectObjects([...meshes.values()].filter(mesh => mesh.visible), false)[0];
   if (hit) selectPart(hit.object.name);
  });
 }

 function applyVisibility() {
  const part = active?.parts.find(item => item.id === selected);
  let visible = 0;
  for (const [id, mesh] of meshes) {
   const isSelected = id === selected;
   const dim = Boolean(selected && !isSelected);
   mesh.visible = isolated ? isSelected : !hiddenParts.has(id);
   mesh.material.transparent = dim;
   mesh.material.opacity = dim ? .24 : 1;
   mesh.material.depthWrite = !dim;
   mesh.material.needsUpdate = true;
   visible += Number(mesh.visible);
  }
  for (const row of root.querySelectorAll('.ml-part-row')) {
   const id = row.dataset.id;
   const hidden = isolated ? id !== selected : hiddenParts.has(id);
   const item = active.parts.find(part => part.id === id);
   row.classList.toggle('is-hidden', hidden);
   row.querySelector('[data-part]').setAttribute('aria-pressed', String(id === selected));
   const eye = row.querySelector('.ml-eye');
   eye.innerHTML = glyph(hidden ? 'hide' : 'eye');
   eye.setAttribute('aria-label', (hidden ? '显示' : '隐藏') + item.name);
   eye.setAttribute('aria-pressed', String(!hidden));
   eye.disabled = !model;
  }
  $('.ml-part-count').textContent = visible + ' / ' + (active?.parts.length || 0);
  $('.ml-selected').textContent = part?.name || (visible || !model ? '模型总览' : '结构已隐藏');
  $('.ml-selected-id').textContent = part ? part.english + ' · ' + part.id : active?.english || '';
  $('[data-tool=isolate]').disabled = !part || !model;
  $('[data-tool=isolate]').setAttribute('aria-pressed', String(isolated));
  $('[data-tool=hide]').disabled = !part || !model;
  render();
 }

 function selectPart(id) {
  selected = id;
  hiddenParts.delete(id);
  applyVisibility();
  if (isolated) fit();
 }

 function setPartVisibility(id) {
  const wasIsolated = isolated;
  const wasVisible = isolated ? id === selected : !hiddenParts.has(id);
  isolated = false;
  if (wasVisible) {
   hiddenParts.add(id);
   if (selected === id) selected = '';
  } else {
   hiddenParts.delete(id);
   selected = '';
  }
  applyVisibility();
  if (wasIsolated) fit();
 }

 function renderParts() {
  $('.ml-part-list').replaceChildren(...active.parts.map(part => {
   const row = document.createElement('div');
   row.className = 'ml-part-row';
   row.dataset.id = part.id;
   const item = button('', () => selectPart(part.id));
   item.dataset.part = part.id;
   const swatch = document.createElement('i');
   swatch.style.backgroundColor = part.color;
   swatch.setAttribute('aria-hidden', 'true');
   const label = document.createElement('span');
   label.textContent = part.name;
   item.append(swatch, label);
   item.title = part.name + ' · ' + part.id;
   const eye = button('', () => setPartVisibility(part.id));
   eye.className = 'ml-eye';
   row.append(item, eye);
   return row;
  }));
 }

 function matchingModels() {
  const q = input.value.trim().toLowerCase();
  const exact = [], partial = [];
  for (const item of catalog?.items || []) {
   if (category && item.category !== category) continue;
   const values = [item.name, item.english, item.category, item.id, ...item.keywords];
   for (const part of item.parts) values.push(part.name, part.english);
   const terms = values.map(value => value.toLowerCase());
   if (terms.includes(q) || item.parts.some(part => part.id.toLowerCase() === q)) exact.push(item);
   else if (terms.some(value => value.includes(q))) partial.push(item);
  }
  return [...exact, ...partial];
 }

 function renderModels() {
  const items = matchingModels();
  $('.ml-count').textContent = items.length + ' 个模型';
  $('.ml-results').replaceChildren(...items.map(item => {
   const entry = button('', () => show(item));
   entry.dataset.model = item.id;
   entry.setAttribute('aria-pressed', String(item.id === active?.id));
   const title = document.createElement('strong');
   title.textContent = item.name;
   const detail = document.createElement('small');
   detail.textContent = item.parts.length + ' 项结构';
   entry.append(title, detail);
   return entry;
  }));
  if (!items.length) {
   const empty = document.createElement('p');
   empty.className = 'ml-empty';
   empty.textContent = /穴位|经络/.test(input.value) ? '在「穴位图谱」页签查看点位。' : '未找到相关结构，试试模型名称或 FMA 编号。';
   $('.ml-results').append(empty);
  }
 }

 async function show(item) {
  pending?.abort();
  const request = new AbortController();
  pending = request;
  active = item;
  if (model) {
   scene.remove(model);
   disposeModel(model);
   model = null;
  }
  meshes = new Map();
  selected = '';
  isolated = false;
  hiddenParts = new Set(item.parts.filter(part => part.cavity).map(part => part.id));
  view = 'front';
  $('.ml-title').textContent = item.name;
  $('.ml-category').textContent = item.category;
  $('.ml-summary').textContent = item.summary;
  $('.ml-credit').textContent = catalog.credit;
  $('.ml-source').href = item.url;
  $('.ml-license').href = catalog.license_url;
  $('[data-tool=info]').disabled = false;
  $('[data-tool=find]').disabled = false;
  root.querySelectorAll(viewerButtons).forEach(control => control.disabled = true);
  root.querySelectorAll('[data-model]').forEach(control => {
   control.setAttribute('aria-pressed', String(control.dataset.model === item.id));
  });
  renderParts();
  applyVisibility();
  status('载入模型', '正在载入解剖结构…');
  try {
   initViewer();
   const url = base + '/anatomy/library/' + encodeURIComponent(item.asset);
   const response = await fetch(url, {signal:request.signal});
   if (!response.ok) throw Error('模型文件暂不可用');
   const gltf = await loader.parseAsync(await response.arrayBuffer(), '');
   if (request.signal.aborted || dead) {
    disposeModel(gltf.scene);
    return;
   }
   model = gltf.scene;
   for (const part of item.parts) {
    const mesh = model.getObjectByName(part.id);
    if (!mesh?.isMesh) throw Error('模型与结构清单不一致');
    mesh.material.dispose();
    mesh.material = new THREE.MeshStandardMaterial({color:part.color, roughness:.72, side:THREE.DoubleSide});
    meshes.set(part.id, mesh);
   }
   scene.add(model);
   applyVisibility();
   fit('front');
   root.querySelectorAll(viewerButtons).forEach(control => control.disabled = false);
   status('模型已就绪');
  } catch (error) {
   if (request.signal.aborted || dead) return;
   status('载入失败', renderer ? error.message + '，请重新载入。' : '设备暂时无法启用三维显示，可先查看结构清单。', true);
  }
 }

 async function loadCatalog() {
  pending?.abort();
  const request = new AbortController();
  pending = request;
  status('载入目录', '正在读取解剖目录…');
  try {
   const response = await fetch(base + '/api/anatomy/models', {credentials:'same-origin', signal:request.signal});
   if (!response.ok) throw Error(response.status === 401 ? '会话已过期，请重新登录' : '解剖目录暂不可用');
   const value = await response.json();
   if (!Array.isArray(value.items) || !value.items.length || value.provider !== 'bodyparts3d') {
    throw Error('请将服务端更新至新版解剖模型库');
   }
   if (dead || request.signal.aborted) return;
   catalog = value;
   const categories = ['', ...new Set(catalog.items.map(item => item.category))];
   $('.ml-topics').replaceChildren(...categories.map((label, i) => {
    const tab = button(label || '全部', () => {
     category = label;
     root.querySelectorAll('.ml-topics button').forEach(item => item.setAttribute('aria-pressed', String(item === tab)));
     renderModels();
    });
    tab.setAttribute('aria-pressed', String(i === 0));
    return tab;
   }));
   $('.ml-quick').replaceChildren(...catalog.items.map(item => {
    const shortcut = button(shortName[item.id] || item.name, () => show(item));
    shortcut.dataset.model = item.id;
    shortcut.setAttribute('aria-label', '查看' + item.name);
    return shortcut;
   }));
   renderModels();
   show(catalog.items[0]);
  } catch (error) {
   if (!dead && !request.signal.aborted) status('目录暂不可用', error.message, true);
  }
 }

 function setAll(visible) {
  const wasIsolated = isolated;
  hiddenParts = new Set(visible ? [] : meshes.keys());
  selected = '';
  isolated = false;
  applyVisibility();
  if (wasIsolated) fit();
 }

 function showCapture() {
  render();
  const source = renderer.domElement;
  const copy = document.createElement('canvas');
  copy.width = source.width;
  copy.height = source.height + 26;
  const ctx = copy.getContext('2d');
  ctx.fillStyle = getComputedStyle($('.ml-stage')).backgroundColor;
  ctx.fillRect(0, 0, copy.width, copy.height);
  ctx.drawImage(source, 0, 0);
  ctx.fillStyle = '#f4f1eb';
  ctx.font = '12px system-ui';
  ctx.fillText(catalog.credit, 10, source.height + 17, copy.width - 20);
  const image = copy.toDataURL('image/png');
  capture.querySelector('img').src = image;
  capture.querySelector('a').href = image;
  capture.querySelector('a').download = 'anatomy-' + active.id + '.png';
  capture.showModal();
 }

 function toggleFullscreen() {
  if (dialog.open) {
   dialog.close();
   return;
  }
  dialog.append(shell);
  $('.ml-exit').hidden = false;
  $('.ml-expand').setAttribute('aria-label', '还原窗口');
  $('.ml-back').setAttribute('aria-label', '返回工作台');
  $('.ml-expand').title = '退出全屏';
  dialog.showModal();
 }

 dialog.addEventListener('close', () => {
  if (dead) return;
  dialog.before(shell);
  $('.ml-exit').hidden = true;
  $('.ml-expand').setAttribute('aria-label', '进入全屏');
  $('.ml-expand').title = '全屏';
  $('.ml-back').setAttribute('aria-label', '返回穴位图谱');
  $('.ml-expand').focus();
 });
 capture.querySelector('button').onclick = () => capture.close();
 capture.addEventListener('close', () => {
  capture.querySelector('img').removeAttribute('src');
  capture.querySelector('a').removeAttribute('href');
 });
 $('.ml-expand').onclick = toggleFullscreen;
 $('.ml-exit').onclick = () => dialog.close();
 $('.ml-back').onclick = () => {
  if (dialog.open) dialog.close();
  else root.dispatchEvent(new CustomEvent('anatomy-library-back', {bubbles:true}));
 };
 root.querySelectorAll('.ml-panel-tabs [data-panel]').forEach(item => item.onclick = () => setPanel(item.dataset.panel));
 for (const mode of ['models', 'parts', 'info']) {
  $('[data-tool='+mode+']').onclick = () => setPanel(panel === mode ? '' : mode);
 }
 compact.addEventListener('change', () => setPanel(compact.matches ? '' : 'parts'), {signal:events.signal});
 setPanel(compact.matches ? '' : 'parts');

 input.addEventListener('input', renderModels);
 $('.ml-search').onsubmit = event => {
  event.preventDefault();
  const item = matchingModels()[0];
  if (item) show(item);
 };
 reload.onclick = () => active ? show(active) : loadCatalog();
 $('[data-action=show-all]').onclick = () => setAll(true);
 $('[data-action=hide-all]').onclick = () => setAll(false);
 $('[data-action=projection]').onclick = changeProjection;
 $('[data-tool=isolate]').onclick = () => {
  isolated = !isolated;
  applyVisibility();
  fit();
 };
 $('[data-tool=hide]').onclick = () => setPartVisibility(selected);
 $('[data-tool=pan]').onclick = () => {
  panning = !panning;
  controls.mouseButtons.LEFT = panning ? THREE.MOUSE.PAN : THREE.MOUSE.ROTATE;
  controls.touches.ONE = panning ? THREE.TOUCH.PAN : THREE.TOUCH.ROTATE;
  $('[data-tool=pan]').setAttribute('aria-pressed', String(panning));
  $('.ml-hint').textContent = panning ? '拖动平移 · 滚轮或双指缩放' : '拖动旋转 · 滚轮或双指缩放 · 点选结构';
 };
 $('[data-tool=capture]').onclick = showCapture;
 $('[data-tool=find]').onclick = () => {
  const query = active.parts.find(part => part.id === selected)?.name || active.name;
  if (standalone) location.href = base + '/?anatomy_q=' + encodeURIComponent(query) + '#search';
  else root.dispatchEvent(new CustomEvent('anatomy-search', {detail:query, bubbles:true}));
 };
 root.querySelectorAll('[data-view]').forEach(item => item.onclick = () => fit(item.dataset.view === 'reset' ? 'front' : item.dataset.view));
 for (const [action, scale] of [['zoom-in', 1.22], ['zoom-out', 1/1.22]]) {
  $('[data-action='+action+']').onclick = () => {
   if (orthographic) {
    camera.zoom = THREE.MathUtils.clamp(camera.zoom * scale, controls.minZoom, controls.maxZoom);
    camera.updateProjectionMatrix();
   } else camera.position.sub(controls.target).multiplyScalar(1 / scale).add(controls.target);
   controls.update();
   render();
  };
 }
 for (const [action, theta, phi] of [['up',0,-.15], ['down',0,.15], ['left',.18,0], ['right',-.18,0]]) {
  $('[data-orbit='+action+']').onclick = () => {
   const offset = camera.position.clone().sub(controls.target);
   const spherical = new THREE.Spherical().setFromVector3(offset);
   spherical.theta += theta;
   spherical.phi = THREE.MathUtils.clamp(spherical.phi + phi, .04, Math.PI - .04);
   camera.position.setFromSpherical(spherical).add(controls.target);
   controls.update();
   render();
  };
 }
 loadCatalog();

 return {dispose() {
  dead = true;
  events.abort();
  pending?.abort();
  capture.close();
  dialog.close();
  observer?.disconnect();
  controls?.dispose();
  disposeModel(model);
  renderer?.dispose();
  renderer?.forceContextLoss();
 }};
}
