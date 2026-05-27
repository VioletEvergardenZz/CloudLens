// 本文件用于云账号存储的单元测试。
// 文件职责：覆盖多云 provider 保存、读取和切换校验。

package api

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCloudAccountStoreSupportsHuaweiProvider(t *testing.T) {
	store, err := newCloudAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("初始化云账号存储失败: %v", err)
	}
	defer store.Close()
	projectID := "project-cn-north-4"

	account, err := store.Create(cloudAccountUpsert{
		Provider:        "huawei",
		Name:            "华为云测试账号",
		AccessKeyID:     "huawei-ak",
		AccessKeySecret: "huawei-secret",
		ProjectID:       &projectID,
		MetricPeriod:    "60",
	})
	if err != nil {
		t.Fatalf("创建华为云账号失败: %v", err)
	}
	if account.Provider != "huawei" {
		t.Fatalf("Provider 期望 huawei，实际 %s", account.Provider)
	}

	cfg, saved, err := store.HuaweiConfig(account.ID)
	if err != nil {
		t.Fatalf("读取华为云配置失败: %v", err)
	}
	if saved.Provider != "huawei" {
		t.Fatalf("保存账号 Provider 期望 huawei，实际 %s", saved.Provider)
	}
	if cfg.AccessKeyID != "huawei-ak" || cfg.AccessKeySecret != "huawei-secret" {
		t.Fatalf("华为云 AK/SK 解密结果不符合预期: %#v", cfg)
	}
	if cfg.ProjectID != "project-cn-north-4" {
		t.Fatalf("华为云 ProjectID 期望 project-cn-north-4，实际 %s", cfg.ProjectID)
	}
	if len(cfg.Regions) != 0 {
		t.Fatalf("云账号运行配置不应再限定地域，实际 %#v", cfg.Regions)
	}
	if cfg.MetricPeriod != "60" {
		t.Fatalf("采样周期期望 60，实际 %s", cfg.MetricPeriod)
	}
	if _, _, err := store.AliyunConfig(account.ID); err == nil {
		t.Fatal("华为云账号不应该能作为阿里云配置读取")
	}
}

func TestCloudAccountStoreSwitchProviderRequiresCredentials(t *testing.T) {
	store, err := newCloudAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("初始化云账号存储失败: %v", err)
	}
	defer store.Close()
	projectID := "project-cn-north-4"

	account, err := store.Create(cloudAccountUpsert{
		Provider:        "aliyun",
		Name:            "阿里云测试账号",
		AccessKeyID:     "aliyun-ak",
		AccessKeySecret: "aliyun-secret",
		MetricPeriod:    "60",
	})
	if err != nil {
		t.Fatalf("创建阿里云账号失败: %v", err)
	}

	if _, err := store.Update(account.ID, cloudAccountUpsert{Provider: "huawei"}); err == nil || !strings.Contains(err.Error(), "重新填写 AccessKey") {
		t.Fatalf("切换云平台缺少凭据时应返回明确错误，实际: %v", err)
	}

	updated, err := store.Update(account.ID, cloudAccountUpsert{
		Provider:        "huawei",
		AccessKeyID:     "huawei-ak",
		AccessKeySecret: "huawei-secret",
		ProjectID:       &projectID,
	})
	if err != nil {
		t.Fatalf("带凭据切换到华为云失败: %v", err)
	}
	if updated.Provider != "huawei" {
		t.Fatalf("Provider 期望 huawei，实际 %s", updated.Provider)
	}

	cfg, _, err := store.HuaweiConfig(account.ID)
	if err != nil {
		t.Fatalf("切换后读取华为云配置失败: %v", err)
	}
	if cfg.AccessKeyID != "huawei-ak" || cfg.AccessKeySecret != "huawei-secret" {
		t.Fatalf("切换后的华为云 AK/SK 不符合预期: %#v", cfg)
	}
	if cfg.ProjectID != "project-cn-north-4" {
		t.Fatalf("切换后的华为云 ProjectID 不符合预期: %s", cfg.ProjectID)
	}
}

