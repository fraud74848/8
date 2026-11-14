// Consolidated source; original file boundaries are marked below.
package handlers

import (
	context "context"
	sql "database/sql"
	csvPackage2 "encoding/csv"
	fmt "fmt"
	gin "github.com/gin-gonic/gin"
	uuid "github.com/google/uuid"
	redisPackage2 "github.com/redis/go-redis/v9"
	excelize "github.com/xuri/excelize/v2"
	rate "golang.org/x/time/rate"
	io "io"
	logPackage2 "log"
	multipart "mime/multipart"
	os "os"
	filepath "path/filepath"
	strconv "strconv"
	strings "strings"
	sync "sync"
	middleware "tg-attendance/backend/middleware"
	models "tg-attendance/backend/models"
	services "tg-attendance/backend/services"
	logx "tg-attendance/backend/services/logx"
	time "time"
)

// ---- Source: backend/handlers/attendance_import.go ----
const maxAttendanceImportRows = 500

var attendanceStatusImportMap = map[string]string{
	"出勤":        "work",
	"半休":        "rest_half",
	"全休":        "rest_full",
	"半假":        "leave",
	"旷工":        "absent",
	"全假":        "off_post",
	"离职":        "resigned",
	"work":      "work",
	"rest_half": "rest_half",
	"rest_full": "rest_full",
	"leave":     "leave",
	"absent":    "absent",
	"off_post":  "off_post",
	"resigned":  "resigned",
	"pending":   "pending",
}

type attendanceImportColumnMap struct {
	HireDate     int
	Name         int
	Position     int
	WorkLocation int
	EmployeeID   int
	Username     int
	Password     int
	DayColumns   map[int]int
}
type attendanceImportRow struct {
	HireDate     string
	Name         string
	Position     string
	WorkLocation string
	EmployeeID   string
	Username     string
	Password     string
	DayStatus    map[int]string
}

