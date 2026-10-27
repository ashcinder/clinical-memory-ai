package cn.linji.clinical;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.app.Dialog;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.media.MediaRecorder;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.OpenableColumns;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.File;
import java.io.FileOutputStream;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.Consumer;
import static cn.linji.clinical.Json.*;

/** Pure Java / Android framework UI. All server and AI requests go through the Go API. */
public final class MainActivity extends Activity {
    private final int ink=Color.rgb(24,54,64),teal=Color.rgb(35,110,98),blue=Color.rgb(35,110,98),muted=Color.rgb(109,130,137);
    private final int canvas=Color.rgb(245,248,247),border=Color.rgb(226,235,232),mint=Color.rgb(225,241,234),soft=Color.rgb(242,247,245);
    private final ExecutorService executor=Executors.newSingleThreadExecutor();
    private final Handler handler=new Handler(Looper.getMainLooper());
    private Api api;
    private TokenStore tokenStore;
    private LinearLayout body;
    private ScrollView mainScroll;
    private TextView status;
    private boolean working=false;
    private String screen="检索",displayName="医生",conversationId=null,chatMode="knowledge";
    private final List<JSONObject> queryFiles=new ArrayList<>(),chatFiles=new ArrayList<>(),chatMessages=new ArrayList<>();
    private Consumer<JSONObject> onPicked,recordDone;
    private MediaRecorder recorder;
    private File recordingFile;
    private AlertDialog recordingDialog;
    private final Runnable recordingTimeout=this::stopRecording;

