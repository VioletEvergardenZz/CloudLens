// 本文件用于本地运行体检。
// 文件职责：检查云账号存储、密钥权限、CORS、Dashboard 降级和告警闭环状态。

package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

func (h *handler) buildRuntimeChecks() []cloudOpsCheck {
	cfg := h.runtimeConfig()
	var cloudStore *cloudAccountStore
	if h != nil {
		cloudStore = h.cloudStore
	}
	checks := make([]cloudOpsCheck, 0, 8)
	checks = append(checks, buildCloudStoreRuntimeCheck(cloudStore)...)
	checks = append(checks, buildCORSRuntimeCheck(cfg))
	checks = append(checks, buildCloudAssetsRuntimeCheck(cfg))
	checks = append(checks, h.buildDashboardRuntimeCheck())
	checks = append(checks, h.buildAlertWorkflowRuntimeCheck())
	return checks
}

func buildCloudStoreRuntimeCheck(store *cloudAccountStore) []cloudOpsCheck {
	if store == nil {
		return []cloudOpsCheck{{
			Name:       "云账号存储",
			Status:     "error",
			Severity:   "critical",
			Message:    "云账号 SQLite 存储未初始化",
			Suggestion: "确认 data/cloud 目录存在且进程有读写权限",
		}}
	}
	dbPath := strings.TrimSpace(store.DBPath())
	checks := []cloudOpsCheck{{
		Name:     "云账号存储",
		Status:   "ok",
		Severity: "info",
		Message:  fmt.Sprintf("云账号 SQLite 已加载: %s", dbPath),
	}}
	if _, err := os.Stat(dbPath); err != nil {
		checks = append(checks, cloudOpsCheck{
			Name:       "云账号数据库文件",
			Status:     "error",
			Severity:   "critical",
			Message:    fmt.Sprintf("无法读取云账号数据库: %v", err),
			Suggestion: "检查 CLOUD_DATA_DIR、挂载卷和进程权限",
		})
	} else {
		checks = append(checks, cloudOpsCheck{
			Name:     "云账号数据库文件",
			Status:   "ok",
			Severity: "info",
			Message:  "云账号数据库文件可访问",
		})
	}
	dataDir := filepath.Dir(dbPath)
	secretPath := filepath.Join(dataDir, "secret.key")
	if info, err := os.Stat(secretPath); err != nil {
		checks = append(checks, cloudOpsCheck{
			Name:       "云账号密钥权限",
			Status:     "error",
			Severity:   "critical",
			Message:    fmt.Sprintf("无法读取云账号本机密钥: %v", err),
			Suggestion: "重新启动后端生成密钥，或检查 data/cloud/secret.key 挂载权限",
		})
	} else if info.Mode().Perm() != cloudSecretKeyPerm {
		checks = append(checks, cloudOpsCheck{
			Name:       "云账号密钥权限",
			Status:     "warning",
			Severity:   "warning",
			Message:    fmt.Sprintf("secret.key 当前权限为 %o，建议为 %o", info.Mode().Perm(), cloudSecretKeyPerm),
			Suggestion: "执行 chmod 600 data/cloud/secret.key，或重启后端触发自动收紧",
		})
	} else {
		checks = append(checks, cloudOpsCheck{
			Name:     "云账号密钥权限",
			Status:   "ok",
			Severity: "info",
			Message:  "secret.key 权限已收紧为 0600",
		})
	}
	if err := checkDirWritable(dataDir); err != nil {
		checks = append(checks, cloudOpsCheck{
			Name:       "云账号数据目录写入",
			Status:     "error",
			Severity:   "critical",
			Message:    fmt.Sprintf("data/cloud 不可写: %v", err),
			Suggestion: "检查容器 volume、宿主机目录属主和读写权限",
		})
	} else {
		checks = append(checks, cloudOpsCheck{
			Name:     "云账号数据目录写入",
			Status:   "ok",
			Severity: "info",
			Message:  "data/cloud 可写",
		})
	}
	return checks
}