func normalizeImportHeader(cell string) string {
	cell = strings.TrimSpace(cell)
	cell = strings.Trim(cell, "\ufeff")
	cell = strings.ReplaceAll(cell, " ", "")
	return strings.ToLower(cell)
}
func findImportColumn(header []string, keys ...string) int {
	for i, raw := range header {
		norm := normalizeImportHeader(raw)
		for _, key := range keys {
			if norm == normalizeImportHeader(key) {
				return i
			}
		}
	}
	return -1
}
func parseAttendanceImportHeader(header []string) (attendanceImportColumnMap, error) {
	if len(header) == 0 {
		return attendanceImportColumnMap{}, fmt.Errorf("表头为空")
	}

	m := attendanceImportColumnMap{
		HireDate:     findImportColumn(header, "入职日期", "hire_date"),
		Name:         findImportColumn(header, "姓名", "name", "员工姓名"),
		Position:     findImportColumn(header, "岗位", "position"),
		WorkLocation: findImportColumn(header, "办公地点", "公地点", "work_location", "工作地点"),
		EmployeeID:   findImportColumn(header, "工号", "employee_id", "员工工号"),
		Username:     findImportColumn(header, "登录账号", "username", "子账号"),
		Password:     findImportColumn(header, "初始密码", "password", "密码"),
		DayColumns:   make(map[int]int),
	}

	if m.Name < 0 {
		return m, fmt.Errorf("缺少「姓名」列")
	}

	for i, raw := range header {
		dayText := strings.TrimSpace(raw)
		if dayText == "" {
			continue
		}
		if day, err := strconv.Atoi(dayText); err == nil && day >= 1 && day <= 31 {
			m.DayColumns[day] = i
		}
	}

	return m, nil
}
func parseAttendanceStatus(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if status, ok := attendanceStatusImportMap[raw]; ok {
		return status
	}
	if status, ok := attendanceStatusImportMap[strings.ToLower(raw)]; ok {
		return status
	}
	return ""
}
func parseAttendanceImportRows(rows [][]string, cols attendanceImportColumnMap) ([]attendanceImportRow, error) {
	if len(rows) < 2 {
		return nil, fmt.Errorf("文件中没有数据行")
	}

	getCell := func(row []string, idx int) string {
		if idx < 0 || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	items := make([]attendanceImportRow, 0)
	for rowIdx, row := range rows[1:] {
		name := getCell(row, cols.Name)
		if name == "" {
			continue
		}

		item := attendanceImportRow{
			HireDate:     getCell(row, cols.HireDate),
			Name:         name,
			Position:     getCell(row, cols.Position),
			WorkLocation: getCell(row, cols.WorkLocation),
			EmployeeID:   getCell(row, cols.EmployeeID),
			Username:     getCell(row, cols.Username),
			Password:     getCell(row, cols.Password),
			DayStatus:    make(map[int]string),
		}

		if item.WorkLocation == "" {
			item.WorkLocation = "现场"
		}

		for day, colIdx := range cols.DayColumns {
			status := parseAttendanceStatus(getCell(row, colIdx))
			if status != "" {
				item.DayStatus[day] = status
			}
		}

		items = append(items, item)
		if len(items) > maxAttendanceImportRows {
			return nil, fmt.Errorf("单次最多导入 %d 名员工", maxAttendanceImportRows)
		}
		_ = rowIdx
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("未读取到有效员工数据")
	}
	return items, nil
}
func (h *WorkforceHandler) lookupEmployeeID(ctx *gin.Context, employeeID, name string) (uuid.UUID, bool, error) {
	if strings.TrimSpace(employeeID) != "" {
		var id uuid.UUID
		err := h.db.QueryRowContext(ctx.Request.Context(), `
			SELECT id FROM employees WHERE employee_id = $1 LIMIT 1
		`, strings.TrimSpace(employeeID)).Scan(&id)
		if err == nil {
			return id, true, nil
		}
	}

	var id uuid.UUID
	err := h.db.QueryRowContext(ctx.Request.Context(), `
		SELECT id FROM employees WHERE name = $1 ORDER BY created_at ASC LIMIT 1
	`, strings.TrimSpace(name)).Scan(&id)
	if err != nil {
		return uuid.Nil, false, nil
	}
	return id, true, nil
}
func (h *WorkforceHandler) upsertAttendanceForImport(ctx *gin.Context, employeeID uuid.UUID, yearMonth, position string, dayStatus map[int]string) (int, error) {
	if len(dayStatus) == 0 {
		return 0, nil
	}

	parts := strings.Split(yearMonth, "-")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid year_month")
	}

	updated := 0
	for day, status := range dayStatus {
		if status == "" {
			continue
		}
		dateKey := fmt.Sprintf("%s-%02d", yearMonth, day)
		_, err := h.db.ExecContext(ctx.Request.Context(), `
			INSERT INTO attendance_records (employee_id, year_month, date, status, position_snapshot, manually_edited, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, TRUE, NOW(), NOW())
			ON CONFLICT (employee_id, year_month, date) DO UPDATE SET
				status = EXCLUDED.status,
				position_snapshot = EXCLUDED.position_snapshot,
				manually_edited = TRUE,
				updated_at = NOW()
		`, employeeID, yearMonth, dateKey, status, position)
		if err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}
func (h *WorkforceHandler) ensurePerformanceForMonth(ctx *gin.Context, employeeID uuid.UUID, name, position, yearMonth string) error {
	_, err := h.db.ExecContext(ctx.Request.Context(), `
		INSERT INTO performance_records (employee_id, employee_name, position, score_records, total_score, grade, month, created_at, updated_at)
		VALUES ($1, $2, $3, '[]'::jsonb, 10, '合格', $4, NOW(), NOW())
		ON CONFLICT (employee_id, month) DO NOTHING
	`, employeeID, name, position, yearMonth)
	return err
}

// ImportAttendanceRecords 导入考勤表，新员工可自动创建子账号与邀请码
func (h *WorkforceHandler) ImportAttendanceRecords(c *gin.Context) {
	yearMonth := strings.TrimSpace(c.PostForm("year_month"))
	defaultPassword := strings.TrimSpace(c.PostForm("default_password"))

	if yearMonth == "" {
		models.SendError(c, 400, 1001, "year_month 不能为空")
		return
	}
	if len(defaultPassword) > 0 && len(defaultPassword) < 6 {
		models.SendError(c, 400, 1001, "默认密码至少 6 位")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		models.SendError(c, 400, 1001, "请上传文件")
		return
	}

	rows, err := readSiteStatsRowsFromFileHeader(file)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}

	cols, err := parseAttendanceImportHeader(rows[0])
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}

	items, err := parseAttendanceImportRows(rows, cols)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}

	createdBy := parseContextUserUUID(c)
	createdEmployees := 0
	updatedEmployees := 0
	attendanceCells := 0
	newAccounts := make([]gin.H, 0)

	for _, item := range items {
		employeeUUID, found, err := h.lookupEmployeeID(c, item.EmployeeID, item.Name)
		if err != nil {
			models.SendError(c, 500, 5000, err.Error())
			return
		}

		if !found {
			username := strings.TrimSpace(item.Username)
			employeeCode := strings.TrimSpace(item.EmployeeID)
			if username == "" {
				username = employeeCode
			}
			if employeeCode == "" {
				employeeCode = username
			}
			if username == "" {
				models.SendError(c, 400, 1001, fmt.Sprintf("员工「%s」不存在，请填写登录账号或工号以自动创建子账号", item.Name))
				return
			}
			if username != employeeCode {
				employeeCode = username
			}

			password := strings.TrimSpace(item.Password)
			if password == "" {
				password = defaultPassword
			}
			if len(password) < 6 {
				models.SendError(c, 400, 1001, fmt.Sprintf("员工「%s」需填写初始密码，或在导入时设置默认密码（至少6位）", item.Name))
				return
			}

			result, err := h.workforce.CreateEmployeeAccount(c.Request.Context(), &services.CreateEmployeeAccountInput{
				Username:     username,
				Password:     password,
				EmployeeID:   employeeCode,
				Name:         item.Name,
				Position:     item.Position,
				HireDate:     item.HireDate,
				WorkLocation: item.WorkLocation,
				CreatedBy:    createdBy,
			})
			if err != nil {
				if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
					models.SendError(c, 400, 1001, fmt.Sprintf("员工「%s」创建失败：登录账号或工号已存在", item.Name))
					return
				}
				models.SendError(c, 500, 5000, fmt.Sprintf("创建员工「%s」失败: %v", item.Name, err))
				return
			}

			employeeUUID = result.EmployeeID
			createdEmployees++
			newAccounts = append(newAccounts, gin.H{
				"name":       item.Name,
				"username":   username,
				"employeeId": result.EmployeeID.String(),
				"inviteCode": result.InviteCode,
			})
		} else {
			updatedEmployees++
		}

		if err := h.ensurePerformanceForMonth(c, employeeUUID, item.Name, item.Position, yearMonth); err != nil {
			models.SendError(c, 500, 5000, err.Error())
			return
		}

		count, err := h.upsertAttendanceForImport(c, employeeUUID, yearMonth, item.Position, item.DayStatus)
		if err != nil {
			models.SendError(c, 500, 5000, fmt.Sprintf("导入员工「%s」考勤失败: %v", item.Name, err))
			return
		}
		attendanceCells += count
	}

	Log(c, "CREATE", "ATTENDANCE",
		fmt.Sprintf("导入 %s 月考勤：新增员工 %d，更新 %d，写入 %d 条考勤", yearMonth, createdEmployees, updatedEmployees, attendanceCells),
		"", yearMonth)

	models.Send(c, 200, 0, "Import completed", gin.H{
		"createdEmployees": createdEmployees,
		"updatedEmployees": updatedEmployees,
		"attendanceCells":  attendanceCells,
		"newAccounts":      newAccounts,
	})
}

