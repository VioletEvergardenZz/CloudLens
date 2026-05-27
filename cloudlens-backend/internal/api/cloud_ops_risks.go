// 本文件用于云资源风险识别。
// 文件职责：基于本地资源快照计算到期、公网暴露、RDS 存储和状态风险。

package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

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

func resourceTypeChinese(resourceType string) string {
	switch strings.TrimSpace(resourceType) {
	case "rds":
		return "RDS"
	default:
		return "ECS"
	}
}
