// Consolidated source; original file boundaries are marked below.
package middleware

import (
	context "context"
	sql "database/sql"
	gin "github.com/gin-gonic/gin"
	uuid "github.com/google/uuid"
	os "os"
	strconv "strconv"
	strings "strings"
	sync "sync"
	models "tg-attendance/backend/models"
	services "tg-attendance/backend/services"
	time "time"
)

func normalizeRoleName(roleName string) string {
	switch strings.TrimSpace(strings.ToLower(roleName)) {
	case "管理员", "admin", "administrator", "超级管理员", "superadmin":
		return "admin"
	case "审计员", "auditor":
		return "auditor"
	case "操作员", "operator", "ops":
		return "operator"
	case "员工", "employee":
		return "employee"
	case "查看员", "viewer":
		return "viewer"
	default:
		return "auditor"
	}
}

// RequirePermission checks permission using DB-backed role (JWT carries no permissions).
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetBool("standalonePolicyValidated") {
			c.Next()
			return
		}
		userID := c.GetString("userID")
		if userID == "" {
			models.SendError(c, 401, 1002, "User not authenticated")
			c.Abort()
			return
		}

		role := c.GetString("role")
		var perms []string
		if p, exists := c.Get("permissions"); exists {
			if list, ok := p.([]string); ok {
				perms = list
			}
		}
		if dbRole, dbPerms, ok := loadUserAuthFromDB(c.Request.Context(), userID); ok {
			c.Set("role", dbRole)
			c.Set("permissions", dbPerms)
			role = dbRole
			perms = dbPerms
		}

		if checkPermissionList(role, perms, permission) {
			c.Next()
			return
		}
		models.SendError(c, 403, 1003, "Permission denied: "+permission)
		c.Abort()
	}
}
func RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetBool("standalonePolicyValidated") {
			c.Next()
			return
		}
		userID := c.GetString("userID")
		if userID == "" {
			models.SendError(c, 401, 1002, "User not authenticated")
			c.Abort()
			return
		}

		role := c.GetString("role")
		var perms []string
		if p, exists := c.Get("permissions"); exists {
			if list, ok := p.([]string); ok {
				perms = list
			}
		}
		if dbRole, dbPerms, ok := loadUserAuthFromDB(c.Request.Context(), userID); ok {
			c.Set("role", dbRole)
			c.Set("permissions", dbPerms)
			role = dbRole
			perms = dbPerms
		}

		if checkAnyPermissionList(role, perms, permissions...) {
			c.Next()
			return
		}
		models.SendError(c, 403, 1003, "Permission denied")
		c.Abort()
	}
}

// ---- Source: backend/middleware/employee_scope.go ----
var employeeScopeDB *sql.DB

func InitEmployeeScopeDB(db *sql.DB) {
	employeeScopeDB = db
}

// ResolveScopedEmployeeID loads employees.id for portal user.
func ResolveScopedEmployeeID(c *gin.Context) (uuid.UUID, bool) {
	if employeeScopeDB == nil {
		return uuid.Nil, false
	}
	userID, err := uuid.Parse(c.GetString("userID"))
	if err != nil {
		return uuid.Nil, false
	}
	var empID uuid.UUID
	err = employeeScopeDB.QueryRowContext(c.Request.Context(),
		`SELECT id FROM employees WHERE user_id = $1 AND status = 1`, userID).Scan(&empID)
	if err != nil {
		return uuid.Nil, false
	}
	return empID, true
}

// IsAdminRole reports whether the authenticated user is an administrator.
func IsAdminRole(c *gin.Context) bool {
	return normalizeRoleName(c.GetString("role")) == "admin"
}

// ---- Source: backend/middleware/permissions.go ----
var (
	permissionDB  *sql.DB
	permissionTTL = 30 * time.Second
)

type permissionCacheEntry struct {
	role   string
	perms  []string
	expiry time.Time
}

var permissionCache sync.Map

// InitPermissionDB 启用 RequirePermission 的数据库实时权限校验（带 TTL 缓存）
func InitPermissionDB(db *sql.DB) {
	permissionDB = db
	if v := os.Getenv("PERMISSION_CACHE_TTL_SECONDS"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec >= 0 {
			permissionTTL = time.Duration(sec) * time.Second
		}
	}
}

