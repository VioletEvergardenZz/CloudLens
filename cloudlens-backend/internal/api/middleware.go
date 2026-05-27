// 本文件用于 API 通用中间件与响应工具。
// 文件职责：统一 JSON 响应、CORS 与 panic 兜底。

package api

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/logger"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

// 统一返回 JSON 响应
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// withCORS 用于补充跨域响应头并处理预检请求
// withCORS 负责统一处理跨域策略
// 预检请求在这里直接返回 减少后续业务 handler 的重复分支
func withCORS(cfg *models.Config, next http.Handler) http.Handler {
	allowAll := false
	allowedOrigins := make(map[string]struct{})
	if cfg != nil {
		for _, origin := range strings.FieldsFunc(strings.TrimSpace(cfg.APICORSOrigins), func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			normalized := strings.TrimSpace(origin)
			if normalized == "" {
				continue
			}
			if normalized == "*" {
				allowAll = true
				allowedOrigins = map[string]struct{}{}
				break
			}
			allowedOrigins[normalized] = struct{}{}
		}
	}
	// 未配置白名单时默认放开跨域，降低内网和本地接入门槛
	if !allowAll && len(allowedOrigins) == 0 {
		allowAll = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := strings.TrimSpace(r.Header.Get("Origin"))
		originAllowed := requestOrigin == ""
		if !originAllowed {
			if allowAll {
				originAllowed = true
			} else if _, ok := allowedOrigins[requestOrigin]; ok {
				originAllowed = true
			}
		}

		if originAllowed && requestOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			if !originAllowed {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !originAllowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withRecovery 用于兜底捕获 panic 防止服务崩溃
// withRecovery 防御未捕获 panic
// 这里统一兜底为 500 并记录堆栈 避免单请求异常导致进程退出
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("API发生异常: %v\n%s", recovered, string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器内部错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
