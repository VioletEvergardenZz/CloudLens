// 本文件用于统一资源详情、指标和巡检风险接口。
// 文件职责：让前端逐步从云厂商专属接口迁移到统一资源合同。

package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	aliyuncloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/aliyun"
	huaweicloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/huawei"
)

func (h *handler) cloudResourceByID(w http.ResponseWriter, r *http.Request) {
	resourceID, action, err := parseResourcePath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err := h.rebuildCloudResourceIndexFromSnapshots(); err != nil {
		// 统一资源接口优先返回已有索引，重建失败只作为提示，不放大成 500。
	}
	if action == "" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		resource, err := h.resourceService.Get(resourceID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resource": resource})
		return
	}
	if action == "metrics" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		h.cloudResourceMetrics(w, r, resourceID)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
}

func (h *handler) cloudResourceMetrics(w http.ResponseWriter, r *http.Request, id string) {
	if h == nil || h.resourceService == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "资源索引服务未初始化"})
		return
	}
	resource, err := h.resourceService.Get(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	minutes := parseCloudInt(r.URL.Query().Get("minutes"), 30, 1, 24*60)
	period := r.URL.Query().Get("period")
	switch strings.TrimSpace(resource.Provider) {
	case aliyuncloud.ProviderName:
		h.aliyunResourceMetrics(w, resource, minutes, period)
	case huaweicloud.ProviderName:
		h.huaweiResourceMetrics(w, resource, minutes, period)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "不支持的云平台"})
	}
}

func (h *handler) aliyunResourceMetrics(w http.ResponseWriter, resource *cloudUnifiedResource, minutes int, period string) {
	cfg, _, err := h.cloudStore.AliyunConfig(resource.AccountID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client, err := aliyuncloud.NewClient(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if resource.ResourceType == "rds" {
		metrics, errorsByMetric, err := client.RDSPerformance(resource.ResourceID, resource.Region, resource.Engine, minutes, period)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": humanizeAliyunCloudError(err), "code": classifyAliyunCloudError(err)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"resource": resource,
			"metrics":  metrics,
			"errors":   errorsByMetric,
		})
		return
	}
	metrics := make(map[string]*aliyuncloud.MetricSeries)
	errorsByMetric := make(map[string]string)
	for key, candidates := range buildAliyunOverviewMetricCandidates(resource.ResourceID, firstCloudString(resource.PublicIPs...)) {
		series, metricErr := queryFirstAliyunMetric(client, candidates, resource.ResourceID, resource.Region, minutes, period)
		if metricErr != nil {
			errorsByMetric[key] = humanizeAliyunCloudError(metricErr)
			continue
		}
		metrics[key] = series
	}
	if ecsMetrics, ecsMetricErr := client.InstanceMonitorMetrics(resource.ResourceID, resource.Region, minutes, period); ecsMetricErr == nil {
		for key, series := range ecsMetrics {
			metrics[key] = series
			delete(errorsByMetric, key)
		}
	} else {
		errorsByMetric["ecsMonitor"] = humanizeAliyunCloudError(ecsMetricErr)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"resource": resource,
		"metrics":  metrics,
		"errors":   errorsByMetric,
	})
}

func (h *handler) huaweiResourceMetrics(w http.ResponseWriter, resource *cloudUnifiedResource, minutes int, period string) {
	cfg, _, err := h.cloudStore.HuaweiConfig(resource.AccountID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client, err := huaweicloud.NewClient(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if resource.ResourceType == "rds" {
		metrics, errorsByMetric, err := client.RDSPerformance(resource.ResourceID, resource.NodeID, resource.Region, minutes, period)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": humanizeHuaweiCloudError(err), "code": classifyHuaweiCloudError(err)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"resource": resource,
			"metrics":  metrics,
			"errors":   errorsByMetric,
		})
		return
	}
	metrics := make(map[string]*huaweicloud.MetricSeries)
	errorsByMetric := make(map[string]string)
	for key, candidates := range buildHuaweiOverviewMetricCandidates(resource.ResourceID) {
		series, metricErr := queryFirstHuaweiMetric(client, candidates, resource.ResourceID, resource.Region, minutes, period)
		if metricErr != nil {
			errorsByMetric[key] = humanizeHuaweiCloudError(metricErr)
			continue
		}
		metrics[key] = series
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"resource": resource,
		"metrics":  metrics,
		"errors":   errorsByMetric,
	})
}

func (h *handler) inspectionRisks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := h.rebuildCloudResourceIndexFromSnapshots(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"items":   []any{},
			"summary": map[string]any{"total": 0},
		})
		return
	}
	items, summary, err := h.resourceService.Risks(parseCloudResourceFilter(r))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"items":   []any{},
			"summary": summary,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items, "summary": summary, "total": len(items)})
}

func parseResourcePath(path string) (string, string, error) {
	rest := strings.TrimPrefix(path, "/api/resources/")
	rest = strings.Trim(rest, "/")
	if rest == "" || rest == path {
		return "", "", fmt.Errorf("资源路径不合法")
	}
	parts := strings.Split(rest, "/")
	id, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(id) == "" {
		return "", "", fmt.Errorf("资源 ID 不合法")
	}
	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}
	return id, action, nil
}
