// 本文件用于运行态配置接口。
// 文件职责：接收控制台配置变更，并调用 FileService 完成热更新。

package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// updateConfig 用于更新运行状态或配置
// updateConfig 只允许更新运行态白名单字段
// 静态策略字段仍要求改配置并重启 防止在线热切换引发行为漂移
func (h *handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		WatchDir              *string `json:"watchDir"`
		FileExt               *string `json:"fileExt"`
		UploadWorkers         *int    `json:"uploadWorkers"`
		UploadQueueSize       *int    `json:"uploadQueueSize"`
		UploadRetryDelays     *string `json:"uploadRetryDelays"`
		UploadRetryEnabled    *bool   `json:"uploadRetryEnabled"`
		SystemResourceEnabled *bool   `json:"systemResourceEnabled"`
		Silence               *string `json:"silence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	current := h.fs.Config()
	if current == nil {
		current = h.cfg
	}
	if current == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime config not ready"})
		return
	}
	watchDir := current.WatchDir
	if req.WatchDir != nil {
		watchDir = *req.WatchDir
	}
	fileExt := current.FileExt
	if req.FileExt != nil {
		fileExt = *req.FileExt
	}
	silence := current.Silence
	if req.Silence != nil {
		silence = *req.Silence
	}
	uploadWorkers := current.UploadWorkers
	if req.UploadWorkers != nil {
		uploadWorkers = *req.UploadWorkers
	}
	uploadQueueSize := current.UploadQueueSize
	if req.UploadQueueSize != nil {
		uploadQueueSize = *req.UploadQueueSize
	}
	uploadRetryDelays := current.UploadRetryDelays
	if req.UploadRetryDelays != nil {
		uploadRetryDelays = *req.UploadRetryDelays
	}
	uploadRetryEnabled := current.UploadRetryEnabled
	if req.UploadRetryEnabled != nil {
		uploadRetryEnabled = req.UploadRetryEnabled
	}
	cfg, err := h.fs.UpdateConfig(
		watchDir,
		fileExt,
		strings.TrimSpace(silence),
		uploadWorkers,
		uploadQueueSize,
		strings.TrimSpace(uploadRetryDelays),
		uploadRetryEnabled,
		req.SystemResourceEnabled,
	)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	state := h.fs.State()
	if state == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime state not ready"})
		return
	}
	h.cfg = cfg
	h.invalidateDashboardCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"config": state.ConfigSnapshot(cfg),
	})
}
