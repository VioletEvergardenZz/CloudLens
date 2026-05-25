// 本文件用于云资产运维体检接口。
// 文件职责：汇总云账号、资源快照、风险项和本地运行检查，给控制台提供可解释的诊断数据。
// 边界与容错：体检接口不主动调用云厂商接口，只读取本地状态，避免体检本身放大云 API 抖动。

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

const (
	cloudSnapshotFreshDuration = 30 * time.Minute
	cloudSnapshotStaleDuration = 6 * time.Hour
)

type cloudOpsCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type cloudSnapshotSummary struct {
	AccountID     int64  `json:"accountId"`
	Provider      string `json:"provider"`
	ResourceType  string `json:"resourceType"`
	Total         int    `json:"total"`
	Source        string `json:"source"`
	LastSuccessAt string `json:"lastSuccessAt"`
	LastError     string `json:"lastError,omitempty"`
	AgeSeconds    int64  `json:"ageSeconds"`
	Status        string `json:"status"`
}

type cloudAccountDiagnostic struct {
	Account             cloudAccountRecord     `json:"account"`
	Status              string                 `json:"status"`
	ExpectedPermissions []string               `json:"expectedPermissions"`
	Checks              []cloudOpsCheck        `json:"checks"`
	Snapshots           []cloudSnapshotSummary `json:"snapshots"`
}

type cloudRiskItem struct {
	ID           string `json:"id"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Provider     string `json:"provider"`
	AccountID    int64  `json:"accountId"`
	AccountName  string `json:"accountName"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	Region       string `json:"region"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion"`
	Evidence     string `json:"evidence,omitempty"`
	DetectedAt   string `json:"detectedAt"`
}

type cloudRiskSummary struct {
	Total      int            `json:"total"`
	Critical   int            `json:"critical"`
	Warning    int            `json:"warning"`
	Info       int            `json:"info"`
	ByAccount  map[string]int `json:"byAccount"`
	ByCategory map[string]int `json:"byCategory"`
}

type cloudInspectionReport struct {
	GeneratedAt string                   `json:"generatedAt"`
	Status      string                   `json:"status"`
	Summary     map[string]int           `json:"summary"`
	Diagnostics []cloudAccountDiagnostic `json:"diagnostics"`
	Risks       []cloudRiskItem          `json:"risks"`
	Runtime     []cloudOpsCheck          `json:"runtime"`
	NextActions []string                 `json:"nextActions"`
}

type cloudSnapshotResource struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Provider          string                  `json:"provider"`
	RegionID          string                  `json:"regionId"`
	Status            string                  `json:"status"`
	PublicIPs         []string                `json:"publicIps"`
	EipAddress        string                  `json:"eipAddress"`
	ExpiredAt         string                  `json:"expiredAt"`
	ExpiresInDays     *int                    `json:"expiresInDays"`
	ExpirationStatus  string                  `json:"expirationStatus"`
	ExpirationMessage string                  `json:"expirationMessage"`
	ResourceUsage     *cloudSnapshotUsage     `json:"resourceUsage"`
	ConnectionString  string                  `json:"connectionString"`
	Endpoints         []cloudSnapshotEndpoint `json:"endpoints"`
	DetailErrors      []string                `json:"detailErrors"`
}

type cloudSnapshotUsage struct {
	StorageUsagePercent *float64 `json:"storageUsagePercent"`
	DiskUsedBytes       int64    `json:"diskUsedBytes"`
	Source              string   `json:"source"`
}

type cloudSnapshotEndpoint struct {
	ConnectionString string `json:"connectionString"`
	IPType           string `json:"ipType"`
	VPCID            string `json:"vpcId"`
}

func (h *handler) cloudSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h == nil || h.cloudStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "云账号存储未初始化",
			"items": []cloudSnapshotSummary{},
			"total": 0,
		})
		return
	}
	snapshots, err := h.cloudStore.ListResourceSnapshots()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items := make([]cloudSnapshotSummary, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, buildCloudSnapshotSummary(snapshot))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
		"total": len(items),
	})
}

