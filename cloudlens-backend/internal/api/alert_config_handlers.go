// 本文件用于告警运行态配置接口。
// 文件职责：读取和更新告警配置、告警规则与告警总览。

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

// alertDashboard 用于返回告警模块运行态信息
func (h *handler) alertDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	alertState := h.currentAlertState()
	if alertState == nil {
		// 告警未启用时返回空结果
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "告警未启用",
		})
		return
	}
	enabled := false
	if h != nil && h.fs != nil {
		enabled = h.fs.AlertEnabled()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": enabled,
		"data":    alertState.Dashboard(),
	})
}

// alertConfig 用于读取或更新告警配置
func (h *handler) alertConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config not ready"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		// 读取告警配置快照
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": buildAlertConfigSnapshot(cfg, h.fs.AlertEnabled()),
		})
		return
	case http.MethodPost:
		// 运行时更新告警配置 仅内存生效
		var req struct {
			Enabled         bool   `json:"enabled"`
			SuppressEnabled *bool  `json:"suppressEnabled"`
			RulesFile       string `json:"rulesFile"`
			LogPaths        string `json:"logPaths"`
			PollInterval    string `json:"pollInterval"`
			StartFromEnd    bool   `json:"startFromEnd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		suppressEnabled := true
		if cfg.AlertSuppressEnabled != nil {
			suppressEnabled = *cfg.AlertSuppressEnabled
		}
		if req.SuppressEnabled != nil {
			suppressEnabled = *req.SuppressEnabled
		}
		updated, err := h.fs.UpdateAlertConfig(req.Enabled, suppressEnabled, req.RulesFile, req.LogPaths, req.PollInterval, req.StartFromEnd)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.cfg = updated
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": buildAlertConfigSnapshot(updated, h.fs.AlertEnabled()),
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

// alertRules 用于读取或更新告警规则
func (h *handler) alertRules(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config not ready"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		ruleset := cfg.AlertRules
		if ruleset == nil {
			ruleset = alert.DefaultRuleset()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"rules": ruleset,
		})
		return
	case http.MethodPost:
		var req struct {
			Rules *alert.Ruleset `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		if req.Rules == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rules is required"})
			return
		}
		if err := alert.NormalizeRuleset(req.Rules); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		updated, err := h.fs.UpdateAlertRules(req.Rules)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.cfg = updated
		ruleset := req.Rules
		if updated != nil && updated.AlertRules != nil {
			ruleset = updated.AlertRules
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"rules": ruleset,
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

// buildAlertConfigSnapshot 用于构建后续流程所需的数据
func buildAlertConfigSnapshot(cfg *models.Config, enabled bool) map[string]any {
	if cfg == nil {
		return map[string]any{
			"enabled":         enabled,
			"suppressEnabled": true,
			"rulesFile":       "",
			"logPaths":        "",
			"pollInterval":    "",
			"startFromEnd":    true,
		}
	}
	startFromEnd := true
	if cfg.AlertStartFromEnd != nil {
		startFromEnd = *cfg.AlertStartFromEnd
	}
	suppressEnabled := true
	if cfg.AlertSuppressEnabled != nil {
		suppressEnabled = *cfg.AlertSuppressEnabled
	}
	pollInterval := strings.TrimSpace(cfg.AlertPollInterval)
	if pollInterval == "" {
		pollInterval = "2s"
	}
	return map[string]any{
		"enabled":         enabled,
		"suppressEnabled": suppressEnabled,
		"rulesFile":       "",
		"logPaths":        strings.TrimSpace(cfg.AlertLogPaths),
		"pollInterval":    pollInterval,
		"startFromEnd":    startFromEnd,
	}
}
