// 本文件用于规范化云资源索引持久化。
// 文件职责：把云厂商快照中的 ECS/RDS 资源展开到 cloud_resources 表，支撑筛选、风险和 K8s 关联。

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	appcloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/app/cloud"

	_ "modernc.org/sqlite"
)

type ResourceIndexStore struct {
	mu     sync.Mutex
	db     *sql.DB
	dbPath string
}

func NewResourceIndexStore(dbPath string) (*ResourceIndexStore, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return nil, fmt.Errorf("资源索引数据库路径不能为空")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open resource index sqlite failed: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set resource index sqlite wal failed: %w", err)
	}
	if err := migrateResourceIndexStore(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &ResourceIndexStore{db: db, dbPath: dbPath}, nil
}

func (s *ResourceIndexStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *ResourceIndexStore) UpsertResources(resources []appcloud.Resource) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("资源索引存储未初始化")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO cloud_resources (
			id, account_id, account_name, provider, resource_type, resource_id, name, region, zone, status,
			private_ips_json, public_ips_json, charge_type, expired_at, expires_in_days, expiration_status,
			expiration_message, engine, engine_version, node_id, source, snapshot_at, last_error, labels_json, raw_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			account_id = excluded.account_id,
			account_name = excluded.account_name,
			provider = excluded.provider,
			resource_type = excluded.resource_type,
			resource_id = excluded.resource_id,
			name = excluded.name,
			region = excluded.region,
			zone = excluded.zone,
			status = excluded.status,
			private_ips_json = excluded.private_ips_json,
			public_ips_json = excluded.public_ips_json,
			charge_type = excluded.charge_type,
			expired_at = excluded.expired_at,
			expires_in_days = excluded.expires_in_days,
			expiration_status = excluded.expiration_status,
			expiration_message = excluded.expiration_message,
			engine = excluded.engine,
			engine_version = excluded.engine_version,
			node_id = excluded.node_id,
			source = excluded.source,
			snapshot_at = excluded.snapshot_at,
			last_error = excluded.last_error,
			labels_json = excluded.labels_json,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, resource := range resources {
		expiresInDays := sql.NullInt64{}
		if resource.ExpiresInDays != nil {
			expiresInDays.Valid = true
			expiresInDays.Int64 = int64(*resource.ExpiresInDays)
		}
		if _, err := stmt.Exec(
			resource.ID,
			resource.AccountID,
			resource.AccountName,
			resource.Provider,
			resource.ResourceType,
			resource.ResourceID,
			resource.Name,
			resource.Region,
			resource.Zone,
			resource.Status,
			mustJSON(resource.PrivateIPs),
			mustJSON(resource.PublicIPs),
			resource.ChargeType,
			resource.ExpiredAt,
			expiresInDays,
			resource.ExpirationStatus,
			resource.ExpirationMessage,
			resource.Engine,
			resource.EngineVersion,
			resource.NodeID,
			resource.Source,
			resource.SnapshotAt,
			resource.LastError,
			mustJSON(resource.Labels),
			resource.RawJSON,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ResourceIndexStore) ListResources(filter appcloud.ResourceFilter) ([]appcloud.Resource, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("资源索引存储未初始化")
	}
	query := `
		SELECT id, account_id, account_name, provider, resource_type, resource_id, name, region, zone, status,
			private_ips_json, public_ips_json, charge_type, expired_at, expires_in_days, expiration_status,
			expiration_message, engine, engine_version, node_id, source, snapshot_at, last_error, labels_json, raw_json
		FROM cloud_resources
		WHERE 1=1`
	args := make([]any, 0, 4)
	if strings.TrimSpace(filter.Provider) != "" {
		query += " AND provider = ?"
		args = append(args, strings.TrimSpace(filter.Provider))
	}
	if strings.TrimSpace(filter.ResourceType) != "" {
		query += " AND resource_type = ?"
		args = append(args, strings.TrimSpace(filter.ResourceType))
	}
	if filter.AccountID > 0 {
		query += " AND account_id = ?"
		args = append(args, filter.AccountID)
	}
	if strings.TrimSpace(filter.Region) != "" {
		query += " AND region = ?"
		args = append(args, strings.TrimSpace(filter.Region))
	}
	query += " ORDER BY provider, account_id, resource_type, region, name, resource_id"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]appcloud.Resource, 0)
	for rows.Next() {
		item, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *ResourceIndexStore) GetResource(id string) (*appcloud.Resource, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("资源索引存储未初始化")
	}
	row := s.db.QueryRow(`
		SELECT id, account_id, account_name, provider, resource_type, resource_id, name, region, zone, status,
			private_ips_json, public_ips_json, charge_type, expired_at, expires_in_days, expiration_status,
			expiration_message, engine, engine_version, node_id, source, snapshot_at, last_error, labels_json, raw_json
		FROM cloud_resources
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanResource(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("资源不存在")
		}
		return nil, err
	}
	return &item, nil
}

type resourceScanner interface {
	Scan(dest ...any) error
}

func scanResource(row resourceScanner) (appcloud.Resource, error) {
	var item appcloud.Resource
	var privateIPsJSON, publicIPsJSON, labelsJSON string
	var expiresInDays sql.NullInt64
	err := row.Scan(
		&item.ID,
		&item.AccountID,
		&item.AccountName,
		&item.Provider,
		&item.ResourceType,
		&item.ResourceID,
		&item.Name,
		&item.Region,
		&item.Zone,
		&item.Status,
		&privateIPsJSON,
		&publicIPsJSON,
		&item.ChargeType,
		&item.ExpiredAt,
		&expiresInDays,
		&item.ExpirationStatus,
		&item.ExpirationMessage,
		&item.Engine,
		&item.EngineVersion,
		&item.NodeID,
		&item.Source,
		&item.SnapshotAt,
		&item.LastError,
		&labelsJSON,
		&item.RawJSON,
	)
	if err != nil {
		return item, err
	}
	if expiresInDays.Valid {
		value := int(expiresInDays.Int64)
		item.ExpiresInDays = &value
	}
	_ = json.Unmarshal([]byte(privateIPsJSON), &item.PrivateIPs)
	_ = json.Unmarshal([]byte(publicIPsJSON), &item.PublicIPs)
	_ = json.Unmarshal([]byte(labelsJSON), &item.Labels)
	return item, nil
}

func migrateResourceIndexStore(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cloud_resources (
			id TEXT PRIMARY KEY,
			account_id INTEGER NOT NULL,
			account_name TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			region TEXT NOT NULL DEFAULT '',
			zone TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			private_ips_json TEXT NOT NULL DEFAULT '[]',
			public_ips_json TEXT NOT NULL DEFAULT '[]',
			charge_type TEXT NOT NULL DEFAULT '',
			expired_at TEXT NOT NULL DEFAULT '',
			expires_in_days INTEGER,
			expiration_status TEXT NOT NULL DEFAULT '',
			expiration_message TEXT NOT NULL DEFAULT '',
			engine TEXT NOT NULL DEFAULT '',
			engine_version TEXT NOT NULL DEFAULT '',
			node_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			snapshot_at TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			labels_json TEXT NOT NULL DEFAULT '{}',
			raw_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_cloud_resources_filter
			ON cloud_resources(provider, resource_type, account_id, region);
		CREATE INDEX IF NOT EXISTS idx_cloud_resources_resource_id
			ON cloud_resources(resource_type, resource_id);
	`)
	if err != nil {
		return fmt.Errorf("migrate resource index sqlite failed: %w", err)
	}
	return nil
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
