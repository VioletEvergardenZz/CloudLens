// 本文件用于统一云资源只读索引接口。
// 文件职责：把现有 ECS/RDS 快照收敛成统一资源列表，供后续风险、K8s 关联和前端筛选复用。
// 边界与容错：这里只读取本地快照，不主动调用云厂商 API，避免统一索引放大实时同步失败。

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	appcloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/app/cloud"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
)

type cloudUnifiedResource = appcloud.Resource

type cloudUnifiedSnapshotResource struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Provider          string   `json:"provider"`
	RegionID          string   `json:"regionId"`
	ZoneID            string   `json:"zoneId"`
	Status            string   `json:"status"`
	PrivateIPs        []string `json:"privateIps"`
	PublicIPs         []string `json:"publicIps"`
	EipAddress        string   `json:"eipAddress"`
	NodeID            string   `json:"nodeId"`
	ChargeType        string   `json:"chargeType"`
	PayType           string   `json:"payType"`
	ExpiredAt         string   `json:"expiredAt"`
	ExpiresInDays     *int     `json:"expiresInDays"`
	ExpirationStatus  string   `json:"expirationStatus"`
	ExpirationMessage string   `json:"expirationMessage"`
	Engine            string   `json:"engine"`
	EngineVersion     string   `json:"engineVersion"`
}

func (h *handler) cloudResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h == nil || h.cloudStore == nil || h.resourceService == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "云账号或资源索引存储未初始化",
			"items": []cloudUnifiedResource{},
			"total": 0,
		})
		return
	}
	if err := h.rebuildCloudResourceIndexFromSnapshots(); err != nil {
		logger.Warn("重建云资源索引失败: %v", err)
	}
	filter := parseCloudResourceFilter(r)
	items, err := h.resourceService.List(filter)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"items": []cloudUnifiedResource{},
			"total": 0,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"source": "index",
		"items":  items,
		"total":  len(items),
		"filters": map[string]any{
			"provider":     filter.Provider,
			"resourceType": filter.ResourceType,
			"accountId":    filter.AccountID,
			"region":       filter.Region,
		},
	})
}

func (h *handler) rebuildCloudResourceIndexFromSnapshots() error {
	if h == nil || h.cloudStore == nil || h.resourceService == nil {
		return nil
	}
	accounts, err := h.cloudStore.List()
	if err != nil {
		return err
	}
	accountByID := make(map[int64]cloudAccountRecord, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	snapshots, err := h.cloudStore.ListResourceSnapshots()
	if err != nil {
		return err
	}
	items := make([]appcloud.Resource, 0)
	for _, snapshot := range snapshots {
		account := accountByID[snapshot.AccountID]
		items = append(items, normalizeCloudSnapshotResources(snapshot, account)...)
	}
	return h.resourceService.Upsert(items)
}

type cloudResourceFilter = appcloud.ResourceFilter

func parseCloudResourceFilter(r *http.Request) cloudResourceFilter {
	filter := cloudResourceFilter{
		Provider:     strings.ToLower(strings.TrimSpace(firstCloudQuery(r, "provider"))),
		ResourceType: strings.ToLower(strings.TrimSpace(firstCloudQuery(r, "type", "resourceType"))),
		Region:       strings.TrimSpace(firstCloudQuery(r, "region", "regionId")),
	}
	if raw := strings.TrimSpace(firstCloudQuery(r, "accountId", "accountID")); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil && value > 0 {
			filter.AccountID = value
		}
	}
	return filter
}

func normalizeCloudSnapshotResources(snapshot cloudResourceSnapshot, account cloudAccountRecord) []appcloud.Resource {
	var rawItems []cloudUnifiedSnapshotResource
	if err := json.Unmarshal(snapshot.PayloadJSON, &rawItems); err != nil {
		return nil
	}
	out := make([]appcloud.Resource, 0, len(rawItems))
	for _, raw := range rawItems {
		resourceID := strings.TrimSpace(raw.ID)
		if resourceID == "" {
			continue
		}
		provider := strings.TrimSpace(raw.Provider)
		if provider == "" {
			provider = snapshot.Provider
		}
		chargeType := firstCloudString(raw.ChargeType, raw.PayType)
		publicIPs := append([]string{}, raw.PublicIPs...)
		if strings.TrimSpace(raw.EipAddress) != "" {
			publicIPs = append(publicIPs, strings.TrimSpace(raw.EipAddress))
		}
		rawJSON, _ := json.Marshal(raw)
		out = append(out, appcloud.Resource{
			ID:                buildCloudUnifiedResourceID(snapshot.AccountID, snapshot.ResourceType, resourceID),
			AccountID:         snapshot.AccountID,
			AccountName:       account.Name,
			Provider:          provider,
			ResourceType:      snapshot.ResourceType,
			ResourceID:        resourceID,
			Name:              strings.TrimSpace(raw.Name),
			Region:            strings.TrimSpace(raw.RegionID),
			Zone:              strings.TrimSpace(raw.ZoneID),
			Status:            strings.TrimSpace(raw.Status),
			PrivateIPs:        compactCloudStrings(raw.PrivateIPs),
			PublicIPs:         compactCloudStrings(publicIPs),
			ChargeType:        chargeType,
			ExpiredAt:         strings.TrimSpace(raw.ExpiredAt),
			ExpiresInDays:     raw.ExpiresInDays,
			ExpirationStatus:  strings.TrimSpace(raw.ExpirationStatus),
			ExpirationMessage: strings.TrimSpace(raw.ExpirationMessage),
			Engine:            strings.TrimSpace(raw.Engine),
			EngineVersion:     strings.TrimSpace(raw.EngineVersion),
			NodeID:            strings.TrimSpace(raw.NodeID),
			Source:            strings.TrimSpace(snapshot.Source),
			SnapshotAt:        strings.TrimSpace(snapshot.LastSuccessAt),
			LastError:         strings.TrimSpace(snapshot.LastError),
			RawJSON:           string(rawJSON),
		})
	}
	return out
}

func buildCloudUnifiedResourceID(accountID int64, resourceType, resourceID string) string {
	return strconv.FormatInt(accountID, 10) + ":" + strings.TrimSpace(resourceType) + ":" + strings.TrimSpace(resourceID)
}

func compactCloudStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func cloudUnifiedResourceMatches(item cloudUnifiedResource, filter cloudResourceFilter) bool {
	if filter.Provider != "" && strings.ToLower(item.Provider) != filter.Provider {
		return false
	}
	if filter.ResourceType != "" && strings.ToLower(item.ResourceType) != filter.ResourceType {
		return false
	}
	if filter.AccountID > 0 && item.AccountID != filter.AccountID {
		return false
	}
	if filter.Region != "" && item.Region != filter.Region {
		return false
	}
	return true
}
