// Consolidated source; original file boundaries are marked below.
package services

import (
	bytesPackage2 "bytes"
	context "context"
	aes "crypto/aes"
	cipher "crypto/cipher"
	rand "crypto/rand"
	sha256 "crypto/sha256"
	tls "crypto/tls"
	sql "database/sql"
	base64 "encoding/base64"
	hex "encoding/hex"
	json "encoding/json"
	errorsPackage2 "errors"
	fmt "fmt"
	uuid "github.com/google/uuid"
	minioPackage2 "github.com/minio/minio-go/v7"
	redisPackage2 "github.com/redis/go-redis/v9"
	bcrypt "golang.org/x/crypto/bcrypt"
	_ "golang.org/x/image/webp"
	html "html"
	_ "image/gif"
	io "io"
	logPackage2 "log"
	net "net"
	httpPackage2 "net/http"
	smtp "net/smtp"
	urlPackage2 "net/url"
	os "os"
	filepath "path/filepath"
	regexp "regexp"
	strconv "strconv"
	strings "strings"
	sync "sync"
	models "tg-attendance/backend/models"
	time "time"
)

// ---- Source: backend/services/attendance_review_notify.go ----
const notificationTypeAttendanceReview = "attendance_review_pending"

// NotifyAttendanceReviewPendingUsers inserts popup notifications for users who can open 考勤待确认.
// Layout bell unread also reflects live pending count via the notifications API.
func NotifyAttendanceReviewPendingUsers(
	ctx context.Context,
	db *sql.DB,
	taskID uuid.UUID,
	employeeName, attendanceDate, triggerType string,
) {
	if db == nil || taskID == uuid.Nil {
		return
	}
	kind := "早退"
	if strings.Contains(strings.ToLower(triggerType), "late") {
		kind = "迟到"
	}
	title := "考勤待确认"
	content := fmt.Sprintf("%s 有一条%s考勤异常待确认（%s），请前往【考勤绩效 - 待确认】处理。",
		strings.TrimSpace(employeeName), kind, strings.TrimSpace(attendanceDate))
	payload, _ := json.Marshal(map[string]interface{}{
		"type":           notificationTypeAttendanceReview,
		"taskId":         taskID.String(),
		"employeeName":   employeeName,
		"attendanceDate": attendanceDate,
		"triggerType":    triggerType,
		"path":           "/attendance/review",
	})

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT u.id::text
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles r ON r.id = ur.role_id
		WHERE u.status = 1
		  AND (
			LOWER(TRIM(r.name)) IN ('admin', 'administrator', '管理员', '超级管理员', 'superadmin')
			OR EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(r.permissions) AS p(perm)
				WHERE p.perm = $1
			)
		  )`, models.PermAttendanceReviewView)
	if err != nil {
		return
	}
	defer rows.Close()

	taskIDStr := taskID.String()
	for rows.Next() {
		var userID string
		if rows.Scan(&userID) != nil || strings.TrimSpace(userID) == "" {
			continue
		}
		var exists bool
		_ = db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM user_notifications
				WHERE user_id = $1 AND type = $2
				  AND COALESCE(payload->>'taskId','') = $3
			)`, userID, notificationTypeAttendanceReview, taskIDStr).Scan(&exists)
		if exists {
			continue
		}
		_, _ = db.ExecContext(ctx, `
			INSERT INTO user_notifications (id, user_id, type, title, content, payload, is_read, show_popup)
			VALUES ($1, $2, $3, $4, $5, $6, FALSE, TRUE)
		`, uuid.NewString(), userID, notificationTypeAttendanceReview, title, content, payload)
	}
}

// ---- Source: backend/services/capture_settings.go ----
// CaptureLinkedToCheckinEnvOverride returns (enabled, explicit) when TELEGRAM_AGENT_CAPTURE_ENABLED is set.
func CaptureLinkedToCheckinEnvOverride() (bool, bool) {
	v := strings.TrimSpace(os.Getenv("TELEGRAM_AGENT_CAPTURE_ENABLED"))
	if v == "" {
		return false, false
	}
	lower := strings.ToLower(v)
	return lower == "true" || lower == "1" || lower == "yes", true
}

// CaptureLinkedToCheckin reports whether check-in (client or Telegram) should start/stop capture.
// Default false: follow policy auto capture. Env var overrides DB when explicitly set.
func CaptureLinkedToCheckin(ctx context.Context, db *sql.DB) bool {
	if enabled, explicit := CaptureLinkedToCheckinEnvOverride(); explicit {
		return enabled
	}
	if db == nil {
		return false
	}
	var linked bool
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(capture_linked_to_checkin, FALSE)
		FROM wf_telegram_settings ORDER BY updated_at DESC LIMIT 1`).Scan(&linked)
	if err != nil {
		return false
	}
	return linked
}

// ---- Source: backend/services/cs_tool_settings.go ----
const (
	DefaultCsToolUrl    = "https://dh1888.github.io/168/"
	DefaultCsScriptUrl  = "https://dh1888.github.io/render/"
	maxCsToolCustomMenu = 30
)

var csToolMenuIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

type CsToolMenuItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}
type CsToolSettings struct {
	ToolUrl     string           `json:"toolUrl"`
	ScriptUrl   string           `json:"scriptUrl"`
	CustomMenus []CsToolMenuItem `json:"customMenus"`
	UpdatedAt   string           `json:"updatedAt,omitempty"`
}
type CsToolSettingsService struct {
	db *sql.DB
}

func (s *CsToolSettingsService) GetSettings(ctx context.Context) (*CsToolSettings, error) {
	out := &CsToolSettings{
		ToolUrl:     DefaultCsToolUrl,
		ScriptUrl:   DefaultCsScriptUrl,
		CustomMenus: []CsToolMenuItem{},
	}
	var updatedAt time.Time
	var menusJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT tool_url, script_url, COALESCE(custom_menus, '[]'::jsonb), updated_at
		FROM cs_tool_settings WHERE id = 1`).
		Scan(&out.ToolUrl, &out.ScriptUrl, &menusJSON, &updatedAt)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.ToolUrl = normalizeCsToolURL(out.ToolUrl, DefaultCsToolUrl)
	out.ScriptUrl = normalizeCsToolURL(out.ScriptUrl, DefaultCsScriptUrl)
	out.CustomMenus = decodeCsToolCustomMenus(menusJSON)
	out.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return out, nil
}
func decodeCsToolCustomMenus(raw []byte) []CsToolMenuItem {
	if len(raw) == 0 {
		return []CsToolMenuItem{}
	}
	var items []CsToolMenuItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return []CsToolMenuItem{}
	}
	normalized, err := normalizeCsToolCustomMenus(items)
	if err != nil {
		return []CsToolMenuItem{}
	}
	return normalized
}
func normalizeCsToolCustomMenus(items []CsToolMenuItem) ([]CsToolMenuItem, error) {
	if len(items) > maxCsToolCustomMenu {
		return nil, fmt.Errorf("自定义菜单最多 %d 个", maxCsToolCustomMenu)
	}
	reserved := map[string]bool{"tool": true, "script": true, "custom": true}
	seen := map[string]bool{}
	out := make([]CsToolMenuItem, 0, len(items))
	for _, item := range items {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		name := strings.TrimSpace(item.Name)
		link, err := validateCsToolURL(item.URL, "")
		if err != nil {
			return nil, fmt.Errorf("菜单「%s」跳转地址无效", nameOrID(name, id))
		}
		if id == "" || !csToolMenuIDPattern.MatchString(id) {
			return nil, fmt.Errorf("菜单「%s」标识无效", nameOrID(name, id))
		}
		if reserved[id] {
			return nil, fmt.Errorf("菜单标识 %q 为系统保留", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("菜单标识 %q 重复", id)
		}
		if name == "" {
			return nil, fmt.Errorf("菜单 %q 名称不能为空", id)
		}
		if len([]rune(name)) > 32 {
			return nil, fmt.Errorf("菜单「%s」名称过长", name)
		}
		seen[id] = true
		out = append(out, CsToolMenuItem{ID: id, Name: name, URL: link})
	}
	return out, nil
}
func nameOrID(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if strings.TrimSpace(id) != "" {
		return id
	}
	return "未命名"
}
func validateCsToolURL(raw, fallback string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		if fallback == "" {
			return "", ErrInvalidCsToolURL
		}
		return fallback, nil
	}
	u, err := urlPackage2.Parse(v)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ErrInvalidCsToolURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrInvalidCsToolURL
	}
	return u.String(), nil
}
func normalizeCsToolURL(raw, fallback string) string {
	v, err := validateCsToolURL(raw, fallback)
	if err != nil || v == "" {
		return fallback
	}
	return v
}

var ErrInvalidCsToolURL = &csToolURLError{}

type csToolURLError struct{}

func (e *csToolURLError) Error() string { return "跳转地址必须是有效的 http 或 https URL" }

// ---- Source: backend/services/mail.go ----
type MailService struct {
	enabled  bool
	host     string
	port     int
	user     string
	password string
	from     string
	fromName string
}

func (m *MailService) Enabled() bool {
	return m != nil && m.enabled
}
func (m *MailService) Send(_ context.Context, to, subject, body string) error {
	if m == nil || !m.enabled {
		return nil
	}
	to = strings.TrimSpace(to)
	if to == "" || !strings.Contains(to, "@") {
		return nil
	}

	fromHeader := m.from
	if m.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", encodeRFC2047(m.fromName), m.from)
	}

	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", fromHeader),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", encodeRFC2047(subject)),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		body,
	}, "\r\n")

	addr := net.JoinHostPort(m.host, strconv.Itoa(m.port))
	auth := smtp.PlainAuth("", m.user, m.password, m.host)

	if m.port == 465 {
		return m.sendTLS(addr, to, []byte(msg), auth)
	}
	return smtp.SendMail(addr, auth, m.from, []string{to}, []byte(msg))
}
func (m *MailService) sendTLS(addr, to string, msg []byte, auth smtp.Auth) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName: m.host,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil && m.user != "" {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(m.from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
func encodeRFC2047(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		if r > 127 {
			return fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(value)))
		}
	}
	return value
}

// ---- Source: backend/services/penalty_push.go ----
type PenaltyPushService struct {
	db *sql.DB
}

func NewPenaltyPushService(db *sql.DB) *PenaltyPushService {
	return &PenaltyPushService{db: db}
}

type PenaltyPushSettings struct {
	HasBotToken bool   `json:"hasBotToken"`
	BotUsername string `json:"botUsername"`
	ChatID      string `json:"chatId"`
	ChatTitle   string `json:"chatTitle"`
	ChatType    string `json:"chatType"`
	Enabled     bool   `json:"enabled"`
	Ready       bool   `json:"ready"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

func penaltyPushReady(hasBotToken bool, chatID string, enabled bool) bool {
	return enabled && hasBotToken && strings.TrimSpace(chatID) != ""
}

type PenaltyPushRecord struct {
	Date         string  `json:"date"`
	EmployeeID   string  `json:"employeeId"`
	EmployeeName string  `json:"employeeName"`
	Position     string  `json:"position"`
	Amount       float64 `json:"amount"`
	Category     string  `json:"category"`
	Reason       string  `json:"reason"`
}

func (s *PenaltyPushService) GetSettings(ctx context.Context) (*PenaltyPushSettings, error) {
	out := &PenaltyPushSettings{}
	var tokenEnc, botUsername, chatID, chatTitle, chatType sql.NullString
	var botID sql.NullInt64
	var enabled bool
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT bot_token_enc, bot_username, bot_id, chat_id, chat_title, chat_type, enabled, updated_at
		FROM penalty_push_settings WHERE id = 1`).
		Scan(&tokenEnc, &botUsername, &botID, &chatID, &chatTitle, &chatType, &enabled, &updatedAt)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.HasBotToken = tokenEnc.Valid && strings.TrimSpace(tokenEnc.String) != ""
	if botUsername.Valid {
		out.BotUsername = botUsername.String
	}
	if chatID.Valid {
		out.ChatID = chatID.String
	}
	if chatTitle.Valid {
		out.ChatTitle = chatTitle.String
	}
	if chatType.Valid {
		out.ChatType = chatType.String
	}
	out.Enabled = enabled
	out.Ready = penaltyPushReady(out.HasBotToken, out.ChatID, enabled)
	out.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return out, nil
}
func (s *PenaltyPushService) SetPushEnabled(ctx context.Context, enabled bool) (*PenaltyPushSettings, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO penalty_push_settings (id, enabled, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			updated_at = NOW()`, enabled)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PenaltyPushService) SaveBotToken(ctx context.Context, token string) (*PenaltyPushSettings, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("Bot Token 不能为空")
	}
	username, botID, err := performanceTelegramGetMe(token)
	if err != nil {
		return nil, err
	}
	enc, err := EncryptSecret(token)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO penalty_push_settings (id, bot_token_enc, bot_username, bot_id, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			bot_token_enc = EXCLUDED.bot_token_enc,
			bot_username = EXCLUDED.bot_username,
			bot_id = EXCLUDED.bot_id,
			updated_at = NOW()`, enc, username, botID)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PenaltyPushService) DiscoverChats(ctx context.Context) ([]map[string]string, error) {
	token, err := s.getBotToken(ctx)
	if err != nil {
		return nil, err
	}
	return DiscoverTelegramChats(ctx, s.db, token)
}
func (s *PenaltyPushService) BindChat(ctx context.Context, chatID, chatTitle, chatType string) (*PenaltyPushSettings, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("chatId required")
	}
	if _, err := s.getBotToken(ctx); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(chatTitle)
	if title == "" {
		title = chatID
	}
	chatType = strings.TrimSpace(chatType)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO penalty_push_settings (id, chat_id, chat_title, chat_type, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			chat_id = EXCLUDED.chat_id,
			chat_title = EXCLUDED.chat_title,
			chat_type = EXCLUDED.chat_type,
			updated_at = NOW()`, chatID, title, chatType)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PenaltyPushService) UnbindChat(ctx context.Context) (*PenaltyPushSettings, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE penalty_push_settings
		SET chat_id = NULL, chat_title = NULL, chat_type = NULL, updated_at = NOW()
		WHERE id = 1`)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PenaltyPushService) PushPenaltyRecord(ctx context.Context, rec PenaltyPushRecord) (bool, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return false, err
	}
	if !settings.Ready {
		return false, nil
	}
	token, err := s.getBotToken(ctx)
	if err != nil {
		return false, err
	}
	tgUID, username := lookupEmployeeTelegramProfile(ctx, s.db, rec.EmployeeID, rec.EmployeeName)
	text := buildPenaltyPushHTML(rec, tgUID, username, s.FineCurrencyCode(ctx))
	if err := performanceTelegramSendHTML(token, settings.ChatID, text); err != nil {
		return false, err
	}
	return true, nil
}
func (s *PenaltyPushService) TestPush(ctx context.Context) error {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.ChatID == "" {
		return fmt.Errorf("请先绑定频道或群组")
	}
	token, err := s.getBotToken(ctx)
	if err != nil {
		return err
	}
	text := "⚠️ <b>罚款推送测试</b>\n" + tgReplyDashedLine() + "\n配置成功，后续新增罚款将使用可点击姓名、可复制金额的模版推送到此频道/群组。"
	return performanceTelegramSendHTML(token, settings.ChatID, text)
}
func (s *PenaltyPushService) getBotToken(ctx context.Context) (string, error) {
	var enc sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT bot_token_enc FROM penalty_push_settings WHERE id = 1`).Scan(&enc)
	if err == sql.ErrNoRows || !enc.Valid || strings.TrimSpace(enc.String) == "" {
		return "", fmt.Errorf("请先配置 Bot Token")
	}
	if err != nil {
		return "", err
	}
	return DecryptSecret(enc.String)
}

// FineCurrencyCode reads TG settings fine currency (display only; default CNY).
func (s *PenaltyPushService) FineCurrencyCode(ctx context.Context) string {
	var code string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(NULLIF(TRIM(fine_currency), ''), 'CNY') FROM wf_telegram_settings WHERE id = 1`,
	).Scan(&code)
	if err != nil {
		return "CNY"
	}
	return NormalizeFineCurrency(code)
}
func buildPenaltyPushHTML(rec PenaltyPushRecord, telegramUserID int64, telegramUsername, fineCurrency string) string {
	return fmt.Sprintf(
		"⚠️ <b>罚款通知</b>\n"+
			"%s\n"+
			"👤 员工：%s\n"+
			"📅 日期：%s\n"+
			"💼 岗位：%s\n"+
			"💰 金额：%s\n"+
			"📂 类别：%s\n"+
			"📝 原因：%s",
		tgReplyDashedLine(),
		formatEmployeeProfileHTML(telegramUserID, telegramUsername, rec.EmployeeName),
		tgCopyableText(rec.Date),
		tgCopyableText(rec.Position),
		tgCopyableText(FormatFineMoney(rec.Amount, fineCurrency)),
		tgCopyableText(rec.Category),
		tgCopyableText(formatPenaltyReasonDuration(rec.Reason)),
	)
}
func lookupEmployeeTelegramUserID(ctx context.Context, db *sql.DB, employeeID, employeeName string) int64 {
	userID, _ := lookupEmployeeTelegramProfile(ctx, db, employeeID, employeeName)
	return userID
}
func lookupEmployeeTelegramProfile(ctx context.Context, db *sql.DB, employeeID, employeeName string) (userID int64, username string) {
	if db == nil {
		return 0, ""
	}
	scan := func(query string, args ...interface{}) bool {
		var id int64
		var user sql.NullString
		if err := db.QueryRowContext(ctx, query, args...).Scan(&id, &user); err != nil || id <= 0 {
			return false
		}
		userID = id
		username = strings.TrimSpace(user.String)
		return true
	}
	if id, err := uuid.Parse(strings.TrimSpace(employeeID)); err == nil && id != uuid.Nil {
		if scan(`
			SELECT telegram_user_id, COALESCE(telegram_username, '')
			FROM employee_telegram_bindings
			WHERE employee_id = $1 AND status = 1
			ORDER BY bound_at DESC NULLS LAST LIMIT 1`, id) {
			return userID, username
		}
		if scan(`
			SELECT b.telegram_user_id, COALESCE(b.telegram_username, '')
			FROM device_telegram_bindings b
			JOIN devices d ON d.id = b.device_id
			WHERE d.employee_uuid = $1 AND b.status = 1
			ORDER BY b.bound_at DESC NULLS LAST LIMIT 1`, id) {
			return userID, username
		}
		if scan(`
			SELECT b.telegram_user_id, COALESCE(b.telegram_username, '')
			FROM device_telegram_bindings b
			JOIN wf_shared_workstation_members m ON m.device_id = b.device_id
			WHERE m.employee_id = $1 AND b.status = 1
			ORDER BY b.bound_at DESC NULLS LAST LIMIT 1`, id) {
			return userID, username
		}
		if scan(`
			SELECT b.telegram_user_id, COALESCE(b.telegram_username, '')
			FROM work_sessions ws
			JOIN device_telegram_bindings b ON b.device_id = ws.device_id AND b.status = 1
			WHERE ws.employee_id = $1 AND ws.status = 'open' AND ws.device_id IS NOT NULL
			ORDER BY ws.checkin_time DESC LIMIT 1`, id) {
			return userID, username
		}
	}
	name := strings.TrimSpace(employeeName)
	if name == "" {
		return 0, ""
	}
	if scan(`
		SELECT b.telegram_user_id, COALESCE(b.telegram_username, '')
		FROM employee_telegram_bindings b
		JOIN employees e ON e.id = b.employee_id
		WHERE e.name = $1 AND b.status = 1
		ORDER BY b.bound_at DESC NULLS LAST LIMIT 1`, name) {
		return userID, username
	}
	scan(`
		SELECT b.telegram_user_id, COALESCE(b.telegram_username, '')
		FROM device_telegram_bindings b
		JOIN devices d ON d.id = b.device_id
		WHERE d.employee_name = $1 AND b.status = 1
		ORDER BY b.bound_at DESC NULLS LAST LIMIT 1`, name)
	return userID, username
}

