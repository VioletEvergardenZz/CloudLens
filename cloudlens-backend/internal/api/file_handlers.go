// 本文件用于文件入云扩展接口。
// 文件职责：处理自动上传开关、手动上传、日志读取与日志搜索。

package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/pathutil"
)

// toggleAutoUpload 用于切换自动上传开关并返回最新状态
func (h *handler) toggleAutoUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Path    string `json:"path"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	state := h.fs.State()
	if state == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime state not ready"})
		return
	}
	state.SetAutoUpload(req.Path, req.Enabled)
	h.invalidateDashboardCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"path":   req.Path,
		"status": req.Enabled,
	})
}

// manualUpload 用于校验并触发手动上传请求
func (h *handler) manualUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	cleanedPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(req.Path)))
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || strings.TrimSpace(cfg.WatchDir) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "watch dir not configured"})
		return
	}
	watchDirs := pathutil.SplitWatchDirs(cfg.WatchDir)
	if len(watchDirs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "watch dir not configured"})
		return
	}
	if _, _, err := pathutil.RelativePathAny(watchDirs, cleanedPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	info, err := os.Stat(cleanedPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is a directory"})
		return
	}
	if err := h.fs.EnqueueManualUpload(cleanedPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.invalidateDashboardCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": cleanedPath,
	})
}

// fileLog 用于读取或检索文件日志内容
func (h *handler) fileLog(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Path          string `json:"path"`
		Query         string `json:"query"`
		Limit         int    `json:"limit"`
		CaseSensitive bool   `json:"caseSensitive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	cleanedPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(req.Path)))
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || strings.TrimSpace(cfg.WatchDir) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "watch dir not configured"})
		return
	}
	watchDirs := pathutil.SplitWatchDirs(cfg.WatchDir)
	if len(watchDirs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "watch dir not configured"})
		return
	}
	if _, _, err := pathutil.RelativePathAny(watchDirs, cleanedPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	info, err := os.Stat(cleanedPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is a directory"})
		return
	}
	// query 为空时走 tail 模式，否则走全文检索
	query := strings.TrimSpace(req.Query)
	if query != "" {
		lines, truncated, err := searchFileLogLines(cleanedPath, query, req.Limit, req.CaseSensitive)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"mode":      "search",
			"query":     query,
			"matched":   len(lines),
			"truncated": truncated,
			"lines":     lines,
		})
		return
	}
	lines, err := readFileLogLines(cleanedPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"mode":  "tail",
		"lines": lines,
	})
}

// 读取目标文件内容
func readFileLogLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()
	var data []byte
	if size > maxFileLogBytes {
		start := size - maxFileLogBytes
		buf := make([]byte, maxFileLogBytes)
		n, err := file.ReadAt(buf, start)
		if err != nil && err != io.EOF {
			return nil, err
		}
		data = buf[:n]
	} else {
		data, err = io.ReadAll(file)
		if err != nil {
			return nil, err
		}
	}

	if len(data) == 0 {
		return []string{}, nil
	}
	if !isTextData(data) {
		return nil, fmt.Errorf("仅支持文本文件")
	}

	lines := strings.Split(string(data), "\n")
	if size > maxFileLogBytes && len(lines) > 1 {
		lines = lines[1:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	if maxFileLogLines > 0 && len(lines) > maxFileLogLines {
		lines = lines[len(lines)-maxFileLogLines:]
	}
	return lines, nil
}

// searchFileLogLines 搜索文件内容并返回匹配行
func searchFileLogLines(path, query string, limit int, caseSensitive bool) ([]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	// 先做文本判定，避免扫描二进制文件
	if err := ensureTextFile(file); err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > maxFileSearchLines {
		limit = maxFileSearchLines
	}
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []string{}, false, nil
	}
	normalizedQuery := trimmedQuery
	if !caseSensitive {
		normalizedQuery = strings.ToLower(trimmedQuery)
	}

	// 使用 Scanner 按行扫描，避免一次性读入大文件
	capHint := limit
	if capHint > 64 {
		capHint = 64
	}
	matches := make([]string, 0, capHint)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFileSearchLineBytes)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		haystack := line
		if !caseSensitive {
			haystack = strings.ToLower(line)
		}
		// 包含关键字即视为命中
		if strings.Contains(haystack, normalizedQuery) {
			matches = append(matches, line)
			if len(matches) >= limit {
				return matches, true, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return matches, false, nil
}

// ensureTextFile 用于快速判断是否为文本文件
func ensureTextFile(file *os.File) error {
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	if !isTextData(buf[:n]) {
		return fmt.Errorf("仅支持文本文件")
	}
	// 重置到文件开头，避免影响后续扫描
	_, err = file.Seek(0, io.SeekStart)
	return err
}

// 简单判断是否是文本数据
func isTextData(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}
