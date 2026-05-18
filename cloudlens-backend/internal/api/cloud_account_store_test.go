// 本文件用于云账号存储的单元测试。
// 文件职责：覆盖多云 provider 保存、读取和切换校验。

package api

import (
	"strings"
	"testing"
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
