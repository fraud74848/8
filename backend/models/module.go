// Consolidated source; original file boundaries are marked below.
package models

import (
	gin "github.com/gin-gonic/gin"
	uuid "github.com/google/uuid"
	log "log"
	time "time"
)

// ---- Source: backend/models/default_roles.go ----
const (
	PermInviteView   = "invite:view"
	PermInviteManage = "invite:manage"

	PermCallbackView     = "callback:view"
	PermCallbackManage   = "callback:manage"
	PermCallbackSettings = "callback:settings"

	PermCsToolView     = "cstool:view"
	PermCsToolSettings = "cstool:settings"

	PermDomainMonitorView     = "domain_monitor:view"
	PermDomainMonitorManage   = "domain_monitor:manage"
	PermDomainMonitorSettings = "domain_monitor:settings"

	PermProfileView     = "profile:view"
	PermProfileEdit     = "profile:edit"
	PermProfilePassword = "profile:password"
	PermProfileTotp     = "profile:totp"
	PermProfileInvite   = "profile:invite"

	PermAgentNotificationView = "agent_notification:view" // 查看客户端通知配置
	PermAgentNotificationEdit = "agent_notification:edit" // 编辑客户端通知配置
)

// AllAdminPermissions 管理员拥有全部后台权限
func AllAdminPermissions() []string {
	return []string{
		PermUserView, PermUserCreate, PermUserUpdate, PermUserDelete, PermUserManage,
		PermUserResetSalaryPassword,
		PermRoleView, PermRoleCreate, PermRoleUpdate, PermRoleDelete,
		PermDeviceView, PermDeviceRemote, PermDeviceManage, PermDeviceBind, PermDeviceUnbind,
		PermRecordingView, PermRecordingPlay, PermRecordingExport, PermRecordingDelete, PermRecordingUpload,
		PermScreenshotView, PermScreenshotDelete,
		PermActivityView, PermActivityExport, PermActivityDelete,
		PermPolicyView, PermPolicyCreate, PermPolicyUpdate, PermPolicyDelete,
		PermAuditView,
		PermAttendanceView, PermAttendanceEdit,
		PermAttendanceReviewView, PermAttendanceReviewEdit,
		PermTelegramView, PermTelegramEdit,
		PermAgentNotificationView, PermAgentNotificationEdit,
		PermInviteView, PermInviteManage,
		PermSiteView, PermSiteManage,
		PermStatsView, PermStatsEdit,
		PermSalaryView, PermSalaryViewOthers, PermSalaryEdit, PermSalaryUpload, PermSalaryDelete, PermSalaryPaymentAddressView,
		PermOperationLogView, PermOperationLogExport, PermOperationLogDelete,
		PermDiagnosticView, PermDiagnosticManage, PermDiagnosticExport,
		PermCallbackView, PermCallbackManage, PermCallbackSettings,
		PermCsToolView, PermCsToolSettings,
		PermDomainMonitorView, PermDomainMonitorManage, PermDomainMonitorSettings,
		PermProfileView, PermProfileEdit, PermProfilePassword, PermProfileTotp, PermProfileInvite,
	}
}

// DefaultViewerPermissions 查看员 — 子账号默认角色：
// 查看打卡记录、考勤月历、罚款详情，并可在后台为本人打卡（子账号只能为本人打卡）。
// 修改自己的登录密码不依赖角色权限。
var DefaultViewerPermissions = []string{
	"attendance:view", // 考勤月历
	"records:view",    // 打卡记录
	"punch:view",      // 后台打卡（查看）
	"punch:edit",      // 后台打卡（本人上下班、活动打卡）
	"penalty:view",    // 罚款详情
}

// DefaultRoleSeed 启动时自动创建/同步的默认角色
type DefaultRoleSeed struct {
	Name        string
	Description string
	Permissions []string
	ExactSync   bool // true=每次启动对齐权限列表；false=仅合并缺失权限
}

var DefaultRoleSeeds = []DefaultRoleSeed{
	{
		Name:        "管理员",
		Description: "拥有所有权限",
		Permissions: AllAdminPermissions(),
		ExactSync:   false,
	},
	{
		Name:        "查看员",
		Description: "查看打卡记录、考勤月历、罚款详情；可在后台为本人打卡",
		Permissions: DefaultViewerPermissions,
		ExactSync:   true,
	},
}