func normalizeRoleName(roleName string) string {
	switch strings.TrimSpace(strings.ToLower(roleName)) {
	case "管理员", "admin", "administrator", "超级管理员", "superadmin":
		return "admin"
	case "审计员", "auditor":
		return "auditor"
	case "操作员", "operator", "ops":
		return "operator"
	case "查看员", "viewer":
		return "viewer"
	case "员工", "employee":
		return "employee"
	default:
		return "auditor"
	}
}

var _ = context.Background

type mediaRequestError struct {
	msg string
}

func (e *mediaRequestError) Error() string { return e.msg }

// ---- Source: backend/handlers/operation_log.go ----
type OperationLogHandler struct {
	db *sql.DB
}

func NewOperationLogHandler(db *sql.DB) *OperationLogHandler {
	return &OperationLogHandler{db: db}
}

// saveOperationLog 保存操作日志到数据库（包含 IP 和耗时）
func saveOperationLog(db *sql.DB, log *models.OperationLog) {
	query := `
        INSERT INTO operation_logs (
            id, operator_id, operator_name, operator_role, operation_type,
            operation_module, operation_desc, target_id, target_name,
            ip_address, user_agent, request_method, request_path, execution_time_ms, created_at
        ) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10, $11, NULLIF($12, ''), NULLIF($13, ''), $14, $15)
    `
	_, err := db.Exec(query,
		log.ID, log.OperatorID, log.OperatorName, log.OperatorRole,
		log.OperationType, log.OperationModule, log.OperationDesc,
		log.TargetID, log.TargetName, log.IPAddress, log.UserAgent,
		log.RequestMethod, log.RequestPath, log.ExecutionTimeMs, log.CreatedAt,
	)
	if err != nil {
		logx.Debug("saveOperationLog failed: %v", err)
	}
}

