// 员工 / 子账号 增删后的跨页面同步。
// 「子账号管理」与「考勤明细」(以及后台打卡的员工下拉) 是同一批员工数据的不同入口；
// 任意一侧删除后，通过该事件通知已缓存（keep-alive）的其他页面立即重新加载，
// 避免继续显示已被删除的员工。
export const EMPLOYEES_CHANGED = "tg-employees-changed";

export type EmployeesChangedSource = "accounts" | "attendance";

export function notifyEmployeesChanged(source: EmployeesChangedSource): void {
  window.dispatchEvent(new CustomEvent(EMPLOYEES_CHANGED, { detail: { source } }));
}

export function onEmployeesChanged(
  handler: (source: EmployeesChangedSource | undefined) => void,
): () => void {
  const listener = (e: Event) => handler((e as CustomEvent).detail?.source);
  window.addEventListener(EMPLOYEES_CHANGED, listener);
  return () => window.removeEventListener(EMPLOYEES_CHANGED, listener);
}
