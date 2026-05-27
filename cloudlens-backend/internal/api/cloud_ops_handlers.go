// 本文件用于云资产运维体检接口。
// 文件职责：只保留 HTTP 入口，把诊断、风险、报告和运行检查逻辑交给同组文件。
// 边界与容错：体检接口不主动调用云厂商接口，只读取本地状态，避免体检本身放大云 API 抖动。

package api

import (
	"net/http"
	"strings"
	"time"
)

func (h *handler) cloudSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h == nil || h.cloudStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "云账号存储未初始化",
			"items": []cloudSnapshotSummary{},
			"total": 0,
		})
		return
	}
	snapshots, err := h.cloudStore.ListResourceSnapshots()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items := make([]cloudSnapshotSummary, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, buildCloudSnapshotSummary(snapshot))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
		"total": len(items),
	})
}

func (h *handler) cloudDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	diagnostics, snapshots, err := h.buildCloudDiagnostics()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          false,
			"error":       err.Error(),
			"diagnostics": []cloudAccountDiagnostic{},
			"snapshots":   []cloudSnapshotSummary{},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"diagnostics": diagnostics,
		"snapshots":   snapshots,
		"total":       len(diagnostics),
	})
}

func (h *handler) cloudRisks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	risks, summary, err := h.buildCloudRisks()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"items":   []cloudRiskItem{},
			"summary": cloudRiskSummary{ByAccount: map[string]int{}, ByCategory: map[string]int{}},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"items":   risks,
		"summary": summary,
		"total":   len(risks),
	})
}

func (h *handler) cloudInspectionReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	report := h.buildCloudInspectionReport()
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "markdown") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderCloudInspectionMarkdown(report)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"report": report,
	})
}

func (h *handler) runtimeChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	checks := h.buildRuntimeChecks()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"status":    summarizeCloudOpsChecks(checks),
		"checks":    checks,
		"checkedAt": formatCloudTime(time.Now().UTC()),
	})
}
