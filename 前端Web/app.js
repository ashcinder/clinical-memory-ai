"use strict";
const APP_BASE = new URL(".", document.currentScript.src).pathname.replace(
  /\/$/,
  "",
);
const siteURL = (path) => APP_BASE + path;
const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];
const esc = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const state = {
  view: "search",
  user: null,
  settings: null,
  settingsTab: "account",
  stats: null,
  notes: [],
  total: 0,
  categories: [],
  tagFilters: [],
  scopeTypes: [],
  scopeTags: [],
  taxonomy: null,
  results: null,
  searchFiles: [],
  chatFiles: [],
  conversation: null,
  mode: "knowledge",
  messages: [],
  chatDraft: "",
  busy: false,
  searching: false,
  chatting: false,
  resultSort: "relevance",
};
const titles = {
  search: "经验检索",
  notes: "学习资料",
  materials: "材料归档",
  chat: "AI 对话",
  settings: "服务设置",
};
const ACCEPT =
  ".jpg,.jpeg,.png,.webp,.pdf,.txt,.md,.wav,.mp3,.m4a,.webm,.ogg,.aac,.3gp";
let initialized = false,
  toastTimer,
  recorder,
  recordingChunks = [],
  recordingTarget,
  recordingStream,
  recordTimeout;
let syncVersion = "",
  syncBusy = false,
  pageLoading = false,
  syncNotice = false;
const pendingUploads = { search: [], chat: [] };
let libraryRequest = 0;
let sidebarCollapsed = false, chatHistoryCollapsed = false;
let chatHistoryItems = [];
const deletedConversations = new Set();
let conversationDeleting = false;
let profileSaving = false;
let modelSettingsSaving = false;
try {
  sidebarCollapsed = localStorage.getItem("linji.sidebar.collapsed") === "true";
  chatHistoryCollapsed = localStorage.getItem("linji.chat.historyCollapsed") === "true";
} catch {}

const defaultTypes = [
  {
    id: "record",
    title: "诊疗记录",
    aliases: ["诊疗记录", "诊疗笔记", "临床记录", "病例记录"],
    content_label: "诊疗记录",
    steps_label: "已记录过程",
  },
  {
    id: "tutorial",
    title: "操作教程",
    aliases: ["操作教程", "教程", "操作流程", "技能教程", "临床技能"],
    content_label: "教程原文",
    steps_label: "操作步骤",
  },
  {
    id: "knowledge",
    title: "知识要点",
    aliases: ["知识要点", "知识", "知识笔记", "理论知识"],
    content_label: "知识原文",
    steps_label: "学习要点",
  },
  {
    id: "review",
    title: "经验复盘",
    aliases: ["经验复盘", "经验总结", "复盘", "学习心得"],
    content_label: "复盘原文",
    steps_label: "复盘条目",
  },
  {
    id: "followup",
    title: "随访记录",
    aliases: ["随访记录", "随访", "复诊记录"],
    content_label: "随访原文",
    steps_label: "随访过程",
  },
  {
    id: "atlas",
    title: "图谱资料",
    aliases: ["图谱资料", "图谱", "图片资料", "解剖图谱"],
    content_label: "资料说明",
    steps_label: "核对要点",
  },
  {
    id: "other",
    title: "其他资料",
    aliases: [],
    content_label: "原文内容",
    steps_label: "已记录条目",
  },
];
function catalogTypes() {
  return state.taxonomy?.note_types || defaultTypes;
}
function typeOf(note) {
  return (
    catalogTypes().find((t) => t.id === note.note_type) ||
    catalogTypes().find((t) => t.aliases.includes(note.category)) ||
    catalogTypes().find((t) => t.id === "other")
  );
}
function typesOf(note) { const ids=note.note_types?.length?note.note_types:[typeOf(note).id];return ids.map((id)=>catalogTypes().find((t)=>t.id===id)).filter(Boolean); }
function typeBadges(note) { return typesOf(note).map((t)=>`<span class="category-label">${esc(t.title)}</span>`).join(""); }
function toggled(values,value) { return values.includes(value)?values.filter((v)=>v!==value):[...values,value]; }
function matchesTypes(note,ids) { return !ids.length||typesOf(note).some((t)=>ids.includes(t.id)); }
function matchesTags(note,tags) { return tags.every((tag)=>(note.tags||[]).includes(tag)); }
function filterParams(types=state.categories,tags=state.tagFilters) { const params=new URLSearchParams();types.forEach((v)=>params.append("note_types",v));tags.forEach((v)=>params.append("tags",v));return params.toString(); }
function categoryChoices(current = "") {
  current ||= catalogTypes()[0]?.title || "其他资料";
  const items = catalogTypes()
    .map((t) => t.title);
  if (!items.includes(current)) items.unshift(current);
  return items
    .map(
      (v) =>
        `<option value="${esc(v)}" ${v === current ? "selected" : ""}>${esc(v)}</option>`,
    )
    .join("");
}
function topTags(notes, limit = 20) {
  const map = new Map();
  for (const n of notes)
    for (const t of new Set(n.tags || [])) map.set(t, (map.get(t) || 0) + 1);
  return [...map]
    .map(([name, count]) => ({ name, count }))
    .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name))
    .slice(0, limit);
}
function readingContent(note) {
  const e = note.search_match || {};
  let passages = e.passages || [];
  if (!passages.length) {
    const content = note.content || "";
    passages = [
      {
        field: "content",
        source: "笔记正文",
        text:
          content.length <= 800 ? content : e.excerpt || content.slice(0, 480),
        matched: false,
        truncated: content.length > 800,
      },
    ];
  }
  const chosen = passages
    .filter(
      (p) =>
        p.field !== "steps" ||
        !note.steps?.length ||
        note.steps
          .slice(6)
          .some((step) =>
            (e.terms || []).some((term) =>
              step.toLowerCase().includes(term.toLowerCase()),
            ),
          ),
    )
    .slice(0, 3);
  // A late matching step must remain visible even when it is outside the initial step list.
  if (!chosen.length && passages.length) chosen.push(passages[0]);
  const isFull = chosen.some((p) => p.field === "content" && !p.truncated);
  return (
    chosen
      .map(
        (p) =>
          `<section class="matched-content ${p.matched ? "is-match" : ""}"><div class="reading-source"><span>${p.matched ? "命中原文" : "笔记原文"}</span>${p.source && p.source !== "笔记正文" ? `<span>${esc(p.source)}</span>` : ""}${p.attachment_id ? `<button class="text-button" data-result-file="${p.attachment_id}">核对原件</button>` : ""}</div><div class="reading-text">${marked(p.text, e.terms || []) || '<span class="muted">尚无正文，请查看已保存条目。</span>'}</div></section>`,
      )
      .join("") +
    (!isFull && note.content
      ? `<details class="full-note-content"><summary>${chosen.some((p) => p.field === "content") ? "展开笔记全文" : "查看笔记正文"} <span>${[...note.content].length} 字</span></summary><div class="reading-text">${marked(note.content, e.terms || [])}</div></details>`
      : "")
  );
}
async function filterLibrary(types, tags = state.tagFilters) {
  if (state.searching || state.busy) return;
  types=Array.isArray(types)?types:types?[types]:[];tags=Array.isArray(tags)?tags:tags?[tags]:[];
  const requestID=++libraryRequest,view=state.view,previous={types:state.categories,tags:state.tagFilters};
  const trigger=document.activeElement,focus=trigger?.dataset.category!==undefined?["category",trigger.dataset.category]:trigger?.dataset.tag!==undefined?["tag",trigger.dataset.tag]:null;
  state.categories=types;state.tagFilters=tags;
  if(state.view==="notes"||!state.results){try{const data=await api("/notes?"+filterParams(types,tags));if(requestID!==libraryRequest||view!==state.view)return;state.notes=data.items;state.total=data.total;}catch(error){if(requestID===libraryRequest){state.categories=previous.types;state.tagFilters=previous.tags;toast(error.message);}return;}}
  if(requestID!==libraryRequest||view!==state.view)return;const y=scrollY;renderLibrary();if(focus){const b=$$(`[data-${focus[0]}]`).find((e)=>e.dataset[focus[0]]===focus[1]);b?.focus({preventScroll:true});}window.scrollTo(0,y);
}

const icons = {
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>',
  image:
    '<rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="8" cy="9" r="1.5"/><path d="m4 17 5-5 4 4 3-3 5 5"/>',
  mic: '<rect x="9" y="3" width="6" height="12" rx="3"/><path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3M8 22h8"/>',
  arrow: '<path d="M5 12h14m-6-6 6 6-6 6"/>',
  file: '<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9Z"/><path d="M14 3v6h6M8 13h8M8 17h5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  trash: '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v5M14 11v5"/>',
  send: '<path d="M12 19V5m-6 6 6-6 6 6"/>',
  chevron: '<path d="m8 10 4 4 4-4"/>',
  paperclip: '<path d="m8 13 7-7a3 3 0 0 1 4 4l-9 9a5 5 0 0 1-7-7l9-9"/><path d="m7 14 8-8"/>',
  history: '<rect x="4" y="4" width="16" height="16" rx="3"/><path d="M9 4v16M13 9h4M13 13h4"/>',
  copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M15 8V4H4v11h4"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
};
function icon(name) {
  return `<svg class="ui-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${icons[name] || icons.file}</svg>`;
}
let dialogDirtyCheck = null,
  dialogBackAction = null;
function allowDiscard() {
  return (
    !dialogDirtyCheck?.() || window.confirm("有尚未保存的修改，确定放弃吗？")
  );
}
let taxonomySaving = false;
function closeDialog(force = false) {
  if (taxonomySaving) return toast("正在保存分类与标签，请稍候");
  if (conversationDeleting && !force) return toast("正在删除对话，请稍候");
  if (state.busy) return toast("材料正在上传，请稍候");
  if (!force && !allowDiscard()) return;
  dialogDirtyCheck = null;
  dialogBackAction = null;
  $("#dialog").close();
}
function setDialogBack(action) {
  dialogBackAction = action;
  const backButton = $("#dialog-back");
  if (backButton) backButton.hidden = !action;
}
function bindDraft(form, extra = () => "") {
  const snapshot = () =>
    JSON.stringify([...new FormData(form).entries()]) + extra();
  const initial = snapshot();
  dialogDirtyCheck = () => form.isConnected && snapshot() !== initial;
  form.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      form.requestSubmit();
    }
  });
}
function canLeavePage() {
  if (state.busy || state.searching || state.chatting || conversationDeleting || taxonomySaving || profileSaving || modelSettingsSaving) {
    toast("请等待当前操作完成");
    return false;
  }
  const dirty =
    $("#ai-settings-form")?.dataset.dirty === "true" ||
    $("#profile-form")?.dataset.dirty === "true" ||
    !!$("#chat-input")?.value.trim();
  return !dirty || window.confirm("有尚未保存或发送的内容，确定离开吗？");
}