// formatPenaltyReasonDuration converts trailing "N 秒" in reason text to 分/秒 display.
func formatPenaltyReasonDuration(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return reason
	}
	const suffix = " 秒"
	if !strings.HasSuffix(reason, suffix) {
		return reason
	}
	body := strings.TrimSuffix(reason, suffix)
	idx := strings.LastIndex(body, " ")
	if idx < 0 {
		return reason
	}
	numPart := strings.TrimSpace(body[idx+1:])
	prefix := strings.TrimSpace(body[:idx])
	var sec int
	if _, err := fmt.Sscanf(numPart, "%d", &sec); err != nil || sec < 0 {
		return reason
	}
	if prefix == "" {
		return formatDurationZh(sec)
	}
	return prefix + " " + formatDurationZh(sec)
}

// ---- Source: backend/services/performance_push.go ----
type PerformancePushService struct {
	db *sql.DB
}

func NewPerformancePushService(db *sql.DB) *PerformancePushService {
	return &PerformancePushService{db: db}
}

type PerformancePushSettings struct {
	HasBotToken bool   `json:"hasBotToken"`
	BotUsername string `json:"botUsername"`
	ChatID      string `json:"chatId"`
	ChatTitle   string `json:"chatTitle"`
	ChatType    string `json:"chatType"`
	Enabled     bool   `json:"enabled"`
	Ready       bool   `json:"ready"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

func performancePushReady(hasBotToken bool, chatID string, enabled bool) bool {
	return enabled && hasBotToken && strings.TrimSpace(chatID) != ""
}

type PerformancePushRecord struct {
	Date         string `json:"date"`
	EmployeeID   string `json:"employeeId"`
	EmployeeName string `json:"employeeName"`
	Position     string `json:"position"`
	Score        int    `json:"score"`
	Reason       string `json:"reason"`
}

func (s *PerformancePushService) GetSettings(ctx context.Context) (*PerformancePushSettings, error) {
	out := &PerformancePushSettings{}
	var tokenEnc, botUsername, chatID, chatTitle, chatType sql.NullString
	var botID sql.NullInt64
	var enabled bool
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT bot_token_enc, bot_username, bot_id, chat_id, chat_title, chat_type, enabled, updated_at
		FROM performance_push_settings WHERE id = 1`).
		Scan(&tokenEnc, &botUsername, &botID, &chatID, &chatTitle, &chatType, &enabled, &updatedAt)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.HasBotToken = tokenEnc.Valid && strings.TrimSpace(tokenEnc.String) != ""
	if botUsername.Valid {
		out.BotUsername = botUsername.String
	}
	if chatID.Valid {
		out.ChatID = chatID.String
	}
	if chatTitle.Valid {
		out.ChatTitle = chatTitle.String
	}
	if chatType.Valid {
		out.ChatType = chatType.String
	}
	out.Enabled = enabled
	out.Ready = performancePushReady(out.HasBotToken, out.ChatID, enabled)
	out.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return out, nil
}
func (s *PerformancePushService) SetPushEnabled(ctx context.Context, enabled bool) (*PerformancePushSettings, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO performance_push_settings (id, enabled, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			updated_at = NOW()`, enabled)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PerformancePushService) SaveBotToken(ctx context.Context, token string) (*PerformancePushSettings, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("Bot Token 不能为空")
	}
	username, botID, err := performanceTelegramGetMe(token)
	if err != nil {
		return nil, err
	}
	enc, err := EncryptSecret(token)
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO performance_push_settings (id, bot_token_enc, bot_username, bot_id, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			bot_token_enc = EXCLUDED.bot_token_enc,
			bot_username = EXCLUDED.bot_username,
			bot_id = EXCLUDED.bot_id,
			updated_at = NOW()`, enc, username, botID)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PerformancePushService) DiscoverChats(ctx context.Context) ([]map[string]string, error) {
	token, err := s.getBotToken(ctx)
	if err != nil {
		return nil, err
	}
	return DiscoverTelegramChats(ctx, s.db, token)
}
func (s *PerformancePushService) BindChat(ctx context.Context, chatID, chatTitle, chatType string) (*PerformancePushSettings, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("chatId required")
	}
	if _, err := s.getBotToken(ctx); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(chatTitle)
	if title == "" {
		title = chatID
	}
	chatType = strings.TrimSpace(chatType)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO performance_push_settings (id, chat_id, chat_title, chat_type, updated_at)
		VALUES (1, $1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			chat_id = EXCLUDED.chat_id,
			chat_title = EXCLUDED.chat_title,
			chat_type = EXCLUDED.chat_type,
			updated_at = NOW()`, chatID, title, chatType)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PerformancePushService) UnbindChat(ctx context.Context) (*PerformancePushSettings, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE performance_push_settings
		SET chat_id = NULL, chat_title = NULL, chat_type = NULL, updated_at = NOW()
		WHERE id = 1`)
	if err != nil {
		return nil, err
	}
	return s.GetSettings(ctx)
}
func (s *PerformancePushService) PushScoreRecord(ctx context.Context, rec PerformancePushRecord) (bool, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return false, err
	}
	if !settings.Ready {
		return false, nil
	}
	token, err := s.getBotToken(ctx)
	if err != nil {
		return false, err
	}
	tgUID, username := lookupEmployeeTelegramProfile(ctx, s.db, rec.EmployeeID, rec.EmployeeName)
	text := buildPerformancePushHTML(rec, tgUID, username)
	if err := performanceTelegramSendHTML(token, settings.ChatID, text); err != nil {
		return false, err
	}
	return true, nil
}
func (s *PerformancePushService) TestPush(ctx context.Context) error {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.ChatID == "" {
		return fmt.Errorf("请先绑定频道或群组")
	}
	token, err := s.getBotToken(ctx)
	if err != nil {
		return err
	}
	text := "📋 <b>绩效推送测试</b>\n" + tgReplyDashedLine() + "\n配置成功，后续新增绩效将使用可点击姓名、可复制分数的模版推送到此频道/群组。"
	return performanceTelegramSendHTML(token, settings.ChatID, text)
}
func (s *PerformancePushService) getBotToken(ctx context.Context) (string, error) {
	var enc sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT bot_token_enc FROM performance_push_settings WHERE id = 1`).Scan(&enc)
	if err == sql.ErrNoRows || !enc.Valid || strings.TrimSpace(enc.String) == "" {
		return "", fmt.Errorf("请先配置 Bot Token")
	}
	if err != nil {
		return "", err
	}
	return DecryptSecret(enc.String)
}
func buildPerformancePushHTML(rec PerformancePushRecord, telegramUserID int64, telegramUsername string) string {
	changeType := "加分"
	scoreText := fmt.Sprintf("+%d分", rec.Score)
	if rec.Score < 0 {
		changeType = "扣分"
		scoreText = fmt.Sprintf("%d分", rec.Score)
	}
	return fmt.Sprintf(
		"📋 <b>绩效变动通知</b>\n"+
			"%s\n"+
			"👤 员工：%s\n"+
			"📅 日期：%s\n"+
			"💼 岗位：%s\n"+
			"📊 变动：%s（%s）\n"+
			"📝 原因：%s",
		tgReplyDashedLine(),
		formatEmployeeProfileHTML(telegramUserID, telegramUsername, rec.EmployeeName),
		tgCopyableText(rec.Date),
		tgCopyableText(rec.Position),
		tgCopyableText(scoreText),
		html.EscapeString(changeType),
		tgCopyableText(rec.Reason),
	)
}
func performanceTelegramGetMe(token string) (string, int64, error) {
	client := &httpPackage2.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", 0, err
	}
	if !out.OK {
		if out.Description == "" {
			out.Description = "getMe failed"
		}
		return "", 0, fmt.Errorf("%s", out.Description)
	}
	return out.Result.Username, out.Result.ID, nil
}
func performanceTelegramSendHTML(token, chatID, htmlText string) error {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       htmlText,
		"parse_mode": "HTML",
	}
	body, _ := json.Marshal(payload)
	resp, err := httpPackage2.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		"application/json",
		bytesPackage2.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if !parsed.OK {
		if parsed.Description == "" {
			parsed.Description = string(raw)
		}
		return fmt.Errorf("telegram sendMessage: %s", parsed.Description)
	}
	return nil
}

func migrateSchema(db *sql.DB) error {
	migrations := []string{
		// 出款和考勤业务表（如果缺失则创建）
		`CREATE TABLE IF NOT EXISTS employees (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id VARCHAR(100),
            name VARCHAR(255),
            position VARCHAR(100),
            hire_date DATE,
            work_location VARCHAR(255),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employees_employee_id ON employees(employee_id)`,
		`CREATE TABLE IF NOT EXISTS attendance_records (
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            year_month VARCHAR(7) NOT NULL,
            date DATE NOT NULL,
            status VARCHAR(50) NOT NULL,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (employee_id, year_month, date)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_attendance_records_employee_id ON attendance_records(employee_id)`,
		`CREATE TABLE IF NOT EXISTS performance_records (
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            employee_name VARCHAR(255),
            position VARCHAR(100),
            score_records JSONB,
            total_score INTEGER DEFAULT 0,
            grade VARCHAR(50),
            month VARCHAR(7) NOT NULL,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (employee_id, month)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_performance_records_month ON performance_records(month)`,
		`CREATE TABLE IF NOT EXISTS penalty_records (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID REFERENCES employees(id) ON DELETE SET NULL,
            employee_name VARCHAR(255),
            position VARCHAR(100),
            amount NUMERIC(12,2) NOT NULL DEFAULT 0,
            category VARCHAR(100),
            reason TEXT,
            penalty_date DATE NOT NULL,
            created_by VARCHAR(100),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_penalty_records_employee_id ON penalty_records(employee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_penalty_records_penalty_date ON penalty_records(penalty_date)`,
		`CREATE TABLE IF NOT EXISTS sites (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            code VARCHAR(100) NOT NULL UNIQUE,
            name VARCHAR(255) NOT NULL,
            sort_order INTEGER NOT NULL DEFAULT 0,
            is_active BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE
        )`,
		`CREATE TABLE IF NOT EXISTS employee_accounts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
            name VARCHAR(255),
            account_name VARCHAR(255) NOT NULL,
            shift VARCHAR(50),
            is_active BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE,
            UNIQUE (site_id, account_name)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employee_accounts_site_id ON employee_accounts(site_id)`,
		`CREATE TABLE IF NOT EXISTS site_stats (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_account_id UUID NOT NULL REFERENCES employee_accounts(id) ON DELETE CASCADE,
            site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
            stat_date DATE NOT NULL,
            shift VARCHAR(50),
            order_count INTEGER NOT NULL DEFAULT 0,
            avg_time_seconds INTEGER NOT NULL DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE,
            UNIQUE (employee_account_id, stat_date, shift)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_site_stats_site_id ON site_stats(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_site_stats_employee_account_id ON site_stats(employee_account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_site_stats_stat_date ON site_stats(stat_date)`,

		// 用户表
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE`,

		// 策略表
		`ALTER TABLE policies ADD COLUMN IF NOT EXISTS is_current BOOLEAN DEFAULT FALSE`,

		// 录屏表
		`ALTER TABLE recordings ADD COLUMN IF NOT EXISTS duration INTEGER DEFAULT 0`,
		`ALTER TABLE recordings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,

		// 截图表
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS width INTEGER DEFAULT 0`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS height INTEGER DEFAULT 0`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS format VARCHAR(10) DEFAULT 'jpg'`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS uploaded_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS encrypted BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS employee_uuid UUID REFERENCES employees(id) ON DELETE SET NULL`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS employee_name VARCHAR(255)`,
		`ALTER TABLE screenshots ADD COLUMN IF NOT EXISTS employee_number VARCHAR(64)`,
		`CREATE INDEX IF NOT EXISTS idx_screenshots_employee_uuid ON screenshots(employee_uuid)`,

		// 活动日志表
		`ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS reported BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS reported_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0`,
		`ALTER TABLE activity_logs ADD COLUMN IF NOT EXISTS error_message TEXT DEFAULT ''`,

		`ALTER TABLE upload_tasks ADD COLUMN IF NOT EXISTS task_type VARCHAR(50) NOT NULL DEFAULT 'upload_recordings'`,
		`ALTER TABLE upload_tasks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()`,

		`ALTER TABLE cleanup_policy ADD COLUMN IF NOT EXISTS attendance_retention_months INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cleanup_policy ADD COLUMN IF NOT EXISTS performance_retention_months INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cleanup_policy ADD COLUMN IF NOT EXISTS penalty_retention_months INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cleanup_policy ADD COLUMN IF NOT EXISTS salary_retention_months INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cleanup_policy ADD COLUMN IF NOT EXISTS site_stats_retention_months INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE cleanup_logs ADD COLUMN IF NOT EXISTS deleted_business_records INTEGER NOT NULL DEFAULT 0`,

		// 索引（建议先添加列，后添加索引）
		`CREATE INDEX IF NOT EXISTS idx_screenshots_device_captured ON screenshots(device_id, captured_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_recordings_uploaded ON recordings(uploaded)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_device_started ON activity_logs(device_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_reported ON activity_logs(reported)`,

		`ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_status_check`,
		`ALTER TABLE devices ADD CONSTRAINT devices_status_check CHECK (status IN ('pending', 'approved', 'rejected'))`,

		// 修复错误数据
		`UPDATE devices SET status = 'approved' WHERE status NOT IN ('pending', 'approved', 'rejected')`,

		// ========== 工资记录表 ==========
		`CREATE TABLE IF NOT EXISTS salary_records (
            id VARCHAR(36) PRIMARY KEY,
            employee_id VARCHAR(100) NOT NULL,
            employee_name VARCHAR(255) NOT NULL,
            position VARCHAR(100),
            hire_date_dept VARCHAR(255),
            months_employed INTEGER DEFAULT 0,
            last_month_base DECIMAL(12,2) DEFAULT 0,
            total_increase DECIMAL(12,2) DEFAULT 0,
            base_salary DECIMAL(12,2) DEFAULT 0,
            attendance_days DECIMAL(10,2) DEFAULT 0,
            actual_base_salary DECIMAL(12,2) DEFAULT 0,
            attendance_adjustment DECIMAL(12,2) DEFAULT 0,
            half_year_unpaid_leave DECIMAL(12,2) DEFAULT 0,
            exchange_rate DECIMAL(10,4) DEFAULT 0,
            performance_score INTEGER DEFAULT 0,
            performance_bonus_b DECIMAL(12,2) DEFAULT 0,
            reward DECIMAL(12,2) DEFAULT 0,
            penalty DECIMAL(12,2) DEFAULT 0,
            advance DECIMAL(12,2) DEFAULT 0,
            other_deduction DECIMAL(12,2) DEFAULT 0,
            protection_return DECIMAL(12,2) DEFAULT 0,
            protection_fee DECIMAL(12,2) DEFAULT 0,
            total_salary DECIMAL(12,2) DEFAULT 0,
            remark TEXT,
            year_month VARCHAR(7) NOT NULL,
            paid_status VARCHAR(16) NOT NULL DEFAULT 'pending',
            paid_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(employee_id, year_month)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_salary_records_employee_id ON salary_records(employee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_salary_records_year_month ON salary_records(year_month)`,
		`CREATE INDEX IF NOT EXISTS idx_salary_records_employee_name ON salary_records(employee_name)`,
		`ALTER TABLE salary_records ADD COLUMN IF NOT EXISTS paid_status VARCHAR(16) NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE salary_records ADD COLUMN IF NOT EXISTS paid_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE salary_records ADD COLUMN IF NOT EXISTS sheet_values JSONB`,
		`CREATE TABLE IF NOT EXISTS salary_month_layouts (
            year_month VARCHAR(7) PRIMARY KEY,
            headers JSONB NOT NULL DEFAULT '[]'::jsonb,
            name_column_index INTEGER NOT NULL DEFAULT 1,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS salary_configs (
            config_key VARCHAR(64) PRIMARY KEY,
            config_name VARCHAR(64) NOT NULL DEFAULT '',
            total_increase DECIMAL(12,2) NOT NULL DEFAULT 0,
            full_attendance_days INTEGER NOT NULL DEFAULT 0,
            full_attendance_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
            full_attendance_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            exchange_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            cny_thb_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            cny_usd_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            thb_usd_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            performance_full_score_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
            performance_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            protection_fee DECIMAL(12,2) NOT NULL DEFAULT 0,
            protection_fee_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            payout_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            position_caps JSONB NOT NULL DEFAULT '[]'::jsonb,
            sort_order INTEGER NOT NULL DEFAULT 0,
            is_builtin BOOLEAN NOT NULL DEFAULT false,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`DROP TRIGGER IF EXISTS update_salary_records_updated_at ON salary_records;
        CREATE TRIGGER update_salary_records_updated_at
            BEFORE UPDATE ON salary_records
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		// ========== 员工收款地址表 ==========
		`CREATE TABLE IF NOT EXISTS employee_payment_accounts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL UNIQUE REFERENCES employees(id) ON DELETE CASCADE,
            account_holder VARCHAR(255) NOT NULL DEFAULT '',
            bank_name VARCHAR(255) NOT NULL DEFAULT '',
            bank_account VARCHAR(128) NOT NULL DEFAULT '',
            usdt_network VARCHAR(32) NOT NULL DEFAULT '',
            usdt_address VARCHAR(255) NOT NULL DEFAULT '',
            other_wallet_type VARCHAR(64) NOT NULL DEFAULT '',
            other_wallet_address VARCHAR(255) NOT NULL DEFAULT '',
            preferred_method VARCHAR(16) NOT NULL DEFAULT '',
            remark TEXT NOT NULL DEFAULT '',
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employee_payment_accounts_employee_id ON employee_payment_accounts(employee_id)`,
		`ALTER TABLE employee_payment_accounts ADD COLUMN IF NOT EXISTS preferred_method VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE employee_payment_accounts ADD COLUMN IF NOT EXISTS notification_email VARCHAR(255) NOT NULL DEFAULT ''`,
		`DROP TRIGGER IF EXISTS update_employee_payment_accounts_updated_at ON employee_payment_accounts;
        CREATE TRIGGER update_employee_payment_accounts_updated_at
            BEFORE UPDATE ON employee_payment_accounts
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		// ========== 工资打款截图 ==========
		`CREATE TABLE IF NOT EXISTS salary_payment_proofs (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            year_month VARCHAR(7) NOT NULL,
            file_path VARCHAR(512) NOT NULL,
            file_name VARCHAR(255) NOT NULL,
            content_type VARCHAR(128) NOT NULL DEFAULT 'image/jpeg',
            uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
            uploaded_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(employee_id, year_month)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_salary_payment_proofs_employee_month ON salary_payment_proofs(employee_id, year_month)`,

		// ========== 用户通知（公告、工资发放等）==========
		`CREATE TABLE IF NOT EXISTS user_notifications (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            type VARCHAR(32) NOT NULL DEFAULT 'announcement',
            title VARCHAR(255) NOT NULL,
            content TEXT NOT NULL,
            payload JSONB,
            is_read BOOLEAN NOT NULL DEFAULT FALSE,
            show_popup BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_user_notifications_user_id ON user_notifications(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_notifications_unread ON user_notifications(user_id, is_read) WHERE is_read = FALSE`,

		// ========== 操作日志表 ==========
		`CREATE TABLE IF NOT EXISTS operation_logs (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            operator_id VARCHAR(36),
            operator_name VARCHAR(100) NOT NULL,
            operator_role VARCHAR(50),
            operation_type VARCHAR(50) NOT NULL,
            operation_module VARCHAR(50) NOT NULL,
            operation_desc TEXT NOT NULL,
            target_id VARCHAR(100),
            target_name VARCHAR(255),
            ip_address VARCHAR(45),
            user_agent TEXT,
            request_method VARCHAR(10),
            request_path VARCHAR(255),
            request_params TEXT,
            response_status INTEGER,
            execution_time_ms INTEGER,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`CREATE INDEX IF NOT EXISTS idx_operation_logs_operator_name ON operation_logs(operator_name)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_logs_created_at ON operation_logs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_logs_operation_module ON operation_logs(operation_module)`,
		`CREATE INDEX IF NOT EXISTS idx_operation_logs_operation_type ON operation_logs(operation_type)`,
		// 兼容旧库：表已存在但缺少列时补齐（必须放在 CREATE TABLE 之后，否则新库会报 42P01）
		`ALTER TABLE operation_logs ADD COLUMN IF NOT EXISTS execution_time_ms INTEGER DEFAULT 0`,
		`ALTER TABLE operation_logs ADD COLUMN IF NOT EXISTS target_name VARCHAR(255)`,

		// Session SSOT + login security settings (existing DB upgrade)
		`CREATE TABLE IF NOT EXISTS auth_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            subject_type VARCHAR(16) NOT NULL,
            subject_id UUID NOT NULL,
            status VARCHAR(16) NOT NULL DEFAULT 'active',
            version INT NOT NULL DEFAULT 1,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            revoked_at TIMESTAMP WITH TIME ZONE,
            revoke_reason VARCHAR(64),
            client_ip INET,
            user_agent TEXT,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_subject ON auth_sessions(subject_type, subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_status ON auth_sessions(status) WHERE status = 'active'`,

		`CREATE TABLE IF NOT EXISTS system_settings (
            key VARCHAR(128) PRIMARY KEY,
            value TEXT NOT NULL DEFAULT '',
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`CREATE TABLE IF NOT EXISTS diagnostic_events (
            id UUID PRIMARY KEY,
            source VARCHAR(16) NOT NULL,
            device_id UUID NULL REFERENCES devices(id) ON DELETE SET NULL,
            module VARCHAR(64) NOT NULL,
            level VARCHAR(16) NOT NULL,
            message TEXT NOT NULL,
            context JSONB,
            correlation_id VARCHAR(128),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS idx_diagnostic_events_created ON diagnostic_events(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_diagnostic_events_expires ON diagnostic_events(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_diagnostic_events_module ON diagnostic_events(module, level, created_at DESC)`,

		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS salary_view_password_hash TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS screen_lock_password_hash TEXT`,

		`CREATE INDEX IF NOT EXISTS idx_upload_tasks_device_status ON upload_tasks(device_id, status)`,

		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS force_offlined_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_use_default BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_allow_user_exit BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_allow_auto_start_toggle BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_restart_on_kill BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_temp_exit_until TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,
		`CREATE TABLE IF NOT EXISTS device_exit_unlock_codes (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            code_hash TEXT NOT NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            created_by UUID REFERENCES users(id) ON DELETE SET NULL
        )`,
		`CREATE INDEX IF NOT EXISTS idx_device_exit_unlock_codes_device_active
            ON device_exit_unlock_codes(device_id, expires_at DESC)
            WHERE used_at IS NULL`,

		// ========== 安全事件（签名失败爆发 / 登录失败爆发等）==========
		`CREATE TABLE IF NOT EXISTS security_events (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            event_type VARCHAR(64) NOT NULL,
            subject_type VARCHAR(32) NOT NULL DEFAULT 'unknown',
            subject_id VARCHAR(128) NOT NULL DEFAULT '',
            ip_address VARCHAR(64),
            reason VARCHAR(255),
            event_count INTEGER NOT NULL DEFAULT 1,
            details JSONB,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_security_events_created ON security_events(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_security_events_type_created ON security_events(event_type, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_security_events_subject ON security_events(subject_type, subject_id, created_at DESC)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			// 记录警告但继续执行
			logPackage2.Printf("Migration warning (non-fatal): %v\nSQL: %s\n", err, migration)
		}
	}
	// Ensure devices table has device_secret column
	if _, err := db.Exec(`ALTER TABLE devices ADD COLUMN IF NOT EXISTS device_secret VARCHAR(128)`); err != nil {
		logPackage2.Printf("Warning: failed to add device_secret column: %v", err)
	}
	if err := ensureClientProtectionSchema(db); err != nil {
		logPackage2.Printf("Warning: failed to ensure client protection schema: %v", err)
	}

	// 为现有设备生成 device_secret（如果为空）
	if err := ensureDeviceSecrets(db); err != nil {
		logPackage2.Printf("Warning: failed to ensure device secrets: %v", err)
	}
	return nil
}
func ensureClientProtectionSchema(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS force_offlined_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_use_default BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_allow_user_exit BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_allow_auto_start_toggle BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_restart_on_kill BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_temp_exit_until TIMESTAMP WITH TIME ZONE`,
		`CREATE TABLE IF NOT EXISTS device_exit_unlock_codes (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            code_hash TEXT NOT NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            created_by UUID REFERENCES users(id) ON DELETE SET NULL
        )`,
		`CREATE INDEX IF NOT EXISTS idx_device_exit_unlock_codes_device_active
            ON device_exit_unlock_codes(device_id, expires_at DESC)
            WHERE used_at IS NULL`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ensureDeviceSecrets 为 devices 表中缺失 device_secret 的行生成随机 secret
func ensureDeviceSecrets(db *sql.DB) error {
	rows, err := db.Query(`SELECT id FROM devices WHERE device_secret IS NULL OR device_secret = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		// 生成 32 字节随机数，hex 编码
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			logPackage2.Printf("failed to generate device secret for %s: %v", id, err)
			continue
		}
		secret := hex.EncodeToString(b)
		if _, err := db.Exec(`UPDATE devices SET device_secret = $1 WHERE id = $2`, secret, id); err != nil {
			logPackage2.Printf("failed to store device secret for %s: %v", id, err)
			continue
		}
		logPackage2.Printf("Generated device_secret for device %s", id)
	}
	return nil
}

// ensureAuthSessionsSchema creates session/login-security tables required for SSOT auth.
func ensureAuthSessionsSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS auth_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            subject_type VARCHAR(16) NOT NULL,
            subject_id UUID NOT NULL,
            status VARCHAR(16) NOT NULL DEFAULT 'active',
            version INT NOT NULL DEFAULT 1,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            revoked_at TIMESTAMP WITH TIME ZONE,
            revoke_reason VARCHAR(64),
            client_ip INET,
            user_agent TEXT,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_subject ON auth_sessions(subject_type, subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_status ON auth_sessions(status) WHERE status = 'active'`,
		`CREATE TABLE IF NOT EXISTS system_settings (
            key VARCHAR(128) PRIMARY KEY,
            value TEXT NOT NULL DEFAULT '',
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS salary_view_password_hash TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS screen_lock_password_hash TEXT`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec failed: %w\nSQL: %s", err, stmt)
		}
	}
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'auth_sessions')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("auth_sessions table still missing after migration")
	}
	logPackage2.Println("auth_sessions schema ready")
	return nil
}

// closeDuplicateOpenWorkSessions keeps the newest open session per partition and closes the rest.
func closeDuplicateOpenWorkSessions(db *sql.DB, partitionBy string) error {
	_, err := db.Exec(fmt.Sprintf(`
		WITH ranked AS (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY %s ORDER BY checkin_time DESC) AS rn
			FROM work_sessions WHERE status = 'open'
		)
		UPDATE work_sessions ws
		SET status = 'closed', checkout_time = COALESCE(ws.checkout_time, NOW()), updated_at = NOW()
		FROM ranked r WHERE ws.id = r.id AND r.rn > 1`, partitionBy))
	return err
}
func closeDuplicateOpenActivitySessions(db *sql.DB) error {
	_, err := db.Exec(`
		WITH ranked AS (
			SELECT id, ROW_NUMBER() OVER (PARTITION BY employee_id ORDER BY started_at DESC) AS rn
			FROM wf_tg_activity_sessions WHERE status = 'open'
		)
		UPDATE wf_tg_activity_sessions ws
		SET status = 'closed', ended_at = COALESCE(ws.ended_at, NOW())
		FROM ranked r WHERE ws.id = r.id AND r.rn > 1`)
	return err
}

// ensureWorkforceSchema creates employee invite / session tables for existing DB upgrades.
func ensureWorkforceSchema(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS department VARCHAR(100)`,
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS status SMALLINT NOT NULL DEFAULT 1`,
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS resigned_month VARCHAR(7)`,
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_employees_user_id ON employees(user_id) WHERE user_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_employees_department ON employees(department)`,
		`CREATE INDEX IF NOT EXISTS idx_employees_status ON employees(status)`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS position_snapshot VARCHAR(100)`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS checkin_time TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS checkout_time TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS manually_edited BOOLEAN NOT NULL DEFAULT FALSE`,
		`UPDATE attendance_records SET status = 'work' WHERE status = 'working'`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS employee_uuid UUID REFERENCES employees(id) ON DELETE SET NULL`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS employee_session_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_ip VARCHAR(64)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_employee_uuid ON devices(employee_uuid)`,
		`CREATE TABLE IF NOT EXISTS device_invite_codes (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            code VARCHAR(32) NOT NULL UNIQUE,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            created_by UUID REFERENCES users(id) ON DELETE SET NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used_at TIMESTAMP WITH TIME ZONE,
            used_by_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
            revoked_at TIMESTAMP WITH TIME ZONE,
            superseded_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_invite_codes_employee ON device_invite_codes(employee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_codes_code ON device_invite_codes(code)`,
		`CREATE TABLE IF NOT EXISTS work_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            attendance_date DATE NOT NULL,
            device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
            checkin_time TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            checkout_time TIMESTAMP WITH TIME ZONE,
            status VARCHAR(20) NOT NULL DEFAULT 'open',
            recording_id UUID REFERENCES recordings(id) ON DELETE SET NULL,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_work_sessions_employee_date ON work_sessions(employee_id, attendance_date DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec failed: %w\nSQL: %s", err, stmt)
		}
	}
	if err := closeDuplicateOpenWorkSessions(db, "employee_id"); err != nil {
		return fmt.Errorf("dedupe open work_sessions: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_work_sessions_open ON work_sessions(employee_id) WHERE status = 'open'`); err != nil {
		return fmt.Errorf("exec failed: %w\nSQL: CREATE UNIQUE INDEX idx_work_sessions_open", err)
	}
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'device_invite_codes')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("device_invite_codes table still missing after migration")
	}
	logPackage2.Println("workforce schema ready")
	return nil
}

// ensureWorkforceTelegramSchema adds Telegram attendance integration tables (P0).
func ensureWorkforceTelegramSchema(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE work_sessions ADD COLUMN IF NOT EXISTS late_minutes INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE work_sessions ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'agent'`,

		`CREATE TABLE IF NOT EXISTS wf_telegram_settings (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            enabled BOOLEAN NOT NULL DEFAULT FALSE,
            bot_token_enc TEXT,
            bot_username VARCHAR(64),
            bot_id BIGINT,
            worker_secret VARCHAR(128),
            timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
            day_start VARCHAR(5) NOT NULL DEFAULT '09:00',
            day_end VARCHAR(5) NOT NULL DEFAULT '21:00',
            grace_minutes INT NOT NULL DEFAULT 5,
            dual_shift_enabled BOOLEAN NOT NULL DEFAULT FALSE,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`CREATE TABLE IF NOT EXISTS wf_telegram_groups (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            chat_id VARCHAR(64) NOT NULL UNIQUE,
            chat_title VARCHAR(256),
            enabled BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`CREATE TABLE IF NOT EXISTS employee_telegram_bindings (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            telegram_user_id BIGINT NOT NULL,
            telegram_username VARCHAR(128),
            chat_id VARCHAR(64),
            bound_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            status SMALLINT NOT NULL DEFAULT 1,
            UNIQUE(employee_id),
            UNIQUE(telegram_user_id)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employee_telegram_bindings_employee ON employee_telegram_bindings(employee_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec failed: %w\nSQL: %s", err, stmt)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wf_telegram_settings`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(`INSERT INTO wf_telegram_settings (enabled) VALUES (FALSE)`); err != nil {
			return err
		}
	}
	// Keep a single settings row. Extra rows + heartbeat bumps of updated_at can make
	// GetSettings (ORDER BY updated_at DESC) flip between configs (e.g. triple shift off).
	if count > 1 {
		if _, err := db.Exec(`
			DELETE FROM wf_telegram_settings
			WHERE id NOT IN (
				SELECT id FROM (
					SELECT id FROM wf_telegram_settings
					ORDER BY updated_at DESC NULLS LAST, id DESC
					LIMIT 1
				) keep_one
			)`); err != nil {
			return fmt.Errorf("dedupe wf_telegram_settings: %w", err)
		}
	}

	p1Stmts := []string{
		`CREATE TABLE IF NOT EXISTS wf_tg_activity_types (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            code VARCHAR(32) NOT NULL UNIQUE,
            name VARCHAR(64) NOT NULL,
            max_times_per_day INT NOT NULL DEFAULT 0,
            time_limit_seconds INT NOT NULL DEFAULT 300,
            sort_order INT NOT NULL DEFAULT 0,
            enabled BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_activity_fines (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            activity_code VARCHAR(32) NOT NULL REFERENCES wf_tg_activity_types(code) ON DELETE CASCADE,
            segment_minutes INT NOT NULL,
            fine_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
            UNIQUE(activity_code, segment_minutes)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_activity_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            activity_code VARCHAR(32) NOT NULL,
            activity_name VARCHAR(64) NOT NULL,
            attendance_date DATE NOT NULL,
            started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            ended_at TIMESTAMP WITH TIME ZONE,
            elapsed_seconds INT NOT NULL DEFAULT 0,
            limit_seconds INT NOT NULL DEFAULT 0,
            overtime_seconds INT NOT NULL DEFAULT 0,
            fine_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
            penalty_record_id UUID,
            status VARCHAR(16) NOT NULL DEFAULT 'open',
            source VARCHAR(32) NOT NULL DEFAULT 'telegram',
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_wf_tg_activity_sessions_employee ON wf_tg_activity_sessions(employee_id, attendance_date DESC)`,
	}
	for _, stmt := range p1Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p1 failed: %w\nSQL: %s", err, stmt)
		}
	}
	if err := closeDuplicateOpenActivitySessions(db); err != nil {
		return fmt.Errorf("dedupe open activity sessions: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_wf_tg_activity_sessions_open ON wf_tg_activity_sessions(employee_id) WHERE status = 'open'`); err != nil {
		return fmt.Errorf("exec p1 index failed: %w", err)
	}
	if err := seedDefaultTelegramActivityTypes(db); err != nil {
		return err
	}

	p2Stmts := []string{
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS night_start VARCHAR(5) NOT NULL DEFAULT '21:00'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS night_end VARCHAR(5) NOT NULL DEFAULT '09:00'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS daily_reset_time VARCHAR(5) NOT NULL DEFAULT '09:00'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS daily_reset_enabled BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS daily_reset_execution_offset_minutes INT NOT NULL DEFAULT 120`,
		`ALTER TABLE work_sessions ADD COLUMN IF NOT EXISTS shift_type VARCHAR(16) NOT NULL DEFAULT 'day'`,
		`CREATE TABLE IF NOT EXISTS wf_tg_group_shift_state (
            chat_id VARCHAR(64) PRIMARY KEY,
            current_shift VARCHAR(16) NOT NULL DEFAULT 'day',
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_handovers (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            employee_name VARCHAR(128) NOT NULL DEFAULT '',
            chat_id VARCHAR(64),
            from_shift VARCHAR(16) NOT NULL,
            to_shift VARCHAR(16) NOT NULL,
            note TEXT,
            handover_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_wf_tg_handovers_at ON wf_tg_handovers(handover_at DESC)`,
		`CREATE TABLE IF NOT EXISTS wf_tg_daily_reset_log (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            reset_date DATE NOT NULL,
            closed_work_sessions INT NOT NULL DEFAULT 0,
            closed_activities INT NOT NULL DEFAULT 0,
            run_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(reset_date)
        )`,
	}
	for _, stmt := range p2Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p2 failed: %w\nSQL: %s", err, stmt)
		}
	}

	p4Stmts := []string{
		`DROP INDEX IF EXISTS idx_work_sessions_open`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS shift_window_disabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS work_end_grace_minutes INT NOT NULL DEFAULT 5`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS require_pc_login BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS capture_linked_to_checkin BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS web_punch_only BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS invited_employee_uuid UUID REFERENCES employees(id) ON DELETE SET NULL`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS checkin_source VARCHAR(32)`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS checkout_source VARCHAR(32)`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS shift_type VARCHAR(16)`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS activity_seconds INT`,
		// Backfill from still-present work_sessions (before future daily purge removes them).
		`UPDATE attendance_records ar
			SET shift_type = ws.shift_type
			FROM (
				SELECT DISTINCT ON (employee_id, attendance_date)
					employee_id, attendance_date, COALESCE(NULLIF(TRIM(shift_type), ''), 'day') AS shift_type
				FROM work_sessions
				WHERE COALESCE(TRIM(shift_type), '') <> ''
				ORDER BY employee_id, attendance_date, checkin_time DESC
			) ws
			WHERE ar.employee_id = ws.employee_id
			  AND ar.date = ws.attendance_date
			  AND (ar.shift_type IS NULL OR TRIM(ar.shift_type) = '')`,
		`UPDATE attendance_records ar
			SET activity_seconds = COALESCE(act.total_sec, 0)
			FROM (
				SELECT employee_id, attendance_date, COALESCE(SUM(elapsed_seconds), 0)::int AS total_sec
				FROM wf_tg_activity_sessions
				WHERE status = 'closed'
				GROUP BY employee_id, attendance_date
			) act
			WHERE ar.employee_id = act.employee_id
			  AND ar.date = act.attendance_date
			  AND ar.activity_seconds IS NULL
			  AND ar.checkin_time IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS wf_tg_bind_tokens (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            token VARCHAR(32) NOT NULL UNIQUE,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used_at TIMESTAMP WITH TIME ZONE,
            used_by_telegram_user_id BIGINT,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_wf_tg_bind_tokens_employee ON wf_tg_bind_tokens(employee_id)`,
		`ALTER TABLE wf_tg_activity_types ADD COLUMN IF NOT EXISTS max_concurrent INT NOT NULL DEFAULT 0`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS shift_type VARCHAR(16) NOT NULL DEFAULT 'day'`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS is_shared_workstation BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE TABLE IF NOT EXISTS wf_shared_workstation_members (
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (device_id, employee_id)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_work_fines (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            checkin_type VARCHAR(16) NOT NULL,
            segment_minutes INT NOT NULL,
            fine_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
            UNIQUE(checkin_type, segment_minutes)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_handover_configs (
            chat_id VARCHAR(64) PRIMARY KEY,
            handover_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            handover_day INT NOT NULL DEFAULT 0,
            handover_month INT NOT NULL DEFAULT 0,
            handover_day_start_time VARCHAR(5) NOT NULL DEFAULT '15:00',
            night_start_time VARCHAR(5) NOT NULL DEFAULT '21:00',
            day_start_time VARCHAR(5) NOT NULL DEFAULT '09:00',
            handover_night_hours INT NOT NULL DEFAULT 18,
            handover_day_hours INT NOT NULL DEFAULT 18,
            normal_night_hours INT NOT NULL DEFAULT 12,
            normal_day_hours INT NOT NULL DEFAULT 12,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_employee_shift_state (
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            chat_id VARCHAR(64) NOT NULL DEFAULT '',
            shift_type VARCHAR(16) NOT NULL,
            record_date DATE NOT NULL,
            session_id UUID,
            started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (employee_id, shift_type)
        )`,
		`DROP TABLE IF EXISTS wf_tg_group_shift_state`,
		`CREATE TABLE IF NOT EXISTS wf_tg_group_shift_state (
            chat_id VARCHAR(64) NOT NULL,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            shift_type VARCHAR(16) NOT NULL,
            record_date DATE NOT NULL,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (chat_id, employee_id, shift_type)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_handover_cycles (
            chat_id VARCHAR(64) NOT NULL,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            handover_date DATE NOT NULL,
            shift_type VARCHAR(16) NOT NULL,
            cycle_number INT NOT NULL,
            cycle_start_time TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            total_work_seconds INT NOT NULL DEFAULT 0,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (chat_id, employee_id, handover_date, shift_type, cycle_number)
        )`,
	}
	for _, stmt := range p4Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p4 failed: %w\nSQL: %s", err, stmt)
		}
	}
	if err := closeDuplicateOpenWorkSessions(db, "employee_id, shift_type"); err != nil {
		return fmt.Errorf("dedupe open work_sessions by shift: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_work_sessions_open_shift ON work_sessions(employee_id, shift_type) WHERE status = 'open'`); err != nil {
		return fmt.Errorf("exec p4 index failed: %w", err)
	}

	p5Stmts := []string{
		`ALTER TABLE wf_tg_handover_configs ADD COLUMN IF NOT EXISTS handover_reset_threshold_hours NUMERIC(6,2) NOT NULL DEFAULT 12`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS source_chat_id VARCHAR(64)`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS telegram_message_id BIGINT`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS limit_warning_sent BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS auto_ended BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE TABLE IF NOT EXISTS wf_tg_push_settings (
            chat_id VARCHAR(64) PRIMARY KEY,
            channel_id VARCHAR(64),
            notification_group_id VARCHAR(64),
            extra_work_group_id VARCHAR(64),
            enable_channel_push BOOLEAN NOT NULL DEFAULT TRUE,
            enable_group_push BOOLEAN NOT NULL DEFAULT TRUE,
            enable_admin_push BOOLEAN NOT NULL DEFAULT FALSE,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_monthly_activity_stats (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            year_month VARCHAR(7) NOT NULL,
            chat_id VARCHAR(64) NOT NULL DEFAULT '',
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            shift_type VARCHAR(16) NOT NULL DEFAULT 'day',
            activity_code VARCHAR(64) NOT NULL,
            activity_name VARCHAR(128) NOT NULL DEFAULT '',
            activity_count INT NOT NULL DEFAULT 0,
            total_seconds INT NOT NULL DEFAULT 0,
            fine_total NUMERIC(12,2) NOT NULL DEFAULT 0,
            archived_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(year_month, chat_id, employee_id, shift_type, activity_code)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_monthly_work_stats (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            year_month VARCHAR(7) NOT NULL,
            chat_id VARCHAR(64) NOT NULL DEFAULT '',
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            shift_type VARCHAR(16) NOT NULL DEFAULT 'day',
            session_count INT NOT NULL DEFAULT 0,
            work_fine_total NUMERIC(12,2) NOT NULL DEFAULT 0,
            archived_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(year_month, chat_id, employee_id, shift_type)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_reset_export_log (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            reset_date DATE NOT NULL,
            export_path TEXT,
            archived_month VARCHAR(7),
            closed_work_sessions INT NOT NULL DEFAULT 0,
            closed_activities INT NOT NULL DEFAULT 0,
            run_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(reset_date)
        )`,
		`CREATE TABLE IF NOT EXISTS wf_tg_relay_states (
            chat_id VARCHAR(64) NOT NULL,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            relay_handover_date DATE NOT NULL,
            successor_record_date DATE NOT NULL,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (chat_id, employee_id)
        )`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS admin_telegram_user_ids TEXT`,
	}
	for _, stmt := range p5Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p5 failed: %w\nSQL: %s", err, stmt)
		}
	}
	p7Stmts := []string{
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS grace_minutes_after INT NOT NULL DEFAULT 360`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS work_end_grace_after INT NOT NULL DEFAULT 360`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS shift_grace_before INT`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS shift_grace_after INT`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS work_end_grace_before INT`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS work_end_grace_after INT`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS daily_reset_time VARCHAR(5)`,
		`ALTER TABLE work_sessions ADD COLUMN IF NOT EXISTS shift_detail VARCHAR(16)`,
		`ALTER TABLE work_sessions ADD COLUMN IF NOT EXISTS source_chat_id VARCHAR(64)`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS telegram_message_id BIGINT`,
		`DROP INDEX IF EXISTS idx_work_sessions_open_shift`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS overtime_immediate_sent BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS overtime_5min_sent BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS last_overtime_reminder_min INT NOT NULL DEFAULT -1`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS force_back_sent BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_tg_activity_sessions ADD COLUMN IF NOT EXISTS end_source VARCHAR(32)`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS bot_mode VARCHAR(16) NOT NULL DEFAULT 'polling'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS webhook_url TEXT`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS webhook_secret VARCHAR(64)`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS bot_ui_lang VARCHAR(8) NOT NULL DEFAULT 'both'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS fine_currency VARCHAR(16) NOT NULL DEFAULT 'CNY'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS triple_shift_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS middle_start VARCHAR(5) NOT NULL DEFAULT '13:00'`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS middle_end VARCHAR(5) NOT NULL DEFAULT '01:00'`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS bot_ui_lang VARCHAR(8)`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS worker_pairing_code VARCHAR(16)`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS worker_pairing_expires_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS worker_last_seen_at TIMESTAMP WITH TIME ZONE`,
		`CREATE TABLE IF NOT EXISTS wf_telegram_discovered_chats (
			chat_id VARCHAR(64) PRIMARY KEY,
			chat_title VARCHAR(256),
			chat_type VARCHAR(32),
			first_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wf_tg_discovered_last_seen ON wf_telegram_discovered_chats(last_seen_at DESC)`,
		`CREATE TABLE IF NOT EXISTS wf_tg_group_reset_log (
			chat_id VARCHAR(64) NOT NULL,
			reset_date DATE NOT NULL,
			closed_work_sessions INT NOT NULL DEFAULT 0,
			closed_activities INT NOT NULL DEFAULT 0,
			run_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			UNIQUE(chat_id, reset_date)
		)`,
		`ALTER TABLE employee_telegram_bindings ADD COLUMN IF NOT EXISTS last_group_bot_message_id BIGINT`,
		`CREATE TABLE IF NOT EXISTS wf_tg_activity_sessions_archive (
			id UUID PRIMARY KEY,
			employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
			activity_code VARCHAR(32) NOT NULL,
			activity_name VARCHAR(64) NOT NULL,
			attendance_date DATE NOT NULL,
			started_at TIMESTAMP WITH TIME ZONE NOT NULL,
			ended_at TIMESTAMP WITH TIME ZONE,
			elapsed_seconds INT NOT NULL DEFAULT 0,
			overtime_seconds INT NOT NULL DEFAULT 0,
			fine_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
			status VARCHAR(16) NOT NULL,
			source VARCHAR(32) NOT NULL DEFAULT 'telegram',
			end_source VARCHAR(32),
			shift_type VARCHAR(16) NOT NULL DEFAULT 'day',
			source_chat_id VARCHAR(64),
			archived_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wf_tg_activity_archive_emp ON wf_tg_activity_sessions_archive(employee_id, attendance_date DESC)`,
	}
	for _, stmt := range p7Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p7 failed: %w\nSQL: %s", err, stmt)
		}
	}
	if err := closeDuplicateOpenWorkSessions(db, "employee_id, shift_type, attendance_date"); err != nil {
		return fmt.Errorf("dedupe open work_sessions by shift/date: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_work_sessions_open_shift_date ON work_sessions(employee_id, shift_type, attendance_date) WHERE status = 'open'`); err != nil {
		return fmt.Errorf("exec p7 index failed: %w", err)
	}
	p8Stmts := []string{
		`CREATE TABLE IF NOT EXISTS performance_push_settings (
            id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
            bot_token_enc TEXT NOT NULL DEFAULT '',
            bot_username VARCHAR(128),
            bot_id BIGINT,
            chat_id VARCHAR(64),
            chat_title VARCHAR(255),
            chat_type VARCHAR(32),
            enabled BOOLEAN NOT NULL DEFAULT FALSE,
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS penalty_push_settings (
            id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
            bot_token_enc TEXT NOT NULL DEFAULT '',
            bot_username VARCHAR(128),
            bot_id BIGINT,
            chat_id VARCHAR(64),
            chat_title VARCHAR(255),
            chat_type VARCHAR(32),
            enabled BOOLEAN NOT NULL DEFAULT FALSE,
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS agent_notification_settings (
            id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
            enabled BOOLEAN NOT NULL DEFAULT TRUE,
            salary_paid_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            penalty_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            performance_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            attendance_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            activity_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            default_duration_sec INT NOT NULL DEFAULT 300,
            startup_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            local_login_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            local_attendance_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            local_bind_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            local_duration_sec INT NOT NULL DEFAULT 60,
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`,
		`INSERT INTO agent_notification_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING`,
		`ALTER TABLE agent_notification_settings ADD COLUMN IF NOT EXISTS startup_enabled BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE agent_notification_settings ADD COLUMN IF NOT EXISTS local_login_enabled BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE agent_notification_settings ADD COLUMN IF NOT EXISTS local_attendance_enabled BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE agent_notification_settings ADD COLUMN IF NOT EXISTS local_bind_enabled BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE agent_notification_settings ADD COLUMN IF NOT EXISTS local_duration_sec INT NOT NULL DEFAULT 60`,
		`CREATE TABLE IF NOT EXISTS cs_tool_settings (
            id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
            tool_url TEXT NOT NULL DEFAULT 'https://dh1888.github.io/168/',
            script_url TEXT NOT NULL DEFAULT 'https://dh1888.github.io/render/',
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`,
		`INSERT INTO cs_tool_settings (id, tool_url, script_url) VALUES (
            1,
            'https://dh1888.github.io/168/',
            'https://dh1888.github.io/render/'
        ) ON CONFLICT (id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS cs_tool_shift_roster (
            id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
            payload JSONB NOT NULL DEFAULT '{}'::jsonb,
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`,
		`INSERT INTO cs_tool_shift_roster (id, payload) VALUES (1, '{}'::jsonb) ON CONFLICT (id) DO NOTHING`,
	}
	for _, stmt := range p8Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p8 failed: %w\nSQL: %s", err, stmt)
		}
	}
	p9Stmts := []string{
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS late_tolerance_minutes INT`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS early_leave_tolerance_minutes INT`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS check_out_grace_before INT`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS late_tolerance_minutes INT`,
		`ALTER TABLE wf_telegram_groups ADD COLUMN IF NOT EXISTS early_leave_tolerance_minutes INT`,
	}
	for _, stmt := range p9Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p9 failed: %w\nSQL: %s", err, stmt)
		}
	}
	if _, err := db.Exec(`
		UPDATE wf_telegram_settings SET
			late_tolerance_minutes = COALESCE(late_tolerance_minutes, grace_minutes_after, 360),
			early_leave_tolerance_minutes = COALESCE(early_leave_tolerance_minutes, work_end_grace_minutes, 120),
			check_out_grace_before = COALESCE(check_out_grace_before, 120)`); err != nil {
		return fmt.Errorf("backfill grace tolerance settings: %w", err)
	}
	if _, err := db.Exec(`
		UPDATE wf_telegram_groups SET
			early_leave_tolerance_minutes = COALESCE(early_leave_tolerance_minutes, work_end_grace_before)
		WHERE work_end_grace_before IS NOT NULL AND early_leave_tolerance_minutes IS NULL`); err != nil {
		return fmt.Errorf("backfill group early leave tolerance: %w", err)
	}
	if _, err := db.Exec(`
		ALTER TABLE wf_telegram_settings
			ALTER COLUMN late_tolerance_minutes SET DEFAULT 360,
			ALTER COLUMN early_leave_tolerance_minutes SET DEFAULT 120,
			ALTER COLUMN check_out_grace_before SET DEFAULT 120`); err != nil {
		return fmt.Errorf("set grace tolerance defaults: %w", err)
	}
	p10Stmts := []string{
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS attendance_mode VARCHAR(32) NOT NULL DEFAULT 'pc_tg'`,
		`CREATE TABLE IF NOT EXISTS device_telegram_bindings (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            telegram_user_id BIGINT NOT NULL,
            telegram_username VARCHAR(128),
            chat_id VARCHAR(64),
            bound_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            status SMALLINT NOT NULL DEFAULT 1,
            UNIQUE(device_id)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_device_telegram_bindings_device ON device_telegram_bindings(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_device_telegram_bindings_tg_user ON device_telegram_bindings(telegram_user_id)`,
	}
	for _, stmt := range p10Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p10 failed: %w\nSQL: %s", err, stmt)
		}
	}
	// Allow one Telegram account to bind multiple shared-workstation devices.
	// Existing installs may still have UNIQUE(telegram_user_id); drop it safely.
	if _, err := db.Exec(`ALTER TABLE device_telegram_bindings DROP CONSTRAINT IF EXISTS device_telegram_bindings_telegram_user_id_key`); err != nil {
		return fmt.Errorf("drop device_telegram_bindings telegram_user_id unique: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_device_telegram_bindings_tg_user ON device_telegram_bindings(telegram_user_id)`); err != nil {
		return fmt.Errorf("create idx_device_telegram_bindings_tg_user: %w", err)
	}
	p11Stmts := []string{
		`CREATE TABLE IF NOT EXISTS work_sessions_archive (
			id UUID PRIMARY KEY,
			employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
			attendance_date DATE NOT NULL,
			device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
			checkin_time TIMESTAMP WITH TIME ZONE NOT NULL,
			checkout_time TIMESTAMP WITH TIME ZONE,
			status VARCHAR(20) NOT NULL,
			recording_id UUID,
			source VARCHAR(32) NOT NULL DEFAULT 'agent',
			shift_type VARCHAR(16) NOT NULL DEFAULT 'day',
			shift_detail VARCHAR(16),
			source_chat_id VARCHAR(64),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			archived_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_work_sessions_archive_emp_date ON work_sessions_archive(employee_id, attendance_date DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_work_sessions_archive_date ON work_sessions_archive(attendance_date DESC)`,
		`CREATE TABLE IF NOT EXISTS wf_tg_monthly_export_log (
			year_month VARCHAR(7) NOT NULL,
			chat_id VARCHAR(64) NOT NULL DEFAULT '',
			export_path TEXT,
			pushed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			PRIMARY KEY (year_month, chat_id)
		)`,
		`CREATE TABLE IF NOT EXISTS wf_tg_attendance_review_tasks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
			attendance_date DATE NOT NULL,
			year_month VARCHAR(7) NOT NULL,
			shift_type VARCHAR(16) NOT NULL DEFAULT 'day',
			chat_id VARCHAR(64) NOT NULL DEFAULT '',
			trigger_type VARCHAR(32) NOT NULL DEFAULT 'early_checkout_review',
			work_seconds INT NOT NULL DEFAULT 0,
			early_minutes INT NOT NULL DEFAULT 0,
			overtime_threshold_hours INT NOT NULL DEFAULT 5,
			early_threshold_hours INT NOT NULL DEFAULT 5,
			proposed_fine NUMERIC(12,2) NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			tg_message_id BIGINT,
			confirmed_by VARCHAR(128),
			confirm_source VARCHAR(32),
			confirmed_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			UNIQUE(employee_id, attendance_date, shift_type, trigger_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wf_tg_att_review_month ON wf_tg_attendance_review_tasks(year_month, status, created_at DESC)`,
	}
	for _, stmt := range p11Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p11 failed: %w\nSQL: %s", err, stmt)
		}
	}
	p12Stmts := []string{
		`ALTER TABLE wf_telegram_settings ALTER COLUMN late_tolerance_minutes SET DEFAULT 0`,
		`ALTER TABLE wf_telegram_settings ALTER COLUMN early_leave_tolerance_minutes SET DEFAULT 10`,
		`ALTER TABLE wf_tg_handover_configs ALTER COLUMN handover_day SET DEFAULT 0`,
		`UPDATE wf_telegram_settings SET late_tolerance_minutes = 0 WHERE late_tolerance_minutes = 360`,
		`UPDATE wf_telegram_settings SET early_leave_tolerance_minutes = 10 WHERE early_leave_tolerance_minutes = 120`,
		`UPDATE wf_telegram_groups SET late_tolerance_minutes = 0 WHERE late_tolerance_minutes = 360`,
		`UPDATE wf_telegram_groups SET early_leave_tolerance_minutes = 10 WHERE early_leave_tolerance_minutes = 120`,
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS show_keyboard_info_buttons BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE wf_telegram_settings ALTER COLUMN show_keyboard_info_buttons SET DEFAULT FALSE`,
	}
	for _, stmt := range p12Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p12 failed: %w\nSQL: %s", err, stmt)
		}
	}
	p13Stmts := []string{
		`CREATE TABLE IF NOT EXISTS wf_tg_user_lang_prefs (
			telegram_user_id BIGINT PRIMARY KEY,
			preferred_lang VARCHAR(8) NOT NULL DEFAULT 'zh',
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`,
	}
	for _, stmt := range p13Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p13 failed: %w\nSQL: %s", err, stmt)
		}
	}
	p14Stmts := []string{
		`ALTER TABLE cs_tool_settings ADD COLUMN IF NOT EXISTS custom_menus JSONB NOT NULL DEFAULT '[]'::jsonb`,
	}
	for _, stmt := range p14Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p14 failed: %w\nSQL: %s", err, stmt)
		}
	}
	// Punch reply language is per employee (shared TG / multi-device must not inherit).
	p15Stmts := []string{
		`CREATE TABLE IF NOT EXISTS wf_tg_employee_lang_prefs (
			employee_id UUID PRIMARY KEY REFERENCES employees(id) ON DELETE CASCADE,
			preferred_lang VARCHAR(8) NOT NULL DEFAULT 'zh',
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`,
		// One-time: copy personal TG prefs only (exactly one employee binding per telegram user).
		`INSERT INTO wf_tg_employee_lang_prefs (employee_id, preferred_lang, updated_at)
		SELECT b.employee_id, p.preferred_lang, COALESCE(p.updated_at, NOW())
		FROM wf_tg_user_lang_prefs p
		INNER JOIN employee_telegram_bindings b
			ON b.telegram_user_id = p.telegram_user_id AND b.status = 1
		WHERE (
			SELECT COUNT(*) FROM employee_telegram_bindings b2
			WHERE b2.telegram_user_id = p.telegram_user_id AND b2.status = 1
		) = 1
		ON CONFLICT (employee_id) DO NOTHING`,
	}
	for _, stmt := range p15Stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec p15 failed: %w\nSQL: %s", err, stmt)
		}
	}
	logPackage2.Println("workforce telegram schema ready")
	return nil
}
func seedDefaultTelegramActivityTypes(db *sql.DB) error {
	defaults := []struct {
		code, name string
		limitSec   int
		maxTimes   int
		order      int
	}{
		{"wc_small", "小厕", 180, 0, 1},
		{"wc_large", "大厕", 600, 0, 2},
		{"smoke", "抽烟", 300, 0, 3},
		{"eat", "吃饭", 1800, 0, 4},
	}
	for _, d := range defaults {
		_, err := db.Exec(`
			INSERT INTO wf_tg_activity_types (code, name, max_times_per_day, time_limit_seconds, sort_order, enabled)
			VALUES ($1, $2, $3, $4, $5, TRUE)
			ON CONFLICT (code) DO NOTHING`,
			d.code, d.name, d.maxTimes, d.limitSec, d.order)
		if err != nil {
			return err
		}
	}
	return nil
}
func initEmbeddedSchema(db *sql.DB) error {
	statements := []string{
		// `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,

		// ========== 用户表 ==========
		`CREATE TABLE IF NOT EXISTS users (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            username VARCHAR(100) NOT NULL UNIQUE,
            password_hash VARCHAR(255) NOT NULL,
            email VARCHAR(255),
            real_name VARCHAR(100),
            status SMALLINT NOT NULL DEFAULT 1,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE,
            last_login_at TIMESTAMP WITH TIME ZONE
        )`,

		// ========== 角色表 ==========
		`CREATE TABLE IF NOT EXISTS roles (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            name VARCHAR(100) NOT NULL UNIQUE,
            description TEXT,
            permissions JSONB NOT NULL DEFAULT '[]',
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		// ========== 用户角色关联表 ==========
		`CREATE TABLE IF NOT EXISTS user_roles (
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (user_id, role_id)
        )`,

		// ========== 设备表 ==========
		`CREATE TABLE IF NOT EXISTS devices (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id VARCHAR(100),
            employee_name VARCHAR(255),
            employee_number VARCHAR(100),
            position VARCHAR(100),
            device_fingerprint VARCHAR(255) NOT NULL UNIQUE,
            hostname VARCHAR(255) NOT NULL,
            os_version VARCHAR(255),
            agent_version VARCHAR(50),
            status VARCHAR(20) NOT NULL DEFAULT 'pending',
            bound_at TIMESTAMP WITH TIME ZONE,
            last_seen_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE,
            device_secret VARCHAR(128)
        )`,

		// ========== 设备表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_devices_device_fingerprint ON devices(device_fingerprint)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen_at)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_employee_id ON devices(employee_id)`,

		// ========== 录屏记录表 ==========
		`CREATE TABLE IF NOT EXISTS recordings (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            start_time TIMESTAMP WITH TIME ZONE NOT NULL,
            end_time TIMESTAMP WITH TIME ZONE NOT NULL,
            file_path TEXT NOT NULL,
            file_size BIGINT NOT NULL DEFAULT 0,
            sha256 VARCHAR(64),
            uploaded BOOLEAN NOT NULL DEFAULT FALSE,
            duration INTEGER DEFAULT 0,
            updated_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		// ========== 录屏表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_recordings_device_id ON recordings(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_recordings_start_time ON recordings(start_time)`,
		`CREATE INDEX IF NOT EXISTS idx_recordings_uploaded ON recordings(uploaded)`,
		`CREATE INDEX IF NOT EXISTS idx_recordings_device_start ON recordings(device_id, start_time DESC)`,

		// ========== 截图表（✅ 添加新字段） ==========
		`CREATE TABLE IF NOT EXISTS screenshots (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            captured_at TIMESTAMP WITH TIME ZONE NOT NULL,
            file_path TEXT NOT NULL,
            file_size BIGINT NOT NULL DEFAULT 0,
            uploaded BOOLEAN NOT NULL DEFAULT FALSE,
            width INTEGER DEFAULT 0,
            height INTEGER DEFAULT 0,
            format VARCHAR(10) DEFAULT 'jpg',
            uploaded_at TIMESTAMP WITH TIME ZONE,
            encrypted BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		// ========== 截图表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_screenshots_device_id ON screenshots(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_screenshots_captured_at ON screenshots(captured_at)`,
		`CREATE INDEX IF NOT EXISTS idx_screenshots_device_captured ON screenshots(device_id, captured_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_screenshots_uploaded ON screenshots(uploaded)`,

		// ========== 活动日志表（✅ 添加新字段） ==========
		`CREATE TABLE IF NOT EXISTS activity_logs (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            process_name VARCHAR(255),
            window_title TEXT,
            browser_title TEXT,
            started_at TIMESTAMP WITH TIME ZONE NOT NULL,
            ended_at TIMESTAMP WITH TIME ZONE,
            is_idle BOOLEAN NOT NULL DEFAULT FALSE,
            duration_seconds INTEGER NOT NULL DEFAULT 0,
            reported BOOLEAN DEFAULT FALSE,
            reported_at TIMESTAMP WITH TIME ZONE,
            retry_count INTEGER DEFAULT 0,
            error_message TEXT DEFAULT '',
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		// ========== 活动日志表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_device_id ON activity_logs(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_started_at ON activity_logs(started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_process_name ON activity_logs(process_name)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_device_started ON activity_logs(device_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_reported ON activity_logs(reported)`,

		// ========== 策略表 ==========
		`CREATE TABLE IF NOT EXISTS policies (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            name VARCHAR(200) NOT NULL,
            version INTEGER NOT NULL DEFAULT 1,
            content JSONB NOT NULL,
            signature VARCHAR(255),
            is_current BOOLEAN NOT NULL DEFAULT FALSE,
            status SMALLINT NOT NULL DEFAULT 1,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE
        )`,

		// ========== 策略表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_policies_is_current ON policies(is_current) WHERE is_current = TRUE`,
		`CREATE INDEX IF NOT EXISTS idx_policies_version ON policies(version)`,

		// ========== 上传任务表 ==========
		`CREATE TABLE IF NOT EXISTS upload_tasks (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            task_type VARCHAR(50) NOT NULL DEFAULT 'upload_recordings',
            start_time TIMESTAMP WITH TIME ZONE NOT NULL,
            end_time TIMESTAMP WITH TIME ZONE,
            status VARCHAR(20) NOT NULL DEFAULT 'pending',
            progress INTEGER NOT NULL DEFAULT 0,
            result TEXT,
            completed_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		// ========== 上传任务表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_upload_tasks_device_id ON upload_tasks(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_upload_tasks_status ON upload_tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_upload_tasks_created_at ON upload_tasks(created_at)`,

		// ========== 清理策略表 ==========
		`CREATE TABLE IF NOT EXISTS cleanup_policy (
            id VARCHAR(50) PRIMARY KEY DEFAULT 'default',
            auto_cleanup_enabled BOOLEAN NOT NULL DEFAULT TRUE,
            recording_retention_days INTEGER NOT NULL DEFAULT 30,
            screenshot_retention_days INTEGER NOT NULL DEFAULT 30,
            log_retention_days INTEGER NOT NULL DEFAULT 90,
            disk_usage_threshold_percent INTEGER NOT NULL DEFAULT 80,
            cleanup_interval_minutes INTEGER NOT NULL DEFAULT 60,
            last_run_at TIMESTAMP WITH TIME ZONE,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		// ========== 清理日志表 ==========
		`CREATE TABLE IF NOT EXISTS cleanup_logs (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            trigger_type VARCHAR(20) NOT NULL,
            status VARCHAR(20) NOT NULL,
            deleted_recordings INTEGER NOT NULL DEFAULT 0,
            deleted_screenshots INTEGER NOT NULL DEFAULT 0,
            deleted_logs INTEGER NOT NULL DEFAULT 0,
            deleted_bytes BIGINT NOT NULL DEFAULT 0,
            message TEXT,
            started_at TIMESTAMP WITH TIME ZONE NOT NULL,
            completed_at TIMESTAMP WITH TIME ZONE NOT NULL
        )`,

		// ========== 清理日志表索引 ==========
		`CREATE INDEX IF NOT EXISTS idx_cleanup_logs_started_at ON cleanup_logs(started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_cleanup_logs_trigger_type ON cleanup_logs(trigger_type)`,

		// ========== 出款与考勤业务表 ==========
		`CREATE TABLE IF NOT EXISTS employees (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id VARCHAR(100),
            name VARCHAR(255),
            position VARCHAR(100),
            hire_date DATE,
            work_location VARCHAR(255),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employees_employee_id ON employees(employee_id)`,
		`CREATE TABLE IF NOT EXISTS attendance_records (
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            year_month VARCHAR(7) NOT NULL,
            date DATE NOT NULL,
            status VARCHAR(50) NOT NULL,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (employee_id, year_month, date)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_attendance_records_employee_id ON attendance_records(employee_id)`,
		`CREATE TABLE IF NOT EXISTS performance_records (
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            employee_name VARCHAR(255),
            position VARCHAR(100),
            score_records JSONB,
            total_score INTEGER DEFAULT 0,
            grade VARCHAR(50),
            month VARCHAR(7) NOT NULL,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            PRIMARY KEY (employee_id, month)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_performance_records_month ON performance_records(month)`,
		`CREATE TABLE IF NOT EXISTS penalty_records (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID REFERENCES employees(id) ON DELETE SET NULL,
            employee_name VARCHAR(255),
            position VARCHAR(100),
            amount NUMERIC(12,2) NOT NULL DEFAULT 0,
            category VARCHAR(100),
            reason TEXT,
            penalty_date DATE NOT NULL,
            created_by VARCHAR(100),
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_penalty_records_employee_id ON penalty_records(employee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_penalty_records_penalty_date ON penalty_records(penalty_date)`,
		`CREATE TABLE IF NOT EXISTS sites (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            code VARCHAR(100) NOT NULL UNIQUE,
            name VARCHAR(255) NOT NULL,
            sort_order INTEGER NOT NULL DEFAULT 0,
            is_active BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE
        )`,
		`CREATE TABLE IF NOT EXISTS employee_accounts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
            name VARCHAR(255),
            account_name VARCHAR(255) NOT NULL,
            shift VARCHAR(50),
            is_active BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE,
            UNIQUE (site_id, account_name)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employee_accounts_site_id ON employee_accounts(site_id)`,
		`CREATE TABLE IF NOT EXISTS site_stats (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_account_id UUID NOT NULL REFERENCES employee_accounts(id) ON DELETE CASCADE,
            site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
            stat_date DATE NOT NULL,
            shift VARCHAR(50),
            order_count INTEGER NOT NULL DEFAULT 0,
            avg_time_seconds INTEGER NOT NULL DEFAULT 0,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE,
            UNIQUE (employee_account_id, stat_date, shift)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_site_stats_site_id ON site_stats(site_id)`,
		`CREATE INDEX IF NOT EXISTS idx_site_stats_employee_account_id ON site_stats(employee_account_id)`,
		`CREATE INDEX IF NOT EXISTS idx_site_stats_stat_date ON site_stats(stat_date)`,

		// ========== 添加 updated_at 自动更新触发器 ==========
		`CREATE OR REPLACE FUNCTION update_updated_at_column()
        RETURNS TRIGGER AS $$
        BEGIN
            NEW.updated_at = NOW();
            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql`,

		// ========== 为 users 表添加触发器 ==========
		`DROP TRIGGER IF EXISTS update_users_updated_at ON users;
        CREATE TRIGGER update_users_updated_at
            BEFORE UPDATE ON users
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		// ========== 为 policies 表添加触发器 ==========
		`DROP TRIGGER IF EXISTS update_policies_updated_at ON policies;
        CREATE TRIGGER update_policies_updated_at
            BEFORE UPDATE ON policies
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		`ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_status_check`,
		`ALTER TABLE devices ADD CONSTRAINT devices_status_check CHECK (status IN ('pending', 'approved', 'rejected'))`,

		// ========== 工资记录表 ==========
		`CREATE TABLE IF NOT EXISTS salary_records (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id VARCHAR(100) NOT NULL,
            employee_name VARCHAR(255) NOT NULL,
            position VARCHAR(100),
            hire_date_dept VARCHAR(255),
            months_employed INTEGER DEFAULT 0,
            last_month_base DECIMAL(12,2) DEFAULT 0,
            total_increase DECIMAL(12,2) DEFAULT 0,
            base_salary DECIMAL(12,2) DEFAULT 0,
            attendance_days DECIMAL(10,2) DEFAULT 0,
            actual_base_salary DECIMAL(12,2) DEFAULT 0,
            attendance_adjustment DECIMAL(12,2) DEFAULT 0,
            half_year_unpaid_leave DECIMAL(12,2) DEFAULT 0,
            exchange_rate DECIMAL(10,4) DEFAULT 0,
            performance_score INTEGER DEFAULT 0,
            performance_bonus_b DECIMAL(12,2) DEFAULT 0,
            reward DECIMAL(12,2) DEFAULT 0,
            penalty DECIMAL(12,2) DEFAULT 0,
            advance DECIMAL(12,2) DEFAULT 0,
            other_deduction DECIMAL(12,2) DEFAULT 0,
            protection_return DECIMAL(12,2) DEFAULT 0,
            protection_fee DECIMAL(12,2) DEFAULT 0,
            total_salary DECIMAL(12,2) DEFAULT 0,
            remark TEXT,
            year_month VARCHAR(7) NOT NULL,
            paid_status VARCHAR(16) NOT NULL DEFAULT 'pending',
            paid_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(employee_id, year_month)
        )`,

		`CREATE INDEX IF NOT EXISTS idx_salary_records_employee_id ON salary_records(employee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_salary_records_year_month ON salary_records(year_month)`,
		`CREATE INDEX IF NOT EXISTS idx_salary_records_employee_name ON salary_records(employee_name)`,
		`ALTER TABLE salary_records ADD COLUMN IF NOT EXISTS paid_status VARCHAR(16) NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE salary_records ADD COLUMN IF NOT EXISTS paid_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE salary_records ADD COLUMN IF NOT EXISTS sheet_values JSONB`,
		`CREATE TABLE IF NOT EXISTS salary_month_layouts (
            year_month VARCHAR(7) PRIMARY KEY,
            headers JSONB NOT NULL DEFAULT '[]'::jsonb,
            name_column_index INTEGER NOT NULL DEFAULT 1,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE TABLE IF NOT EXISTS salary_configs (
            config_key VARCHAR(64) PRIMARY KEY,
            config_name VARCHAR(64) NOT NULL DEFAULT '',
            total_increase DECIMAL(12,2) NOT NULL DEFAULT 0,
            full_attendance_days INTEGER NOT NULL DEFAULT 0,
            full_attendance_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
            full_attendance_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            exchange_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            cny_thb_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            cny_usd_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            thb_usd_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
            performance_full_score_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
            performance_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            protection_fee DECIMAL(12,2) NOT NULL DEFAULT 0,
            protection_fee_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            payout_currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
            position_caps JSONB NOT NULL DEFAULT '[]'::jsonb,
            sort_order INTEGER NOT NULL DEFAULT 0,
            is_builtin BOOLEAN NOT NULL DEFAULT false,
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`DROP TRIGGER IF EXISTS update_salary_records_updated_at ON salary_records;
        CREATE TRIGGER update_salary_records_updated_at
            BEFORE UPDATE ON salary_records
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		// ========== 员工收款地址表 ==========
		`CREATE TABLE IF NOT EXISTS employee_payment_accounts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL UNIQUE REFERENCES employees(id) ON DELETE CASCADE,
            account_holder VARCHAR(255) NOT NULL DEFAULT '',
            bank_name VARCHAR(255) NOT NULL DEFAULT '',
            bank_account VARCHAR(128) NOT NULL DEFAULT '',
            usdt_network VARCHAR(32) NOT NULL DEFAULT '',
            usdt_address VARCHAR(255) NOT NULL DEFAULT '',
            other_wallet_type VARCHAR(64) NOT NULL DEFAULT '',
            other_wallet_address VARCHAR(255) NOT NULL DEFAULT '',
            preferred_method VARCHAR(16) NOT NULL DEFAULT '',
            remark TEXT NOT NULL DEFAULT '',
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_employee_payment_accounts_employee_id ON employee_payment_accounts(employee_id)`,
		`ALTER TABLE employee_payment_accounts ADD COLUMN IF NOT EXISTS preferred_method VARCHAR(16) NOT NULL DEFAULT ''`,
		`ALTER TABLE employee_payment_accounts ADD COLUMN IF NOT EXISTS notification_email VARCHAR(255) NOT NULL DEFAULT ''`,
		`DROP TRIGGER IF EXISTS update_employee_payment_accounts_updated_at ON employee_payment_accounts;
        CREATE TRIGGER update_employee_payment_accounts_updated_at
            BEFORE UPDATE ON employee_payment_accounts
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		`CREATE TABLE IF NOT EXISTS salary_payment_proofs (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            year_month VARCHAR(7) NOT NULL,
            file_path VARCHAR(512) NOT NULL,
            file_name VARCHAR(255) NOT NULL,
            content_type VARCHAR(128) NOT NULL DEFAULT 'image/jpeg',
            uploaded_by UUID REFERENCES users(id) ON DELETE SET NULL,
            uploaded_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE(employee_id, year_month)
        )`,
		`CREATE INDEX IF NOT EXISTS idx_salary_payment_proofs_employee_month ON salary_payment_proofs(employee_id, year_month)`,

		// ========== 用户通知（公告、工资发放等）==========
		`CREATE TABLE IF NOT EXISTS user_notifications (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            type VARCHAR(32) NOT NULL DEFAULT 'announcement',
            title VARCHAR(255) NOT NULL,
            content TEXT NOT NULL,
            payload JSONB,
            is_read BOOLEAN NOT NULL DEFAULT FALSE,
            show_popup BOOLEAN NOT NULL DEFAULT TRUE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_user_notifications_user_id ON user_notifications(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_notifications_unread ON user_notifications(user_id, is_read) WHERE is_read = FALSE`,

		// ========== 员工主数据 / 邀请码 / 工作 Session（Workforce）==========
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS department VARCHAR(100)`,
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS status SMALLINT NOT NULL DEFAULT 1`,
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS resigned_month VARCHAR(7)`,
		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_employees_user_id ON employees(user_id) WHERE user_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_employees_department ON employees(department)`,
		`CREATE INDEX IF NOT EXISTS idx_employees_status ON employees(status)`,

		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS position_snapshot VARCHAR(100)`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS checkin_time TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS checkout_time TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE attendance_records ADD COLUMN IF NOT EXISTS manually_edited BOOLEAN NOT NULL DEFAULT FALSE`,
		`UPDATE attendance_records SET status = 'work' WHERE status = 'working'`,

		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS employee_uuid UUID REFERENCES employees(id) ON DELETE SET NULL`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS employee_session_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_ip VARCHAR(64)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_employee_uuid ON devices(employee_uuid)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_one_employee ON devices(employee_uuid) WHERE employee_uuid IS NOT NULL AND status != 'rejected'`,

		`CREATE TABLE IF NOT EXISTS device_invite_codes (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            code VARCHAR(32) NOT NULL UNIQUE,
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            created_by UUID REFERENCES users(id) ON DELETE SET NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used_at TIMESTAMP WITH TIME ZONE,
            used_by_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
            revoked_at TIMESTAMP WITH TIME ZONE,
            superseded_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_invite_codes_employee ON device_invite_codes(employee_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invite_codes_code ON device_invite_codes(code)`,

		`CREATE TABLE IF NOT EXISTS work_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
            attendance_date DATE NOT NULL,
            device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
            checkin_time TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            checkout_time TIMESTAMP WITH TIME ZONE,
            status VARCHAR(20) NOT NULL DEFAULT 'open',
            recording_id UUID REFERENCES recordings(id) ON DELETE SET NULL,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_work_sessions_employee_date ON work_sessions(employee_id, attendance_date DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_work_sessions_open ON work_sessions(employee_id) WHERE status = 'open'`,

		`CREATE TABLE IF NOT EXISTS auth_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            subject_type VARCHAR(16) NOT NULL,
            subject_id UUID NOT NULL,
            status VARCHAR(16) NOT NULL DEFAULT 'active',
            version INT NOT NULL DEFAULT 1,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            revoked_at TIMESTAMP WITH TIME ZONE,
            revoke_reason VARCHAR(64),
            client_ip INET,
            user_agent TEXT,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_subject ON auth_sessions(subject_type, subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_status ON auth_sessions(status) WHERE status = 'active'`,

		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS force_offlined_at TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_use_default BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_allow_user_exit BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_allow_auto_start_toggle BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_restart_on_kill BOOLEAN`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS client_protection_temp_exit_until TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,
		`CREATE TABLE IF NOT EXISTS device_exit_unlock_codes (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            code_hash TEXT NOT NULL,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            used_at TIMESTAMP WITH TIME ZONE,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            created_by UUID REFERENCES users(id) ON DELETE SET NULL
        )`,
		`CREATE INDEX IF NOT EXISTS idx_device_exit_unlock_codes_device_active
            ON device_exit_unlock_codes(device_id, expires_at DESC)
            WHERE used_at IS NULL`,

		// 共享工位：允许多员工轮流使用同一终端（取消一人一机唯一约束）
		`DROP INDEX IF EXISTS idx_devices_one_employee`,

		`CREATE TABLE IF NOT EXISTS system_settings (
            key VARCHAR(128) PRIMARY KEY,
            value TEXT NOT NULL DEFAULT '',
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`CREATE TABLE IF NOT EXISTS auth_sessions (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            subject_type VARCHAR(16) NOT NULL,
            subject_id UUID NOT NULL,
            status VARCHAR(16) NOT NULL DEFAULT 'active',
            version INT NOT NULL DEFAULT 1,
            expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
            revoked_at TIMESTAMP WITH TIME ZONE,
            revoke_reason VARCHAR(64),
            client_ip INET,
            user_agent TEXT,
            created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_subject ON auth_sessions(subject_type, subject_id)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_status ON auth_sessions(status) WHERE status = 'active'`,

		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS salary_view_password_hash TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS screen_lock_password_hash TEXT`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			// 如果表已存在，忽略错误
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("failed to execute statement: %s\nError: %v", stmt, err)
			}
		}
	}
	return nil
}

func seedDefaultAdmin(db *sql.DB) error {
	// 插入/更新默认角色
	var adminRoleID string
	for _, role := range models.DefaultRoleSeeds {
		permissionsJSON, _ := json.Marshal(role.Permissions)

		var roleID string
		err := db.QueryRow(`
			INSERT INTO roles (id, name, description, permissions, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())
			ON CONFLICT (name) DO UPDATE SET
				description = EXCLUDED.description,
				updated_at = NOW()
			RETURNING id`,
			role.Name, role.Description, string(permissionsJSON)).Scan(&roleID)
		if err != nil {
			return err
		}
		if role.Name == "管理员" {
			adminRoleID = roleID
		}
	}

	if adminRoleID == "" {
		return fmt.Errorf("admin role not found")
	}

	for _, role := range models.DefaultRoleSeeds {
		if role.ExactSync {
			if err := syncRolePermissionsExact(db, role.Name, role.Permissions); err != nil {
				return err
			}
		} else if err := mergeRolePermissions(db, role.Name, role.Permissions); err != nil {
			return err
		}
	}

	if err := retireLegacyEmployeeRole(db); err != nil {
		return err
	}
	if err := grantAttendanceReviewPermsFromAttendance(db); err != nil {
		logPackage2.Printf("grant attendance_review perms: %v", err)
	}

	// 创建默认管理员用户（已存在时不覆盖密码）
	username := os.Getenv("ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD"))
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	isProduction := env == "production" || env == "prod"
	if password == "" {
		if isProduction {
			return fmt.Errorf("ADMIN_PASSWORD is required in production")
		}
		password = "admin123"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	var userID string
	err = db.QueryRow(`
		INSERT INTO users (id, username, password_hash, email, real_name, status, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 1, NOW())
		ON CONFLICT (username) DO NOTHING
		RETURNING id`,
		username, string(hash), "admin@example.com", "系统管理员").Scan(&userID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		err = db.QueryRow(`SELECT id FROM users WHERE username = $1`, username).Scan(&userID)
		if err != nil {
			return err
		}
	}

	// 关联用户和角色
	_, err = db.Exec(`
		INSERT INTO user_roles (user_id, role_id, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id, role_id) DO NOTHING`,
		userID, adminRoleID)

	// 初始化清理策略
	_, err = db.Exec(`
		INSERT INTO cleanup_policy (id, auto_cleanup_enabled, recording_retention_days, 
			screenshot_retention_days, log_retention_days, disk_usage_threshold_percent, 
			cleanup_interval_minutes, updated_at)
		VALUES (gen_random_uuid(), TRUE, 5, 5, 3, 70, 60, NOW())
		ON CONFLICT (id) DO NOTHING`)

	return err
}

// mergeRolePermissions 将缺失的权限合并进已有角色（不删除自定义权限）
func mergeRolePermissions(db *sql.DB, roleName string, required []string) error {
	var permissionsJSON string
	err := db.QueryRow(`SELECT permissions FROM roles WHERE name = $1`, roleName).Scan(&permissionsJSON)
	if err != nil {
		return err
	}

	var existing []string
	if permissionsJSON != "" && permissionsJSON != "null" {
		if err := json.Unmarshal([]byte(permissionsJSON), &existing); err != nil {
			return err
		}
	}

	seen := make(map[string]struct{}, len(existing)+len(required))
	for _, p := range existing {
		seen[p] = struct{}{}
	}
	merged := append([]string(nil), existing...)
	for _, p := range required {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		merged = append(merged, p)
	}

	if len(merged) == len(existing) {
		return nil
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE roles SET permissions = $1, updated_at = NOW() WHERE name = $2`, string(out), roleName)
	return err
}

// syncRolePermissionsExact 将角色权限对齐为指定列表（用于默认角色模板）
func syncRolePermissionsExact(db *sql.DB, roleName string, permissions []string) error {
	out, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE roles SET permissions = $1, updated_at = NOW() WHERE name = $2`, string(out), roleName)
	return err
}

// grantAttendanceReviewPermsFromAttendance 给已有 attendance 权限的角色补齐待确认权限（自定义角色不丢菜单）
func grantAttendanceReviewPermsFromAttendance(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, COALESCE(permissions::text, '[]') FROM roles`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, permissionsJSON string
		if err := rows.Scan(&id, &permissionsJSON); err != nil {
			return err
		}
		var existing []string
		if permissionsJSON != "" && permissionsJSON != "null" {
			if err := json.Unmarshal([]byte(permissionsJSON), &existing); err != nil {
				continue
			}
		}
		seen := make(map[string]struct{}, len(existing)+2)
		for _, p := range existing {
			seen[p] = struct{}{}
		}
		changed := false
		if _, ok := seen[models.PermAttendanceView]; ok {
			if _, has := seen[models.PermAttendanceReviewView]; !has {
				existing = append(existing, models.PermAttendanceReviewView)
				seen[models.PermAttendanceReviewView] = struct{}{}
				changed = true
			}
		}
		if _, ok := seen[models.PermAttendanceEdit]; ok {
			if _, has := seen[models.PermAttendanceReviewEdit]; !has {
				existing = append(existing, models.PermAttendanceReviewEdit)
				seen[models.PermAttendanceReviewEdit] = struct{}{}
				changed = true
			}
		}
		if !changed {
			continue
		}
		out, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		if _, err = db.Exec(`UPDATE roles SET permissions = $1, updated_at = NOW() WHERE id = $2`, string(out), id); err != nil {
			return err
		}
	}
	return rows.Err()
}

// retireLegacyEmployeeRole 迁移并删除已废弃的「员工」角色
func retireLegacyEmployeeRole(db *sql.DB) error {
	var employeeRoleID string
	err := db.QueryRow(`SELECT id FROM roles WHERE name = '员工'`).Scan(&employeeRoleID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	var viewerRoleID string
	if err := db.QueryRow(`SELECT id FROM roles WHERE name = '查看员'`).Scan(&viewerRoleID); err != nil {
		return err
	}

	if _, err = db.Exec(`
		INSERT INTO user_roles (user_id, role_id, created_at)
		SELECT ur.user_id, $1, NOW()
		FROM user_roles ur
		WHERE ur.role_id = $2
		ON CONFLICT (user_id, role_id) DO NOTHING`, viewerRoleID, employeeRoleID); err != nil {
		return err
	}
	if _, err = db.Exec(`DELETE FROM user_roles WHERE role_id = $1`, employeeRoleID); err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM roles WHERE id = $1`, employeeRoleID)
	return err
}

type MinIOClient struct {
	bucket    string
	enabled   bool
	rootDir   string
	endpoint  string
	accessKey string
	secretKey string
	useSSL    bool
	client    *minioPackage2.Client
}

func (m *MinIOClient) usesRemote() bool {
	return m != nil && m.client != nil
}
func (m *MinIOClient) cleanObjectName(objectName string) (string, error) {
	cleanObjectName := strings.TrimPrefix(filepath.ToSlash(objectName), "/")
	if cleanObjectName == "" {
		return "", fmt.Errorf("object name is required")
	}
	return cleanObjectName, nil
}

// RemoveObject 删除 MinIO 对象或本地镜像文件
func (m *MinIOClient) RemoveObject(ctx context.Context, objectName string) error {
	if m == nil || !m.enabled {
		return nil
	}

	cleanObjectName, err := m.cleanObjectName(objectName)
	if err != nil {
		return err
	}

	if m.usesRemote() {
		return m.client.RemoveObject(ctx, m.bucket, cleanObjectName, minioPackage2.RemoveObjectOptions{})
	}

	filePath := filepath.Join(m.rootDir, filepath.FromSlash(cleanObjectName))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

type DeviceRepository struct {
	db *sql.DB
}

func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// ✅ 修复：使用 sql.NullString 处理 NULL 字段
func (r *DeviceRepository) Get(ctx context.Context, id uuid.UUID) (*models.Device, error) {
	var d models.Device
	var boundAt, lastSeenAt sql.NullTime
	var employeeID, employeeName, employeeNumber, position sql.NullString

	query := `SELECT id, employee_id, employee_name, employee_number, position, device_fingerprint, hostname, os_version, agent_version, status, bound_at, last_seen_at, created_at, device_secret
              FROM devices WHERE id = $1`
	var deviceSecret sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&d.ID,
		&employeeID, &employeeName, &employeeNumber, &position,
		&d.DeviceFingerprint, &d.Hostname,
		&d.OSVersion, &d.AgentVersion,
		&d.Status, &boundAt, &lastSeenAt, &d.CreatedAt, &deviceSecret)
	if err == sql.ErrNoRows {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	// 处理 NULL 值
	if employeeID.Valid {
		d.EmployeeID = employeeID.String
	}
	if employeeName.Valid {
		d.EmployeeName = employeeName.String
	}
	if employeeNumber.Valid {
		d.EmployeeNumber = employeeNumber.String
	}
	if position.Valid {
		d.Position = position.String
	}
	if boundAt.Valid {
		d.BoundAt = &boundAt.Time
	}
	if lastSeenAt.Valid {
		d.LastSeenAt = &lastSeenAt.Time
	}
	if deviceSecret.Valid {
		d.DeviceSecret = deviceSecret.String
	}
	return &d, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// ---- Source: backend/services/session_config.go ----
func envInt(key string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// WSSessionGuardInterval returns how often WS connections revalidate session state.
func WSSessionGuardInterval() time.Duration {
	seconds := envInt("WS_SESSION_GUARD_INTERVAL_SECONDS", 10)
	if seconds < 3 {
		seconds = 3
	}
	if seconds > 120 {
		seconds = 120
	}
	return time.Duration(seconds) * time.Second
}

// ---- Source: backend/services/session_revoke_bus.go ----
const sessionRevokeChannel = "session:revoke:v1"

// SessionRevokeEvent is published when a session becomes invalid.
type SessionRevokeEvent struct {
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
}

func (s *SessionService) publishSessionRevoke(ctx context.Context, sessionID, reason string) {
	if s == nil || s.redis == nil || sessionID == "" {
		return
	}
	payload, err := json.Marshal(SessionRevokeEvent{
		SessionID: sessionID,
		Reason:    reason,
	})
	if err != nil {
		return
	}
	pubCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.redis.Publish(pubCtx, sessionRevokeChannel, payload).Err(); err != nil {
		logPackage2.Printf("[SessionRevoke] publish failed session=%s: %v", sessionID, err)
	}
}

// ---- Source: backend/services/session_service.go ----
const (
	SessionSubjectUser   = "user"
	SessionSubjectDevice = "device"

	SessionStatusActive  = "active"
	SessionStatusRevoked = "revoked"
	SessionStatusExpired = "expired"
)

var (
	ErrSessionNotFound    = errorsPackage2.New("session not found")
	ErrSessionRevoked     = errorsPackage2.New("session revoked")
	ErrSessionExpired     = errorsPackage2.New("session expired")
	ErrSessionUnavailable = errorsPackage2.New("session store unavailable")
)

type SessionService struct {
	db         *sql.DB
	redis      *redisPackage2.Client
	revokeHook func(SessionRevokeEvent)
}

func (s *SessionService) emitSessionRevoke(sessionID, reason string) {
	if sessionID == "" {
		return
	}
	event := SessionRevokeEvent{SessionID: sessionID, Reason: reason}
	if s.revokeHook != nil {
		s.revokeHook(event)
	}
}
func sessionCacheKey(id string) string      { return "sess:" + id }
func sessionRefreshKey(token string) string { return "sess:refresh:" + token }
func (s *SessionService) RevokeSessionsBySubject(ctx context.Context, subjectType string, subjectID uuid.UUID, reason string) error {
	if s.db == nil {
		return ErrSessionUnavailable
	}

	now := models.UTCNow()
	rows, err := s.db.QueryContext(ctx, `
		UPDATE auth_sessions
		SET status = $4, revoked_at = $5, revoke_reason = $6, updated_at = $5
		WHERE subject_type = $1 AND subject_id = $2 AND status = $3
		RETURNING id`,
		subjectType, subjectID, SessionStatusActive, SessionStatusRevoked, now, reason,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		s.invalidateSessionCache(ctx, id.String())
		s.writeSessionTombstone(ctx, id.String(), SessionStatusRevoked, reason)
		s.publishSessionRevoke(ctx, id.String(), reason)
		s.emitSessionRevoke(id.String(), reason)
	}
	return rows.Err()
}
func (s *SessionService) RevokeSubjectRefreshTokens(ctx context.Context, subjectType string, subjectID uuid.UUID, reason string) error {
	if err := s.RevokeSessionsBySubject(ctx, subjectType, subjectID, reason); err != nil {
		return err
	}
	if s.redis == nil {
		return nil
	}
	indexKey := sessionRefreshIndexKey(subjectType, subjectID.String())
	tokens, err := s.redis.SMembers(ctx, indexKey).Result()
	if err != nil && err != redisPackage2.Nil {
		return err
	}
	pipe := s.redis.Pipeline()
	for _, token := range tokens {
		pipe.Del(ctx, sessionRefreshKey(token))
	}
	pipe.Del(ctx, indexKey)
	_, err = pipe.Exec(ctx)
	return err
}
func sessionRefreshIndexKey(subjectType, subjectID string) string {
	return "sess:refresh_index:" + subjectType + ":" + subjectID
}
func (s *SessionService) invalidateSessionCache(ctx context.Context, sessionID string) {
	if s.redis == nil || sessionID == "" {
		return
	}
	_ = s.redis.Del(ctx, sessionCacheKey(sessionID)).Err()
}

// ---- Source: backend/services/session_tombstone.go ----
const sessionTombstoneTTL = 24 * time.Hour

type sessionTombstonePayload struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func sessionTombstoneKey(id string) string { return "sess:tomb:" + id }
func (s *SessionService) writeSessionTombstone(ctx context.Context, sessionID, status, reason string) {
	if s == nil || s.redis == nil || sessionID == "" {
		return
	}
	payload, err := json.Marshal(sessionTombstonePayload{
		Status: status,
		Reason: reason,
	})
	if err != nil {
		return
	}
	tombCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.redis.Set(tombCtx, sessionTombstoneKey(sessionID), payload, sessionTombstoneTTL).Err(); err != nil {
		logPackage2.Printf("[Session] tombstone write failed session=%s: %v", sessionID, err)
	}
}

// ---- Source: backend/services/token_crypto.go ----
func deriveAppSecretKey() []byte {
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN_SECRET"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("DM_SECRET_KEY"))
	}
	if raw == "" {
		return nil
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) >= 32 {
		return b[:32]
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
func EncryptSecret(plain string) (string, error) {
	key := deriveAppSecretKey()
	if len(key) != 32 {
		return "", errorsPackage2.New("secret key not configured (set JWT_SECRET or TELEGRAM_TOKEN_SECRET)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
func DecryptSecret(enc string) (string, error) {
	key := deriveAppSecretKey()
	if len(key) != 32 {
		return "", errorsPackage2.New("secret key not configured")
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid ciphertext")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
func TelegramAttendanceEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TELEGRAM_ATTENDANCE_ENABLED")))
	if v == "false" || v == "0" || v == "no" {
		return false
	}
	return true
}

// ---- Source: backend/services/upload_chunk_store.go ----
const uploadChunkRoot = "upload-chunks"

// UploadChunkStore persists upload parts in shared object storage (MinIO/S3 or UPLOADS_DIR mirror).
type UploadChunkStore struct {
	minio *MinIOClient
}

func (s *UploadChunkStore) chunkObjectName(uploadID string, index int) string {
	return filepath.ToSlash(filepath.Join(uploadChunkRoot, uploadID, fmt.Sprintf("chunk_%05d", index)))
}
func (s *UploadChunkStore) chunkPrefix(uploadID string) string {
	return filepath.ToSlash(filepath.Join(uploadChunkRoot, uploadID)) + "/"
}
func (s *UploadChunkStore) Open(ctx context.Context, uploadID string, index int) (io.ReadCloser, error) {
	if s == nil || s.minio == nil {
		return nil, fmt.Errorf("upload chunk store unavailable")
	}
	return s.minio.Open(ctx, s.chunkObjectName(uploadID, index))
}
func (s *UploadChunkStore) DeleteAll(ctx context.Context, uploadID string) error {
	if s == nil || s.minio == nil {
		return nil
	}
	return s.minio.RemovePrefix(ctx, s.chunkPrefix(uploadID))
}

type orderedChunkReader struct {
	store       *UploadChunkStore
	ctx         context.Context
	uploadID    string
	totalChunks int
	current     int
	cur         io.ReadCloser
	closed      bool
	readErr     error
}

func (r *orderedChunkReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.EOF
	}
	if r.readErr != nil {
		return 0, r.readErr
	}
	for {
		if r.cur == nil {
			if r.current >= r.totalChunks {
				return 0, io.EOF
			}
			rc, err := r.store.Open(r.ctx, r.uploadID, r.current)
			if err != nil {
				r.readErr = fmt.Errorf("open chunk %d: %w", r.current, err)
				return 0, r.readErr
			}
			r.cur = rc
		}
		n, err := r.cur.Read(p)
		if n > 0 {
			if err == io.EOF {
				_ = r.cur.Close()
				r.cur = nil
				r.current++
			}
			return n, nil
		}
		if err == io.EOF {
			_ = r.cur.Close()
			r.cur = nil
			r.current++
			continue
		}
		if err != nil {
			r.readErr = err
			return 0, err
		}
	}
}
func (r *orderedChunkReader) Close() error {
	r.closed = true
	if r.cur != nil {
		return r.cur.Close()
	}
	return nil
}

// Open returns a reader for an object in shared storage.
func (m *MinIOClient) Open(ctx context.Context, objectName string) (io.ReadCloser, error) {
	if m == nil || !m.enabled {
		return nil, fmt.Errorf("storage unavailable")
	}
	cleanObjectName, err := m.cleanObjectName(objectName)
	if err != nil {
		return nil, err
	}
	if m.usesRemote() {
		return m.client.GetObject(ctx, m.bucket, cleanObjectName, minioPackage2.GetObjectOptions{})
	}
	filePath := filepath.Join(m.rootDir, filepath.FromSlash(cleanObjectName))
	return os.Open(filePath)
}

// ListObjectNames lists object keys under prefix.
func (m *MinIOClient) ListObjectNames(ctx context.Context, prefix string) ([]string, error) {
	if m == nil || !m.enabled {
		return nil, fmt.Errorf("storage unavailable")
	}
	cleanPrefix, err := m.cleanObjectName(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return nil, err
	}
	cleanPrefix = strings.TrimSuffix(cleanPrefix, "/") + "/"

	if m.usesRemote() {
		var names []string
		for obj := range m.client.ListObjects(ctx, m.bucket, minioPackage2.ListObjectsOptions{
			Prefix:    cleanPrefix,
			Recursive: true,
		}) {
			if obj.Err != nil {
				return nil, obj.Err
			}
			names = append(names, obj.Key)
		}
		return names, nil
	}

	dir := filepath.Join(m.rootDir, filepath.FromSlash(cleanPrefix))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, filepath.ToSlash(filepath.Join(cleanPrefix, e.Name())))
	}
	return names, nil
}

// RemovePrefix deletes all objects under prefix.
func (m *MinIOClient) RemovePrefix(ctx context.Context, prefix string) error {
	if m == nil || !m.enabled {
		return nil
	}
	names, err := m.ListObjectNames(ctx, prefix)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := m.RemoveObject(ctx, name); err != nil {
			return err
		}
	}
	if m.usesRemote() {
		return nil
	}
	cleanPrefix, err := m.cleanObjectName(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return err
	}
	dir := filepath.Join(m.rootDir, filepath.FromSlash(strings.TrimSuffix(cleanPrefix, "/")))
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// UploadCompleteLock provides cross-instance mutual exclusion for upload completion.
type UploadCompleteLock struct {
	redis *redisPackage2.Client
}

func (l *UploadCompleteLock) release(ctx context.Context, key, token string) error {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0`
	return l.redis.Eval(ctx, script, []string{key}, token).Err()
}

// ---- Source: backend/services/upload_manifest.go ----
const (
	uploadManifestPrefix      = "upload:manifest:"
	uploadManifestPartsSuffix = ":parts"
)

// UploadManifestStore tracks chunk receipt in Redis and mirrors manifest.json to object storage.
type UploadManifestStore struct {
	redis      *redisPackage2.Client
	chunkStore *UploadChunkStore
}

func (s *UploadManifestStore) metaKey(uploadID string) string {
	return uploadManifestPrefix + uploadID
}
func (s *UploadManifestStore) partsKey(uploadID string) string {
	return uploadManifestPrefix + uploadID + uploadManifestPartsSuffix
}
func (s *UploadManifestStore) manifestObjectName(uploadID string) string {
	return filepath.ToSlash(filepath.Join(uploadChunkRoot, uploadID, "manifest.json"))
}

// Delete removes manifest keys and object storage manifest snapshot.
func (s *UploadManifestStore) Delete(ctx context.Context, uploadID string) error {
	if s == nil || s.redis == nil {
		return nil
	}
	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, s.metaKey(uploadID))
	pipe.Del(ctx, s.partsKey(uploadID))
	pipe.Del(ctx, s.metaKey(uploadID)+":checksums")
	pipe.Del(ctx, s.metaKey(uploadID)+":meta")
	_, err := pipe.Exec(ctx)
	if s.chunkStore != nil && s.chunkStore.minio != nil {
		_ = s.chunkStore.minio.RemoveObject(ctx, s.manifestObjectName(uploadID))
	}
	return err
}

// ---- Source: backend/services/user_repository.go ----
// UserRepository 用户仓库
// 主存储：PostgreSQL，内存作为一级缓存
type UserRepository struct {
	db    *sql.DB
	mu    sync.RWMutex
	users map[uuid.UUID]*models.User // 缓存
}

// NewUserRepository 创建用户仓库
func NewUserRepository(db *sql.DB) *UserRepository {
	repo := &UserRepository{
		db:    db,
		users: make(map[uuid.UUID]*models.User),
	}
	if db != nil {
		repo.loadUsers() // 加载缓存
	}
	return repo
}

// loadUsers 加载所有用户到缓存
func (r *UserRepository) loadUsers() {
	// PostgreSQL 兼容的用户+角色查询
	query := `
		SELECT 
			u.id, 
			u.username, 
			u.password_hash, 
			u.email, 
			u.real_name, 
			u.status, 
			COALESCE(u.totp_enabled, FALSE),
			u.created_at,
			u.last_login_at,
			COALESCE(
				(
					SELECT json_agg(
						json_build_object(
							'id', r.id,
							'name', r.name,
							'description', r.description,
							'permissions', r.permissions,
							'created_at', r.created_at,
							'updated_at', r.updated_at
						)
					)
					FROM user_roles ur
					JOIN roles r ON ur.role_id = r.id
					WHERE ur.user_id = u.id
				),
				'[]'::json
			) as roles
		FROM users u
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var user models.User
		var passwordHash string
		var createdAt time.Time
		var lastLogin sql.NullTime
		var rolesJSON string

		if err := rows.Scan(&user.ID, &user.Username, &passwordHash, &user.Email, &user.RealName, &user.Status, &user.TotpEnabled, &createdAt, &lastLogin, &rolesJSON); err != nil {
			continue
		}
		user.PasswordHash = passwordHash
		user.CreatedAt = createdAt
		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}

		// 解析用户关联的角色
		var roles []models.Role
		if err := json.Unmarshal([]byte(rolesJSON), &roles); err == nil {
			user.Roles = roles
		}

		r.users[user.ID] = &user
	}
}

// Get 根据ID获取用户
func (r *UserRepository) Get(ctx context.Context, id uuid.UUID) (*models.User, error) {
	// 先查缓存
	r.mu.RLock()
	if user, ok := r.users[id]; ok {
		userCopy := *user
		userCopy.PasswordHash = ""
		r.mu.RUnlock()
		return &userCopy, nil
	}
	r.mu.RUnlock()

	// 缓存未命中，查询数据库
	query := `
		SELECT id, username, password_hash, email, real_name, status,
		       COALESCE(totp_enabled, FALSE), created_at, last_login_at
		FROM users WHERE id = $1
	`
	var user models.User
	var passwordHash string
	var createdAt time.Time
	var lastLogin sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &passwordHash, &user.Email, &user.RealName, &user.Status,
		&user.TotpEnabled, &createdAt, &lastLogin)
	if err == sql.ErrNoRows {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	user.PasswordHash = passwordHash
	user.CreatedAt = createdAt

	// 获取角色
	roles, _ := r.GetUserRoles(ctx, user.ID)
	user.Roles = roles

	// 更新缓存
	r.mu.Lock()
	r.users[user.ID] = &user
	r.mu.Unlock()

	userCopy := user
	userCopy.PasswordHash = ""
	return &userCopy, nil
}

// Delete 删除用户
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// 不能删除管理员
	var username string
	err := r.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = $1`, id).Scan(&username)
	if err != nil {
		return err
	}
	if username == "admin" {
		return fmt.Errorf("cannot delete admin user")
	}

	// 使用事务
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 删除用户角色关联
	_, err = tx.Exec(`DELETE FROM user_roles WHERE user_id = $1`, id)
	if err != nil {
		return err
	}

	// 删除用户
	_, err = tx.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// 从缓存中删除
	r.mu.Lock()
	delete(r.users, id)
	r.mu.Unlock()

	return nil
}

// ChangePassword 修改用户密码（管理员操作）
func (r *UserRepository) ChangePassword(ctx context.Context, id uuid.UUID, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, string(hash), id)
	if err != nil {
		return err
	}

	// 更新缓存
	r.mu.Lock()
	if user, ok := r.users[id]; ok {
		user.PasswordHash = string(hash)
	}
	r.mu.Unlock()

	return nil
}

// DeleteRole 删除角色
func (r *UserRepository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	// 检查是否有用户关联此角色
	var userCount int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles WHERE role_id = $1`, id).Scan(&userCount)
	if err != nil {
		return err
	}
	if userCount > 0 {
		return fmt.Errorf("cannot delete role: it is assigned to %d user(s)", userCount)
	}

	// 先删除角色关联
	_, err = r.db.ExecContext(ctx, `DELETE FROM user_roles WHERE role_id = $1`, id)
	if err != nil {
		return err
	}

	// 删除角色
	_, err = r.db.ExecContext(ctx, `DELETE FROM roles WHERE id = $1`, id)
	return err
}

// GetUserRoles 获取用户的所有角色
func (r *UserRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	rolesByUser, err := r.GetRolesForUserIDs(ctx, []uuid.UUID{userID})
	if err != nil {
		return nil, err
	}
	return rolesByUser[userID], nil
}

// GetRolesForUserIDs loads roles for many users in one query.
func (r *UserRepository) GetRolesForUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]models.Role, error) {
	result := make(map[uuid.UUID][]models.Role, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT ur.user_id, r.id, r.name, r.description, r.permissions, r.created_at, r.updated_at
		FROM user_roles ur
		INNER JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id IN (%s)
		ORDER BY ur.user_id, r.name`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var userID uuid.UUID
		var role models.Role
		var permissionsJSON string
		if err := rows.Scan(&userID, &role.ID, &role.Name, &role.Description, &permissionsJSON, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(permissionsJSON), &role.Permissions); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], role)
	}
	return result, rows.Err()
}

var wsSessionRecheckInterval = 10 * time.Second

func init() {
	wsSessionRecheckInterval = WSSessionGuardInterval()
}
