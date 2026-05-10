// 本文件用于云账号配置 API。
// 文件职责：提供阿里云 ECS 账号的新增、列表、删除和权限测试入口。
// 边界与容错：接口只管理查询凭据，不执行云资源写操作。

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	aliyuncloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/aliyun"
)

type cloudAccountCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func (h *handler) cloudAccountsHandler(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.cloudStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "云账号存储未初始化"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.cloudStore.List()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"items": sanitizeCloudAccounts(items),
			"total": len(items),
		})
	case http.MethodPost:
		var input cloudAccountUpsert
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		item, err := h.cloudStore.Create(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"account": sanitizeCloudAccount(*item),
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *handler) cloudAccountByIDHandler(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.cloudStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "云账号存储未初始化"})
		return
	}
	id, action, err := parseCloudAccountPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if action == "test" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		h.cloudAccountTest(w, id)
		return
	}
	if action != "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := h.cloudStore.Get(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"account": sanitizeCloudAccount(*item),
		})
	case http.MethodPut:
		var input cloudAccountUpsert
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		item, err := h.cloudStore.Update(id, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"account": sanitizeCloudAccount(*item),
		})
	case http.MethodDelete:
		if err := h.cloudStore.Delete(id); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *handler) cloudAccountTest(w http.ResponseWriter, id int64) {
	cfg, account, err := h.cloudStore.AliyunConfig(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client, err := aliyuncloud.NewClient(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	checkedAt := time.Now().UTC()
	checks := make([]cloudAccountCheck, 0, 2)
	instances, err := client.ListInstances(nil)
	if err != nil {
		message := humanizeAliyunCloudError(err)
		checks = append(checks, cloudAccountCheck{
			Name:    "ECS 实例读取",
			Status:  "error",
			OK:      false,
			Message: message,
		})
		_ = h.cloudStore.UpdateCheck(id, "error", message, checkedAt)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"provider":  aliyuncloud.ProviderName,
			"account":   sanitizeCloudAccount(*account),
			"checks":    checks,
			"checkedAt": formatCloudTime(checkedAt),
		})
		return
	}
	ecsMessage := fmt.Sprintf("ECS 只读正常，发现 %d 台实例", len(instances))
	checks = append(checks, cloudAccountCheck{
		Name:    "ECS 实例读取",
		Status:  "ok",
		OK:      true,
		Message: ecsMessage,
	})

	status := "ok"
	message := ecsMessage
	if len(instances) == 0 {
		status = "warning"
		message = "ECS 权限可用，但当前地域未发现实例"
		checks = append(checks, cloudAccountCheck{
			Name:    "云监控读取",
			Status:  "skipped",
			OK:      false,
			Message: "未发现 ECS 实例，无法验证云监控指标",
		})
	} else {
		first := instances[0]
		series, metricErr := client.Metric("cpu_total", first.ID, first.RegionID, 30, account.MetricPeriod)
		if metricErr != nil {
			status = "warning"
			message = humanizeAliyunCloudError(metricErr)
			checks = append(checks, cloudAccountCheck{
				Name:    "云监控读取",
				Status:  "error",
				OK:      false,
				Message: message,
			})
		} else if series == nil || len(series.Points) == 0 {
			status = "warning"
			message = "云监控接口可访问，但暂未返回 CPU 指标，请检查实例监控插件、地域和指标延迟"
			checks = append(checks, cloudAccountCheck{
				Name:    "云监控读取",
				Status:  "empty",
				OK:      false,
				Message: message,
			})
		} else {
			checks = append(checks, cloudAccountCheck{
				Name:    "云监控读取",
				Status:  "ok",
				OK:      true,
				Message: "云监控 CPU 指标读取正常",
			})
		}
	}
	if err := h.cloudStore.UpdateCheck(id, status, message, checkedAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updated, _ := h.cloudStore.Get(id)
	if updated == nil {
		updated = account
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        status == "ok",
		"provider":  aliyuncloud.ProviderName,
		"account":   sanitizeCloudAccount(*updated),
		"checks":    checks,
		"checkedAt": formatCloudTime(checkedAt),
	})
}

func parseCloudAccountPath(path string) (int64, string, error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/cloud/accounts/"), "/")
	if trimmed == "" || strings.HasPrefix(trimmed, "/api/") {
		return 0, "", fmt.Errorf("云账号 ID 不能为空")
	}
	parts := strings.Split(trimmed, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", fmt.Errorf("云账号 ID 不合法")
	}
	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}
	return id, action, nil
}

func sanitizeCloudAccounts(items []cloudAccountRecord) []cloudAccountRecord {
	out := make([]cloudAccountRecord, 0, len(items))
	for _, item := range items {
		out = append(out, sanitizeCloudAccount(item))
	}
	return out
}

func sanitizeCloudAccount(item cloudAccountRecord) cloudAccountRecord {
	item.AccessKeyID = ""
	item.AccessKeySecretCipher = ""
	if item.Provider == "" {
		item.Provider = aliyuncloud.ProviderName
	}
	return item
}
