<template>
  <router-view v-if="$route.path === '/login' || $route.meta.popup" />
  <div v-else class="shell" :class="{'sidebar-collapsed':collapsed}">
    <aside ref="sidebarRef">
      <router-link class="brand" to="/"><img class="brand-icon" src="https://raw.githubusercontent.com/dh1888/dh18/main/ico/DHPG.png" alt="TG打卡考勤" /><div>TG打卡考勤<small>考勤管理工作台</small></div></router-link>
      <nav>
        <router-link v-for="item in menus" :key="item.path" :to="item.path" :title="item.label" :data-menu-path="item.path" :class="{'router-link-exact-active':activeMenuPath===item.path}"><el-icon class="menu-icon"><component :is="item.icon" /></el-icon><span class="menu-label">{{item.label}}</span></router-link>
        <button v-if="canUseTools" class="settings-toggle" @click="toggleTools" :aria-expanded="toolsOpen" title="工具"><el-icon class="menu-icon"><Suitcase /></el-icon><span class="menu-label">工具</span><el-icon v-if="!collapsed" class="group-chevron" :class="{'is-open':toolsOpen}"><ArrowRight /></el-icon></button>
        <div v-if="canUseTools" v-show="toolsOpen" class="settings-items">
          <a v-for="item in toolLinks" :key="item.href" :href="item.href" target="_blank" rel="noopener noreferrer" :title="item.label"><el-icon class="menu-icon"><component :is="item.icon" /></el-icon><span class="menu-label">{{item.label}}</span></a>
        </div>
        <button class="settings-toggle" :class="{'group-active':inSettings}" @click="toggleSettings" :aria-expanded="settingsOpen" title="设置"><el-icon class="menu-icon"><Setting /></el-icon><span class="menu-label">设置</span><el-icon v-if="!collapsed" class="group-chevron" :class="{'is-open':settingsOpen}"><ArrowRight /></el-icon></button>
        <div v-show="settingsOpen" class="settings-items">
          <router-link v-for="item in settingsMenus" :key="item.path" :to="item.path" :title="item.label" :data-menu-path="item.path" :class="{'router-link-exact-active':activeMenuPath===item.path}"><el-icon class="menu-icon"><component :is="item.icon" /></el-icon><span class="menu-label">{{item.label}}</span></router-link>
        </div>
      </nav>
    </aside>
    <section class="workspace">
      <header>
        <div class="header-left"><button class="sidebar-toggle" @click="toggleSidebar" :aria-label="collapsed?'展开菜单':'收起菜单'"><el-icon :size="21"><Expand v-if="collapsed"/><Fold v-else/></el-icon></button><span>{{title}}</span></div>
        <div class="header-right"><button v-if="user.role !== 'admin'" @click="bindTelegram">绑定 TG</button><el-dropdown trigger="hover" placement="bottom-end" :show-timeout="80" :hide-timeout="150"><div class="account-trigger"><el-avatar :size="30" :src="user.avatar||'/avatar'"><el-icon><User /></el-icon></el-avatar><span>{{user.username}}</span></div><template #dropdown><el-dropdown-menu><el-dropdown-item :icon="Key" @click="router.push('/password')">个人资料</el-dropdown-item><el-dropdown-item :icon="SwitchButton" divided @click="user.logout">退出登录</el-dropdown-item></el-dropdown-menu></template></el-dropdown></div>
      </header>
      <WorkspaceTabs :key="user.userId" @activate-page="syncSidebarToRoute" />
    </section>
  </div>
