// Consolidated source; original file boundaries are marked below.
package services

import (
	bytesPackage2 "bytes"
	context "context"
	aes "crypto/aes"
	cipher "crypto/cipher"
	rand "crypto/rand"
	sha256 "crypto/sha256"
	sql "database/sql"
	base64 "encoding/base64"
	json "encoding/json"
	errorsPackage2 "errors"
	fmt "fmt"
	uuid "github.com/google/uuid"
	bcrypt "golang.org/x/crypto/bcrypt"
	_ "golang.org/x/image/webp"
	html "html"
	_ "image/gif"
	io "io"
	logPackage2 "log"
	httpPackage2 "net/http"
	os "os"
	strings "strings"
	models "tg-attendance/backend/models"
	time "time"
)

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

		// 用户表
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE`,

		// 录屏表
		`ALTER TABLE recordings ADD COLUMN IF NOT EXISTS duration INTEGER DEFAULT 0`,
		`ALTER TABLE recordings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,

		// 索引（建议先添加列，后添加索引）
		`CREATE INDEX IF NOT EXISTS idx_recordings_uploaded ON recordings(uploaded)`,

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
		`DROP TRIGGER IF EXISTS update_salary_records_updated_at ON salary_records;
        CREATE TRIGGER update_salary_records_updated_at
            BEFORE UPDATE ON salary_records
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		// ========== 员工收款地址表 ==========

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

		// Shared login security settings (existing DB upgrade)

		`CREATE TABLE IF NOT EXISTS system_settings (
            key VARCHAR(128) PRIMARY KEY,
            value TEXT NOT NULL DEFAULT '',
            updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
        )`,

		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS salary_view_password_hash TEXT`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS screen_lock_password_hash TEXT`,

		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			// 记录警告但继续执行
			logPackage2.Printf("Migration warning (non-fatal): %v\nSQL: %s\n", err, migration)
		}
	}
	return nil
}

// ensureLoginSecuritySchema ensures the shared login settings used by standalone auth.
func ensureLoginSecuritySchema(db *sql.DB) error {
	stmts := []string{
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
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS shift_window_disabled BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE wf_telegram_settings ALTER COLUMN shift_window_disabled SET DEFAULT TRUE`,
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
		`ALTER TABLE wf_telegram_settings ADD COLUMN IF NOT EXISTS bot_ui_lang VARCHAR(8) NOT NULL DEFAULT 'zh'`,
		`ALTER TABLE wf_telegram_settings ALTER COLUMN bot_ui_lang SET DEFAULT 'zh'`,
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

		`DROP TRIGGER IF EXISTS update_salary_records_updated_at ON salary_records;
        CREATE TRIGGER update_salary_records_updated_at
            BEFORE UPDATE ON salary_records
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column()`,

		// ========== 员工收款地址表 ==========

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

		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE`,

		// 共享工位：允许多员工轮流使用同一终端（取消一人一机唯一约束）
		`DROP INDEX IF EXISTS idx_devices_one_employee`,

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

	return nil
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

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
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
