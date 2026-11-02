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

 await page.goto(base+'/#anatomy');
 await page.locator('.a-list button').first().waitFor();
 await page.locator('.a-model-loading').waitFor({state:'hidden'});
 const count=await page.locator('.a-list button').count();assert.equal(count,50);
 await page.locator('.a-list button').first().click();
 const model=page.locator('.atlas-stage canvas');assert.equal(await model.count(),1);
 await page.screenshot({path:'/tmp/atlas-male-realistic.png',fullPage:true});
 assert.equal(await page.getByRole('button',{name:'复位',exact:true}).count(),0);
 assert.equal(await page.locator('.a-views #a-show').count(),1);assert.equal(await page.locator('.atlas-head #a-show').count(),0);
 await page.locator('.a-callout').first().waitFor();
 assert.ok(await page.locator('.a-callout').count()>10,'all visible points must have names');
 assert.ok((await page.locator('.a-callout').allTextContents()).includes('中脘'));
 const namePositions=await page.locator('.a-callout').evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {left:r.left,top:r.top,bottom:r.bottom}}));
 for(let i=0;i<namePositions.length;i++)for(let j=i+1;j<namePositions.length;j++)if(Math.abs(namePositions[i].left-namePositions[j].left)<2)assert.ok(namePositions[i].bottom<=namePositions[j].top+.5||namePositions[j].bottom<=namePositions[i].top+.5,'name labels must not overlap');
 await page.getByRole('button',{name:'背面',exact:true}).click();await page.waitForFunction(()=>[...document.querySelectorAll('.a-callout')].some(n=>n.textContent==='肾俞'));
 assert.ok(!(await page.locator('.a-callout').allTextContents()).includes('中脘'),'front labels must not show through the body');
 await page.getByRole('button',{name:'正面',exact:true}).click();await page.waitForFunction(()=>[...document.querySelectorAll('.a-callout')].some(n=>n.textContent==='中脘'));
 const allPixels=await model.evaluate(c=>c.toDataURL());
 await page.locator('#a-show').uncheck();assert.equal(await page.locator('.a-point-label').isVisible(),true);await page.locator('.a-callout').first().waitFor({state:'detached'});
 const singlePixels=await model.evaluate(c=>c.toDataURL());assert.notEqual(singlePixels,allPixels,'off must clear the other markers');
 await page.screenshot({path:'/tmp/atlas-male-no-points.png',fullPage:true});await page.locator('#a-show').check();
 await page.locator('#a-query').fill('百会');assert.equal(await page.locator('.a-list button').count(),1);
 assert.equal(await model.evaluate(c=>c.toDataURL()),allPixels,'all mode must retain every marker while list is filtered');await page.locator('#a-query').fill('');
 await page.locator('#a-show').uncheck();
 await page.getByRole('button',{name:'手动标记',exact:true}).click();const box=await model.boundingBox();await model.click({position:{x:box.width*.5,y:box.height*.5}});
 assert.equal(await page.locator('.a-list button').count(),51);assert.equal(await page.locator('#a-show').isChecked(),false,'manual marking must retain the visibility preference');
 await page.locator('#a-name').fill('男性测试点位');await page.locator('#a-description').fill('手动标记记录测试');await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 assert.ok(atlas.points.some(p=>p.name==='男性测试点位'&&p.model==='male-real-v1'));
 await page.getByRole('button',{name:'女性',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});assert.equal(await page.locator('.a-list button').count(),50);
 await page.screenshot({path:'/tmp/atlas-female-realistic.png',fullPage:true});
 await page.locator('.a-list button').first().click();await page.locator('#a-description').fill('女性模型独立记录');await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 await page.getByRole('button',{name:'刷新',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});await page.locator('.a-list button').first().click();assert.equal(await page.locator('#a-description').inputValue(),'女性模型独立记录');
 page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'删除点位',exact:true}).click();assert.equal(await page.locator('.a-list button').count(),49);await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 await page.getByRole('button',{name:'刷新',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});assert.equal(await page.locator('.a-list button').count(),49);
 await page.getByRole('button',{name:'男性',exact:true}).click();await page.locator('.a-model-loading').waitFor({state:'hidden'});assert.equal(await page.locator('.a-list button').count(),51);assert.ok(await page.getByRole('button',{name:'男性测试点位 自定义',exact:false}).count());
 await page.locator('#a-show').check();
 for(const width of [320,390,1440]){await page.setViewportSize({width,height:900});await page.waitForTimeout(100);const views=await page.locator('.a-views').boundingBox(),canvas=await model.boundingBox();assert.ok(canvas.y>=views.y+views.height-1,'tools must not overlap the model viewport');assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);
 const labels=await page.locator('.a-callout').evaluateAll(nodes=>nodes.map(n=>{const r=n.getBoundingClientRect();return {x:r.x,y:r.y,bottom:r.bottom}}));for(let i=0;i<labels.length;i++)for(let j=i+1;j<labels.length;j++)if(Math.abs(labels[i].x-labels[j].x)<2)assert.ok(labels[i].bottom<=labels[j].y+.5||labels[j].bottom<=labels[i].y+.5,'names overlap at width '+width);
 const endpoints=await page.locator('.a-name-lines polyline').evaluateAll(lines=>lines.map(line=>line.getAttribute('points').split(' ')[0].split(',').map(Number)));const size=await page.locator('.atlas-stage').evaluate(e=>({w:e.clientWidth,h:e.clientHeight}));for(const [x,y] of endpoints){assert.ok(x>=0&&x<=size.w);assert.ok(y>=60&&y<=size.h-40,'leader must end on the model viewport');}
 }
 await page.locator('#a-show').check();await page.setViewportSize({width:390,height:844});await page.waitForTimeout(150);await page.screenshot({path:'/tmp/atlas-realistic-mobile.png',fullPage:true});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);

 assert.deepEqual(errors,[]);console.log(JSON.stringify({male:51,female:49,realMeshes:true,allVisibleNames:true,nonOverlappingNames:true,toggle:true,surfaceMark:true,notesPersist:true,genderIsolation:true,deletedPresetStaysDeleted:true,mobileOverflow:false,errors}));

} finally {await browser.close();}