func buildCORSRuntimeCheck(cfg *models.Config) cloudOpsCheck {
	origins := ""
	if cfg != nil {
		origins = strings.TrimSpace(cfg.APICORSOrigins)
	}
	if origins == "" || origins == "*" {
		return cloudOpsCheck{
			Name:       "CORS 白名单",
			Status:     "warning",
			Severity:   "warning",
			Message:    "当前 CORS 默认允许任意 Origin，适合本地调试但不适合公网暴露",
			Suggestion: "生产环境请设置 API_CORS_ORIGINS 或 api_cors_origins 为前端域名白名单",
		}
	}
	return cloudOpsCheck{
		Name:     "CORS 白名单",
		Status:   "ok",
		Severity: "info",
		Message:  fmt.Sprintf("已配置 CORS 白名单: %s", origins),
	}
}

func buildCloudAssetsRuntimeCheck(cfg *models.Config) cloudOpsCheck {
	if cfg == nil {
		return cloudOpsCheck{
			Name:       "云资产开关",
			Status:     "warning",
			Severity:   "warning",
			Message:    "运行配置不可用，无法判断云资产开关",
			Suggestion: "确认后端启动时配置文件加载正常",
		}
	}
	if !cfg.CloudAssetsEnabled {
		return cloudOpsCheck{
			Name:       "云资产开关",
			Status:     "warning",
			Severity:   "warning",
			Message:    "cloud_assets_enabled 未启用，控制台云资产能力可能不可用",
			Suggestion: "如需使用多云监控主线，请在配置中开启 cloud_assets_enabled",
		}
	}
	return cloudOpsCheck{
		Name:     "云资产开关",
		Status:   "ok",
		Severity: "info",
		Message:  "cloud_assets_enabled 已启用",
	}
}

func (h *handler) buildDashboardRuntimeCheck() cloudOpsCheck {
	if h == nil || h.fs == nil {
		return cloudOpsCheck{
			Name:       "Dashboard 降级",
			Status:     "warning",
			Severity:   "warning",
			Message:    "FileService 未注入，/api/dashboard 将只能返回兜底结构",
			Suggestion: "确认后端按完整服务模式启动",
		}
	}
	return cloudOpsCheck{
		Name:     "Dashboard 降级",
		Status:   "ok",
		Severity: "info",
		Message:  "/api/dashboard 支持运行态未就绪时返回 200 降级数据",
	}
}

func (h *handler) buildAlertWorkflowRuntimeCheck() cloudOpsCheck {
	state := h.currentAlertState()
	enabled := false
	if h != nil && h.fs != nil {
		enabled = h.fs.AlertEnabled()
	} else if cfg := h.runtimeConfig(); cfg != nil {
		enabled = cfg.AlertEnabled
	}
	if state == nil {
		return cloudOpsCheck{
			Name:       "告警处置闭环",
			Status:     "warning",
			Severity:   "warning",
			Message:    "告警运行态未初始化，处置闭环接口当前没有可处理事件",
			Suggestion: "需要告警闭环时开启 alert_enabled，并确认告警管理器初始化成功",
		}
	}
	dashboard := state.Dashboard()
	openCount := 0
	for _, decision := range dashboard.Decisions {
		if decision.Workflow.Status != string(alert.WorkflowRecovered) {
			openCount++
		}
	}
	if !enabled {
		return cloudOpsCheck{
			Name:       "告警处置闭环",
			Status:     "warning",
			Severity:   "warning",
			Message:    "告警模块未启用，但处置闭环接口已具备读取和更新能力",
			Suggestion: "需要生产告警闭环时开启 alert_enabled，并配置告警日志来源",
		}
	}
	return cloudOpsCheck{
		Name:     "告警处置闭环",
		Status:   "ok",
		Severity: "info",
		Message:  fmt.Sprintf("告警闭环接口可用，当前未恢复事件 %d 条", openCount),
	}
}

func (h *handler) runtimeConfig() *models.Config {
	if h == nil {
		return nil
	}
	if h.fs != nil {
		if cfg := h.fs.Config(); cfg != nil {
			return cfg
		}
	}
	return h.cfg
}

func checkDirWritable(dir string) error {
	file, err := os.CreateTemp(dir, ".cloudlens-write-check-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