// SaveOperationLog 公共方法：供其他 handler 调用记录操作日志
func (h *OperationLogHandler) SaveOperationLog(c *gin.Context, operationType, operationModule, operationDesc, targetID, targetName string) {
	operatorID := c.GetString("userID")
	username := c.GetString("username")
	operatorName := resolveOperatorDisplayName(h.db, operatorID, username)
	operatorRole := c.GetString("role")

	if operatorID == "" && operatorName == "" {
		return
	}

	log := &models.OperationLog{
		ID:              uuid.New().String(),
		OperatorID:      operatorID,
		OperatorName:    operatorName,
		OperatorRole:    operatorRole,
		OperationType:   NormalizeOperationType(operationType),
		OperationModule: NormalizeOperationModule(operationModule),
		OperationDesc:   operationDesc,
		TargetID:        targetID,
		TargetName:      targetName,
		IPAddress:       c.ClientIP(),
		UserAgent:       c.Request.UserAgent(),
		RequestMethod:   c.Request.Method,
		RequestPath:     c.Request.URL.Path,
		ExecutionTimeMs: 0,
		CreatedAt:       time.Now(),
	}

	go saveOperationLog(h.db, log)
}

// GlobalLogger 全局操作日志处理器，在 main.go 中初始化
var GlobalLogger *OperationLogHandler

// Log 全局日志记录函数，方便在各个 handler 中调用
func Log(c *gin.Context, operationType, operationModule, operationDesc, targetID, targetName string) {
	if GlobalLogger != nil {
		GlobalLogger.SaveOperationLog(c, operationType, operationModule, operationDesc, targetID, targetName)
	}
}
func resolveOperatorDisplayName(db *sql.DB, operatorID, username string) string {
	var realName sql.NullString

	if operatorID != "" {
		if err := db.QueryRow(`SELECT real_name FROM users WHERE id::text = $1`, operatorID).Scan(&realName); err == nil {
			if realName.Valid && strings.TrimSpace(realName.String) != "" {
				return strings.TrimSpace(realName.String)
			}
		}
	}

	if username != "" {
		if err := db.QueryRow(`SELECT real_name FROM users WHERE username = $1`, username).Scan(&realName); err == nil {
			if realName.Valid && strings.TrimSpace(realName.String) != "" {
				return strings.TrimSpace(realName.String)
			}
		}
		return username
	}

	return ""
}

