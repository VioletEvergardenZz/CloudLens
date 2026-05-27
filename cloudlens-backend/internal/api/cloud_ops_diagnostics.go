// 本文件用于云账号诊断与资源快照摘要。
// 文件职责：读取本地快照和账号状态，不主动调用云厂商接口。

package api

import (
	"fmt"
	"strings"
	"time"
)

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

func expectedCloudPermissions(provider string) []string {
	switch normalizeCloudProvider(provider) {
	case "huawei":
		return []string{"ECS 只读", "RDS 只读", "CES 监控只读", "IAM 项目可见性"}
	default:
		return []string{"AliyunECSReadOnlyAccess", "AliyunRDSReadOnlyAccess", "AliyunCloudMonitorReadOnlyAccess"}
	}
}