async function api(path, options = {}) {
  const init = {
    ...options,
    credentials: "same-origin",
    headers: { ...options.headers },
  };
  if (options.body && !(options.body instanceof FormData)) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(options.body);
  }
  let response;
  try {
    response = await fetch(siteURL("/api" + path), init);
  } catch {
    throw new Error("无法连接后端，请检查服务是否启动。");
  }
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error(`服务返回异常（${response.status}）`);
  }
  if (!response.ok) {
    if (response.status === 401 && state.user) {
      state.user = null;
      showAuth(true);
    }
    throw new Error(
      typeof data.detail === "string"
        ? data.detail
        : data.detail?.[0]?.msg || `请求失败（${response.status}）`,
    );
  }
  return data;
}
function toast(message) {
  const el = $("#toast");
  el.textContent = message;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), 5500);
}
function dateLabel(value) {
  return value ? value.slice(0, 10).replaceAll("-", ".") : "";
}
function today() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
function prettySize(size) {
  return size > 1048576
    ? (size / 1048576).toFixed(1) + " MB"
    : Math.ceil(size / 1024) + " KB";
}
function heading(eyebrow, title, subtitle) {
  return `<section class="page-heading"><h1>${esc(title)}</h1></section>`;
}
function showAuth(isInitialized) {
  document.body.classList.remove("chat-open");
  deletedConversations.clear();
  state.results = null;
  state.notes = [];
  state.searchFiles = [];
  state.chatFiles = [];
  state.messages = [];
  state.chatDraft = "";
  state.conversation = null;
  state.lastQuery = "";
  state.completedQuery = "";
  state.resultFiles = [];
  state.scopeTypes=[];state.scopeTags=[];state.completedScope=[];state.completedScopeTags=[];
  initialized = isInitialized;
  $("#workspace").hidden = true;
  $("#auth").hidden = false;
  $("#auth-title").textContent = initialized ? "登录" : "创建账户";
  $("#name-field").hidden = initialized;
  $("#auth-submit").textContent = initialized ? "登录" : "创建账户";
  $("#account-menu").open = false;
  closeConversationMenu();
  $("#password").autocomplete = initialized
    ? "current-password"
    : "new-password";
}
$("#auth-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const button = $("#auth-submit");
  button.disabled = true;
  $("#auth-error").textContent = "";
  try {
    const result = await api(initialized ? "/auth/login" : "/auth/setup", {
      method: "POST",
      body: {
        username: $("#username").value.trim(),
        password: $("#password").value,
        display_name: $("#display-name").value.trim() || "医生",
      },
    });
    state.user = result.user;
    $("#password").value = "";
    await enterWorkspace();
  } catch (e) {
    $("#auth-error").textContent = e.message;
  } finally {
    button.disabled = false;
  }
});
async function refreshData() {
  const data = await Promise.all([
    api("/settings"),
    api("/stats"),
    api("/auth/me"),
    api("/taxonomy").catch(() => ({ note_types: defaultTypes })),
  ]);
  state.settings = data[0];
  state.stats = data[1];
  state.user = data[2];
  state.taxonomy = data[3];
  state.scopeTypes=state.scopeTypes.filter((id)=>catalogTypes().some((t)=>t.id===id));
  state.scopeTags=state.scopeTags.filter((tag)=>(state.taxonomy?.tags||[]).some((t)=>t.name===tag));
  $("#profile-name").textContent = state.user.display_name;
  $("#account-trigger").setAttribute("aria-label", "账户：" + state.user.display_name);
  $("#avatar").textContent = state.user.display_name.slice(0, 1);
}
function hasSyncDraft() {
  return (
    $("#dialog").open ||
    recorder?.state === "recording" ||
    (state.view === "search" && state.searchFiles.length > 0) ||
    (state.view === "chat" && state.chatFiles.length > 0) ||
    $("#ai-settings-form")?.dataset.dirty === "true" ||
    $("#profile-form")?.dataset.dirty === "true" ||
    !!$("#chat-input")?.value.trim() ||
    !!$("#query")?.value.trim()
  );
}
async function checkSync(force = false) {
  if (!state.user || document.hidden || syncBusy || state.busy || state.chatting || pageLoading || profileSaving || modelSettingsSaving)
    return;
  if (
    force &&
    hasSyncDraft() &&
    !window.confirm("同步会重新载入当前页面，是否放弃尚未保存的输入？")
  )
    return;
  syncBusy = true;
  try {
    const remote = await api("/sync");
    if (!syncVersion && !force) {
      syncVersion = remote.version;
      return;
    }
    if (force || remote.version !== syncVersion) {
      if (!force && hasSyncDraft()) {
        if (!syncNotice) {
          toast("其他设备有更新，结束编辑后将同步");
          syncNotice = true;
        }
        return;
      }
      syncVersion = remote.version;
      syncNotice = false;
      if (force) {
        state.searchFiles = [];
        state.chatFiles = [];
        state.lastQuery = "";
      }
      state.results = null;
      if (state.view === "chat" && state.conversation) {
        const conversation = await api("/conversations/" + state.conversation);
        state.messages = conversation.messages;
        state.mode = conversation.conversation.mode;
      }
      await navigate(state.view);
      $("#model-badge").textContent = searchModeLabel();
      const chatModel = $("#chat-model-settings");
      if (chatModel) chatModel.innerHTML = chatEngineLabel() + icon("chevron");
      if (force) toast("已同步最新数据");
    }
  } catch {
    /* A temporarily unavailable backend must not discard the editor. */
  } finally {
    syncBusy = false;
  }
}
setInterval(() => checkSync(), 5000);
window.addEventListener("focus", () => checkSync());
document.addEventListener("visibilitychange", () => {
  if (!document.hidden) checkSync();
});
$("#sync-now").addEventListener("click", () => checkSync(true));
async function enterWorkspace() {
  await refreshData();
  $("#auth").hidden = true;
  $("#workspace").hidden = false;
  $("#profile-name").textContent = state.user.display_name;
  $("#account-trigger").setAttribute("aria-label", "账户：" + state.user.display_name);
  $("#avatar").textContent = state.user.display_name.slice(0, 1);
  $("#model-badge").textContent = searchModeLabel();
  const view = location.hash.slice(1);
  await navigate(titles[view] ? view : "search");
}
$("#logout").addEventListener("click", async () => {
  try {
    if (!canLeavePage()) return;
    $("#account-menu").open = false;
    await api("/auth/logout", { method: "POST" });
    location.reload();
  } catch (e) {
    toast(e.message);
  }
});
$$("[data-view]").forEach((button) =>
  button.addEventListener("click", () => {
    if (button.dataset.view !== state.view) navigate(button.dataset.view);
  }),
);
window.addEventListener("hashchange", () => {
  const view = location.hash.slice(1);
  if (state.user && titles[view] && view !== state.view) navigate(view);
});
$("#new-note").addEventListener("click", () => editNote());
$("#sidebar-toggle").onclick = () => {sidebarCollapsed = !sidebarCollapsed; try {localStorage.setItem("linji.sidebar.collapsed", String(sidebarCollapsed));} catch {} applySidebarState();};
$("#chat-history-toggle").onclick = () => {chatHistoryCollapsed = !chatHistoryCollapsed; try {localStorage.setItem("linji.chat.historyCollapsed", String(chatHistoryCollapsed));} catch {} applySidebarState();};
$("#account-profile").onclick = () => {$("#account-menu").open = false;openSettingsTab("account");};
document.addEventListener("pointerdown", (event) => {const menu=$("#conversation-context-menu"); if(menu&&!menu.contains(event.target))closeConversationMenu(); if(!$("#account-menu").contains(event.target))$("#account-menu").open=false;});
document.addEventListener("keydown", (event) => {if(event.key==="Escape"&&$("#conversation-context-menu")){event.preventDefault();event.stopPropagation();closeConversationMenu(true);}else if(event.key==="Escape")$("#account-menu").open=false;},true);
window.addEventListener("resize", () => closeConversationMenu());
document.addEventListener("scroll", () => closeConversationMenu(), true);
applySidebarState();
$$(".sidebar [data-view]").forEach((button) => {const name=button.textContent.trim();button.setAttribute("aria-label", name);button.title=name;});

$("#close-dialog").addEventListener("click", () => closeDialog());
$("#dialog-back")?.addEventListener("click", () => {
  if (allowDiscard()) dialogBackAction?.();
});
$("#dialog").addEventListener("cancel", (e) => {
  e.preventDefault();
  closeDialog();
});
window.addEventListener("beforeunload", (e) => {
  if (
    dialogDirtyCheck?.() ||
    state.busy ||
    state.chatting ||
    !!$("#chat-input")?.value.trim() ||
    $("#ai-settings-form")?.dataset.dirty === "true" ||
    $("#profile-form")?.dataset.dirty === "true"
  ) {
    e.preventDefault();
    e.returnValue = "";
  }
});
$("#dialog").addEventListener("click", (e) => {
  if (e.target === $("#dialog")) {
    const r = $("#dialog").getBoundingClientRect();
    if (
      e.clientX < r.left ||
      e.clientX > r.right ||
      e.clientY < r.top ||
      e.clientY > r.bottom
    )
      closeDialog();
  }
});
function openDialog(title, content, { compact = false, picker = false } = {}) {
  $("#dialog").classList.toggle("facet-picker-dialog", picker);
  $("#dialog").classList.toggle("compact-confirm", compact);
  dialogDirtyCheck = null;
  setDialogBack(null);
  $("#dialog-title").textContent = title;
  $("#dialog-content").innerHTML = content;
  $("#dialog-content").scrollTop = 0;
  if (!$("#dialog").open) $("#dialog").showModal();
  $("#dialog").scrollTop = 0;
}
async function navigate(view) {
  if (!titles[view]) return;
  if (view !== state.view && !canLeavePage()) {
    location.hash = state.view;
    return;
  }
  if (recorder?.state === "recording") {
    toast("请先结束当前录音");
    return;
  }
  libraryRequest++;
  state.view = view;
  $("#main").classList.remove("reading-results", "chat-page");
  document.body.classList.toggle("chat-open", view === "chat");
  $(".main-shell").classList.toggle("chat-active", view === "chat");
  $("#sidebar-chat").hidden = view !== "chat";
  $("#chat-history-toggle").hidden = view !== "chat";
  $("#account-menu").open = false;
  closeConversationMenu();
  location.hash = view;
  $("#page-title").textContent = titles[view];
  $$("[data-view]").forEach((b) =>
    b.classList.toggle("active", b.dataset.view === view),
  );
  pageLoading = true;
  $("#main").innerHTML = '<div class="loading">正在读取个人资料库…</div>';
  try {
    await refreshData();
    if (view === "search" || view === "notes") {
      state.categories = [];
      state.tagFilters = [];
      const data = await api("/notes");
      state.notes = data.items;
      state.total = data.total;
      renderLibrary();
    } else if (view === "materials") await renderMaterials();
    else if (view === "chat") await renderChat();
    else await renderSettings();
  } catch (e) {
    $("#main").innerHTML =
      `<div class="empty-state"><h3>暂时无法载入</h3><p>${esc(e.message)}</p><button id="retry" class="secondary">重试</button></div>`;
    $("#retry").onclick = () => navigate(view);
  } finally {
    pageLoading = false;
  }
}
function searchModeLabel() {
  return state.settings?.cloud_search_enabled === false
    ? "本地搜索"
    : state.settings?.ai_enabled
      ? state.settings.ai_provider.toUpperCase() + " 搜索"
      : "本地搜索";
}
function searchSwitch(compact = false) {
  return `<label class="cloud-search-toggle"><input type="checkbox" id="cloud-search-toggle" ${state.settings.cloud_search_enabled !== false ? "checked" : ""}><span class="switch-track" aria-hidden="true"></span><span>${compact?"云端 AI":"云端 AI 辅助"}</span></label>`;
}
function bindSearchSwitch() {
  const toggle = $("#cloud-search-toggle");
  if (!toggle) return;
  toggle.onchange = async () => {
    const enabled = toggle.checked;
    if (state.busy || state.searching || state.chatting) {
      toggle.checked = !enabled;
      toast("请等待当前操作完成");
      return;
    }
    toggle.disabled = true;
    try {
      await api("/search-settings", {
        method: "PUT",
        body: { cloud_search_enabled: enabled },
      });
      state.settings.cloud_search_enabled = enabled;
      $("#model-badge").textContent = searchModeLabel();
      toast(
        enabled
          ? "云端 AI 搜索已开启"
          : "云端 AI 搜索已关闭",
      );
    } catch (e) {
      toggle.checked = !enabled;
      toast(e.message);
    } finally {
      toggle.disabled = false;
    }
  };
}
function statsStrip() {
  return `<div class="overview-strip"><span><strong>${state.stats.notes}</strong>篇笔记</span><span><strong>${state.stats.attachments}</strong>份材料</span></div>`;
}
function extractionLabel(a) {
  return (a.extracted_text || "").startsWith("【固定演示")
    ? "示例预置文本"
    : a.status === "ready"
      ? "已识别"
      : a.mime.startsWith("image/")
        ? "可按指纹检索 · 暂无文字"
        : "待补充文本";
}
async function applyDemoSample(file, attachment) {
  if (attachment.status === "ready") return attachment;
  const match = await window.LinjiDemoSamples.match(file);
  if (!match) return attachment;
  try {
    const updated = await api("/attachments/" + attachment.id + "/text", {
      method: "PUT",
      body: { text: match.text },
    });
    updated.demo_match = true;
    return updated;
  } catch (error) {
    attachment.extraction_error = "示例文本保存失败：" + error.message;
    return attachment;
  }
}
async function chooseDemoSample(target) {
  let samples;
  try {
    const response = await fetch(siteURL("/demo-assets.json"));
    if (!response.ok) throw new Error("无法读取测试素材");
    samples = Object.values(await response.json());
  } catch (error) {
    toast(error.message);
    return;
  }
  openDialog(
    "选择测试素材",
    `<p class="notice">这些是固定演示文件。系统会匹配完整文件并使用预置描述或转写；真实新材料需要配置识别服务。</p><div class="material-list">${samples.map((a, i) => `<button type="button" class="material-row" data-demo-index="${i}"><span class="file-icon">${esc(a.kind)}</span><span class="material-info"><b>${esc(a.name)}</b><small>点击后作为搜索输入</small></span><span>↗</span></button>`).join("")}</div>`,
  );
  $$("[data-demo-index]").forEach(
    (button) =>
      (button.onclick = async () => {
        const item = samples[Number(button.dataset.demoIndex)];
        button.disabled = true;
        try {
          const response = await fetch(
            siteURL("/demo-files/" + encodeURIComponent(item.name)),
          );
          if (!response.ok) throw new Error("无法读取示例文件");
          const blob = await response.blob();
          $("#dialog").close();
          await uploadFiles(
            [new File([blob], item.name, { type: blob.type })],
            target,
          );
        } catch (error) {
          toast(error.message);
          button.disabled = false;
        }
      }),
  );
}
function marked(value, terms = []) {
  const text = String(value || ""),
    lower = text.toLowerCase();
  let cursor = 0,
    out = "";
  const keys = [
    ...new Set(
      terms
        .map((t) => String(t).toLowerCase())
        .filter((t) => t.length > 1 && t.length <= 80),
    ),
  ];
  while (cursor < text.length) {
    let next = -1,
      key = "";
    for (const term of keys) {
      const at = lower.indexOf(term, cursor);
      if (
        at >= 0 &&
        (next < 0 || at < next || (at === next && term.length > key.length))
      ) {
        next = at;
        key = term;
      }
    }
    if (next < 0) {
      out += esc(text.slice(cursor));
      break;
    }
    out +=
      esc(text.slice(cursor, next)) +
      "<mark>" +
      esc(text.slice(next, next + key.length)) +
      "</mark>";
    cursor = next + key.length;
  }
  return out;
}
function resultEvidence(note) {
  const e = note.search_match;
  if (e) return { ...e, terms: (e.terms || []).slice(0, 8) };
  return {
    terms: [],
    fields: [],
    excerpt: note.content || "",
    source: "笔记正文",
    attachment_ids: [],
  };
}
function searchCards(notes) {
  if (!notes.length)
    return `<div class="empty-state result-empty-state">${icon("search")}<h3>${state.categories.length || state.tagFilters.length ? "当前筛选下没有匹配资料" : "没有找到相关资料"}</h3><p>换个关键词，或核对材料文字。</p><div class="empty-actions"><button class="secondary" id="refine-query">修改检索内容</button>${state.categories.length || state.tagFilters.length ? '<button class="text-button" id="reset-result-filter">清除筛选</button>' : ""}</div></div>`;
  const labels = {
    title: "标题",
    tags: "标签",
    content: "正文",
    steps: "记录条目",
    attachment: "材料文字",
    image: "图片",
  };
  return notes
    .map((n, index) => {
      const e = resultEvidence(n),
        type = typeOf(n),
        files = n.attachments || [],
        images = files.filter((a) => a.mime.startsWith("image/")).slice(0, 2),
        others = files.filter((a) => !a.mime.startsWith("image/")).slice(0, 2);
      const matches = [...new Set(e.fields.map((f) => labels[f] || f))],
        image = e.image_matches?.length
          ? e.image_matches.some((m) => m.method === "exact")
            ? "相同图片"
            : "近似图片，需核对"
          : "";
      return `<article class="result-card learning-result" aria-labelledby="result-title-${n.id}"><div class="result-meta"><span class="note-type-badges">${typeBadges(n)}</span><span class="meta-dot">·</span><time>${dateLabel(n.occurred_on)}</time></div>
 <h3 class="result-title" id="result-title-${n.id}"><button data-note="${n.id}">${marked(n.title, e.terms)}</button></h3>
 <div class="result-columns ${files.length ? "" : "content-only"}"><div class="result-reading">${readingContent(n)}
 ${
   n.steps.length
     ? `<section class="result-steps"><h4>${esc(type.steps_label)} <span>${n.steps.length} 项</span></h4><ol role="list">${n.steps
         .slice(0, 6)
         .map((step) => `<li>${marked(step, e.terms)}</li>`)
         .join("")}</ol>${
         n.steps.length > 6
           ? `<details class="more-steps"><summary>展开其余 ${n.steps.length - 6} 项</summary><ol role="list" start="7">${n.steps
               .slice(6)
               .map((step) => `<li>${marked(step, e.terms)}</li>`)
               .join("")}</ol></details>`
           : ""
       }</section>`
     : ""
 }
 </div>${files.length ? `<aside class="result-materials"><h4>原始材料 <span>${files.length} 份</span></h4><div class="result-thumbs">${images.map((a) => `<button data-result-file="${a.id}" aria-label="查看原图：${esc(a.filename)}"><img src="${APP_BASE}/api/attachments/${a.id}/thumbnail" alt="${esc(a.filename)}" loading="lazy"><span>${esc(a.filename)}</span></button>`).join("")}</div>${others.map((a) => `<button class="result-file" data-result-file="${a.id}">${icon(a.mime.startsWith("audio/") ? "mic" : "file")}<span><b>${esc(a.filename)}</b></span>${icon("arrow")}</button>`).join("")}${files.length > images.length + others.length ? `<button class="text-button" data-note="${n.id}">查看全部材料</button>` : ""}</aside>` : ""}</div>
 <div class="result-footer"><div class="tags">${n.tags
   .slice(0, 6)
   .map(
     (t) =>
       `<button class="tag" data-tag="${esc(t)}" title="按此标签筛选">${esc(t)}</button>`,
   )
   .join(
     "",
   )}</div><button class="secondary small" data-note="${n.id}">查看笔记 ${icon("arrow")}</button></div>
 <details class="match-explanation"><summary>匹配依据${image ? " · " + esc(image) : ""}</summary><p>${matches.length ? "文字命中：" + matches.map(esc).join("、") : "基于相关内容召回，请核对原文。"}</p></details></article>`;
    })
    .join("");
}
function searchSummary() {
  const r = state.results, files = state.resultFiles || [];
  const typeNames=(state.completedScope||[]).map((id)=>catalogTypes().find((t)=>t.id===id)?.title||id);
  const submittedTags=state.completedScopeTags||[];
  const scope=(typeNames.length||submittedTags.length)?`<div class="submitted-scope"><span>检索条件</span>${typeNames.length?`<span><b>类型</b> ${typeNames.map(esc).join("、")}</span>`:""}${submittedTags.length?`<span><b>标签</b> ${submittedTags.map(esc).join("、")}</span>`:""}</div>`:"";
  return `${scope}${files.length ? `<details class="query-details"><summary>核对识别文字${!r.query ? " · 暂无可用文字" : ""}</summary><p>${esc(r.query || "暂无识别文字。图片仍可按指纹匹配，录音可补充转写。")}</p></details>` : ""}${(r.warnings || []).length ? `<div class="search-warnings" role="status">${r.warnings.map((w) => `<p>${esc(w)}</p>`).join("")}</div>` : ""}`;
}

