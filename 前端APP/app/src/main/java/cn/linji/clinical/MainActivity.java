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
import android.view.ViewGroup;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.Spinner;
import android.widget.AdapterView;
import android.widget.ArrayAdapter;
import android.widget.CheckBox;
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
import java.util.function.BooleanSupplier;
import android.widget.ProgressBar;
import android.widget.HorizontalScrollView;
import android.graphics.drawable.RippleDrawable;
import android.content.res.ColorStateList;
import android.text.TextWatcher;
import android.text.Editable;
import static cn.linji.clinical.Json.*;

/** Pure Java / Android framework UI. All server and AI requests go through the Go API. */
public final class MainActivity extends Activity {
    private final int ink=Color.rgb(32,55,58),teal=Color.rgb(34,102,92),blue=Color.rgb(34,102,92),muted=Color.rgb(96,117,122);
    private final int canvas=Color.rgb(243,246,247),border=Color.rgb(225,232,233),mint=Color.rgb(232,242,238),soft=Color.rgb(246,249,248);
    private final ExecutorService executor=Executors.newSingleThreadExecutor();
    private final Handler handler=new Handler(Looper.getMainLooper());
    private Api api;
    private TokenStore tokenStore;
    private LinearLayout body,screenHost,chatMessageList;
    private EditText chatInput;
    private View chatSendButton;
    private TextView chatModelView;
    private boolean chatSending=false;
    private String chatError="",recallNoteId="";
    private ScrollView mainScroll;
    private TextView status;
    private ProgressBar busyIndicator;
    private String searchDraft="",chatDraft="";
 private final List<String> searchCategories=new ArrayList<>(),searchTags=new ArrayList<>(),browseTypes=new ArrayList<>(),browseTags=new ArrayList<>(),searchScopes=new ArrayList<>(),scopeTags=new ArrayList<>();
 private String completedScopeSummary="";
 private String settingsSection="account";
    private JSONObject lastSearch=null;
    private JSONObject detailTemplates=new JSONObject(),detailFieldSpecs=new JSONObject(),detailPrimaryFields=new JSONObject();
    private JSONArray learningTypes=LearningCatalog.defaults(),learningTags=new JSONArray();

    private boolean dateOrder=false;
    private String completedSearchQuery="",completedInputLabel="";
    private boolean completedSearchHadFiles=false;
    private LinearLayout searchAttachmentList;
    private Runnable searchInputChanged=()->{};
    private boolean pickingSearchMaterial=false;
    private boolean working=false, syncChecking=false, settingsDirty=false;
    private String syncVersion="";
    private final Runnable syncLoop=new Runnable(){public void run(){pollSync();handler.postDelayed(this,5000);}};
    private String screen="检索",displayName="医生",conversationId=null,chatMode="knowledge";
    private final List<JSONObject> queryFiles=new ArrayList<>(),chatFiles=new ArrayList<>(),chatMessages=new ArrayList<>();
    private Consumer<JSONObject> onPicked,recordDone;
    private java.util.function.BiConsumer<List<JSONObject>, Boolean> onBatchPicked;
    private int batchCapacity;
    private Runnable onBatchUploadStart;
    private MediaRecorder recorder;
    private File recordingFile;
    private AlertDialog recordingDialog;
    private final Runnable recordingTimeout=this::stopRecording;

