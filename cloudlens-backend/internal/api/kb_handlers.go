// 本文件用于知识库 HTTP 处理器 将知识库能力通过统一路由暴露给控制台

// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/kb"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/metrics"
)

// kbArticles 统一处理知识库列表查询与新增
// GET/POST 共用一个入口便于保持参数处理和错误返回一致
func (h *handler) kbArticles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		pageSize := parsePositiveInt(r.URL.Query().Get("pageSize"), 20)
		items, total, err := h.kb.ListArticles(kb.ListQuery{
			Query:           strings.TrimSpace(r.URL.Query().Get("q")),
			Status:          strings.TrimSpace(r.URL.Query().Get("status")),
			Severity:        strings.TrimSpace(r.URL.Query().Get("severity")),
			Tag:             strings.TrimSpace(r.URL.Query().Get("tag")),
			Page:            page,
			PageSize:        pageSize,
			IncludeArchived: parseBoolQuery(r, "includeArchived"),
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"items":    items,
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		})
		return
	case http.MethodPost:
		var req struct {
			Title      string   `json:"title"`
			Summary    string   `json:"summary"`
			Category   string   `json:"category"`
			Severity   string   `json:"severity"`
			Content    string   `json:"content"`
			Tags       []string `json:"tags"`
			CreatedBy  string   `json:"createdBy"`
			ChangeNote string   `json:"changeNote"`
			SourceType string   `json:"sourceType"`
			SourceRef  string   `json:"sourceRef"`
			RefTitle   string   `json:"refTitle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		article, err := h.kb.CreateArticle(kb.CreateArticleInput{
			Title:      req.Title,
			Summary:    req.Summary,
			Category:   req.Category,
			Severity:   req.Severity,
			Content:    req.Content,
			Tags:       req.Tags,
			CreatedBy:  req.CreatedBy,
			ChangeNote: req.ChangeNote,
			SourceType: req.SourceType,
			SourceRef:  req.SourceRef,
			RefTitle:   req.RefTitle,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"article": article,
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

// kbArticleByID 通过路径段分发详情 更新 审批与回滚动作
// 这里显式拆分 action 分支 防止不同动作共享参数时出现歧义
func (h *handler) kbArticleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/kb/articles/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "article id required"})
		return
	}
	parts := strings.Split(path, "/")
	articleID := strings.TrimSpace(parts[0])
	if articleID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "article id required"})
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			article, err := h.kb.GetArticle(articleID)
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		case http.MethodPut:
			var req struct {
				Title      string   `json:"title"`
				Summary    string   `json:"summary"`
				Category   string   `json:"category"`
				Severity   string   `json:"severity"`
				Content    string   `json:"content"`
				Tags       []string `json:"tags"`
				UpdatedBy  string   `json:"updatedBy"`
				ChangeNote string   `json:"changeNote"`
				SourceType string   `json:"sourceType"`
				SourceRef  string   `json:"sourceRef"`
				RefTitle   string   `json:"refTitle"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			article, err := h.kb.UpdateArticle(articleID, kb.UpdateArticleInput{
				Title:      req.Title,
				Summary:    req.Summary,
				Category:   req.Category,
				Severity:   req.Severity,
				Content:    req.Content,
				Tags:       req.Tags,
				UpdatedBy:  req.UpdatedBy,
				ChangeNote: req.ChangeNote,
				SourceType: req.SourceType,
				SourceRef:  req.SourceRef,
				RefTitle:   req.RefTitle,
			})
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
	}

	if len(parts) == 2 && r.Method == http.MethodPost {
		action := strings.TrimSpace(parts[1])
		switch action {
		case "submit", "approve", "reject", "archive":
			start := time.Now()
			var req struct {
				Operator string `json:"operator"`
				Comment  string `json:"comment"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			article, err := h.kb.ApplyAction(articleID, action, req.Operator, req.Comment)
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			metrics.Global().ObserveKBReviewLatency(time.Since(start))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		case "rollback":
			start := time.Now()
			var req struct {
				TargetVersion int    `json:"targetVersion"`
				Operator      string `json:"operator"`
				Comment       string `json:"comment"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			article, err := h.kb.RollbackArticle(articleID, req.TargetVersion, req.Operator, req.Comment)
			if err != nil {
				status := http.StatusBadRequest
				if err == kb.ErrNotFound {
					status = http.StatusNotFound
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			metrics.Global().ObserveKBReviewLatency(time.Since(start))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "article": article})
			return
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unsupported action"})
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
}

func (h *handler) kbSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	var req struct {
		Query           string `json:"query"`
		Limit           int    `json:"limit"`
		IncludeArchived bool   `json:"includeArchived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	items, err := h.kb.Search(req.Query, req.Limit, req.IncludeArchived)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics.Global().ObserveKBSearch(len(items))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
	})
}

func (h *handler) kbPendingReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	items, err := h.kb.PendingReviews(limit)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
	})
}

func (h *handler) kbGates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"gates": kb.DefaultQualityGates(),
	})
}

func (h *handler) kbImportDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h.kb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge base is not ready"})
		return
	}
	var req struct {
		Path     string `json:"path"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	result, err := h.kb.ImportDocs(req.Path, req.Operator)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": result,
	})
}