function selectionChips(types,tags) {
 const names=types.map((id)=>catalogTypes().find((t)=>t.id===id)).filter(Boolean);
 const chip=(kind,key,title,attribute)=>`<button type="button" class="filter-selection" ${attribute}="${esc(key)}" aria-label="取消${kind}：${esc(title)}"><small>${kind}</small><span class="filter-selection-value">${esc(title)}</span>${icon("close")}</button>`;
 return names.map((t)=>chip("类型",t.id,t.title,"data-remove-type")).join("")+tags.map((tag)=>chip("标签",tag,tag,"data-remove-tag")).join("");
}
function libraryFilterBar(visible,filtered,hasResults,search) {
 const tagged=visible.filter((n)=>matchesTags(n,state.tagFilters));
 const typeNotes=visible.filter((n)=>matchesTypes(n,state.categories));
 const types=catalogTypes().map((t)=>({...t,count:hasResults?tagged.filter((n)=>typesOf(n).some((v)=>v.id===t.id)).length:t.count||0}));
 const tags=hasResults?topTags(typeNotes,Infinity):[...(state.taxonomy?.tags||[])];
 for(const tag of state.tagFilters)if(!tags.some((t)=>t.name===tag))tags.push({name:tag,count:0});
 const quick=[...tags].sort((a,b)=>Number(state.tagFilters.includes(b.name))-Number(state.tagFilters.includes(a.name))).slice(0,6);
 const active=state.categories.length+state.tagFilters.length,total=hasResults?visible.length:state.stats.notes,count=hasResults?filtered.length:state.total;
 const choice=(value,title,count,selected,attr)=>`<button class="facet-choice ${selected?"is-selected":""}" ${attr}="${esc(value)}" aria-pressed="${selected}"><span class="facet-check" aria-hidden="true">${selected?icon("check"):""}</span><span>${esc(title)}</span>${count!==null?`<small>${count}</small>`:""}</button>`;
 return `<div class="retrieval-filter-panel multi-filters"><div class="section-heading"><h2>${hasResults?"检索结果":search?"最近资料":"全部资料"}<span class="count-badge">${count}</span>${hasResults&&active?`<small class="filtered-total">/ ${total} 篇</small>`:""}</h2><div class="section-actions"><button class="text-button" id="manage-library-taxonomy">管理分类与标签</button>${hasResults?`<span class="result-engine">${(state.results.engine||"").includes("AI")?"AI 辅助":"本地匹配"}</span><label class="sort-control"><span class="visually-hidden">结果排序</span><select id="result-sort"><option value="relevance" ${state.resultSort==="relevance"?"selected":""}>按匹配顺序</option><option value="date" ${state.resultSort==="date"?"selected":""}>按记录日期</option></select></label><button class="text-button" id="clear-search">结束检索</button>`:`<button class="text-button" id="library-action">${search?"全部笔记 "+icon("arrow"):"导出笔记"}</button>`}</div></div>
 <div class="facet-row"><span class="facet-label" id="type-filter-label">资料类型<small>匹配任一种</small></span><div class="facet-options" role="group" aria-labelledby="type-filter-label">${choice("","全部",null,!state.categories.length,"data-category")}${types.map((t)=>choice(t.id,t.title,t.count,state.categories.includes(t.id),"data-category")).join("")}</div></div>
 <div class="facet-row"><span class="facet-label" id="tag-filter-label">标签<small>同时满足</small></span><div class="facet-options" role="group" aria-labelledby="tag-filter-label">${choice("","全部",null,!state.tagFilters.length,"data-tag")}${quick.map((t)=>choice(t.name,t.name,t.count,state.tagFilters.includes(t.name),"data-tag")).join("")}<button class="tag-picker-trigger" id="open-tag-picker" type="button">${icon("search")}查找标签<span>${tags.length}</span></button></div></div>
 ${active?`<div class="active-filters multi-selected"><span class="selection-count">已选 <b>${active}</b></span>${selectionChips(state.categories,state.tagFilters)}<button type="button" class="text-button" id="clear-library-filters">清除筛选</button></div>`:""}</div>`;
}
function openFacetPicker(kind,selected,onApply,entries=null) {
 const isType=kind==="type",pending=[...selected],items=entries||(isType?catalogTypes().map((t)=>({id:t.id,name:t.title,count:t.count||0})):state.taxonomy?.tags.map((t)=>({...t,id:t.name}))||[]);
 let values=[...pending];
 for(const key of values)if(!items.some((v)=>v.id===key))items.unshift({id:key,name:key,count:0});
 openDialog(isType?"选择资料类型":"选择标签",`<section class="facet-picker"><label class="tag-picker-search">${icon("search")}<span class="visually-hidden">查找名称</span><input id="tag-picker-query" placeholder="查找名称" autocomplete="off"></label><div id="tag-picker-list" class="tag-picker-list multi-picker" role="group" aria-label="${isType?"资料类型":"标签"}列表"></div><div class="picker-footer"><span id="tag-picker-count" role="status"></span><button class="text-button" type="button" id="picker-clear">清空</button><button class="primary small" type="button" id="picker-apply">应用筛选</button></div></section>`, { picker: true });
 const draw=()=>{const q=$('#tag-picker-query').value.trim().toLocaleLowerCase();const found=items.filter((v)=>v.name.toLocaleLowerCase().includes(q));$('#tag-picker-list').innerHTML=found.map((v)=>`<button type="button" class="tag-choice ${values.includes(v.id)?"selected":""}" data-pick="${esc(v.id)}" aria-pressed="${values.includes(v.id)}"><span class="facet-check" aria-hidden="true">${values.includes(v.id)?icon("check"):""}</span><span class="tag-choice-label">${esc(v.name)}</span><small>${v.count} 篇</small></button>`).join("")||'<p class="empty-history">没有找到</p>';$('#tag-picker-count').textContent=`已选 ${values.length} 项`;$('#picker-clear').disabled=!values.length;$$("[data-pick]").forEach((b)=>b.onclick=()=>{if(!isType&&!values.includes(b.dataset.pick)&&values.length>=20)return toast("最多选择 20 个标签");const key=b.dataset.pick;values=toggled(values,key);draw();$$("[data-pick]").find((item)=>item.dataset.pick===key)?.focus({preventScroll:true});});};
 $('#tag-picker-query').oninput=draw;$('#picker-clear').onclick=()=>{values=[];draw();};$('#picker-apply').onclick=()=>{closeDialog();onApply(values);};draw();$('#tag-picker-query').focus();
}

function updateSearchScopeUI() {
 const names=state.scopeTypes.map((id)=>catalogTypes().find((t)=>t.id===id)?.title||id);
 const typeLabel=!names.length?"全部类型":names.length===1?names[0]:names.length+" 个类型";
 const tagLabel=!state.scopeTags.length?"全部标签":state.scopeTags.length===1?state.scopeTags[0]:state.scopeTags.length+" 个标签";
 $('#search-scope').innerHTML=`<span>${esc(typeLabel)}</span>${icon("chevron")}`;
 $('#search-scope-tags').innerHTML=`<span>${esc(tagLabel)}</span>${icon("chevron")}`;
 const root=$('#search-scope-selection');root.hidden=!names.length&&!state.scopeTags.length;
 root.innerHTML=names.map((name,i)=>`<button type="button" class="scope-condition" data-scope-remove-type="${esc(state.scopeTypes[i])}" aria-label="移除检索类型：${esc(name)}"><small>类型</small><span>${esc(name)}</span>${icon("close")}</button>`).join("")+state.scopeTags.map((tag)=>`<button type="button" class="scope-condition" data-scope-remove-tag="${esc(tag)}" aria-label="移除检索标签：${esc(tag)}"><small>标签</small><span>${esc(tag)}</span>${icon("close")}</button>`).join("")+(root.hidden?"":'<button type="button" class="text-button scope-clear" id="clear-search-scope">清空条件</button>');
 $$('[data-scope-remove-type]').forEach((b)=>b.onclick=()=>{state.scopeTypes=state.scopeTypes.filter((id)=>id!==b.dataset.scopeRemoveType);updateSearchScopeUI();});
 $$('[data-scope-remove-tag]').forEach((b)=>b.onclick=()=>{state.scopeTags=state.scopeTags.filter((tag)=>tag!==b.dataset.scopeRemoveTag);updateSearchScopeUI();});
 $('#clear-search-scope')?.addEventListener('click',()=>{state.scopeTypes=[];state.scopeTags=[];updateSearchScopeUI();});
}
function openTagPicker() {
 const hasResults=state.view==="search"&&!!state.results;
 const entries=hasResults?topTags(state.results.items.filter((n)=>matchesTypes(n,state.categories)),Infinity).map((t)=>({...t,id:t.name})):null;
 openFacetPicker("tag",state.tagFilters,(values)=>filterLibrary(state.categories,values),entries);
}

function bindResultMaterials(notes) {
  const files = notes.flatMap((n) => n.attachments || []);
  $$("[data-result-file]").forEach(
    (button) =>
      (button.onclick = () =>
        showMaterial(files.find((f) => f.id === button.dataset.resultFile))),
  );
}
function clipboardImages(event) {
  const items = [...(event.clipboardData?.items || [])];
  const files = items
    .filter((i) => i.kind === "file" && i.type.startsWith("image/"))
    .map((i) => i.getAsFile())
    .filter(Boolean);
  return files.length
    ? files
    : [...(event.clipboardData?.files || [])].filter((f) =>
        f.type.startsWith("image/"),
      );
}
function bindImagePaste(element, target, onComplete, capacity) {
  if (!element) return;
  element.addEventListener("paste", (event) => {
    const images = clipboardImages(event);
    if (!images.length) return;
    event.preventDefault();
    if (state.busy) return toast("图片正在上传，请稍候再粘贴");
    const supported = images.filter((f) =>
      ["image/png", "image/jpeg", "image/webp"].includes(f.type),
    );
    if (!supported.length)
      return toast("粘贴的图片格式不支持，请使用 PNG、JPG 或 WebP");
    const files = supported.map(
      (f, i) =>
        new File(
          [f],
          `粘贴图片-${Date.now()}-${i + 1}.${f.type === "image/jpeg" ? "jpg" : f.type === "image/webp" ? "webp" : "png"}`,
          { type: f.type },
        ),
    );
    const limit = capacity ? capacity() : files.length;
    if (files.length > limit) toast("最多还能添加 " + limit + " 份材料");
    if (limit > 0) uploadFiles(files.slice(0, limit), target, onComplete);
  });
}