    @Override protected void onCreate(Bundle state){
        super.onCreate(state);
        if(state!=null){searchDraft=state.getString("search_draft","");ArrayList<String> types=state.getStringArrayList("search_types"),tags=state.getStringArrayList("search_tags");if(types!=null)searchScopes.addAll(types);if(tags!=null)scopeTags.addAll(tags);try{queryFiles.addAll(objects(new JSONArray(state.getString("search_files","[]"))));}catch(Exception ignored){}pickingSearchMaterial=state.getBoolean("picking_search_material",false);if(pickingSearchMaterial)onPicked=file->addSearchFile(file,null);}
        if((getApplicationInfo().flags&ApplicationInfo.FLAG_DEBUGGABLE)==0)getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
        clearExports();tokenStore=new TokenStore(this);
        api=new Api(this,getSharedPreferences("settings",MODE_PRIVATE).getString("baseUrl",BuildConfig.DEFAULT_SERVER_URL),tokenStore.load());
        if(api.token.isEmpty())showAuth();else work(()->(JSONObject)api.request("/auth/me"),u->{displayName=str(u,"display_name");showScreen("检索");});
    }
    @Override protected void onSaveInstanceState(Bundle saved){
        saved.putString("search_draft",searchDraft);saved.putStringArrayList("search_types",new ArrayList<>(searchScopes));saved.putStringArrayList("search_tags",new ArrayList<>(scopeTags));saved.putBoolean("picking_search_material",pickingSearchMaterial);
        List<JSONObject> metadata=new ArrayList<>();for(JSONObject file:queryFiles)metadata.add(obj("id",id(file),"filename",str(file,"filename"),"mime",str(file,"mime"),"status",str(file,"status"),"size",file.optLong("size")));saved.putString("search_files",array(metadata).toString());super.onSaveInstanceState(saved);
    }
    @Override protected void onStart(){super.onStart();handler.postDelayed(syncLoop,1000);}
    private boolean hasDraftText(View view){if(view instanceof EditText)return !((EditText)view).getText().toString().trim().isEmpty();if(view instanceof ViewGroup){ViewGroup group=(ViewGroup)view;for(int i=0;i<group.getChildCount();i++)if(hasDraftText(group.getChildAt(i)))return true;}return false;}
    private android.webkit.WebView anatomyWeb;
    private boolean hasSyncDraft(){return "穴位".equals(screen) || settingsDirty || ("检索".equals(screen)&&!queryFiles.isEmpty()) || ("对话".equals(screen)&&!chatFiles.isEmpty()) || (body!=null&&hasDraftText(body));}
    private void pollSync(){
        if(api==null||api.token.isEmpty()||working||syncChecking||!hasWindowFocus()||recorder!=null||isFinishing())return;
        syncChecking=true;executor.execute(()->{try{JSONObject remote=(JSONObject)api.request("/sync");runOnUiThread(()->{syncChecking=false;if(isDestroyed()||api.token.isEmpty())return;String version=str(remote,"version");if(syncVersion.isEmpty()){syncVersion=version;return;}if(!syncVersion.equals(version)){if(working||!hasWindowFocus()||hasSyncDraft()){if(status!=null)status.setText("其他设备有更新 · 点击同步");return;}syncVersion=version;displayName=str(remote.optJSONObject("user"),"display_name");reloadSyncedScreen();}});}catch(Exception ignored){runOnUiThread(()->syncChecking=false);}});
    }
    private void reloadSyncedScreen(){lastSearch=null;if("对话".equals(screen)&&conversationId!=null){String target=conversationId;work(()->(JSONObject)api.request("/conversations/"+target),data->{if(target.equals(conversationId)){chatMessages.clear();chatMessages.addAll(objects(array(data,"messages")));showScreen(screen);}});}else showScreen(screen);}
    private void syncNow(){if(working)return;if(anatomyWeb!=null){anatomyWeb.evaluateJavascript("document.querySelector('.model-online:not([hidden]) [data-action=reload], .model-online:not([hidden]) [data-command=reload], .model-local:not([hidden]) [data-action=reload]')?.click()",null);return;}if(hasSyncDraft()){new AlertDialog.Builder(this).setTitle("同步最新数据？").setMessage("将重新载入当前页面，尚未保存的输入会被放弃。").setNegativeButton("取消",null).setPositiveButton("同步",(d,w)->{queryFiles.clear();chatFiles.clear();searchDraft="";chatDraft="";settingsDirty=false;reloadSyncedScreen();}).show();}else reloadSyncedScreen();}
    private void editProfile(JSONObject user){
        Panel p=panel("个人账户");if(p.dialog.getWindow()!=null)p.dialog.getWindow().setLayout(getResources().getDisplayMetrics().widthPixels-dp(28),Math.min(dp(440),(int)(getResources().getDisplayMetrics().heightPixels*.85)));String saved=str(user,"display_name");text(p.root,"登录用户名",11,muted,false);text(p.root,str(user,"username"),14,ink,true);EditText name=input(p.root,"显示名称",saved);name.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(40)});name.setSingleLine(true);TextView status=text(p.root,"已保存",11,muted,false),error=text(p.root,"",12,Color.rgb(163,77,67),false);error.setVisibility(View.GONE);LinearLayout actions=row();
        Button reset=button("还原",false,()->{name.setText(saved);name.setSelection(name.length());}),save=button("保存修改",true,()->{});actions.addView(reset,new LinearLayout.LayoutParams(0,dp(44),1));LinearLayout.LayoutParams saveParams=new LinearLayout.LayoutParams(0,dp(44),1);saveParams.leftMargin=dp(8);actions.addView(save,saveParams);p.root.addView(actions);p.dialog.dirty=()->!saved.equals(name.getText().toString().trim());
        Runnable changed=()->{boolean dirty=p.dialog.dirty.getAsBoolean(),valid=!name.getText().toString().trim().isEmpty();save.setEnabled(dirty&&valid&&!working);save.setAlpha(dirty&&valid?1f:.45f);reset.setEnabled(dirty&&!working);status.setText(dirty?"未保存":"已保存");error.setVisibility(View.GONE);};
        name.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence v,int a,int b,int c){}public void onTextChanged(CharSequence v,int a,int b,int c){changed.run();}public void afterTextChanged(Editable e){}});changed.run();
        save.setOnClickListener(v->{String value=name.getText().toString().trim();if(working||value.isEmpty()||value.equals(saved))return;name.setEnabled(false);save.setEnabled(false);reset.setEnabled(false);status.setText("正在保存…");error.setVisibility(View.GONE);work(()->(JSONObject)api.request("/auth/me","PUT",obj("display_name",value)),updated->{displayName=str(updated,"display_name");p.dialog.dismiss();toast("账户信息已保存");if("设置".equals(screen)&&"account".equals(settingsSection))showScreen("设置");else if(MainActivity.this.status!=null)MainActivity.this.status.setText(displayName+" · 点击同步");},failure->{name.setEnabled(true);save.setEnabled(true);reset.setEnabled(true);status.setText("保存失败");error.setText(failure.getMessage()==null?"保存失败，请重试":failure.getMessage());error.setVisibility(View.VISIBLE);});});
    }

    private int dp(int n){return Math.round(n*getResources().getDisplayMetrics().density);}
    private LinearLayout column(){LinearLayout l=new LinearLayout(this);l.setOrientation(LinearLayout.VERTICAL);return l;}
    private LinearLayout row(){LinearLayout l=new LinearLayout(this);l.setOrientation(LinearLayout.HORIZONTAL);l.setGravity(Gravity.CENTER_VERTICAL);return l;}
    private GradientDrawable bg(int color,int radius,Integer stroke){GradientDrawable d=new GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));if(stroke!=null)d.setStroke(dp(1),stroke);return d;}
    private TextView text(LinearLayout parent,String value,float size,int color,boolean bold){
        TextView t=new TextView(this);t.setText(value);t.setTextSize(size);t.setTextColor(color);t.setLineSpacing(dp(4),1);
        if(bold)t.setTypeface(t.getTypeface(),Typeface.BOLD);
        LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(10);parent.addView(t,p);return t;
    }
    private TextView text(LinearLayout p,String v){return text(p,v,14,ink,false);}
    private Button button(String title,boolean primary,Runnable action){
        Button b=new Button(this);b.setText(title);b.setAllCaps(false);b.setTextSize(14);b.setTypeface(Typeface.create("sans-serif-medium",Typeface.NORMAL));
        b.setTextColor(primary?Color.WHITE:ink);b.setBackground(new RippleDrawable(ColorStateList.valueOf(primary?Color.rgb(70,132,114):mint),bg(primary?teal:Color.WHITE,12,primary?null:border),null));
        b.setPadding(dp(16),dp(10),dp(16),dp(10));b.setMinHeight(dp(48));b.setMinimumHeight(dp(48));b.setMinimumWidth(0);
        b.setStateListAnimator(null);b.setOnClickListener(v->action.run());return b;
    }
    private Button addButton(LinearLayout parent,String title,boolean primary,Runnable action){Button b=button(title,primary,action);LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.topMargin=dp(4);p.bottomMargin=dp(12);parent.addView(b,p);return b;}
    private Button addButton(LinearLayout p,String title,Runnable a){return addButton(p,title,false,a);}
    private EditText input(LinearLayout parent,String label,String value,boolean multiline){
        TextView caption=text(parent,label,12,muted,true);caption.setLetterSpacing(.02f);
        EditText e=new EditText(this);e.setText(value);e.setContentDescription(label);e.setTextSize(15);e.setTextColor(ink);e.setHintTextColor(Color.rgb(150,166,167));e.setPadding(dp(14),dp(12),dp(14),dp(12));android.graphics.drawable.StateListDrawable field=new android.graphics.drawable.StateListDrawable();field.addState(new int[]{android.R.attr.state_focused},bg(Color.WHITE,12,teal));field.addState(new int[]{},bg(Color.WHITE,12,border));e.setBackground(field);e.setGravity(Gravity.TOP);e.setSelectAllOnFocus(false);
        e.setInputType(InputType.TYPE_CLASS_TEXT|(multiline?InputType.TYPE_TEXT_FLAG_MULTI_LINE:0));if(multiline)e.setMinLines(4);else e.setSingleLine(true);
        LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(16);parent.addView(e,p);return e;
    }
    private EditText input(LinearLayout p,String label,String value){return input(p,label,value,false);}
    private LinearLayout card(LinearLayout parent){LinearLayout c=column();c.setPadding(dp(22),dp(24),dp(22),dp(22));c.setBackground(bg(Color.WHITE,16,border));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(16);parent.addView(c,p);return c;}
    private TextView chip(String value,int color,int fill){TextView t=new TextView(this);t.setText(value);t.setTextColor(color);t.setTextSize(11);t.setTypeface(null,Typeface.BOLD);t.setPadding(dp(9),dp(5),dp(9),dp(5));t.setBackground(bg(fill,6,null));return t;}
    private void pageHeading(String title,String subtitle){TextView h=text(body,title,27,ink,true);h.setLetterSpacing(-.01f);LinearLayout.LayoutParams p=(LinearLayout.LayoutParams)h.getLayoutParams();p.bottomMargin=dp(14);}
    private void toast(String message){Toast.makeText(this,message,Toast.LENGTH_LONG).show();}
    private <T> void work(Callable<T> action,Consumer<T> done){work(action,done,e->{});}
    private <T> void work(Callable<T> action,Consumer<T> done,Consumer<Exception> failed){
        if(working){toast("正在处理，请稍候");return;}working=true;if(status!=null)status.setText("正在处理，请稍候…");if(busyIndicator!=null)busyIndicator.setVisibility(View.VISIBLE);
        executor.execute(()->{
            try{T result=action.call();runOnUiThread(()->{if(isDestroyed())return;working=false;if(busyIndicator!=null)busyIndicator.setVisibility(View.INVISIBLE);if(status!=null)status.setText(displayName+" · 点击同步");try{done.accept(result);}catch(Exception e){toast(e.getMessage()==null?"界面更新失败":e.getMessage());}});}
            catch(Exception e){runOnUiThread(()->{if(isDestroyed())return;working=false;if(busyIndicator!=null)busyIndicator.setVisibility(View.INVISIBLE);if(status!=null)status.setText("操作未完成 · 可重试");failed.accept(e);if(e instanceof Api.ApiException&&((Api.ApiException)e).code==401){api.token="";tokenStore.save("");showAuth();}toast(e.getMessage()==null?"操作失败，请检查后端连接":e.getMessage());});}
        });
    }
    private LinearLayout scrollRoot(){ScrollView scroll=new ScrollView(this);scroll.setFillViewport(true);LinearLayout root=column();root.setPadding(dp(22),dp(26),dp(22),dp(30));scroll.addView(root);setContentView(scroll);return root;}
    private void showAuth(){searchScopes.clear();scopeTags.clear();completedScopeSummary="";lastSearch=null;searchDraft="";chatDraft="";completedSearchQuery="";searchTags.clear();browseTypes.clear();browseTags.clear();queryFiles.clear();chatFiles.clear();chatMessages.clear();conversationId=null;recallNoteId="";settingsDirty=false;syncVersion="";
        LinearLayout root=scrollRoot();root.setBackgroundColor(canvas);
        LinearLayout brand=row();LinearLayout.LayoutParams brandGap=new LinearLayout.LayoutParams(-1,-2);brandGap.bottomMargin=dp(28);root.addView(brand,brandGap);
        TextView mark=chip("临",teal,mint);mark.setTextSize(21);mark.setGravity(Gravity.CENTER);mark.setPadding(0,0,0,0);brand.addView(mark,new LinearLayout.LayoutParams(dp(46),dp(46)));
        LinearLayout wordmark=column();LinearLayout.LayoutParams wp=new LinearLayout.LayoutParams(-1,-2);wp.leftMargin=dp(12);brand.addView(wordmark,wp);
        TextView name=new TextView(this);name.setText("临记");name.setTextColor(ink);name.setTextSize(19);name.setTypeface(null,Typeface.BOLD);wordmark.addView(name);
        TextView descriptor=new TextView(this);descriptor.setText("临床共享资料库");descriptor.setTextSize(11);descriptor.setTextColor(muted);wordmark.addView(descriptor);
        LinearLayout loginCard=card(root);text(loginCard,"连接共享资料库",16,ink,true);
        EditText address=input(loginCard,"服务地址",api.baseUrl);address.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_URI);
        TextView connection=text(loginCard,"",11,muted,false);connection.setVisibility(View.GONE);LinearLayout form=column();
        addButton(loginCard,"连接资料库",true,()->{
            String url=address.getText().toString().trim().replaceAll("/+$","");if(!url.equals(api.baseUrl)){api.token="";tokenStore.save("");}
            syncVersion="";api.baseUrl=url;getSharedPreferences("settings",MODE_PRIVATE).edit().putString("baseUrl",url).apply();
            work(()->(JSONObject)api.request("/health"),health->{
                boolean initialized=health.optBoolean("initialized");connection.setTextColor(teal);connection.setVisibility(View.VISIBLE);connection.setText(initialized?"已连接":"已连接，请创建账户");form.removeAllViews();
                EditText username=input(form,"用户名","");EditText password=input(form,"密码","");password.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);
                EditText display=initialized?null:input(form,"显示名称","医生");
                addButton(form,initialized?"登录":"创建账户",true,()->{
                    if(!address.getText().toString().trim().replaceAll("/+$","").equals(api.baseUrl)){toast("地址已更改，请重新连接");return;}
                    JSONObject data=obj("username",username.getText().toString().trim(),"password",password.getText().toString(),"display_name",display==null?"医生":display.getText().toString());
                    work(()->(JSONObject)api.request(initialized?"/auth/login":"/auth/setup","POST",data),result->{api.token=str(result,"token");tokenStore.save(api.token);displayName=str(result.optJSONObject("user"),"display_name");password.setText("");showScreen("检索");});
                });
            });
        });loginCard.addView(form);
    }
    private void shell(){
        LinearLayout root=column();root.setBackgroundColor(canvas);
        LinearLayout head=row();head.setPadding(dp(22),dp(14),dp(22),dp(14));head.setBackgroundColor(Color.WHITE);
        TextView mark=chip("临",teal,mint);mark.setBackground(bg(Color.WHITE,11,border));mark.setTextSize(19);mark.setGravity(Gravity.CENTER);mark.setPadding(0,0,0,0);head.addView(mark,new LinearLayout.LayoutParams(dp(38),dp(38)));
        LinearLayout brand=column();LinearLayout.LayoutParams brandParams=new LinearLayout.LayoutParams(0,-2,1);brandParams.leftMargin=dp(11);head.addView(brand,brandParams);
        TextView name=new TextView(this);name.setText("临记");name.setTextSize(17);name.setTypeface(null,Typeface.BOLD);name.setTextColor(ink);brand.addView(name);
        status=new TextView(this);status.setText(displayName+" · 点击同步");status.setTextSize(11);status.setTextColor(muted);status.setContentDescription("同步最新数据");status.setOnClickListener(v->syncNow());brand.addView(status);brand.setOnClickListener(v->syncNow());
        TextView add=chip("＋",Color.WHITE,teal);add.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(teal,13,null),null));add.setTextSize(22);add.setGravity(Gravity.CENTER);add.setPadding(0,0,0,0);add.setContentDescription("对话".equals(screen)?"新建对话":"新建诊疗笔记");add.setOnClickListener(v->{if(working)toast("请等待当前操作完成");else if("对话".equals(screen))resetChat(chatMode);else editNote(null,null);});head.addView(add,new LinearLayout.LayoutParams(dp(40),dp(40)));root.addView(head);
        busyIndicator=new ProgressBar(this,null,android.R.attr.progressBarStyleHorizontal);busyIndicator.setIndeterminate(true);busyIndicator.setIndeterminateTintList(ColorStateList.valueOf(teal));busyIndicator.setVisibility(View.INVISIBLE);root.addView(busyIndicator,new LinearLayout.LayoutParams(-1,dp(2)));
        ScrollView scroll=new ScrollView(this);mainScroll=scroll;scroll.setFillViewport(false);scroll.setClipToPadding(false);scroll.setVerticalScrollBarEnabled(false);
        body=column();body.setPadding(dp(20),dp(16),dp(20),dp(24));scroll.addView(body);screenHost=column();root.addView(screenHost,new LinearLayout.LayoutParams(-1,0,1));screenHost.addView(scroll,new LinearLayout.LayoutParams(-1,-1));
        LinearLayout nav=row();nav.setGravity(Gravity.CENTER);nav.setPadding(dp(8),dp(9),dp(8),dp(9));nav.setBackgroundColor(Color.WHITE);
        for(String item:Arrays.asList("检索","笔记","穴位","材料","对话","设置")){
            boolean active=screen.equals(item);LinearLayout tab=column();tab.setGravity(Gravity.CENTER);tab.setPadding(0,dp(3),0,dp(3));tab.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(Color.WHITE,10,null),null));
            LinearLayout iconPill=row();iconPill.setGravity(Gravity.CENTER);iconPill.setBackground(bg(active?mint:Color.WHITE,11,null));LineIcon icon=new LineIcon(this,item,active?teal:muted);iconPill.addView(icon,new LinearLayout.LayoutParams(dp(22),dp(22)));tab.addView(iconPill,new LinearLayout.LayoutParams(dp(48),dp(30)));
            TextView label=new TextView(this);label.setText("笔记".equals(item)?"学习资料":item);label.setGravity(Gravity.CENTER);label.setTextSize(10.5f);label.setTextColor(active?teal:muted);label.setTypeface(null,active?Typeface.BOLD:Typeface.NORMAL);LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.topMargin=dp(3);tab.addView(label,lp);
            tab.setContentDescription(item);tab.setClickable(true);tab.setFocusable(true);tab.setOnClickListener(v->{if(working)toast("请等待当前操作完成");else navigateScreen(item);});
            LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(0,dp(58),1);p.setMarginStart(dp(2));p.setMarginEnd(dp(2));nav.addView(tab,p);
        }
        root.addView(nav);setContentView(root);
    }
    private void showScreen(String name){if(anatomyWeb!=null){anatomyWeb.destroy();anatomyWeb=null;}searchInputChanged=()->{};searchAttachmentList=null;screen=name;settingsDirty=false;shell();switch(name){case "检索":searchScreen();break;case "笔记":notesScreen();break;case "材料":materialsScreen();break;case "穴位":anatomyScreen();break;case "对话":chatScreen();break;default:settingsScreen();}}
    private JSONArray ids(List<JSONObject> files){List<String> ids=new ArrayList<>();for(JSONObject f:files)ids.add(id(f));return array(ids);}
    private String fileSummary(List<JSONObject> files){List<String> labels=new ArrayList<>();for(JSONObject f:files)labels.add(str(f,"filename")+(str(f,"extracted_text").startsWith("【固定演示")?" · 示例预置文本":("ready".equals(str(f,"status"))?" · 已识别":str(f,"mime").startsWith("image/")?" · 可按指纹检索":" · 待补充文本")));return String.join("\n",labels);}
    private void addSearchFile(JSONObject f,TextView label){if(queryFiles.size()>=5){toast("最多添加 5 份搜索材料");return;}queryFiles.add(f);if(label!=null){label.setText(fileSummary(queryFiles));label.setVisibility(View.VISIBLE);}renderSearchAttachments();searchInputChanged.run();View clear=body.findViewWithTag("clear-search-files");if(clear!=null)clear.setVisibility(View.VISIBLE);if(!"ready".equals(str(f,"status"))&&!str(f,"mime").startsWith("image/"))showAttachment(f);}
    private void cloudSearchControl(LinearLayout parent){cloudSearchControl(parent,false);}
    private void cloudSearchControl(LinearLayout parent,boolean compact){
        android.widget.Switch toggle=new android.widget.Switch(this);toggle.setText(compact?"云端 AI":"云端 AI 辅助");toggle.setContentDescription("云端 AI 辅助");toggle.setTextSize(11);toggle.setSwitchPadding(dp(6));toggle.setTextColor(teal);toggle.setEnabled(false);parent.addView(toggle,new LinearLayout.LayoutParams(-1,dp(42)));
        TextView hint=text(parent,"正在读取检索方式…",10,muted,false);if(compact)hint.setVisibility(View.GONE);boolean[] changing={false};
        executor.execute(()->{try{JSONObject settings=(JSONObject)api.request("/settings");runOnUiThread(()->{if(!toggle.isAttachedToWindow())return;toggle.setChecked(settings.optBoolean("cloud_search_enabled",true));toggle.setTextColor(toggle.isChecked()?teal:muted);if("对话".equals(screen))updateChatModel(settings);toggle.setEnabled(true);hint.setText(toggle.isChecked()?"按配置调用模型":"本地检索");toggle.setOnCheckedChangeListener((button,checked)->{
            if(changing[0])return;toggle.setTextColor(checked?teal:muted);if(working){changing[0]=true;toggle.setChecked(!checked);toggle.setTextColor(toggle.isChecked()?teal:muted);changing[0]=false;toast("请等待当前操作完成");return;}toggle.setEnabled(false);working=true;
            executor.execute(()->{try{api.request("/search-settings","PUT",obj("cloud_search_enabled",checked));runOnUiThread(()->{working=false;toggle.setEnabled(true);put(settings,"cloud_search_enabled",checked);if("对话".equals(screen)){updateChatModel(settings);updateChatSend();}hint.setText(checked?"可调用配置的模型":"本地检索");toast(checked?"云端 AI 搜索已开启":"云端 AI 搜索已关闭");});}catch(Exception e){runOnUiThread(()->{working=false;changing[0]=true;toggle.setChecked(!checked);toggle.setTextColor(toggle.isChecked()?teal:muted);changing[0]=false;toggle.setEnabled(true);toast("保存搜索设置失败，请重试");});}});
        });});}catch(Exception e){runOnUiThread(()->{hint.setText("搜索设置读取失败，请进入设置重试");if(compact){toggle.setText("AI 不可用");toggle.setContentDescription("AI 设置读取失败，请进入设置重试");}});}});
    }

    private JSONObject learningType(JSONObject note){return LearningCatalog.type(learningTypes,note);}

    private String filterURL(List<String> types,List<String> tags){try{StringBuilder out=new StringBuilder();for(String id:types)out.append("&note_types=").append(java.net.URLEncoder.encode(id,"UTF-8"));for(String tag:tags)out.append("&tags=").append(java.net.URLEncoder.encode(tag,"UTF-8"));return out.toString();}catch(Exception e){throw new IllegalArgumentException(e);}}
    private void loadLearningCatalog(){try{applyLearningCatalog((JSONObject)api.request("/taxonomy"));}catch(Exception ignored){}}
    private void applyLearningCatalog(JSONObject value){if(value.optJSONObject("detail_field_specs")!=null)detailFieldSpecs=value.optJSONObject("detail_field_specs");if(value.optJSONObject("detail_primary_fields")!=null)detailPrimaryFields=value.optJSONObject("detail_primary_fields");if(value.optJSONObject("detail_templates")!=null)detailTemplates=value.optJSONObject("detail_templates");if(array(value,"note_types").length()>0)learningTypes=array(value,"note_types");learningTags=array(value,"tags");List<String> ids=new ArrayList<>(),names=new ArrayList<>();for(JSONObject t:objects(learningTypes))ids.add(str(t,"id"));for(JSONObject t:objects(learningTags))names.add(str(t,"name"));searchScopes.retainAll(ids);scopeTags.retainAll(names);searchCategories.retainAll(ids);browseTypes.retainAll(ids);searchTags.retainAll(names);browseTags.retainAll(names);}
    private List<String> typeIDs(JSONObject note){List<String> ids=strings(array(note,"note_types"));if(ids.isEmpty())ids.add(str(learningType(note),"id"));return ids;}
    private String typeNames(JSONObject note){List<String> names=new ArrayList<>();for(String id:typeIDs(note))for(JSONObject t:objects(learningTypes))if(id.equals(str(t,"id")))names.add(str(t,"title"));return String.join(" · ",names);}
    private boolean matchesTypes(JSONObject n,List<String> types){if(types.isEmpty())return true;for(String id:typeIDs(n))if(types.contains(id))return true;return false;}
    private boolean matchesTags(JSONObject n,List<String> tags){return strings(array(n,"tags")).containsAll(tags);}
    private void toggle(List<String> values,String value){if(values.contains(value))values.remove(value);else values.add(value);}
    private void replace(List<String> target,List<String> values){target.clear();target.addAll(values);}
    private void chooseTypes(List<String> selected,Consumer<List<String>> picked){List<JSONObject> items=new ArrayList<>();for(JSONObject t:objects(learningTypes))items.add(obj("id",str(t,"id"),"name",str(t,"title"),"count",t.optInt("count")));multiPicker("资料类型",items,selected,picked,100);}
    private void multiPicker(String title,List<JSONObject> items,List<String> selected,Consumer<List<String>> picked,int max){
        List<String> pending=new ArrayList<>(selected);LinearLayout root=column();root.setPadding(dp(20),dp(4),dp(20),dp(4));
        EditText query=new EditText(this);query.setTextSize(14);query.setTextColor(ink);query.setHintTextColor(muted);query.setHint("查找"+title);query.setContentDescription("查找名称");query.setSingleLine(true);query.setPadding(dp(12),dp(8),dp(12),dp(8));query.setBackground(bg(soft,8,border));root.addView(query,new LinearLayout.LayoutParams(-1,dp(42)));
        TextView summary=text(root,"已选 0 项",11,muted,false);summary.setPadding(0,dp(10),0,0);ScrollView scroll=new ScrollView(this);LinearLayout list=column();scroll.setFillViewport(false);scroll.addView(list);int listHeight=Math.min(dp(300),(int)(getResources().getDisplayMetrics().heightPixels*.38f));root.addView(scroll,new LinearLayout.LayoutParams(-1,listHeight));
        Runnable[] draw=new Runnable[1];AlertDialog[] popup=new AlertDialog[1];Runnable update=()->{summary.setText("已选 "+pending.size()+" 项");if(popup[0]!=null)popup[0].getButton(AlertDialog.BUTTON_NEUTRAL).setEnabled(!pending.isEmpty());};
        draw[0]=()->{list.removeAllViews();String term=query.getText().toString().trim().toLowerCase(java.util.Locale.ROOT);int found=0;
            for(JSONObject item:items){String id=str(item,"id"),name=str(item,"name");if(!name.toLowerCase(java.util.Locale.ROOT).contains(term))continue;found++;LinearLayout line=row();line.setPadding(dp(6),0,dp(10),0);line.setMinimumHeight(dp(46));line.setBackground(bg(pending.contains(id)?mint:Color.WHITE,8,pending.contains(id)?Color.rgb(128,172,159):border));
                CheckBox choice=new CheckBox(this);choice.setText(name);choice.setTextSize(13);choice.setTextColor(ink);choice.setButtonTintList(ColorStateList.valueOf(teal));choice.setChecked(pending.contains(id));choice.setContentDescription(title+"："+name);choice.setMinHeight(dp(44));choice.setPadding(0,dp(5),dp(6),dp(5));line.addView(choice,new LinearLayout.LayoutParams(0,-2,1));TextView count=new TextView(this);count.setText(item.optInt("count")+" 篇");count.setTextSize(10);count.setTextColor(muted);line.addView(count,new LinearLayout.LayoutParams(-2,-2));
                choice.setOnCheckedChangeListener((v,on)->{if(on&&!pending.contains(id)){if(pending.size()>=max){choice.setChecked(false);toast("最多选择 "+max+" 项");return;}pending.add(id);}else if(!on)pending.remove(id);line.setBackground(bg(pending.contains(id)?mint:Color.WHITE,8,pending.contains(id)?Color.rgb(128,172,159):border));update.run();});line.setOnClickListener(v->choice.setChecked(!choice.isChecked()));LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(7);list.addView(line,lp);
            }
            if(found==0)text(list,"没有找到"+title,12,muted,false);scroll.getLayoutParams().height=Math.min(listHeight,Math.max(dp(54),found*dp(54)));scroll.requestLayout();update.run();
        };
        query.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence x,int a,int b,int c){}public void onTextChanged(CharSequence x,int a,int b,int c){draw[0].run();}public void afterTextChanged(Editable e){}});
        AlertDialog dialog=new AlertDialog.Builder(this).setTitle("选择"+title).setView(root).setNeutralButton("清空",null).setNegativeButton("取消",null).setPositiveButton("应用",(d,w)->picked.accept(new ArrayList<>(pending))).create();dialog.setOnShowListener(d->{popup[0]=dialog;dialog.getButton(AlertDialog.BUTTON_NEUTRAL).setOnClickListener(v->{pending.clear();draw[0].run();});draw[0].run();});dialog.show();dialog.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE|WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_HIDDEN);
    }
    // Selected chips wrap onto as many rows as necessary; none are hidden horizontally.
    private void wrapChoices(LinearLayout root,List<String> labels,List<String> keys,List<String> selected,Consumer<String> clicked){
        int width=getResources().getDisplayMetrics().widthPixels-dp(90);LinearLayout line=row();root.addView(line);int used=0;
        for(int i=0;i<labels.size();i++){String key=keys.get(i);Button chip=filterChip((selected.contains(key)?"✓ ":"")+labels.get(i),selected.contains(key),()->clicked.accept(key));chip.setContentDescription(labels.get(i));chip.setSelected(selected.contains(key));chip.setMaxWidth(width);chip.setSingleLine(false);chip.setTextSize(12);chip.measure(View.MeasureSpec.makeMeasureSpec(width,View.MeasureSpec.AT_MOST),View.MeasureSpec.makeMeasureSpec(0,View.MeasureSpec.UNSPECIFIED));int desired=Math.min(width,chip.getMeasuredWidth()+dp(7));if(used>0&&used+desired>width){line=row();root.addView(line);used=0;}LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(Math.min(width,desired),-2);lp.rightMargin=dp(5);lp.bottomMargin=dp(7);line.addView(chip,lp);used+=desired+dp(5);}
    }
    private void selectedFilters(LinearLayout root,List<String> types,List<String> tags,Runnable redraw){
        if(types.isEmpty()&&tags.isEmpty())return;
        SelectedFilterFlow flow=new SelectedFilterFlow(this,dp(6));LinearLayout.LayoutParams params=new LinearLayout.LayoutParams(-1,-2);params.topMargin=dp(10);params.bottomMargin=dp(6);root.addView(flow,params);
        TextView count=new TextView(this);count.setText("已选 "+(types.size()+tags.size()));count.setTextSize(11);count.setTextColor(muted);count.setGravity(Gravity.CENTER_VERTICAL);count.setMinHeight(dp(44));count.setPadding(0,0,dp(6),0);flow.addView(count);
        for(String id:new ArrayList<>(types)){String name=str(learningType(obj("note_type",id)),"title");Button chip=filterChip("类型 · "+name+" ×",true,()->{types.remove(id);redraw.run();});chip.setContentDescription("取消类型："+name);chip.setSingleLine(false);chip.setMinHeight(dp(44));chip.setMinimumHeight(dp(44));flow.addView(chip);}
        for(String tag:new ArrayList<>(tags)){Button chip=filterChip("标签 · "+tag+" ×",true,()->{tags.remove(tag);redraw.run();});chip.setContentDescription("取消标签："+tag);chip.setSingleLine(false);chip.setMinHeight(dp(44));chip.setMinimumHeight(dp(44));flow.addView(chip);}
        Button clear=filterChip("清除筛选",false,()->{types.clear();tags.clear();redraw.run();});clear.setTextColor(teal);clear.setMinHeight(dp(44));clear.setMinimumHeight(dp(44));flow.addView(clear);
    }

    private java.util.Map<String,Integer> tagCountsFor(JSONArray notes){java.util.Map<String,Integer> counts=new java.util.LinkedHashMap<>();if(notes==null){for(JSONObject t:objects(learningTags))counts.put(str(t,"name"),t.optInt("count"));}else for(JSONObject n:objects(notes))for(String tag:new java.util.LinkedHashSet<>(strings(array(n,"tags"))))counts.put(tag,counts.getOrDefault(tag,0)+1);return counts;}
    private Button filterChip(String title,boolean selected,Runnable action){Button b=button(title,false,action);b.setMinWidth(0);b.setTextSize(11);b.setTextColor(selected?teal:muted);b.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(selected?mint:Color.WHITE,9,selected?Color.rgb(177,205,192):null),null));b.setPadding(dp(10),dp(6),dp(10),dp(6));b.setMinHeight(dp(38));b.setMinimumHeight(dp(38));return b;}
    private LinearLayout facetRow(LinearLayout parent,String title){LinearLayout line=row();TextView label=new TextView(this);label.setText(title);label.setTextSize(11);label.setTextColor(muted);line.addView(label,new LinearLayout.LayoutParams(dp(56),dp(40)));label.setGravity(Gravity.CENTER_VERTICAL);HorizontalScrollView scroll=new HorizontalScrollView(this);scroll.setHorizontalScrollBarEnabled(false);LinearLayout choices=row();scroll.addView(choices);line.addView(scroll,new LinearLayout.LayoutParams(0,-2,1));LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.topMargin=dp(8);parent.addView(line,lp);return choices;}
    private void addFilterChoice(LinearLayout row,String title,boolean selected,Runnable action){Button b=filterChip(title,selected,action);LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-2,dp(40));lp.rightMargin=dp(5);row.addView(b,lp);}
    private void chooseTags(JSONArray notes,List<String> selected,Consumer<List<String>> picked){java.util.Map<String,Integer> counts=tagCountsFor(notes);List<JSONObject> items=new ArrayList<>();for(String tag:selected)counts.putIfAbsent(tag,0);for(String name:counts.keySet())items.add(obj("id",name,"name",name,"count",counts.get(name)));items.sort((a,b)->Integer.compare(b.optInt("count"),a.optInt("count")));multiPicker("标签",items,selected,picked,20);}


    private void drawReading(LinearLayout root,JSONObject note,JSONObject evidence,JSONArray terms){
        JSONArray passages=evidence==null?new JSONArray():array(evidence,"passages");
        if(passages.length()==0){String body=str(note,"content");passages=array(Arrays.asList(obj("field","content","source","笔记正文","text",body.length()<=800?body:cut(body,480)+"…","matched",false,"truncated",body.length()>800)));}
        int shown=0;boolean full=false;boolean lateStep=false;JSONArray sourceSteps=array(note,"steps");for(int i=6;i<sourceSteps.length();i++)for(String term:strings(terms))if(sourceSteps.optString(i).toLowerCase(java.util.Locale.ROOT).contains(term.toLowerCase(java.util.Locale.ROOT)))lateStep=true;
        for(JSONObject passage:objects(passages)){if(shown>=3)break;if("steps".equals(str(passage,"field"))&&shown>0&&!lateStep)continue;shown++;
            String source=str(passage,"source");TextView label=text(root,(passage.optBoolean("matched")?"命中原文":"笔记原文")+(source.isEmpty()||"笔记正文".equals(source)?"":" · "+source),12,teal,true);TextView content=text(root,str(passage,"text"),16,ink,false);content.setTextIsSelectable(true);content.setLineSpacing(dp(7),1);highlightMatches(content,terms);
            if("content".equals(str(passage,"field"))&&!passage.optBoolean("truncated"))full=true;
            if(!str(passage,"attachment_id").isEmpty()){String target=str(passage,"attachment_id");for(JSONObject f:objects(array(note,"attachments")))if(target.equals(id(f)))addButton(root,"核对命中的原件",()->showAttachment(f));}
        }
        if(!full&&!str(note,"content").isEmpty()){TextView body=text(root,str(note,"content"),16,ink,false);body.setTextIsSelectable(true);highlightMatches(body,terms);body.setVisibility(View.GONE);Button toggle=button("展开笔记全文",false,()->{});toggle.setTextSize(12);toggle.setOnClickListener(v->{boolean open=body.getVisibility()==View.GONE;body.setVisibility(open?View.VISIBLE:View.GONE);toggle.setText(open?"收起笔记全文":"展开笔记全文");});root.addView(toggle,new LinearLayout.LayoutParams(-1,dp(42)));}
    }
    private String scopeDescription(){List<String> parts=new ArrayList<>(),names=new ArrayList<>();for(String id:searchScopes)names.add(str(learningType(obj("note_type",id)),"title"));if(!names.isEmpty())parts.add("类型："+String.join("、",names));if(!scopeTags.isEmpty())parts.add("标签："+String.join("、",scopeTags));return String.join("；",parts);}
    private LinearLayout searchTool(String icon,String label,String description,Runnable action){
        LinearLayout control=row();control.setGravity(Gravity.CENTER);control.setPadding(dp(6),0,dp(6),0);control.setMinimumHeight(dp(44));control.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(Color.TRANSPARENT,8,null),null));LineIcon glyph=new LineIcon(this,icon,muted);control.addView(glyph,new LinearLayout.LayoutParams(dp(19),dp(19)));if(!label.isEmpty()){TextView title=new TextView(this);title.setText(label);title.setTextSize(12);title.setTextColor(muted);LinearLayout.LayoutParams tp=new LinearLayout.LayoutParams(-2,-2);tp.leftMargin=dp(6);control.addView(title,tp);}control.setContentDescription(description);control.setClickable(true);control.setFocusable(true);control.setOnClickListener(v->{if(working)toast("请等待当前操作完成");else action.run();});return control;
    }
    private Button searchLink(String label,Runnable action){Button link=filterChip(label,false,action);link.setTextColor(teal);link.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(Color.TRANSPARENT,7,null),null));link.setMinHeight(dp(44));link.setMinimumHeight(dp(44));return link;}
    private void renderSearchAttachments(){
        if(searchAttachmentList==null||!"检索".equals(screen))return;searchAttachmentList.removeAllViews();searchAttachmentList.setVisibility(queryFiles.isEmpty()?View.GONE:View.VISIBLE);
        for(JSONObject file:new ArrayList<>(queryFiles)){
            LinearLayout line=row();line.setPadding(dp(8),dp(2),0,dp(2));line.setBackground(bg(soft,8,border));LineIcon icon=new LineIcon(this,str(file,"mime").startsWith("image/")?"图片":str(file,"mime").startsWith("audio/")?"录音":"附件",muted);line.addView(icon,new LinearLayout.LayoutParams(dp(18),dp(18)));
            TextView name=new TextView(this);name.setText(str(file,"filename"));name.setTextColor(ink);name.setTextSize(12);name.setSingleLine(true);name.setEllipsize(android.text.TextUtils.TruncateAt.MIDDLE);name.setGravity(Gravity.CENTER_VERTICAL);name.setMinHeight(dp(44));name.setPadding(dp(8),0,0,0);name.setContentDescription("查看材料："+str(file,"filename"));name.setOnClickListener(v->{if(!working)showAttachment(file);});line.addView(name,new LinearLayout.LayoutParams(0,-2,1));line.addView(searchTool("关闭","","移除材料："+str(file,"filename"),()->{queryFiles.remove(file);renderSearchAttachments();searchInputChanged.run();}),new LinearLayout.LayoutParams(dp(44),dp(44)));LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(7);searchAttachmentList.addView(line,lp);
        }
    }
    private void searchScreen(){
        body.setPadding(dp(20),dp(8),dp(20),dp(20));TextView heading=text(body,"经验检索",21,ink,true);((LinearLayout.LayoutParams)heading.getLayoutParams()).bottomMargin=dp(8);
        LinearLayout intake = card(body);
        intake.setPadding(dp(15),dp(13),dp(15),dp(10));
        intake.setBackground(bg(Color.rgb(237,246,241),14,Color.rgb(197,221,211)));
        text(intake,"从材料生成笔记",17,ink,true);
        text(intake,"一次导入多份材料，整理成可核对的草稿。",12,muted,false);
        LinearLayout intakeActions = row();
        intake.addView(intakeActions);
        Button importAction = button("导入材料并整理",true,()->openOrganizer("pick"));
        Button demoAction = button("试用示例",false,()->openOrganizer("demo"));
        LinearLayout.LayoutParams importWidth = new LinearLayout.LayoutParams(0,dp(44),1);
        importWidth.rightMargin = dp(8);
        intakeActions.addView(importAction,importWidth);
        intakeActions.addView(demoAction,new LinearLayout.LayoutParams(-2,dp(44)));
        LinearLayout search=card(body);search.setPadding(dp(16),dp(12),dp(16),dp(10));search.setBackground(bg(Color.WHITE,14,border));
        LinearLayout entry=row();entry.setGravity(Gravity.TOP);LineIcon magnifier=new LineIcon(this,"检索",Color.rgb(124,147,143));LinearLayout.LayoutParams ip=new LinearLayout.LayoutParams(dp(21),dp(21));ip.topMargin=dp(7);ip.rightMargin=dp(10);entry.addView(magnifier,ip);
        EditText query=new EditText(this);query.setTextSize(16);query.setTextColor(ink);query.setHintTextColor(Color.rgb(139,155,158));query.setHint("搜索笔记或关键词");query.setText(searchDraft);query.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_FLAG_MULTI_LINE);query.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(2000)});query.setGravity(Gravity.TOP);query.setMinLines(2);query.setMaxLines(5);query.setMinHeight(dp(60));query.setPadding(0,dp(3),0,dp(10));query.setBackgroundColor(Color.TRANSPARENT);query.setTag("search-query");query.setContentDescription("检索内容");entry.addView(query,new LinearLayout.LayoutParams(0,-2,1));search.addView(entry);
        searchAttachmentList=column();search.addView(searchAttachmentList);renderSearchAttachments();
        LinearLayout actions=row();actions.addView(searchTool("图片","材料","添加图片、音频或文档",()->{if(queryFiles.size()>=5){toast("最多添加 5 份材料");return;}pickingSearchMaterial=true;pickFile(file->addSearchFile(file,null));}),new LinearLayout.LayoutParams(dp(74),dp(44)));actions.addView(searchTool("录音","录音","录音输入",()->{if(queryFiles.size()>=5){toast("最多添加 5 份材料");return;}record(file->addSearchFile(file,null));}),new LinearLayout.LayoutParams(dp(74),dp(44)));actions.addView(new View(this),new LinearLayout.LayoutParams(0,1,1));Button submit=button("检索",true,()->{});submit.setTextSize(14);submit.setPadding(dp(12),0,dp(12),0);submit.setMinHeight(dp(44));submit.setMinimumHeight(dp(44));submit.setContentDescription("检索记录");actions.addView(submit,new LinearLayout.LayoutParams(dp(88),dp(44)));search.addView(actions);
        TextView error=text(search,"",12,Color.rgb(164,80,67),false);error.setVisibility(View.GONE);error.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        SelectedFilterFlow range=new SelectedFilterFlow(this,dp(6));LinearLayout.LayoutParams rp=new LinearLayout.LayoutParams(-1,-2);rp.topMargin=dp(-6);rp.bottomMargin=dp(4);body.addView(range,rp);Button typeScope=filterChip("全部类型 ⌄",false,()->{}),tagScope=filterChip("全部标签 ⌄",false,()->{});typeScope.setContentDescription("选择检索资料类型");tagScope.setContentDescription("选择检索标签");for(Button control:Arrays.asList(typeScope,tagScope)){control.setMinHeight(dp(44));control.setMinimumHeight(dp(44));control.setTextSize(11);control.setPadding(dp(8),0,dp(8),0);range.addView(control);}LinearLayout cloud=column();cloudSearchControl(cloud,true);range.addView(cloud);
        LinearLayout scopeSummary=column();body.addView(scopeSummary);Runnable[] updateScopes=new Runnable[1];updateScopes[0]=()->{typeScope.setText(searchScopes.isEmpty()?"全部类型 ⌄":"类型 "+searchScopes.size()+" ⌄");tagScope.setText(scopeTags.isEmpty()?"全部标签 ⌄":"标签 "+scopeTags.size()+" ⌄");scopeSummary.removeAllViews();selectedFilters(scopeSummary,searchScopes,scopeTags,()->updateScopes[0].run());searchInputChanged.run();};
        typeScope.setOnClickListener(v->{if(!working)chooseTypes(searchScopes,types->{replace(searchScopes,types);updateScopes[0].run();});});tagScope.setOnClickListener(v->{if(!working)chooseTags(null,scopeTags,tags->{replace(scopeTags,tags);updateScopes[0].run();});});
        searchInputChanged=()->{boolean ready=!query.getText().toString().trim().isEmpty()||!queryFiles.isEmpty()||!searchScopes.isEmpty()||!scopeTags.isEmpty();submit.setEnabled(ready&&!working);submit.setAlpha(ready?1f:.45f);};
        query.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence x,int st,int count,int after){}public void onTextChanged(CharSequence x,int st,int before,int count){searchDraft=x.toString();error.setVisibility(View.GONE);searchInputChanged.run();}public void afterTextChanged(Editable e){}});updateScopes[0].run();
        LinearLayout results=column();results.setPadding(0,dp(12),0,0);body.addView(results);
        submit.setOnClickListener(v->{if(working)return;String value=query.getText().toString().trim();if(value.isEmpty()&&queryFiles.isEmpty()&&searchScopes.isEmpty()&&scopeTags.isEmpty())return;error.setVisibility(View.GONE);android.view.inputmethod.InputMethodManager keyboard=(android.view.inputmethod.InputMethodManager)getSystemService(INPUT_METHOD_SERVICE);if(keyboard!=null)keyboard.hideSoftInputFromWindow(query.getWindowToken(),0);query.clearFocus();query.setEnabled(false);submit.setEnabled(false);submit.setText("查找中");final String submittedScope=scopeDescription(),inputLabel=!value.isEmpty()?value:queryFiles.isEmpty()?"按条件检索":queryFiles.size()==1?str(queryFiles.get(0),"filename"):queryFiles.size()+" 份材料";final boolean hadFiles=!queryFiles.isEmpty();JSONObject payload=obj("query",value,"attachment_ids",ids(queryFiles),"note_types",array(searchScopes),"tags",array(scopeTags),"limit",100);work(()->(JSONObject)api.request("/search","POST",payload),data->{lastSearch=data;completedSearchQuery=value;completedInputLabel=inputLabel;completedSearchHadFiles=hadFiles;completedScopeSummary=submittedScope;searchCategories.clear();searchTags.clear();dateOrder=false;query.setEnabled(true);submit.setText("检索");searchInputChanged.run();searchResults(results,data,value);results.post(()->mainScroll.smoothScrollTo(0,Math.max(0,results.getTop())));},failure->{query.setEnabled(true);submit.setText("重试");searchInputChanged.run();error.setText("检索失败："+(failure.getMessage()==null?"请重试":failure.getMessage()));error.setVisibility(View.VISIBLE);});});
        if(lastSearch!=null){searchResults(results,lastSearch,completedSearchQuery);}else{LinearLayout recentHead=row();TextView title=new TextView(this);title.setText("最近笔记");title.setGravity(Gravity.CENTER_VERTICAL);title.setTextSize(16);title.setTextColor(ink);title.setTypeface(null,Typeface.BOLD);recentHead.addView(title,new LinearLayout.LayoutParams(0,dp(44),1));recentHead.addView(searchLink("全部笔记 →",()->navigateScreen("笔记")));results.addView(recentHead);work(()->{loadLearningCatalog();return (JSONObject)api.request("/notes?limit=5");},data->{updateScopes[0].run();drawRecentNotes(results,array(data,"items"));});searchInputChanged.run();}
    }
    private void drawRecentNotes(LinearLayout parent,JSONArray notes){
        if(notes.length()==0){text(parent,"还没有笔记",14,muted,false);addButton(parent,"新建笔记",()->editNote(null,null));return;}
        LinearLayout list=column();list.setBackground(bg(Color.WHITE,12,border));parent.addView(list,new LinearLayout.LayoutParams(-1,-2));int index=0;
        for(JSONObject note:objects(notes)){if(index++>0)divider(list,0,0);LinearLayout item=column();item.setPadding(dp(15),dp(13),dp(15),dp(12));LinearLayout meta=row();TextView kind=new TextView(this);kind.setText(typeNames(note));kind.setSingleLine(true);kind.setEllipsize(android.text.TextUtils.TruncateAt.END);kind.setTextSize(10);kind.setTextColor(muted);meta.addView(kind,new LinearLayout.LayoutParams(0,-2,1));TextView date=new TextView(this);date.setText(str(note,"occurred_on"));date.setTextSize(10);date.setTextColor(muted);meta.addView(date);item.addView(meta);TextView title=text(item,str(note,"title"),16,ink,true);((LinearLayout.LayoutParams)title.getLayoutParams()).topMargin=dp(7);((LinearLayout.LayoutParams)title.getLayoutParams()).bottomMargin=dp(4);TextView excerpt=text(item,str(note,"content"),12,muted,false);excerpt.setMaxLines(2);excerpt.setEllipsize(android.text.TextUtils.TruncateAt.END);((LinearLayout.LayoutParams)excerpt.getLayoutParams()).bottomMargin=0;item.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(Color.TRANSPARENT,10,null),null));item.setClickable(true);item.setFocusable(true);item.setContentDescription("打开笔记："+str(note,"title"));item.setOnClickListener(v->{if(!working)showNote(id(note));});list.addView(item,new LinearLayout.LayoutParams(-1,-2));}
    }
    private void editSearchQuery(){mainScroll.smoothScrollTo(0,0);EditText q=body.findViewWithTag("search-query");if(q!=null){q.requestFocus();q.setSelection(q.length());android.view.inputmethod.InputMethodManager keyboard=(android.view.inputmethod.InputMethodManager)getSystemService(INPUT_METHOD_SERVICE);if(keyboard!=null)q.post(()->keyboard.showSoftInput(q,android.view.inputmethod.InputMethodManager.SHOW_IMPLICIT));}}
    private void showResultFilters(LinearLayout parent,JSONObject result,String originalQuery){
        List<String> pendingTypes=new ArrayList<>(searchCategories),pendingTags=new ArrayList<>(searchTags);Panel p=panel("筛选结果");if(p.dialog.getWindow()!=null)p.dialog.getWindow().setLayout(getResources().getDisplayMetrics().widthPixels-dp(24),Math.min(dp(500),(int)(getResources().getDisplayMetrics().heightPixels*.8)));Runnable[] draw=new Runnable[1];draw[0]=()->{p.root.removeAllViews();text(p.root,"资料类型 · 匹配任一种",12,muted,false);addButton(p.root,pendingTypes.isEmpty()?"全部类型":"已选 "+pendingTypes.size()+" 个类型",()->chooseTypes(pendingTypes,types->{replace(pendingTypes,types);draw[0].run();}));text(p.root,"标签 · 同时满足",12,muted,false);addButton(p.root,pendingTags.isEmpty()?"全部标签":"已选 "+pendingTags.size()+" 个标签",()->chooseTags(array(result,"items"),pendingTags,tags->{replace(pendingTags,tags);draw[0].run();}));selectedFilters(p.root,pendingTypes,pendingTags,()->draw[0].run());int total=0;for(JSONObject note:objects(array(result,"items")))if(matchesTypes(note,pendingTypes)&&matchesTags(note,pendingTags))total++;LinearLayout actions=row();Button reset=searchLink("重置",()->{pendingTypes.clear();pendingTags.clear();draw[0].run();});actions.addView(reset,new LinearLayout.LayoutParams(-2,dp(44)));Button apply=button("显示 "+total+" 篇结果",true,()->{replace(searchCategories,pendingTypes);replace(searchTags,pendingTags);p.dialog.dismiss();searchResults(parent,result,originalQuery);});apply.setTextSize(13);LinearLayout.LayoutParams ap=new LinearLayout.LayoutParams(0,dp(44),1);ap.leftMargin=dp(12);actions.addView(apply,ap);LinearLayout.LayoutParams footer=new LinearLayout.LayoutParams(-1,-2);footer.topMargin=dp(18);p.root.addView(actions,footer);};draw[0].run();
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
    private void highlightMatches(TextView view,JSONArray terms){
        String value=view.getText().toString();android.text.SpannableString marked=new android.text.SpannableString(value);
        for(String term:strings(terms)){if(term.length()<2||term.length()>80)continue;for(int at=0;at+term.length()<=value.length();at++){if(value.regionMatches(true,at,term,0,term.length()))marked.setSpan(new android.text.style.BackgroundColorSpan(Color.rgb(231,240,215)),at,at+term.length(),android.text.Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);}}
        view.setText(marked);
    }
    private void loadResultThumbnail(android.widget.ImageView image,JSONObject attachment){
        image.post(()->{if(isDestroyed()||executor.isShutdown())return;executor.execute(()->{try{if(!image.isAttachedToWindow())return;byte[] data=api.download("/attachments/"+id(attachment)+"/thumbnail");android.graphics.Bitmap bitmap=android.graphics.BitmapFactory.decodeByteArray(data,0,data.length);runOnUiThread(()->{if(!isDestroyed()&&image.isAttachedToWindow()){if(bitmap!=null)image.setImageBitmap(bitmap);else image.setVisibility(View.GONE);}});}catch(Exception ignored){runOnUiThread(()->{if(!isDestroyed())image.setVisibility(View.GONE);});}});});
    }
    private String matchDescription(JSONObject evidence){List<String> lines=new ArrayList<>();for(JSONObject match:objects(array(evidence,"image_matches"))){String method=str(match,"method");lines.add("exact".equals(method)?"相同图片":"visual".equals(method)?"原图视觉比对":"dino".equals(method)?"DINOv2 本地相似候选":"近似指纹，未完成视觉验证");if(!str(match,"reason").isEmpty())lines.add(str(match,"reason"));List<String> differences=strings(array(match,"differences"));if(!differences.isEmpty())lines.add("差异："+String.join("；",differences));}List<String> labels=new ArrayList<>();for(String field:strings(array(evidence,"fields"))){switch(field){case "title":labels.add("标题");break;case "tags":labels.add("标签");break;case "content":labels.add("正文");break;case "steps":labels.add("记录条目");break;case "attachment":labels.add("材料文字");break;case "image":labels.add("图片");break;}}if(!labels.isEmpty())lines.add("命中位置："+String.join(" · ",labels));return lines.isEmpty()?"相关内容，请核对原文":String.join("\n",lines);}
    private void searchResults(LinearLayout parent,JSONObject result,String originalQuery){
        final int filterScrollY=mainScroll.getScrollY();
        parent.removeAllViews();JSONArray all=array(result,"items");List<JSONObject> notes=new ArrayList<>(),typed=new ArrayList<>();for(JSONObject n:objects(all)){if(matchesTypes(n,searchCategories))typed.add(n);if(matchesTypes(n,searchCategories)&&matchesTags(n,searchTags))notes.add(n);}if(dateOrder)notes.sort((x,y)->str(y,"occurred_on").compareTo(str(x,"occurred_on")));
        LinearLayout filters=column();LinearLayout resultHead=row();TextView resultHeading=new TextView(this);resultHeading.setText("检索结果");resultHeading.setTextSize(18);resultHeading.setTextColor(ink);resultHeading.setTypeface(null,Typeface.BOLD);resultHead.addView(resultHeading);TextView count=new TextView(this);count.setText("  "+notes.size()+(searchCategories.isEmpty()&&searchTags.isEmpty()?"":" / "+all.length())+" 篇");count.setTextSize(12);count.setTextColor(muted);count.setGravity(Gravity.CENTER_VERTICAL);resultHead.addView(count,new LinearLayout.LayoutParams(0,dp(44),1));int active=searchCategories.size()+searchTags.size();resultHead.addView(searchTool("筛选",active==0?"筛选":"筛选 "+active,"筛选结果",()->showResultFilters(parent,result,originalQuery)),new LinearLayout.LayoutParams(-2,dp(44)));filters.addView(resultHead);
        TextView submitted=text(filters,completedInputLabel.isEmpty()?originalQuery:completedInputLabel,12,muted,false);submitted.setMaxLines(2);submitted.setEllipsize(android.text.TextUtils.TruncateAt.END);if(!completedScopeSummary.isEmpty())text(filters,completedScopeSummary,11,muted,false);selectedFilters(filters,searchCategories,searchTags,()->searchResults(parent,result,originalQuery));
        LinearLayout actions=row();Button sort=searchLink(dateOrder?"最新记录 ↓":"按相关度",()->{dateOrder=!dateOrder;searchResults(parent,result,originalQuery);});sort.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);actions.addView(sort,new LinearLayout.LayoutParams(0,dp(44),1));actions.addView(searchLink("修改查询",this::editSearchQuery));LinearLayout.LayoutParams tools=new LinearLayout.LayoutParams(-1,-2);tools.bottomMargin=dp(10);filters.addView(actions,tools);parent.addView(filters);
        for(String warning:strings(array(result,"warnings")))text(parent,warning,11,Color.rgb(141,112,68),false);
        if(notes.isEmpty()){LinearLayout empty=card(parent);text(empty,"当前没有匹配资料",17,ink,true);text(empty,"调整类型或标签，或修改查询。",13,muted,false);if(!searchCategories.isEmpty()||!searchTags.isEmpty())addButton(empty,"清除筛选",()->{searchCategories.clear();searchTags.clear();searchResults(parent,result,originalQuery);});}
        for(JSONObject note:notes){JSONObject evidence=note.optJSONObject("search_match"),type=learningType(note);JSONArray terms=evidence==null?new JSONArray():array(evidence,"terms");LinearLayout c=card(parent);text(c,typeNames(note)+"  ·  "+str(note,"occurred_on"),11,teal,false);TextView title=text(c,str(note,"title"),22,ink,true);highlightMatches(title,terms);title.setOnClickListener(v->showNote(id(note)));
            drawReading(c,note,evidence,terms);
            JSONArray steps=array(note,"steps");if(steps.length()>0){sectionLabel(c,str(type,"steps_label"),steps.length()+" 项");for(int i=0;i<Math.min(6,steps.length());i++)stepRow(c,steps.optString(i),i+1,terms);if(steps.length()>6){LinearLayout remaining=column();for(int i=6;i<steps.length();i++)stepRow(remaining,steps.optString(i),i+1,terms);remaining.setVisibility(View.GONE);Button more=button("展开其余 "+(steps.length()-6)+" 项",false,()->{});more.setTextSize(11);more.setOnClickListener(v->{boolean open=remaining.getVisibility()==View.GONE;remaining.setVisibility(open?View.VISIBLE:View.GONE);more.setText(open?"收起其余条目":"展开其余 "+(steps.length()-6)+" 项");});c.addView(more,new LinearLayout.LayoutParams(-1,dp(40)));c.addView(remaining);}}
            JSONArray files=array(note,"attachments");if(evidence!=null){List<JSONObject> sorted=objects(files);List<String> matched=strings(array(evidence,"attachment_ids"));sorted.sort((a,b)->Boolean.compare(matched.contains(id(b)),matched.contains(id(a))));files=array(sorted);}if(files.length()>0){divider(c,14,0);sectionLabel(c,"原始材料",files.length()+" 份");for(int i=0;i<Math.min(2,files.length());i++){JSONObject f=files.optJSONObject(i);if(str(f,"mime").startsWith("image/")){android.widget.ImageView image=new android.widget.ImageView(this);image.setScaleType(android.widget.ImageView.ScaleType.FIT_CENTER);image.setBackground(bg(soft,8,border));image.setContentDescription("查看图片："+str(f,"filename"));c.addView(image,new LinearLayout.LayoutParams(-1,dp(110)));image.setOnClickListener(v->showAttachment(f));loadResultThumbnail(image,f);}Button open=addButton(c,str(f,"filename"),()->showAttachment(f));open.setTextSize(12);}}
            if(array(note,"tags").length()>0){HorizontalScrollView tagStrip=new HorizontalScrollView(this);tagStrip.setHorizontalScrollBarEnabled(false);LinearLayout chips=row();List<String> values=strings(array(note,"tags"));for(String tag:values.subList(0,Math.min(3,values.size())))addFilterChoice(chips,tag,searchTags.contains(tag),()->{toggle(searchTags,tag);searchResults(parent,result,originalQuery);});if(values.size()>3){Button more=filterChip("+"+(values.size()-3),false,()->chooseTags(array(java.util.Collections.singletonList(note)),searchTags,selected->{replace(searchTags,selected);searchResults(parent,result,originalQuery);}));more.setContentDescription("查看这篇笔记的全部标签");chips.addView(more,new LinearLayout.LayoutParams(-2,dp(40)));}tagStrip.addView(chips);c.addView(tagStrip,new LinearLayout.LayoutParams(-1,-2));}Button openNote=searchLink("查看完整笔记 →",()->showNote(id(note)));LinearLayout.LayoutParams openParams=new LinearLayout.LayoutParams(-2,dp(44));openParams.gravity=Gravity.END;c.addView(openNote,openParams);
            if(evidence!=null){LinearLayout detail=column();text(detail,matchDescription(evidence),10,muted,false);detail.setVisibility(View.GONE);Button why=button("匹配依据",false,()->detail.setVisibility(detail.getVisibility()==View.GONE?View.VISIBLE:View.GONE));why.setTextSize(10);why.setBackgroundColor(Color.TRANSPARENT);c.addView(why,new LinearLayout.LayoutParams(-1,dp(34)));c.addView(detail);}
        }
        if(completedSearchHadFiles){LinearLayout transcript=column();TextView t=text(transcript,str(result,"query"),12,muted,false);t.setTextIsSelectable(true);transcript.setVisibility(View.GONE);addButton(parent,"核对查询材料的识别文字",()->transcript.setVisibility(transcript.getVisibility()==View.GONE?View.VISIBLE:View.GONE));parent.addView(transcript);}
        parent.addView(searchLink("结束检索",()->{lastSearch=null;searchDraft="";completedInputLabel="";completedSearchHadFiles=false;searchCategories.clear();searchTags.clear();queryFiles.clear();showScreen("检索");}),new LinearLayout.LayoutParams(-2,dp(44)));
        parent.post(()->mainScroll.scrollTo(0,filterScrollY));
    }
    private void listNotes(LinearLayout parent,JSONArray notes){
        if(notes.length()==0){LinearLayout empty=card(parent);text(empty,"还没有找到相关记录",15,ink,true);text(empty,"试试其他关键词，或先保存一篇笔记。",12,muted,false);return;}
        for(JSONObject note:objects(notes)){
            LinearLayout c=card(parent);LinearLayout top=row();TextView tag=chip(typeNames(note),muted,Color.WHITE);tag.setPadding(0,0,0,0);top.addView(tag,new LinearLayout.LayoutParams(0,-2,1));TextView date=new TextView(this);date.setText(str(note,"occurred_on"));date.setTextColor(muted);date.setTextSize(11);date.setGravity(Gravity.END|Gravity.CENTER_VERTICAL);top.addView(date,new LinearLayout.LayoutParams(-2,-2));LinearLayout.LayoutParams meta=new LinearLayout.LayoutParams(-1,-2);meta.bottomMargin=dp(10);c.addView(top,meta);
            text(c,str(note,"title"),18,ink,true);String content=str(note,"content");TextView excerpt=text(c,content.isEmpty()?"打开查看治疗步骤与材料":cut(content,120),13,muted,false);excerpt.setMaxLines(2);excerpt.setEllipsize(android.text.TextUtils.TruncateAt.END);
            View divider=new View(this);divider.setBackgroundColor(border);LinearLayout.LayoutParams line=new LinearLayout.LayoutParams(-1,dp(1));line.topMargin=dp(3);line.bottomMargin=dp(12);c.addView(divider,line);
            TextView foot=text(c,array(note,"steps").length()+" 个步骤  ·  "+array(note,"attachments").length()+" 份材料",11,muted,false);((LinearLayout.LayoutParams)foot.getLayoutParams()).bottomMargin=0;
            c.setFocusable(true);c.setClickable(true);c.setContentDescription("打开笔记："+str(note,"title"));c.setOnClickListener(v->showNote(id(note)));
        }
    }
    private void anatomyScreen(){
        android.webkit.WebView web=new android.webkit.WebView(this);anatomyWeb=web;
        web.getSettings().setJavaScriptEnabled(true);web.getSettings().setDomStorageEnabled(true);
        web.getSettings().setAllowFileAccess(false);web.getSettings().setAllowContentAccess(false);
        web.getSettings().setMixedContentMode(android.webkit.WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        String origin=api.baseUrl.replaceAll("/+$", "");android.net.Uri allowed=android.net.Uri.parse(origin);
        web.setWebChromeClient(new android.webkit.WebChromeClient(){@Override public boolean onJsConfirm(android.webkit.WebView v,String url,String message,android.webkit.JsResult result){new AlertDialog.Builder(MainActivity.this).setMessage(message).setPositiveButton("确定",(d,w)->result.confirm()).setNegativeButton("取消",(d,w)->result.cancel()).setOnCancelListener(d->result.cancel()).show();return true;}});
        web.setWebViewClient(new android.webkit.WebViewClient(){
            @Override public boolean shouldOverrideUrlLoading(android.webkit.WebView v,android.webkit.WebResourceRequest req){android.net.Uri uri=req.getUrl();if(!req.isForMainFrame()&&"https".equals(uri.getScheme())&&"sketchfab.com".equals(uri.getHost()))return false;if(req.isForMainFrame()&&req.hasGesture()&&"https".equals(uri.getScheme())&&"dbarchive.biosciencedbc.jp".equals(uri.getHost())&&("/en/bodyparts3d/desc.html".equals(uri.getPath())||"/en/bodyparts3d/lic.html".equals(uri.getPath()))){
                try{startActivity(new Intent(Intent.ACTION_VIEW,uri));}
                catch(android.content.ActivityNotFoundException e){toast("未找到可打开来源的浏览器");}
                return true;
            }if(!allowed.getScheme().equals(uri.getScheme())||!allowed.getAuthority().equals(uri.getAuthority()))return true;String q=uri.getQueryParameter("anatomy_q");if(q!=null){searchDraft=q;showScreen("检索");return true;}return !(uri.getPath().equals(allowed.getPath()+"/anatomy/index.html")||uri.getPath().equals(allowed.getPath()+"/anatomy/"));}
        });
        android.webkit.CookieManager cookies=android.webkit.CookieManager.getInstance();cookies.setAcceptCookie(true);cookies.setAcceptThirdPartyCookies(web,false);
        String cookie="linji_session="+api.token+"; Path="+(allowed.getPath().isEmpty()?"/":allowed.getPath()+"/")+"; HttpOnly; SameSite=Strict"+("https".equals(allowed.getScheme())?"; Secure":"");
        screenHost.removeAllViews();screenHost.addView(web,new LinearLayout.LayoutParams(-1,-1));
        cookies.setCookie(origin+"/",cookie,ok->{cookies.flush();web.loadUrl(origin+"/anatomy/");});
    }
    private void notesScreen(){
        body.setPadding(dp(20),dp(12),dp(20),dp(24));
        pageHeading("学习资料","按用途整理笔记，以标签连接同一主题。");
        work(()->{loadLearningCatalog();return (JSONObject)api.request("/notes?limit=100"+filterURL(browseTypes,browseTags));},data->{
            LinearLayout controls=row();Button type=button("资料类型："+(browseTypes.isEmpty()?"全部":browseTypes.size()+" 项"),false,()->chooseTypes(browseTypes,selected->{replace(browseTypes,selected);showScreen("笔记");}));type.setTextSize(12);controls.addView(type,new LinearLayout.LayoutParams(0,dp(44),1));Button tag=button("标签："+(browseTags.isEmpty()?"全部":browseTags.size()+" 项"),false,()->chooseTags(null,browseTags,selected->{replace(browseTags,selected);showScreen("笔记");}));tag.setTextSize(12);controls.addView(tag,new LinearLayout.LayoutParams(0,dp(44),1));body.addView(controls);selectedFilters(body,browseTypes,browseTags,()->showScreen("笔记"));addButton(body,"管理分类与标签",this::showTaxonomyManager);sectionLabel(body,"资料列表",data.optInt("total")+" 篇");LinearLayout results=column();body.addView(results);listNotes(results,array(data,"items"));int[] offset={array(data,"items").length()};Button more=button("加载更多资料",false,()->{});more.setVisibility(offset[0]<data.optInt("total")?View.VISIBLE:View.GONE);more.setOnClickListener(v->work(()->(JSONObject)api.request("/notes?offset="+offset[0]+filterURL(browseTypes,browseTags)),next->{JSONArray items=array(next,"items");listNotes(results,items);offset[0]+=items.length();more.setVisibility(offset[0]<next.optInt("total")?View.VISIBLE:View.GONE);}));body.addView(more);
            if(data.optInt("total")==0)addButton(results,"新建学习资料",true,()->editNote(null,null));
        });
    }
    private final class GuardedDialog extends Dialog {
        BooleanSupplier dirty=()->false;
        GuardedDialog(){super(MainActivity.this);setCanceledOnTouchOutside(false);}
        void requestClose(){if(working){toast("请等待当前操作完成");return;}if(dirty.getAsBoolean())new AlertDialog.Builder(MainActivity.this).setTitle("放弃未保存的修改？").setMessage("当前输入尚未保存，可以返回继续编辑。").setNegativeButton("继续编辑",null).setPositiveButton("放弃修改",(d,w)->super.dismiss()).show();else super.dismiss();}
        @Override public void cancel(){requestClose();}
    }
    private final class Panel{final GuardedDialog dialog;final LinearLayout root;Panel(GuardedDialog d,LinearLayout r){dialog=d;root=r;}}
    private Panel panel(String title){
        GuardedDialog d=new GuardedDialog();LinearLayout outer=column();outer.setBackground(bg(Color.WHITE,18,null));
        LinearLayout header=row();header.setPadding(dp(22),dp(16),dp(16),dp(16));
        TextView heading=new TextView(this);heading.setText(title);heading.setTextColor(ink);heading.setTextSize(15);heading.setMaxLines(2);heading.setTypeface(null,Typeface.BOLD);header.addView(heading,new LinearLayout.LayoutParams(0,-2,1));
        Button close=button("关闭",false,d::requestClose);close.setTextSize(12);close.setContentDescription("关闭当前窗口");header.addView(close,new LinearLayout.LayoutParams(dp(62),dp(42)));outer.addView(header);divider(outer,0,0);
        ScrollView scroll=new ScrollView(this);scroll.setFillViewport(false);scroll.setVerticalScrollBarEnabled(false);LinearLayout root=column();root.setPadding(dp(22),dp(23),dp(22),dp(26));scroll.addView(root);outer.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));
        d.setContentView(outer);d.show();if(d.getWindow()!=null){d.getWindow().setBackgroundDrawableResource(android.R.color.transparent);d.getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);d.getWindow().setLayout(getResources().getDisplayMetrics().widthPixels-dp(18),(int)(getResources().getDisplayMetrics().heightPixels*.9));}return new Panel(d,root);
    }
    private void divider(LinearLayout parent,int before,int after){View v=new View(this);v.setBackgroundColor(border);LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,dp(1));p.topMargin=dp(before);p.bottomMargin=dp(after);parent.addView(v,p);}
    private void sectionLabel(LinearLayout parent,String title,String count){LinearLayout r=row();TextView h=new TextView(this);h.setText(title);h.setTextSize(13);h.setTypeface(null,Typeface.BOLD);h.setTextColor(ink);r.addView(h,new LinearLayout.LayoutParams(0,-2,1));TextView c=new TextView(this);c.setText(count);c.setTextSize(11);c.setTextColor(muted);r.addView(c);LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.topMargin=dp(18);p.bottomMargin=dp(13);parent.addView(r,p);}
    private void stepRow(LinearLayout parent,String value,int index,JSONArray terms){LinearLayout r=row();r.setGravity(Gravity.TOP);r.setPadding(dp(12),dp(12),dp(14),dp(12));r.setBackground(bg(soft,11,null));TextView number=chip(String.valueOf(index),teal,mint);number.setGravity(Gravity.CENTER);number.setPadding(0,0,0,0);r.addView(number,new LinearLayout.LayoutParams(dp(27),dp(27)));TextView t=new TextView(this);t.setText(value);t.setTextSize(15);t.setTextColor(ink);t.setLineSpacing(dp(5),1);t.setTextIsSelectable(true);highlightMatches(t,terms);LinearLayout.LayoutParams tp=new LinearLayout.LayoutParams(0,-2,1);tp.leftMargin=dp(12);r.addView(t,tp);LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(8);parent.addView(r,p);}
    private void navigateScreen(String target){if(screen.equals(target))return;if(anatomyWeb!=null){anatomyWeb.evaluateJavascript("document.querySelector('#atlas-root')?.dataset.dirty === 'true'",value->{if("true".equals(value))new AlertDialog.Builder(this).setMessage("有未保存的图谱修改，确定放弃吗？").setNegativeButton("继续编辑",null).setPositiveButton("放弃修改",(d,w)->showScreen(target)).show();else showScreen(target);});return;}if("对话".equals(screen)){EditText draft=body.findViewWithTag("chat-input");if(draft!=null)chatDraft=draft.getText().toString();}if(settingsDirty){new AlertDialog.Builder(this).setTitle("设置尚未保存").setMessage("保存后才能在其他设备使用，是否放弃修改？").setNegativeButton("继续编辑",null).setPositiveButton("放弃修改",(d,w)->showScreen(target)).show();}else showScreen(target);}
    private void showNote(String noteId){work(()->(JSONObject)api.request("/notes/"+noteId),note->{
        Panel p=panel("完整记录");LinearLayout root=p.root;text(root,typeNames(note)+"  ·  "+str(note,"occurred_on"),11,teal,false);text(root,str(note,"title"),23,ink,true);text(root,String.join("  ·  ",strings(array(note,"tags"))),11,muted,false);
        JSONObject details=note.optJSONObject("details");if(details!=null&&details.length()>0){sectionLabel(root,"专属信息","");java.util.Iterator<String> keys=details.keys();while(keys.hasNext()){String key=keys.next();text(root,key,12,teal,true);TextView value=text(root,details.optString(key),15,ink,false);value.setTextIsSelectable(true);}}
        sectionLabel(root,str(learningType(note),"content_label"),"");TextView content=text(root,str(note,"content").isEmpty()?"尚未填写正文":str(note,"content"),15,ink,false);content.setTextIsSelectable(true);
        JSONArray steps=array(note,"steps");sectionLabel(root,str(learningType(note),"steps_label"),steps.length()+" 项");for(int i=0;i<steps.length();i++)stepRow(root,steps.optString(i),i+1,new JSONArray());if(steps.length()==0)text(root,"尚未填写步骤",12,muted,false);
        JSONArray files=array(note,"attachments");sectionLabel(root,"原始材料",files.length()+" 份");for(JSONObject file:objects(files))addButton(root,str(file,"filename"),()->showAttachment(file));if(files.length()==0)text(root,"未关联原始材料",12,muted,false);
        divider(root,15,15);addButton(root,"编辑笔记",true,()->{p.dialog.dismiss();editNote(note,null);});addButton(root,"回顾这篇记录",()->startNoteRecall(note,p));
        Button delete=addButton(root,"删除笔记",()->new AlertDialog.Builder(this).setTitle("删除笔记？").setMessage("所有账户都将无法查看这篇笔记；原件保留。").setNegativeButton("取消",null).setPositiveButton("删除",(d,w)->work(()->api.request("/notes/"+noteId,"DELETE"),v->{lastSearch=null;p.dialog.dismiss();showScreen(screen);})).show());delete.setTextColor(Color.rgb(164,80,67));
    });}
    private void startNoteRecall(JSONObject note,Panel panel){
        if(working||chatSending){toast("请等待当前操作完成");return;}
        Runnable open=()->{panel.dialog.dismiss();conversationId=null;recallNoteId="";chatMessages.clear();chatFiles.clear();chatMode="knowledge";chatError="";chatSending=false;chatDraft="请回顾「"+str(note,"title")+"」的记录与已保存步骤";recallNoteId=id(note);showScreen("对话");if(chatInput!=null){chatInput.setSelection(chatInput.length());updateChatSend();}};
        if(!chatDraft.trim().isEmpty()||!chatFiles.isEmpty())new AlertDialog.Builder(this).setTitle("开始回顾这篇笔记？").setMessage("当前尚未发送的内容将被替换。").setNegativeButton("保留当前内容",null).setPositiveButton("开始回顾",(d,w)->open.run()).show();else open.run();
    }
    private List<String> split(String text,String regex){List<String> result=new ArrayList<>();for(String value:text.split(regex)){String s=value.trim();if(!s.isEmpty())result.add(s);}return result;}
    private void editNote(JSONObject note,JSONObject material){
        if (!working) work(()->{loadLearningCatalog();return true;},v->editNoteLoaded(note,material,""));
    }
    private void openOrganizer(String entry) {
        if (!working) work(()->{loadLearningCatalog();return true;},v->editNoteLoaded(null,null,entry));
    }
    private void editNoteLoaded(JSONObject note,JSONObject material,String entry){
        boolean directImport = !entry.isEmpty();
        java.util.LinkedHashMap<String,EditText> detailInputs=new java.util.LinkedHashMap<>();
        java.util.LinkedHashMap<String,String> detailValues=new java.util.LinkedHashMap<>();
        JSONObject savedDetails=note==null?null:note.optJSONObject("details");
        if(savedDetails!=null){java.util.Iterator<String> keys=savedDetails.keys();while(keys.hasNext()){String key=keys.next();detailValues.put(key,savedDetails.optString(key));}}
        LinearLayout detailBox=column();
        Runnable[] drawDetails={()->{}};
        Panel p=panel(note==null?(directImport?"导入材料并整理":"新建笔记"):"编辑笔记");LinearLayout root=p.root;EditText title=input(root,"标题",note==null?str(material,"filename"):str(note,"title"));List<String> selectedTypes=note==null?new ArrayList<>(Arrays.asList(str(learningTypes.optJSONObject(0),"id"))):new ArrayList<>(typeIDs(note));TextView category=text(root,"",13,teal,true);Runnable typeSummary=()->{List<String> names=new ArrayList<>();for(String id:selectedTypes)names.add(str(learningType(obj("note_type",id)),"title"));category.setText("已选类型："+String.join("、",names));drawDetails[0].run();};typeSummary.run();addButton(root,"选择资料类型（可多选）",()->chooseTypes(selectedTypes,chosen->{replace(selectedTypes,chosen);if(selectedTypes.size()>1)selectedTypes.remove("other");if(selectedTypes.isEmpty())selectedTypes.add("other");typeSummary.run();}));root.addView(detailBox);
        final boolean[] moreOpen={false};
        java.util.Set<String> knownDetails=new java.util.HashSet<>(),customDetails=new java.util.HashSet<>();
        java.util.Iterator<String> templateKeys=detailTemplates.keys();while(templateKeys.hasNext())knownDetails.addAll(strings(array(detailTemplates,templateKeys.next())));
        for(String key:detailValues.keySet())if(!knownDetails.contains(key))customDetails.add(key);
        drawDetails[0]=()->{
            for(java.util.Map.Entry<String,EditText> e:detailInputs.entrySet())detailValues.put(e.getKey(),e.getValue().getText().toString());
            detailInputs.clear();detailBox.removeAllViews();detailBox.setPadding(dp(12),dp(12),dp(12),dp(8));detailBox.setBackground(bg(Color.WHITE,10,border));
            LinearLayout heading=row();TextView headingTitle=new TextView(this);headingTitle.setText("专属信息");headingTitle.setTextSize(14);headingTitle.setTypeface(null,Typeface.BOLD);headingTitle.setTextColor(ink);heading.addView(headingTitle,new LinearLayout.LayoutParams(0,-2,1));TextView optional=new TextView(this);optional.setText("选填");optional.setTextSize(10);optional.setTextColor(muted);heading.addView(optional);LinearLayout.LayoutParams hp=new LinearLayout.LayoutParams(-1,-2);hp.bottomMargin=dp(12);detailBox.addView(heading,hp);
            java.util.LinkedHashSet<String> fields=new java.util.LinkedHashSet<>(),primary=new java.util.LinkedHashSet<>();
            for(String type:selectedTypes){fields.addAll(strings(array(detailTemplates,type)));List<String> core=strings(array(detailPrimaryFields,type));if(core.isEmpty()){List<String> fallback=strings(array(detailTemplates,type));core=fallback.subList(0,Math.min(4,fallback.size()));}primary.addAll(core);}
            for(String key:detailValues.keySet())if(!detailValues.get(key).trim().isEmpty()||customDetails.contains(key))fields.add(key);
            List<String> common=new ArrayList<>(),extra=new ArrayList<>();for(String key:fields)(primary.contains(key)?common:extra).add(key);
            LinearLayout identity=column();identity.setPadding(dp(10),dp(10),dp(10),dp(2));identity.setBackground(bg(Color.rgb(245,248,246),8,null));detailBox.addView(identity);List<String> identityKeys=new ArrayList<>(),clinicalKeys=new ArrayList<>();for(String key:common){String kind=str(detailFieldSpecs.optJSONObject(key),"kind");if("long".equals(kind)||"multi".equals(kind))clinicalKeys.add(key);else identityKeys.add(key);}drawDetailGroup(identity,identityKeys,detailValues,detailInputs);if(!clinicalKeys.isEmpty()){LinearLayout clinical=column();LinearLayout.LayoutParams clp=new LinearLayout.LayoutParams(-1,-2);clp.topMargin=dp(10);detailBox.addView(clinical,clp);drawDetailGroup(clinical,clinicalKeys,detailValues,detailInputs);}
            LinearLayout extended=column(),footer=row();footer.setPadding(0,dp(2),0,0);
            if(!extra.isEmpty()){
                int filled=0;for(String key:extra)if(!detailValues.getOrDefault(key,"").trim().isEmpty())filled++;
                String count=extra.size()+" 项"+(filled>0?" · 已填 "+filled+" 项":"");
                Button toggle=button((moreOpen[0]?"收起":"更多信息")+" · "+count+"  ⌄",false,()->{});toggle.setTextSize(12);toggle.setMinHeight(dp(40));toggle.setMinimumHeight(dp(40));toggle.setPadding(dp(8),dp(4),dp(8),dp(4));toggle.setBackgroundColor(Color.TRANSPARENT);toggle.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);footer.addView(toggle,new LinearLayout.LayoutParams(0,dp(40),1));
                toggle.setOnClickListener(v->{moreOpen[0]=!moreOpen[0];extended.setVisibility(moreOpen[0]?View.VISIBLE:View.GONE);toggle.setText((moreOpen[0]?"收起":"更多信息")+" · "+count+"  ⌄");});
                detailBox.addView(extended);drawDetailGroup(extended,extra,detailValues,detailInputs);extended.setVisibility(moreOpen[0]?View.VISIBLE:View.GONE);
            }
            Button add=button("＋ 自定义字段",false,()->{EditText name=new EditText(this);name.setHint("如：职业、疗程、文献编号");new AlertDialog.Builder(this).setTitle("自定义字段名称").setView(name).setNegativeButton("取消",null).setPositiveButton("添加",(d,w)->{String key=name.getText().toString().trim();if(key.isEmpty()||key.length()>60){toast("请输入 1–60 字的字段名称");return;}if(detailInputs.containsKey(key)){toast("该字段已存在");return;}if(detailInputs.size()>=100){toast("最多 100 个字段");return;}detailValues.put(key,"");customDetails.add(key);moreOpen[0]=true;drawDetails[0].run();}).show();});add.setTextSize(12);add.setMinHeight(dp(40));add.setMinimumHeight(dp(40));add.setBackgroundColor(Color.TRANSPARENT);add.setTextColor(teal);add.setPadding(dp(8),0,0,0);footer.addView(add,new LinearLayout.LayoutParams(-2,dp(40)));detailBox.addView(footer);
        };drawDetails[0].run();
        LinearLayout dateTags=row();dateTags.setGravity(Gravity.TOP);LinearLayout dateCell=column(),tagsCell=column();LinearLayout.LayoutParams dcp=new LinearLayout.LayoutParams(0,-2,1);dcp.rightMargin=dp(10);dateTags.addView(dateCell,dcp);dateTags.addView(tagsCell,new LinearLayout.LayoutParams(0,-2,1));LinearLayout.LayoutParams dtp=new LinearLayout.LayoutParams(-1,-2);dtp.topMargin=dp(14);root.addView(dateTags,dtp);EditText date=input(dateCell,"记录日期",note==null?LocalDate.now().toString():str(note,"occurred_on"));EditText tags=input(tagsCell,"标签 · 逗号分隔",note==null?"":String.join("，",strings(array(note,"tags"))));EditText content=input(root,"正文",note==null?str(material,"extracted_text"):str(note,"content"),true);EditText steps=input(root,"步骤 / 学习要点（每行一条）",note==null?"":String.join("\n",strings(array(note,"steps"))),true);
        addButton(root,"选择已有标签",()->{List<String> names=new ArrayList<>();for(JSONObject tag:objects(learningTags))names.add(str(tag,"name"));if(names.isEmpty()){toast("暂无标签，可直接输入或在设置中新增");return;}List<String> selected=split(tags.getText().toString(),"[,，]");boolean[] checked=new boolean[names.size()];for(int i=0;i<names.size();i++)checked[i]=selected.contains(names.get(i));new AlertDialog.Builder(this).setTitle("选择已有标签").setMultiChoiceItems(names.toArray(new String[0]),checked,(d,i,on)->checked[i]=on).setNegativeButton("取消",null).setPositiveButton("确定",(d,w)->{List<String> next=new ArrayList<>(selected);for(int i=0;i<names.size();i++){if(checked[i]&&!next.contains(names.get(i)))next.add(names.get(i));if(!checked[i])next.remove(names.get(i));}if(next.size()>20){toast("每篇笔记最多 20 个标签");return;}tags.setText(String.join("，",next));}).show();});
        List<JSONObject> files = note == null ? new ArrayList<>() : new ArrayList<>(objects(array(note,"attachments")));
        if (material != null) files.add(material);
        LinearLayout fileView = column();
        LinearLayout intake = column();
        if (directImport) {
            intake.setPadding(dp(14),dp(10),dp(14),dp(8));
            intake.setBackground(bg(Color.rgb(237,246,241),14,border));
            root.addView(intake,0);
        } else root.addView(intake);
        intake.addView(fileView);
        drawEditorFiles(fileView,files);
        Button uploadButton = addButton(intake,directImport?"＋ 选择文件并生成":"＋ 批量关联材料",directImport,()->{});
        final JSONObject[] appliedDraft = {null};
        EditText organizerRule = new EditText(this);
        CheckBox rememberRule = new CheckBox(this);
        Button generate = null;
        TextView progress = null;
        if (note == null) {
            LinearLayout organizer = column();
            if (directImport) intake.addView(organizer);
            else {
                addButton(root,"从材料生成笔记",false,()->{
                    organizer.setVisibility(organizer.getVisibility() == View.VISIBLE ? View.GONE : View.VISIBLE);
                });
                root.addView(organizer);
                organizer.setVisibility(View.GONE);
            }
            text(organizer,"也可以粘贴零散文字。整理使用已配置的对话模型。",12,muted,false);
            EditText source = input(organizer,"零散文字","",true);
            source.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(48000)});
            organizerRule.setHint("本次整理要求（可选）");
            organizerRule.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(1000)});
            organizer.addView(organizerRule);
            rememberRule.setText("保存后在同类笔记中复用这条要求");
            organizer.addView(rememberRule);
            Button run = addButton(organizer,"生成草稿",false,()->{});
            TextView runStatus = text(organizer,"",12,teal,false);
            runStatus.setVisibility(View.GONE);
            runStatus.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
            generate = run;
            progress = runStatus;
            run.setOnClickListener(v->{
                if (working) return;
                String type = selectedTypes.get(0);
                String sourceText = source.getText().toString();
                String rule = organizerRule.getText().toString();
                JSONArray sourceIDs = ids(files);
                JSONObject request = obj("note_type",type,"attachment_ids",sourceIDs,"text",sourceText,"rule",rule);
                runStatus.setVisibility(View.VISIBLE);
                runStatus.setText("正在整理材料…");
                work(()->(JSONObject)api.request("/notes/organize","POST",request),draft->{
                    if (!p.dialog.isShowing()) return;
                    runStatus.setText("草稿已生成，请核对后应用。");
                    previewOrganizedNote(draft,()->{
                        if (!type.equals(selectedTypes.get(0)) || !sourceIDs.toString().equals(ids(files).toString()) ||
                                !sourceText.equals(source.getText().toString()) || !rule.equals(organizerRule.getText().toString())) {
                            toast("类型、材料或整理要求已改变，请重新生成");
                            return;
                        }
                        if (title.getText().toString().trim().isEmpty()) title.setText(str(draft,"title"));
                        content.setText(str(draft,"content"));
                        List<String> parts = strings(array(draft,"parts"));
                        EditText partInput = detailInputs.get("标准部位");
                        String value = String.join("、",parts);
                        if (partInput != null) partInput.setText(value);
                        if (!parts.isEmpty()) {
                            detailValues.put("标准部位",value);
                            customDetails.add("标准部位");
                            moreOpen[0] = true;
                            drawDetails[0].run();
                        }
                        appliedDraft[0] = obj("id",str(draft,"id"),"type",type,"ids",sourceIDs.toString(),"rule",rule);
                        toast("草稿已填入正文，请核对后保存");
                    },!content.getText().toString().trim().isEmpty());
                },error->runStatus.setText(error.getMessage()==null?"整理失败，请重试。":error.getMessage()));
            });
        }
        Button run = generate;
        TextView runStatus = progress;
        java.util.function.BiConsumer<List<JSONObject>, Boolean> finishImport = (uploaded,complete)->{
            if (!p.dialog.isShowing()) return;
            files.addAll(uploaded);
            drawEditorFiles(fileView,files);
            if (!directImport || run == null || runStatus == null) return;
            if (complete && uploaded.stream().allMatch(file->"ready".equals(str(file,"status")) &&
                    !str(file,"extracted_text").trim().isEmpty())) run.performClick();
            else {
                runStatus.setVisibility(View.VISIBLE);
                runStatus.setText("有材料上传或提取未完成。请核对材料，再点击「生成草稿」。");
            }
        };
        uploadButton.setOnClickListener(v->pickEditorFiles(20-files.size(),finishImport,()->{
            if (directImport && runStatus != null) {
                runStatus.setVisibility(View.VISIBLE);
                runStatus.setText("正在上传并读取材料…");
            }
        }));
        java.util.function.Supplier<String> snapshot=()->title.getText()+"\u001f"+selectedTypes.toString()+"\u001f"+date.getText()+"\u001f"+tags.getText()+"\u001f"+content.getText()+"\u001f"+steps.getText()+"\u001f"+ids(files).toString()+detailPayload(detailInputs).toString();String initial=snapshot.get();p.dialog.dirty=()->!snapshot.get().equals(initial);
        addButton(root,"保存笔记",true,()->{JSONObject data=obj("expected_version",note==null?"":str(note,"version"),"details",detailPayload(detailInputs),"title",title.getText().toString(),"note_types",array(selectedTypes),"occurred_on",date.getText().toString(),"tags",array(split(tags.getText().toString(),"[,，]")),"content",content.getText().toString(),"steps",array(split(steps.getText().toString(),"\\n")),"attachment_ids",ids(files));JSONObject draft = appliedDraft[0];
            if (draft != null) {
                if (!str(draft,"type").equals(selectedTypes.get(0)) || !str(draft,"ids").equals(ids(files).toString())) {
                    toast("资料类型或材料有变化，请重新生成并应用草稿");
                    return;
                }
                put(data,"organizer_draft_id",str(draft,"id"));
                put(data,"remember_organizer_rule",rememberRule.isChecked() && str(draft,"rule").equals(organizerRule.getText().toString()));
            }
            work(()->api.request(note==null?"/notes":"/notes/"+id(note),note==null?"POST":"PUT",data),v->{lastSearch=null;p.dialog.dismiss();toast("笔记已保存");showScreen(screen);});});
        if ("pick".equals(entry)) uploadButton.performClick();
        if ("demo".equals(entry) && runStatus != null) {
            runStatus.setVisibility(View.VISIBLE);
            runStatus.setText("正在导入两份虚构示例…");
            List<JSONObject> samples = new ArrayList<>();
            work(()->{
                for (String[] sample : new String[][]{{"observation.txt","演示观察.txt"},{"followup.txt","演示随访.txt"}}) {
                    samples.add(api.upload(sample[1],Api.readBytes(getAssets().open(sample[0]),4096)));
                }
                return samples;
            },uploaded->finishImport.accept(uploaded,true),error->finishImport.accept(samples,false));
        }
    }
    private void previewOrganizedNote(JSONObject draft,Runnable apply,boolean replacesText) {
        StringBuilder preview = new StringBuilder(str(draft,"content"));
        List<String> parts = strings(array(draft,"parts"));
        if (!parts.isEmpty()) preview.append("\n\n标准部位 · ").append(String.join("、",parts));
        for (String warning : strings(array(draft,"warnings"))) preview.append("\n").append(warning);
        List<String> missing = strings(array(draft,"missing"));
        if (!missing.isEmpty()) preview.append("\n缺少资料 · ").append(String.join("、",missing));
        JSONArray sourceNames = array(draft,"source_names");
        int index = 0;
        preview.append("\n\n核对原文");
        for (JSONObject evidence : objects(array(draft,"evidence"))) {
            preview.append("\n〔").append(++index).append("〕")
                    .append(sourceNames.optString(evidence.optInt("source")-1)).append(" · ").append(str(evidence,"quote"));
        }
        new AlertDialog.Builder(this).setTitle(str(draft,"title")).setMessage(preview)
                .setNegativeButton("返回",null)
                .setPositiveButton(replacesText?"替换当前正文":"应用到编辑器",(dialog,which)->apply.run()).show();
    }

    private void drawDetailGroup(LinearLayout parent,List<String> fields,java.util.Map<String,String> values,java.util.Map<String,EditText> inputs){
        LinearLayout pair=null;int columns=0;
        for(String key:fields){JSONObject spec=detailFieldSpecs.optJSONObject(key);String kind=spec==null?"short":spec.optString("kind","short");boolean wide="long".equals(kind)||"multi".equals(kind);
            LinearLayout cell=column();if(wide){pair=null;columns=0;parent.addView(cell);}else{if(pair==null||columns==2){pair=row();pair.setGravity(Gravity.TOP);parent.addView(pair);columns=0;}LinearLayout.LayoutParams cp=new LinearLayout.LayoutParams(0,-2,1);if(columns>0)cp.leftMargin=dp(10);pair.addView(cell,cp);columns++;}
            TextView caption=text(cell,key+("multi".equals(kind)?" · 多选":""),11,muted,true);((LinearLayout.LayoutParams)caption.getLayoutParams()).bottomMargin=dp(4);
            EditText field=new EditText(this);field.setText(values.getOrDefault(key,""));field.setContentDescription(key);field.setTextSize(13);field.setTextColor(ink);field.setHintTextColor(muted);field.setPadding(dp(10),dp(8),dp(10),dp(8));field.setBackground(bg(Color.WHITE,7,border));field.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(4000)});inputs.put(key,field);
            boolean picker="single".equals(kind)||"multi".equals(kind)||"date".equals(kind);
            if(picker){
                Button select=button("",false,()->{});select.setTextSize(13);select.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);select.setPadding(dp(10),dp(6),dp(10),dp(6));select.setMinHeight(dp(42));select.setMinimumHeight(dp(42));select.setContentDescription(key+("multi".equals(kind)?"，可多选":"，选择"));
                Runnable paint=()->{String value=field.getText().toString();select.setText(value.isEmpty()?("multi".equals(kind)?"＋ 选择或添加多项":"选择"+key+" ⌄"):value.replace("\n"," · ")+" ⌄");select.setTextColor(value.isEmpty()?muted:teal);};paint.run();select.setOnClickListener(v->{if("date".equals(kind)){java.time.LocalDate date=java.time.LocalDate.now();try{date=java.time.LocalDate.parse(field.getText().toString());}catch(Exception ignored){}new android.app.DatePickerDialog(this,(d,y,m,day)->{field.setText(String.format(java.util.Locale.ROOT,"%04d-%02d-%02d",y,m+1,day));paint.run();},date.getYear(),date.getMonthValue()-1,date.getDayOfMonth()).show();}else chooseDetailValues(key,kind,spec,field,paint);});
                LinearLayout.LayoutParams sp=new LinearLayout.LayoutParams(-1,-2);sp.bottomMargin=dp(10);cell.addView(select,sp);
            }else{
                field.setInputType(InputType.TYPE_CLASS_TEXT|("long".equals(kind)?InputType.TYPE_TEXT_FLAG_MULTI_LINE:0));if("long".equals(kind)){field.setMinLines(2);field.setGravity(Gravity.TOP);}else field.setSingleLine(true);field.setHint("年龄".equals(key)?"岁 / 月龄":"未填写");LinearLayout.LayoutParams fp=new LinearLayout.LayoutParams(-1,-2);fp.bottomMargin=dp(10);cell.addView(field,fp);
            }
        }
    }
    private void chooseDetailValues(String key,String kind,JSONObject spec,EditText field,Runnable paint){
        boolean multi="multi".equals(kind);List<String> selected=split(field.getText().toString(),"[、，,;；\\n]");List<String> options=new ArrayList<>(strings(array(spec,"options")));for(String value:selected)if(!options.contains(value))options.add(value);
        LinearLayout root=column();root.setPadding(dp(20),dp(4),dp(20),dp(4));EditText custom=new EditText(this);custom.setHint(multi?"补充项目，用顿号分隔":"其他内容（可手动填写）");custom.setTextSize(14);custom.setSingleLine(true);custom.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(4000)});
        ScrollView scroll=new ScrollView(this);LinearLayout choices=column();scroll.addView(choices);root.addView(scroll,new LinearLayout.LayoutParams(-1,Math.min(dp(230),Math.max(dp(48),options.size()*dp(44)))));root.addView(custom);
        final String[] single={field.getText().toString()};
        for(String option:options){if(multi){CheckBox box=new CheckBox(this);box.setText(option);box.setTextSize(14);box.setChecked(selected.contains(option));box.setMinHeight(dp(44));box.setOnCheckedChangeListener((v,on)->{if(on&&!selected.contains(option))selected.add(option);if(!on)selected.remove(option);});choices.addView(box);}else{android.widget.RadioButton radio=new android.widget.RadioButton(this);radio.setText(option);radio.setTextSize(14);radio.setMinHeight(dp(44));radio.setChecked(option.equals(single[0]));radio.setOnClickListener(v->{single[0]=option;for(int i=0;i<choices.getChildCount();i++)((android.widget.RadioButton)choices.getChildAt(i)).setChecked(choices.getChildAt(i)==radio);custom.setText("");});choices.addView(radio);}}
        new AlertDialog.Builder(this).setTitle(key+(multi?" · 可多选":" · 单选")).setView(root).setNeutralButton("清空",(d,w)->{field.setText("");paint.run();}).setNegativeButton("取消",null).setPositiveButton("应用",(d,w)->{String added=custom.getText().toString().trim();if(multi){for(String value:split(added,"[、，,;；\\n]"))if(!selected.contains(value))selected.add(value);String value=String.join("\n",selected);if(value.length()>4000){toast("该字段内容过长");return;}field.setText(value);}else field.setText(added.isEmpty()?single[0]:added);paint.run();}).show();
    }
    private JSONObject detailPayload(java.util.Map<String,EditText> inputs){JSONObject result=new JSONObject();for(java.util.Map.Entry<String,EditText> e:inputs.entrySet()){String value=e.getValue().getText().toString();if(!value.trim().isEmpty())try{result.put(e.getKey(),value);}catch(org.json.JSONException ex){throw new RuntimeException(ex);}}return result;}
    private void drawEditorFiles(LinearLayout parent,List<JSONObject> files){parent.removeAllViews();for(JSONObject f:files){text(parent,str(f,"filename"),12,teal,false);addButton(parent,"移除关联",()->{files.remove(f);drawEditorFiles(parent,files);});}}
    private void materialsScreen(){
        pageHeading("材料归档","让图片、录音和文档，回到对应的记录。");addButton(body,"＋ 上传原始材料",true,()->pickFile(f->{showScreen("材料");showAttachment(f);}));
        work(()->(JSONArray)api.request("/attachments"),files->{
            final String[] kind={"all"};EditText filter=input(body,"查找材料","");filter.setHint("按文件名查找");
            HorizontalScrollView scroll=new HorizontalScrollView(this);scroll.setHorizontalScrollBarEnabled(false);LinearLayout tabs=row();scroll.addView(tabs);body.addView(scroll);LinearLayout listing=column();LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.topMargin=dp(15);body.addView(listing,lp);
            Runnable[] paint=new Runnable[1];paint[0]=()->{listing.removeAllViews();int count=0;String query=filter.getText().toString().trim().toLowerCase(java.util.Locale.ROOT);for(JSONObject f:objects(files)){String mime=str(f,"mime");boolean match=kind[0].equals("all")||(kind[0].equals("document")?!mime.startsWith("image/")&&!mime.startsWith("audio/"):mime.startsWith(kind[0]+"/"));if(!match||!str(f,"filename").toLowerCase(java.util.Locale.ROOT).contains(query))continue;count++;LinearLayout c=card(listing);LinearLayout item=row();if(mime.startsWith("image/")){android.widget.ImageView image=new android.widget.ImageView(this);image.setScaleType(android.widget.ImageView.ScaleType.FIT_CENTER);image.setBackground(bg(soft,6,border));item.addView(image,new LinearLayout.LayoutParams(dp(48),dp(48)));loadResultThumbnail(image,f);}else item.addView(chip(mime.startsWith("audio/")?"音频":"文档",teal,mint));LinearLayout info=column();TextView title=text(info,str(f,"filename"),14,ink,true);title.setMaxLines(2);text(info,(f.isNull("note_id")?"待关联笔记":"已关联笔记")+" · "+str(f,"created_at").substring(0,10),11,muted,false);LinearLayout.LayoutParams ip=new LinearLayout.LayoutParams(0,-2,1);ip.leftMargin=dp(14);item.addView(info,ip);c.addView(item);c.setClickable(true);c.setFocusable(true);c.setContentDescription("查看材料："+str(f,"filename"));c.setOnClickListener(v->showAttachment(f));}if(count==0){LinearLayout empty=card(listing);text(empty,files.length()==0?"先保存一份原始材料":"没有符合条件的材料",16,ink,true);text(empty,files.length()==0?"上传后核对识别文字，再关联到笔记。":"换个文件名或类型试试。",12,muted,false);}};
            for(String[] type:new String[][]{{"all","全部"},{"image","图片"},{"audio","音频"},{"document","文档"}}){Button b=button(type[1],false,()->{});b.setTextSize(12);b.setOnClickListener(v->{kind[0]=type[0];for(int i=0;i<tabs.getChildCount();i++)tabs.getChildAt(i).setBackground(bg(tabs.getChildAt(i)==b?mint:Color.WHITE,7,border));paint[0].run();});LinearLayout.LayoutParams tp=new LinearLayout.LayoutParams(-2,dp(38));tp.rightMargin=dp(8);tabs.addView(b,tp);}
            filter.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence a,int st,int c,int after){}public void onTextChanged(CharSequence a,int st,int before,int count){paint[0].run();}public void afterTextChanged(Editable e){}});paint[0].run();
        });
    }
    private void afterDiscard(Panel p,Runnable action){if(p.dialog.dirty.getAsBoolean())new AlertDialog.Builder(this).setTitle("放弃未保存的文字？").setMessage("重新提取会替换当前识别文字。").setNegativeButton("继续编辑",null).setPositiveButton("重新提取",(d,w)->action.run()).show();else action.run();}
    private void showAttachment(JSONObject f){
        Panel p=panel("原始材料");LinearLayout root=p.root;text(root,str(f,"filename"),17,ink,true);if(str(f,"mime").startsWith("image/")){android.widget.ImageView image=new android.widget.ImageView(this);image.setScaleType(android.widget.ImageView.ScaleType.FIT_CENTER);image.setBackground(bg(soft,8,border));image.setContentDescription("原始图片预览");root.addView(image,new LinearLayout.LayoutParams(-1,dp(190)));loadResultThumbnail(image,f);}addButton(root,"打开原件",()->openFile(f));EditText extraction=input(root,"识别 / 转写文本（可以手动修正）",str(f,"extracted_text"),true);text(root,"请对照原件核对文字；保存后会更新关联笔记的检索。",11,muted,false);p.dialog.dirty=()->!extraction.getText().toString().equals(str(f,"extracted_text"));
        addButton(root,"保存文本",true,()->work(()->(JSONObject)api.request("/attachments/"+id(f)+"/text","PUT",obj("text",extraction.getText().toString())),updated->{put(f,"extracted_text",str(updated,"extracted_text"));put(f,"status",str(updated,"status"));lastSearch=null;p.dialog.dismiss();lastSearch=null;toast("文本已保存");}));
        addButton(root,"重新提取",()->afterDiscard(p,()->work(()->(JSONObject)api.request("/attachments/"+id(f)+"/extract","POST"),updated->{put(f,"extracted_text",str(updated,"extracted_text"));put(f,"status",str(updated,"status"));extraction.setText(str(updated,"extracted_text"));toast("提取完成，请核对文本；为空时可手动补充");})));
        addButton(root,f.isNull("note_id")?"用于新笔记":"查看关联笔记",()->{p.dialog.dismiss();if(f.isNull("note_id"))editNote(null,f);else showNote(str(f,"note_id"));});
        addButton(root,"删除材料",()->new AlertDialog.Builder(this).setTitle("删除材料？").setMessage("原始文件和识别文本都会删除。").setNegativeButton("取消",null).setPositiveButton("删除",(d,w)->work(()->api.request("/attachments/"+id(f),"DELETE"),v->{queryFiles.removeIf(x->id(x).equals(id(f)));chatFiles.removeIf(x->id(x).equals(id(f)));p.dialog.dismiss();showScreen(screen);})).show());
    }
    private File saveExport(String name,byte[] bytes)throws Exception{File dir=new File(getCacheDir(),"exports");if(!dir.exists()&&!dir.mkdirs())throw new IllegalStateException("无法创建临时目录");File file=new File(dir,name);try(FileOutputStream out=new FileOutputStream(file)){out.write(bytes);}return file;}
    private Uri exportUri(File file){return Uri.parse("content://"+getPackageName()+".files/"+file.getName());}
    private void openFile(JSONObject f){work(()->{String extension=str(f,"filename");extension=extension.substring(extension.lastIndexOf('.')+1).replaceAll("[^A-Za-z0-9]","");return saveExport(UUID.randomUUID()+"."+extension,api.download("/attachments/"+id(f)+"/file"));},file->{try{startActivity(new Intent(Intent.ACTION_VIEW).setDataAndType(exportUri(file),str(f,"mime")).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION));}catch(Exception e){toast("设备没有此格式的查看器，仍可在这里查看识别文本");}});}
    private void selectedMaterials(List<JSONObject> files,TextView label){if(files.isEmpty())return;Panel p=panel("本次使用的材料");for(JSONObject f:new ArrayList<>(files)){LinearLayout c=card(p.root);text(c,str(f,"filename"),14,ink,true);addButton(c,"查看 / 核对文字",()->showAttachment(f));addButton(c,"从本次输入移除",()->{files.remove(f);if("对话".equals(screen))updateChatSend();label.setText(fileSummary(files));label.setVisibility(files.isEmpty()?View.GONE:View.VISIBLE);p.dialog.dismiss();toast("已移除；原件仍在材料归档中");});}}
    private void addChatFile(JSONObject f,TextView label){if(chatFiles.size()>=5){toast("最多添加 5 份对话材料");return;}chatFiles.add(f);label.setVisibility(View.VISIBLE);label.setText(fileSummary(chatFiles));if(!"ready".equals(str(f,"status")))showAttachment(f);}
    private void resetChat(String mode){if(working){toast("请等待当前操作完成");return;}EditText input=body==null?null:body.findViewWithTag("chat-input");String draft=input==null?chatDraft:input.getText().toString();Runnable reset=()->{chatMode=mode;conversationId=null;recallNoteId="";chatMessages.clear();chatFiles.clear();chatDraft="";chatError="";showScreen("对话");};if(!draft.trim().isEmpty()||!chatFiles.isEmpty())new AlertDialog.Builder(this).setTitle("开始新的对话？").setMessage("当前尚未发送的文字与材料将从输入框移除。").setNegativeButton("继续编辑",null).setPositiveButton("新建对话",(d,w)->reset.run()).show();else reset.run();}
    private View chatIcon(String name,String label,boolean primary,Runnable action){LinearLayout control=row();control.setGravity(Gravity.CENTER);control.setBackground(new RippleDrawable(ColorStateList.valueOf(mint),bg(primary?teal:Color.WHITE,22,null),null));LineIcon icon=new LineIcon(this,name,primary?Color.WHITE:muted);control.addView(icon,new LinearLayout.LayoutParams(dp(22),dp(22)));control.setContentDescription(label);control.setClickable(true);control.setFocusable(true);control.setOnClickListener(v->action.run());return control;}
    private void updateChatSend(){if(chatSendButton==null||chatInput==null)return;boolean enabled=!working&&(!chatInput.getText().toString().trim().isEmpty()||!chatFiles.isEmpty());chatSendButton.setEnabled(enabled);chatSendButton.setAlpha(enabled?1f:.35f);}
    private void updateChatModel(JSONObject settings){if(chatModelView==null||!chatModelView.isAttachedToWindow())return;String label;if("knowledge".equals(chatMode)&&(!settings.optBoolean("cloud_search_enabled",true)||!settings.optBoolean("ai_enabled")))label="本地笔记回顾";else if(settings.optBoolean("ai_enabled")){int index=Arrays.asList(AI_IDS).indexOf(str(settings,"ai_provider"));label=index>0?AI_NAMES[index]:"AI 对话";}else label="配置对话模型";chatModelView.setText(label+"  ⌄");}
    private void drawChatMessages(){
        if(chatMessageList==null||!chatMessageList.isAttachedToWindow())return;
        chatMessageList.removeAllViews();chatMessageList.setGravity(chatMessages.isEmpty()?Gravity.CENTER:Gravity.TOP);
        if(chatMessages.isEmpty()){TextView mark=text(chatMessageList,"临",31,teal,false);mark.setGravity(Gravity.CENTER);TextView title=text(chatMessageList,"knowledge".equals(chatMode)?"想回顾哪份笔记？":"有什么可以帮你？",23,ink,true);title.setGravity(Gravity.CENTER);return;}
        for(JSONObject message:chatMessages){
            boolean user="user".equals(str(message,"role"));LinearLayout line=row();line.setGravity(user?Gravity.END:Gravity.TOP);LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(24);chatMessageList.addView(line,lp);
            if(user){TextView bubble=new TextView(this);bubble.setText(str(message,"content"));bubble.setTextSize(15);bubble.setTextColor(ink);bubble.setLineSpacing(dp(5),1);bubble.setPadding(dp(15),dp(11),dp(15),dp(11));bubble.setBackground(bg(Color.rgb(237,243,242),18,null));bubble.setMaxWidth(getResources().getDisplayMetrics().widthPixels-dp(84));bubble.setTextIsSelectable(true);line.addView(bubble,new LinearLayout.LayoutParams(-2,-2));continue;}
            TextView avatar=chip("临",teal,Color.WHITE);avatar.setTextSize(18);avatar.setGravity(Gravity.CENTER);avatar.setPadding(0,0,0,0);avatar.setBackground(bg(Color.WHITE,9,border));line.addView(avatar,new LinearLayout.LayoutParams(dp(29),dp(31)));
            LinearLayout content=column();LinearLayout.LayoutParams cp=new LinearLayout.LayoutParams(0,-2,1);cp.leftMargin=dp(11);line.addView(content,cp);
            if("local".equals(str(message,"provider")))text(content,"本地原文摘录",10,muted,false);
            TextView answer=text(content,"",15,ink,false);answer.setText(ChatText.format(str(message,"content")));answer.setLineSpacing(dp(6),1);answer.setTextIsSelectable(true);answer.setLinksClickable(true);answer.setMovementMethod(android.text.method.LinkMovementMethod.getInstance());
            if(array(message,"sources").length()>0){text(content,"笔记来源",11,muted,false);for(JSONObject source:objects(array(message,"sources"))){Button open=filterChip(str(source,"title")+"  ↗",false,()->showNote(id(source)));open.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);open.setTextSize(12);open.setMaxLines(2);open.setBackground(bg(soft,10,border));LinearLayout.LayoutParams sp=new LinearLayout.LayoutParams(-1,-2);sp.bottomMargin=dp(7);content.addView(open,sp);}}
            Button copy=filterChip("复制",false,()->{android.content.ClipboardManager clipboard=(android.content.ClipboardManager)getSystemService(CLIPBOARD_SERVICE);clipboard.setPrimaryClip(android.content.ClipData.newPlainText("AI 回答",str(message,"content")));toast("已复制回答");});copy.setContentDescription("复制回答");copy.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);copy.setTextSize(11);content.addView(copy,new LinearLayout.LayoutParams(-2,dp(38)));
        }
        if(chatSending){LinearLayout pending=row();ProgressBar progress=new ProgressBar(this);progress.setIndeterminateTintList(ColorStateList.valueOf(teal));pending.addView(progress,new LinearLayout.LayoutParams(dp(17),dp(17)));TextView label=text(pending,"正在回答",12,muted,false);label.setPadding(dp(12),dp(8),0,0);chatMessageList.addView(pending);}
    }
    private void chatScreen(){
        screenHost.removeAllViews();LinearLayout chatRoot=column();chatRoot.setBackgroundColor(Color.WHITE);screenHost.addView(chatRoot,new LinearLayout.LayoutParams(-1,-1));body=chatRoot;
        LinearLayout toolbar=row();toolbar.setPadding(dp(20),dp(5),dp(14),dp(5));chatModelView=new TextView(this);chatModelView.setText("knowledge".equals(chatMode)?"笔记回顾  ⌄":"日常对话  ⌄");chatModelView.setTextSize(13);chatModelView.setTextColor(ink);chatModelView.setGravity(Gravity.CENTER_VERTICAL);chatModelView.setContentDescription("对话模型设置");chatModelView.setOnClickListener(v->{if(!working){settingsSection="models";navigateScreen("设置");}});toolbar.addView(chatModelView,new LinearLayout.LayoutParams(0,dp(44),1));toolbar.addView(chatIcon("历史","历史对话",false,this::showHistory),new LinearLayout.LayoutParams(dp(44),dp(44)));chatRoot.addView(toolbar);
        ScrollView messages=new ScrollView(this);mainScroll=messages;messages.setFillViewport(true);messages.setVerticalScrollBarEnabled(false);chatMessageList=column();chatMessageList.setPadding(dp(18),dp(20),dp(18),dp(20));messages.addView(chatMessageList);chatRoot.addView(messages,new LinearLayout.LayoutParams(-1,0,1));
        LinearLayout composer=column();composer.setPadding(dp(14),dp(9),dp(14),dp(7));composer.setBackground(bg(Color.WHITE,21,Color.rgb(204,222,213)));LinearLayout.LayoutParams composerParams=new LinearLayout.LayoutParams(-1,-2);composerParams.setMargins(dp(12),dp(4),dp(12),dp(9));chatRoot.addView(composer,composerParams);
        chatInput=new EditText(this);chatInput.setTag("chat-input");chatInput.setContentDescription("输入对话内容");chatInput.setText(chatDraft);chatInput.setHint("knowledge".equals(chatMode)?"输入问题，回顾笔记":"输入问题");chatInput.setHintTextColor(muted);chatInput.setTextColor(ink);chatInput.setTextSize(15);chatInput.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_FLAG_MULTI_LINE|InputType.TYPE_TEXT_FLAG_CAP_SENTENCES);chatInput.setMinLines(2);chatInput.setMaxLines(5);chatInput.setGravity(Gravity.TOP);chatInput.setPadding(dp(2),dp(5),dp(2),dp(7));chatInput.setBackgroundColor(Color.TRANSPARENT);chatInput.setImeOptions(android.view.inputmethod.EditorInfo.IME_ACTION_SEND);composer.addView(chatInput,new LinearLayout.LayoutParams(-1,-2));
        TextView files=text(composer,fileSummary(chatFiles),11,teal,false);files.setVisibility(chatFiles.isEmpty()?View.GONE:View.VISIBLE);files.setOnClickListener(v->selectedMaterials(chatFiles,files));files.setMaxLines(2);files.setEllipsize(android.text.TextUtils.TruncateAt.END);files.setContentDescription("已选对话材料");
        LinearLayout options=row();for(String mode:new String[]{"knowledge","general"}){Button choice=filterChip("knowledge".equals(mode)?"笔记回顾":"日常对话",chatMode.equals(mode),()->resetChat(mode));LinearLayout.LayoutParams mp=new LinearLayout.LayoutParams(-2,dp(36));mp.rightMargin=dp(5);options.addView(choice,mp);}if("knowledge".equals(chatMode)){LinearLayout cloud=column();LinearLayout.LayoutParams cloudParams=new LinearLayout.LayoutParams(0,-2,1);cloudParams.leftMargin=dp(5);options.addView(cloud,cloudParams);cloudSearchControl(cloud);if(cloud.getChildCount()>1)cloud.getChildAt(1).setVisibility(View.GONE);}else executor.execute(()->{try{JSONObject settings=(JSONObject)api.request("/settings");runOnUiThread(()->{if(chatRoot.isAttachedToWindow())updateChatModel(settings);});}catch(Exception ignored){}});composer.addView(options);
        LinearLayout actions=row();actions.addView(chatIcon("附件","添加图片或材料",false,()->{if(!working)pickFile(f->{addChatFile(f,files);updateChatSend();});}),new LinearLayout.LayoutParams(dp(42),dp(42)));actions.addView(chatIcon("录音","录音输入",false,()->{if(!working)record(f->{addChatFile(f,files);updateChatSend();});}),new LinearLayout.LayoutParams(dp(42),dp(42)));View space=new View(this);actions.addView(space,new LinearLayout.LayoutParams(0,1,1));chatSendButton=chatIcon("发送","发送消息",true,()->sendChatMessage(files));actions.addView(chatSendButton,new LinearLayout.LayoutParams(dp(39),dp(39)));composer.addView(actions);
        TextView error=text(chatRoot,chatError,11,Color.rgb(164,80,67),false);error.setTag("chat-error");error.setPadding(dp(22),0,dp(22),0);error.setVisibility(chatError.isEmpty()?View.GONE:View.VISIBLE);
        chatInput.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence v,int a,int b,int c){}public void onTextChanged(CharSequence v,int a,int b,int c){chatDraft=v.toString();updateChatSend();}public void afterTextChanged(Editable e){}});chatInput.setOnEditorActionListener((view,action,event)->{if(action==android.view.inputmethod.EditorInfo.IME_ACTION_SEND){sendChatMessage(files);return true;}return false;});
        drawChatMessages();updateChatSend();if(!chatMessages.isEmpty())messages.post(()->messages.fullScroll(View.FOCUS_DOWN));
    }
    private void sendChatMessage(TextView files){
        if(working||chatSending)return;if(recorder!=null){toast("请先结束录音");return;}
        String typed=chatInput.getText().toString().trim();String value=typed.isEmpty()&&!chatFiles.isEmpty()?("knowledge".equals(chatMode)?"请根据所附图片或材料检索相关历史记录。":"请描述所附图片或材料中的可见内容。"):typed;if(value.isEmpty())return;
        JSONObject data=obj("message",value,"mode",chatMode,"note_id",recallNoteId,"attachment_ids",ids(chatFiles));if(conversationId!=null)put(data,"conversation_id",conversationId);JSONObject optimistic=obj("role","user","content",value);chatMessages.add(optimistic);chatSending=true;chatError="";TextView error=body.findViewWithTag("chat-error");if(error!=null)error.setVisibility(View.GONE);chatInput.setEnabled(false);chatSendButton.setEnabled(false);chatSendButton.setAlpha(.35f);android.view.inputmethod.InputMethodManager keyboard=(android.view.inputmethod.InputMethodManager)getSystemService(INPUT_METHOD_SERVICE);if(keyboard!=null)keyboard.hideSoftInputFromWindow(chatInput.getWindowToken(),0);chatInput.clearFocus();drawChatMessages();mainScroll.post(()->mainScroll.fullScroll(View.FOCUS_DOWN));
        work(()->(JSONObject)api.request("/chat","POST",data),reply->{chatSending=false;chatDraft="";chatInput.setText("");chatInput.setEnabled(true);if(array(reply,"warnings").length()>0)toast(String.join("；",strings(array(reply,"warnings"))));chatMessages.add(put(reply,"role","assistant"));conversationId=str(reply,"conversation_id");chatFiles.clear();files.setText("");files.setVisibility(View.GONE);drawChatMessages();updateChatSend();mainScroll.post(()->{if(chatMessageList.getChildCount()>0){View last=chatMessageList.getChildAt(chatMessageList.getChildCount()-1);mainScroll.smoothScrollTo(0,Math.max(0,last.getTop()-dp(14)));}});},failure->{chatSending=false;chatMessages.remove(optimistic);chatInput.setEnabled(true);chatError="发送失败，输入内容已保留，可重试。";if(error!=null){error.setText(chatError);error.setVisibility(View.VISIBLE);}drawChatMessages();updateChatSend();});
    }
    private void confirmDeleteChat(JSONObject conversation,Consumer<Boolean> deleted){
        if(working)return;
        boolean current=id(conversation).equals(conversationId);
        String warning=current&&(!chatDraft.trim().isEmpty()||!chatFiles.isEmpty())?"当前未发送的输入也会清空。":"";
        new AlertDialog.Builder(this).setTitle("删除对话").setMessage("删除「"+str(conversation,"title")+"」及其中的聊天记录？"+warning).setNegativeButton("取消",null).setPositiveButton("删除",(d,w)->work(()->api.request("/conversations/"+id(conversation),"DELETE"),result->{
            if(current){conversationId=null;recallNoteId="";chatMessages.clear();chatFiles.clear();chatDraft="";chatError="";}
            deleted.accept(current);toast("对话已删除");
        })).show();
    }
    private void showHistory(){
        if(working)return;
        work(()->(JSONArray)api.request("/conversations"),items->{
            Panel p=panel("历史对话");EditText query=input(p.root,"搜索历史对话","");LinearLayout list=column();p.root.addView(list);Runnable[] redraw=new Runnable[1];
            redraw[0]=()->{
                list.removeAllViews();String term=query.getText().toString().trim().toLowerCase(java.util.Locale.ROOT);int count=0;
                for(JSONObject h:objects(items))if(str(h,"title").toLowerCase(java.util.Locale.ROOT).contains(term)){
                    count++;LinearLayout row=row();list.addView(row);
                    Button choice=button(str(h,"title"),false,()->{
                        Runnable open=()->work(()->(JSONObject)api.request("/conversations/"+id(h)),data->{conversationId=id(h);recallNoteId=str(data.optJSONObject("conversation"),"note_id");chatMode=str(h,"mode");chatMessages.clear();chatMessages.addAll(objects(array(data,"messages")));chatFiles.clear();chatDraft="";chatError="";p.dialog.dismiss();showScreen("对话");});
                        if(!chatDraft.trim().isEmpty()||!chatFiles.isEmpty())new AlertDialog.Builder(this).setTitle("切换对话？").setMessage("当前输入尚未发送，切换后会清空。").setNegativeButton("继续编辑",null).setPositiveButton("切换",(d,w)->open.run()).show();else open.run();
                    });
                    choice.setGravity(Gravity.START|Gravity.CENTER_VERTICAL);choice.setTextSize(13);choice.setMaxLines(2);row.addView(choice,new LinearLayout.LayoutParams(0,dp(48),1));
                    Runnable remove=()->confirmDeleteChat(h,current->{
                        for(int i=items.length()-1;i>=0;i--)if(id(h).equals(str(items.optJSONObject(i),"id")))items.remove(i);
                        if(current){p.dialog.dismiss();showScreen("对话");}else redraw[0].run();
                    });
                    choice.setOnLongClickListener(v->{remove.run();return true;});
                    Button delete=filterChip("删除",false,remove);delete.setTextColor(Color.rgb(163,77,67));delete.setContentDescription("删除对话："+str(h,"title"));row.addView(delete,new LinearLayout.LayoutParams(dp(56),dp(44)));
                    text(list,str(h,"created_at").split("T")[0]+" · "+("knowledge".equals(str(h,"mode"))?"笔记回顾":"日常对话"),10,muted,false);
                }
                if(count==0)text(list,term.isEmpty()?"暂无历史对话":"没有找到对话",12,muted,false);
            };
            query.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence v,int a,int b,int c){}public void onTextChanged(CharSequence v,int a,int b,int c){redraw[0].run();}public void afterTextChanged(Editable e){}});redraw[0].run();
        });
    }
    private static final String[] AI_IDS={"none","deepseek","glm","kimi","minimax"};
    private static final String[] AI_NAMES={"关闭","DeepSeek","智谱 GLM","Kimi","MiniMax"};
    private Spinner aiSelector(LinearLayout root,String label,String selected){
        text(root,label,12,muted,true);Spinner spinner=new Spinner(this);
        ArrayAdapter<String> adapter=new ArrayAdapter<>(this,android.R.layout.simple_spinner_item,AI_NAMES);
        adapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item);spinner.setAdapter(adapter);
        int index=Arrays.asList(AI_IDS).indexOf(selected);spinner.setSelection(Math.max(0,index));
        spinner.setPadding(dp(12),dp(10),dp(12),dp(10));spinner.setBackground(bg(Color.WHITE,9,border));
        LinearLayout.LayoutParams params=new LinearLayout.LayoutParams(-1,dp(52));params.bottomMargin=dp(16);root.addView(spinner,params);return spinner;
    }
    private void editAIProvider(String id,String name,JSONObject providers,JSONObject pending,TextView state){
        JSONObject p=providers.optJSONObject(id), saved=pending.optJSONObject(id);
        Panel panel=panel(name+" · API 配置");LinearLayout root=panel.root;
        text(root,"密钥仅保存在后端。留空会保留原有密钥，模型名称请以服务商提供的为准。",12,muted,false);
        EditText key=input(root,"API 密钥",saved==null?"":str(saved,"api_key"));key.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);key.setHint(p!=null&&p.optBoolean("has_key")?"已保存，留空保持不变":"请输入 API 密钥");
        EditText chat=input(root,"对话模型",saved==null?str(p,"chat_model"):str(saved,"chat_model"));
        EditText vision=input(root,"图片模型",saved==null?str(p,"vision_model"):str(saved,"vision_model"));
        CheckBox clear=new CheckBox(this);clear.setText("清除已保存的密钥");clear.setTextColor(muted);clear.setChecked(saved!=null&&saved.optBoolean("clear_key"));root.addView(clear);
        addButton(root,"完成",true,()->{String value=key.getText().toString().trim();if(clear.isChecked()&&!value.isEmpty()){toast("不能同时替换和清除密钥");return;}
            put(pending,id,obj("api_key",value,"chat_model",chat.getText().toString().trim(),"vision_model",vision.getText().toString().trim(),"clear_key",clear.isChecked()));
            settingsDirty=true;key.setText("");state.setText(name+" · 已修改，保存后生效");panel.dialog.dismiss();});
    }
    private void editASR(JSONObject config,JSONObject pending,TextView state){
        JSONObject asr=config.optJSONObject("asr"), saved=pending.optJSONObject("asr");Panel panel=panel("音频转写 · API 配置");LinearLayout root=panel.root;
        text(root,"使用兼容音频转写接口的服务，地址填写 HTTPS 基础地址。",12,muted,false);
        EditText url=input(root,"服务地址",saved==null?str(asr,"base_url"):str(saved,"base_url"));url.setHint("https://服务地址/v1");url.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_URI);
        EditText key=input(root,"API 密钥",saved==null?"":str(saved,"api_key"));key.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);key.setHint(asr.optBoolean("has_key")?"已保存，留空保持不变":"输入 API 密钥");
        EditText model=input(root,"转写模型",saved==null?str(asr,"model"):str(saved,"model"));
        CheckBox clear=new CheckBox(this);clear.setText("清除已保存的密钥");clear.setTextColor(muted);clear.setChecked(saved!=null&&saved.optBoolean("clear_key"));root.addView(clear);
        addButton(root,"完成",true,()->{String value=key.getText().toString().trim();if(clear.isChecked()&&!value.isEmpty()){toast("不能同时替换和清除密钥");return;}
            put(pending,"asr",obj("api_key",value,"base_url",url.getText().toString().trim(),"model",model.getText().toString().trim(),"clear_key",clear.isChecked()));
            settingsDirty=true;key.setText("");state.setText("音频转写 · 已修改，保存后生效");panel.dialog.dismiss();});
    }
    private void showTaxonomyManager(){
        if(working)return;work(()->(JSONObject)api.request("/taxonomy"),catalog->{applyLearningCatalog(catalog);new TaxonomyManager(catalog);});
    }
    private final class TaxonomyManager {
        JSONObject catalog;String kind="type";final Panel panel;final EditText name,query;final LinearLayout list;final Button types,tags,add;boolean changed=false;
        TaxonomyManager(JSONObject initial){
            catalog=initial;panel=panel("分类与标签");LinearLayout tabs=row();types=filterChip("资料类型",true,()->select("type"));tags=filterChip("标签",false,()->select("tag"));tabs.addView(types,new LinearLayout.LayoutParams(0,dp(44),1));tabs.addView(tags,new LinearLayout.LayoutParams(0,dp(44),1));panel.root.addView(tabs);
            name=input(panel.root,"新增名称","");name.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(40)});add=addButton(panel.root,"新增资料类型",true,()->mutate("create","",name.getText().toString(),null));
            query=input(panel.root,"查找名称","");list=column();panel.root.addView(list);
            panel.dialog.dirty=()->!name.getText().toString().trim().isEmpty();panel.dialog.setOnDismissListener(d->{if(changed&&!"设置".equals(screen))showScreen(screen);});
            query.addTextChangedListener(new TextWatcher(){public void beforeTextChanged(CharSequence v,int st,int n,int after){}public void onTextChanged(CharSequence v,int st,int before,int n){draw();}public void afterTextChanged(Editable e){}});draw();
        }
        void select(String value){if(working)return;if(!name.getText().toString().trim().isEmpty()){toast("请先保存新增名称或清空输入");return;}kind=value;query.setText("");draw();}
        void draw(){
            list.removeAllViews();boolean type="type".equals(kind);types.setBackground(bg(type?mint:Color.WHITE,9,border));tags.setBackground(bg(type?Color.WHITE:mint,9,border));add.setText(type?"新增资料类型":"新增标签");String term=query.getText().toString().trim().toLowerCase(java.util.Locale.ROOT);int count=0;
            for(JSONObject item:objects(array(catalog,type?"note_types":"tags"))){String title=str(item,type?"title":"name"),key=str(item,type?"id":"name");if(!title.toLowerCase(java.util.Locale.ROOT).contains(term))continue;count++;LinearLayout entry=column();list.addView(entry);LinearLayout line=row(),info=column();text(info,title,14,ink,true);TextView meta=text(info,item.optInt("count")+" 篇",11,muted,false);line.addView(info,new LinearLayout.LayoutParams(0,-2,1));Button rename=button("重命名",false,()->rename(key,title));rename.setTextSize(11);rename.setContentDescription("重命名"+(type?"资料类型：":"标签：")+title);line.addView(rename,new LinearLayout.LayoutParams(dp(64),dp(44)));if(type&&"other".equals(key)){TextView reserved=new TextView(MainActivity.this);reserved.setText("保留类型");reserved.setTextSize(10);reserved.setTextColor(muted);line.addView(reserved,new LinearLayout.LayoutParams(dp(56),-2));}else{Button delete=filterChip("删除",false,()->{String fallback="其他资料";for(JSONObject t:objects(array(catalog,"note_types")))if("other".equals(str(t,"id")))fallback=str(t,"title");String warning=type?"删除后解除此类型；没有其他类型的笔记归入「"+fallback+"」。":"删除后，从 "+item.optInt("count")+" 篇笔记移除此标签。";new AlertDialog.Builder(MainActivity.this).setTitle("删除「"+title+"」？").setMessage(warning+"笔记正文和材料保留。").setNegativeButton("取消",null).setPositiveButton("删除",(d,w)->mutate("delete",key,"",null)).show();});delete.setTextColor(Color.rgb(163,77,67));delete.setContentDescription("删除"+(type?"资料类型：":"标签：")+title);line.addView(delete,new LinearLayout.LayoutParams(dp(48),dp(44)));}entry.addView(line);divider(entry,5,10);}
            if(count==0)text(list,"没有找到"+(type?"资料类型":"标签"),12,muted,false);
        }
        void rename(String key,String title){if(working)return;EditText input=new EditText(MainActivity.this);input.setText(title);input.setTextColor(ink);input.setSelectAllOnFocus(true);input.setContentDescription("新名称");input.setSingleLine(true);input.setFilters(new android.text.InputFilter[]{new android.text.InputFilter.LengthFilter(40)});AlertDialog dialog=new AlertDialog.Builder(MainActivity.this).setTitle("重命名").setView(input).setNegativeButton("取消",null).setPositiveButton("保存",null).create();dialog.show();dialog.getButton(AlertDialog.BUTTON_POSITIVE).setOnClickListener(v->mutate("rename",key,input.getText().toString(),dialog));}
        void mutate(String action,String id,String value,AlertDialog edit){if(working)return;work(()->(JSONObject)api.request("/taxonomy","POST",obj("kind",kind,"action",action,"id",id,"name",value,"expected_version",str(catalog,"version"))),updated->{catalog=updated;applyLearningCatalog(updated);lastSearch=null;searchCategories.clear();searchTags.clear();browseTypes.clear();browseTags.clear();changed=true;if("create".equals(action))name.setText("");if(edit!=null)edit.dismiss();draw();toast("已保存");});}
    }
    private void switchSettingsSection(String section){
        if(working||section.equals(settingsSection))return;Runnable change=()->{settingsSection=section;showScreen("设置");};if(settingsDirty)new AlertDialog.Builder(this).setTitle("模型设置尚未保存").setMessage("离开后会放弃这些修改。").setNegativeButton("继续编辑",null).setPositiveButton("放弃修改",(d,w)->change.run()).show();else change.run();
    }
    private void exportNotes(){work(()->saveExport("clinical-notes-"+System.currentTimeMillis()+".json",api.download("/export")),file->{Intent share=new Intent(Intent.ACTION_SEND).setType("application/json").putExtra(Intent.EXTRA_STREAM,exportUri(file)).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);startActivity(Intent.createChooser(share,"保存 / 分享笔记"));});}
    private void logoutAccount(){if(working)return;work(()->api.request("/auth/logout","POST"),v->{api.token="";tokenStore.save("");queryFiles.clear();chatFiles.clear();chatMessages.clear();conversationId=null;recallNoteId="";settingsSection="account";clearExports();showAuth();});}
    private void accountSettingsScreen(){
        work(()->(JSONObject)api.request("/auth/me"),user->{displayName=str(user,"display_name");if(status!=null)status.setText(displayName+" · 点击同步");LinearLayout identity=card(body),identityRow=row();TextView avatar=chip(displayName.substring(0,1),teal,mint);avatar.setTextSize(21);avatar.setGravity(Gravity.CENTER);identityRow.addView(avatar,new LinearLayout.LayoutParams(dp(44),dp(44)));LinearLayout info=column();text(info,displayName,18,ink,true);text(info,"登录用户名："+str(user,"username"),12,muted,false);LinearLayout.LayoutParams infoParams=new LinearLayout.LayoutParams(0,-2,1);infoParams.leftMargin=dp(13);identityRow.addView(info,infoParams);identity.addView(identityRow);addButton(identity,"编辑显示名称",()->editProfile(user));
            LinearLayout connection=card(body);text(connection,"当前连接",15,ink,true);text(connection,"同一服务内，所有账户共享笔记、类型和标签。",11,muted,false);TextView address=text(connection,api.baseUrl,12,ink,false);address.setTextIsSelectable(true);addButton(connection,"复制服务地址",()->{android.content.ClipboardManager clipboard=(android.content.ClipboardManager)getSystemService(CLIPBOARD_SERVICE);clipboard.setPrimaryClip(android.content.ClipData.newPlainText("服务地址",api.baseUrl));toast("地址已复制");});
            LinearLayout data=card(body);text(data,"资料导出",15,ink,true);text(data,"导出笔记 JSON，不含附件原件。",11,muted,false);addButton(data,"导出笔记",this::exportNotes);LinearLayout login=card(body);text(login,"登录状态",15,ink,true);text(login,"当前账户："+str(user,"username"),11,muted,false);Button logout=addButton(login,"退出登录",this::logoutAccount);logout.setTextColor(Color.rgb(163,77,67));
        });
    }
    private void settingsScreen(){
        pageHeading("设置与服务","");LinearLayout tabs=row();for(String[] tab:new String[][]{{"account","个人账户"},{"models","模型与检索"},{"library","分类与标签"}}){Button choice=filterChip(tab[1],settingsSection.equals(tab[0]),()->switchSettingsSection(tab[0]));choice.setTextSize(11);tabs.addView(choice,new LinearLayout.LayoutParams(0,dp(44),1));}body.addView(tabs);divider(body,10,18);
        if("account".equals(settingsSection)){accountSettingsScreen();return;}
        if("library".equals(settingsSection)){work(()->(JSONObject)api.request("/taxonomy"),catalog->{applyLearningCatalog(catalog);LinearLayout card=card(body);text(card,"分类与标签",17,ink,true);text(card,learningTypes.length()+" 个资料类型 · "+learningTags.length()+" 个标签",12,muted,false);addButton(card,"管理分类与标签",true,this::showTaxonomyManager);});return;}
        work(()->obj("settings",api.request("/settings"),"config",api.request("/ai-config"),"user",api.request("/auth/me")),data->{
            JSONObject s=data.optJSONObject("settings"), config=data.optJSONObject("config"), providers=config.optJSONObject("providers"), pending=new JSONObject(), pendingASR=new JSONObject();

            LinearLayout local=card(body);text(local,"检索方式",16,ink,true);android.widget.Switch cloudSearch=new android.widget.Switch(this);cloudSearch.setText("云端 AI 辅助");cloudSearch.setTextSize(13);cloudSearch.setTextColor(teal);cloudSearch.setChecked(config.optBoolean("cloud_search_enabled",true));cloudSearch.setOnCheckedChangeListener((v,on)->settingsDirty=true);local.addView(cloudSearch,new LinearLayout.LayoutParams(-1,dp(44)));text(local,"关闭后使用本地检索与识别；日常对话仍可调用模型。",11,muted,false);text(local,"本地 OCR："+(s.optBoolean("local_ocr_languages")?"已就绪":"未就绪")+" · 音频转写："+(s.optBoolean("local_asr")?"已就绪（≤120秒）":"未就绪"),11,muted,false);
            LinearLayout selection=card(body);text(selection,"使用的模型",16,ink,true);
            Spinner chat=aiSelector(selection,"对话模型",str(config,"chat_provider"));Spinner vision=aiSelector(selection,"图片理解",str(config,"vision_provider"));
            AdapterView.OnItemSelectedListener changed=new AdapterView.OnItemSelectedListener(){public void onNothingSelected(AdapterView<?> p){}public void onItemSelected(AdapterView<?> p,View v,int position,long id){if(!AI_IDS[chat.getSelectedItemPosition()].equals(str(config,"chat_provider"))||!AI_IDS[vision.getSelectedItemPosition()].equals(str(config,"vision_provider")))settingsDirty=true;}};chat.setOnItemSelectedListener(changed);vision.setOnItemSelectedListener(changed);
            text(body,"服务配置",16,ink,true);
            for(int i=1;i<AI_IDS.length;i++){final String id=AI_IDS[i],name=AI_NAMES[i];JSONObject provider=providers.optJSONObject(id);LinearLayout c=card(body);
                LinearLayout service=row();LinearLayout info=column();TextView state=text(info,name,14,ink,false);TextView keyState=text(info,provider!=null&&provider.optBoolean("has_key")?"已配置":"未配置",11,muted,false);((LinearLayout.LayoutParams)keyState.getLayoutParams()).bottomMargin=0;service.addView(info,new LinearLayout.LayoutParams(0,-2,1));Button configure=button("配置",false,()->editAIProvider(id,name,providers,pending,state));service.addView(configure,new LinearLayout.LayoutParams(-2,dp(42)));c.addView(service);
            }
            LinearLayout audio=card(body);LinearLayout audioRow=row();TextView audioState=new TextView(this);audioState.setText("音频转写");audioState.setTextSize(15);audioState.setTextColor(ink);audioRow.addView(audioState,new LinearLayout.LayoutParams(0,-2,1));audioRow.addView(button("配置",false,()->editASR(config,pendingASR,audioState)),new LinearLayout.LayoutParams(-2,dp(42)));audio.addView(audioRow);
            LinearLayout imageIndex=card(body);text(imageIndex,"图片检索索引",15,ink,true);text(imageIndex,"使用已保存的图片模型更新历史图片索引，保留人工修订的文字。",12,muted,false);
            addButton(imageIndex,"更新图片索引",()->{if(settingsDirty){toast("请先保存模型设置");return;}work(()->{String after="";int indexed=0,failed=0;boolean more=true;while(more){JSONObject result=(JSONObject)api.request("/image-index","POST",obj("after",after,"limit",2));indexed+=result.optInt("indexed");failed+=array(result,"failed_ids").length();after=str(result,"after");more=result.optBoolean("has_more");}return "索引已更新："+indexed+" 张可用，"+failed+" 张失败";},value->toast(String.valueOf(value)));});
            text(body,"云端识别会发送材料，请先脱敏。模型调用可能产生费用。",11,muted,false);
            addButton(body,"保存设置",true,()->{JSONObject payload=obj("cloud_search_enabled",cloudSearch.isChecked(),"expected_version",str(config,"version"),"chat_provider",AI_IDS[chat.getSelectedItemPosition()],"vision_provider",AI_IDS[vision.getSelectedItemPosition()],"providers",pending,"asr",pendingASR.optJSONObject("asr"));
                work(()->api.request("/ai-config","PUT",payload),updated->{toast("模型设置已保存，立即生效");showScreen("设置");});});

        });
    }
    private void pickFile(Consumer<JSONObject> done){if(working){toast("请等待当前操作完成");return;}onPicked=done;Intent i=new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*");i.putExtra(Intent.EXTRA_MIME_TYPES,new String[]{"image/*","audio/*","application/pdf","text/plain","text/markdown"});startActivityForResult(i,101);}
    private void pickEditorFiles(int capacity,java.util.function.BiConsumer<List<JSONObject>, Boolean> done,Runnable started) {
        if (working) return;
        if (capacity <= 0) {
            toast("最多关联 20 份材料");
            return;
        }
        batchCapacity = capacity;
        onBatchPicked = done;
        onBatchUploadStart = started;
        Intent picker = new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*");
        picker.putExtra(Intent.EXTRA_ALLOW_MULTIPLE,true);
        picker.putExtra(Intent.EXTRA_MIME_TYPES,new String[]{"image/*","audio/*","application/pdf","text/plain","text/markdown","text/csv",
                "application/vnd.openxmlformats-officedocument.wordprocessingml.document"});
        startActivityForResult(picker,103);
    }

    @Override protected void onActivityResult(int requestCode,int resultCode,Intent data) {
        super.onActivityResult(requestCode,resultCode,data);
        if (requestCode == 103) {
            java.util.function.BiConsumer<List<JSONObject>, Boolean> done = onBatchPicked;
            onBatchPicked = null;
            Runnable started = onBatchUploadStart;
            onBatchUploadStart = null;
            if (resultCode != RESULT_OK || data == null || done == null) return;
            List<Uri> selected = new ArrayList<>();
            android.content.ClipData clip = data.getClipData();
            if (clip != null) {
                if (clip.getItemCount() > batchCapacity) {
                    toast("最多可再关联 "+batchCapacity+" 份材料，请重新选择");
                    return;
                }
                for (int i=0;i<clip.getItemCount();i++) selected.add(clip.getItemAt(i).getUri());
            } else if (data.getData() != null) selected.add(data.getData());
            if (selected.isEmpty()) return;
            if (started != null) started.run();
            List<JSONObject> uploaded = new ArrayList<>();
            List<String> failures = new ArrayList<>();
            work(()->{
                for (Uri uri:selected) {
                    try { uploaded.add(readPickedFile(uri)); }
                    catch (Exception error) { failures.add(error.getMessage()); }
                }
                return uploaded;
            },files->{
                toast(failures.isEmpty() ? "已关联 "+files.size()+" 份材料" :
                        "已关联 "+files.size()+" 份，"+failures.size()+" 份失败："+failures.get(0));
                done.accept(files,failures.isEmpty());
            });
            return;
        }
        if (requestCode != 101) return;
        pickingSearchMaterial = false;
        Consumer<JSONObject> done = onPicked;
        onPicked = null;
        if (resultCode == RESULT_OK && data != null && data.getData() != null && done != null)
            uploadPickedFile(data.getData(),done);
    }

    private JSONObject readPickedFile(Uri uri) throws Exception {
        String name = "material.txt";
        try (Cursor cursor = getContentResolver().query(uri,new String[]{OpenableColumns.DISPLAY_NAME},null,null,null)) {
            if (cursor != null && cursor.moveToFirst()) name = cursor.getString(0);
        }
        java.io.InputStream stream = getContentResolver().openInputStream(uri);
        if (stream == null) throw new IllegalArgumentException("无法读取文件");
        return api.upload(name,Api.readBytes(stream,20*1024*1024));
    }

    private void uploadPickedFile(Uri uri,Consumer<JSONObject> done) {
        if (isDestroyed() || isFinishing()) return;
        if (working) {
            handler.postDelayed(()->uploadPickedFile(uri,done),150);
            return;
        }
        work(()->readPickedFile(uri),file->{
            toast(file.optBoolean("demo_match")?"已匹配演示文件，载入预置文本":
                    "ready".equals(str(file,"status"))?"材料已保存并提取文本":"材料已保存，请核对或补充识别文本");
            done.accept(file);
        });
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
    @Override protected void onStop(){handler.removeCallbacks(syncLoop);super.onStop();if(recorder!=null){cancelRecording();toast("录音已取消，可返回后重新录制");}}
    @Override public void onBackPressed(){
        android.webkit.WebView web=anatomyWeb;
        if(web==null){super.onBackPressed();return;}
        web.evaluateJavascript("(()=>{const dialog=document.querySelector('.ml-capture[open]')||document.querySelector('.ml-fullscreen[open],.a-fullscreen[open]');if(dialog){dialog.close();return true;}const panel=document.querySelector('.model-local:not([hidden]) .atlas-side:not([hidden]) .a-side-head button,.model-local:not([hidden]) .a-channels-open .a-rail-close');if(!panel)return false;panel.click();return true;})()",closed->{
            if(anatomyWeb==web&&!"true".equals(closed))navigateScreen("检索");
        });
    }
    @Override protected void onDestroy(){if(anatomyWeb!=null){anatomyWeb.destroy();anatomyWeb=null;}cancelRecording();executor.shutdownNow();super.onDestroy();}
}
