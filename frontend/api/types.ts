// frontend/api/types.ts - 完整的类型定义（与后端 Go 模型对齐）

// ========== 设备相关（camelCase） ==========
export interface Device {
  id: string;
  /** 设备ID（可能与 id 相同或不同，用于兼容旧版本） */
  deviceId?: string;
  /** 设备指纹，用于唯一标识设备 */
  deviceFingerprint: string;
  /** 员工ID（系统内部） */
  employeeId: string;
  /** 员工姓名 */
  employeeName: string;
  /** 员工工号 */
  employeeNumber: string;
  /** 职位 */
  position?: string;
  /** 设备名称/主机名 */
  hostname: string;
  /** 操作系统版本 */
  osVersion: string;
  /** Agent 版本号 */
  agentVersion: string;
  /** 审核状态: pending(待审核), approved(已审核), rejected(已拒绝) */
  status: "pending" | "approved" | "rejected";
  /** 在线状态: online(在线), offline(离线) - 前端动态计算，后端不存储 */
  onlineStatus: "online" | "offline";
  /** 最后在线时间 (UTC) */
  lastSeenAt: string | null;
  /** 绑定时间 (UTC) */
  boundAt: string | null;
  /** 创建时间 (UTC) */
  createdAt: string;
  /** 更新时间 (UTC) */
  updatedAt?: string;
  /** 截图统计（前端扩展字段，从 screenshotApi.getEmployeeStats 获取） */
  screenshotStats?: {
    /** 今日截图数量 */
    todayCount: number;
    /** 总截图数量 */
    totalCount: number;
  };
  /** 员工 UUID（共享工位展开行） */
  employeeUuid?: string;
  /** 是否为当前登录会话 */
  isActiveSession?: boolean;
  /** 是否为共享工位设备 */
  isSharedWorkstation?: boolean;
  /** 同设备其他共享成员姓名（顿号分隔） */
  sharedWithNames?: string;
  /** 是否处于强制下线锁（禁止重新注册，直至后台放行） */
  forceOfflined?: boolean;
}

export interface DeviceListResponse {
  items: Device[];
  total: number;
  page: number;
  pageSize: number;
}

// ========== 用户相关 ==========
export interface User {
  id: string;
  username: string;
  realName?: string;
  email?: string;
  status: number;
  totpEnabled?: boolean;
  roleIds?: string[];
  roles?: Role[];
  createdAt: string;
  updatedAt: string;
  lastLogin?: string | null;
}

export interface Role {
  id: string;
  name: string;
  description?: string;
  permissions: string[];
  createdAt: string;
  updatedAt: string;
}

// 在文件末尾添加

