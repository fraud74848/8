export function formatSeconds(sec: number) {
  if (!sec) return "0秒";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  // 满 1 小时后换算成「X小时MM分」，例如 3660 秒 → 1小时01分钟
  if (h > 0) {
    const mm = String(m).padStart(2, "0");
    return s ? `${h}小时${mm}分${String(s).padStart(2, "0")}秒` : `${h}小时${mm}分钟`;
  }
  if (m > 0) return s ? `${m}分${s}秒` : `${m}分钟`;
  return `${s}秒`;
}

export function formatDateTime(v: string) {
  if (!v) return "-";
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN", { hour12: false });
}

export function formatToleranceGrace(late?: number, early?: number) {
  if (late == null && early == null) return "全局";
  const parts: string[] = [];
  if (late != null) parts.push(`迟到${late}`);
  if (early != null) parts.push(`早退${early}`);
  return parts.join("/") || "全局";
}

export function formatGroupGrace(before?: number, after?: number) {
  if (!before && !after) return "全局";
  const parts: string[] = [];
  if (before) parts.push(`前${before}`);
  if (after) parts.push(`后${after}`);
  return parts.join("/") || "全局";
}

export function formatPushChatType(type?: string) {
  if (type === "channel") return "频道";
  if (type === "supergroup") return "超级群";
  if (type === "group") return "群组";
  return type || "-";
}

export function groupOptionLabel(g: { chatTitle?: string; chatId?: string; chatType?: string }) {
  const title = (g.chatTitle || "").trim() || "未命名";
  const typeLabel =
    g.chatType === "channel" ? "频道" : g.chatType === "supergroup" ? "超级群" : g.chatType === "group" ? "群" : "";
  const suffix = typeLabel ? ` · ${typeLabel}` : "";
  return `${title}${suffix} (${g.chatId || ""})`;
}

export function todayDateStr() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function yesterdayDateStr() {
  const d = new Date();
  d.setDate(d.getDate() - 1);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function currentYearMonthStr() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

/** TG 罚款币种：与后端 NormalizeFineCurrency 一致，默认 CNY */
export type FineCurrencyCode = "CNY" | "THB" | "USD";

export function normalizeFineCurrency(code?: string | null): FineCurrencyCode {
  const c = String(code || "").trim().toUpperCase();
  if (c === "THB" || c === "฿" || c === "BAHT") return "THB";
  if (c === "USD" || c === "$" || c === "US$") return "USD";
  return "CNY";
}

export function fineCurrencySymbol(code?: string | null): string {
  switch (normalizeFineCurrency(code)) {
    case "THB":
      return "฿";
    case "USD":
      return "$";
    default:
      return "¥";
  }
}

export function formatFineAmount(amount: number | string | null | undefined, code?: string | null): string {
  const n = Number(amount);
  const v = Number.isFinite(n) ? n : 0;
  return `${fineCurrencySymbol(code)}${v.toFixed(2)}`;
}

function fmtYmd(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** 'YYYY-MM-DD' 加减天数 */
export function shiftDateStr(date: string, delta: number) {
  const [y, m, d] = date.split("-").map(Number);
  return fmtYmd(new Date(y, m - 1, d + delta));
}

/** 日期区间选择器的快捷项：今天、昨天、近3天、近7天、近一个月 */
export const dateRangeShortcuts = [
  { text: "今天", value: () => [new Date(), new Date()] },
  {
    text: "昨天",
    value: () => {
      const d = new Date();
      d.setDate(d.getDate() - 1);
      return [d, d];
    },
  },
  {
    text: "近3天",
    value: () => {
      const s = new Date();
      s.setDate(s.getDate() - 2);
      return [s, new Date()];
    },
  },
  {
    text: "近7天",
    value: () => {
      const s = new Date();
      s.setDate(s.getDate() - 6);
      return [s, new Date()];
    },
  },
  {
    text: "近一个月",
    value: () => {
      const s = new Date();
      s.setMonth(s.getMonth() - 1);
      s.setDate(s.getDate() + 1);
      return [s, new Date()];
    },
  },
];

/** 区间 -> 后端 date 参数（单日 or "起~止"） */
export function rangeToDateParam(range: string[] | null | undefined) {
  const today = todayDateStr();
  if (!range || range.length < 2 || !range[0]) return `${today}~${today}`;
  return range[0] === range[1] ? range[0] : `${range[0]}~${range[1]}`;
}