func TestCloudAccountStoreUpdateCanClearHuaweiProjectID(t *testing.T) {
	store, err := newCloudAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("初始化云账号存储失败: %v", err)
	}
	defer store.Close()

	projectID := "project-cn-south-1"
	account, err := store.Create(cloudAccountUpsert{
		Provider:        "huawei",
		Name:            "华为云测试账号",
		AccessKeyID:     "huawei-ak",
		AccessKeySecret: "huawei-secret",
		ProjectID:       &projectID,
		MetricPeriod:    "60",
	})
	if err != nil {
		t.Fatalf("创建华为云账号失败: %v", err)
	}

	emptyProjectID := ""
	updated, err := store.Update(account.ID, cloudAccountUpsert{
		Provider:  "huawei",
		ProjectID: &emptyProjectID,
	})
	if err != nil {
		t.Fatalf("清空 ProjectID 失败: %v", err)
	}
	if updated.ProjectID != "" {
		t.Fatalf("ProjectID 应允许清空，实际: %s", updated.ProjectID)
	}

	cfg, _, err := store.HuaweiConfig(account.ID)
	if err != nil {
		t.Fatalf("读取华为云配置失败: %v", err)
	}
	if cfg.ProjectID != "" {
		t.Fatalf("清空后配置中的 ProjectID 应为空，实际: %s", cfg.ProjectID)
	}
}

func TestLoadOrCreateCloudSecretKeyTightensExistingFilePerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不完整支持 POSIX 权限位，密钥权限收紧在运行检查中按平台提示")
	}
	keyPath := filepath.Join(t.TempDir(), "secret.key")
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)), 0o644); err != nil {
		t.Fatalf("写入测试密钥失败: %v", err)
	}

	loaded, err := loadOrCreateCloudSecretKey(keyPath)
	if err != nil {
		t.Fatalf("读取测试密钥失败: %v", err)
	}
	if hex.EncodeToString(loaded) != hex.EncodeToString(key) {
		t.Fatalf("读取到的密钥不符合预期")
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("读取测试密钥权限失败: %v", err)
	}
	if info.Mode().Perm() != cloudSecretKeyPerm {
		t.Fatalf("历史密钥文件权限应收紧为 %o，实际 %o", cloudSecretKeyPerm, info.Mode().Perm())
	}
}

func TestCloudAccountStoreResourceSnapshotLifecycle(t *testing.T) {
	store, err := newCloudAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("初始化云账号存储失败: %v", err)
	}
	defer store.Close()

	account, err := store.Create(cloudAccountUpsert{
		Provider:        "aliyun",
		Name:            "阿里云测试账号",
		AccessKeyID:     "aliyun-ak",
		AccessKeySecret: "aliyun-secret",
		MetricPeriod:    "60",
	})
	if err != nil {
		t.Fatalf("创建阿里云账号失败: %v", err)
	}

	items := []map[string]any{
		{"id": "i-001", "name": "web-1", "status": "Running"},
		{"id": "i-002", "name": "web-2", "status": "Stopped"},
	}
	if err := store.SaveResourceSnapshot(account.ID, account.Provider, "ecs", items, "live"); err != nil {
		t.Fatalf("保存资源快照失败: %v", err)
	}
	snapshot, err := store.GetResourceSnapshot(account.ID, "instances")
	if err != nil {
		t.Fatalf("读取资源快照失败: %v", err)
	}
	if snapshot.Total != len(items) {
		t.Fatalf("资源快照数量期望 %d，实际 %d", len(items), snapshot.Total)
	}
	var saved []map[string]any
	if err := json.Unmarshal(snapshot.PayloadJSON, &saved); err != nil {
		t.Fatalf("快照 JSON 无法解析: %v", err)
	}
	if len(saved) != len(items) {
		t.Fatalf("快照 JSON 数量期望 %d，实际 %d", len(items), len(saved))
	}

	if err := store.RecordResourceSnapshotError(account.ID, "ecs", "云 API 临时失败"); err != nil {
		t.Fatalf("记录资源快照错误失败: %v", err)
	}
	snapshot, err = store.GetResourceSnapshot(account.ID, "ecs")
	if err != nil {
		t.Fatalf("重新读取资源快照失败: %v", err)
	}
	if snapshot.LastError != "云 API 临时失败" {
		t.Fatalf("LastError 未按预期保存，实际: %s", snapshot.LastError)
	}

	if err := store.Delete(account.ID); err != nil {
		t.Fatalf("删除账号失败: %v", err)
	}
	if _, err := store.GetResourceSnapshot(account.ID, "ecs"); err == nil {
		t.Fatal("删除账号后不应继续读取到资源快照")
	}
}