// ---- Source: backend/models/diagnostic.go ----
const (
	PermDiagnosticView   = "diagnostic:view"
	PermDiagnosticManage = "diagnostic:manage"
	PermDiagnosticExport = "diagnostic:export"
)

// ---- Source: backend/models/models.go ----
// APIResponse API 统一响应结构
type APIResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp string      `json:"timestamp"`
	RequestID string      `json:"requestId"`
}

// User 用户模型
type User struct {
	ID           uuid.UUID  `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Email        string     `json:"email"`
	RealName     string     `json:"realName"`
	DepartmentID *uuid.UUID `json:"departmentId,omitempty"`
	Status       int        `json:"status"` // 1:启用, 0:禁用
	TotpEnabled  bool       `json:"totpEnabled"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastLogin    *time.Time `json:"lastLogin,omitempty"`
	Roles        []Role     `json:"roles,omitempty"`
}

// Role 角色模型
type Role struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Device 设备模型
type Device struct {
	ID                  uuid.UUID  `json:"id"`
	EmployeeID          string     `json:"employeeId"`   // 员工ID（系统内部）
	EmployeeName        string     `json:"employeeName"` // 员工姓名
	EmployeeNumber      string     `json:"employeeNumber"`
	Position            string     `json:"position"`
	DeviceFingerprint   string     `json:"deviceFingerprint"`
	Hostname            string     `json:"hostname"`
	OSVersion           string     `json:"osVersion"`
	AgentVersion        string     `json:"agentVersion"`
	Status              string     `json:"status"` // online, offline, pending
	OnlineStatus        string     `json:"onlineStatus" gorm:"-"`
	LastSeenAt          *time.Time `json:"lastSeenAt"`
	BoundAt             *time.Time `json:"boundAt"` // ✅ 添加：绑定时间
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           *time.Time `json:"updatedAt,omitempty"` // ✅ 添加：更新时间
	DeviceSecret        string     `json:"-"`
	ActiveInviteCode    string     `json:"activeInviteCode,omitempty"`
	EmployeeUUID        string     `json:"employeeUuid,omitempty"`
	IsActiveSession     bool       `json:"isActiveSession"`
	IsSharedWorkstation bool       `json:"isSharedWorkstation"`
	SharedWithNames     string     `json:"sharedWithNames,omitempty"`
	// ForceOfflined: admin forced the device offline; blocks token refresh/register until cleared.
	ForceOfflined bool `json:"forceOfflined"`
}

const (
	// 用户管理权限
	PermUserView                = "user:view"
	PermUserCreate              = "user:create"
	PermUserUpdate              = "user:update"
	PermUserDelete              = "user:delete"
	PermUserManage              = "user:manage"
	PermUserResetSalaryPassword = "user:reset_salary_view_password"

	// 角色管理权限
	PermRoleView   = "role:view"
	PermRoleCreate = "role:create"
	PermRoleUpdate = "role:update"
	PermRoleDelete = "role:delete"

	// 设备管理权限
	PermDeviceView   = "device:view"
	PermDeviceRemote = "device:remote"
	PermDeviceManage = "device:manage"
	PermDeviceBind   = "device:bind"
	PermDeviceUnbind = "device:unbind"

	// 录屏管理权限
	PermRecordingView   = "recording:view"
	PermRecordingPlay   = "recording:play"
	PermRecordingExport = "recording:export"
	PermRecordingDelete = "recording:delete"
	PermRecordingUpload = "recording:upload"

	// 截图管理权限
	PermScreenshotView   = "screenshot:view"
	PermScreenshotDelete = "screenshot:delete"

	// 活动日志权限
	PermActivityView   = "activity:view"
	PermActivityExport = "activity:export"
	PermActivityDelete = "activity:delete"

	// 策略管理权限
	PermPolicyView   = "policy:view"
	PermPolicyCreate = "policy:create"
	PermPolicyUpdate = "policy:update"
	PermPolicyDelete = "policy:delete"

	// 审计权限
	PermAuditView = "audit:view"

	// ========== 出款管理权限（新增） ==========
	PermAttendanceView       = "attendance:view"        // 查看考勤绩效
	PermAttendanceEdit       = "attendance:edit"        // 编辑考勤绩效
	PermAttendanceReviewView = "attendance_review:view" // 查看考勤待确认
	PermAttendanceReviewEdit = "attendance_review:edit" // 处理考勤待确认 / 改阈值
	PermTelegramView         = "telegram:view"          // 查看 Telegram 考勤配置与记录
	PermTelegramEdit         = "telegram:edit"          // 编辑 Telegram 考勤配置
	PermSiteView             = "site:view"              // 查看出款站点
	PermSiteManage           = "site:manage"            // 管理出款站点
	PermStatsView            = "stats:view"             // 查看出款统计
	PermStatsEdit            = "stats:edit"             // 编辑出款统计（上传数据）

	// ========== 工资管理权限（新增） ==========
	PermSalaryUpload             = "salary:upload"               // 上传工资表
	PermSalaryView               = "salary:view"                 // 查看工资（默认仅本人）
	PermSalaryViewOthers         = "salary:view_others"          // 工资查看：可下拉选择其他员工
	PermSalaryEdit               = "salary:edit"                 // 编辑工资
	PermSalaryDelete             = "salary:delete"               // 删除工资
	PermSalaryPaymentAddressView = "salary:payment_address:view" // 查看员工收款地址（管理员）
)