// ---- Source: backend/handlers/operation_log_labels.go ----
var operationTypeLabels = map[string]string{
	"CREATE":   "创建",
	"UPDATE":   "修改",
	"DELETE":   "删除",
	"UPLOAD":   "上传",
	"EXPORT":   "导出",
	"VIEW":     "查看",
	"DOWNLOAD": "下载",
	"LOGIN":    "登录",
	"LOGOUT":   "登出",
	"QUERY":    "查询",
	"POST":     "创建",
	"PUT":      "修改",
	"PATCH":    "修改",
	"OTHER":    "其他",
}
var operationModuleLabels = map[string]string{
	"USER":                "用户管理",
	"ROLE":                "角色管理",
	"DEVICE":              "设备管理",
	"RECORDING":           "录屏管理",
	"SCREENSHOT":          "截图管理",
	"ACTIVITY":            "活动日志",
	"POLICY":              "策略管理",
	"EMPLOYEE":            "员工管理",
	"ATTENDANCE":          "考勤管理",
	"PERFORMANCE":         "绩效管理",
	"PENALTY":             "罚款管理",
	"SITE_STATS":          "出款统计",
	"SALARY":              "工资管理",
	"TASK":                "任务管理",
	"TELEGRAM_ATTENDANCE": "Telegram考勤",
	"INVITE_CODE":         "邀请码管理",
	"EMPLOYEE_ACCOUNT":    "员工账号",
	"SYSTEM":              "系统设置",
	"AUTH":                "登录认证",
	"NOTIFICATION":        "消息通知",
	"REMOTE_VIEW":         "远程查看",
	"MEDIA":               "媒体访问",
	"DIAGNOSTIC":          "诊断中心",
	"DOMAIN_MONITOR":      "域名监控",
	"CALLBACK":            "运营工具",
	"OTHER":               "其他",
}

// NormalizeOperationType 将操作类型转为中文展示
func NormalizeOperationType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "其他"
	}
	if cn, ok := operationTypeLabels[strings.ToUpper(value)]; ok {
		return cn
	}
	if cn, ok := operationTypeLabels[value]; ok {
		return cn
	}
	return value
}

// NormalizeOperationModule 将操作模块转为中文展示
func NormalizeOperationModule(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "其他"
	}
	if cn, ok := operationModuleLabels[strings.ToUpper(value)]; ok {
		return cn
	}
	if cn, ok := operationModuleLabels[value]; ok {
		return cn
	}
	return value
}

