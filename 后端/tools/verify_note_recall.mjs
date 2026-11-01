// Browser regression using synthetic API fixtures. No real account or data is changed.
// Install Playwright separately, or set PLAYWRIGHT_MODULE_PATH to its index.mjs.
import assert from 'node:assert/strict';
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE_PATH || 'playwright');
const base=(process.argv[2] || 'http://localhost:5173').replace(/\/$/,'');
const browser=await chromium.launch({channel:'chrome',headless:true});
try {
 const page=await browser.newPage();
 const errors=[];const sent=[];page.on('pageerror',e=>errors.push(e.message));
 const note={id:'11111111111111111111111111111111',title:'回顾按钮回归验证',category:'诊疗笔记',note_type:'record',note_types:['record'],tags:['回归验证'],content:'合成资料，用于检查回顾按钮。',steps:['检查第一步','检查第二步'],attachments:[],occurred_on:'2026-11-01',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',version:'fixture-v1'};
 await page.route('**/api/**',async route=>{
  const p=new URL(route.request().url()).pathname.replace(new URL(base).pathname.replace(/\/$/,''),'');
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

 await page.goto(base+'/#notes');
 await page.getByRole('button').filter({has:page.getByRole('heading',{name:note.title,exact:true})}).click();
 await page.getByRole('button',{name:'回顾这篇记录',exact:true}).click();
 const input=page.getByRole('textbox',{name:'输入对话内容',exact:true});
 await input.waitFor();
 assert.match(await input.inputValue(),new RegExp(note.title));
 assert.equal(await page.getByRole('button',{name:'发送消息',exact:true}).isEnabled(),true,'RECALL_READY_FAIL: prefilled recall must enable Send');
 assert.equal(await page.locator('#dialog').evaluate(el=>el.open),false);
 await page.evaluate(()=>renderChat());
 assert.match(await input.inputValue(),new RegExp(note.title),'recall draft must survive rerender');
 assert.equal(sent.length,0,'recall must not send automatically');
 await page.evaluate(id=>showNote(id),note.id);
 page.once('dialog',dialog=>dialog.dismiss());
 await page.getByRole('button',{name:'回顾这篇记录',exact:true}).click();
 assert.equal(await page.locator('#dialog').evaluate(el=>el.open),true,'cancel must preserve current dialog');
 await page.getByRole('button',{name:'关闭弹窗',exact:true}).click();
 await page.getByRole('button',{name:'发送消息',exact:true}).click();
 await page.getByText('合成回顾回复',{exact:true}).waitFor();
 assert.equal(sent.length,1);
 assert.equal(sent[0].note_id,note.id,'selected note ID must reach the backend');
 await page.getByRole('button',{name:'新对话',exact:true}).click();
 await page.getByRole('button',{name:'日常对话',exact:true}).click();
 await input.fill('普通对话');
 await page.getByRole('button',{name:'发送消息',exact:true}).click();
 await page.getByText('合成回顾回复',{exact:true}).waitFor();
 assert.equal(sent.length,2);
 assert.equal(sent[1].note_id,undefined,'new chat must clear previous note scope');
 assert.deepEqual(errors,[]);
 console.log(JSON.stringify({url:base,recallReady:true,draftRetained:true,cancelRetainsDraft:true,selectedNoteSent:true,newChatClearsScope:true,scriptErrors:errors}));
} finally {await browser.close();}