</template>
<script setup lang="ts">
import {ElMessage,ElMessageBox} from 'element-plus';
import {Odometer,User,Setting,Tickets,Position,Calendar,Checked,Wallet,Lock,Key,Delete,ArrowRight,Fold,Expand,SwitchButton,Suitcase,Document,Notebook,Picture} from '@element-plus/icons-vue';
import WorkspaceTabs from './components/WorkspaceTabs.vue';
import request from './api/request';
import {computed,ref,watch,nextTick,onMounted,onUnmounted} from 'vue';
import {useRoute,useRouter} from 'vue-router';
import {useUserStore,canOpenPage} from './store/user';
const user=useUserStore(),route=useRoute(),router=useRouter();
function receivePermissions(event:Event){const next=(event as CustomEvent).detail;if(JSON.stringify(next)!==JSON.stringify(user.permissions))user.permissions=next}
async function refreshIdentity(){if(user.isAuthenticated&&document.visibilityState==='visible'&&!(await user.validateSession()))void router.replace('/login')}
let permissionTimer:ReturnType<typeof setInterval>|undefined;
onMounted(()=>{window.addEventListener('tg-permissions',receivePermissions);window.addEventListener('focus',refreshIdentity);permissionTimer=setInterval(refreshIdentity,30000)});
onUnmounted(()=>{window.removeEventListener('tg-permissions',receivePermissions);window.removeEventListener('focus',refreshIdentity);if(permissionTimer)clearInterval(permissionTimer)});

// 新窗口（如后台打卡小窗口）首次加载时，路由守卫还在解析目标页，此时 route 仍是默认的 '/'。
// 权限一到就按 '/' 判断会把没有首页权限的子账号误踢到 /password，所以路由未解析完成前不判断。
watch(()=>user.permissions,()=>{if(!route.matched.length)return;if(user.isAuthenticated&&route.path!=='/login'&&!canOpenPage(route.path))router.replace('/password')},{deep:true});
function stored(key:string,fallback:boolean){try{const v=localStorage.getItem(key);return v===null?fallback:v==='true'}catch{return fallback}}
function persist(key:string,value:boolean){try{localStorage.setItem(key,String(value))}catch{}}
const collapsed=ref(stored('tg-sidebar-collapsed',false));
type MenuGroup = 'settings' | 'tools' | '';
function restoredMenuGroup(): MenuGroup {
  try { const value=localStorage.getItem('tg-open-menu-group'); if(value==='settings'||value==='tools'||value==='')return value; } catch {}
  return 'settings';
}
const openMenuGroup=ref<MenuGroup>(restoredMenuGroup());
const settingsOpen=computed(()=>openMenuGroup.value==='settings');
const toolsOpen=computed(()=>openMenuGroup.value==='tools');
const canUseTools=computed(()=>user.hasPermission('tools:view'));
function setOpenMenuGroup(group:MenuGroup){openMenuGroup.value=group;try{localStorage.setItem('tg-open-menu-group',group)}catch{}}
function toggleSidebar(){collapsed.value=!collapsed.value;persist('tg-sidebar-collapsed',collapsed.value)}
function toggleSettings(){setOpenMenuGroup(settingsOpen.value?'':'settings')}
function toggleTools(){if(canUseTools.value)setOpenMenuGroup(toolsOpen.value?'':'tools')}
watch(canUseTools,allowed=>{if(!allowed&&toolsOpen.value)setOpenMenuGroup('')},{immediate:true});
// 工具：外部网页，点击后在新标签页打开
const toolLinks=[{href:'https://dh1888.github.io/168/main.html',label:'读取文件',icon:Document},{href:'https://dh1888.github.io/168/readerText.html',label:'读取TXT文件',icon:Notebook},{href:'https://dhmubai1688-ai.github.io/ll18/',label:'图片编辑',icon:Picture}];
const allMenus=[{path:'/',label:'首页概览',icon:Odometer},{path:'/telegram-punch',label:'后台打卡',icon:Position},{path:'/telegram-records',label:'打卡记录',icon:Tickets},{path:'/attendance',label:'考勤明细',icon:Calendar},{path:'/penalty-details',label:'罚款详情',icon:Wallet},{path:'/attendance-review',label:'待确认审批',icon:Checked}];
const settingItems=[{path:'/telegram',label:'打卡规则',icon:Setting},{path:'/accounts',label:'子账号管理',icon:User},{path:'/roles',label:'角色管理',icon:Lock},{path:'/password',label:'个人资料',icon:User},{path:'/memory',label:'清理内存',icon:Delete}];
const menus=computed(()=>allMenus.filter(x=>canOpenPage(x.path))),settingsMenus=computed(()=>settingItems.filter(x=>canOpenPage(x.path)));
const sidebarRef=ref<HTMLElement|null>(null);
const activeMenuPath=computed(()=>{
  const record=route.matched[route.matched.length-1];
  return record?.aliasOf?.path||route.path;
});
const inSettings=computed(()=>settingItems.some(x=>x.path===activeMenuPath.value));
async function syncSidebarToRoute(){
  if(!route.matched.length||route.path==='/login'||route.meta.popup)return;
  setOpenMenuGroup(inSettings.value?'settings':'');
  await nextTick();
  const item=sidebarRef.value?.querySelector<HTMLElement>(`[data-menu-path="${activeMenuPath.value}"]`);
  item?.scrollIntoView({block:'nearest',inline:'nearest'});
}
watch(()=>route.fullPath,syncSidebarToRoute,{immediate:true});
const title=computed(()=>[...allMenus,...settingItems].find(x=>x.path===activeMenuPath.value)?.label||'TG打卡考勤');
async function bindTelegram(){try{await ElMessageBox.prompt('请输入你的数字 Telegram 用户 ID，保存后绑定到当前子账号。','绑定 TG',{inputPlaceholder:'Telegram 用户 ID',inputValidator:v=>/^[1-9]\d{0,15}$/.test((v||'').trim())||'请输入有效的数字 TG ID',confirmButtonText:'保存绑定',closeOnClickModal:false,beforeClose:async(action,instance,done)=>{if(action!=='confirm'){done();return}instance.confirmButtonLoading=true;try{await request.post('/standalone/bind-telegram',{telegramUserId:instance.inputValue.trim()});ElMessage.success('TG 绑定成功');done()}catch(e:any){ElMessage.error(e.message||'绑定失败')}finally{instance.confirmButtonLoading=false}}})}catch{}}

