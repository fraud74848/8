<template>
  <div class="page-tabs" role="tablist" aria-label="已打开页面">
    <transition-group name="tab" tag="div" class="tabs-track" ref="trackRef" @wheel.prevent="onWheel">
      <div v-for="tab in tabs" :key="tab.path" :data-path="tab.path" class="page-tab" :class="{active: tab.path===activePath}">
        <button class="tab-main" role="tab" :aria-selected="tab.path===activePath" @click="activateTab(tab.fullPath)">
          <el-icon class="tab-icon" :size="15"><component :is="icons[tab.path]||Document" /></el-icon>
          <span class="tab-title">{{tab.title}}</span>
        </button>
        <button :disabled="tabs.length<2" class="tab-close" :aria-label="`关闭${tab.title}`" @click.stop="close(tab.path)">
          <svg viewBox="0 0 12 12" width="10" height="10" aria-hidden="true"><path d="M2 2l8 8M10 2l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" fill="none"/></svg>
        </button>
      </div>
    </transition-group>
    <el-dropdown class="tab-actions" trigger="click" placement="bottom-end" @command="onCommand" @visible-change="menuOpen=$event">
      <button class="tab-menu" :class="{'is-open':menuOpen}" aria-label="标签页操作" title="标签页操作"><el-icon :size="16"><ArrowDown /></el-icon></button>
      <template #dropdown><el-dropdown-menu>
        <el-dropdown-item command="refresh" :icon="Refresh">刷新</el-dropdown-item>
        <el-dropdown-item command="current" :icon="Close" :disabled="tabs.length<2">关闭当前</el-dropdown-item>
        <el-dropdown-item command="others" :icon="CircleClose" :disabled="tabs.length<2">关闭其他</el-dropdown-item>
        <el-dropdown-item command="all" :icon="Delete" divided :disabled="!tabs.length">全部关闭</el-dropdown-item>
      </el-dropdown-menu></template>
    </el-dropdown>
  </div>
  <main><router-view v-slot="{ Component, route: pageRoute }">
    <keep-alive :include="cachedNames"><component :is="Component" :key="canonical(pageRoute)+':'+user.permissions.join(',')+':'+(refreshKeys[canonical(pageRoute)]||0)" /></keep-alive>
  </router-view></main>
