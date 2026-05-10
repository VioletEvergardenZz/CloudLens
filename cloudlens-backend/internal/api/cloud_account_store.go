// 本文件用于云账号本地持久化。
// 文件职责：保存阿里云 ECS 只读账号配置，并对 AccessKey Secret 做本机加密落盘。
// 边界与容错：只保存控制台录入的查询凭据，不负责云资源缓存与采集调度。

package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	aliyuncloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/aliyun"

	_ "modernc.org/sqlite"
)

const (
	defaultCloudDataDir = "data/cloud"
	cloudTimeLayout     = time.RFC3339Nano
)

type cloudAccountStore struct {
	mu     sync.Mutex
	db     *sql.DB
	dbPath string
	codec  *cloudSecretCodec
}

type cloudAccountRecord struct {
	ID                    int64     `json:"id"`
	Provider              string    `json:"provider"`
	Name                  string    `json:"name"`
	AccessKeyID           string    `json:"accessKeyId,omitempty"`
	AccessKeyIDMasked     string    `json:"accessKeyIdMasked"`
	AccessKeySecretCipher string    `json:"-"`
	Regions               []string  `json:"regions"`
	MetricPeriod          string    `json:"metricPeriod"`
	Enabled               bool      `json:"enabled"`
	LastCheckStatus       string    `json:"lastCheckStatus"`
	LastCheckMessage      string    `json:"lastCheckMessage"`
	LastCheckedAt         string    `json:"lastCheckedAt"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type cloudAccountUpsert struct {
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	AccessKeyID     string   `json:"accessKeyId"`
	AccessKeySecret string   `json:"accessKeySecret"`
	Regions         []string `json:"regions"`
	MetricPeriod    string   `json:"metricPeriod"`
	Enabled         *bool    `json:"enabled"`
}

type cloudSecretCodec struct {
	aead cipher.AEAD
}

func newCloudAccountStore(dataDir string) (*cloudAccountStore, error) {
	root := resolveCloudDataDir(dataDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create cloud data dir failed: %w", err)
	}
	codec, err := newCloudSecretCodec(filepath.Join(root, "secret.key"))
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(root, "cloud.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open cloud sqlite failed: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set cloud sqlite wal failed: %w", err)
	}
	if err := migrateCloudAccountStore(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &cloudAccountStore{db: db, dbPath: dbPath, codec: codec}, nil
}

func (s *cloudAccountStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *cloudAccountStore) DBPath() string {
	if s == nil {
		return ""
	}
	return s.dbPath
}

func (s *cloudAccountStore) List() ([]cloudAccountRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, provider, name, access_key_id, access_key_secret_cipher, regions_json,
			metric_period, enabled, last_check_status, last_check_message, last_checked_at, created_at, updated_at
		FROM cloud_accounts
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]cloudAccountRecord, 0)
	for rows.Next() {
		item, err := scanCloudAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *cloudAccountStore) Get(id int64) (*cloudAccountRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("云账号存储未初始化")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(`
		SELECT id, provider, name, access_key_id, access_key_secret_cipher, regions_json,
			metric_period, enabled, last_check_status, last_check_message, last_checked_at, created_at, updated_at
		FROM cloud_accounts
		WHERE id = ?
	`, id)
	item, err := scanCloudAccount(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("云账号不存在")
		}
		return nil, err
	}
	return &item, nil
}

func (s *cloudAccountStore) Create(input cloudAccountUpsert) (*cloudAccountRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("云账号存储未初始化")
	}
	if err := validateCloudAccountInput(input, true); err != nil {
		return nil, err
	}
	secretCipher, err := s.codec.Encrypt(input.AccessKeySecret)
	if err != nil {
		return nil, err
	}
	regionsJSON := mustCloudJSON(normalizeCloudRegions(input.Regions))
	now := time.Now().UTC()
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	s.mu.Lock()
	result, err := s.db.Exec(`
		INSERT INTO cloud_accounts (
			provider, name, access_key_id, access_key_secret_cipher, regions_json,
			metric_period, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"aliyun",
		strings.TrimSpace(input.Name),
		strings.TrimSpace(input.AccessKeyID),
		secretCipher,
		regionsJSON,
		normalizeMetricPeriod(input.MetricPeriod),
		boolToInt(enabled),
		formatCloudTime(now),
		formatCloudTime(now),
	)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *cloudAccountStore) Update(id int64, input cloudAccountUpsert) (*cloudAccountRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("云账号存储未初始化")
	}
	if id <= 0 {
		return nil, fmt.Errorf("云账号 ID 不合法")
	}
	if err := validateCloudAccountInput(input, false); err != nil {
		return nil, err
	}
	current, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	secretCipher := current.AccessKeySecretCipher
	if strings.TrimSpace(input.AccessKeySecret) != "" {
		secretCipher, err = s.codec.Encrypt(input.AccessKeySecret)
		if err != nil {
			return nil, err
		}
	}
	enabled := current.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	regions := normalizeCloudRegions(input.Regions)
	if len(regions) == 0 {
		regions = current.Regions
	}
	accessKeyID := strings.TrimSpace(input.AccessKeyID)
	if accessKeyID == "" {
		accessKeyID = current.AccessKeyID
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = current.Name
	}
	metricPeriod := normalizeMetricPeriod(input.MetricPeriod)
	if metricPeriod == "" {
		metricPeriod = current.MetricPeriod
	}

	now := time.Now().UTC()
	s.mu.Lock()
	_, err = s.db.Exec(`
		UPDATE cloud_accounts
		SET name = ?, access_key_id = ?, access_key_secret_cipher = ?, regions_json = ?,
			metric_period = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`,
		name,
		accessKeyID,
		secretCipher,
		mustCloudJSON(regions),
		metricPeriod,
		boolToInt(enabled),
		formatCloudTime(now),
		id,
	)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *cloudAccountStore) Delete(id int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("云账号存储未初始化")
	}
	if id <= 0 {
		return fmt.Errorf("云账号 ID 不合法")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM cloud_accounts WHERE id = ?`, id)
	return err
}

func (s *cloudAccountStore) UpdateCheck(id int64, status, message string, checkedAt time.Time) error {
	if s == nil || s.db == nil || id <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		UPDATE cloud_accounts
		SET last_check_status = ?, last_check_message = ?, last_checked_at = ?, updated_at = ?
		WHERE id = ?
	`,
		strings.TrimSpace(status),
		strings.TrimSpace(message),
		formatCloudTime(checkedAt),
		formatCloudTime(time.Now().UTC()),
		id,
	)
	return err
}