// ---- Source: backend/handlers/penalty_push.go ----
func (h *AdminHandler) GetPenaltyPushSettings(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	settings, err := h.penaltyPush.GetSettings(c.Request.Context())
	if err != nil {
		models.SendError(c, 500, 5000, err.Error())
		return
	}
	models.Send(c, 200, 0, "success", settings)
}
func (h *AdminHandler) SavePenaltyPushToken(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	var req struct {
		BotToken string `json:"botToken"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	settings, err := h.penaltyPush.SaveBotToken(c.Request.Context(), req.BotToken)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	Log(c, "UPDATE", "PENALTY", "保存罚款 Telegram 推送 Bot Token", "", settings.BotUsername)
	models.Send(c, 200, 0, "Token 已保存", settings)
}
func (h *AdminHandler) SavePenaltyPushEnabled(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	settings, err := h.penaltyPush.SetPushEnabled(c.Request.Context(), req.Enabled)
	if err != nil {
		models.SendError(c, 500, 5000, err.Error())
		return
	}
	action := "关闭罚款 Telegram 自动推送"
	if req.Enabled {
		action = "开启罚款 Telegram 自动推送"
	}
	Log(c, "UPDATE", "PENALTY", action, "", fmt.Sprintf("enabled=%v", req.Enabled))
	models.Send(c, 200, 0, "已保存", settings)
}
func (h *AdminHandler) DiscoverPenaltyPushChats(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	groups, err := h.penaltyPush.DiscoverChats(c.Request.Context())
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	models.Send(c, 200, 0, "success", groups)
}
func (h *AdminHandler) BindPenaltyPushChat(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	var req struct {
		ChatID    string `json:"chatId" binding:"required"`
		ChatTitle string `json:"chatTitle"`
		ChatType  string `json:"chatType"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	settings, err := h.penaltyPush.BindChat(c.Request.Context(), req.ChatID, req.ChatTitle, req.ChatType)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	Log(c, "UPDATE", "PENALTY",
		fmt.Sprintf("绑定罚款推送频道/群组：%s", settings.ChatTitle),
		settings.ChatID, settings.ChatTitle)
	models.Send(c, 200, 0, "绑定成功", settings)
}
func (h *AdminHandler) UnbindPenaltyPushChat(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	settings, err := h.penaltyPush.UnbindChat(c.Request.Context())
	if err != nil {
		models.SendError(c, 500, 5000, err.Error())
		return
	}
	Log(c, "UPDATE", "PENALTY", "解绑罚款推送频道/群组", "", "")
	models.Send(c, 200, 0, "已解绑", settings)
}
func (h *AdminHandler) TestPenaltyPush(c *gin.Context) {
	if h.penaltyPush == nil {
		models.SendError(c, 500, 5000, "penalty push service unavailable")
		return
	}
	if err := h.penaltyPush.TestPush(c.Request.Context()); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	Log(c, "UPDATE", "PENALTY", "测试罚款 Telegram 推送", "", "")
	models.Send(c, 200, 0, "测试消息已发送", nil)
}
func (h *AdminHandler) NotifyPenaltyPush(c *gin.Context) {
	if h.penaltyPush == nil {
		models.Send(c, 200, 0, "skipped", gin.H{"sent": false})
		return
	}
	var req services.PenaltyPushRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	sent, err := h.penaltyPush.PushPenaltyRecord(c.Request.Context(), req)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	if sent {
		Log(c, "CREATE", "PENALTY",
			fmt.Sprintf("推送罚款：%s %s %s（%s）",
				req.Date, req.EmployeeName,
				services.FormatFineMoney(req.Amount, h.penaltyPush.FineCurrencyCode(c.Request.Context())),
				req.Reason),
			req.EmployeeName, req.EmployeeName)
	}
	models.Send(c, 200, 0, "success", gin.H{"sent": sent})
}

// ---- Source: backend/handlers/performance_push.go ----
func (h *AdminHandler) GetPerformancePushSettings(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	settings, err := h.perfPush.GetSettings(c.Request.Context())
	if err != nil {
		models.SendError(c, 500, 5000, err.Error())
		return
	}
	models.Send(c, 200, 0, "success", settings)
}
func (h *AdminHandler) SavePerformancePushToken(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	var req struct {
		BotToken string `json:"botToken"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	settings, err := h.perfPush.SaveBotToken(c.Request.Context(), req.BotToken)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	Log(c, "UPDATE", "PERFORMANCE", "保存绩效 Telegram 推送 Bot Token", "", settings.BotUsername)
	models.Send(c, 200, 0, "Token 已保存", settings)
}
func (h *AdminHandler) SavePerformancePushEnabled(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	settings, err := h.perfPush.SetPushEnabled(c.Request.Context(), req.Enabled)
	if err != nil {
		models.SendError(c, 500, 5000, err.Error())
		return
	}
	action := "关闭绩效 Telegram 自动推送"
	if req.Enabled {
		action = "开启绩效 Telegram 自动推送"
	}
	Log(c, "UPDATE", "PERFORMANCE", action, "", fmt.Sprintf("enabled=%v", req.Enabled))
	models.Send(c, 200, 0, "已保存", settings)
}
func (h *AdminHandler) DiscoverPerformancePushChats(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	groups, err := h.perfPush.DiscoverChats(c.Request.Context())
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	models.Send(c, 200, 0, "success", groups)
}
func (h *AdminHandler) BindPerformancePushChat(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	var req struct {
		ChatID    string `json:"chatId" binding:"required"`
		ChatTitle string `json:"chatTitle"`
		ChatType  string `json:"chatType"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	settings, err := h.perfPush.BindChat(c.Request.Context(), req.ChatID, req.ChatTitle, req.ChatType)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	Log(c, "UPDATE", "PERFORMANCE",
		fmt.Sprintf("绑定绩效推送频道/群组：%s", settings.ChatTitle),
		settings.ChatID, settings.ChatTitle)
	models.Send(c, 200, 0, "绑定成功", settings)
}
func (h *AdminHandler) UnbindPerformancePushChat(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	settings, err := h.perfPush.UnbindChat(c.Request.Context())
	if err != nil {
		models.SendError(c, 500, 5000, err.Error())
		return
	}
	Log(c, "UPDATE", "PERFORMANCE", "解绑绩效推送频道/群组", "", "")
	models.Send(c, 200, 0, "已解绑", settings)
}
func (h *AdminHandler) TestPerformancePush(c *gin.Context) {
	if h.perfPush == nil {
		models.SendError(c, 500, 5000, "performance push service unavailable")
		return
	}
	if err := h.perfPush.TestPush(c.Request.Context()); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	Log(c, "UPDATE", "PERFORMANCE", "测试绩效 Telegram 推送", "", "")
	models.Send(c, 200, 0, "测试消息已发送", nil)
}
func (h *AdminHandler) NotifyPerformancePush(c *gin.Context) {
	if h.perfPush == nil {
		models.Send(c, 200, 0, "skipped", gin.H{"sent": false})
		return
	}
	var req services.PerformancePushRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	sent, err := h.perfPush.PushScoreRecord(c.Request.Context(), req)
	if err != nil {
		models.SendError(c, 400, 1001, err.Error())
		return
	}
	if sent {
		change := "加分"
		if req.Score < 0 {
			change = "扣分"
		}
		Log(c, "CREATE", "PERFORMANCE",
			fmt.Sprintf("推送绩效%s：%s %s %d分（%s）", change, req.Date, req.EmployeeName, req.Score, req.Reason),
			req.EmployeeName, req.EmployeeName)
	}
	models.Send(c, 200, 0, "success", gin.H{"sent": sent})
}

func readSiteStatsRowsFromFileHeader(file *multipart.FileHeader) ([][]string, error) {
	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	return readSiteStatsRows(src, file.Filename)
}
func readSiteStatsRows(src io.Reader, filename string) ([][]string, error) {
	isCSV := strings.HasSuffix(strings.ToLower(filename), ".csv")
	if isCSV {
		reader := csvPackage2.NewReader(src)
		reader.Comma = ','
		reader.FieldsPerRecord = -1
		reader.TrimLeadingSpace = true
		reader.LazyQuotes = true
		rows, err := reader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("CSV文件解析失败: %w", err)
		}
		return rows, nil
	}

	f, err := excelize.OpenReader(src)
	if err != nil {
		return nil, fmt.Errorf("无法解析 Excel 文件: %w", err)
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ---- Source: backend/handlers/upload.go ----
// ========== 常量定义 ==========
const (
	sessionTTL                 = 1 * time.Hour // 会话过期时间
	sessionCleanupInterval     = 10 * time.Minute
	nonceCacheTTL              = 10 * time.Minute
	rateLimiterCleanupInterval = 5 * time.Minute // 限流器清理间隔
)

// ========== 上传会话结构 ==========
type uploadSession struct {
	UploadID       string
	DeviceID       string
	FileName       string
	FileType       string
	FileID         string
	TotalChunks    int
	Received       map[int]bool
	ChunkChecksums map[int]string
	LastActive     time.Time
	TotalSize      int64
	mu             sync.RWMutex
	Completed      bool      // 是否已完成
	Completing     bool      // 是否正在完成中（防并发）
	CompletedAt    time.Time // 完成时间（响应返回）
	// CompletedOnce sync.Once     // 确保只执行一次（双重保险）
}

// ========== 会话管理器 ==========
type uploadSessionManager struct {
	sessions map[string]*uploadSession
	mu       sync.RWMutex
	stopCh   chan struct{}
	once     sync.Once
}

var sessionManager = &uploadSessionManager{
	sessions: make(map[string]*uploadSession),
	stopCh:   make(chan struct{}),
}

// startSessionManager 启动会话管理器（单个 init）
func startSessionManager() {
	go sessionManager.cleanupLoop()
}
func init() {
	startSessionManager()
	startNonceCache()
	startRateLimiterCleanup()
}
func (m *uploadSessionManager) cleanupLoop() {
	ticker := time.NewTicker(sessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}
func (m *uploadSessionManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, session := range m.sessions {
		// 安全读取状态
		session.mu.RLock()
		completed := session.Completed
		completing := session.Completing
		lastActive := session.LastActive
		completedAt := session.CompletedAt
		session.mu.RUnlock()

		// ⭐⭐⭐ 最关键：正在完成的不删
		if completing {
			continue
		}

		// ✅ 活跃 session 过期清理（默认 1 小时）
		if !completed && now.Sub(lastActive) > sessionTTL {
			logPackage2.Printf("[Cleanup] Removing expired session: %s (last active: %v)",
				id, lastActive.Format("2006-01-02 15:04:05"))
			delete(m.sessions, id)
			purgeUploadArtifacts(id)
			m.removeSessionFromRedis(id)
			continue
		}

		// ✅ 已完成 session 兜底清理（60分钟，远超 Complete 的 30 分钟）
		if completed && !completedAt.IsZero() && now.Sub(completedAt) > 60*time.Minute {
			logPackage2.Printf("[Cleanup] Removing stale completed session: %s (completed at %v)",
				id, completedAt.Format("2006-01-02 15:04:05"))
			delete(m.sessions, id)
			purgeUploadArtifacts(id)
			m.removeSessionFromRedis(id)
		}
	}
}
func (m *uploadSessionManager) stop() {
	m.once.Do(func() {
		close(m.stopCh)
	})
}

// ========== Nonce 缓存（防重放攻击） ==========
type nonceCache struct {
	items  map[string]time.Time
	mu     sync.RWMutex
	stopCh chan struct{}
	once   sync.Once
}

var nonceCacheInstance = &nonceCache{
	items:  make(map[string]time.Time),
	stopCh: make(chan struct{}),
}

func startNonceCache() {
	go nonceCacheInstance.cleanupLoop()
}
func (c *nonceCache) cleanupLoop() {
	ticker := time.NewTicker(nonceCacheTTL)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}
func (c *nonceCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for nonce, createdAt := range c.items {
		if now.Sub(createdAt) > nonceCacheTTL {
			delete(c.items, nonce)
		}
	}
}
func (c *nonceCache) stop() {
	c.once.Do(func() {
		close(c.stopCh)
	})
}

// ========== IP 限流器（带最后使用时间） ==========
type rateLimiterEntry struct {
	*rate.Limiter
	lastUsed time.Time
}
type ipRateLimiter struct {
	limiters map[string]*rateLimiterEntry
	mu       sync.RWMutex
	stopCh   chan struct{}
	once     sync.Once
}

var globalRateLimiter = &ipRateLimiter{
	limiters: make(map[string]*rateLimiterEntry),
	stopCh:   make(chan struct{}),
}

func startRateLimiterCleanup() {
	go globalRateLimiter.cleanupLoop()
}
func (l *ipRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rateLimiterCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}
func (l *ipRateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for ip, entry := range l.limiters {
		if now.Sub(entry.lastUsed) > 1*time.Hour {
			delete(l.limiters, ip)
		}
	}
}
func (l *ipRateLimiter) stop() {
	l.once.Do(func() {
		close(l.stopCh)
	})
}

// ========== 上传并发控制器 ==========
type uploadConcurrencyController struct {
	uploads map[string]int
	mu      sync.RWMutex
}

func (c *uploadConcurrencyController) release(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count, exists := c.uploads[ip]; exists && count > 0 {
		c.uploads[ip]--
		if c.uploads[ip] == 0 {
			delete(c.uploads, ip)
		}
	}
}

const uploadSessionRedisPrefix = "upload:session:"

var uploadSessionRedis *redisPackage2.Client
var uploadChunkStore *services.UploadChunkStore
var uploadManifestStore *services.UploadManifestStore

func purgeUploadArtifacts(uploadID string) {
	if uploadManifestStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = uploadManifestStore.Delete(ctx, uploadID)
		cancel()
	}
	if uploadChunkStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := uploadChunkStore.DeleteAll(ctx, uploadID); err != nil {
			logPackage2.Printf("[Upload] failed to delete chunks for %s: %v", uploadID, err)
		}
	}
	os.RemoveAll(filepath.Join(os.TempDir(), "uploads", uploadID))
}
func uploadSessionRedisKey(uploadID string) string {
	return uploadSessionRedisPrefix + uploadID
}
func (m *uploadSessionManager) removeSessionFromRedis(uploadID string) {
	if uploadSessionRedis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := uploadSessionRedis.Del(ctx, uploadSessionRedisKey(uploadID)).Err(); err != nil {
		logPackage2.Printf("[UploadSession] Failed to delete session %s from Redis: %v", uploadID, err)
	}
}

func revokeUserSessions(ctx context.Context, userID uuid.UUID, reason string) error {
	svc := middleware.GetSessionService()
	if svc == nil {
		return fmt.Errorf("session service unavailable")
	}
	return svc.RevokeSubjectRefreshTokens(ctx, services.SessionSubjectUser, userID, reason)
}