</template>
<script setup lang="ts">
import {computed,nextTick,ref,watch} from 'vue';
import {useRoute,useRouter,type RouteLocationNormalizedLoaded} from 'vue-router';
import {useUserStore,canOpenPage} from '../store/user';
import {ArrowDown,Refresh,Close,CircleClose,Delete,Document,Odometer,User,Setting,Tickets,Position,Calendar,Checked,Wallet,Lock,Key,TrendCharts} from '@element-plus/icons-vue';
const route=useRoute(),router=useRouter(),user=useUserStore();
const emit=defineEmits<{(event:'activate-page'):void}>();
async function activateTab(fullPath:string){await router.push(fullPath);emit('activate-page')}
const pages:Record<string,[string,string]>={
 '/roles':['角色管理','Accounts'],'/password':['个人资料','Accounts'],'/memory':['清理内存','Accounts'],
 '/':['首页概览','Overview'],'/accounts':['子账号管理','Accounts'],
 '/telegram':['打卡规则','TelegramAttendance'],'/telegram-records':['打卡记录','TelegramRecords'],
 '/telegram-punch':['后台打卡','TelegramActivityPunch'],'/attendance':['考勤明细','AttendanceRecords'],
 '/attendance-review':['待确认审批','AttendanceReview'],'/penalty-details':['罚款详情','PenaltyDetails'],
 '/attendance/performance':['员工绩效','PerformanceReview']
};
const icons:Record<string,any>={
 '/':Odometer,'/telegram-punch':Position,'/telegram-records':Tickets,'/attendance':Calendar,'/penalty-details':Wallet,
 '/attendance-review':Checked,'/attendance/performance':TrendCharts,'/telegram':Setting,'/accounts':User,'/roles':Lock,'/password':Key,'/memory':Delete
};
const menuOpen=ref(false),trackRef=ref<any>(null);
function trackEl():HTMLElement|null{const r=trackRef.value;return r?(r.$el||r):null}
function onWheel(e:WheelEvent){const el=trackEl();if(el)el.scrollLeft+=Math.abs(e.deltaX)>Math.abs(e.deltaY)?e.deltaX:e.deltaY}
type PageTab={path:string;fullPath:string;title:string};
const tabs=ref<PageTab[]>([]);
function canonical(r:RouteLocationNormalizedLoaded){const record=r.matched[r.matched.length-1];return record?.aliasOf?.path || r.path}
const activePath=computed(()=>canonical(route));
watch([activePath,()=>tabs.value.length],async()=>{
 await nextTick();
 const el=trackEl();const node=el?.querySelector<HTMLElement>('.page-tab.active');
 node?.scrollIntoView({behavior:'smooth',block:'nearest',inline:'nearest'});
},{immediate:true,flush:'post'});
const cachedNames=computed(()=>tabs.value.map(t=>pages[t.path]?.[1]).filter(name=>!!name && name!=='Accounts'));
function allowed(path:string){return !!pages[path] && canOpenPage(path)}
watch(()=>user.permissions,()=>{tabs.value=tabs.value.filter(t=>allowed(t.path))},{deep:true});
watch(()=>user.userId,()=>{
 tabs.value=[];
 try{const saved=JSON.parse(sessionStorage.getItem('attendance-tabs:'+user.userId)||'[]');if(Array.isArray(saved))tabs.value=saved.filter(t=>typeof t.path==='string'&&allowed(t.path)).map(t=>({path:t.path,fullPath:t.path,title:pages[t.path][0]}));}catch{}
},{immediate:true});
watch(()=>[route.fullPath,user.userId],()=>{
 const path=canonical(route);if(!allowed(path))return;
 const tab=tabs.value.find(t=>t.path===path);
 if(tab)tab.fullPath=route.fullPath;else tabs.value.push({path,fullPath:route.fullPath,title:pages[path][0]});
},{immediate:true});
watch(tabs,t=>{try{sessionStorage.setItem('attendance-tabs:'+user.userId,JSON.stringify(t.map(x=>({path:x.path}))));}catch{}},{deep:true});
async function close(path:string){
 if(tabs.value.length<2)return;
 const index=tabs.value.findIndex(t=>t.path===path);
 if(path===activePath.value){const next=tabs.value[index-1]||tabs.value[index+1];await router.push(next.fullPath);}
 tabs.value=tabs.value.filter(t=>t.path!==path);
}
function closeOthers(){tabs.value=tabs.value.filter(t=>t.path===activePath.value)}
const refreshKeys=ref<Record<string,number>>({});
function refreshCurrent(){const p=activePath.value;refreshKeys.value={...refreshKeys.value,[p]:(refreshKeys.value[p]||0)+1}}
async function closeAll(){
 tabs.value=[];
 const home=['/','/password'].find(p=>canOpenPage(p))||'/password';
 if(canonical(route)===home){const t=pages[home];tabs.value=[{path:home,fullPath:home,title:t[0]}];refreshCurrent()}
 else await router.push(home);
}
function onCommand(cmd:string){
 if(cmd==='refresh')refreshCurrent();
 else if(cmd==='current')void close(activePath.value);
 else if(cmd==='others')closeOthers();
 else if(cmd==='all')void closeAll();
}
</script>
<style scoped>
.page-tabs{
  display:flex;align-items:center;gap:10px;padding:0 24px;
  height:var(--workspace-bar-height);position:sticky;top:var(--workspace-bar-height);z-index:99;
  background:linear-gradient(180deg,rgba(255,255,255,.92),rgba(246,250,251,.82));
  -webkit-backdrop-filter:blur(16px) saturate(170%);backdrop-filter:blur(16px) saturate(170%);
  border-bottom:1px solid rgba(32,92,80,.10);
  box-shadow:0 8px 24px -18px rgba(16,43,45,.45);
}
/* 底部流光细线 */
.page-tabs::after{
  content:"";position:absolute;left:0;right:0;bottom:-1px;height:1px;pointer-events:none;
  background:linear-gradient(90deg,transparent,#229ed9 22%,#8b5cf6 50%,#f59e0b 78%,transparent);
  opacity:.55;background-size:200% 100%;animation:tab-flow 8s linear infinite;
}
.tabs-track{
  display:flex;align-items:center;gap:8px;flex:1;min-width:0;height:100%;
  overflow-x:auto;overflow-y:hidden;scrollbar-width:none;position:relative;
  -webkit-mask-image:linear-gradient(90deg,transparent 0,#000 14px,#000 calc(100% - 14px),transparent 100%);
  mask-image:linear-gradient(90deg,transparent 0,#000 14px,#000 calc(100% - 14px),transparent 100%);
  padding:0 6px;
}
.tabs-track::-webkit-scrollbar{display:none}

.page-tab{
  position:relative;display:flex;align-items:center;flex-shrink:0;height:34px;overflow:hidden;
  border-radius:11px;border:1px solid rgba(32,92,80,.12);
  background:rgba(255,255,255,.62);color:#5b6f75;
  transition:transform .22s cubic-bezier(.2,.8,.2,1),background .25s,box-shadow .25s,border-color .25s,color .25s;
}
.page-tab:hover{
  transform:translateY(-1px);background:#fff;border-color:rgba(34,158,217,.45);
  box-shadow:0 6px 14px -6px rgba(34,158,217,.45);color:#205c50;
}
.tab-main{
  display:flex;align-items:center;gap:7px;border:0;background:transparent;color:inherit;
  padding:0 6px 0 13px;height:100%;white-space:nowrap;font-size:13px;font-weight:500;letter-spacing:.2px;outline:none;
}
.tab-icon{opacity:.75;transition:transform .25s,opacity .25s}
.page-tab:hover .tab-icon{transform:scale(1.15) rotate(-6deg);opacity:1}

/* 激活态：青 → 蓝渐变 + 光晕 + 流光扫过 */
.page-tab.active{
  color:#fff;border-color:transparent;transform:translateY(-1px);
  background:linear-gradient(120deg,#205c50 0%,#1f7a78 45%,#229ed9 100%);
  box-shadow:0 8px 18px -6px rgba(34,158,217,.65),0 0 0 1px rgba(255,255,255,.18) inset,0 0 18px -2px rgba(34,158,217,.35);
}
.page-tab.active::before{
  content:"";position:absolute;inset:0;pointer-events:none;
  background:linear-gradient(105deg,transparent 35%,rgba(255,255,255,.38) 50%,transparent 65%);
  transform:translateX(-120%);animation:tab-sheen 3.8s ease-in-out infinite;
}
.page-tab.active::after{
  content:"";position:absolute;left:14px;right:14px;bottom:0;height:2px;border-radius:2px;
  background:linear-gradient(90deg,#b9edbc,#fff,#b9edbc);box-shadow:0 0 8px 1px rgba(185,237,188,.9);
}
.page-tab.active .tab-icon{opacity:1;filter:drop-shadow(0 0 4px rgba(255,255,255,.75))}

.tab-close{
  position:relative;z-index:1;display:flex;align-items:center;justify-content:center;
  width:20px;height:20px;margin:0 7px 0 2px;padding:0;border:0;border-radius:50%;
  background:transparent;color:currentColor;opacity:0;transform:scale(.6);pointer-events:none;
  transition:opacity .18s,transform .22s cubic-bezier(.2,.8,.2,1),background .15s;
}
.page-tab:hover .tab-close,.page-tab.active .tab-close,.page-tab:focus-within .tab-close{opacity:.85;transform:scale(1);pointer-events:auto}
.tab-close:hover{opacity:1;background:rgba(244,63,94,.16);color:#f43f5e;transform:rotate(90deg) scale(1.1)}
.page-tab.active .tab-close:hover{background:rgba(255,255,255,.3);color:#fff}
.tab-close:disabled{display:none}
.tab-main:focus-visible{box-shadow:0 0 0 2px #fff,0 0 0 4px #229ed9;border-radius:10px}

.tab-actions{flex-shrink:0}
.tab-menu{
  display:flex;align-items:center;justify-content:center;width:34px;height:34px;border-radius:11px;outline:none;
  border:1px solid rgba(32,92,80,.14);background:rgba(255,255,255,.7);color:#5b6f75;
  transition:all .22s cubic-bezier(.2,.8,.2,1);
}
.tab-menu .el-icon{transition:transform .25s}
.tab-menu:hover,.tab-menu.is-open{
  color:#fff;border-color:transparent;transform:translateY(-1px);
  background:linear-gradient(135deg,#205c50,#229ed9);box-shadow:0 6px 14px -5px rgba(34,158,217,.6);
}
.tab-menu.is-open .el-icon{transform:rotate(180deg)}

/* 标签进出场动画 */
.tab-enter-active{transition:opacity .28s,transform .32s cubic-bezier(.2,.9,.3,1.3)}
.tab-leave-active{transition:opacity .2s,transform .2s;position:absolute}
.tab-move{transition:transform .28s cubic-bezier(.2,.8,.2,1)}
.tab-enter-from{opacity:0;transform:translateY(10px) scale(.85)}
.tab-leave-to{opacity:0;transform:scale(.8)}

@keyframes tab-sheen{0%,55%{transform:translateX(-120%)}100%{transform:translateX(120%)}}
@keyframes tab-flow{to{background-position:200% 0}}
@media(prefers-reduced-motion:reduce){
  .page-tabs::after,.page-tab.active::before{animation:none}
  .page-tab,.tab-close,.tab-menu,.tab-icon{transition:none}
}
@media(max-width:900px){.page-tabs{padding:0 10px;gap:6px}.tab-main{padding-left:10px;font-size:12px}}
</style>