func loadUserAuthFromDB(ctx context.Context, userID string) (role string, perms []string, ok bool) {
	if permissionDB == nil || userID == "" {
		return "", nil, false
	}
	if permissionTTL > 0 {
		if v, hit := permissionCache.Load(userID); hit {
			entry := v.(permissionCacheEntry)
			if time.Now().Before(entry.expiry) {
				return entry.role, entry.perms, true
			}
			permissionCache.Delete(userID)
		}
	}

	roleName := "auditor"
	err := permissionDB.QueryRowContext(ctx, `
		SELECT COALESCE(r.name, 'auditor')
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1
		LIMIT 1`, userID).Scan(&roleName)
	if err != nil && err != sql.ErrNoRows {
		return "", nil, false
	}

	rows, err := permissionDB.QueryContext(ctx, `
		SELECT DISTINCT jsonb_array_elements_text(r.permissions)
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return "", nil, false
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			continue
		}
		seen[perm] = struct{}{}
	}
	perms = make([]string, 0, len(seen))
	for p := range seen {
		perms = append(perms, p)
	}

	normalizedRole := normalizeRoleName(roleName)
	if permissionTTL > 0 {
		permissionCache.Store(userID, permissionCacheEntry{
			role:   normalizedRole,
			perms:  perms,
			expiry: time.Now().Add(permissionTTL),
		})
	}
	return normalizedRole, perms, true
}
func checkPermissionList(role string, perms []string, required string) bool {
	if normalizeRoleName(role) == "admin" {
		return true
	}
	for _, p := range perms {
		if p == required {
			return true
		}
	}
	return false
}
func checkAnyPermissionList(role string, perms []string, required ...string) bool {
	if normalizeRoleName(role) == "admin" {
		return true
	}
	allowed := make(map[string]struct{}, len(required))
	for _, p := range required {
		allowed[p] = struct{}{}
	}
	for _, p := range perms {
		if _, ok := allowed[p]; ok {
			return true
		}
	}
	return false
}

// UserHasPermission checks whether the authenticated user holds a permission.
func UserHasPermission(c *gin.Context, permission string) bool {
	if c.GetString("standaloneRole") != "" {
		if c.GetString("standaloneRole") == "admin" {
			return true
		}
		raw, _ := c.Get("permissions")
		perms, _ := raw.([]string)
		for _, p := range perms {
			if p == permission {
				return true
			}
		}
		return false
	}
	userID := c.GetString("userID")
	if userID == "" {
		return false
	}
	role := c.GetString("role")
	var perms []string
	if p, ok := c.Get("permissions"); ok {
		if list, ok := p.([]string); ok {
			perms = list
		}
	}
	if dbRole, dbPerms, ok := loadUserAuthFromDB(c.Request.Context(), userID); ok {
		role = dbRole
		perms = dbPerms
	}
	return checkPermissionList(role, perms, permission)
}

// ---- Source: backend/middleware/session_auth.go ----
var sessionSvc *services.SessionService

func GetSessionService() *services.SessionService {
	return sessionSvc
}

// ---- Source: backend/middleware/telegram_worker.go ----
// AuthenticateTelegramWorker validates the Telegram Worker shared secret from
// X-Telegram-Worker-Secret or Authorization: Bearer. On failure it writes 401
// and returns false (caller should Abort).
func AuthenticateTelegramWorker(c *gin.Context, telegram *services.WorkforceTelegramService) bool {
	secret := strings.TrimSpace(c.GetHeader("X-Telegram-Worker-Secret"))
	if secret == "" {
		secret = strings.TrimSpace(c.GetHeader("Authorization"))
		secret = strings.TrimPrefix(secret, "Bearer ")
	}
	if err := telegram.ValidateWorkerSecret(c.Request.Context(), secret); err != nil {
		models.SendError(c, 401, 1002, "invalid worker secret")
		return false
	}
	return true
}

// TelegramWorkerAuth is a Gin middleware that requires a valid worker secret.
// Mount on the authenticated /telegram/worker subgroup only — do not mount on /pair.
func TelegramWorkerAuth(telegram *services.WorkforceTelegramService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if telegram == nil || !AuthenticateTelegramWorker(c, telegram) {
			if telegram == nil {
				models.SendError(c, 401, 1002, "invalid worker secret")
			}
			c.Abort()
			return
		}
		c.Next()
	}
}
