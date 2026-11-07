import * as THREE from './vendor/three.module.js';
import {OrbitControls} from './vendor/OrbitControls.js';
import {GLTFLoader} from './vendor/GLTFLoader.js';

const disposeModel = model => model?.traverse(node => {
 if (!node.isMesh) return;
 node.geometry.dispose();
 node.material.dispose();
});

export function mountLibrary(root, base = '') {
 root.innerHTML = `<section class="medical-library" aria-label="人体解剖模型">
  <div class="ml-layout">
   <aside class="ml-catalog">
    <div class="ml-catalog-head"><strong>解剖目录</strong><span class="ml-count" role="status"></span></div>
    <form class="ml-search"><label class="ml-sr" for="ml-keyword">查找模型或结构</label>
     <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="10" cy="10" r="6"/><path d="m14.5 14.5 5 5"/></svg>
     <input id="ml-keyword" type="search" maxlength="100" placeholder="搜索结构、名称或 FMA">
    </form>
    <div class="ml-topics" aria-label="解剖系统"></div>
    <div class="ml-results" aria-label="模型列表"></div>
   </aside>
   <section class="ml-detail" aria-label="模型阅览">
    <header class="ml-model-heading">
     <div><p class="ml-category"></p><h3 class="ml-title">人体解剖</h3><p class="ml-english"></p></div>
     <span class="ml-live" role="status">载入目录</span>
    </header>
    <div class="ml-workbench">
     <div class="ml-stage">
      <div class="ml-camera" role="group" aria-label="模型视角">
       <button type="button" data-view="front" disabled>正面</button><button type="button" data-view="back" disabled>背面</button>
       <button type="button" data-view="side" disabled>左侧</button><button type="button" data-view="reset" disabled>复位</button>
       <button type="button" class="ml-jump-parts">结构清单 ↓</button>
      </div>
      <div class="ml-viewport" aria-label="可旋转与缩放的三维模型"></div>
      <div class="ml-message" role="status"><p>正在读取解剖目录…</p><button type="button" data-action="reload" hidden>重新载入</button></div>
      <p class="ml-hint">拖动旋转 · 滚轮或双指缩放 · 点选结构</p>
     </div>
     <aside class="ml-parts" aria-label="模型结构">
      <div class="ml-parts-head"><strong>模型结构</strong><span class="ml-part-count"></span><button type="button" class="ml-jump-stage">回到模型 ↑</button></div>
      <div class="ml-part-list"></div>
      <div class="ml-selection" hidden>
       <strong class="ml-selected"></strong><small class="ml-selected-id"></small>
       <div><button type="button" class="ml-isolate" aria-pressed="false" disabled>单独查看</button>
        <button type="button" class="ml-clear">显示全部</button></div>
      </div>
     </aside>
    </div>
    <footer class="ml-foot">
     <div><p class="ml-summary"></p><p class="ml-credit">BodyParts3D · DBCLS</p>
      <a class="ml-source" rel="noopener noreferrer" hidden>数据来源 ↗</a>
      <a class="ml-license" rel="noopener noreferrer" hidden>CC BY 4.0 ↗</a></div>
     <button type="button" class="ml-find" disabled>查找相关笔记</button>
    </footer>
   </section>
  </div>
 </section>`;

 const $ = selector => root.querySelector(selector);
 const stage = $('.ml-viewport');
 const input = $('#ml-keyword');
 const notice = $('.ml-message');
 const reload = $('[data-action=reload]');
 const standalone = document.body.classList.contains('atlas-standalone');
 if (!standalone) root.querySelectorAll('.ml-foot a').forEach(a => a.target = '_blank');

 let catalog, active, category = '', selected = '', isolated = false;
 let renderer, scene, camera, controls, observer, model, pending, dead = false;
 let meshes = new Map();
 const loader = new GLTFLoader();
 const directions = {
  front: new THREE.Vector3(0, .045, 1),
  back: new THREE.Vector3(0, .045, -1),
  side: new THREE.Vector3(1, .045, 0),
 };
 let view = 'front';

 function button(label, onClick) {
  const item = document.createElement('button');
  item.type = 'button';
  item.textContent = label;
  item.onclick = onClick;
  return item;
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

 function fit(direction = view, target = model) {
  if (!target || !controls) return;
  const bounds = new THREE.Box3().setFromObject(target);
  const sphere = bounds.getBoundingSphere(new THREE.Sphere());
  const vertical = THREE.MathUtils.degToRad(camera.fov / 2);
  const horizontal = Math.atan(Math.tan(vertical) * camera.aspect);
  const size = bounds.getSize(new THREE.Vector3());
  const width = direction === 'side' ? size.z : size.x;
  const depth = direction === 'side' ? size.x : size.z;
  const distance = 1.13 * Math.max(size.y / (2 * Math.tan(vertical)), width / (2 * Math.tan(horizontal))) + depth / 2;
  controls.target.copy(sphere.center);
  camera.position.copy(sphere.center).addScaledVector(directions[direction], distance);
  controls.minDistance = Math.max(.035, sphere.radius * .25);
  controls.maxDistance = Math.max(12, distance * 3);
  controls.update();
  view = direction;
  render();
 }

 function initViewer() {
  if (renderer) return;
  renderer = new THREE.WebGLRenderer({alpha: true, antialias: true, powerPreference: 'low-power'});
  renderer.setPixelRatio(Math.min(devicePixelRatio, 1.8));
  renderer.setClearColor(0, 0);
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.14;
  renderer.domElement.setAttribute('aria-label', '三维解剖图。也可从结构清单选择部件。');
  stage.append(renderer.domElement);
  scene = new THREE.Scene();
  camera = new THREE.PerspectiveCamera(33, 1, .015, 60);
  scene.add(new THREE.HemisphereLight(0xf6fbff, 0xbeb2a0, 2.15));
  const key = new THREE.DirectionalLight(0xfff5e7, 2.3);
  key.position.set(-3, 5, 5);
  const rim = new THREE.DirectionalLight(0xd7f2f6, 1.4);
  rim.position.set(4, 3, -5);
  scene.add(key, rim);
  controls = new OrbitControls(camera, renderer.domElement);
  controls.enableDamping = false;
  controls.addEventListener('change', render);
  const resize = () => {
   const w = stage.clientWidth, h = stage.clientHeight;
   if (!w || !h) return;
   renderer.setSize(w, h, false);
   camera.aspect = w / h;
   camera.updateProjectionMatrix();
   render();
  };
  observer = new ResizeObserver(resize);
  observer.observe(stage);
  resize();

  const raycaster = new THREE.Raycaster();
  let start;
  renderer.domElement.addEventListener('pointerdown', event => {
   start = {x: event.clientX, y: event.clientY, id: event.pointerId};
  });
  renderer.domElement.addEventListener('pointerup', event => {
   if (!start || start.id !== event.pointerId || !model) return;
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

 function selectPart(id) {
  selected = id;
  if (!selected) isolated = false;
  const part = active.parts.find(item => item.id === id);
  for (const item of active.parts) {
   const mesh = meshes.get(item.id);
   if (!mesh) continue;
   const isSelected = item.id === selected;
   const dim = Boolean(selected && !isSelected);
   mesh.visible = isolated ? isSelected : !item.cavity || isSelected;
   mesh.material.transparent = dim;
   mesh.material.opacity = dim ? .22 : 1;
   mesh.material.depthWrite = !dim;
   mesh.material.needsUpdate = true;
  }
  root.querySelectorAll('[data-part]').forEach(item => {
   item.setAttribute('aria-pressed', String(item.dataset.part === selected));
  });
  $('.ml-selection').hidden = !part;
  $('.ml-selected').textContent = part?.name || '';
  $('.ml-selected-id').textContent = part ? part.english + ' · ' + part.id : '';
  $('.ml-isolate').disabled = !part || !model;
  $('.ml-isolate').setAttribute('aria-pressed', String(isolated));
  $('.ml-isolate').textContent = isolated ? '退出单独查看' : '单独查看';
  if (isolated) fit(view, meshes.get(selected));
  else render();
 }

 function renderParts() {
  $('.ml-part-count').textContent = active.parts.length + ' 项';
  $('.ml-part-list').replaceChildren(...active.parts.map(part => {
   const item = button('', () => selectPart(part.id));
   item.dataset.part = part.id;
   const swatch = document.createElement('i');
   swatch.style.backgroundColor = part.color;
   swatch.setAttribute('aria-hidden', 'true');
   const label = document.createElement('span');
   label.textContent = part.name;
   const identifier = document.createElement('small');
   identifier.textContent = part.id;
   item.append(swatch, label, identifier);
   return item;
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
   detail.textContent = item.category + ' · ' + item.parts.length + ' 项结构';
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
  isolated = false;
  view = 'front';
  $('.ml-title').textContent = item.name;
  $('.ml-english').textContent = item.english;
  $('.ml-category').textContent = item.category;
  $('.ml-summary').textContent = item.summary;
  $('.ml-credit').textContent = catalog.credit;
  $('.ml-source').href = item.url;
  $('.ml-license').href = catalog.license_url;
  $('.ml-source').hidden = $('.ml-license').hidden = false;
  $('.ml-find').disabled = false;
  root.querySelectorAll('[data-view]').forEach(item => item.disabled = true);
  renderModels();
  renderParts();
  selectPart('');
  status('载入模型', '正在载入解剖结构…');

  try {
   initViewer();
   const url = base + '/anatomy/library/' + encodeURIComponent(item.asset);
   const response = await fetch(url, {signal: request.signal});
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
    mesh.material = new THREE.MeshStandardMaterial({color: part.color, roughness: .72, side: THREE.DoubleSide});
    meshes.set(part.id, mesh);
   }
   scene.add(model);
   selectPart(selected);
   fit('front');
   root.querySelectorAll('[data-view]').forEach(item => item.disabled = false);
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
   const response = await fetch(base + '/api/anatomy/models', {credentials: 'same-origin', signal: request.signal});
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
     root.querySelectorAll('.ml-topics button').forEach(item => {
      item.setAttribute('aria-pressed', String(item === tab));
     });
     const matches = matchingModels();
     if (matches.length && !matches.some(item => item.id === active?.id)) show(matches[0]);
     else renderModels();
    });
    tab.setAttribute('aria-pressed', String(i === 0));
    return tab;
   }));
   show(catalog.items[0]);
  } catch (error) {
   if (!dead && !request.signal.aborted) status('目录暂不可用', error.message, true);
  }
 }

 $('.ml-jump-parts').onclick = () => $('.ml-parts').scrollIntoView({block:'start'});
 $('.ml-jump-stage').onclick = () => $('.ml-stage').scrollIntoView({block:'start'});
 input.addEventListener('input', renderModels);
 $('.ml-search').onsubmit = event => {
  event.preventDefault();
  const item = matchingModels()[0];
  if (item) show(item);
 };
 reload.onclick = () => active ? show(active) : loadCatalog();
 $('.ml-isolate').onclick = () => {
  isolated = !isolated;
  selectPart(selected);
  if (!isolated) fit();
 };
 $('.ml-clear').onclick = () => {
  const wasIsolated = isolated;
  selectPart('');
  if (wasIsolated) fit();
 };
 root.querySelectorAll('[data-view]').forEach(item => item.onclick = () => {
  if (item.dataset.view === 'reset') {
   selectPart('');
   fit('front');
  } else fit(item.dataset.view, isolated ? meshes.get(selected) : model);
 });
 $('.ml-find').onclick = () => {
  const query = active.parts.find(part => part.id === selected)?.name || active.name;
  if (standalone) location.href = base + '/?anatomy_q=' + encodeURIComponent(query) + '#search';
  else root.dispatchEvent(new CustomEvent('anatomy-search', {detail: query, bubbles: true}));
 };
 loadCatalog();

 return {dispose() {
  dead = true;
  pending?.abort();
  observer?.disconnect();
  controls?.dispose();
  disposeModel(model);
  renderer?.dispose();
  renderer?.forceContextLoss();
 }};
}
