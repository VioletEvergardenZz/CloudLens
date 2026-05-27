// 本文件用于 API 查询参数通用解析。
// 文件职责：沉淀跨 handler 复用的小工具，避免散落在服务入口文件中。

package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

// resolveWriteTimeout 用于解析依赖并返回可用结果
func resolveWriteTimeout(cfg *models.Config) time.Duration {
	base := 90 * time.Second
	if cfg == nil {
		return base
	}
	aiTimeout := parseAITimeout(cfg.AITimeout)
	if aiTimeout > 0 {
		candidate := aiTimeout + 5*time.Second
		if candidate > base {
			base = candidate
		}
	}
	return base
}

// parseBoolQuery 用于解析输入参数或配置
func parseBoolQuery(r *http.Request, key string) bool {
	if r == nil {
		return false
	}
	raw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func parsePositiveInt(raw string, fallback int) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}
	val, err := strconv.Atoi(trimmed)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}
