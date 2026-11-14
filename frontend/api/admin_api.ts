// frontend/api/admin_api.ts
import request from "./request";

const adminApi = {
  // ========== 考勤管理 ==========
  getEmployees: (params?: {
    skip?: number;
    limit?: number;
    search?: string;
    device_bound?: "true" | "false";
    employment_scope?: "all" | "month_visible";
    year_month?: string;
    performance_eligibility?: "true" | "false";
    performance_hire_cutoff_day?: number;
  }) => request.get<any>("/admin/employees", { params }),

  createEmployeeAccount: (data: any) => request.post("/admin/employee-accounts", data),

  updateEmployee: (id: string, data: any) =>
    request.put(`/admin/employees/${id}`, data),

  deleteEmployee: (id: string) => request.delete(`/admin/employees/${id}`),

  getRecordsByEmployees: (yearMonth: string, employeeIds: string[]) =>
    request.get("/admin/attendance/records", {
      params: { year_month: yearMonth, employee_ids: employeeIds },
    }),

  saveRecords: (data: {
    year_month: string;
    data: any;
    clears?: Record<string, string[]>;
    restoreEmployees?: string[];
  }) => request.post("/admin/attendance/records", data),

  importAttendanceRecords: (formData: FormData) =>
    request.post("/admin/attendance/records/import", formData, {
      headers: { "Content-Type": "multipart/form-data" },
    }),

  getTodayAttendanceRate: () => request.get("/admin/attendance/today-rate"),

  // ========== 绩效管理 ==========
  getPerformance: (params: {
    month: string;
    employee_id?: string;
    employment_scope?: "all" | "month_visible";
    year_month?: string;
    performance_hire_cutoff_day?: number;
  }) =>
    request.get("/admin/performance", { params }),

  getPerformanceRuleSettings: () =>
    request.get<{ hireCutoffDay: number }>("/admin/performance/rule-settings"),

  savePerformanceRuleSettings: (data: { hireCutoffDay: number }) =>
    request.put<{ hireCutoffDay: number }>("/admin/performance/rule-settings", data),

  batchSavePerformance: (data: { month: string; items: any[] }) =>
    request.post("/admin/performance/batch", data),

  getPerformancePushSettings: () =>
    request.get("/admin/performance/push-settings"),

  savePerformancePushEnabled: (enabled: boolean) =>
    request.put("/admin/performance/push-settings/enabled", { enabled }),

  savePerformancePushToken: (botToken: string) =>
    request.put("/admin/performance/push-settings/token", { botToken }),

  discoverPerformancePushChats: () =>
    request.post("/admin/performance/push-settings/discover"),

  bindPerformancePushChat: (data: {
    chatId: string;
    chatTitle?: string;
    chatType?: string;
  }) => request.put("/admin/performance/push-settings/bind", data),

  unbindPerformancePushChat: () =>
    request.delete("/admin/performance/push-settings/bind"),

  testPerformancePush: () =>
    request.post("/admin/performance/push-settings/test"),

  pushPerformanceNotify: (data: {
    date: string;
    employeeId?: string;
    employeeName: string;
    position?: string;
    score: number;
    reason: string;
  }) => request.post("/admin/performance/push-notify", data),

  getPenaltyPushSettings: () =>
    request.get("/admin/penalty/push-settings"),

  savePenaltyPushEnabled: (enabled: boolean) =>
    request.put("/admin/penalty/push-settings/enabled", { enabled }),

  savePenaltyPushToken: (botToken: string) =>
    request.put("/admin/penalty/push-settings/token", { botToken }),

  discoverPenaltyPushChats: () =>
    request.post("/admin/penalty/push-settings/discover"),

  bindPenaltyPushChat: (data: {
    chatId: string;
    chatTitle?: string;
    chatType?: string;
  }) => request.put("/admin/penalty/push-settings/bind", data),

  unbindPenaltyPushChat: () =>
    request.delete("/admin/penalty/push-settings/bind"),

  testPenaltyPush: () =>
    request.post("/admin/penalty/push-settings/test"),

  pushPenaltyNotify: (data: {
    date: string;
    employeeId?: string;
    employeeName: string;
    position?: string;
    amount: number;
    category: string;
    reason: string;
  }) => request.post("/admin/penalty/push-notify", data),

  // ========== 罚款管理 ==========
  getPenaltyRecords: (params: {
    month?: string;
    employee_id?: string;
    page?: number;
    page_size?: number;
  }) => request.get("/admin/penalty/records", { params }),

  createPenaltyRecord: (data: any) =>
    request.post("/admin/penalty/record", data),

  deletePenaltyRecord: (id: string) =>
    request.delete(`/admin/penalty/records/${id}`),

  updatePenaltyRecord: (
    id: string,
    data: {
      amount: number;
      category: string;
      reason: string;
      penalty_date: string;
    },
  ) => request.put(`/admin/penalty/records/${id}`, data)
};

export default adminApi;
