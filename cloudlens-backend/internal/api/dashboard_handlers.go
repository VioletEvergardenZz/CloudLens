// 本文件用于 Dashboard 接口和短缓存。
// 文件职责：聚合控制台首页数据，并在运行态缺失时提供降级响应。

package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/state"
)

// dashboard 用于返回系统总览数据供控制台展示
// 控制台高频轮询优先读取短缓存 只有 refresh=true 才强制重算
// 这样可以降低目录扫描与状态聚合的热点开销
func (h *handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h == nil || h.fs == nil {
		writeJSON(w, http.StatusOK, buildFallbackDashboard(nil))
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil {
		cfg = &models.Config{}
	}
	runtimeState := h.fs.State()
	if runtimeState == nil {
		logger.Warn("dashboard fallback: runtime state not ready")
		writeJSON(w, http.StatusOK, buildFallbackDashboard(cfg))
		return
	}
	// 从 ?mode=... 里取值
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "light" || mode == "lite" {
		writeJSON(w, http.StatusOK, runtimeState.DashboardLite(cfg))
		return
	}
	forceRefresh := parseBoolQuery(r, "refresh")
	if !forceRefresh {
		if cached, ok := h.loadDashboardCache(); ok {
			w.Header().Set("X-Dashboard-Cache", "hit")
			writeJSON(w, http.StatusOK, cached)
			return
		}
	}
	payload := runtimeState.Dashboard(cfg)
	h.storeDashboardCache(payload)
	w.Header().Set("X-Dashboard-Cache", "miss")
	writeJSON(w, http.StatusOK, payload)
}

// buildFallbackDashboard 在运行态缺失时提供最小可用结构，避免前端直接报 500
func buildFallbackDashboard(cfg *models.Config) state.DashboardData {
	if cfg == nil {
		cfg = &models.Config{}
	}
	fallbackState := state.NewRuntimeState(cfg)
	return fallbackState.DashboardLite(cfg)
}

// loadDashboardCache 用于加载运行数据
func (h *handler) loadDashboardCache() (any, bool) {
	if h == nil {
		return nil, false
	}
	h.dashboardCacheMu.Lock()
	defer h.dashboardCacheMu.Unlock()
	if h.dashboardCacheData == nil {
		return nil, false
	}
	if h.dashboardCacheExpire.IsZero() || time.Now().After(h.dashboardCacheExpire) {
		h.dashboardCacheData = nil
		h.dashboardCacheExpire = time.Time{}
		return nil, false
	}
	return h.dashboardCacheData, true
}

// storeDashboardCache 用于写入仪表盘缓存减少重复采集开销
func (h *handler) storeDashboardCache(payload any) {
	if h == nil {
		return
	}
	ttl := h.dashboardCacheTTL
	if ttl <= 0 {
		ttl = defaultDashboardTTL
	}
	h.dashboardCacheMu.Lock()
	h.dashboardCacheData = payload
	h.dashboardCacheExpire = time.Now().Add(ttl)
	h.dashboardCacheMu.Unlock()
}

// invalidateDashboardCache 用于在配置变更后失效仪表盘缓存
func (h *handler) invalidateDashboardCache() {
	if h == nil {
		return
	}
	h.dashboardCacheMu.Lock()
	h.dashboardCacheData = nil
	h.dashboardCacheExpire = time.Time{}
	h.dashboardCacheMu.Unlock()
}