func (s *cloudAccountStore) AliyunConfig(id int64) (aliyuncloud.Config, *cloudAccountRecord, error) {
	account, err := s.Get(id)
	if err != nil {
		return aliyuncloud.Config{}, nil, err
	}
	if !account.Enabled {
		return aliyuncloud.Config{}, nil, fmt.Errorf("云账号已停用")
	}
	secret, err := s.codec.Decrypt(account.AccessKeySecretCipher)
	if err != nil {
		return aliyuncloud.Config{}, nil, fmt.Errorf("解密云账号密钥失败: %w", err)
	}
	return aliyuncloud.Config{
		AccessKeyID:     account.AccessKeyID,
		AccessKeySecret: secret,
		Regions:         account.Regions,
		MetricPeriod:    account.MetricPeriod,
	}, account, nil
}

func newCloudSecretCodec(keyPath string) (*cloudSecretCodec, error) {
	key, err := loadOrCreateCloudSecretKey(keyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cloud secret cipher failed: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create cloud secret gcm failed: %w", err)
	}
	return &cloudSecretCodec{aead: aead}, nil
}

func (c *cloudSecretCodec) Encrypt(plain string) (string, error) {
	if c == nil || c.aead == nil {
		return "", fmt.Errorf("云账号密钥加密器未初始化")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("create cloud secret nonce failed: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, []byte(strings.TrimSpace(plain)), nil)
	raw := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (c *cloudSecretCodec) Decrypt(cipherText string) (string, error) {
	if c == nil || c.aead == nil {
		return "", fmt.Errorf("云账号密钥加密器未初始化")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cipherText))
	if err != nil {
		return "", err
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) <= nonceSize {
		return "", fmt.Errorf("密文长度不合法")
	}
	plain, err := c.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func loadOrCreateCloudSecretKey(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		decoded, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("decode cloud secret key failed: %w", err)
		}
		if len(decoded) != 32 {
			return nil, fmt.Errorf("cloud secret key length invalid")
		}
		return decoded, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read cloud secret key failed: %w", err)
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("create cloud secret key failed: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, fmt.Errorf("write cloud secret key failed: %w", err)
	}
	return key, nil
}

type cloudAccountScanner interface {
	Scan(dest ...any) error
}

func scanCloudAccount(row cloudAccountScanner) (cloudAccountRecord, error) {
	var (
		item         cloudAccountRecord
		regionsJSON  string
		enabledValue int
		createdAt    string
		updatedAt    string
	)
	if err := row.Scan(
		&item.ID,
		&item.Provider,
		&item.Name,
		&item.AccessKeyID,
		&item.AccessKeySecretCipher,
		&regionsJSON,
		&item.MetricPeriod,
		&enabledValue,
		&item.LastCheckStatus,
		&item.LastCheckMessage,
		&item.LastCheckedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return item, err
	}
	item.Regions = parseCloudStringList(regionsJSON)
	item.Enabled = enabledValue != 0
	item.AccessKeyIDMasked = maskAccessKeyID(item.AccessKeyID)
	item.CreatedAt = parseCloudTime(createdAt)
	item.UpdatedAt = parseCloudTime(updatedAt)
	return item, nil
}

func migrateCloudAccountStore(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("cloud sqlite is nil")
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cloud_accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL DEFAULT 'aliyun',
			name TEXT NOT NULL,
			access_key_id TEXT NOT NULL,
			access_key_secret_cipher TEXT NOT NULL,
			regions_json TEXT NOT NULL DEFAULT '[]',
			metric_period TEXT NOT NULL DEFAULT '60',
			enabled INTEGER NOT NULL DEFAULT 1,
			last_check_status TEXT NOT NULL DEFAULT '',
			last_check_message TEXT NOT NULL DEFAULT '',
			last_checked_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_cloud_accounts_provider_enabled
			ON cloud_accounts(provider, enabled, updated_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate cloud sqlite failed: %w", err)
		}
	}
	return nil
}

func validateCloudAccountInput(input cloudAccountUpsert, secretRequired bool) error {
	if provider := strings.TrimSpace(input.Provider); provider != "" && provider != "aliyun" {
		return fmt.Errorf("当前仅支持阿里云 ECS 账号")
	}
	if secretRequired {
		if strings.TrimSpace(input.Name) == "" {
			return fmt.Errorf("账号名称不能为空")
		}
		if strings.TrimSpace(input.AccessKeyID) == "" {
			return fmt.Errorf("AccessKey ID 不能为空")
		}
		if strings.TrimSpace(input.AccessKeySecret) == "" {
			return fmt.Errorf("AccessKey Secret 不能为空")
		}
		if len(normalizeCloudRegions(input.Regions)) == 0 {
			return fmt.Errorf("至少填写一个地域，例如 cn-hangzhou")
		}
	}
	return nil
}

func resolveCloudDataDir(raw string) string {
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	if env := strings.TrimSpace(os.Getenv("CLOUD_DATA_DIR")); env != "" {
		return env
	}
	return defaultCloudDataDir
}

func normalizeCloudRegions(regions []string) []string {
	out := make([]string, 0, len(regions))
	seen := make(map[string]struct{}, len(regions))
	for _, region := range regions {
		for _, field := range strings.FieldsFunc(region, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			value := strings.TrimSpace(field)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func normalizeMetricPeriod(period string) string {
	trimmed := strings.TrimSpace(period)
	if trimmed == "" {
		return "60"
	}
	return trimmed
}

func parseCloudStringList(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil
	}
	return normalizeCloudRegions(out)
}

func mustCloudJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func maskAccessKeyID(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:4] + "****" + trimmed[len(trimmed)-4:]
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatCloudTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(cloudTimeLayout)
}

func parseCloudTime(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}
	}
	if t, err := time.Parse(cloudTimeLayout, trimmed); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