function fileChips(files, target) {
  return (
    files
      .map(
        (a) =>
          `<span class="media-chip"><button type="button" class="file-chip" data-file="${a.id}">${a.mime.startsWith("image/") ? `<img src="${APP_BASE}/api/attachments/${a.id}/thumbnail" alt="" loading="lazy">` : ""}<span><b>${esc(a.filename)}</b><small>${extractionLabel(a)}</small></span></button><button type="button" class="remove-media" data-remove="${a.id}" aria-label="移除 ${esc(a.filename)}">×</button></span>`,
      )
      .join("") +
    (pendingUploads[target] || [])
      .map(
        (p) =>
          `<span class="media-chip upload-pending">${p.url ? `<img src="${p.url}" alt="粘贴图片预览">` : ""}<span><b>${esc(p.name)}</b><small>正在上传与识别…</small></span></span>`,
      )
      .join("")
  );
}
function cards(notes) {
  if (!notes.length)
    return `<div class="empty-state"><div class="empty-icon">${icon("file")}</div><h3>${state.stats.notes ? "这个分类暂无笔记" : "暂无笔记"}</h3><p>${state.stats.notes ? "调整分类或标签筛选。" : "新建笔记并关联材料。"}</p><div class="empty-actions"><button class="primary" id="empty-new">＋ 新建笔记</button>${!state.stats.notes ? '<button class="secondary" id="import-demo">体验虚构示例</button>' : ""}</div></div>`;
  return notes
    .map(
      (n) =>
        `<button class="note-card" data-note="${n.id}"><div class="card-meta"><span class="note-type-badges">${typeBadges(n)}</span><span>${dateLabel(n.occurred_on)}</span></div><h3>${esc(n.title)}</h3><p>${esc(n.content || n.attachments?.map((a) => a.extracted_text).join(" ") || "打开笔记，查看治疗步骤与关联材料。")}</p><div class="tags">${n.tags
          .slice(0, 4)
          .map((t) => `<span class="tag">${esc(t)}</span>`)
          .join(
            "",
          )}</div><div class="card-bottom"><span>${n.steps.length} 个记录步骤</span><span>${n.attachments.length} 份材料&nbsp; ↗</span></div></button>`,
    )
    .join("");
}
function bindCards(root = document) {
  $$("[data-note]", root).forEach(
    (b) => (b.onclick = () => showNote(b.dataset.note)),
  );
  $("#empty-new", root)?.addEventListener("click", () => editNote());
  $("#import-demo", root)?.addEventListener("click", async (e) => {
    e.target.disabled = true;
    try {
      await api("/demo", { method: "POST" });
      state.results = null;
      await navigate(state.view);
      toast("已导入 3 篇明确标注的虚构示例");
    } catch (e) {
      toast(e.message);
    }
  });
}
function renderLibrary() {
  const search = state.view === "search",
    hasResults = search && !!state.results;
  const visible = hasResults ? state.results.items : state.notes;
  let filtered=visible.filter((n)=>matchesTypes(n,state.categories)&&matchesTags(n,state.tagFilters));
  if(hasResults&&state.resultSort==="date")filtered.sort((a,b)=>b.occurred_on.localeCompare(a.occurred_on));
  $("#main").classList.toggle("reading-results", hasResults);
  $("#main").innerHTML =
    heading(
      "",
      search ? "查阅历史笔记" : "学习资料",
      search
        ? hasResults
          ? "直接阅读命中的原文与条目，必要时对照原件。"
          : "输入主题、技能名称或图片，查阅已有教程、知识与诊疗笔记。"
        : "把观察、过程与原始材料，留在同一份记录里。",
    ) +
    (search
      ? `<section class="search-panel ${hasResults ? "has-results" : ""}"><form id="search-form" class="search-form"><div class="search-input-wrap">${icon("search")}<label class="visually-hidden" for="query">检索内容</label><textarea id="query" class="search-input" placeholder="搜索笔记，或粘贴图片" maxlength="2000" rows="2">${esc(state.lastQuery || "")}</textarea></div><div id="search-files" class="attachment-chips">${fileChips(state.searchFiles, "search")}</div><div class="search-tools"><div class="input-actions"><button type="button" class="tool-btn" id="search-upload" aria-label="添加图片、音频或文档" title="图片、音频或文档">${icon("image")}材料</button><button type="button" class="tool-btn" id="search-record">${icon("mic")}录音</button></div><button class="primary" id="search-submit">检索记录 ${icon("arrow")}</button></div><div class="search-preferences"><div class="search-range-controls" role="group" aria-label="检索范围"><div class="search-scope"><span>资料类型</span><button id="search-scope" type="button" aria-label="选择检索资料类型" aria-haspopup="dialog"></button></div><div class="search-scope"><span>标签</span><button id="search-scope-tags" type="button" aria-label="选择检索标签" aria-haspopup="dialog"></button></div></div>${searchSwitch(true)}</div><div id="search-scope-selection" class="search-scope-selection" aria-label="检索条件" hidden></div><div id="search-busy" class="busy-text" hidden>正在上传并识别材料，请稍候…</div><p id="search-error" class="error" role="alert" hidden></p></form><div class="search-help"><span>可直接粘贴图片 · 单份不超过 20 MB</span><span class="keyboard-hint"><kbd>⌘</kbd> / <kbd>Ctrl</kbd> + <kbd>Enter</kbd> 检索</span></div></section>`
      : "") +
    (hasResults ? "" : statsStrip()) +
    `<section id="library-results" aria-label="${hasResults ? "检索结果" : "笔记列表"}" tabindex="-1">${libraryFilterBar(visible, filtered, hasResults, search)}${hasResults ? searchSummary() : ""}<div class="notes-grid ${hasResults ? "search-results" : ""}" id="note-list" aria-live="polite">${hasResults ? searchCards(filtered) : cards(filtered)}</div>${!hasResults && state.notes.length < state.total ? '<button class="secondary load-more" id="load-more">加载更多笔记</button>' : ""}</section>`;
  bindCards();
  bindResultMaterials(filtered);
  $$("[data-category]").forEach((b)=>b.onclick=()=>filterLibrary(b.dataset.category?toggled(state.categories,b.dataset.category):[]));
  $("#library-action")?.addEventListener("click", () =>
    search
      ? navigate("notes")
      : window.open(siteURL("/api/export"), "_blank", "noopener"),
  );
  $("#clear-search")?.addEventListener("click", () => {
    state.results = null;
    state.lastQuery = "";
    state.searchFiles = [];
    state.categories = [];
    state.tagFilters = [];
    renderLibrary();
    $("#query")?.focus();
  });
  $$("#refine-query, #edit-query").forEach(
    (b) =>
      (b.onclick = () => {
        $("#query").focus();
        $("#search-form").scrollIntoView({
          behavior: "smooth",
          block: "center",
        });
      }),
  );
  $("#reset-result-filter")?.addEventListener("click", () => {
    filterLibrary("", "");
  });
  $("#result-sort")?.addEventListener("change", (e) => {
    const y = window.scrollY;
    state.resultSort = e.target.value;
    renderLibrary();
    window.scrollTo(0, y);
  });
  $("#load-more")?.addEventListener("click", async (e) => {
    e.target.disabled = true;
    try {
      const result = await api(
        "/notes?offset="+state.notes.length+"&"+filterParams(),
      );
      state.notes.push(...result.items);
      renderLibrary();
    } catch (err) {
      toast(err.message);
      e.target.disabled = false;
    }
  });
  $$("[data-tag]").forEach((b)=>b.onclick=()=>filterLibrary(state.categories,b.dataset.tag?toggled(state.tagFilters,b.dataset.tag):[]));
  $("#manage-library-taxonomy").onclick=()=>openTaxonomyManager();$("#open-tag-picker").onclick=openTagPicker;
  $$("[data-remove-type]").forEach((b)=>b.onclick=()=>filterLibrary(state.categories.filter((id)=>id!==b.dataset.removeType)));
  $$("[data-remove-tag]").forEach((b)=>b.onclick=()=>filterLibrary(state.categories,state.tagFilters.filter((tag)=>tag!==b.dataset.removeTag)));
  $("#clear-library-filters")?.addEventListener("click",()=>filterLibrary([],[]));
  if (!search) return;
  $("#search-scope").onclick=()=>openFacetPicker("type",state.scopeTypes,(ids)=>{state.scopeTypes=ids;updateSearchScopeUI();});
  $("#search-scope-tags").onclick=()=>openFacetPicker("tag",state.scopeTags,(tags)=>{state.scopeTags=tags;updateSearchScopeUI();});
  updateSearchScopeUI();
  $("#search-form").onsubmit = doSearch;
  bindSearchSwitch();
  bindImagePaste($("#search-form"), "search");
  $("#query").onkeydown = (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      $("#search-form").requestSubmit();
    }
  };
  $("#query").oninput = () => {
    state.lastQuery = $("#query").value;
  };
  $("#search-upload").onclick = () => chooseFiles("search");
  $("#search-record").onclick = () => toggleRecording("search");
  bindFileChips("search");
}
function bindFileChips(target) {
  const root = $("#" + target + "-files");
  if (!root) return;
  $$("[data-file]", root).forEach(
    (b) =>
      (b.onclick = () =>
        showMaterial(
          state[target + "Files"].find((a) => a.id === b.dataset.file),
        )),
  );
  $$("[data-remove]", root).forEach(
    (b) =>
      (b.onclick = () => {
        state[target + "Files"] = state[target + "Files"].filter(
          (a) => a.id !== b.dataset.remove,
        );
        root.innerHTML = fileChips(state[target + "Files"], target);
        bindFileChips(target);
        if (target === "chat") sizeChatInput();
      }),
  );
}
async function doSearch(e) {
  e?.preventDefault();
  if (state.busy || state.searching) return;
  const input = $("#query"),
    query = input.value.trim();
  if (!query && !state.searchFiles.length && !state.scopeTypes.length && !state.scopeTags.length) {
    $("#search-error").hidden = false;
    $("#search-error").textContent = "请输入关键词、选择类型或标签，或添加图片、录音。";
    input.focus();
    return;
  }
  const button = $("#search-submit"),
    form = $("#search-form"),
    error = $("#search-error");
  state.searching = true;
  button.disabled = true;
  button.innerHTML = '<span class="spinner"></span> 正在查找';
  form.setAttribute("aria-busy", "true");
  error.hidden = true;
  input.disabled = true;
  $$("button,input", form).forEach((b) => (b.disabled = true));
  try {
    const result = await api("/search", {
      method: "POST",
      body: {
        query,
        attachment_ids: state.searchFiles.map((a) => a.id),
        note_types: state.scopeTypes,
        tags: state.scopeTags,
        limit: 100,
      },
    });
    state.lastQuery = query;
    state.completedQuery = query;
    state.completedScope = [...state.scopeTypes];
    state.completedScopeTags = [...state.scopeTags];
    state.resultFiles = [...state.searchFiles];
    state.results = result;
    state.categories = [];
    state.tagFilters = [];
    state.resultSort = "relevance";
    state.searching = false;
    renderLibrary();
    const results = $("#library-results");
    results.focus({ preventScroll: true });
    results.scrollIntoView({
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "auto"
        : "smooth",
      block: "start",
    });
  } catch (err) {
    error.hidden = false;
    error.textContent = err.message + "。请保留当前内容后重试。";
    toast(err.message);
  } finally {
    state.searching = false;
    if (form.isConnected) {
      form.removeAttribute("aria-busy");
      input.disabled = false;
      $$("button,input", form).forEach((b) => (b.disabled = false));
      button.innerHTML = "重新检索 " + icon("arrow");
    }
  }
}
function chooseFiles(target, onComplete) {
  if (state.busy || state.searching || state.chatting)
    return toast("请等待当前操作完成");
  const picker = document.createElement("input");
  picker.type = "file";
  picker.accept = ACCEPT;
  picker.multiple = true;
  picker.onchange = () => uploadFiles([...picker.files], target, onComplete);
  picker.click();
}
async function uploadFiles(files, target, onComplete) {
  if (state.busy || state.searching || state.chatting)
    return toast("请等待当前操作完成");
  const hasBucket = target === "search" || target === "chat";
  const available = hasBucket
    ? Math.max(0, 5 - state[target + "Files"].length)
    : target === "editor"
      ? Math.max(0, 20 - $$("#editor-file-list .editor-file").length)
      : 10;
  if (files.length > available)
    toast("本次最多可再添加 " + available + " 份材料");
  const selected = files.slice(0, available).filter((file) => {
    if (file.size > 20 * 1024 * 1024) {
      toast(file.name + " 超过 20MB");
      return false;
    }
    return true;
  });
  if (!selected.length) return;
  state.busy = true;
  const busy = $("#" + target + "-busy");
  if (busy) busy.hidden = false;
  const paint = () => {
    const root = $("#" + target + "-files");
    if (root && hasBucket) {
      root.innerHTML = fileChips(state[target + "Files"], target);
      bindFileChips(target);
    }
  };
  const pending = selected.map((file) => ({
    file,
    name: file.name,
    url: file.type.startsWith("image/") ? URL.createObjectURL(file) : "",
  }));
  if (hasBucket) pendingUploads[target] = pending;
  paint();
  const uploaded = [];
  try {
    for (const item of pending) {
      try {
        const form = new FormData();
        form.append("file", item.file);
        let a = await api("/attachments", { method: "POST", body: form });
        a = await applyDemoSample(item.file, a);
        uploaded.push(a);
        if (hasBucket) state[target + "Files"].push(a);
        if (a.extraction_error) toast(a.extraction_error);
      } catch (error) {
        toast(item.name + "：" + error.message);
      } finally {
        if (hasBucket)
          pendingUploads[target] = pendingUploads[target].filter(
            (p) => p !== item,
          );
        paint();
        if (item.url) URL.revokeObjectURL(item.url);
      }
    }
    if (onComplete && uploaded.length) await onComplete(uploaded);
    else if (target === "materials") await renderMaterials();
    if (uploaded.length)
      toast(
        uploaded.some((a) => a.demo_match)
          ? "已载入固定演示文件的预置文本。"
          : "已添加 " +
              uploaded.length +
              " 份材料" +
              (uploaded.some((a) => a.status !== "ready")
                ? "；可点击材料核对或补充文本。"
                : ""),
      );
  } catch (error) {
    toast(error.message);
  } finally {
    state.busy = false;
    if (hasBucket) pendingUploads[target] = [];
    for (const p of pending) if (p.url) URL.revokeObjectURL(p.url);
    paint();
    if (target === "chat") sizeChatInput();
    if (busy?.isConnected) busy.hidden = true;
  }
}