func (h *handler) cloudDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	diagnostics, snapshots, err := h.buildCloudDiagnostics()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          false,
			"error":       err.Error(),
			"diagnostics": []cloudAccountDiagnostic{},
			"snapshots":   []cloudSnapshotSummary{},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"diagnostics": diagnostics,
		"snapshots":   snapshots,
		"total":       len(diagnostics),
	})
}

func (h *handler) cloudRisks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	risks, summary, err := h.buildCloudRisks()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"items":   []cloudRiskItem{},
			"summary": cloudRiskSummary{ByAccount: map[string]int{}, ByCategory: map[string]int{}},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"items":   risks,
		"summary": summary,
		"total":   len(risks),
	})
}

func (h *handler) cloudInspectionReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	report := h.buildCloudInspectionReport()
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "markdown") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderCloudInspectionMarkdown(report)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"report": report,
	})
}

func (h *handler) runtimeChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	checks := h.buildRuntimeChecks()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"status":    summarizeCloudOpsChecks(checks),
		"checks":    checks,
		"checkedAt": formatCloudTime(time.Now().UTC()),
	})
}

func (h *handler) buildCloudDiagnostics() ([]cloudAccountDiagnostic, []cloudSnapshotSummary, error) {
	if h == nil || h.cloudStore == nil {
		return nil, nil, fmt.Errorf("云账号存储未初始化")
	}
	accounts, err := h.cloudStore.List()
	if err != nil {
		return nil, nil, err
	}
	snapshots, err := h.cloudStore.ListResourceSnapshots()
	if err != nil {
		return nil, nil, err
	}
	snapshotByAccount := make(map[int64][]cloudSnapshotSummary)
	allSnapshots := make([]cloudSnapshotSummary, 0, len(snapshots))
	for _, snapshot := range snapshots {
		summary := buildCloudSnapshotSummary(snapshot)
		allSnapshots = append(allSnapshots, summary)
		snapshotByAccount[summary.AccountID] = append(snapshotByAccount[summary.AccountID], summary)
	}

	out := make([]cloudAccountDiagnostic, 0, len(accounts))
	for _, account := range accounts {
		item := cloudAccountDiagnostic{
			Account:             sanitizeCloudAccount(account),
			ExpectedPermissions: expectedCloudPermissions(account.Provider),
			Snapshots:           snapshotByAccount[account.ID],
		}
		item.Checks = append(item.Checks, buildCloudAccountChecks(account, item.Snapshots)...)
		item.Status = summarizeCloudOpsChecks(item.Checks)
		out = append(out, item)
	}
	return out, allSnapshots, nil
}

