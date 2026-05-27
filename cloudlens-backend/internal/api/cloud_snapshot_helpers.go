// 本文件用于云资源快照的写入与降级读取。
// 文件职责：让云厂商接口在实时查询失败时可返回最近一次成功数据。
// 边界与容错：快照只作为只读兜底，不替代实时云资源状态。

package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
)

func cloudAccountID(account *cloudAccountRecord) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

func (h *handler) saveCloudResourceSnapshot(account *cloudAccountRecord, resourceType string, items any) {
	if h == nil || h.cloudStore == nil || account == nil {
		return
	}
	if err := h.cloudStore.SaveResourceSnapshot(account.ID, account.Provider, resourceType, items, "live"); err != nil {
		logger.Warn("保存云资源快照失败 account=%d resource=%s: %v", account.ID, resourceType, err)
	}
	if h.resourceService != nil {
		payload, err := json.Marshal(items)
		if err == nil {
			resources := normalizeCloudSnapshotResources(cloudResourceSnapshot{
				AccountID:     account.ID,
				Provider:      account.Provider,
				ResourceType:  resourceType,
				PayloadJSON:   payload,
				Source:        "live",
				LastSuccessAt: formatCloudTime(time.Now().UTC()),
			}, *account)
			if err := h.resourceService.Upsert(resources); err != nil {
				logger.Warn("更新云资源索引失败 account=%d resource=%s: %v", account.ID, resourceType, err)
			}
		}
	}
}

func (h *handler) writeCloudSnapshotFallback(
	w http.ResponseWriter,
	account *cloudAccountRecord,
	provider string,
	resourceType string,
	liveErr error,
	humanize func(error) string,
	classify func(error) string,
) bool {
	if h == nil || h.cloudStore == nil || account == nil {
		return false
	}
	message := humanize(liveErr)
	if message == "" && liveErr != nil {
		message = liveErr.Error()
	}
	if err := h.cloudStore.RecordResourceSnapshotError(account.ID, resourceType, message); err != nil {
		logger.Warn("记录云资源快照错误失败 account=%d resource=%s: %v", account.ID, resourceType, err)
	}
	snapshot, err := h.cloudStore.GetResourceSnapshot(account.ID, resourceType)
	if err != nil {
		if err != sql.ErrNoRows {
			logger.Warn("读取云资源快照失败 account=%d resource=%s: %v", account.ID, resourceType, err)
		}
		return false
	}
	payload := snapshot.PayloadJSON
	if !json.Valid(payload) {
		payload = json.RawMessage("[]")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId":     account.ID,
		"ok":            true,
		"degraded":      true,
		"stale":         true,
		"provider":      provider,
		"resource":      snapshot.ResourceType,
		"source":        "snapshot",
		"snapshotAt":    snapshot.LastSuccessAt,
		"lastSuccessAt": snapshot.LastSuccessAt,
		"syncError":     message,
		"code":          classify(liveErr),
		"items":         payload,
		"total":         snapshot.Total,
	})
	return true
}
