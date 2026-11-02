// Browser regression using synthetic API fixtures. No real account or data is changed.
// Install Playwright separately, or set PLAYWRIGHT_MODULE_PATH to its index.mjs.
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE_PATH || 'playwright');
const base=(process.argv[2] || 'http://localhost:5173').replace(/\/$/,'');
const browser=await chromium.launch({channel:'chrome',headless:true,args:['--use-angle=swiftshader','--enable-unsafe-swiftshader']});
try {
 const page=await browser.newPage();
 const errors=[];const sent=[];let atlas={revision:'0',points:[]};page.on('pageerror',e=>errors.push(e.message));
 const note={id:'11111111111111111111111111111111',title:'回顾按钮回归验证',category:'诊疗笔记',note_type:'record',note_types:['record'],tags:['回归验证'],content:'合成资料，用于检查回顾按钮。',steps:['检查第一步','检查第二步'],attachments:[],occurred_on:'2026-11-01',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',version:'fixture-v1'};
 await page.route('**/api/**',async route=>{
  const p=new URL(route.request().url()).pathname.replace(new URL(base).pathname.replace(/\/$/,''),'');
  if(p.endsWith('/api/anatomy')){if(route.request().method()==='PUT'){const payload=route.request().postDataJSON();atlas={...payload,revision:String(+atlas.revision+1)};}await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(atlas)});return;}
  if(p.endsWith('/api/chat')){const body=route.request().postDataJSON();sent.push(body);await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({conversation_id:'fixture-chat',note_id:body.note_id||'',content:'合成回顾回复',sources:body.note_id?[{id:note.id,title:note.title,occurred_on:note.occurred_on}]:[],provider:'local',warnings:[]})});return;}
  const bodies={
   '/api/health':{status:'ok',initialized:true},
   '/api/auth/me':{id:991,username:'qa_recall',display_name:'回归验证'},
   '/api/settings':{cloud_search_enabled:false,ai_enabled:false,ai_provider:'none',local_ocr_available:true},
   '/api/stats':{notes:1,attachments:0,conversations:0,categories:[]},
   '/api/taxonomy':{note_types:[{id:'record',title:'诊疗记录',aliases:['诊疗笔记'],count:1,content_label:'诊疗记录',steps_label:'已记录过程'}],tags:[{name:'回归验证',count:1}],version:'fixture-catalog'},
   '/api/notes':{items:[note],total:1},
   ['/api/notes/'+note.id]:note,
   '/api/conversations':[],
   '/api/sync':{version:'fixture-sync',library_scope:'shared',user:{id:991,username:'qa_recall',display_name:'回归验证'}}
  };
  await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(bodies[p]??{})});
 });

 await page.goto(base+(process.argv.includes('--standalone')?'/anatomy/':'/#anatomy'));
 await page.locator('.a-list button').first().waitFor();
 await page.locator('.a-model-loading').waitFor({state:'hidden'});
 const count=await page.locator('.a-list button').count();assert.equal(count,50);
 const initialStageHeight=(await page.locator('.atlas-stage').boundingBox()).height;
 await page.locator('.a-list button').first().click();await page.waitForTimeout(100);
 assert.equal((await page.locator('.atlas-stage').boundingBox()).height,initialStageHeight,'selection must not resize the model viewport');
 const model=page.locator('.atlas-stage canvas');assert.equal(await model.count(),1);
 await page.screenshot({path:'/tmp/atlas-male-realistic.png',fullPage:true});
 assert.equal(await page.getByRole('button',{name:'复位',exact:true}).count(),1);
 assert.equal(await page.locator('.a-views #a-show').count(),1);assert.equal(await page.locator('.atlas-head #a-show').count(),0);
 await page.locator('.a-callout').first().waitFor();
 assert.ok(await page.locator('.a-callout').count()>10,'all visible points must have names');
 assert.ok((await page.locator('.a-callout').allTextContents()).includes('中脘'));
 const namePositions=await page.locator('.a-callout').evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {left:r.left,top:r.top,bottom:r.bottom}}));
 for(let i=0;i<namePositions.length;i++)for(let j=i+1;j<namePositions.length;j++)if(Math.abs(namePositions[i].left-namePositions[j].left)<2)assert.ok(namePositions[i].bottom<=namePositions[j].top+.5||namePositions[j].bottom<=namePositions[i].top+.5,'name labels must not overlap');
 await page.getByRole('button',{name:'背面',exact:true}).click();await page.waitForFunction(()=>[...document.querySelectorAll('.a-callout')].some(n=>n.textContent==='肾俞'));
 assert.ok(!(await page.locator('.a-callout').allTextContents()).includes('中脘'),'front labels must not show through the body');
 // Choosing an opposite-face point must keep the current orbit instead of resetting it.
 await page.locator('.a-list button').filter({hasText:'中脘'}).click();
 assert.equal(await page.locator('.a-orientation').textContent(),'背面','selecting a point must preserve the viewing angle');
 assert.ok((await page.locator('.a-selection').textContent()).includes('人体背侧'));
 assert.equal(await page.locator('.a-selected-ring').isVisible(),false,'occluded selection must not shine through the body');
 await page.getByRole('button',{name:'正面',exact:true}).click();await page.waitForFunction(()=>[...document.querySelectorAll('.a-callout')].some(n=>n.textContent==='中脘'));
 await page.locator('.a-callout[aria-pressed=true]').waitFor();
 assert.equal(await page.locator('.a-selected-ring').isVisible(),true);
 assert.ok((await page.locator('.a-selection').textContent()).includes('已选：中脘'));assert.equal(await page.locator('.a-point-label').count(),0);
 // Drag and zoom, then select a visible callout: every surface anchor must stay put.
 const rect=await model.boundingBox();
 await page.mouse.move(rect.x+rect.width*.5,rect.y+rect.height*.5);await page.mouse.down();
 await page.mouse.move(rect.x+rect.width*.56,rect.y+rect.height*.52,{steps:8});await page.mouse.up();
 await page.mouse.wheel(0,-90);await page.waitForTimeout(150);
 const anchors=()=>page.locator('.a-callout-line').evaluateAll(nodes=>nodes.map(n=>n.getAttribute('points').split(' ')[0]).sort());
 const before=await anchors();await page.locator('.a-callout').filter({hasText:'神阙'}).click();await page.waitForTimeout(100);
 assert.deepEqual(await anchors(),before,'selecting a callout must retain orbit, pan and zoom');
 await page.getByRole('button',{name:'正面',exact:true}).click();
 await page.locator('.a-list button').filter({hasText:'中脘'}).click();await page.waitForTimeout(100);
 // Free view pans, changes the orbit center, and resets without altering point records.
 const free=page.getByRole('button',{name:'自由视角',exact:true}),center=page.getByRole('button',{name:'选择中心',exact:true}),reset=page.getByRole('button',{name:'复位',exact:true});
 assert.equal(await center.isDisabled(),true);await free.click();assert.equal(await free.getAttribute('aria-pressed'),'true');
 const original=await anchors();const bodyRing=await page.locator('.a-selected-ring').boundingBox();
 await center.click();await page.mouse.click(bodyRing.x+bodyRing.width/2,bodyRing.y+bodyRing.height/2);await page.waitForTimeout(100);
 assert.equal(await center.getAttribute('aria-pressed'),'false');assert.ok((await page.locator('.a-hint').textContent()).includes('旋转中心已更新'));
 const centeredRing=await page.locator('.a-selected-ring').boundingBox(),viewport=await model.boundingBox();
 assert.ok(Math.abs(centeredRing.x+centeredRing.width/2-viewport.x-viewport.width/2)<4,'chosen surface must become the orbit center');
 assert.ok(Math.abs(centeredRing.y+centeredRing.height/2-viewport.y-viewport.height/2)<4);
 const pan=page.getByRole('button',{name:'平移',exact:true});await pan.click();
 const beforePan=await anchors();await page.mouse.move(viewport.x+viewport.width/2,viewport.y+viewport.height/2);await page.mouse.down();await page.mouse.move(viewport.x+viewport.width/2+35,viewport.y+viewport.height/2+15,{steps:6});await page.mouse.up();await page.waitForTimeout(100);
 assert.notDeepEqual(await anchors(),beforePan,'pan must translate the view');
 await reset.click();await page.waitForTimeout(100);assert.deepEqual(await anchors(),original,'reset must restore the default view');
 await free.click();assert.equal(await center.isDisabled(),true);assert.equal(await page.locator('.atlas').evaluate(e=>e.parentElement.dataset.dirty),'false','camera changes must not edit point data');
 const allPixels=await model.evaluate(c=>c.toDataURL());
 await page.locator('#a-show').check();assert.equal(await page.locator('.a-selection').isVisible(),true);await page.waitForTimeout(100);assert.equal(await page.locator('.a-callout').count(),1,'current-only mode must retain the selected name and leader');assert.ok((await page.locator('.a-callout').textContent()).includes('中脘'));assert.ok((await page.locator('.a-callout').textContent()).includes('CV12'));
 assert.equal(await page.locator('.a-selected-ring').isVisible(),true);assert.equal(await page.locator('.a-selection').isVisible(),true);
 // The current card survives occlusion and selecting another point without restoring the camera.
 await page.getByRole('button',{name:'背面',exact:true}).click();await page.waitForTimeout(100);
 assert.equal(await page.locator('.a-callout').count(),1);assert.ok((await page.locator('.a-callout').textContent()).includes('背侧'));
 assert.equal(await page.locator('.a-selected-ring').isVisible(),false);assert.equal(await page.locator('.a-callout-line').evaluate(l=>getComputedStyle(l).display),'none');
 await page.locator('.a-list button').filter({hasText:'迎香 · 左'}).click();await page.waitForTimeout(100);
 assert.equal(await page.locator('.a-orientation').textContent(),'背面');assert.ok((await page.locator('.a-callout').textContent()).includes('LI20'));
 await page.getByRole('button',{name:'正面',exact:true}).click();await page.waitForTimeout(100);
 assert.equal(await page.locator('.a-selected-ring').isVisible(),true);
 await page.locator('.a-list button').filter({hasText:'中脘'}).click();await page.waitForTimeout(100);
 const singlePixels=await model.evaluate(c=>c.toDataURL());assert.notEqual(singlePixels,allPixels,'current-only mode must clear the other markers');
 await page.screenshot({path:'/tmp/atlas-male-no-points.png',fullPage:true});await page.locator('#a-show').uncheck();
 await page.locator('#a-query').fill('百会');assert.equal(await page.locator('.a-list button').count(),1);
 assert.equal(await model.evaluate(c=>c.toDataURL()),allPixels,'all mode must retain every marker while list is filtered');await page.locator('#a-query').fill('');
 await page.locator('#a-show').check();
 const placement=await page.locator('.a-selected-ring').boundingBox();await page.getByRole('button',{name:'手动标记',exact:true}).click();const box=await model.boundingBox();await model.click({position:{x:placement.x+placement.width/2-box.x,y:placement.y+placement.height/2-box.y}});
 assert.equal(await page.locator('.a-list button').count(),51);assert.equal(await page.locator('#a-show').isChecked(),true,'manual marking must retain the visibility preference');
 await page.locator('#a-name').fill('男性测试点位');await page.locator('#a-description').fill('手动标记记录测试');await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 assert.ok(atlas.points.some(p=>p.name==='男性测试点位'&&p.model==='male-real-v1'));
 await page.getByRole('button',{name:'女性',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});assert.equal(await page.locator('.a-list button').count(),50);
 await page.screenshot({path:'/tmp/atlas-female-realistic.png',fullPage:true});
 await page.locator('.a-list button').first().click();await page.locator('#a-description').fill('女性模型独立记录');await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 await page.getByRole('button',{name:'刷新',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});await page.locator('.a-list button').first().click();assert.equal(await page.locator('#a-description').inputValue(),'女性模型独立记录');
 page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'删除点位',exact:true}).click();assert.equal(await page.locator('.a-list button').count(),49);await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 await page.getByRole('button',{name:'刷新',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});assert.equal(await page.locator('.a-list button').count(),49);
 await page.getByRole('button',{name:'男性',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});assert.equal(await page.locator('.a-list button').count(),51);assert.ok(await page.getByRole('button',{name:'男性测试点位 自定义',exact:false}).count());
 await page.locator('#a-show').uncheck();
 for(const width of [320,390,1440]){await page.setViewportSize({width,height:900});await page.waitForTimeout(100);const views=await page.locator('.a-views').boundingBox(),canvas=await model.boundingBox();assert.ok(canvas.y>=views.y+views.height-1,'tools must not overlap the model viewport');assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);
 const labels=await page.locator('.a-callout').evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,bottom:r.bottom}}));for(let i=0;i<labels.length;i++)for(let j=i+1;j<labels.length;j++)if(Math.abs(labels[i].x-labels[j].x)<2)assert.ok(labels[i].bottom<=labels[j].y+.5||labels[j].bottom<=labels[i].y+.5,'names overlap at width '+width);
 const endpoints=await page.locator('.a-name-lines polyline').evaluateAll(lines=>lines.map(line=>line.getAttribute('points').split(' ')[0].split(',').map(Number)));const size=await page.locator('.atlas-stage').evaluate(e=>({w:e.clientWidth,h:e.clientHeight}));for(const [x,y] of endpoints){assert.ok(x>=0&&x<=size.w);assert.ok(y>=60&&y<=size.h-40,'leader must end on the model viewport');}
 }
 await page.locator('#a-show').uncheck();await page.setViewportSize({width:390,height:844});await page.waitForTimeout(150);await page.screenshot({path:'/tmp/atlas-realistic-mobile.png',fullPage:true});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);

 // A tall model must retain its camera controls while the page scrolls.
 for(const width of [1440,390]){await page.setViewportSize({width,height:760});await page.evaluate(()=>{const r=document.querySelector('.atlas-stage').getBoundingClientRect();window.scrollTo(0,scrollY+r.top+220)});await page.waitForTimeout(100);
 const tools=await page.locator('.a-views').boundingBox();assert.ok(tools.y>=0&&tools.y<=16,'camera toolbar must stick to the viewport');
 assert.equal(await page.locator('[data-action=reset-view]').evaluate(e=>{const r=e.getBoundingClientRect();return document.elementFromPoint(r.x+r.width/2,r.y+r.height/2)?.closest('button')===e}),true);}
 assert.deepEqual(errors,[]);console.log(JSON.stringify({male:51,female:49,realMeshes:true,allVisibleNames:true,nonOverlappingNames:true,toggle:true,selectionKeepsCamera:true,selectedRing:true,currentName:true,stableViewport:true,stickyToolbar:true,occlusionLabel:true,freeView:true,pan:true,chooseCenter:true,reset:true,surfaceMark:true,notesPersist:true,genderIsolation:true,deletedPresetStaysDeleted:true,mobileOverflow:false,errors}));

} finally {await browser.close();}