    @Override protected void onCreate(Bundle state){
        super.onCreate(state);
        if((getApplicationInfo().flags&ApplicationInfo.FLAG_DEBUGGABLE)==0)getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
        clearExports();tokenStore=new TokenStore(this);
        api=new Api(this,getSharedPreferences("settings",MODE_PRIVATE).getString("baseUrl","http://10.0.2.2:8000"),tokenStore.load());
        if(api.token.isEmpty())showAuth();else work(()->(JSONObject)api.request("/auth/me"),u->{displayName=str(u,"display_name");showScreen("检索");});
    }
    private int dp(int n){return Math.round(n*getResources().getDisplayMetrics().density);}
    private LinearLayout column(){LinearLayout l=new LinearLayout(this);l.setOrientation(LinearLayout.VERTICAL);return l;}
    private LinearLayout row(){LinearLayout l=new LinearLayout(this);l.setOrientation(LinearLayout.HORIZONTAL);l.setGravity(Gravity.CENTER_VERTICAL);return l;}
    private GradientDrawable bg(int color,int radius,Integer stroke){GradientDrawable d=new GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));if(stroke!=null)d.setStroke(dp(1),stroke);return d;}
    private TextView text(LinearLayout parent,String value,float size,int color,boolean bold){
        TextView t=new TextView(this);t.setText(value);t.setTextSize(size);t.setTextColor(color);t.setLineSpacing(dp(4),1);
        if(bold)t.setTypeface(t.getTypeface(),Typeface.BOLD);
        LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(12);parent.addView(t,p);return t;
    }
    private TextView text(LinearLayout p,String v){return text(p,v,14,ink,false);}
    private Button button(String title,boolean primary,Runnable action){
        Button b=new Button(this);b.setText(title);b.setAllCaps(false);b.setTextSize(14);b.setTypeface(Typeface.create("sans-serif-medium",Typeface.NORMAL));
        b.setTextColor(primary?Color.WHITE:ink);b.setBackground(bg(primary?teal:Color.WHITE,12,primary?null:border));
        b.setPadding(dp(16),dp(10),dp(16),dp(10));b.setMinHeight(dp(48));b.setMinimumHeight(dp(48));b.setMinimumWidth(0);
        b.setStateListAnimator(null);b.setOnClickListener(v->action.run());return b;
    }
    private Button addButton(LinearLayout parent,String title,boolean primary,Runnable action){Button b=button(title,primary,action);LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.topMargin=dp(4);p.bottomMargin=dp(12);parent.addView(b,p);return b;}
    private Button addButton(LinearLayout p,String title,Runnable a){return addButton(p,title,false,a);}
    private EditText input(LinearLayout parent,String label,String value,boolean multiline){
        TextView caption=text(parent,label,12,muted,true);caption.setLetterSpacing(.02f);
        EditText e=new EditText(this);e.setText(value);e.setTextSize(15);e.setTextColor(ink);e.setHintTextColor(Color.rgb(150,166,167));e.setPadding(dp(15),dp(14),dp(15),dp(14));e.setBackground(bg(Color.WHITE,12,border));e.setGravity(Gravity.TOP);
        e.setInputType(InputType.TYPE_CLASS_TEXT|(multiline?InputType.TYPE_TEXT_FLAG_MULTI_LINE:0));if(multiline)e.setMinLines(4);else e.setSingleLine(true);
        LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(16);parent.addView(e,p);return e;
    }
    private EditText input(LinearLayout p,String label,String value){return input(p,label,value,false);}
    private LinearLayout card(LinearLayout parent){LinearLayout c=column();c.setPadding(dp(20),dp(19),dp(20),dp(18));c.setBackground(bg(Color.WHITE,18,border));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(12);parent.addView(c,p);return c;}
    private TextView chip(String value,int color,int fill){TextView t=new TextView(this);t.setText(value);t.setTextColor(color);t.setTextSize(11);t.setTypeface(null,Typeface.BOLD);t.setPadding(dp(9),dp(5),dp(9),dp(5));t.setBackground(bg(fill,8,null));return t;}
    private void pageHeading(String title,String subtitle){TextView h=text(body,title,25,ink,true);h.setLetterSpacing(.01f);text(body,subtitle,13,muted,false);}
    private void toast(String message){Toast.makeText(this,message,Toast.LENGTH_LONG).show();}
    private <T> void work(Callable<T> action,Consumer<T> done){
        if(working){toast("正在处理，请稍候");return;}working=true;if(status!=null)status.setText("正在处理…");
        executor.execute(()->{
            try{T result=action.call();runOnUiThread(()->{if(isDestroyed())return;working=false;if(status!=null)status.setText("个人资料库 · "+displayName);try{done.accept(result);}catch(Exception e){toast(e.getMessage()==null?"界面更新失败":e.getMessage());}});}
            catch(Exception e){runOnUiThread(()->{if(isDestroyed())return;working=false;if(status!=null)status.setText("连接异常，请重试");if(e instanceof Api.ApiException&&((Api.ApiException)e).code==401){api.token="";tokenStore.save("");showAuth();}toast(e.getMessage()==null?"操作失败，请检查后端连接":e.getMessage());});}
        });
    }
    private LinearLayout scrollRoot(){ScrollView scroll=new ScrollView(this);scroll.setFillViewport(true);LinearLayout root=column();root.setPadding(dp(22),dp(26),dp(22),dp(30));scroll.addView(root);setContentView(scroll);return root;}
    private void showAuth(){
        LinearLayout root=scrollRoot();root.setBackgroundColor(canvas);
        LinearLayout brand=row();LinearLayout.LayoutParams brandGap=new LinearLayout.LayoutParams(-1,-2);brandGap.bottomMargin=dp(44);root.addView(brand,brandGap);
        TextView mark=chip("临",Color.WHITE,teal);mark.setTextSize(21);mark.setGravity(Gravity.CENTER);mark.setPadding(0,0,0,0);brand.addView(mark,new LinearLayout.LayoutParams(dp(46),dp(46)));
        LinearLayout wordmark=column();LinearLayout.LayoutParams wp=new LinearLayout.LayoutParams(-1,-2);wp.leftMargin=dp(12);brand.addView(wordmark,wp);
        TextView name=new TextView(this);name.setText("临记");name.setTextColor(ink);name.setTextSize(19);name.setTypeface(null,Typeface.BOLD);wordmark.addView(name);
        TextView descriptor=new TextView(this);descriptor.setText("你的个人诊疗经验库");descriptor.setTextSize(11);descriptor.setTextColor(muted);wordmark.addView(descriptor);
        text(root,"让过去的经验，\n在需要时回来。",28,ink,true);
        text(root,"记录、关联、回顾，找回每一次经验。",13,muted,false);
        LinearLayout loginCard=card(root);text(loginCard,"连接个人资料库",16,ink,true);
        EditText address=input(loginCard,"服务地址",api.baseUrl);address.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_URI);
        TextView connection=text(loginCard,"模拟器默认地址已填好；真机请输入电脑的局域网地址。",11,muted,false);LinearLayout form=column();
        addButton(loginCard,"连接资料库",true,()->{
            String url=address.getText().toString().trim().replaceAll("/+$","");if(!url.equals(api.baseUrl)){api.token="";tokenStore.save("");}
            api.baseUrl=url;getSharedPreferences("settings",MODE_PRIVATE).edit().putString("baseUrl",url).apply();
            work(()->(JSONObject)api.request("/health"),health->{
                boolean initialized=health.optBoolean("initialized");connection.setTextColor(teal);connection.setText(initialized?"已连接，请登录你的账户":"已连接，请创建第一个账户");form.removeAllViews();
                EditText username=input(form,"用户名","");EditText password=input(form,"密码","");password.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);
                EditText display=initialized?null:input(form,"显示名称","医生");
                addButton(form,initialized?"登录":"创建账户",true,()->{
                    if(!address.getText().toString().trim().replaceAll("/+$","").equals(api.baseUrl)){toast("地址已更改，请重新连接");return;}
                    JSONObject data=obj("username",username.getText().toString().trim(),"password",password.getText().toString(),"display_name",display==null?"医生":display.getText().toString());
                    work(()->(JSONObject)api.request(initialized?"/auth/login":"/auth/setup","POST",data),result->{api.token=str(result,"token");tokenStore.save(api.token);displayName=str(result.optJSONObject("user"),"display_name");password.setText("");showScreen("检索");});
                });
            });
        });loginCard.addView(form);text(root,"每一份记录，都只属于你的个人资料库。",11,muted,false);
    }
    private void shell(){
        LinearLayout root=column();root.setBackgroundColor(canvas);
        LinearLayout head=row();head.setPadding(dp(20),dp(12),dp(20),dp(12));head.setBackgroundColor(Color.WHITE);
        TextView mark=chip("临",Color.WHITE,teal);mark.setTextSize(19);mark.setGravity(Gravity.CENTER);mark.setPadding(0,0,0,0);head.addView(mark,new LinearLayout.LayoutParams(dp(38),dp(38)));
        LinearLayout brand=column();LinearLayout.LayoutParams brandParams=new LinearLayout.LayoutParams(0,-2,1);brandParams.leftMargin=dp(11);head.addView(brand,brandParams);
        TextView name=new TextView(this);name.setText("临记");name.setTextSize(17);name.setTypeface(null,Typeface.BOLD);name.setTextColor(ink);brand.addView(name);
        status=new TextView(this);status.setText(displayName+" · 个人资料库");status.setTextSize(10);status.setTextColor(muted);brand.addView(status);
        TextView add=chip("＋",teal,mint);add.setTextSize(22);add.setGravity(Gravity.CENTER);add.setPadding(0,0,0,0);add.setContentDescription("新建诊疗笔记");add.setOnClickListener(v->{if(working)toast("请等待当前操作完成");else editNote(null,null);});head.addView(add,new LinearLayout.LayoutParams(dp(40),dp(40)));root.addView(head);
        ScrollView scroll=new ScrollView(this);mainScroll=scroll;scroll.setFillViewport(false);scroll.setClipToPadding(false);scroll.setVerticalScrollBarEnabled(false);
        body=column();body.setPadding(dp(20),dp(23),dp(20),dp(24));scroll.addView(body);root.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));
        LinearLayout nav=row();nav.setGravity(Gravity.CENTER);nav.setPadding(dp(8),dp(7),dp(8),dp(8));nav.setBackgroundColor(Color.WHITE);
        for(String item:Arrays.asList("检索","笔记","材料","对话","设置")){
            boolean active=screen.equals(item);LinearLayout tab=column();tab.setGravity(Gravity.CENTER);tab.setPadding(0,dp(3),0,dp(3));tab.setBackground(bg(active?soft:Color.WHITE,14,null));
            LineIcon icon=new LineIcon(this,item,active?teal:muted);tab.addView(icon,new LinearLayout.LayoutParams(dp(23),dp(23)));
            TextView label=new TextView(this);label.setText(item);label.setGravity(Gravity.CENTER);label.setTextSize(10.5f);label.setTextColor(active?teal:muted);label.setTypeface(null,active?Typeface.BOLD:Typeface.NORMAL);LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.topMargin=dp(3);tab.addView(label,lp);
            tab.setContentDescription(item);tab.setClickable(true);tab.setFocusable(true);tab.setOnClickListener(v->{if(working)toast("请等待当前操作完成");else showScreen(item);});
            LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(0,dp(55),1);p.setMarginStart(dp(2));p.setMarginEnd(dp(2));nav.addView(tab,p);
        }
        root.addView(nav);setContentView(root);
    }
    private void showScreen(String name){screen=name;shell();switch(name){case "检索":searchScreen();break;case "笔记":notesScreen();break;case "材料":materialsScreen();break;case "对话":chatScreen();break;default:settingsScreen();}}
    private JSONArray ids(List<JSONObject> files){List<String> ids=new ArrayList<>();for(JSONObject f:files)ids.add(id(f));return array(ids);}
    private String fileSummary(List<JSONObject> files){List<String> labels=new ArrayList<>();for(JSONObject f:files)labels.add(str(f,"filename")+(str(f,"extracted_text").startsWith("【固定演示")?" · 示例预置文本":("ready".equals(str(f,"status"))?" · 已识别":" · 待补充文本")));return String.join("\n",labels);}
    private void addSearchFile(JSONObject f,TextView label){if(queryFiles.size()>=5){toast("最多添加 5 份搜索材料");return;}queryFiles.add(f);label.setText(fileSummary(queryFiles));label.setVisibility(View.VISIBLE);View clear=body.findViewWithTag("clear-search-files");if(clear!=null)clear.setVisibility(View.VISIBLE);if(!"ready".equals(str(f,"status")))showAttachment(f);}
    private void searchScreen(){
        TextView eyebrow=chip("你的个人经验库",teal,mint);LinearLayout.LayoutParams badge=new LinearLayout.LayoutParams(-2,-2);badge.bottomMargin=dp(15);body.addView(eyebrow,badge);
        TextView headline=text(body,"把经验，留在手边。",27,ink,true);headline.setLetterSpacing(.015f);
        text(body,"用一句话、一张图或一段录音，回到当时的记录。",13,muted,false);
        LinearLayout search=card(body);
        TextView heading=text(search,"搜索你的诊疗经验",15,ink,true);heading.setLetterSpacing(.01f);
        EditText query=new EditText(this);query.setTextSize(15);query.setTextColor(ink);query.setHintTextColor(Color.rgb(150,166,167));query.setHint("例如：咳嗽随访的复诊记录");query.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_FLAG_MULTI_LINE);query.setGravity(Gravity.TOP);query.setMinLines(2);query.setPadding(dp(14),dp(12),dp(14),dp(12));query.setBackground(bg(canvas,12,null));
        LinearLayout.LayoutParams queryLayout=new LinearLayout.LayoutParams(-1,-2);queryLayout.bottomMargin=dp(12);search.addView(query,queryLayout);
        TextView files=text(search,fileSummary(queryFiles),11,teal,false);if(queryFiles.isEmpty())files.setVisibility(View.GONE);
        TextView clearFiles=new TextView(this);clearFiles.setTag("clear-search-files");clearFiles.setText("×  清空已选材料");clearFiles.setTextSize(11);clearFiles.setTextColor(teal);clearFiles.setPadding(0,0,0,dp(10));clearFiles.setClickable(true);clearFiles.setFocusable(true);clearFiles.setVisibility(queryFiles.isEmpty()?View.GONE:View.VISIBLE);clearFiles.setOnClickListener(v->{queryFiles.clear();files.setText("");files.setVisibility(View.GONE);clearFiles.setVisibility(View.GONE);});search.addView(clearFiles);
        LinearLayout actions=row();LinearLayout.LayoutParams half=new LinearLayout.LayoutParams(0,dp(46),1);half.rightMargin=dp(8);
        Button addMedia=button("＋ 图片 / 音频",false,()->{if(queryFiles.size()>=5){toast("最多添加 5 份搜索材料");return;}pickFile(f->addSearchFile(f,files));});addMedia.setTextSize(12);actions.addView(addMedia,half);
        Button mic=button("◉ 录音输入",false,()->{if(queryFiles.size()>=5){toast("最多添加 5 份搜索材料");return;}record(f->addSearchFile(f,files));});mic.setTextSize(12);actions.addView(mic,new LinearLayout.LayoutParams(0,dp(46),1));search.addView(actions);
        TextView trySamples=new TextView(this);trySamples.setText("◇  试用图片 / 录音示例");trySamples.setTextSize(12);trySamples.setTextColor(teal);trySamples.setPadding(dp(3),dp(13),dp(3),dp(4));trySamples.setClickable(true);trySamples.setFocusable(true);trySamples.setOnClickListener(v->chooseDemoSample(files));search.addView(trySamples);
        LinearLayout results=column();Button submit=addButton(search,"检索经验   →",true,()->{android.view.inputmethod.InputMethodManager keyboard=(android.view.inputmethod.InputMethodManager)getSystemService(INPUT_METHOD_SERVICE);if(keyboard!=null)keyboard.hideSoftInputFromWindow(query.getWindowToken(),0);query.clearFocus();work(()->(JSONObject)api.request("/search","POST",obj("query",query.getText().toString(),"attachment_ids",ids(queryFiles))),data->{results.removeAllViews();text(results,"检索结果",18,ink,true);listNotes(results,array(data,"items"));for(String warning:strings(array(data,"warnings")))text(results,warning,11,muted,false);results.post(()->mainScroll.smoothScrollTo(0,Math.max(0,results.getTop()-dp(16))));});});
        LinearLayout.LayoutParams submitParams=(LinearLayout.LayoutParams)submit.getLayoutParams();submitParams.topMargin=dp(14);submitParams.bottomMargin=0;submit.setLayoutParams(submitParams);
        TextView note=text(body,"从原始笔记出发，每一次回顾都有出处。",11,muted,false);note.setPadding(dp(2),dp(3),0,dp(8));
        body.addView(results);text(results,"最近记录",18,ink,true);work(()->(JSONObject)api.request("/notes?limit=10"),data->listNotes(results,array(data,"items")));
    }
    private void chooseDemoSample(TextView label){
        if(queryFiles.size()>=5){toast("最多添加 5 份搜索材料");return;}
        try {
            String json=new String(Api.readBytes(getAssets().open("demo-assets.json"),1024*1024),java.nio.charset.StandardCharsets.UTF_8);
            JSONObject manifest=new JSONObject(json);List<JSONObject> entries=new ArrayList<>();java.util.Iterator<String> keys=manifest.keys();while(keys.hasNext()){JSONObject entry=manifest.optJSONObject(keys.next());if(entry!=null)entries.add(entry);}
            entries.sort((a,b)->(str(a,"kind")+str(a,"name")).compareTo(str(b,"kind")+str(b,"name")));
            Panel p=panel("选择测试素材");text(p.root,"这些是固定演示文件，将使用预置描述或转写。",12,muted,false);
            for(JSONObject entry:entries)addButton(p.root,str(entry,"kind")+"  ·  "+str(entry,"name"),()->{
                String name=str(entry,"name");p.dialog.dismiss();
                work(()->{byte[] bytes=Api.readBytes(getAssets().open("demo-files/"+name),20*1024*1024);return api.upload(name,bytes);},file->{toast(file.optBoolean("demo_match")?"已匹配示例，载入预置文本":"示例已上传，请检查识别文本");addSearchFile(file,label);});
            });
        }catch(Exception e){toast("无法加载测试素材");}
    }
    private void listNotes(LinearLayout parent,JSONArray notes){
        if(notes.length()==0){LinearLayout empty=card(parent);text(empty,"还没有找到相关记录",15,ink,true);text(empty,"试试其他关键词，或先保存一篇笔记。",12,muted,false);return;}
        for(JSONObject note:objects(notes)){
            LinearLayout c=card(parent);LinearLayout top=row();TextView tag=chip(str(note,"category"),teal,mint);top.addView(tag);TextView date=new TextView(this);date.setText(str(note,"occurred_on"));date.setTextColor(muted);date.setTextSize(11);date.setGravity(Gravity.END|Gravity.CENTER_VERTICAL);top.addView(date,new LinearLayout.LayoutParams(0,-2,1));LinearLayout.LayoutParams meta=new LinearLayout.LayoutParams(-1,-2);meta.bottomMargin=dp(14);c.addView(top,meta);
            text(c,str(note,"title"),16,ink,true);String content=str(note,"content");TextView excerpt=text(c,content.isEmpty()?"打开查看治疗步骤与材料":cut(content,120),12,muted,false);excerpt.setMaxLines(3);excerpt.setEllipsize(android.text.TextUtils.TruncateAt.END);
            View divider=new View(this);divider.setBackgroundColor(border);LinearLayout.LayoutParams line=new LinearLayout.LayoutParams(-1,dp(1));line.topMargin=dp(3);line.bottomMargin=dp(12);c.addView(divider,line);
            TextView foot=text(c,array(note,"steps").length()+" 个步骤  ·  "+array(note,"attachments").length()+" 份材料     查看记录  →",11,teal,true);((LinearLayout.LayoutParams)foot.getLayoutParams()).bottomMargin=0;
            c.setFocusable(true);c.setClickable(true);c.setContentDescription("打开笔记："+str(note,"title"));c.setOnClickListener(v->showNote(id(note)));
        }
    }
    private void notesScreen(){
        pageHeading("我的笔记","将观察、步骤与材料整理在一起。");addButton(body,"＋ 新建诊疗笔记",true,()->editNote(null,null));LinearLayout results=column();body.addView(results);int[] offset={0};Runnable[] load=new Runnable[1];Button more=button("加载更多笔记",false,()->load[0].run());
        load[0]=()->work(()->(JSONObject)api.request("/notes?offset="+offset[0]),data->{JSONArray items=array(data,"items");listNotes(results,items);offset[0]+=items.length();more.setVisibility(offset[0]<data.optInt("total")?View.VISIBLE:View.GONE);if(offset[0]==0)addButton(results,"导入虚构示例",()->work(()->api.request("/demo","POST"),v->showScreen("笔记")));});body.addView(more);load[0].run();
    }
    private static final class Panel{final Dialog dialog;final LinearLayout root;Panel(Dialog d,LinearLayout r){dialog=d;root=r;}}
    private Panel panel(String title){
        Dialog d=new Dialog(this);LinearLayout root=column();root.setPadding(dp(20),dp(20),dp(20),dp(26));root.setBackground(bg(Color.WHITE,20,null));
        LinearLayout header=row();LinearLayout.LayoutParams hp=new LinearLayout.LayoutParams(-1,-2);hp.bottomMargin=dp(22);root.addView(header,hp);
        TextView heading=new TextView(this);heading.setText(title);heading.setTextColor(ink);heading.setTextSize(20);heading.setTypeface(null,Typeface.BOLD);header.addView(heading,new LinearLayout.LayoutParams(0,-2,1));
        TextView close=chip("×",muted,soft);close.setTextSize(22);close.setGravity(Gravity.CENTER);close.setPadding(0,0,0,0);close.setContentDescription("关闭");close.setClickable(true);close.setOnClickListener(v->d.dismiss());header.addView(close,new LinearLayout.LayoutParams(dp(38),dp(38)));
        ScrollView scroll=new ScrollView(this);scroll.setFillViewport(false);scroll.setVerticalScrollBarEnabled(false);scroll.addView(root);d.setContentView(scroll);d.show();if(d.getWindow()!=null){d.getWindow().setBackgroundDrawableResource(android.R.color.transparent);d.getWindow().setLayout(-1,(int)(getResources().getDisplayMetrics().heightPixels*.9));}return new Panel(d,root);
    }
    private void showNote(String noteId){work(()->(JSONObject)api.request("/notes/"+noteId),note->{
        Panel p=panel(str(note,"title"));LinearLayout root=p.root;text(root,str(note,"category")+" · "+str(note,"occurred_on"),12,teal,false);text(root,String.join("  /  ",strings(array(note,"tags"))),11,muted,false);text(root,str(note,"content"));text(root,"治疗过程 / 回顾步骤",16,teal,true);int i=1;for(String step:strings(array(note,"steps")))text(root,(i++)+".  "+step);text(root,"关联的原始材料",16,teal,true);for(JSONObject file:objects(array(note,"attachments")))addButton(root,str(file,"filename"),()->showAttachment(file));
        addButton(root,"编辑笔记",true,()->{p.dialog.dismiss();editNote(note,null);});addButton(root,"带入 AI 回顾",()->{p.dialog.dismiss();conversationId=null;chatMessages.clear();chatFiles.clear();chatMode="knowledge";showScreen("对话");EditText input=body.findViewWithTag("chat-input");if(input!=null)input.setText("请回顾「"+str(note,"title")+"」的治疗步骤");});
        addButton(root,"删除笔记",()->new AlertDialog.Builder(this).setTitle("删除笔记？").setMessage("笔记会被删除，关联原件仍保留在材料归档中。").setNegativeButton("取消",null).setPositiveButton("删除",(d,w)->work(()->api.request("/notes/"+noteId,"DELETE"),v->{p.dialog.dismiss();showScreen(screen);})).show());
    });}
    private List<String> split(String text,String regex){List<String> result=new ArrayList<>();for(String value:text.split(regex)){String s=value.trim();if(!s.isEmpty())result.add(s);}return result;}
    private void editNote(JSONObject note,JSONObject material){
        Panel p=panel(note==null?"新建诊疗笔记":"编辑诊疗笔记");LinearLayout root=p.root;EditText title=input(root,"标题",note==null?str(material,"filename"):str(note,"title"));EditText category=input(root,"分类",note==null?"诊疗笔记":str(note,"category"));EditText date=input(root,"就诊 / 记录日期 YYYY-MM-DD",note==null?LocalDate.now().toString():str(note,"occurred_on"));EditText tags=input(root,"标签（逗号分隔）",note==null?"":String.join("，",strings(array(note,"tags"))));EditText content=input(root,"诊疗记录",note==null?str(material,"extracted_text"):str(note,"content"),true);EditText steps=input(root,"治疗 / 回顾步骤（每行一个）",note==null?"":String.join("\n",strings(array(note,"steps"))),true);
        List<JSONObject> files=note==null?new ArrayList<>():new ArrayList<>(objects(array(note,"attachments")));if(material!=null)files.add(material);LinearLayout fileView=column();root.addView(fileView);drawEditorFiles(fileView,files);addButton(root,"＋ 关联新材料",()->pickFile(f->{if(files.size()>=20){toast("最多关联 20 份材料");return;}files.add(f);drawEditorFiles(fileView,files);}));text(root,"云端识别会发送原件到对应服务。请先脱敏并核对识别内容。",11,muted,false);
        addButton(root,"保存笔记",true,()->{JSONObject data=obj("title",title.getText().toString(),"category",category.getText().toString(),"occurred_on",date.getText().toString(),"tags",array(split(tags.getText().toString(),"[,，]")),"content",content.getText().toString(),"steps",array(split(steps.getText().toString(),"\\n")),"attachment_ids",ids(files));work(()->api.request(note==null?"/notes":"/notes/"+id(note),note==null?"POST":"PUT",data),v->{p.dialog.dismiss();toast("笔记已保存");showScreen(screen);});});
    }
    private void drawEditorFiles(LinearLayout parent,List<JSONObject> files){parent.removeAllViews();for(JSONObject f:files){text(parent,str(f,"filename"),12,teal,false);addButton(parent,"移除关联",()->{files.remove(f);drawEditorFiles(parent,files);});}}
    private void materialsScreen(){
        pageHeading("材料归档","图片、录音与文档，都可以关联到笔记。");addButton(body,"＋ 上传原始材料",true,()->pickFile(f->{showScreen("材料");showAttachment(f);}));
        work(()->(JSONArray)api.request("/attachments"),files->{if(files.length()==0)text(body,"还没有材料，上传后可核对文本并关联笔记。",13,muted,false);for(JSONObject f:objects(files)){LinearLayout c=card(body);text(c,str(f,"filename"),15,ink,true);text(c,("ready".equals(str(f,"status"))?"已识别":"待补充文本")+" · "+(f.isNull("note_id")?"未关联笔记":"已关联笔记"),11,teal,false);addButton(c,"查看 / 核对材料",()->showAttachment(f));}});
    }
    private void showAttachment(JSONObject f){
        Panel p=panel("原始材料 · 核对文本");LinearLayout root=p.root;text(root,str(f,"filename"),16,ink,true);addButton(root,"打开原件",()->openFile(f));EditText extraction=input(root,"识别 / 转写文本（可以手动修正）",str(f,"extracted_text"),true);text(root,"保存会更新关联笔记的检索索引。未配置服务时，请手动补充文本。",11,muted,false);
        addButton(root,"保存文本",true,()->work(()->(JSONObject)api.request("/attachments/"+id(f)+"/text","PUT",obj("text",extraction.getText().toString())),updated->{put(f,"extracted_text",str(updated,"extracted_text"));put(f,"status",str(updated,"status"));p.dialog.dismiss();toast("文本已保存");}));
        addButton(root,"重新提取",()->work(()->(JSONObject)api.request("/attachments/"+id(f)+"/extract","POST"),updated->{put(f,"extracted_text",str(updated,"extracted_text"));put(f,"status",str(updated,"status"));extraction.setText(str(updated,"extracted_text"));toast("提取完成，请核对文本；为空时可手动补充");}));
        addButton(root,f.isNull("note_id")?"用于新笔记":"查看关联笔记",()->{p.dialog.dismiss();if(f.isNull("note_id"))editNote(null,f);else showNote(str(f,"note_id"));});
        addButton(root,"删除材料",()->new AlertDialog.Builder(this).setTitle("删除材料？").setMessage("原始文件和识别文本都会删除。").setNegativeButton("取消",null).setPositiveButton("删除",(d,w)->work(()->api.request("/attachments/"+id(f),"DELETE"),v->{queryFiles.removeIf(x->id(x).equals(id(f)));chatFiles.removeIf(x->id(x).equals(id(f)));p.dialog.dismiss();showScreen(screen);})).show());
    }
    private File saveExport(String name,byte[] bytes)throws Exception{File dir=new File(getCacheDir(),"exports");if(!dir.exists()&&!dir.mkdirs())throw new IllegalStateException("无法创建临时目录");File file=new File(dir,name);try(FileOutputStream out=new FileOutputStream(file)){out.write(bytes);}return file;}
    private Uri exportUri(File file){return Uri.parse("content://"+getPackageName()+".files/"+file.getName());}
    private void openFile(JSONObject f){work(()->{String extension=str(f,"filename");extension=extension.substring(extension.lastIndexOf('.')+1).replaceAll("[^A-Za-z0-9]","");return saveExport(UUID.randomUUID()+"."+extension,api.download("/attachments/"+id(f)+"/file"));},file->{try{startActivity(new Intent(Intent.ACTION_VIEW).setDataAndType(exportUri(file),str(f,"mime")).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION));}catch(Exception e){toast("设备没有此格式的查看器，仍可在这里查看识别文本");}});}
    private void addChatFile(JSONObject f,TextView label){if(chatFiles.size()>=5){toast("最多添加 5 份对话材料");return;}chatFiles.add(f);label.setText(fileSummary(chatFiles));if(!"ready".equals(str(f,"status")))showAttachment(f);}
    private void chatScreen(){
        pageHeading("AI 对话","回顾笔记，或聊聊日常。");LinearLayout modes=row();String[][] modeLabels={{"knowledge","历史经验回顾"},{"general","日常对话"}};for(String[] entry:modeLabels){Button b=button(entry[1],chatMode.equals(entry[0]),()->{chatMode=entry[0];conversationId=null;chatMessages.clear();chatFiles.clear();showScreen("对话");});LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(0,-2,1);lp.setMarginEnd(dp(6));modes.addView(b,lp);}body.addView(modes);
        addButton(body,"＋ 新对话",()->{conversationId=null;chatMessages.clear();chatFiles.clear();showScreen("对话");});addButton(body,"查看历史对话",this::showHistory);
        LinearLayout messages=column();body.addView(messages);if(chatMessages.isEmpty())text(messages,"可以回顾自己的笔记，或切换日常对话。未配置 DeepSeek 时展示本地原文摘录。",12,muted,false);for(JSONObject m:chatMessages){LinearLayout c=card(messages);text(c,"user".equals(str(m,"role"))?displayName:"临记助手",11,teal,false);text(c,str(m,"content"));for(JSONObject source:objects(array(m,"sources")))addButton(c,"来源："+str(source,"title"),()->showNote(id(source)));}
        EditText message=input(body,"输入对话内容","",true);message.setTag("chat-input");TextView files=text(body,fileSummary(chatFiles),11,teal,false);addButton(body,"＋ 添加图片 / 音频 / 文档",()->pickFile(f->addChatFile(f,files)));addButton(body,"♩ 录音输入",()->record(f->addChatFile(f,files)));
        addButton(body,"发送  ↗",true,()->{String value=message.getText().toString().trim();if(value.isEmpty()){toast("请输入问题");return;}JSONObject data=obj("message",value,"mode",chatMode,"attachment_ids",ids(chatFiles));if(conversationId!=null)put(data,"conversation_id",conversationId);work(()->(JSONObject)api.request("/chat","POST",data),reply->{chatMessages.add(obj("role","user","content",value));chatMessages.add(put(reply,"role","assistant"));conversationId=str(reply,"conversation_id");chatFiles.clear();showScreen("对话");});});text(body,"历史记录供回顾，诊疗决策由医生独立判断。",10,muted,false);
    }
    private void showHistory(){work(()->(JSONArray)api.request("/conversations"),items->{Panel p=panel("历史对话");if(items.length()==0)text(p.root,"还没有对话记录。");for(JSONObject h:objects(items))addButton(p.root,str(h,"title"),()->work(()->(JSONObject)api.request("/conversations/"+id(h)),data->{conversationId=id(h);chatMode=str(h,"mode");chatMessages.clear();chatMessages.addAll(objects(array(data,"messages")));chatFiles.clear();p.dialog.dismiss();showScreen("对话");}));});}
    private void settingsScreen(){
        pageHeading("我的设置","管理连接与个人资料。");work(()->(JSONObject)api.request("/settings"),s->{LinearLayout c=card(body);text(c,"模型服务 "+str(s,"ai_provider")+"："+(s.optBoolean("ai_enabled")?"已配置":"本地演示模式"),14,teal,false);text(c,"模型："+str(s,"ai_model"),12,ink,false);text(c,"后端：Go / 数据库：MySQL",12,teal,false);text(c,"图片："+(s.optBoolean("vision_enabled")?str(s,"vision_provider")+" / "+str(s,"vision_model"):"可手动核对文本"),12,ink,false);text(c,"音频："+(s.optBoolean("asr_enabled")?"ASR 已配置":"请配置转写服务或手动填写"),12,ink,false);text(c,"接口："+api.baseUrl,12,muted,false);text(body,"在后端 .env 配置模型密钥并重启，密钥只保存在后端。",12,muted,false);text(body,str(s,"external_data_notice"),12,muted,false);
            addButton(body,"导出笔记 JSON",()->work(()->saveExport("clinical-notes-"+System.currentTimeMillis()+".json",api.download("/export")),file->{Intent share=new Intent(Intent.ACTION_SEND).setType("application/json").putExtra(Intent.EXTRA_STREAM,exportUri(file)).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);startActivity(Intent.createChooser(share,"保存 / 分享笔记"));}));
            addButton(body,"退出账户 / 更换后端",()->work(()->api.request("/auth/logout","POST"),v->{api.token="";tokenStore.save("");queryFiles.clear();chatFiles.clear();chatMessages.clear();conversationId=null;clearExports();showAuth();}));
        });
    }
    private void pickFile(Consumer<JSONObject> done){if(working){toast("请等待当前操作完成");return;}onPicked=done;Intent i=new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*");i.putExtra(Intent.EXTRA_MIME_TYPES,new String[]{"image/*","audio/*","application/pdf","text/plain","text/markdown"});startActivityForResult(i,101);}
    @Override protected void onActivityResult(int requestCode,int resultCode,Intent data){
        super.onActivityResult(requestCode,resultCode,data);if(requestCode!=101||resultCode!=RESULT_OK||data==null||data.getData()==null||onPicked==null)return;Uri uri=data.getData();Consumer<JSONObject> done=onPicked;
        work(()->{String name="material.txt";try(Cursor c=getContentResolver().query(uri,new String[]{OpenableColumns.DISPLAY_NAME},null,null,null)){if(c!=null&&c.moveToFirst())name=c.getString(0);}java.io.InputStream stream=getContentResolver().openInputStream(uri);if(stream==null)throw new IllegalArgumentException("无法读取文件");return api.upload(name,Api.readBytes(stream,20*1024*1024));},f->{toast(f.optBoolean("demo_match")?"已匹配演示文件，载入预置文本":"ready".equals(str(f,"status"))?"材料已保存并提取文本":"材料已保存，请核对或补充识别文本");done.accept(f);});
    }
    private void record(Consumer<JSONObject> done){if(working){toast("请等待当前操作完成");return;}recordDone=done;if(checkSelfPermission(Manifest.permission.RECORD_AUDIO)!=PackageManager.PERMISSION_GRANTED){requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO},102);return;}startRecording();}
    @Override public void onRequestPermissionsResult(int code,String[] permissions,int[] results){super.onRequestPermissionsResult(code,permissions,results);if(code==102){if(results.length>0&&results[0]==PackageManager.PERMISSION_GRANTED)startRecording();else toast("未授权麦克风，也可以上传已有音频");}}
    @SuppressWarnings("deprecation") private void startRecording(){
        try{recordingFile=new File(getCacheDir(),"recording-"+System.currentTimeMillis()+".m4a");recorder=android.os.Build.VERSION.SDK_INT>=31?new MediaRecorder(this):new MediaRecorder();recorder.setAudioSource(MediaRecorder.AudioSource.MIC);recorder.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4);recorder.setAudioEncoder(MediaRecorder.AudioEncoder.AAC);recorder.setOutputFile(recordingFile.getAbsolutePath());recorder.setMaxDuration(120000);recorder.prepare();recorder.start();recordingDialog=new AlertDialog.Builder(this).setTitle("正在录音…").setMessage("最长 2 分钟，结束后上传到个人材料库。").setCancelable(false).setPositiveButton("结束并上传",(d,w)->stopRecording()).setNegativeButton("取消",(d,w)->cancelRecording()).show();handler.postDelayed(recordingTimeout,119000);}catch(Exception e){cancelRecording();toast("无法开始录音，请检查麦克风权限");}
    }
    private void stopRecording(){
        if(recorder==null)return;handler.removeCallbacks(recordingTimeout);if(recordingDialog!=null)recordingDialog.dismiss();recordingDialog=null;try{recorder.stop();}catch(Exception e){recorder.release();recorder=null;if(recordingFile!=null)recordingFile.delete();toast("录音太短，请重新录制");return;}recorder.release();recorder=null;File file=recordingFile;Consumer<JSONObject> done=recordDone;if(file==null||done==null)return;
        work(()->{try{return api.upload(file.getName(),java.nio.file.Files.readAllBytes(file.toPath()));}finally{file.delete();}},done);
    }
    private void cancelRecording(){handler.removeCallbacks(recordingTimeout);if(recorder!=null){try{recorder.stop();}catch(Exception ignored){}recorder.release();recorder=null;}if(recordingFile!=null)recordingFile.delete();if(recordingDialog!=null)recordingDialog.dismiss();recordingDialog=null;}
    private void clearExports(){File dir=new File(getCacheDir(),"exports");File[] files=dir.listFiles();if(files!=null)for(File f:files)f.delete();dir.delete();}
    @Override protected void onStop(){super.onStop();if(recorder!=null){cancelRecording();toast("录音已取消，可返回后重新录制");}}
    @Override protected void onDestroy(){cancelRecording();executor.shutdownNow();super.onDestroy();}
}