func (h *handler) buildCloudRisks() ([]cloudRiskItem, cloudRiskSummary, error) {
	summary := cloudRiskSummary{
		ByAccount:  make(map[string]int),
		ByCategory: make(map[string]int),
	}
	if h == nil || h.cloudStore == nil {
		return nil, summary, fmt.Errorf("云账号存储未初始化")
	}
	accounts, err := h.cloudStore.List()
	if err != nil {
		return nil, summary, err
	}
	accountByID := make(map[int64]cloudAccountRecord, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	snapshots, err := h.cloudStore.ListResourceSnapshots()
	if err != nil {
		return nil, summary, err
	}

	now := time.Now().UTC()
	risks := make([]cloudRiskItem, 0)
	for _, snapshot := range snapshots {
		account := accountByID[snapshot.AccountID]
		risks = append(risks, buildSnapshotFreshnessRisks(snapshot, account, now)...)
		resources, err := decodeCloudSnapshotResources(snapshot)
		if err != nil {
			risks = append(risks, cloudRiskItem{
				ID:           fmt.Sprintf("snapshot-json-%d-%s", snapshot.AccountID, snapshot.ResourceType),
				Severity:     "warning",
				Category:     "snapshot_decode",
				Provider:     snapshot.Provider,
				AccountID:    snapshot.AccountID,
				AccountName:  account.Name,
				ResourceType: snapshot.ResourceType,
				Message:      "资源快照无法解析，风险中心暂时无法识别该批资源",
				Suggestion:   "重新触发资源同步；如果仍失败，请检查后端日志中的快照序列化错误",
				Evidence:     err.Error(),
				DetectedAt:   formatCloudTime(now),
			})
			continue
		}
		for _, resource := range resources {
			risks = append(risks, buildResourceRisks(snapshot, account, resource, now)...)
		}
	}
	sortCloudRisks(risks)
	summary.Total = len(risks)
	for _, item := range risks {
		switch item.Severity {
		case "critical":
			summary.Critical++
		case "warning":
			summary.Warning++
		default:
			summary.Info++
		}
		accountKey := strings.TrimSpace(item.AccountName)
		if accountKey == "" {
			accountKey = fmt.Sprintf("%d", item.AccountID)
		}
		summary.ByAccount[accountKey]++
		summary.ByCategory[item.Category]++
	}
	return risks, summary, nil
}

func (h *handler) buildCloudInspectionReport() cloudInspectionReport {
	diagnostics, _, diagErr := h.buildCloudDiagnostics()
	risks, riskSummary, riskErr := h.buildCloudRisks()
	runtimeChecks := h.buildRuntimeChecks()
	if diagErr != nil {
		runtimeChecks = append(runtimeChecks, cloudOpsCheck{
			Name:       "云账号诊断",
			Status:     "error",
			Severity:   "critical",
			Message:    diagErr.Error(),
			Suggestion: "确认 data/cloud 目录可写，或查看后端启动日志中的云账号存储初始化错误",
		})
	}
	if riskErr != nil {
		runtimeChecks = append(runtimeChecks, cloudOpsCheck{
			Name:       "风险中心",
			Status:     "error",
			Severity:   "critical",
			Message:    riskErr.Error(),
			Suggestion: "风险中心依赖资源快照，请先确认云账号存储可用",
		})
	}
	nextActions := buildCloudInspectionNextActions(diagnostics, risks, runtimeChecks)
	summary := map[string]int{
		"accounts":      len(diagnostics),
		"risks":         len(risks),
		"criticalRisk":  riskSummary.Critical,
		"warningRisk":   riskSummary.Warning,
		"runtimeChecks": len(runtimeChecks),
	}
	report := cloudInspectionReport{
		GeneratedAt: formatCloudTime(time.Now().UTC()),
		Status:      summarizeCloudOpsChecks(runtimeChecks),
		Summary:     summary,
		Diagnostics: diagnostics,
		Risks:       risks,
		Runtime:     runtimeChecks,
		NextActions: nextActions,
	}
	if riskSummary.Critical > 0 || hasCloudOpsCheckStatus(runtimeChecks, "error") {
		report.Status = "error"
	} else if riskSummary.Warning > 0 || len(risks) > 0 || report.Status == "warning" {
		report.Status = "warning"
	}
	return report
}

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

func buildCloudAccountChecks(account cloudAccountRecord, snapshots []cloudSnapshotSummary) []cloudOpsCheck {
	checks := make([]cloudOpsCheck, 0, 4)
	if !account.Enabled {
		checks = append(checks, cloudOpsCheck{
			Name:       "账号启用状态",
			Status:     "warning",
			Severity:   "warning",
			Message:    "云账号已停用，资源同步不会使用该账号",
			Suggestion: "确认是否为主动停用；如需恢复同步，请重新启用账号",
		})
	} else {
		checks = append(checks, cloudOpsCheck{
			Name:     "账号启用状态",
			Status:   "ok",
			Severity: "info",
			Message:  "云账号已启用",
		})
	}
	checkStatus := strings.TrimSpace(account.LastCheckStatus)
	checkMessage := strings.TrimSpace(account.LastCheckMessage)
	switch checkStatus {
	case "ok":
		checks = append(checks, cloudOpsCheck{Name: "最近账号测试", Status: "ok", Severity: "info", Message: fallbackCloudMessage(checkMessage, "最近一次账号测试通过")})
	case "warning":
		checks = append(checks, cloudOpsCheck{Name: "最近账号测试", Status: "warning", Severity: "warning", Message: fallbackCloudMessage(checkMessage, "最近一次账号测试存在局部失败"), Suggestion: "优先检查 RDS 或云监控只读权限"})
	case "error":
		checks = append(checks, cloudOpsCheck{Name: "最近账号测试", Status: "error", Severity: "critical", Message: fallbackCloudMessage(checkMessage, "最近一次账号测试失败"), Suggestion: "重新测试账号，并检查 AK/SK、RAM/IAM 权限和地域可见性"})
	default:
		checks = append(checks, cloudOpsCheck{Name: "最近账号测试", Status: "warning", Severity: "warning", Message: "尚未执行账号测试", Suggestion: "在云账号管理中执行一次账号测试，确认 ECS/RDS/监控权限"})
	}
	if len(snapshots) == 0 {
		checks = append(checks, cloudOpsCheck{
			Name:       "资源快照覆盖",
			Status:     "warning",
			Severity:   "warning",
			Message:    "该账号尚未形成 ECS/RDS 资源快照",
			Suggestion: "进入资源总览触发一次同步，或调用对应 provider 的 instances 接口",
		})
		return checks
	}
	seen := make(map[string]cloudSnapshotSummary, len(snapshots))
	for _, snapshot := range snapshots {
		seen[snapshot.ResourceType] = snapshot
	}
	for _, resourceType := range []string{"ecs", "rds"} {
		snapshot, ok := seen[resourceType]
		if !ok {
			checks = append(checks, cloudOpsCheck{
				Name:       fmt.Sprintf("%s 快照覆盖", strings.ToUpper(resourceType)),
				Status:     "warning",
				Severity:   "warning",
				Message:    fmt.Sprintf("尚未形成 %s 快照", strings.ToUpper(resourceType)),
				Suggestion: "同步对应资源列表后，风险中心才能覆盖该资源类型",
			})
			continue
		}
		status := "ok"
		severity := "info"
		if snapshot.Status == "stale" {
			status = "warning"
			severity = "warning"
		}
		checks = append(checks, cloudOpsCheck{
			Name:     fmt.Sprintf("%s 快照覆盖", strings.ToUpper(resourceType)),
			Status:   status,
			Severity: severity,
			Message:  fmt.Sprintf("最近快照包含 %d 条资源，状态 %s", snapshot.Total, snapshot.Status),
		})
	}
	return checks
}

func buildCloudSnapshotSummary(snapshot cloudResourceSnapshot) cloudSnapshotSummary {
	lastSuccess := parseCloudTime(snapshot.LastSuccessAt)
	ageSeconds := int64(0)
	status := "unknown"
	if !lastSuccess.IsZero() {
		ageSeconds = int64(time.Since(lastSuccess).Seconds())
		switch {
		case time.Since(lastSuccess) <= cloudSnapshotFreshDuration:
			status = "fresh"
		case time.Since(lastSuccess) <= cloudSnapshotStaleDuration:
			status = "aging"
		default:
			status = "stale"
		}
	}
	if strings.TrimSpace(snapshot.LastError) != "" && status != "stale" {
		status = "degraded"
	}
	return cloudSnapshotSummary{
		AccountID:     snapshot.AccountID,
		Provider:      snapshot.Provider,
		ResourceType:  snapshot.ResourceType,
		Total:         snapshot.Total,
		Source:        snapshot.Source,
		LastSuccessAt: snapshot.LastSuccessAt,
		LastError:     snapshot.LastError,
		AgeSeconds:    ageSeconds,
		Status:        status,
	}
}

func buildSnapshotFreshnessRisks(snapshot cloudResourceSnapshot, account cloudAccountRecord, now time.Time) []cloudRiskItem {
	lastSuccess := parseCloudTime(snapshot.LastSuccessAt)
	if lastSuccess.IsZero() || now.Sub(lastSuccess) <= cloudSnapshotStaleDuration {
		return nil
	}
	return []cloudRiskItem{{
		ID:           fmt.Sprintf("snapshot-stale-%d-%s", snapshot.AccountID, snapshot.ResourceType),
		Severity:     "warning",
		Category:     "snapshot_stale",
		Provider:     snapshot.Provider,
		AccountID:    snapshot.AccountID,
		AccountName:  account.Name,
		ResourceType: snapshot.ResourceType,
		Message:      fmt.Sprintf("%s 资源快照超过 6 小时未刷新", strings.ToUpper(snapshot.ResourceType)),
		Suggestion:   "检查云账号权限、网络连通性和同步入口；必要时重新执行账号测试",
		Evidence:     fmt.Sprintf("lastSuccessAt=%s", snapshot.LastSuccessAt),
		DetectedAt:   formatCloudTime(now),
	}}
}

func buildResourceRisks(snapshot cloudResourceSnapshot, account cloudAccountRecord, resource cloudSnapshotResource, now time.Time) []cloudRiskItem {
	resourceID := strings.TrimSpace(resource.ID)
	if resourceID == "" {
		resourceID = "unknown"
	}
	resourceName := strings.TrimSpace(resource.Name)
	if resourceName == "" {
		resourceName = resourceID
	}
	base := cloudRiskItem{
		Provider:     firstCloudString(resource.Provider, snapshot.Provider),
		AccountID:    snapshot.AccountID,
		AccountName:  account.Name,
		ResourceType: snapshot.ResourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Region:       strings.TrimSpace(resource.RegionID),
		DetectedAt:   formatCloudTime(now),
	}
	risks := make([]cloudRiskItem, 0, 4)
	if resource.ExpirationStatus == "expired" || resource.ExpirationStatus == "expiring" {
		item := base
		item.ID = fmt.Sprintf("expiration-%d-%s-%s", snapshot.AccountID, snapshot.ResourceType, resourceID)
		item.Category = "expiration"
		item.Severity = "warning"
		if resource.ExpirationStatus == "expired" {
			item.Severity = "critical"
		}
		item.Message = fmt.Sprintf("%s %s", resourceTypeChinese(snapshot.ResourceType), fallbackCloudMessage(resource.ExpirationMessage, "存在到期风险"))
		item.Suggestion = "确认续费策略；若是临时资源，请在处置记录中标记预期下线时间"
		item.Evidence = fmt.Sprintf("expiredAt=%s", resource.ExpiredAt)
		risks = append(risks, item)
	}
	if snapshot.ResourceType == "ecs" && hasPublicAddress(resource) {
		item := base
		item.ID = fmt.Sprintf("public-ip-%d-%s", snapshot.AccountID, resourceID)
		item.Category = "public_exposure"
		item.Severity = "warning"
		item.Message = "ECS 存在公网地址，请确认安全组和暴露面符合预期"
		item.Suggestion = "优先核对安全组入站规则；非必要公网访问建议改为内网、堡垒机或 VPN"
		item.Evidence = strings.Join(append(resource.PublicIPs, resource.EipAddress), ",")
		risks = append(risks, item)
	}
	if snapshot.ResourceType == "rds" {
		if percent := storageUsagePercent(resource); percent != nil {
			if *percent >= 90 {
				item := base
				item.ID = fmt.Sprintf("rds-storage-critical-%d-%s", snapshot.AccountID, resourceID)
				item.Category = "rds_storage"
				item.Severity = "critical"
				item.Message = "RDS 存储使用率已超过 90%"
				item.Suggestion = "尽快扩容或清理归档数据，避免写入失败"
				item.Evidence = fmt.Sprintf("%.1f%%", *percent)
				risks = append(risks, item)
			} else if *percent >= 80 {
				item := base
				item.ID = fmt.Sprintf("rds-storage-warning-%d-%s", snapshot.AccountID, resourceID)
				item.Category = "rds_storage"
				item.Severity = "warning"
				item.Message = "RDS 存储使用率已超过 80%"
				item.Suggestion = "观察增长趋势并准备扩容窗口"
				item.Evidence = fmt.Sprintf("%.1f%%", *percent)
				risks = append(risks, item)
			}
		}
		if rdsHasPublicEndpoint(resource) {
			item := base
			item.ID = fmt.Sprintf("rds-public-%d-%s", snapshot.AccountID, resourceID)
			item.Category = "public_exposure"
			item.Severity = "warning"
			item.Message = "RDS 疑似存在公网连接地址"
			item.Suggestion = "确认白名单、连接地址类型和业务访问来源；非必要请关闭公网地址"
			item.Evidence = firstRDSEndpointEvidence(resource)
			risks = append(risks, item)
		}
	}
	if isCloudResourceNotRunning(resource.Status, snapshot.ResourceType) {
		item := base
		item.ID = fmt.Sprintf("status-%d-%s-%s", snapshot.AccountID, snapshot.ResourceType, resourceID)
		item.Category = "resource_status"
		item.Severity = "warning"
		item.Message = fmt.Sprintf("%s 当前状态为 %s", resourceTypeChinese(snapshot.ResourceType), resource.Status)
		item.Suggestion = "确认资源是否预期停机；非预期时检查实例事件、欠费、锁定或变更记录"
		risks = append(risks, item)
	}
	if len(resource.DetailErrors) > 0 {
		item := base
		item.ID = fmt.Sprintf("detail-error-%d-%s-%s", snapshot.AccountID, snapshot.ResourceType, resourceID)
		item.Category = "detail_partial_failure"
		item.Severity = "info"
		item.Message = "资源详情补充存在局部失败"
		item.Suggestion = "补齐对应详情接口权限后，可提升风险识别准确度"
		item.Evidence = strings.Join(resource.DetailErrors, "; ")
		risks = append(risks, item)
	}
	return risks
}

func decodeCloudSnapshotResources(snapshot cloudResourceSnapshot) ([]cloudSnapshotResource, error) {
	var resources []cloudSnapshotResource
	if len(snapshot.PayloadJSON) == 0 {
		return resources, nil
	}
	if err := json.Unmarshal(snapshot.PayloadJSON, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func sortCloudRisks(items []cloudRiskItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left := cloudRiskSeverityWeight(items[i].Severity)
		right := cloudRiskSeverityWeight(items[j].Severity)
		if left != right {
			return left > right
		}
		if items[i].AccountID != items[j].AccountID {
			return items[i].AccountID < items[j].AccountID
		}
		return items[i].ID < items[j].ID
	})
}

func cloudRiskSeverityWeight(severity string) int {
	switch strings.TrimSpace(severity) {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func hasPublicAddress(resource cloudSnapshotResource) bool {
	if strings.TrimSpace(resource.EipAddress) != "" {
		return true
	}
	for _, ip := range resource.PublicIPs {
		if strings.TrimSpace(ip) != "" {
			return true
		}
	}
	return false
}

func storageUsagePercent(resource cloudSnapshotResource) *float64 {
	if resource.ResourceUsage == nil {
		return nil
	}
	return resource.ResourceUsage.StorageUsagePercent
}

func rdsHasPublicEndpoint(resource cloudSnapshotResource) bool {
	if strings.Contains(strings.ToLower(resource.ConnectionString), "public") {
		return true
	}
	for _, endpoint := range resource.Endpoints {
		raw := strings.ToLower(strings.TrimSpace(endpoint.ConnectionString + " " + endpoint.IPType))
		if strings.Contains(raw, "public") || strings.Contains(raw, "internet") || strings.Contains(raw, "公网") {
			return true
		}
	}
	return false
}

func firstRDSEndpointEvidence(resource cloudSnapshotResource) string {
	if strings.TrimSpace(resource.ConnectionString) != "" {
		return resource.ConnectionString
	}
	for _, endpoint := range resource.Endpoints {
		if strings.TrimSpace(endpoint.ConnectionString) != "" {
			return endpoint.ConnectionString
		}
	}
	return ""
}

func isCloudResourceNotRunning(status, resourceType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" {
		return false
	}
	switch strings.TrimSpace(resourceType) {
	case "ecs":
		return normalized != "running"
	case "rds":
		switch normalized {
		case "running", "available", "normal", "active":
			return false
		default:
			return true
		}
	default:
		return false
	}
}

func expectedCloudPermissions(provider string) []string {
	switch normalizeCloudProvider(provider) {
	case "huawei":
		return []string{"ECS 只读", "RDS 只读", "CES 监控只读", "IAM 项目可见性"}
	default:
		return []string{"AliyunECSReadOnlyAccess", "AliyunRDSReadOnlyAccess", "AliyunCloudMonitorReadOnlyAccess"}
	}
}

func summarizeCloudOpsChecks(checks []cloudOpsCheck) string {
	status := "ok"
	for _, check := range checks {
		switch check.Status {
		case "error":
			return "error"
		case "warning":
			status = "warning"
		}
	}
	return status
}

func hasCloudOpsCheckStatus(checks []cloudOpsCheck, status string) bool {
	for _, check := range checks {
		if check.Status == status {
			return true
		}
	}
	return false
}

func buildCloudInspectionNextActions(diagnostics []cloudAccountDiagnostic, risks []cloudRiskItem, runtimeChecks []cloudOpsCheck) []string {
	out := make([]string, 0, 5)
	if hasCloudOpsCheckStatus(runtimeChecks, "error") {
		out = append(out, "先处理运行体检中的 error 项，确保本地数据目录、密钥和数据库可用")
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Status == "error" || diagnostic.Status == "warning" {
			out = append(out, fmt.Sprintf("重新测试云账号 %s，并按提示补齐只读权限", diagnostic.Account.Name))
			break
		}
	}
	for _, risk := range risks {
		if risk.Severity == "critical" {
			out = append(out, "优先处理 critical 风险，尤其是已过期资源和 RDS 存储高水位")
			break
		}
	}
	if len(risks) > 0 {
		out = append(out, "将可确认的风险同步到告警处置记录，形成 open -> processing -> recovered 闭环")
	}
	if len(out) == 0 {
		out = append(out, "保持账号测试和资源同步节奏，下一步可接入第二轮 provider 抽象与告警联动")
	}
	return out
}

func renderCloudInspectionMarkdown(report cloudInspectionReport) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# CloudLens 轻量巡检报告\n\n")
	fmt.Fprintf(&buf, "- 生成时间：%s\n", report.GeneratedAt)
	fmt.Fprintf(&buf, "- 总体状态：%s\n", report.Status)
	fmt.Fprintf(&buf, "- 云账号：%d\n", report.Summary["accounts"])
	fmt.Fprintf(&buf, "- 风险项：%d（critical %d / warning %d）\n\n", report.Summary["risks"], report.Summary["criticalRisk"], report.Summary["warningRisk"])
	fmt.Fprintf(&buf, "## 下一步动作\n\n")
	for _, action := range report.NextActions {
		fmt.Fprintf(&buf, "- %s\n", action)
	}
	fmt.Fprintf(&buf, "\n## 运行体检\n\n")
	for _, check := range report.Runtime {
		fmt.Fprintf(&buf, "- **%s**：%s，%s\n", check.Name, check.Status, check.Message)
	}
	fmt.Fprintf(&buf, "\n## 云账号诊断\n\n")
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(&buf, "- **%s**：%s，快照 %d 组\n", diagnostic.Account.Name, diagnostic.Status, len(diagnostic.Snapshots))
	}
	fmt.Fprintf(&buf, "\n## 风险明细\n\n")
	if len(report.Risks) == 0 {
		fmt.Fprintf(&buf, "- 暂无风险项\n")
		return buf.String()
	}
	for _, risk := range report.Risks {
		fmt.Fprintf(&buf, "- [%s] %s / %s：%s\n", risk.Severity, risk.AccountName, risk.ResourceName, risk.Message)
	}
	return buf.String()
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

func fallbackCloudMessage(message, fallback string) string {
	if strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	return fallback
}

func resourceTypeChinese(resourceType string) string {
	switch strings.TrimSpace(resourceType) {
	case "rds":
		return "RDS"
	default:
		return "ECS"
	}
}
