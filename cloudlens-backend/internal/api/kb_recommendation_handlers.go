// 本文件用于知识库推荐与告警联动。
// 文件职责：根据告警上下文或查询词返回知识库推荐，并记录推荐追踪。

package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/alert"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/kb"
)

func (h *handler) kbRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	rule := strings.TrimSpace(r.URL.Query().Get("rule"))
	message := strings.TrimSpace(r.URL.Query().Get("message"))
	alertID := strings.TrimSpace(r.URL.Query().Get("alertId"))

	decision, found := h.findAlertDecision(alertID)
	if found {
		// 优先复用告警决策快照，避免前端拼接查询词导致推荐结果不可重放
		if rule == "" {
			rule = strings.TrimSpace(decision.Rule)
		}
		if message == "" {
			message = strings.TrimSpace(decision.Message)
		}
	}
	if query == "" {
		query = buildKBRecommendationQuery(rule, message, alertID)
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 3)
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"items": []kb.Article{},
			"trace": nil,
		})
		return
	}
	items, err := h.kb.Recommendations(query, limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	trace := buildKBRecommendationTrace(alertID, query, rule, message, decision, found, items)
	if found {
		state := h.currentAlertState()
		if state != nil && trace != nil {
			state.AttachKnowledgeTrace(alertID, *trace)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
		"trace": trace,
	})
}

func buildKBRecommendationQuery(rule, message, alertID string) string {
	rule = strings.TrimSpace(rule)
	message = strings.TrimSpace(message)
	switch {
	case rule != "" && message != "":
		return rule + " " + message
	case rule != "":
		return rule
	case message != "":
		return message
	default:
		return strings.TrimSpace(alertID)
	}
}

func buildKBRecommendationTrace(alertID, query, rule, message string, decision alert.Decision, found bool, items []kb.Article) *alert.RecommendationTrace {
	if strings.TrimSpace(alertID) == "" {
		return nil
	}
	articles := make([]alert.RecommendationArticle, 0, len(items))
	for _, item := range items {
		articles = append(articles, alert.RecommendationArticle{
			ArticleID: item.ID,
			Title:     item.Title,
			Version:   item.CurrentVersion,
			Status:    item.Status,
			Severity:  item.Severity,
		})
	}
	trace := &alert.RecommendationTrace{
		AlertID:  strings.TrimSpace(alertID),
		LinkedAt: time.Now().Format("2006-01-02 15:04:05"),
		Query:    strings.TrimSpace(query),
		Rule:     strings.TrimSpace(rule),
		Message:  strings.TrimSpace(message),
		HitCount: len(articles),
		Articles: articles,
	}
	if found {
		trace.DecisionStatus = strings.TrimSpace(decision.Status)
		trace.DecisionReason = strings.TrimSpace(decision.Reason)
	}
	trace.LinkID = fmt.Sprintf("kb-link-%s-%d", trace.AlertID, time.Now().UnixNano())
	return trace
}