async function toggleRecording(target) {
  const button = $("#" + target + "-record");
  if (recorder?.state === "recording") {
    recorder.stop();
    return;
  }
  if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder)
    return toast("录音需要 localhost 或 HTTPS；也可以上传已有音频文件。");
  try {
    recordingStream = await navigator.mediaDevices.getUserMedia({
      audio: true,
    });
    recordingChunks = [];
    recordingTarget = target;
    const type = ["audio/webm", "audio/mp4"].find((t) =>
      MediaRecorder.isTypeSupported(t),
    );
    recorder = new MediaRecorder(
      recordingStream,
      type ? { mimeType: type } : {},
    );
    recorder.ondataavailable = (e) => {
      if (e.data.size) recordingChunks.push(e.data);
    };
    recorder.onstop = async () => {
      clearTimeout(recordTimeout);
      recordingStream.getTracks().forEach((t) => t.stop());
      button.classList.remove("recording");
      button.innerHTML = icon("mic") + (target === "chat" ? "" : " 录音");
      button.setAttribute("aria-label", "录音");
      button.title = "录音";
      const mime = recorder.mimeType || "audio/webm";
      const blob = new Blob(recordingChunks, { type: mime });
      await uploadFiles(
        [
          new File(
            [blob],
            `语音记录-${Date.now()}.${mime.includes("mp4") ? "m4a" : "webm"}`,
            { type: mime },
          ),
        ],
        recordingTarget,
      );
    };
    recorder.start();
    button.classList.add("recording");
    button.textContent = "● 结束录音";
    button.setAttribute("aria-label", "结束录音");
    button.title = "结束录音";
    recordTimeout = setTimeout(() => {
      if (recorder.state === "recording") recorder.stop();
    }, 120000);
  } catch (e) {
    recordingStream?.getTracks().forEach((t) => t.stop());
    toast("无法开始录音，请允许麦克风权限或上传音频文件。");
  }
}
async function showNote(id) {
  if (!allowDiscard()) return;
  try {
    const n = await api("/notes/" + id);
    const type = typeOf(n);
    openDialog(
      "完整记录",
      `<div class="reader-meta"><span class="note-type-badges">${typeBadges(n)}</span><time>${dateLabel(n.occurred_on)}</time></div><h1 class="note-detail-title">${esc(n.title)}</h1><div class="tags">${n.tags.map((t) => `<span class="tag">${esc(t)}</span>`).join("")}</div><section class="reader-section"><h3>${esc(type.content_label)}</h3><div class="note-body">${esc(n.content) || '<span class="muted">尚未填写正文</span>'}</div></section><section class="reader-section"><h3>${esc(type.steps_label)} <span>${n.steps.length} 项</span></h3>${n.steps.length ? `<ol class="step-list">${n.steps.map((step, i) => `<li><span class="step-number">${i + 1}</span><span class="step-text">${esc(step)}</span></li>`).join("")}</ol>` : '<p class="muted">尚未记录步骤，可编辑补充。</p>'}</section><section class="reader-section"><h3>原始材料 <span>${n.attachments.length} 份</span></h3><div class="reader-materials">${n.attachments.map((a) => `<button class="reader-file" data-material="${a.id}">${a.mime.startsWith("image/") ? `<img src="${APP_BASE}/api/attachments/${a.id}/thumbnail" alt="${esc(a.filename)}" loading="lazy">` : icon(a.mime.startsWith("audio/") ? "mic" : "file")}<span><b>${esc(a.filename)}</b><small>${extractionLabel(a)}</small></span>${icon("arrow")}</button>`).join("") || '<p class="muted">尚无关联材料</p>'}</div></section><div class="detail-meta">最后更新 ${dateLabel(n.updated_at)}</div><div class="form-actions reader-actions"><button class="text-button danger" id="delete-note">删除笔记</button><button class="secondary" id="recall-note">回顾这篇记录</button><button class="primary" id="edit-note">编辑笔记</button></div>`,
    );
    $("#edit-note").onclick = () => editNote(n);
    $("#recall-note").onclick = async () => {
      closeDialog(true);
      state.conversation = null;
      state.messages = [];
      state.mode = "knowledge";
      await navigate("chat");
      $("#chat-input").value = `请回顾「${n.title}」的记录与已保存步骤`;
    };
    $$("[data-material]").forEach(
      (b) =>
        (b.onclick = () =>
          showMaterial(
            n.attachments.find((a) => a.id === b.dataset.material),
            () => showNote(id),
          )),
    );
    $("#delete-note").onclick = () =>
      confirmAction(
        "删除这篇笔记？",
        "笔记将被删除，关联原件仍保留在材料归档中。",
        async () => {
          await api("/notes/" + id, { method: "DELETE" });
          closeDialog(true);
          state.results = null;
          await navigate(state.view);
          toast("笔记已删除");
        },
        () => showNote(id),
      );
  } catch (err) {
    toast(err.message);
  }
}
function confirmAction(title, desc, action, cancel) {
  openDialog(
    title,
    `<p class="note-body">${esc(desc)}</p><div class="form-actions"><button class="secondary" id="cancel-confirm">取消</button><button class="primary" id="do-confirm">确认删除</button></div>`,
  );
  $("#cancel-confirm").onclick = cancel || (() => $("#dialog").close());
  $("#do-confirm").onclick = async (e) => {
    e.target.disabled = true;
    try {
      await action();
    } catch (e) {
      toast(e.message);
    }
  };
}
function editNote(note) {
 const chosenTypes=note?typesOf(note).map((t)=>t.id):[catalogTypes()[0].id];
  let files = [...(note?.attachments || [])];
  openDialog(
    note?.id ? "编辑诊疗笔记" : "新建诊疗笔记",
    `<form id="note-form"><div class="form-grid"><label class="full">笔记标题<input name="title" value="${esc(note?.title || "")}" maxlength="160" placeholder="输入标题" required></label><fieldset class="full editor-types"><legend>资料类型 <small>可多选</small></legend><div class="editor-type-options">${catalogTypes().map((t)=>`<label><input type="checkbox" name="note_types" value="${esc(t.id)}" ${chosenTypes.includes(t.id)?"checked":""}><span>${esc(t.title)}</span></label>`).join("")}</div><div id="editor-type-summary" class="selection-preview"></div></fieldset><label>就诊 / 记录日期<input type="date" name="occurred_on" value="${note?.occurred_on || today()}" required></label><label class="full">标签 <small>用逗号分隔</small><input name="tags" value="${esc(note?.tags.join("，") || "")}" placeholder="如：咳嗽，复诊，观察记录"></label><label class="full"><span id="editor-content-label">笔记正文</span><textarea name="content" rows="7" maxlength="50000" placeholder="输入笔记内容">${esc(note?.content || "")}</textarea></label><label class="full"><span id="editor-steps-label">记录条目</span> <small>每行一条</small><textarea name="steps" rows="5" placeholder="核对初诊记录&#10;整理处置过程&#10;回顾随访变化">${esc(note?.steps.join("\n") || "")}</textarea></label><div class="full learning-editor-tools"><button type="button" class="text-button" id="insert-outline">插入整理提纲</button><span>仅用于空白正文</span></div><div class="editor-files full"><div class="inline-row between"><h4>关联原始材料</h4><button type="button" class="text-button" id="editor-upload">＋ 上传材料</button></div><div id="editor-file-list"></div><div class="busy-text" id="editor-busy" hidden>正在上传和识别…</div></div></div><p id="note-error" class="error" role="alert"></p><div class="form-actions"><button class="secondary" type="button" id="cancel-note">取消</button><button class="primary" id="save-note">保存笔记</button></div></form>`,
  );
  function drawFiles() {
    $("#editor-file-list").innerHTML = files.length
      ? files
          .map(
            (a) =>
              `<div class="editor-file"><span>${esc(a.filename)} · ${extractionLabel(a)}</span><div><button type="button" data-copy-text="${a.id}">插入文本</button> <button type="button" class="remove-file" data-unlink="${a.id}">移除关联</button></div></div>`,
          )
          .join("")
      : '<small class="muted">支持图片、音频、PDF、TXT / Markdown，单份不超过 20MB。</small>';
    $$("[data-unlink]").forEach(
      (b) =>
        (b.onclick = () => {
          files = files.filter((a) => a.id !== b.dataset.unlink);
          drawFiles();
        }),
    );
    $$("[data-copy-text]").forEach(
      (b) =>
        (b.onclick = () => {
          const text = files.find(
            (a) => a.id === b.dataset.copyText,
          )?.extracted_text;
          if (!text) return toast("尚无识别文本，请先到材料归档中补充");
          const body = $('[name="content"]');
          body.value += (body.value ? "\n\n" : "") + text;
        }),
    );
  }
  drawFiles();
  const tagInput = $('#note-form [name="tags"]');
  if ((state.taxonomy?.tags || []).length) {
    tagInput.closest("label").insertAdjacentHTML("afterend", `<details class="full editor-tag-picker"><summary>选择已有标签</summary><div class="facet-tags">${state.taxonomy.tags.map((t) => `<button type="button" class="facet-tag" data-editor-tag="${esc(t.name)}">${esc(t.name)}</button>`).join("")}</div></details>`);
    const drawTagChoices = () => { const selected=tagInput.value.split(/[,，;；\n]/).map((v)=>v.trim()).filter(Boolean);$$("[data-editor-tag]").forEach((b)=>{b.classList.toggle("selected",selected.includes(b.dataset.editorTag));b.setAttribute("aria-pressed",String(selected.includes(b.dataset.editorTag)));}); };
    $$("[data-editor-tag]").forEach((b)=>b.onclick=()=>{let tags=tagInput.value.split(/[,，;；\n]/).map((v)=>v.trim()).filter(Boolean);if(tags.includes(b.dataset.editorTag))tags=tags.filter((v)=>v!==b.dataset.editorTag);else {if(tags.length>=20)return toast("每篇笔记最多 20 个标签");tags.push(b.dataset.editorTag);}tagInput.value=tags.join("，");tagInput.dispatchEvent(new Event("input",{bubbles:true}));drawTagChoices();});
    tagInput.addEventListener("input",drawTagChoices);drawTagChoices();
  }
  const editorTypeIDs=()=>$$('#note-form [name="note_types"]:checked').map((e)=>e.value);
  const editorType=()=>catalogTypes().find((t)=>t.id===editorTypeIDs()[0])||catalogTypes().find((t)=>t.id==="other");
  function updateLabels(){const type=editorType();$('#editor-content-label').textContent=type.content_label;$('#editor-steps-label').textContent=type.steps_label;$('#editor-type-summary').textContent="已选："+editorTypeIDs().map((id)=>catalogTypes().find((t)=>t.id===id).title).join("、");}
  $$('#note-form [name="note_types"]').forEach((input)=>input.onchange=()=>{if(input.checked){if(input.value==="other")$$('#note-form [name="note_types"]').forEach((e)=>e.checked=e===input);else $('#note-form [name="note_types"][value="other"]').checked=false;}if(!editorTypeIDs().length)$('#note-form [name="note_types"][value="other"]').checked=true;updateLabels();});updateLabels();
  $("#insert-outline").onclick = () => {
    const input = $('#note-form [name="content"]');
    if (input.value.trim())
      return toast("正文已有内容，保留原文；提纲只用于空白正文。");
    const type = editorType();
    input.value =
      {
        tutorial:
          "学习目标\n\n适用情境与前提\n\n原始依据与日期\n\n注意事项与待确认问题",
        knowledge: "核心概念\n\n要点与辨析\n\n参考来源与日期\n\n待确认问题",
        review: "原始记录\n\n经验与易错点\n\n仍需确认的问题",
        followup: "原始记录与时间\n\n变化与后续记录\n\n待确认事项",
        atlas: "图片与标注说明\n\n对应记录\n\n来源与日期",
      }[type.id] ||
      "记录背景\n\n观察与原始资料\n\n记录经过\n\n结果与待确认事项";
    input.focus();
  };
  bindImagePaste(
    $("#note-form"),
    "editor",
    async (uploaded) => {
      files.push(...uploaded);
      drawFiles();
    },
    () => Math.max(0, 20 - files.length),
  );
  $("#editor-upload").onclick = () =>
    chooseFiles("editor", async (uploaded) => {
      files.push(...uploaded);
      drawFiles();
    });
  bindDraft($("#note-form"), () => files.map((f) => f.id).join(","));
  $("#cancel-note").onclick = () => closeDialog();
  $("#note-form").addEventListener("input", () => {
    $("#save-note").textContent = "保存笔记";
  });
  $("#note-form").onsubmit = async (e) => {
    e.preventDefault();
    if (state.busy) return toast("请等待材料上传完成");
    const form = new FormData(e.target);
    const button = $("#save-note");
    button.disabled = true;
    button.textContent = "正在保存…";
    $("#note-error").textContent = "";
    try {
      const data = {
        expected_version: note?.version || "",
        title: form.get("title").trim(),
        category: editorType().title,
        note_types: editorTypeIDs(),
        occurred_on: form.get("occurred_on"),
        tags: form
          .get("tags")
          .split(/[,，]/)
          .map((t) => t.trim())
          .filter(Boolean),
        content: form.get("content"),
        steps: form
          .get("steps")
          .split("\n")
          .map((t) => t.trim())
          .filter(Boolean),
        attachment_ids: files.map((a) => a.id),
      };
      await api("/notes" + (note?.id ? "/" + note.id : ""), {
        method: note?.id ? "PUT" : "POST",
        body: data,
      });
      closeDialog(true);
      state.results = null;
      await navigate(state.view);
      toast("笔记已保存");
    } catch (e) {
      $("#note-error").textContent = e.message;
    } finally {
      button.disabled = false;
      button.textContent = "保存笔记";
    }
  };
}
async function renderMaterials() {
  const files = await api("/attachments");
  let kind = "all";
  $("#main").innerHTML =
    heading("", "材料归档", "让每张图片、每段录音，都能回到对应的记录。") +
    `<div class="upload-zone"><div class="upload-zone-copy">${icon("image")}<div><h3>添加原始材料</h3><p>图片、音频、PDF 或文字文档 · 单份不超过 20 MB</p></div></div><button class="primary" id="material-upload">${icon("plus")}上传材料</button></div><div id="materials-busy" class="busy-text" hidden>正在上传并识别材料…</div><div class="material-toolbar"><div class="filter-tabs">${[
      ["all", "全部"],
      ["image", "图片"],
      ["audio", "音频"],
      ["document", "文档"],
    ]
      .map(
        ([k, l]) =>
          `<button data-kind="${k}" class="${k === "all" ? "active" : ""}">${l}</button>`,
      )
      .join(
        "",
      )}</div><label class="material-search">${icon("search")}<input id="material-filter" placeholder="按文件名查找" aria-label="按文件名查找材料"></label></div><div class="material-list" id="material-list"></div>`;
  function draw() {
    const q = $("#material-filter").value.trim().toLowerCase();
    const items = files.filter(
      (a) =>
        (!q || a.filename.toLowerCase().includes(q)) &&
        (kind === "all" ||
          (kind === "document"
            ? !["image", "audio"].some((t) => a.mime.startsWith(t + "/"))
            : a.mime.startsWith(kind + "/"))),
    );
    $("#material-list").innerHTML =
      items
        .map(
          (a) =>
            `<button class="material-row" data-material="${a.id}">${a.mime.startsWith("image/") ? `<img class="material-mini" src="${APP_BASE}/api/attachments/${a.id}/thumbnail" alt="" loading="lazy">` : `<span class="file-icon">${icon(a.mime.startsWith("audio/") ? "mic" : "file")}</span>`}<span class="material-info"><b>${esc(a.filename)}</b><small>${prettySize(a.size)} · ${dateLabel(a.created_at)} · ${a.note_id ? "已关联笔记" : "待关联笔记"}</small></span><span class="tag">${extractionLabel(a)}</span>${icon("arrow")}</button>`,
        )
        .join("") ||
      `<div class="empty-state"><h3>${files.length ? "没有符合条件的材料" : "先保存一份原始材料"}</h3><p>${files.length ? "换个文件名或材料类型试试。" : "上传后可以核对识别文字，再关联到笔记。"}</p></div>`;
    $$("[data-material]").forEach(
      (b) =>
        (b.onclick = () =>
          showMaterial(files.find((a) => a.id === b.dataset.material))),
    );
  }
  $("#material-upload").onclick = () => chooseFiles("materials");
  $("#material-filter").oninput = draw;
  $$("[data-kind]").forEach(
    (b) =>
      (b.onclick = () => {
        kind = b.dataset.kind;
        $$("[data-kind]").forEach((x) => x.classList.toggle("active", x === b));
        draw();
      }),
  );
  draw();
}
async function showMaterial(a, back = null) {
  if (!allowDiscard()) return;
  if (!a) return;
  const url = siteURL("/api/attachments/" + a.id + "/file");
  openDialog(
    "原始材料",
    `${a.mime.startsWith("image/") ? `<img class="material-preview" src="${url}" alt="${esc(a.filename)}">` : a.mime.startsWith("audio/") ? `<audio class="material-preview" controls src="${url}"></audio>` : ""}<div class="inline-row between"><b>${esc(a.filename)}</b><a class="text-button" href="${url}" target="_blank" rel="noopener">打开原件 ↗</a></div><p class="detail-meta">${prettySize(a.size)} · ${a.status === "ready" ? "已提取文本" : "尚无可用识别文本"}</p><label>识别 / 转写文本 <small>可修正，保存后用于检索</small><textarea id="material-text" rows="9" maxlength="50000" placeholder="核对识别结果，也可以手动填写，或使用当前检索设置重新提取。">${esc(a.extracted_text)}</textarea></label>${!a.note_id ? '<p class="notice">关联笔记后，材料可被检索。</p>' : ""}<div class="form-actions"><button class="text-button danger" id="delete-material">删除材料</button><button class="secondary" id="retry-extract">重新提取</button><button class="secondary" id="material-note">用于新笔记</button><button class="primary" id="save-material">保存文本</button></div>`,
  );
  setDialogBack(back);
  const originalText = a.extracted_text || "";
  dialogDirtyCheck = () =>
    !!$("#material-text") && $("#material-text").value !== originalText;
  $("#material-note").textContent = a.note_id ? "查看关联笔记" : "整理成新笔记";
  $("#save-material").onclick = async (e) => {
    e.target.disabled = true;
    try {
      const updated = await api("/attachments/" + a.id + "/text", {
        method: "PUT",
        body: { text: $("#material-text").value },
      });
      Object.assign(a, updated);
      for (const target of ["search", "chat"]) {
        state[target + "Files"] = state[target + "Files"].map((x) =>
          x.id === a.id ? a : x,
        );
        const root = $("#" + target + "-files");
        if (root) {
          root.innerHTML = fileChips(state[target + "Files"], target);
          bindFileChips(target);
        }
      }
      toast("文本已保存，检索索引已更新");
      closeDialog(true);
      if (state.view === "materials") await renderMaterials();
    } catch (e) {
      toast(e.message);
    } finally {
      e.target.disabled = false;
    }
  };
  $("#retry-extract").onclick = async (e) => {
    if (!allowDiscard()) return;
    e.target.disabled = true;
    e.target.textContent = "正在识别…";
    try {
      Object.assign(
        a,
        await api("/attachments/" + a.id + "/extract", { method: "POST" }),
      );
      dialogDirtyCheck = null;
      await showMaterial(a, back);
      toast(
        a.status === "ready"
          ? "识别完成，请核对文本"
          : "未获得识别文本，请配置服务或手动填写",
      );
    } catch (e) {
      toast(e.message);
    } finally {
      e.target.disabled = false;
      e.target.textContent = "重新提取";
    }
  };
  $("#material-note").onclick = () => {
    if (a.note_id) {
      if (!allowDiscard()) return;
      dialogDirtyCheck = null;
      showNote(a.note_id);
    } else
      editNote({
        attachments: [a],
        title: a.filename.replace(/\.[^.]+$/, ""),
        category: "诊疗笔记",
        tags: [],
        content: $("#material-text").value,
        steps: [],
        occurred_on: today(),
      });
  };
  $("#delete-material").onclick = () =>
    confirmAction(
      "删除这份材料？",
      "原始文件和识别文本将被删除，同时移除与笔记的关联。",
      async () => {
        await api("/attachments/" + a.id, { method: "DELETE" });
        state.searchFiles = state.searchFiles.filter((x) => x.id !== a.id);
        state.chatFiles = state.chatFiles.filter((x) => x.id !== a.id);
        $("#dialog").close();
        await navigate(state.view);
        toast("材料已删除");
      },
      () => showMaterial(a, back),
    );
}
function chatInline(text) {
  const tokens = [], keep = (html) => `\u0000${tokens.push(html)-1}\u0000`;
  let out = esc(text).replace(/`([^`\n]+)`/g, (_, code) => keep(`<code>${code}</code>`));
  out = out.replace(/\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, title, href) => keep(`<a href="${href}" target="_blank" rel="noopener noreferrer">${title}</a>`));
  out = out.replace(/\*\*([^*\n]+)\*\*/g, "<strong>$1</strong>");
  return out.replace(/\u0000(\d+)\u0000/g, (_, i) => tokens[Number(i)] || "");
}
function chatMarkdown(source) {
  const lines = String(source || "").replace(/\r\n?/g, "\n").split("\n");
  const html = []; let i = 0;
  const boundary = (v) => /^\s*(?:```|~~~|#{1,6}\s|[-*+]\s|\d+[.)]\s|>\s?|(?:---+|___+)\s*$)/.test(v);
  const cells = (v) => v.trim().replace(/^\||\|$/g, "").split("|").map((x) => x.trim());
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) { i++; continue; }
    const fence = line.match(/^\s*(```|~~~)(.*)$/);
    if (fence) {
      const text = []; i++;
      while (i < lines.length && !lines[i].trim().startsWith(fence[1])) text.push(lines[i++]);
      if (i < lines.length) i++;
      html.push(`<pre><code>${esc(text.join("\n"))}</code></pre>`); continue;
    }
    const heading = line.match(/^\s*(#{1,6})\s+(.+)$/);
    if (heading) { const level = Math.min(heading[1].length+1, 6); html.push(`<h${level}>${chatInline(heading[2])}</h${level}>`); i++; continue; }
    if (/^\s*(?:---+|___+)\s*$/.test(line)) { html.push("<hr>"); i++; continue; }
    if (i+1 < lines.length && line.includes("|") && /^\s*\|?\s*:?-{3,}/.test(lines[i+1]) && cells(lines[i+1]).every((v) => /^:?-{3,}:?$/.test(v))) {
      const head = cells(line), body = []; i += 2;
      while (i < lines.length && lines[i].includes("|") && lines[i].trim()) body.push(cells(lines[i++]));
      html.push(`<div class="chat-table"><table><thead><tr>${head.map((c) => `<th>${chatInline(c)}</th>`).join("")}</tr></thead><tbody>${body.map((row) => `<tr>${head.map((_, n) => `<td>${chatInline(row[n] || "")}</td>`).join("")}</tr>`).join("")}</tbody></table></div>`); continue;
    }
    if (/^\s*[-*+]\s+/.test(line) || /^\s*\d+[.)]\s+/.test(line)) {
      const ordered = /^\s*\d+[.)]\s+/.test(line), pattern = ordered ? /^\s*\d+[.)]\s+(.*)$/ : /^\s*[-*+]\s+(.*)$/;
      const items = [], start = ordered ? Number(line.match(/\d+/)[0]) : 1;
      while (i < lines.length && pattern.test(lines[i])) items.push(`<li>${chatInline(lines[i++].replace(pattern, "$1"))}</li>`);
      html.push(ordered ? `<ol start="${start}">${items.join("")}</ol>` : `<ul>${items.join("")}</ul>`); continue;
    }
    if (/^\s*>/.test(line)) { const text = []; while(i < lines.length && /^\s*>/.test(lines[i])) text.push(chatInline(lines[i++].replace(/^\s*>\s?/, ""))); html.push(`<blockquote>${text.join("<br>")}</blockquote>`); continue; }
    const paragraph = [chatInline(line)]; i++;
    while (i < lines.length && lines[i].trim() && !boundary(lines[i]) && !(i+1 < lines.length && lines[i].includes("|") && /^\s*\|?\s*:?-{3,}/.test(lines[i+1]))) paragraph.push(chatInline(lines[i++]));
    html.push(`<p>${paragraph.join("<br>")}</p>`);
  }
  return html.join("");
}
function chatEngineLabel() {
  if (state.mode === "knowledge" && (state.settings.cloud_search_enabled === false || !state.settings.ai_enabled)) return "本地笔记回顾";
  const names = {deepseek:"DeepSeek",glm:"GLM",kimi:"Kimi",minimax:"MiniMax"};
  return state.settings.ai_enabled ? names[state.settings.ai_provider] || "AI 对话" : "配置对话模型";
}
function applySidebarState() {
  document.body.classList.toggle("sidebar-collapsed", sidebarCollapsed);
  const toggle = $("#sidebar-toggle");
  toggle.setAttribute("aria-expanded", String(!sidebarCollapsed));
  toggle.setAttribute("aria-label", sidebarCollapsed ? "展开侧栏" : "折叠侧栏");
  toggle.title = sidebarCollapsed ? "展开侧栏" : "折叠侧栏";
  $("#sidebar-chat").classList.toggle("history-collapsed", chatHistoryCollapsed);
  const historyToggle = $("#chat-history-toggle");
  historyToggle.setAttribute("aria-expanded", String(!chatHistoryCollapsed));
  historyToggle.setAttribute("aria-label", chatHistoryCollapsed ? "展开历史对话" : "收起历史对话");
  historyToggle.title = chatHistoryCollapsed ? "展开历史对话" : "收起历史对话";
}
function chatHistoryHTML(history) {
  return history.map((h) => `<div class="history-row ${state.conversation === h.id ? "active" : ""}" data-conversation="${esc(h.id)}"><button class="history-item ${state.conversation === h.id ? "active" : ""}" data-history="${esc(h.id)}" ${state.conversation === h.id ? 'aria-current="true"' : ""}><span>${esc(h.title)}</span><small>${dateLabel(h.created_at)} · ${h.mode === "knowledge" ? "笔记回顾" : "日常对话"}</small></button><button class="history-delete" data-delete-conversation="${esc(h.id)}" aria-label="删除对话：${esc(h.title)}" title="删除对话">${icon("trash")}</button></div>`).join("") || '<p class="empty-history">暂无历史对话</p>';
}
function closeConversationMenu(restoreFocus = false) {
  const menu = $("#conversation-context-menu");
  if (!menu) return;
  if (restoreFocus) menu._returnFocus?.focus({preventScroll:true});
  menu.remove();
}
function showConversationMenu(event, row) {
  event.preventDefault(); closeConversationMenu();
  if (state.chatting || state.busy) return;
  const id = row.dataset.conversation;
  const menu = document.createElement("div"); menu.id = "conversation-context-menu"; menu.className = "conversation-context-menu"; menu.setAttribute("role", "menu"); menu.setAttribute("aria-label", "对话操作");
  menu.innerHTML = `<button type="button" role="menuitem" class="danger">${icon("trash")}删除对话</button>`;
  menu._returnFocus = row.querySelector("[data-history]");
  (row.closest("dialog") || document.body).append(menu);
  const rect = row.getBoundingClientRect(), box = menu.getBoundingClientRect();
  const x = event.type === "contextmenu" ? event.clientX : rect.left + 16;
  const y = event.type === "contextmenu" ? event.clientY : rect.bottom;
  menu.style.left = Math.max(8, Math.min(x, innerWidth-box.width-8)) + "px";
  menu.style.top = Math.max(8, Math.min(y, innerHeight-box.height-8)) + "px";
  menu.querySelector("button").onclick = () => requestConversationDelete(id);
  menu.querySelector("button").focus({preventScroll:true});
}
function requestConversationDelete(id) {
  if (state.busy || state.chatting) return toast("请等待当前操作完成");
  const item = chatHistoryItems.find((h) => h.id === id); if (!item) return;
  closeConversationMenu();
  const fromHistory = $("#dialog").open && !!$("#chat-history-query");
  const historyQuery = $("#chat-history-query")?.value || "";
  const active = state.conversation === id;
  const hasDraft = active && (!!$("#chat-input")?.value.trim() || state.chatFiles.length);
  openDialog("删除对话", `<p class="delete-conversation-copy">删除「${esc(item.title)}」及其中的聊天记录？${hasDraft ? "当前未发送的输入也会清空。" : ""}</p><p class="error" role="alert" id="conversation-delete-error" hidden></p><div class="form-actions"><button type="button" class="secondary" id="cancel-conversation-delete">取消</button><button type="button" class="primary delete-confirm" id="confirm-conversation-delete">删除对话</button></div>`, { compact: true });
  $("#cancel-conversation-delete").onclick = () => {closeDialog(true); if (fromHistory) openChatHistory(historyQuery);};
  $("#confirm-conversation-delete").onclick = async () => {
    const cancel = $("#cancel-conversation-delete"), confirm = $("#confirm-conversation-delete"), error = $("#conversation-delete-error");
    cancel.disabled = true; confirm.disabled = true; error.hidden = true;
    conversationDeleting = true;
    try {
      await api("/conversations/" + encodeURIComponent(id), {method:"DELETE"});
      if (active) {
        state.conversation = null; state.messages = []; state.chatDraft = ""; state.chatFiles = [];
        if ($("#chat-input")) { $("#chat-input").value = ""; $("#chat-files").innerHTML = ""; $("#chat-error").hidden = true; drawMessages("latest"); sizeChatInput(); }
      }
      deletedConversations.add(id);
      chatHistoryItems = chatHistoryItems.filter((h) => h.id !== id);
      if (state.stats) state.stats.conversations = Math.max(0, (state.stats.conversations || 0)-1);
      conversationDeleting = false;
      drawChatHistory(chatHistoryItems); closeDialog(true); toast("对话已删除");
      if (fromHistory) await openChatHistory(historyQuery);
    } catch (failure) {error.textContent = failure.message; error.hidden = false; cancel.disabled = false; confirm.disabled = false;}
    finally {conversationDeleting = false;}
  };
  $("#cancel-conversation-delete").focus();
}
function bindChatHistory(root = document) {
  $$("[data-delete-conversation]", root).forEach((button) => button.onclick = () => requestConversationDelete(button.dataset.deleteConversation));
  $$("[data-conversation]", root).forEach((row) => {
    row.oncontextmenu = (event) => showConversationMenu(event, row);
    row.onkeydown = (event) => {if (event.key === "ContextMenu" || (event.shiftKey && event.key === "F10")) showConversationMenu(event, row);};
  });
  $$("[data-history]", root).forEach((button) => button.onclick = async () => {
    if (!canLeavePage()) return;
    try {
      const h = await api("/conversations/" + button.dataset.history);
      if (state.view !== "chat") return;
      state.conversation = h.conversation.id; state.mode = h.conversation.mode;
      state.messages = h.messages; state.chatFiles = []; state.chatDraft = "";
      if ($("#dialog").open) closeDialog();
      await renderChat();
    } catch (e) { toast(e.message); }
  });
}
function drawChatHistory(history) {
  history = history.filter((h) => !deletedConversations.has(h.id));
  chatHistoryItems = history;
  const root = $("#sidebar-chat"); root.hidden = false;
  root.innerHTML = `<button class="sidebar-new-chat" id="sidebar-new-chat">${icon("plus")}新对话</button><h2>历史对话</h2><div class="sidebar-history-list">${chatHistoryHTML(history)}</div>`;
  $("#sidebar-new-chat").onclick = newChat;
  bindChatHistory(root);
  applySidebarState();
  const badge = $("#chat-history-count"); if (badge) badge.textContent = history.length;
}
function newChat() {
  if (!canLeavePage()) return;
  state.conversation = null; state.messages = []; state.chatFiles = []; state.chatDraft = "";
  renderChat().then(() => $("#chat-input")?.focus()).catch((e) => toast(e.message));
}
async function openChatHistory(initialQuery = "") {
  if (state.busy || state.chatting) return toast("请等待当前操作完成");
  try {
    const history = (await api("/conversations")).filter((h) => !deletedConversations.has(h.id));
    chatHistoryItems = history;
    openDialog("历史对话", `<label class="tag-picker-search">${icon("search")}<span class="visually-hidden">搜索历史对话</span><input id="chat-history-query" placeholder="搜索历史对话" autocomplete="off"></label><div id="chat-history-results"></div>`);
    const draw = () => { const q = $("#chat-history-query").value.trim().toLocaleLowerCase(); $("#chat-history-results").innerHTML = chatHistoryHTML(history.filter((h) => h.title.toLocaleLowerCase().includes(q))); bindChatHistory($("#chat-history-results")); };
    $("#chat-history-query").value = initialQuery;
    $("#chat-history-query").oninput = draw; draw();
  } catch (e) { toast(e.message); }
}
function sizeChatInput() {
  const input = $("#chat-input"); if (!input) return;
  input.style.height = "auto"; input.style.height = Math.min(170, Math.max(60, input.scrollHeight)) + "px";
  const send = $("#send-chat"); if (send) send.disabled = state.chatting || state.busy || !(input.value.trim() || state.chatFiles.length);
}
async function renderChat() {
  const history = await api("/conversations");
  if (state.view !== "chat") return;
  $(".main-shell").classList.add("chat-active"); $("#main").classList.add("chat-page");
  $("#main").innerHTML = `<section class="chat-workspace" aria-label="AI 对话"><div class="chat-toolbar"><h1 class="visually-hidden">AI 对话</h1><button class="chat-model" id="chat-model-settings">${chatEngineLabel()}${icon("chevron")}</button><div class="chat-session-actions"><button class="text-button" id="chat-history-open">${icon("history")}<span>历史对话</span><small id="chat-history-count">${history.length}</small></button><button class="text-button" id="new-chat">${icon("plus")}新对话</button></div></div>
    <div class="chat-stage"><div class="chat-messages" id="chat-messages" role="log" aria-label="对话消息" aria-live="polite"></div><div class="chat-composer-shell"><form class="chat-composer" id="chat-form"><label class="visually-hidden" for="chat-input">输入对话内容</label><textarea id="chat-input" maxlength="5000" rows="2" placeholder="${state.mode === "knowledge" ? "输入问题，回顾笔记；也可以粘贴图片" : "输入问题，或粘贴图片"}">${esc(state.chatDraft)}</textarea><div class="attachment-chips" id="chat-files">${fileChips(state.chatFiles, "chat")}</div><div class="chat-composer-options"><div class="mode-switch" role="group" aria-label="对话模式"><button type="button" data-mode="knowledge" aria-pressed="${state.mode === "knowledge"}" class="${state.mode === "knowledge" ? "active" : ""}">笔记回顾</button><button type="button" data-mode="general" aria-pressed="${state.mode === "general"}" class="${state.mode === "general" ? "active" : ""}">日常对话</button></div>${state.mode === "knowledge" ? searchSwitch() : ""}</div><div class="chat-composer-bottom"><div class="input-actions"><button type="button" class="chat-tool" id="chat-upload" aria-label="添加图片或材料" title="添加图片或材料">${icon("paperclip")}</button><button type="button" class="chat-tool" id="chat-record" aria-label="录音" title="录音">${icon("mic")}</button><span class="chat-keyboard-hint">Enter 发送 · Shift + Enter 换行</span></div><div id="chat-busy" class="busy-text" hidden>正在上传和识别材料…</div><button class="chat-send" id="send-chat" aria-label="发送消息" title="发送消息（Enter）">${icon("send")}</button></div></form><p id="chat-error" class="error" role="alert" hidden></p></div></div></section>`;
  drawChatHistory(history); drawMessages("latest");
  $("#chat-model-settings").onclick = () => openSettingsTab("models");
  $("#chat-history-open").onclick = () => openChatHistory();
  $("#chat-upload").onclick = () => chooseFiles("chat"); $("#chat-record").onclick = () => toggleRecording("chat");
  $("#new-chat").onclick = newChat; bindFileChips("chat");
  $$("[data-mode]").forEach((b) => b.onclick = () => {
    if (state.mode === b.dataset.mode || !canLeavePage()) return;
    state.mode = b.dataset.mode; state.conversation = null; state.messages = []; state.chatFiles = []; state.chatDraft = "";
    renderChat().catch((e) => toast(e.message));
  });
  $("#chat-form").onsubmit = sendChat; bindImagePaste($("#chat-form"), "chat", sizeChatInput); bindSearchSwitch();
  $("#chat-input").oninput = () => { state.chatDraft = $("#chat-input").value; sizeChatInput(); };
  $("#chat-input").onkeydown = (e) => {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing && e.keyCode !== 229) { e.preventDefault(); $("#chat-form").requestSubmit(); }
  };
  sizeChatInput();
}
function drawMessages(follow = "preserve") {
  const root = $("#chat-messages"); if (!root) return;
  const top = root.scrollTop, nearBottom = root.scrollHeight-root.scrollTop-root.clientHeight < 120;
  $(".chat-workspace").classList.toggle("is-empty", !state.messages.length);
  root.innerHTML = state.messages.length ? `<div class="chat-thread">${state.messages.map((m, index) => `<article class="message ${m.role === "user" ? "user" : "assistant"}" aria-label="${m.role === "user" ? "你的消息" : "AI 回答"}">${m.role !== "user" ? `<div class="assistant-avatar" aria-hidden="true">临</div>` : ""}<div class="message-main">${m.provider === "local" ? '<span class="message-local">本地原文摘录</span>' : ""}<div class="message-content ${m.role !== "user" ? "chat-markdown" : ""}">${m.role === "user" ? esc(m.content) : chatMarkdown(m.content)}</div>${(m.sources || []).length ? `<div class="message-sources"><span>笔记来源</span>${m.sources.map((src) => `<button class="source-link" data-note="${esc(src.id)}">${icon("file")}<span>${esc(src.title)}</span>${icon("arrow")}</button>`).join("")}</div>` : ""}${m.role !== "user" ? `<div class="message-actions"><button type="button" class="copy-answer" data-copy-answer="${index}" aria-label="复制回答">${icon("copy")}复制</button></div>` : ""}</div></article>`).join("")}${state.chatting ? `<div class="message assistant chat-pending" role="status"><div class="assistant-avatar" aria-hidden="true">临</div><div class="pending-answer"><span class="answer-dots" aria-hidden="true"><i></i><i></i><i></i></span>正在回答</div></div>` : ""}</div>` : `<div class="chat-welcome"><span class="chat-welcome-mark" aria-hidden="true">临</span><h2>${state.mode === "knowledge" ? "想回顾哪份笔记？" : "有什么可以帮你？"}</h2></div>`;
  bindCards(root);
  $$("[data-copy-answer]",root).forEach((b) => b.onclick = async () => { try { await navigator.clipboard.writeText(state.messages[Number(b.dataset.copyAnswer)].content); b.innerHTML = icon("check") + "已复制"; setTimeout(() => { if (b.isConnected) b.innerHTML = icon("copy") + "复制"; },1500); } catch { toast("复制失败，请选择回答文字后复制"); } });
  if (follow === "bottom") root.scrollTop = root.scrollHeight;
  else if ((follow === "latest" || nearBottom) && state.messages.length) {
    const last = root.querySelector(".message.assistant:last-of-type") || root.querySelector(".message:last-child");
    root.scrollTop = last ? last.getBoundingClientRect().top-root.getBoundingClientRect().top+root.scrollTop-24 : root.scrollHeight;
  } else root.scrollTop = top;
}
async function sendChat(e) {
  e.preventDefault();
  if (state.busy || state.chatting) return;
  if (recorder?.state === "recording") return toast("请先结束录音");
  const input = $("#chat-input"), button = $("#send-chat"), error = $("#chat-error");
  const message = input.value.trim() || (state.chatFiles.length ? state.mode === "knowledge" ? "请根据所附图片或材料检索相关历史记录。" : "请描述所附图片或材料中的可见内容。" : "");
  if (!message) return;
  state.chatting = true; error.hidden = true; input.disabled = true; button.disabled = true;
  $$(".chat-toolbar button,#sidebar-chat button,.chat-composer button,.chat-composer input").forEach((b) => b.disabled = true);
  const turn = {role:"user",content:message,sources:[]}; state.messages.push(turn); drawMessages("bottom");
  try {
    const reply = await api("/chat", {method:"POST", body:{message,mode:state.mode,conversation_id:state.conversation,attachment_ids:state.chatFiles.map((a) => a.id)}});
    state.conversation = reply.conversation_id; state.messages.push({role:"assistant",...reply});
    state.chatFiles = []; state.chatDraft = ""; input.value = "";
    $("#chat-files").innerHTML = "";
    if ((reply.warnings || []).length) toast(reply.warnings.join("；"));
    state.chatting = false; drawMessages("preserve");
    api("/conversations").then((history) => {if(state.view === "chat") drawChatHistory(history);}).catch(() => {});
  } catch (failure) {
    state.messages = state.messages.filter((m) => m !== turn); state.chatting = false; drawMessages("preserve");
    error.textContent = failure.message + "，输入内容已保留，可重新发送。"; error.hidden = false;
  } finally {
    state.chatting = false; input.disabled = false;
    $$(".chat-toolbar button,#sidebar-chat button,.chat-composer button,.chat-composer input").forEach((b) => b.disabled = false);
    sizeChatInput();
  }
}

function setSettingsTab(tab) {
 const valid=['account','models','library'];if(!valid.includes(tab))tab='account';state.settingsTab=tab;
 $$('[data-settings-tab]').forEach((button)=>{const active=button.dataset.settingsTab===tab;button.setAttribute('aria-selected',String(active));button.tabIndex=active?0:-1;});
 valid.forEach((id)=>{const panel=$('#settings-'+id+'-panel');if(panel)panel.hidden=id!==tab;});
}
async function openSettingsTab(tab) {
 if(profileSaving||modelSettingsSaving){toast('请等待保存完成');return;}
 if(state.view==='settings'&&$('.settings-workspace')){setSettingsTab(tab);return;}
 state.settingsTab=tab;await navigate('settings');
}
async function renderSettings() {
  const s = state.settings,
    config = await api("/ai-config");
  const providers = [
    ["deepseek", "DeepSeek"],
    ["glm", "智谱 GLM"],
    ["kimi", "Kimi"],
    ["minimax", "MiniMax"],
  ];
  const options = (selected) =>
    `<option value="none" ${selected === "none" ? "selected" : ""}>关闭</option>` +
    providers
      .map(
        ([id, name]) =>
          `<option value="${id}" ${selected === id ? "selected" : ""}>${name}</option>`,
      )
      .join("");
  $("#main").innerHTML =
    heading(
      "YOUR WORKSPACE SERVICES",
      "设置与服务",
      "选择对话和识别服务，设置仅对当前账户生效。",
    ) +
    `<form id="ai-settings-form" class="ai-settings"><section class="setting-card"><h3>检索方式</h3><label class="cloud-search-toggle"><input type="checkbox" name="cloud_search_enabled" ${config.cloud_search_enabled !== false ? "checked" : ""}><span>云端 AI 搜索</span></label><p>关闭后使用本地检索与识别；日常对话仍可调用模型。</p><p>本地 OCR：${s.local_ocr_languages ? "已就绪" : "未就绪"} · 本地音频转写：${s.local_asr ? "已就绪（≤120秒）" : "未就绪"} · 图片指纹：可用</p></section><section class="setting-card ai-selection"><div><h3>使用的模型</h3></div><div class="ai-selection-controls"><label>对话模型<select name="chat_provider">${options(config.chat_provider)}</select></label><label>图片理解<select name="vision_provider">${options(config.vision_provider)}</select></label></div></section><div class="ai-section-head"><div><h3>服务配置</h3></div><span>密钥保存后不会再次显示</span></div><div class="ai-provider-grid">${providers
      .map(([id, name]) => {
        const p = config.providers[id];
        return `<details class="setting-card ai-provider"><summary><div class="ai-provider-head"><h4>${esc(name)}</h4><span class="ai-key-status ${p.has_key ? "configured" : ""}">${p.has_key ? "已配置" : "未配置"}</span></div></summary><div class="ai-provider-fields"><label class="key-field">API 密钥<input type="password" name="${id}_key" autocomplete="new-password" placeholder="${p.has_key ? "留空保持现有密钥" : "输入 API 密钥"}" maxlength="4096"></label><label>对话模型<input name="${id}_chat" value="${esc(p.chat_model)}" maxlength="200"></label><label>图片模型<input name="${id}_vision" value="${esc(p.vision_model)}" maxlength="200"></label>${p.has_key ? `<label class="ai-clear"><input type="checkbox" name="${id}_clear">清除已保存的密钥</label>` : ""}</div></details>`;
      })
      .join(
        "",
      )}</div><details class="setting-card ai-provider"><summary><div class="ai-provider-head"><h4>音频转写</h4><span class="ai-key-status ${config.asr.has_key ? "configured" : ""}">${config.asr.has_key ? "已配置" : "未配置"}</span></div></summary><div class="ai-provider-fields"><p>填写兼容转写接口的 HTTPS 基础地址。</p><label>服务地址<input name="asr_url" type="url" value="${esc(config.asr.base_url)}" placeholder="https://服务地址/v1"></label><label>API 密钥<input type="password" name="asr_key" autocomplete="new-password" placeholder="${config.asr.has_key ? "留空保持现有密钥" : "输入 API 密钥"}" maxlength="4096"></label><label>转写模型<input name="asr_model" value="${esc(config.asr.model)}" maxlength="200"></label>${config.asr.has_key ? '<label class="ai-clear"><input type="checkbox" name="asr_clear">清除已保存的密钥</label>' : ""}</div></details><div class="ai-save-row"><div><p>服务提供方可能收取费用；上传真实资料前请先脱敏。</p></div><button class="primary" type="submit">保存设置</button></div></form>`;

  const modelForm=$('#ai-settings-form');
  const shell=document.createElement('div');shell.className='settings-workspace';
  shell.innerHTML=`<div class="settings-tabs" role="tablist" aria-label="设置分类"><button type="button" role="tab" id="settings-tab-account" data-settings-tab="account" aria-controls="settings-account-panel">个人账户</button><button type="button" role="tab" id="settings-tab-models" data-settings-tab="models" aria-controls="settings-models-panel">模型与检索</button><button type="button" role="tab" id="settings-tab-library" data-settings-tab="library" aria-controls="settings-library-panel">分类与标签</button></div>
  <section id="settings-account-panel" class="settings-account-panel" role="tabpanel" aria-labelledby="settings-tab-account"><div class="account-grid"><section class="account-card account-details"><div class="account-identity"><div class="avatar" id="account-card-avatar">${esc(state.user.display_name.slice(0,1))}</div><div><h2 id="account-card-name">${esc(state.user.display_name)}</h2><p>${esc(state.user.username)}</p></div></div><h3>基本资料</h3><div class="account-readonly"><span>登录用户名</span><strong>${esc(state.user.username)}</strong></div><form id="profile-form" class="account-form"><label for="profile-display-name">显示名称</label><input id="profile-display-name" name="display_name" value="${esc(state.user.display_name)}" maxlength="40" autocomplete="nickname" required><p class="account-field-note">用于页面中的账户名称。</p><p class="account-form-error" id="profile-error" role="alert" hidden></p><div class="profile-actions"><span id="profile-status" role="status" aria-live="polite">已保存</span><button class="secondary small" type="button" id="profile-reset" disabled>还原</button><button class="primary small" type="submit" id="profile-save" disabled>保存修改</button></div></form></section>
  <div class="account-side"><section class="account-card"><h3>当前连接</h3><p class="account-card-note">手机 APP 使用同一服务地址。</p><div class="account-endpoint"><code id="account-service-url">${esc(location.origin+APP_BASE)}</code><button type="button" class="text-button" id="copy-service-url">复制地址</button></div></section><section class="account-card"><h3>资料导出</h3><p class="account-card-note">导出笔记 JSON，不含附件原件。</p><a href="${APP_BASE}/api/export" class="secondary small" download>导出笔记</a></section><section class="account-card account-signout"><div><h3>登录状态</h3><p class="account-card-note">当前账户：${esc(state.user.username)}</p></div><button type="button" class="text-button danger" id="settings-logout">退出登录</button></section></div></div></section>
  <section id="settings-models-panel" class="settings-models-panel" role="tabpanel" aria-labelledby="settings-tab-models" hidden></section>
  <section id="settings-library-panel" role="tabpanel" aria-labelledby="settings-tab-library" hidden><section class="account-card library-settings-card"><div><h2>分类与标签</h2><p id="taxonomy-summary">${catalogTypes().length} 个资料类型 · ${(state.taxonomy?.tags||[]).length} 个标签</p></div><button type="button" class="primary small" id="manage-settings-taxonomy">管理分类与标签</button></section></section>`;
  modelForm.before(shell);$('#settings-models-panel').append(modelForm);
  $('[data-settings-tab="account"]').onclick=()=>setSettingsTab('account');$('[data-settings-tab="models"]').onclick=()=>setSettingsTab('models');$('[data-settings-tab="library"]').onclick=()=>setSettingsTab('library');
  $$('[data-settings-tab]').forEach((button)=>button.onkeydown=(event)=>{const keys=['ArrowLeft','ArrowRight','Home','End'];if(!keys.includes(event.key))return;event.preventDefault();const tabs=$$('[data-settings-tab]'),i=tabs.indexOf(button),next=event.key==='Home'?0:event.key==='End'?tabs.length-1:(i+(event.key==='ArrowRight'?1:-1)+tabs.length)%tabs.length;setSettingsTab(tabs[next].dataset.settingsTab);tabs[next].focus();});
  setSettingsTab(state.settingsTab);
  $('#manage-settings-taxonomy').onclick=()=>openTaxonomyManager();
  $('#settings-logout').onclick=()=>$('#logout').click();
  $('#copy-service-url').onclick=async()=>{try{await navigator.clipboard.writeText(location.origin+APP_BASE);toast('地址已复制');}catch{toast('复制失败，可选中地址复制');}};
  $("#ai-settings-form").addEventListener("input", () => {
    $("#ai-settings-form").dataset.dirty = "true";
  });
  $("#ai-settings-form").addEventListener("change", () => {
    $("#ai-settings-form").dataset.dirty = "true";
  });

  const profileForm=$('#profile-form'),profileInput=$('#profile-display-name'),saveProfile=$('#profile-save'),resetProfile=$('#profile-reset'),profileStatus=$('#profile-status'),profileError=$('#profile-error');
  let savedName=state.user.display_name;
  const profileChanged=()=>{const value=profileInput.value.trim(),dirty=value!==savedName,valid=[...value].length>0&&[...value].length<=40;profileForm.dataset.dirty=String(dirty);saveProfile.disabled=profileSaving||!dirty||!valid;resetProfile.disabled=profileSaving||!dirty;profileStatus.textContent=profileSaving?'正在保存…':dirty?'未保存':'已保存';profileError.hidden=true;profileInput.removeAttribute('aria-invalid');};
  profileInput.addEventListener('input',profileChanged);resetProfile.onclick=()=>{profileInput.value=savedName;profileChanged();profileInput.focus();};
  profileForm.onsubmit=async(event)=>{
    event.preventDefault();if(profileSaving)return;const value=profileInput.value.trim();if(!value||[...value].length>40){profileError.textContent='请输入 1–40 个字符的显示名称';profileError.hidden=false;profileInput.setAttribute('aria-invalid','true');return;}if(value===savedName)return;
    profileSaving=true;$$('[data-settings-tab]').forEach(b=>b.disabled=true);profileInput.disabled=true;saveProfile.disabled=true;resetProfile.disabled=true;profileStatus.textContent='正在保存…';profileError.hidden=true;
    try{const user=await api('/auth/me',{method:'PUT',body:{display_name:value}});state.user=user;savedName=user.display_name;profileInput.value=savedName;$('#profile-name').textContent=savedName;$('#avatar').textContent=savedName.slice(0,1);$('#account-trigger').setAttribute('aria-label','账户：'+savedName);$('#account-card-name').textContent=savedName;$('#account-card-avatar').textContent=savedName.slice(0,1);profileForm.dataset.dirty='false';profileStatus.textContent='已保存';toast('账户信息已保存');}
    catch(error){profileError.textContent=error.message;profileError.hidden=false;profileStatus.textContent='保存失败';}
    finally{profileSaving=false;$$('[data-settings-tab]').forEach(b=>b.disabled=false);profileInput.disabled=false;const dirty=profileInput.value.trim()!==savedName;saveProfile.disabled=!dirty||!profileInput.value.trim();resetProfile.disabled=!dirty;}
  };
  $("#ai-settings-form").onsubmit = async (e) => {
    e.preventDefault();if(modelSettingsSaving||profileSaving)return;
    const form = e.currentTarget,
      button = form.querySelector("button[type=submit]"),
      data = new FormData(form),
      values = {};
    for (const [id] of providers) {
      values[id] = {
        api_key: data.get(id + "_key") || "",
        chat_model: data.get(id + "_chat") || "",
        vision_model: data.get(id + "_vision") || "",
        clear_key: data.has(id + "_clear"),
      };
      if (values[id].clear_key && values[id].api_key) {
        toast("不能同时替换并清除同一个密钥");
        return;
      }
    }
    const asr = {
      api_key: data.get("asr_key") || "",
      base_url: data.get("asr_url") || "",
      model: data.get("asr_model") || "",
      clear_key: data.has("asr_clear"),
    };
    if (asr.clear_key && asr.api_key) {
      toast("不能同时替换并清除音频服务密钥");
      return;
    }
    modelSettingsSaving = true;$$('[data-settings-tab]').forEach(b=>b.disabled=true);
    button.disabled = true;
    button.textContent = "正在保存…";
    try {
      await api("/ai-config", {
        method: "PUT",
        body: {
          cloud_search_enabled: data.has("cloud_search_enabled"),
          expected_version: config.version,
          chat_provider: data.get("chat_provider"),
          vision_provider: data.get("vision_provider"),
          providers: values,
          asr,
        },
      });
      await refreshData();
      $("#model-badge").textContent = searchModeLabel();
      const profileDraft = profileForm.dataset.dirty === "true" ? profileInput.value : null;
      await renderSettings();
      if (profileDraft !== null) { const input = $("#profile-display-name"); input.value = profileDraft; input.dispatchEvent(new Event("input")); }
      toast("模型设置已保存，立即生效");
    } catch (err) {
      toast(err.message);
    } finally {
      modelSettingsSaving = false;$$('[data-settings-tab]').forEach(b=>b.disabled=false);
      button.disabled = false;
      button.textContent = "保存设置";
    }
  };
}
(async () => {
  try {
    const health = await api("/health");
    initialized = health.initialized;
    if (initialized) {
      try {
        state.user = await api("/auth/me");
      } catch {
        showAuth(true);
        return;
      }
      await enterWorkspace();
    } else showAuth(false);
  } catch (e) {
    showAuth(true);
    $("#auth-error").textContent = e.message;
  }
})();


async function openTaxonomyManager(initialKind = "type") {
  if (state.busy || state.chatting) return toast("请等待当前操作完成");
  let catalog;
  try { catalog=await api("/taxonomy"); } catch(error) { return toast(error.message); }
  let kind=initialKind, query="", editing=null, deleting=null;
  const label=()=>kind==="type"?"资料类型":"标签";
  const entries=()=>kind==="type"?catalog.note_types.map((v)=>({...v,key:v.id,name:v.title})):catalog.tags.map((v)=>({...v,key:v.name}));
  openDialog("分类与标签", `<div class="taxonomy-manager"><div class="taxonomy-tabs" role="group" aria-label="分类管理"><button type="button" data-manage-kind="type">资料类型</button><button type="button" data-manage-kind="tag">标签</button></div><form id="taxonomy-create" class="taxonomy-create"><label class="visually-hidden" for="taxonomy-new-name">新增名称</label><input id="taxonomy-new-name" maxlength="40" autocomplete="off"><button class="primary small" id="taxonomy-add">新增</button></form><label class="taxonomy-search"><span class="visually-hidden">查找分类或标签</span><input id="taxonomy-search" placeholder="查找名称" autocomplete="off"></label><p id="taxonomy-error" class="error" role="alert" hidden></p><div id="taxonomy-list" class="taxonomy-list"></div></div>`);
  const error=$('#taxonomy-error');
  dialogDirtyCheck=()=>!!$('#taxonomy-new-name')?.value.trim()||!!$('#taxonomy-rename-name')&&$('#taxonomy-rename-name').value!==editing?.name;
  const draw=()=>{
    $$("[data-manage-kind]").forEach((b)=>{b.classList.toggle("selected",b.dataset.manageKind===kind);b.setAttribute("aria-pressed",String(b.dataset.manageKind===kind));});
    $('#taxonomy-new-name').placeholder="新增"+label();$('#taxonomy-add').textContent="新增"+label();
    const found=entries().filter((v)=>v.name.toLocaleLowerCase().includes(query.toLocaleLowerCase()));
    $('#taxonomy-list').innerHTML=found.map((v)=>`<div class="taxonomy-row" data-taxonomy-row="${esc(v.key)}">${editing?.key===v.key?`<form class="taxonomy-rename"><label class="visually-hidden" for="taxonomy-rename-name">新名称</label><input id="taxonomy-rename-name" value="${esc(v.name)}" maxlength="40" required><button class="text-button" type="submit">保存</button><button class="text-button" type="button" data-cancel-edit>取消</button></form>`:`<div class="taxonomy-row-main"><div class="taxonomy-row-title"><b>${esc(v.name)}</b><small>${v.count} 篇</small></div><div class="taxonomy-row-actions"><button type="button" class="text-button" data-rename-taxonomy="${esc(v.key)}" aria-label="重命名${label()}：${esc(v.name)}">重命名</button>${kind==="type"&&v.key==="other"?'<span class="taxonomy-reserved">保留类型</span>':`<button type="button" class="text-button danger" data-delete-taxonomy="${esc(v.key)}" aria-label="删除${label()}：${esc(v.name)}">删除</button>`}</div></div>`}${deleting?.key===v.key?`<div class="taxonomy-delete-confirm"><p>${kind==="type"?`删除后解除此类型；没有其他类型的笔记归入「${esc(catalog.note_types.find((t)=>t.id==="other").title)}」。`:`删除后，从 ${v.count} 篇笔记移除此标签。`}笔记正文和材料保留。</p><button type="button" class="text-button danger" id="taxonomy-confirm-delete">确认删除</button><button type="button" class="text-button" data-cancel-edit>取消</button></div>`:""}</div>`).join("")||'<p class="empty-history">没有找到'+label()+'</p>';
    $$("[data-rename-taxonomy]").forEach((b)=>b.onclick=()=>{editing=entries().find((v)=>v.key===b.dataset.renameTaxonomy);deleting=null;draw();$('#taxonomy-rename-name').focus();$('#taxonomy-rename-name').select();});
    $$("[data-delete-taxonomy]").forEach((b)=>b.onclick=()=>{deleting=entries().find((v)=>v.key===b.dataset.deleteTaxonomy);editing=null;draw();$('#taxonomy-confirm-delete').focus();});
    $$("[data-cancel-edit]").forEach((b)=>b.onclick=()=>{editing=null;deleting=null;draw();});
    $('.taxonomy-rename')?.addEventListener('submit',(e)=>{e.preventDefault();mutate('rename',editing.key,$('#taxonomy-rename-name').value);});
    $('#taxonomy-confirm-delete')?.addEventListener('click',()=>mutate('delete',deleting.key));
  };
  const mutate=async(action,id="",name="")=>{
    if(taxonomySaving)return;
    taxonomySaving=true;error.hidden=true;$$('button,input',$('.taxonomy-manager')).forEach((e)=>e.disabled=true);
    try {
      catalog=await api('/taxonomy',{method:'POST',body:{kind,action,id,name,expected_version:catalog.version}});state.taxonomy=catalog;
      editing=null;deleting=null;$('#taxonomy-new-name').value="";
      state.results=null;state.categories=[];state.tagFilters=[];
      state.scopeTypes=state.scopeTypes.filter((id)=>catalog.note_types.some((t)=>t.id===id));
      state.scopeTags=state.scopeTags.filter((tag)=>catalog.tags.some((t)=>t.name===tag));
      state.stats=await api('/stats');
      if(state.view==='search'||state.view==='notes'){const data=await api('/notes');state.notes=data.items;state.total=data.total;renderLibrary();}
      if($('#taxonomy-summary'))$('#taxonomy-summary').textContent=`${catalog.note_types.length} 个资料类型 · ${catalog.tags.length} 个标签`;
      toast(action==='create'?'已新增':action==='rename'?'名称已更新':'已删除');
    }catch(failure){error.textContent=failure.message;error.hidden=false;}
    finally{taxonomySaving=false;$$('button,input',$('.taxonomy-manager')).forEach((e)=>e.disabled=false);draw();}
  };
  $('#taxonomy-create').onsubmit=(e)=>{e.preventDefault();mutate('create','',$('#taxonomy-new-name').value);};
  $('#taxonomy-search').oninput=(e)=>{query=e.target.value;editing=null;deleting=null;draw();};
  $$('[data-manage-kind]').forEach((b)=>b.onclick=()=>{if(taxonomySaving)return;if(!allowDiscard())return;kind=b.dataset.manageKind;editing=null;deleting=null;query="";$('#taxonomy-search').value="";$('#taxonomy-new-name').value="";error.hidden=true;draw();});
  draw();$('#taxonomy-new-name').focus();
}
