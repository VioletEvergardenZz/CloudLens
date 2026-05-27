// 本文件用于本机系统资源监控接口。
// 文件职责：提供系统快照与进程终止能力，保持历史扩展能力隔离。

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/sysinfo"
)

// systemDashboard 用于返回系统资源监控数据
func (h *handler) systemDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || !cfg.SystemResourceEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "系统资源未启用，请先在控制台配置开启"})
		return
	}
	if h.sys == nil {
		h.sys = sysinfo.NewCollector(sysinfo.Options{})
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	includeProcesses := mode != "lite" && mode != "light"
	includeProcessEnv := parseBoolQuery(r, "includeEnv")
	limit := -1
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = val
	}
	// mode=lite 时跳过进程列表采集，limit 可限制返回的进程数量
	snapshot, err := h.sys.Snapshot(sysinfo.SnapshotOptions{
		IncludeProcesses: includeProcesses,
		ProcessLimit:     limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !includeProcessEnv {
		for i := range snapshot.SystemProcesses {
			snapshot.SystemProcesses[i].Env = []string{}
		}
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// systemTerminate 用于终止指定 PID 的进程
func (h *handler) systemTerminate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || !cfg.SystemResourceEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "system resource console disabled"})
		return
	}

	var req struct {
		PID   int32 `json:"pid"`
		Force bool  `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
		return
	}
	if req.PID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pid"})
		return
	}
	if req.PID == int32(os.Getpid()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refuse to terminate current api process"})
		return
	}

	result, err := sysinfo.TerminateProcess(req.PID, req.Force)
	if err != nil {
		switch {
		case errors.Is(err, sysinfo.ErrInvalidPID):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pid"})
		case errors.Is(err, sysinfo.ErrProcessNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "process not found"})
		case errors.Is(err, sysinfo.ErrTerminatePermissionDenied):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": result,
	})
}
