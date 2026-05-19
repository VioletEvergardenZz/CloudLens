package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
)

// alertReport 用于测试、回放或外部探针写入一条告警快照。
func (h *handler) alertReport(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	state := h.currentAlertState()
	if state == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "alert state not ready"})
		return
	}

	var decision alert.Decision
	if err := json.NewDecoder(r.Body).Decode(&decision); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if !state.RecordDecision(decision) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid alert decision"})
		return
	}

	saved, _ := state.GetDecision(decision.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"decision": saved,
	})
}

// alertDecisionWorkflow 用于读取单条告警，或更新人工处置闭环状态。
func (h *handler) alertDecisionWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	alertID, action := parseAlertDecisionPath(r.URL.Path)
	if alertID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}

	state := h.currentAlertState()
	if state == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "alert state not ready"})
		return
	}

	if action == "" && r.Method == http.MethodGet {
		decision, found := state.GetDecision(alertID)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"decision": decision,
		})
		return
	}

	if action != "workflow" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert action not found"})
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Status   string `json:"status"`
		Operator string `json:"operator"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	status, ok := alert.ParseWorkflowStatus(strings.TrimSpace(req.Status))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workflow status"})
		return
	}

	decision, found := state.UpdateDecisionWorkflow(alertID, status, req.Operator, req.Note, time.Now())
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"decision": decision,
	})
}

func parseAlertDecisionPath(path string) (string, string) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/alerts/"), "/")
	if trimmed == "" {
		return "", ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), ""
	}
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", ""
}
