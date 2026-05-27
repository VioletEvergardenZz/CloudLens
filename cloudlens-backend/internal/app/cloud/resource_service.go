// 本文件用于云资源应用服务。
// 文件职责：封装统一资源索引的查询、写入和风险计算入口，让 API 层只做参数与响应编排。

package cloud

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ResourceStore interface {
	UpsertResources(resources []Resource) error
	ListResources(filter ResourceFilter) ([]Resource, error)
	GetResource(id string) (*Resource, error)
}

type ResourceService struct {
	store ResourceStore
	now   func() time.Time
}

func NewResourceService(store ResourceStore) *ResourceService {
	return &ResourceService{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *ResourceService) Upsert(resources []Resource) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("资源索引存储未初始化")
	}
	return s.store.UpsertResources(resources)
}

func (s *ResourceService) List(filter ResourceFilter) ([]Resource, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("资源索引存储未初始化")
	}
	return s.store.ListResources(filter)
}

func (s *ResourceService) Get(id string) (*Resource, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("资源索引存储未初始化")
	}
	return s.store.GetResource(id)
}

func (s *ResourceService) Risks(filter ResourceFilter) ([]Risk, RiskSummary, error) {
	resources, err := s.List(filter)
	if err != nil {
		return nil, emptyRiskSummary(), err
	}
	now := time.Now().UTC()
	if s != nil && s.now != nil {
		now = s.now()
	}
	items := make([]Risk, 0)
	for _, resource := range resources {
		items = append(items, buildResourceRisks(resource, now)...)
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := severityWeight(items[i].Severity)
		right := severityWeight(items[j].Severity)
		if left != right {
			return left > right
		}
		if items[i].AccountID != items[j].AccountID {
			return items[i].AccountID < items[j].AccountID
		}
		return items[i].ID < items[j].ID
	})
	return items, summarizeRisks(items), nil
}

func buildResourceRisks(resource Resource, now time.Time) []Risk {
	resourceID := strings.TrimSpace(resource.ResourceID)
	if resourceID == "" {
		resourceID = "unknown"
	}
	resourceName := strings.TrimSpace(resource.Name)
	if resourceName == "" {
		resourceName = resourceID
	}
	base := Risk{
		Provider:     strings.TrimSpace(resource.Provider),
		AccountID:    resource.AccountID,
		AccountName:  strings.TrimSpace(resource.AccountName),
		ResourceType: strings.TrimSpace(resource.ResourceType),
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Region:       strings.TrimSpace(resource.Region),
		DetectedAt:   now.Format(time.RFC3339Nano),
	}
	risks := make([]Risk, 0, 3)
	if resource.ExpirationStatus == "expired" || resource.ExpirationStatus == "expiring" {
		item := base
		item.ID = fmt.Sprintf("expiration-%d-%s-%s", resource.AccountID, resource.ResourceType, resourceID)
		item.Category = "expiration"
		item.Severity = "warning"
		if resource.ExpirationStatus == "expired" {
			item.Severity = "critical"
		}
		item.Message = resourceTypeChinese(resource.ResourceType) + " " + fallback(resource.ExpirationMessage, "存在到期风险")
		item.Suggestion = "确认续费策略；若是临时资源，请在处置记录中标记预期下线时间"
		item.Evidence = "expiredAt=" + strings.TrimSpace(resource.ExpiredAt)
		risks = append(risks, item)
	}
	if resource.ResourceType == "ecs" && len(resource.PublicIPs) > 0 {
		item := base
		item.ID = fmt.Sprintf("public-ip-%d-%s", resource.AccountID, resourceID)
		item.Category = "public_exposure"
		item.Severity = "warning"
		item.Message = "ECS 存在公网地址，请确认安全组和暴露面符合预期"
		item.Suggestion = "优先核对安全组入站规则；非必要公网访问建议改为内网、堡垒机或 VPN"
		item.Evidence = strings.Join(resource.PublicIPs, ",")
		risks = append(risks, item)
	}
	if isResourceNotRunning(resource.Status, resource.ResourceType) {
		item := base
		item.ID = fmt.Sprintf("status-%d-%s-%s", resource.AccountID, resource.ResourceType, resourceID)
		item.Category = "resource_status"
		item.Severity = "warning"
		item.Message = fmt.Sprintf("%s 当前状态为 %s", resourceTypeChinese(resource.ResourceType), resource.Status)
		item.Suggestion = "确认资源是否预期停机；非预期时检查实例事件、欠费、锁定或变更记录"
		risks = append(risks, item)
	}
	return risks
}

func summarizeRisks(items []Risk) RiskSummary {
	summary := emptyRiskSummary()
	for _, item := range items {
		summary.Total++
		switch item.Severity {
		case "critical":
			summary.Critical++
		case "warning":
			summary.Warning++
		default:
			summary.Info++
		}
		accountKey := fallback(item.AccountName, fmt.Sprint(item.AccountID))
		summary.ByAccount[accountKey]++
		summary.ByCategory[item.Category]++
	}
	return summary
}

func emptyRiskSummary() RiskSummary {
	return RiskSummary{ByAccount: map[string]int{}, ByCategory: map[string]int{}}
}

func severityWeight(severity string) int {
	switch strings.TrimSpace(severity) {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func isResourceNotRunning(status, resourceType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" {
		return false
	}
	if resourceType == "ecs" {
		return normalized != "running"
	}
	if resourceType == "rds" {
		return normalized != "running" && normalized != "available"
	}
	return false
}

func resourceTypeChinese(resourceType string) string {
	switch strings.TrimSpace(resourceType) {
	case "ecs":
		return "云服务器"
	case "rds":
		return "数据库"
	default:
		return strings.ToUpper(strings.TrimSpace(resourceType))
	}
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallbackValue
}