// OperationLog 操作日志模型（只记录修改操作）
type OperationLog struct {
	ID              string    `json:"id"`
	OperatorID      string    `json:"operatorId"`
	OperatorName    string    `json:"operatorName"`
	OperatorRole    string    `json:"operatorRole"`
	OperationType   string    `json:"operationType"`   // CREATE, UPDATE, DELETE, UPLOAD
	OperationModule string    `json:"operationModule"` // USER, ROLE, DEVICE, SALARY, ATTENDANCE, PERFORMANCE, PENALTY, SITE_STATS
	OperationDesc   string    `json:"operationDesc"`   // 详细描述，如：修改员工阿富的工资: 底薪18000→18500
	TargetID        string    `json:"targetId"`
	TargetName      string    `json:"targetName"`
	IPAddress       string    `json:"ipAddress"`
	UserAgent       string    `json:"userAgent"`
	RequestMethod   string    `json:"requestMethod"`
	RequestPath     string    `json:"requestPath"`
	OldValue        string    `json:"oldValue"` // 修改前的值（JSON格式）
	NewValue        string    `json:"newValue"`
	ExecutionTimeMs int       `json:"executionTimeMs"` // 修改后的值（JSON格式）
	CreatedAt       time.Time `json:"createdAt"`
}

// 添加权限常量
const (
	// 操作日志权限
	PermOperationLogView   = "operation_log:view"
	PermOperationLogExport = "operation_log:export"
	PermOperationLogDelete = "operation_log:delete"
)

// UTCNow 获取当前 UTC 时间
func UTCNow() time.Time {
	return time.Now().UTC()
}

// FormatUTC 格式化 UTC 时间为 RFC3339
func FormatUTC(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// Send 发送统一格式的响应
func Send(c *gin.Context, statusCode int, code int, message string, data interface{}) {
	requestID := c.GetString("requestID")
	c.JSON(statusCode, APIResponse{
		Code:      code,
		Message:   message,
		Data:      data,
		Timestamp: FormatUTC(UTCNow()),
		RequestID: requestID,
	})
}

// SendError 发送错误响应
func SendError(c *gin.Context, statusCode int, code int, message string) {
	log.Printf("[ERROR] SendError: status=%d, code=%d, message=%s, path=%s",
		statusCode, code, message, c.Request.URL.Path)

	Send(c, statusCode, code, message, nil)
}

// SendInternalError 记录内部错误并返回通用 500 消息（避免泄露实现细节）
func SendInternalError(c *gin.Context, err error) {
	if err != nil {
		log.Printf("[INTERNAL] path=%s err=%v", c.Request.URL.Path, err)
	}
	SendError(c, 500, 5000, "Internal server error")
}

// RequestID 从 Gin Context 中获取请求 ID
func RequestID(c *gin.Context) string {
	if value, ok := c.Get("requestID"); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}
