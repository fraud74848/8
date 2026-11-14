<template>
  <div class="telegram-records-page attendance-management">
    <div class="bg-decoration" aria-hidden="true">
      <div class="blob blob-1 tg-rec-blob-1"></div>
      <div class="blob blob-2 tg-rec-blob-2"></div>
      <div class="blob blob-3 tg-rec-blob-3"></div>
    </div>

    <div class="tg-rec-content">
      <el-alert
        v-if="!moduleEnabled"
        type="info"
        show-icon
        :closable="false"
        class="tg-alert"
        title="Telegram 考勤模块未启用"
      />

      <el-card class="tg-rec-main-card" shadow="never">

        <el-tabs v-model="activeTab" class="tg-tabs" @tab-change="onTabChange">
          <el-tab-pane name="work">
            <template #label>
              <span class="tg-tab-label"><el-icon><Clock /></el-icon>上下班记录</span>
            </template>

            <div class="activity-section">


              <div class="tg-rec-stat-grid">
                <div class="tg-rec-stat-card work">
                  <span class="tg-rec-stat-num">{{ workSummary.workerCount }}</span>
                  <span class="tg-rec-stat-label">{{ workTodayOnly ? "当日上班人数" : "当月上班人数" }}</span>
                </div>
                <div class="tg-rec-stat-card late">
                  <span class="tg-rec-stat-num">{{ workSummary.lateCount }}</span>
                  <span class="tg-rec-stat-label">迟到人数</span>
                </div>
                <div class="tg-rec-stat-card early">
                  <span class="tg-rec-stat-num">{{ workSummary.earlyCount }}</span>
                  <span class="tg-rec-stat-label">早退人数</span>
                </div>
                <div class="tg-rec-stat-card fine">
                  <span class="tg-rec-stat-num">{{ formatFineAmount(workSummary.fineTotal, fineCurrency) }}</span>
                  <span class="tg-rec-stat-label">罚款金额</span>
                </div>
              </div>

              <div class="tg-toolbar">
                <el-switch
                  v-model="workTodayOnly"
                  inline-prompt
                  active-text="按日"
                  inactive-text="按月"
                  @change="onWorkScopeChange"
                />
                <el-date-picker
                  v-if="workTodayOnly"
                  v-model="workDate"
                  type="date"
                  placeholder="选择日期"
                  value-format="YYYY-MM-DD"
                  style="width: 160px"
                  @change="loadWorkSessions"
                />
                <el-date-picker
                  v-else
                  v-model="workMonth"
                  type="month"
                  placeholder="选择月份"
                  format="YYYY年MM月"
                  value-format="YYYY-MM"
                  style="width: 160px"
                  @change="loadWorkSessions"
                />
                <el-input
                  v-model="workSearch"
                  placeholder="搜索员工或工号"
                  clearable
                  class="tg-search-input"
                  :prefix-icon="Search"
                  @input="debouncedLoadWork"
                />
                <el-button type="primary" @click="loadWorkSessions" :loading="loadingWork">
                  <el-icon><Refresh /></el-icon>
                  刷新
                </el-button>
              </div>

              <el-table
                :data="workSessions"
                v-loading="loadingWork"
                stripe
                class="tg-table"
                empty-text="暂无上下班记录"
              >
                <el-table-column prop="employeeName" label="员工" width="100" />
                <el-table-column prop="employeeCode" label="工号" width="100" />
                <el-table-column label="班次" width="80">
                  <template #default="{ row }">{{ formatShift(row.shiftType) }}</template>
                </el-table-column>
                <el-table-column label="来源" width="90">
                  <template #default="{ row }">{{ formatWorkSource(row.source) }}</template>
                </el-table-column>
                <el-table-column label="状态" width="90">
                  <template #default="{ row }">
                    <el-tag :type="row.status === 'open' ? 'warning' : 'success'" size="small">
                      {{ row.status === "open" ? "上班中" : "已下班" }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column prop="attendanceDate" label="考勤日" width="110" />
                <el-table-column label="上班时间" min-width="150">
                  <template #default="{ row }">{{ formatDateTime(row.checkinTime) }}</template>
                </el-table-column>
                <el-table-column label="下班时间" min-width="150">
                  <template #default="{ row }">
                    {{ row.checkoutTime ? formatDateTime(row.checkoutTime) : "-" }}
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </el-tab-pane>

          <el-tab-pane name="sessions">
            <template #label>
              <span class="tg-tab-label"><el-icon><Timer /></el-icon>活动记录 / 排行榜</span>
            </template>

            <div class="activity-section">


              <div v-if="account.hasPermission('data:all')" class="tg-rec-stat-grid">
                <div v-for="card in rankingCards" :key="card.key" class="tg-rec-stat-card rank-card" :class="card.key">
                  <span class="tg-rec-stat-label rank-card-title">{{ card.title }}</span>
                  <span v-if="card.headline" class="tg-rec-stat-num">{{ card.headline }}</span>
                  <ol v-if="card.items.length" class="rank-card-list">
                    <li v-for="(it, i) in card.items" :key="i">
                      <span class="rank-no" :class="`no-${i + 1}`">{{ i + 1 }}</span>
                      <span class="rank-name">{{ it.name }}</span>
                      <span class="rank-val">{{ it.value }}</span>
                    </li>
                  </ol>
                  <span v-else class="muted-cell rank-empty">暂无数据</span>
                </div>
              </div>

              <div class="tg-toolbar">
                <el-switch
                  v-model="sessionTodayOnly"
                  inline-prompt
                  active-text="按日"
                  inactive-text="按月"
                  @change="onSessionScopeChange"
                />
                <el-button-group v-if="sessionTodayOnly">
                  <el-button
                    :type="sessionDate === todayDateStr() ? 'primary' : 'default'"
                    @click="setSessionDate(todayDateStr())"
                  >
                    今天
                  </el-button>
                  <el-button
                    :type="sessionDate === yesterdayDateStr() ? 'primary' : 'default'"
                    @click="setSessionDate(yesterdayDateStr())"
                  >
                    昨天
                  </el-button>
                </el-button-group>
                <el-date-picker
                  v-if="sessionTodayOnly"
                  v-model="sessionDate"
                  type="date"
                  placeholder="选择日期"
                  value-format="YYYY-MM-DD"
                  style="width: 160px"
                  @change="loadActivitySessions"
                />
                <el-date-picker
                  v-else
                  v-model="sessionMonth"
                  type="month"
                  placeholder="选择月份"
                  format="YYYY年MM月"
                  value-format="YYYY-MM"
                  style="width: 160px"
                  @change="loadActivitySessions"
                />
                <el-input
                  v-model="sessionSearch"
                  placeholder="搜索员工或活动"
                  clearable
                  class="tg-search-input"
                  :prefix-icon="Search"
                  @input="debouncedLoadSessions"
                />
                <el-select
                  v-model="sessionShift"
                  clearable
                  placeholder="全部班次"
                  style="width: 120px"
                  @change="loadActivitySessions"
                >
                  <el-option label="白班" value="day" />
                  <el-option label="夜班" value="night" />
                </el-select>
                <el-select
                  v-model="sessionChatId"
                  clearable
                  filterable
                  placeholder="全部群组"
                  style="width: 220px"
                  @change="loadActivitySessions"
                >
                  <el-option
                    v-for="g in groups"
                    :key="g.chatId"
                    :label="g.chatTitle ? `${g.chatTitle} (${g.chatId})` : g.chatId"
                    :value="g.chatId"
                  />
                </el-select>
                <el-button type="primary" @click="loadActivitySessions" :loading="loadingSessions">
                  <el-icon><Refresh /></el-icon>
                  刷新
                </el-button>
              </div>

              <el-table
                :data="activitySessions"
                v-loading="loadingSessions"
                stripe
                class="tg-table"
                empty-text="暂无活动记录"
              >
                <el-table-column prop="employeeName" label="员工" width="100" />
                <el-table-column prop="activityName" label="活动" width="120" />
                <el-table-column label="状态" width="90">
                  <template #default="{ row }">
                    <el-tag :type="row.status === 'open' ? 'warning' : 'success'" size="small">
                      {{ row.status === "open" ? "进行中" : "已结束" }}
                    </el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="时长" width="90">
                  <template #default="{ row }">{{ formatSeconds(row.elapsedSeconds) }}</template>
                </el-table-column>
                <el-table-column label="超时" width="90">
                  <template #default="{ row }">{{ formatSeconds(row.overtimeSeconds) }}</template>
                </el-table-column>
                <el-table-column label="罚款" width="90">
                  <template #default="{ row }">
                    <span v-if="row.fineAmount > 0" class="fine-text">{{ formatFineAmount(row.fineAmount, fineCurrency) }}</span>
                    <span v-else class="muted-cell">-</span>
                  </template>
                </el-table-column>
                <el-table-column label="罚款排名" width="90" align="center">
                  <template #default="{ row }">{{ rankText(row, "fine") }}</template>
                </el-table-column>
                <el-table-column label="次数排名" width="90" align="center">
                  <template #default="{ row }">{{ rankText(row, "count") }}</template>
                </el-table-column>
                <el-table-column label="超时排名" width="90" align="center">
                  <template #default="{ row }">{{ rankText(row, "overtime") }}</template>
                </el-table-column>
                <el-table-column label="开始时间" min-width="160">
                  <template #default="{ row }">{{ formatDateTime(row.startedAt) }}</template>
                </el-table-column>
                <el-table-column label="开始来源" width="80" align="center">
                  <template #default="{ row }">{{ formatActivitySourceShort(row.startSource) }}</template>
                </el-table-column>
                <el-table-column label="结束时间" min-width="160">
                  <template #default="{ row }">
                    {{
                      row.endedAt
                        ? formatDateTime(row.endedAt)
                        : row.status === "open"
                          ? "进行中"
                          : "-"
                    }}
                  </template>
                </el-table-column>
                <el-table-column label="结束来源" width="80" align="center">
                  <template #default="{ row }">{{ formatActivitySourceShort(row.endSource) }}</template>
                </el-table-column>
              </el-table>
            </div>
          </el-tab-pane>
        </el-tabs>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import {useUserStore} from "../../store/user";
const account = useUserStore();
import { ref, computed, onMounted } from "vue";
import { ElMessage } from "element-plus";
import { Timer, Refresh, Search, Clock } from "@element-plus/icons-vue";
import workforceTelegramApi from "@api/workforce_telegram_api";
import { metaApi } from "@api/index";
import {
  formatSeconds,
  formatDateTime,
  todayDateStr,
  yesterdayDateStr,
  currentYearMonthStr,
  formatFineAmount,
  normalizeFineCurrency,
} from "./telegram_helpers";

const moduleEnabled = ref(false);
const fineCurrency = ref("CNY");
const activeTab = ref("work");

const workSessions = ref<any[]>([]);
const loadingWork = ref(false);
const workTodayOnly = ref(true);
const workDate = ref(todayDateStr());
const workMonth = ref(currentYearMonthStr());
const workSearch = ref("");

const groups = ref<any[]>([]);
const activitySessions = ref<any[]>([]);
const loadingSessions = ref(false);
const sessionTodayOnly = ref(true);
const sessionDate = ref(todayDateStr());
const sessionMonth = ref(currentYearMonthStr());
const sessionSearch = ref("");
const sessionShift = ref("");
const sessionChatId = ref("");


const workSummary = ref({ workerCount: 0, lateCount: 0, earlyCount: 0, fineTotal: 0 });

let workSearchTimer: ReturnType<typeof setTimeout> | null = null;
let sessionSearchTimer: ReturnType<typeof setTimeout> | null = null;

interface RankItem {
  name: string;
  value: string;
}
interface RankCard {
  key: string;
  title: string;
  headline?: string;
  items: RankItem[];
}

/** 按员工聚合已结束的活动会话，供排行卡片使用（随所选日期/月份变化） */
const employeeAgg = computed(() => {
  const map = new Map<string, { key: string; name: string; count: number; overtime: number; fine: number }>();
  for (const row of activitySessions.value) {
    if (row.status !== "closed") continue;
    const key = String(row.employeeId || row.employeeName);
    const cur = map.get(key) || { key, name: row.employeeName || "-", count: 0, overtime: 0, fine: 0 };
    cur.count += 1;
    cur.overtime += Number(row.overtimeSeconds) || 0;
    cur.fine += Number(row.fineAmount) || 0;
    map.set(key, cur);
  }
  return Array.from(map.values());
});

type RankField = "count" | "overtime" | "fine";

/** 每位员工在当前范围内的 罚款/次数/超时 名次（数值为 0 不上榜） */
const rankMaps = computed(() => {
  const agg = employeeAgg.value;
  const build = (field: RankField) => {
    const m = new Map<string, number>();
    agg
      .filter((r) => r[field] > 0)
      .sort((a, b) => b[field] - a[field])
      .forEach((r, i) => m.set(r.key, i + 1));
    return m;
  };
  return { fine: build("fine"), count: build("count"), overtime: build("overtime") };
});

function rankText(row: any, field: RankField) {
  const n = rankMaps.value[field].get(String(row.employeeId || row.employeeName));
  return n ? `第${n}名` : "-";
}

const rankingCards = computed<RankCard[]>(() => {
  const agg = employeeAgg.value;
  const top3 = (rows: typeof agg, field: RankField, fmt: (v: number) => string): RankItem[] =>
    rows
      .filter((r) => r[field] > 0)
      .sort((a, b) => b[field] - a[field])
      .slice(0, 3)
      .map((r) => ({ name: r.name, value: fmt(r[field]) }));

  const totalFine = agg.reduce((sum, r) => sum + r.fine, 0);
  // 罚款金额：所选范围内罚款合计 + 单笔罚款最高的前 3 条
  const topSingle: RankItem[] = activitySessions.value
    .filter((r) => r.status === "closed" && Number(r.fineAmount) > 0)
    .sort((a, b) => Number(b.fineAmount) - Number(a.fineAmount))
    .slice(0, 3)
    .map((r) => ({
      name: `${r.employeeName}·${r.activityName}`,
      value: formatFineAmount(r.fineAmount, fineCurrency.value),
    }));

  return [
    {
      key: "fine-rank",
      title: "罚款排行",
      items: top3(agg, "fine", (v) => formatFineAmount(v, fineCurrency.value)),
    },
    {
      key: "fine-amount",
      title: sessionTodayOnly.value ? "罚款金额（当日）" : "罚款金额（当月）",
      headline: formatFineAmount(totalFine, fineCurrency.value),
      items: topSingle,
    },
    { key: "count-rank", title: "次数排行", items: top3(agg, "count", (v) => `${v} 次`) },
    { key: "overtime-rank", title: "超时排行", items: top3(agg, "overtime", (v) => formatSeconds(v)) },
  ];
});

async function loadModuleFlag() {
  try {
    const { data } = await metaApi.modules();
    moduleEnabled.value = !!data?.telegramAttendance;
  } catch {
    moduleEnabled.value = false;
  }
}

function formatShift(shift?: string) {
  if (shift === "night") return "夜班";
  if (shift === "day") return "白班";
  return shift || "-";
}

function formatWorkSource(source?: string) {
  if (source === "telegram") return "Telegram";
  if (source === "agent") return "PC客户端";
  if (source === "web") return "web";
  return source || "-";
}

function formatActivitySourceShort(source?: string) {
  if (!source) return "-";
  if (source === "telegram") return "TG";
  if (source === "agent") return "PC";
  if (source === "web") return "web";
  return source;
}

function debouncedLoadWork() {
  if (workSearchTimer) clearTimeout(workSearchTimer);
  workSearchTimer = setTimeout(loadWorkSessions, 300);
}

function onWorkScopeChange() {
  if (workTodayOnly.value) {
    if (!workDate.value) workDate.value = todayDateStr();
  } else if (!workMonth.value) {
    workMonth.value = currentYearMonthStr();
  }
  loadWorkSessions();
}

async function loadWorkSessions() {
  loadingWork.value = true;
  try {
    const params: { date?: string; yearMonth?: string; search?: string } = {
      search: workSearch.value.trim() || undefined,
    };
    if (workTodayOnly.value) {
      params.date = workDate.value || todayDateStr();
    } else {
      params.yearMonth = workMonth.value || currentYearMonthStr();
    }
    const [res, sumRes] = await Promise.all([
      workforceTelegramApi.listWorkSessions(params),
      workforceTelegramApi.getWorkSummary({ date: params.date, yearMonth: params.yearMonth }).catch(() => null),
    ]);
    workSessions.value = res.data?.items || [];
    const sum = sumRes?.data;
    workSummary.value = {
      workerCount: Number(sum?.workerCount) || 0,
      lateCount: Number(sum?.lateCount) || 0,
      earlyCount: Number(sum?.earlyCount) || 0,
      fineTotal: Number(sum?.fineTotal) || 0,
    };
  } catch (error: any) {
    ElMessage.error(error.message || "加载上下班记录失败");
  } finally {
    loadingWork.value = false;
  }
}

function setSessionDate(date: string) {
  sessionDate.value = date;
  loadActivitySessions();
}

async function loadGroups() {
  try {
    const res = await workforceTelegramApi.listGroups();
    groups.value = res.data?.items || [];
  } catch {
    groups.value = [];
  }
}

function debouncedLoadSessions() {
  if (sessionSearchTimer) clearTimeout(sessionSearchTimer);
  sessionSearchTimer = setTimeout(loadActivitySessions, 300);
}

function onSessionScopeChange() {
  if (sessionTodayOnly.value) {
    if (!sessionDate.value) sessionDate.value = todayDateStr();
  } else if (!sessionMonth.value) {
    sessionMonth.value = currentYearMonthStr();
  }
  loadActivitySessions();
}

async function loadActivitySessions() {
  loadingSessions.value = true;
  try {
    const params: { date?: string; yearMonth?: string; search?: string; shift?: string; chatId?: string } = {
      search: sessionSearch.value.trim() || undefined,
      shift: sessionShift.value || undefined,
      chatId: sessionChatId.value || undefined,
    };
    if (sessionTodayOnly.value) {
      params.date = sessionDate.value || todayDateStr();
    } else {
      params.yearMonth = sessionMonth.value || currentYearMonthStr();
    }
    const res = await workforceTelegramApi.listActivitySessions(params);
    activitySessions.value = res.data?.items || [];
  } catch (error: any) {
    ElMessage.error(error.message || "加载活动记录失败");
  } finally {
    loadingSessions.value = false;
  }
}

function onTabChange(name: string | number) {
  if (name === "work" && workSessions.value.length === 0) loadWorkSessions();
  if (name === "sessions" && activitySessions.value.length === 0) loadActivitySessions();
}

async function loadFineCurrency() {
  try {
    const res = await workforceTelegramApi.getSettings();
    fineCurrency.value = normalizeFineCurrency(res.data?.fineCurrency);
  } catch {
    fineCurrency.value = "CNY";
  }
}

onMounted(async () => {
  await loadModuleFlag();
  await loadFineCurrency();
  await loadGroups();
  await loadWorkSessions();
  await loadActivitySessions();
});
</script>

<style scoped>
@import "./attendance.css";

.telegram-records-page {
  position: relative;
  min-height: 100%;
}

.tg-rec-blob-1 {
  background: linear-gradient(135deg, #229ed9, #0088cc);
  opacity: 0.32;
}

.tg-rec-blob-2 {
  background: linear-gradient(135deg, #667eea, #764ba2);
  opacity: 0.26;
}

.tg-rec-blob-3 {
  background: linear-gradient(135deg, #fbbf24, #f59e0b);
  opacity: 0.2;
}

.tg-rec-content {
  position: relative;
  z-index: 1;
  width: 100%;
  padding: 8px 0 32px;
}

.tg-alert {
  margin-bottom: 16px;
  border-radius: 12px;
}

.tg-rec-main-card {
  border-radius: 18px;
  border: 1px solid rgba(255, 255, 255, 0.65);
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(16px);
  box-shadow: 0 8px 32px rgba(15, 23, 42, 0.08);
}

.tg-rec-main-card :deep(.el-card__header) {
  padding: 20px 24px 12px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.tg-rec-main-card :deep(.el-card__body) {
  padding: 8px 20px 24px;
}

.tg-rec-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}

.tg-rec-stat-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 16px 12px;
  border-radius: 14px;
  background: linear-gradient(145deg, #f8fafc 0%, #f1f5f9 100%);
  border: 1px solid rgba(0, 0, 0, 0.06);
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.tg-rec-stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08);
}

.tg-rec-stat-card.sessions {
  border-top: 3px solid #0088cc;
}

.tg-rec-stat-card.fine {
  border-top: 3px solid #f59e0b;
}

.tg-rec-stat-card.handovers {
  border-top: 3px solid #6366f1;
}

.tg-rec-stat-num {
  font-size: 22px;
  font-weight: 700;
  color: #303133;
}

.tg-rec-stat-label {
  font-size: 12px;
  color: #909399;
}

.tg-tabs :deep(.el-tabs__header) {
  margin-bottom: 20px;
}

.tg-tabs :deep(.el-tabs__nav-wrap::after) {
  height: 1px;
  background: rgba(0, 0, 0, 0.06);
}

.tg-tabs :deep(.el-tabs__item) {
  height: 48px;
  font-size: 14px;
  font-weight: 500;
  color: #606266;
}

.tg-tabs :deep(.el-tabs__item.is-active) {
  color: #0088cc;
  font-weight: 600;
}

.tg-tabs :deep(.el-tabs__active-bar) {
  height: 3px;
  border-radius: 3px;
  background: linear-gradient(90deg, #229ed9, #667eea);
}

.tg-tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.activity-section {
  margin-bottom: 8px;
  padding: 20px;
  border-radius: 16px;
  background: linear-gradient(145deg, #f8fafc 0%, #f1f5f9 100%);
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.activity-section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.sub-block-title {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.section-desc {
  margin: 0;
  padding: 10px 14px;
  font-size: 13px;
  color: #606266;
  background: rgba(0, 136, 204, 0.06);
  border-radius: 10px;
  border-left: 3px solid #0088cc;
}

.inline-desc {
  padding: 0;
  background: none;
  border-left: none;
  color: #909399;
  font-size: 12px;
}

.tg-toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  align-items: center;
}

.tg-search-input {
  width: 280px;
  max-width: 100%;
}

.tg-table {
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
}

.tg-table :deep(.el-table__header th) {
  background: #f5f7fa !important;
  color: #606266;
  font-weight: 600;
}

.tg-table :deep(.el-table__row:hover > td) {
  background: rgba(0, 136, 204, 0.04) !important;
}

.fine-text {
  color: #e6a23c;
  font-weight: 600;
}

.muted-cell {
  color: #c0c4cc;
}

.tg-rec-stat-card.work {
  border-top: 3px solid #0088cc;
}

.tg-rec-stat-card.late {
  border-top: 3px solid #ef4444;
}

.tg-rec-stat-card.early {
  border-top: 3px solid #f97316;
}

.tg-rec-stat-card.rank-card {
  align-items: stretch;
  gap: 8px;
}

.tg-rec-stat-card.fine-rank {
  border-top: 3px solid #f59e0b;
}

.tg-rec-stat-card.fine-amount {
  border-top: 3px solid #ef4444;
}

.tg-rec-stat-card.count-rank {
  border-top: 3px solid #6366f1;
}

.tg-rec-stat-card.overtime-rank {
  border-top: 3px solid #10b981;
}

.rank-card-title {
  font-size: 13px;
  font-weight: 600;
  color: #606266;
  text-align: center;
}

.rank-card .tg-rec-stat-num {
  text-align: center;
}

.rank-card-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rank-card-list li {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  line-height: 1.4;
}

.rank-no {
  flex: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #cbd5e1;
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
}

.rank-no.no-1 {
  background: #fbbf24;
}

.rank-no.no-2 {
  background: #94a3b8;
}

.rank-no.no-3 {
  background: #d97706;
}

.rank-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #303133;
}

.rank-val {
  flex: none;
  color: #0088cc;
  font-weight: 600;
}

.rank-empty {
  text-align: center;
  font-size: 12px;
}

@media (max-width: 900px) {
  .tg-rec-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

}
</style>
