// 本文件用于健康检查接口。
// 文件职责：保持 /api/health 的匿名、轻量、可降级行为。

package api

import (
	"net/http"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

// health 用于返回服务健康与队列指标
func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h == nil || h.fs == nil {
		writeJSON(w, http.StatusOK, models.HealthSnapshot{})
		return
	}
	writeJSON(w, http.StatusOK, h.fs.HealthSnapshot())
}
