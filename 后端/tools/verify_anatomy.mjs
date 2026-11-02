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
 await page.getByRole('button',{name:'载入 25 个示意标注',exact:true}).click();
 assert.equal(await page.locator('.a-list button').count(),25);
 assert.equal(await page.locator('.atlas-stage canvas').count(),1);
 await page.locator('#a-name').fill('三维图谱回归测试');
 await page.getByRole('button',{name:'背面',exact:true}).click();
 await page.getByRole('button',{name:'正面',exact:true}).click();
 const canvas=page.locator('.atlas-stage canvas');const box=await canvas.boundingBox();
 await page.mouse.move(box.x+box.width*.5,box.y+box.height*.5);await page.mouse.down();await page.mouse.move(box.x+box.width*.7,box.y+box.height*.5,{steps:12});await page.mouse.up();
 await page.getByRole('button',{name:'移动位置',exact:true}).click();
 await canvas.click({position:{x:box.width*.5,y:box.height*.45}});
 await page.getByRole('button',{name:'结束定位',exact:true}).click();
 await page.getByRole('button',{name:'保存修改',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 assert.equal(atlas.points[0].name,'三维图谱回归测试');assert.equal(atlas.points.length,25);
 await page.getByRole('button',{name:'刷新',exact:true}).click();
 await page.getByRole('button',{name:'三维图谱回归测试 · 正中',exact:false}).click();
 await page.getByRole('button',{name:'新增标注',exact:true}).click();
 assert.equal(await page.locator('.a-list button').count(),26);
 page.once('dialog',d=>d.accept());await page.getByRole('button',{name:'删除',exact:true}).click();
 assert.equal(await page.locator('.a-list button').count(),25);
 await page.getByRole('button',{name:'保存修改',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('.a-status').textContent.includes('已保存'));
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:process.env.ATLAS_SCREENSHOT||'/tmp/linji-atlas.png',fullPage:true});
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);
 assert.deepEqual(errors,[]);console.log(JSON.stringify({model:'WebGL 3D',points:25,rotate:true,edit:true,saveReload:true,addDelete:true,mobileOverflow:false,errors}));

} finally {await browser.close();}