</script>
<style>
* {
  box-sizing: border-box;
}
body {
  margin: 0;
  font-family:
    Inter,
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    sans-serif;
  color: #233146;
  background: #f5f7fa;
}
a {
  color: inherit;
  text-decoration: none;
}
button {
  cursor: pointer;
}
.shell {
  --workspace-bar-height: 52px;
  --sidebar-width: 228px;
  display: flex;
  min-height: 100vh;
}
aside {
  width: var(--sidebar-width);
  background: #102b2d;
  color: #b8cbca;
  position: fixed;
  inset: 0 auto 0 0;
  padding: 30px 19px;
  display: flex;
  flex-direction: column;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  color: white;
  font-weight: 650;
  font-size: 17px;
}
.brand small {
  display: block;
  font-size: 11px;
  color: #8da9a7;
  font-weight: 400;
  margin-top: 6px;
}
.brand-icon {
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  border-radius: 10px;
  object-fit: contain;
  background: #fff;
}
nav {
  margin-top: 40px;
  display: grid;
  gap: 6px;
}
nav a {
  padding: 13px;
  border-radius: 8px;
  font-size: 15px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.menu-icon { font-size: 20px; flex: 0 0 20px; }
nav a:hover {
  background: #1a3c3d;
}
nav a.router-link-exact-active {
  background: #254b46;
  color: #d6f4cb;
}
.dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #b9edbc;
  margin-right: 5px;
}
.workspace {
  background: radial-gradient(ellipse at 15% 10%, #229ed926, transparent 50%), radial-gradient(ellipse at 85% 35%, #8b5cf621, transparent 50%), radial-gradient(ellipse at 50% 90%, #f59e0b19, transparent 50%), #f5f7fa;
  background-attachment: fixed;
  margin-left: var(--sidebar-width);
  min-width: 0;
  flex: 1;
}
.workspace > header {
  height: var(--workspace-bar-height);
  position: sticky;
  top: 0;
  z-index: 100;
  background: white;
  border-bottom: 1px solid #e6ebee;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 34px;
  font-size: 14px;
}
.header-right {
  display: flex;
  gap: 20px;
  align-items: center;
  font-size: 12px;
  color: #71808e;
}
.header-right button {
  border: 0;
  background: transparent;
  color: #45615c;
}
main {
  padding: 30px;
  max-width: 1650px;
  margin: auto;
}
.workspace .bg-decoration { display: none !important; }
.workspace .attendance-management { background: transparent !important; }
footer {
  font-size: 10px;
  letter-spacing: 0.5px;
  color: #89979c;
  padding: 15px 32px 25px;
}
.el-button--primary {
  --el-button-bg-color: #205c50;
  --el-button-border-color: #205c50;
  --el-button-hover-bg-color: #347567;
  --el-button-hover-border-color: #347567;
}
.muted {
  color: #819096;
  font-size: 13px;
  line-height: 1.8;
}
.panel {
  background: white;
  border: 1px solid #e3e9eb;
  border-radius: 13px;
  padding: 24px;
  margin-top: 24px;
}
.panel h3 {
  margin: 0 0 20px;
  font-size: 15px;
}
.el-table {
  font-size: 12px !important;
}
.sidebar-collapsed{--sidebar-width:72px}
.sidebar-collapsed aside{padding:24px 9px}
.sidebar-collapsed .brand div,.sidebar-collapsed .menu-label{display:none}
.sidebar-collapsed nav a,.sidebar-collapsed .settings-toggle{justify-content:center;padding:13px}
.sidebar-collapsed .settings-items{padding-left:0}
aside{overflow-y:auto;scrollbar-width:none;z-index:110}aside::-webkit-scrollbar{display:none}
.settings-toggle{display:flex;align-items:center;gap:12px;width:100%;padding:13px;border:0;background:transparent;color:inherit;font-size:15px;border-radius:8px;text-align:left}
.settings-toggle:hover,.settings-toggle.group-active{background:#1a3c3d;color:#d6f4cb}
.group-chevron{margin-left:auto;transition:transform .2s}.group-chevron.is-open{transform:rotate(90deg)}
.settings-items{display:grid;gap:5px;padding-left:10px}
.account-trigger{display:flex;align-items:center;gap:8px;cursor:pointer;padding:4px 8px;border-radius:8px;color:#45615c;outline:none}.account-trigger:hover{background:#f0f5f3}.account-trigger span{font-size:13px}
.header-left{display:flex;align-items:center;gap:14px}.sidebar-toggle{border:0;background:transparent;color:#45615c;display:flex;padding:6px}
@media(max-width:900px){main{padding:16px}.workspace>header{padding:0 12px}.header-right{gap:8px}.header-right>span{display:none}}

/* ================= 抽屉统一风格（cool-drawer） ================= */
.cool-drawer-mask{background:rgba(10,28,32,.10)!important;-webkit-backdrop-filter:none;backdrop-filter:none}
.el-drawer.cool-drawer{
  --cd-a:#205c50;--cd-b:#229ed9;--cd-c:#8b5cf6;
  position:absolute;overflow:hidden;right:0;
  background:radial-gradient(120% 60% at 100% 0%,rgba(34,158,217,.13),transparent 60%),radial-gradient(90% 50% at 0% 100%,rgba(139,92,246,.10),transparent 60%),rgba(255,255,255,.92);
  -webkit-backdrop-filter:blur(22px) saturate(170%);backdrop-filter:blur(22px) saturate(170%);
  border-radius:22px 0 0 22px;border-left:1px solid rgba(255,255,255,.7);
  box-shadow:-30px 0 80px -24px rgba(16,43,45,.55),-2px 0 0 rgba(34,158,217,.18);
}
.el-drawer.cool-drawer::before{
  content:"";position:absolute;left:0;right:0;top:0;height:3px;z-index:3;pointer-events:none;
  background:linear-gradient(90deg,var(--cd-a),var(--cd-b),var(--cd-c),var(--cd-b),var(--cd-a));background-size:200% 100%;
  animation:cd-flow 6s linear infinite;
}
.el-drawer.cool-drawer .el-drawer__header{
  margin:0;padding:22px 26px 18px;align-items:center;
  border-bottom:1px solid rgba(32,92,80,.10);
  background:linear-gradient(120deg,rgba(32,92,80,.07),rgba(34,158,217,.07) 55%,rgba(139,92,246,.06));
}
.el-drawer.cool-drawer .el-drawer__header>:first-child,.el-drawer.cool-drawer .el-drawer__title{
  font-size:18px;font-weight:700;letter-spacing:.4px;
  background:linear-gradient(110deg,var(--cd-a),var(--cd-b) 60%,var(--cd-c));
  -webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent;color:transparent;
}
.el-drawer.cool-drawer .el-drawer__close-btn{
  width:34px;height:34px;border-radius:50%;display:grid;place-items:center;color:#5b6f75;
  background:rgba(255,255,255,.7);border:1px solid rgba(32,92,80,.12);
  transition:transform .35s cubic-bezier(.2,.8,.2,1),background .25s,color .25s,box-shadow .25s;
}
.el-drawer.cool-drawer .el-drawer__close-btn:hover{transform:rotate(90deg) scale(1.08);color:#fff;background:linear-gradient(135deg,var(--cd-a),var(--cd-b));box-shadow:0 8px 18px -6px rgba(34,158,217,.7)}
.el-drawer.cool-drawer .el-drawer__body{padding:22px 26px;scrollbar-width:thin;scrollbar-color:#229ed9 transparent}
.el-drawer.cool-drawer .el-drawer__body::-webkit-scrollbar{width:6px}
.el-drawer.cool-drawer .el-drawer__body::-webkit-scrollbar-thumb{border-radius:6px;background:linear-gradient(180deg,var(--cd-b),var(--cd-c))}
.el-drawer.cool-drawer .el-drawer__footer{
  padding:16px 26px;border-top:1px solid rgba(32,92,80,.10);
  background:linear-gradient(180deg,rgba(246,250,251,.55),rgba(246,250,251,.95));
  display:flex;justify-content:flex-end;gap:10px;
}
.el-drawer.cool-drawer .el-drawer__footer .dialog-footer{display:flex;justify-content:flex-end;gap:10px;width:100%}
.el-drawer.cool-drawer .el-form-item__label{font-weight:600;color:#35505a}
.el-drawer.cool-drawer .el-input__wrapper,.el-drawer.cool-drawer .el-select__wrapper,.el-drawer.cool-drawer .el-textarea__inner,.el-drawer.cool-drawer .el-input-number .el-input__wrapper{
  border-radius:11px;background:rgba(255,255,255,.85);box-shadow:0 0 0 1px rgba(32,92,80,.16) inset;transition:box-shadow .25s,transform .25s,background .25s;
}
.el-drawer.cool-drawer .el-input__wrapper:hover,.el-drawer.cool-drawer .el-select__wrapper:hover,.el-drawer.cool-drawer .el-textarea__inner:hover{box-shadow:0 0 0 1px rgba(34,158,217,.55) inset}
.el-drawer.cool-drawer .el-input__wrapper.is-focus,.el-drawer.cool-drawer .el-select__wrapper.is-focused,.el-drawer.cool-drawer .el-textarea__inner:focus{
  background:#fff;box-shadow:0 0 0 1.5px #229ed9 inset,0 8px 20px -10px rgba(34,158,217,.65),0 0 0 4px rgba(34,158,217,.12);
}
.el-drawer.cool-drawer .el-button{border-radius:11px;font-weight:600;transition:transform .2s,box-shadow .25s,background .25s}
.el-drawer.cool-drawer .el-button:not(.is-link):hover{transform:translateY(-1px)}
.el-drawer.cool-drawer .el-button--primary:not(.is-link){
  border:0;background:linear-gradient(120deg,#205c50 0%,#1f7a78 48%,#229ed9 100%);
  box-shadow:0 10px 22px -10px rgba(34,158,217,.8);
}
.el-drawer.cool-drawer .el-button--primary:not(.is-link):hover{box-shadow:0 14px 28px -10px rgba(34,158,217,.95),0 0 18px -2px rgba(34,158,217,.45)}
.el-drawer.cool-drawer .el-checkbox__input.is-checked .el-checkbox__inner,.el-drawer.cool-drawer .el-radio-button__original-radio:checked+.el-radio-button__inner{
  background:linear-gradient(135deg,#205c50,#229ed9);border-color:transparent;box-shadow:0 8px 18px -8px rgba(34,158,217,.8);
}
.el-drawer.cool-drawer .el-radio-button__inner{border-radius:10px!important;border:1px solid rgba(32,92,80,.16)!important;margin-right:6px;padding:9px 22px;font-weight:600}
.el-drawer.cool-drawer .el-radio-button:first-child .el-radio-button__inner,.el-drawer.cool-drawer .el-radio-button:last-child .el-radio-button__inner{border-radius:10px!important}
.el-drawer.cool-drawer .el-form-item,.el-drawer.cool-drawer .batch-add-toolbar,.el-drawer.cool-drawer .batch-add-grid{animation:cd-rise .5s cubic-bezier(.2,.8,.2,1) both}
.el-drawer.cool-drawer .el-form-item:nth-child(1){animation-delay:.05s}.el-drawer.cool-drawer .el-form-item:nth-child(2){animation-delay:.09s}
.el-drawer.cool-drawer .el-form-item:nth-child(3){animation-delay:.13s}.el-drawer.cool-drawer .el-form-item:nth-child(4){animation-delay:.17s}
.el-drawer.cool-drawer .el-form-item:nth-child(5){animation-delay:.21s}.el-drawer.cool-drawer .el-form-item:nth-child(n+6){animation-delay:.25s}
/* 批量添加员工：行卡片 */
.el-drawer.cool-drawer .batch-add-toolbar{padding:12px 14px;border-radius:14px;background:linear-gradient(120deg,rgba(32,92,80,.07),rgba(34,158,217,.09));border:1px solid rgba(34,158,217,.18)}
.el-drawer.cool-drawer .batch-add-grid:not(.batch-add-head){padding:10px;border-radius:14px;background:rgba(255,255,255,.7);border:1px solid rgba(32,92,80,.10);transition:box-shadow .25s,transform .25s,border-color .25s}
.el-drawer.cool-drawer .batch-add-grid:not(.batch-add-head):hover{transform:translateY(-1px);border-color:rgba(34,158,217,.4);box-shadow:0 12px 26px -16px rgba(34,158,217,.7)}
.el-drawer.cool-drawer .batch-add-grid.batch-add-error{border-color:rgba(245,108,108,.6);background:rgba(245,108,108,.06)}
@keyframes cd-flow{to{background-position:200% 0}}
@keyframes cd-rise{from{opacity:0;transform:translateY(10px)}to{opacity:1;transform:none}}
@media(max-width:900px){.el-drawer.cool-drawer{width:100%!important;border-radius:0}}
@media(prefers-reduced-motion:reduce){.el-drawer.cool-drawer::before,.el-drawer.cool-drawer .el-form-item,.el-drawer.cool-drawer .batch-add-toolbar,.el-drawer.cool-drawer .batch-add-grid{animation:none}}

/* ================= 抽屉进阶特效 ================= */
.cool-drawer-mask{background:radial-gradient(70% 90% at 100% 50%,rgba(34,158,217,.10),transparent 60%),rgba(8,24,28,.10)!important}
/* 弹簧滑入 / 快速滑出 */
.drawer-fade-enter-active .el-drawer.cool-drawer,.el-drawer-fade-enter-active .el-drawer.cool-drawer{animation:cd-slide-in .42s cubic-bezier(.22,1,.36,1) both!important}
.drawer-fade-leave-active .el-drawer.cool-drawer,.el-drawer-fade-leave-active .el-drawer.cool-drawer{animation:cd-slide-out .28s cubic-bezier(.4,0,1,1) both!important}
/* 霓虹边缘 + 外发光 */
.el-drawer.cool-drawer{box-shadow:-30px 0 80px -24px rgba(16,43,45,.6),-1px 0 0 rgba(34,158,217,.75),-8px 0 38px -6px rgba(34,158,217,.38),-18px 0 60px -14px rgba(139,92,246,.28)!important}
/* 极光漂移背景 */
.el-drawer.cool-drawer::after{
  content:"";position:absolute;inset:-20%;z-index:0;pointer-events:none;opacity:.9;
  background:radial-gradient(38% 30% at 25% 20%,rgba(34,158,217,.16),transparent 70%),radial-gradient(34% 28% at 80% 55%,rgba(139,92,246,.14),transparent 70%),radial-gradient(40% 30% at 30% 90%,rgba(32,92,80,.14),transparent 70%);
  animation:cd-aurora 14s ease-in-out infinite alternate;
}
.el-drawer.cool-drawer .el-drawer__header,.el-drawer.cool-drawer .el-drawer__body,.el-drawer.cool-drawer .el-drawer__footer{position:relative;z-index:1}
/* 标题前的呼吸光点 */
.el-drawer.cool-drawer .el-drawer__title::before,.el-drawer.cool-drawer .el-drawer__header>:first-child::before{
  content:"";display:inline-block;width:9px;height:9px;margin-right:11px;border-radius:50%;vertical-align:middle;
  background:linear-gradient(135deg,#229ed9,#8b5cf6);box-shadow:0 0 0 0 rgba(34,158,217,.55);animation:cd-pulse 2.2s ease-out infinite;
}
/* 表头光带扫过 */
.el-drawer.cool-drawer .el-drawer__header{overflow:hidden}
.el-drawer.cool-drawer .el-drawer__header::after{
  content:"";position:absolute;top:0;bottom:0;left:-40%;width:35%;pointer-events:none;
  background:linear-gradient(100deg,transparent,rgba(255,255,255,.75),transparent);transform:skewX(-20deg);
  animation:cd-sweep 1.4s .35s ease-out 1 both;
}
/* 主按钮：悬停流光 */
.el-drawer.cool-drawer .el-button--primary:not(.is-link){position:relative;overflow:hidden}
.el-drawer.cool-drawer .el-button--primary:not(.is-link)::after{
  content:"";position:absolute;top:0;bottom:0;left:-60%;width:40%;pointer-events:none;
  background:linear-gradient(100deg,transparent,rgba(255,255,255,.55),transparent);transform:skewX(-20deg);transition:left .6s ease;
}
.el-drawer.cool-drawer .el-button--primary:not(.is-link):hover::after{left:130%}
/* 输入聚焦：渐变光环呼吸 */
.el-drawer.cool-drawer .el-input__wrapper.is-focus,.el-drawer.cool-drawer .el-select__wrapper.is-focused{animation:cd-ring 1.8s ease-in-out infinite}
/* 批量添加：行序号徽标 */
.el-drawer.cool-drawer .batch-add-scroll{counter-reset:cdrow;padding-top:4px}
.el-drawer.cool-drawer .batch-add-grid:not(.batch-add-head){counter-increment:cdrow;position:relative}
.el-drawer.cool-drawer .batch-add-grid:not(.batch-add-head)::before{
  content:counter(cdrow);position:absolute;left:-9px;top:-8px;min-width:20px;height:20px;padding:0 5px;border-radius:10px;
  display:grid;place-items:center;font-size:11px;font-weight:700;color:#fff;z-index:2;
  background:linear-gradient(135deg,#205c50,#229ed9);box-shadow:0 6px 12px -4px rgba(34,158,217,.8);
}
/* 导出/导入切换：选中项发光 */
.el-drawer.cool-drawer .el-radio-button__original-radio:checked+.el-radio-button__inner{color:#fff;box-shadow:0 10px 22px -8px rgba(34,158,217,.9),0 0 18px -2px rgba(34,158,217,.5)}
@keyframes cd-slide-in{from{transform:translateX(100%)}to{transform:translateX(0)}}
@keyframes cd-slide-out{from{transform:translateX(0)}to{transform:translateX(100%)}}
@keyframes cd-aurora{0%{transform:translate3d(-3%,-2%,0) rotate(0deg)}100%{transform:translate3d(4%,3%,0) rotate(8deg)}}
@keyframes cd-pulse{0%{box-shadow:0 0 0 0 rgba(34,158,217,.6)}80%,100%{box-shadow:0 0 0 10px rgba(34,158,217,0)}}
@keyframes cd-sweep{from{left:-40%}to{left:140%}}
@keyframes cd-ring{0%,100%{box-shadow:0 0 0 1.5px #229ed9 inset,0 8px 20px -10px rgba(34,158,217,.65),0 0 0 4px rgba(34,158,217,.10)}50%{box-shadow:0 0 0 1.5px #8b5cf6 inset,0 8px 24px -8px rgba(139,92,246,.6),0 0 0 6px rgba(139,92,246,.12)}}
@media(prefers-reduced-motion:reduce){.el-drawer.cool-drawer::after,.el-drawer.cool-drawer .el-drawer__header::after,.el-drawer.cool-drawer .el-drawer__title::before,.el-drawer.cool-drawer .el-input__wrapper.is-focus,.el-drawer.cool-drawer .el-select__wrapper.is-focused{animation:none}}
</style>