func TestBuildCloudRisksFromSnapshots(t *testing.T) {
	store, err := newCloudAccountStore(t.TempDir())
	if err != nil {
		t.Fatalf("初始化云账号存储失败: %v", err)
	}
	defer store.Close()

	account, err := store.Create(cloudAccountUpsert{
		Provider:        "aliyun",
		Name:            "阿里云测试账号",
		AccessKeyID:     "aliyun-ak",
		AccessKeySecret: "aliyun-secret",
		MetricPeriod:    "60",
	})
	if err != nil {
		t.Fatalf("创建阿里云账号失败: %v", err)
	}

	storageUsage := 91.5
	ecsExpiresInDays := 3
	if err := store.SaveResourceSnapshot(account.ID, account.Provider, "ecs", []cloudSnapshotResource{{
		ID:                "i-public",
		Name:              "公网 ECS",
		Provider:          "aliyun",
		RegionID:          "cn-hangzhou",
		Status:            "Running",
		PublicIPs:         []string{"203.0.113.10"},
		ExpiresInDays:     &ecsExpiresInDays,
		ExpirationStatus:  "expiring",
		ExpirationMessage: "3 天后到期",
	}}, "live"); err != nil {
		t.Fatalf("保存 ECS 快照失败: %v", err)
	}
	if err := store.SaveResourceSnapshot(account.ID, account.Provider, "rds", []cloudSnapshotResource{{
		ID:               "rm-critical",
		Name:             "高水位 RDS",
		Provider:         "aliyun",
		RegionID:         "cn-hangzhou",
		Status:           "Running",
		ResourceUsage:    &cloudSnapshotUsage{StorageUsagePercent: &storageUsage},
		ConnectionString: "rm-public.mysql.rds.aliyuncs.com",
	}}, "live"); err != nil {
		t.Fatalf("保存 RDS 快照失败: %v", err)
	}

	handler := &handler{cloudStore: store}
	risks, summary, err := handler.buildCloudRisks()
	if err != nil {
		t.Fatalf("构建风险失败: %v", err)
	}
	if summary.Critical == 0 {
		t.Fatalf("期望至少识别一个 critical 风险，实际摘要: %#v", summary)
	}
	if !hasRiskCategory(risks, "expiration") || !hasRiskCategory(risks, "public_exposure") || !hasRiskCategory(risks, "rds_storage") {
		t.Fatalf("风险分类缺失，实际: %#v", risks)
	}

	old := time.Now().UTC().Add(-7 * time.Hour)
	if _, err := store.db.Exec(`UPDATE cloud_resource_snapshots SET last_success_at = ?`, formatCloudTime(old)); err != nil {
		t.Fatalf("构造陈旧快照失败: %v", err)
	}
	risks, _, err = handler.buildCloudRisks()
	if err != nil {
		t.Fatalf("构建陈旧快照风险失败: %v", err)
	}
	if !hasRiskCategory(risks, "snapshot_stale") {
		t.Fatalf("期望识别快照陈旧风险，实际: %#v", risks)
	}
}

func hasRiskCategory(items []cloudRiskItem, category string) bool {
	for _, item := range items {
		if item.Category == category {
			return true
		}
	}
	return false
}
