// 本文件用于阿里云云资产只读接口
// 文件职责：把 ECS 实例列表和云监控指标暴露给控制台
// 边界与容错：只调用 Describe* 查询类接口，不承载任何会修改云资源的动作

package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	aliyuncloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/aliyun"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

type aliyunMetricCandidate struct {
	Name      string
	Dimension map[string]string
}

var defaultAliyunOverviewMetrics = map[string][]aliyunMetricCandidate{
	"cpu": {
		{Name: "CPUUtilization"},
		{Name: "cpu_total"},
	},
	"memory": {
		{Name: "memory_usedutilization"},
	},
	"disk": {
		{Name: "diskusage_utilization"},
	},
	"load1m": {
		{Name: "load_1m"},
	},
	"internetIn": {
		{Name: "InternetInRate"},
	},
	"internetOut": {
		{Name: "InternetOutRate"},
	},
	"intranetIn": {
		{Name: "IntranetInRate"},
	},
	"intranetOut": {
		{Name: "IntranetOutRate"},
	},
	"diskReadBps": {
		{Name: "DiskReadBPS"},
		{Name: "disk_readbytes"},
	},
	"diskWriteBps": {
		{Name: "DiskWriteBPS"},
		{Name: "disk_writebytes"},
	},
}

// cloudAliyunInstances 返回当前只读 RAM 账号下的 ECS 实例列表。
// regions 参数仅作为调试限定范围，未传时会自动发现并采集全部可见地域。
func (h *handler) cloudAliyunInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	client, account, err := h.newAliyunCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	regions := splitCloudList(firstCloudQuery(r, "regions", "region"))
	items, err := client.ListInstances(regions)
	if err != nil {
		if h.writeCloudSnapshotFallback(w, account, aliyuncloud.ProviderName, "ecs", err, humanizeAliyunCloudError, classifyAliyunCloudError) {
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": humanizeAliyunCloudError(err),
			"code":  classifyAliyunCloudError(err),
		})
		return
	}
	h.saveCloudResourceSnapshot(account, "ecs", items)
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId": cloudAccountID(account),
		"ok":        true,
		"provider":  aliyuncloud.ProviderName,
		"resource":  "ecs",
		"source":    "live",
		"items":     items,
		"total":     len(items),
	})
}

// cloudAliyunRDSInstances 返回当前只读 RAM 账号下的 RDS 实例列表。
// regions 参数仅作为调试限定范围，未传时会自动发现并采集全部可见地域。
// 详情和连接地址按实例做 best-effort 补充，单个详情接口失败不会丢掉基础实例。
func (h *handler) cloudAliyunRDSInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	client, account, err := h.newAliyunCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	regions := splitCloudList(firstCloudQuery(r, "regions", "region"))
	items, err := client.ListRDSInstances(regions)
	if err != nil {
		if h.writeCloudSnapshotFallback(w, account, aliyuncloud.ProviderName, "rds", err, humanizeAliyunCloudError, classifyAliyunCloudError) {
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": humanizeAliyunCloudError(err),
			"code":  classifyAliyunCloudError(err),
		})
		return
	}
	h.saveCloudResourceSnapshot(account, "rds", items)
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId": cloudAccountID(account),
		"ok":        true,
		"provider":  aliyuncloud.ProviderName,
		"resource":  "rds",
		"source":    "live",
		"items":     items,
		"total":     len(items),
	})
}

// cloudAliyunMetrics 返回单台 ECS 的一个云监控指标序列
// 当前仅用于验证真实链路，前端可按需传 metric=cpu_total 等指标名。
func (h *handler) cloudAliyunMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	query := r.URL.Query()
	instanceID := firstCloudQuery(r, "instanceId", "instanceID", "id")
	if strings.TrimSpace(instanceID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "instanceId 不能为空"})
		return
	}
	minutes := parseCloudInt(query.Get("minutes"), 30, 1, 24*60)
	client, account, err := h.newAliyunCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	series, err := client.Metric(
		firstCloudQuery(r, "metric", "metricName"),
		instanceID,
		firstCloudQuery(r, "region", "regionId"),
		minutes,
		query.Get("period"),
	)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": humanizeAliyunCloudError(err),
			"code":  classifyAliyunCloudError(err),
		})
		return
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId": accountID,
		"ok":        true,
		"provider":  aliyuncloud.ProviderName,
		"series":    series,
	})
}

// cloudAliyunRDSOverview 返回单台 RDS 的性能参数集合。
// 阿里云 RDS 单次最多查询 30 个性能参数，客户端内部会分批并把不支持的 Key 降级为局部错误。
func (h *handler) cloudAliyunRDSOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	query := r.URL.Query()
	dbInstanceID := firstCloudQuery(r, "dbInstanceId", "dbInstanceID", "instanceId", "id")
	region := firstCloudQuery(r, "region", "regionId")
	engine := firstCloudQuery(r, "engine")
	if strings.TrimSpace(dbInstanceID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dbInstanceId 不能为空"})
		return
	}
	minutes := parseCloudInt(query.Get("minutes"), 30, 1, 24*60)
	client, account, err := h.newAliyunCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics, errorsByMetric, err := client.RDSPerformance(dbInstanceID, region, engine, minutes, query.Get("period"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": humanizeAliyunCloudError(err),
			"code":  classifyAliyunCloudError(err),
		})
		return
	}
	availableMetricCount := countAliyunMetricSeries(metrics)
	status := "ok"
	message := ""
	if availableMetricCount == 0 {
		status = "no_metric_data"
		message = "未查询到 RDS 性能数据，请检查 AliyunRDSReadOnlyAccess 权限、实例地域和性能参数是否支持"
		if len(errorsByMetric) > 0 {
			status = "metric_error"
			message = firstCloudMapValue(errorsByMetric)
		}
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId":            accountID,
		"ok":                   true,
		"provider":             aliyuncloud.ProviderName,
		"resource":             "rds",
		"status":               status,
		"message":              message,
		"availableMetricCount": availableMetricCount,
		"metrics":              metrics,
		"errors":               errorsByMetric,
	})
}

// cloudAliyunOverview 返回单台 ECS 的常用监控指标集合。
// 单个指标无数据或查询失败不影响其它指标，便于控制台先展示可用数据。
func (h *handler) cloudAliyunOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	query := r.URL.Query()
	instanceID := firstCloudQuery(r, "instanceId", "instanceID", "id")
	region := firstCloudQuery(r, "region", "regionId")
	publicIP := firstCloudQuery(r, "publicIp", "publicIP", "eip")
	if strings.TrimSpace(instanceID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "instanceId 不能为空"})
		return
	}
	minutes := parseCloudInt(query.Get("minutes"), 30, 1, 24*60)
	client, account, err := h.newAliyunCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics := make(map[string]*aliyuncloud.MetricSeries, len(defaultAliyunOverviewMetrics))
	errorsByMetric := make(map[string]string)
	availableMetricCount := 0
	for key, candidates := range buildAliyunOverviewMetricCandidates(instanceID, publicIP) {
		series, metricErr := queryFirstAliyunMetric(client, candidates, instanceID, region, minutes, query.Get("period"))
		if metricErr != nil {
			errorsByMetric[key] = humanizeAliyunCloudError(metricErr)
			continue
		}
		if series != nil && len(series.Points) > 0 {
			availableMetricCount++
		}
		metrics[key] = series
	}
	if ecsMetrics, ecsMetricErr := client.InstanceMonitorMetrics(instanceID, region, minutes, query.Get("period")); ecsMetricErr == nil {
		for key, series := range ecsMetrics {
			if series != nil && len(series.Points) > 0 {
				metrics[key] = series
				delete(errorsByMetric, key)
			}
		}
	} else {
		errorsByMetric["ecsMonitor"] = humanizeAliyunCloudError(ecsMetricErr)
	}
	availableMetricCount = countAliyunMetricSeries(metrics)
	status := "ok"
	message := ""
	if availableMetricCount == 0 {
		status = "no_metric_data"
		message = "未查询到 ECS 监控数据，请检查 AliyunCloudMonitorReadOnlyAccess 权限、实例地域和云监控插件状态"
		if len(errorsByMetric) > 0 {
			status = "metric_error"
			message = firstCloudMapValue(errorsByMetric)
		}
	}
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accountId":            accountID,
		"ok":                   true,
		"provider":             aliyuncloud.ProviderName,
		"status":               status,
		"message":              message,
		"availableMetricCount": availableMetricCount,
		"metrics":              metrics,
		"errors":               errorsByMetric,
	})
}

func countAliyunMetricSeries(metrics map[string]*aliyuncloud.MetricSeries) int {
	count := 0
	for _, series := range metrics {
		if series != nil && len(series.Points) > 0 {
			count++
		}
	}
	return count
}

func buildAliyunOverviewMetricCandidates(instanceID, publicIP string) map[string][]aliyunMetricCandidate {
	out := make(map[string][]aliyunMetricCandidate, len(defaultAliyunOverviewMetrics))
	for key, candidates := range defaultAliyunOverviewMetrics {
		out[key] = append([]aliyunMetricCandidate{}, candidates...)
	}
	instanceID = strings.TrimSpace(instanceID)
	publicIP = strings.TrimSpace(publicIP)
	if publicIP != "" {
		publicDimension := map[string]string{"instanceId": instanceID, "ip": publicIP}
		out["internetIn"] = append([]aliyunMetricCandidate{{Name: "VPC_PublicIP_InternetInRate", Dimension: publicDimension}}, out["internetIn"]...)
		out["internetOut"] = append([]aliyunMetricCandidate{{Name: "VPC_PublicIP_InternetOutRate", Dimension: publicDimension}}, out["internetOut"]...)
	}
	return out
}

func queryFirstAliyunMetric(client *aliyuncloud.Client, candidates []aliyunMetricCandidate, instanceID, region string, minutes int, period string) (*aliyuncloud.MetricSeries, error) {
	var firstErr error
	var firstEmpty *aliyuncloud.MetricSeries
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Name) == "" {
			continue
		}
		dimensions := candidate.Dimension
		if len(dimensions) == 0 {
			dimensions = map[string]string{"instanceId": instanceID}
		}
		series, err := client.MetricWithDimensions(candidate.Name, dimensions, region, minutes, period)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if series != nil && len(series.Points) > 0 {
			return series, nil
		}
		if firstEmpty == nil {
			firstEmpty = series
		}
	}
	if firstEmpty != nil {
		return firstEmpty, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("没有可用的指标候选")
}

func (h *handler) newAliyunCloudClientFromRequest(r *http.Request) (*aliyuncloud.Client, *cloudAccountRecord, error) {
	accountIDRaw := firstCloudQuery(r, "accountId", "accountID")
	if strings.TrimSpace(accountIDRaw) == "" {
		client, err := h.newAliyunCloudClient()
		return client, nil, err
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(accountIDRaw), 10, 64)
	if err != nil || accountID <= 0 {
		return nil, nil, fmt.Errorf("accountId 不合法")
	}
	if h == nil || h.cloudStore == nil {
		return nil, nil, fmt.Errorf("云账号存储未初始化")
	}
	cfg, account, err := h.cloudStore.AliyunConfig(accountID)
	if err != nil {
		return nil, nil, err
	}
	client, err := aliyuncloud.NewClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	return client, account, nil
}

func (h *handler) newAliyunCloudClient() (*aliyuncloud.Client, error) {
	cfg := aliyunCloudConfig(h.cfg)
	return aliyuncloud.NewClient(cfg)
}

func aliyunCloudConfig(cfg *models.Config) aliyuncloud.Config {
	if cfg == nil {
		return aliyuncloud.Config{}
	}
	return aliyuncloud.Config{
		AccessKeyID:     strings.TrimSpace(cfg.AliyunAccessKeyID),
		AccessKeySecret: strings.TrimSpace(cfg.AliyunAccessKeySecret),
		Region:          strings.TrimSpace(cfg.AliyunRegion),
		Regions:         splitCloudList(cfg.AliyunRegions),
		MetricPeriod:    strings.TrimSpace(cfg.AliyunMetricPeriod),
	}
}

func firstCloudQuery(r *http.Request, keys ...string) string {
	if r == nil {
		return ""
	}
	query := r.URL.Query()
	for _, key := range keys {
		value := strings.TrimSpace(query.Get(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func splitCloudList(raw string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func parseCloudInt(raw string, defaultValue, minValue, maxValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return defaultValue
	}
	if value < minValue {
		return minValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func classifyAliyunCloudError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "invalidaccesskeyid") || strings.Contains(msg, "signaturedoesnotmatch") || strings.Contains(msg, "invalid accesskey"):
		return "ALIYUN_CREDENTIAL_INVALID"
	case strings.Contains(msg, "forbidden") || strings.Contains(msg, "nopermission") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "ram"):
		return "ALIYUN_PERMISSION_DENIED"
	case strings.Contains(msg, "describeinstances"):
		return "ALIYUN_ECS_PERMISSION_OR_REGION_ERROR"
	case strings.Contains(msg, "describedbinstances") || strings.Contains(msg, "describedbinstanceattribute") || strings.Contains(msg, "describedbinstancenetinfo"):
		return "ALIYUN_RDS_PERMISSION_OR_REGION_ERROR"
	case strings.Contains(msg, "describedbinstanceperformance") || strings.Contains(msg, "rds 性能"):
		return "ALIYUN_RDS_METRIC_ERROR"
	case strings.Contains(msg, "云监控") || strings.Contains(msg, "describemetric"):
		return "ALIYUN_CMS_PERMISSION_OR_METRIC_ERROR"
	default:
		return "ALIYUN_API_ERROR"
	}
}

func humanizeAliyunCloudError(err error) string {
	if err == nil {
		return ""
	}
	switch classifyAliyunCloudError(err) {
	case "ALIYUN_CREDENTIAL_INVALID":
		return "阿里云 AccessKey 无效或 Secret 不匹配，请在云账号管理里重新保存凭据"
	case "ALIYUN_PERMISSION_DENIED":
		raw := strings.ToLower(err.Error())
		if strings.Contains(raw, "describemetric") || strings.Contains(raw, "云监控") || strings.Contains(raw, "cms") {
			return "当前 RAM 账号缺少云监控只读权限，请添加 AliyunCloudMonitorReadOnlyAccess"
		}
		if strings.Contains(raw, "describeinstances") || strings.Contains(raw, "ecs") {
			return "当前 RAM 账号缺少 ECS 只读权限，请添加 AliyunECSReadOnlyAccess"
		}
		if strings.Contains(raw, "describedb") || strings.Contains(raw, "rds") {
			return "当前 RAM 账号缺少 RDS 只读权限，请添加 AliyunRDSReadOnlyAccess"
		}
		return "当前 RAM 账号权限不足，请确认 ECS、RDS 和云监控只读权限"
	case "ALIYUN_ECS_PERMISSION_OR_REGION_ERROR":
		return "ECS 实例读取失败，请检查 AliyunECSReadOnlyAccess 权限和地域发现状态"
	case "ALIYUN_RDS_PERMISSION_OR_REGION_ERROR":
		return "RDS 实例读取失败，请检查 AliyunRDSReadOnlyAccess 权限和地域发现状态"
	case "ALIYUN_RDS_METRIC_ERROR":
		return "RDS 性能指标读取失败，请检查 AliyunRDSReadOnlyAccess 权限、实例地域和性能参数支持情况"
	case "ALIYUN_CMS_PERMISSION_OR_METRIC_ERROR":
		return "云监控指标读取失败，请检查 AliyunCloudMonitorReadOnlyAccess 权限、实例地域和云监控插件状态"
	default:
		return err.Error()
	}
}

func firstCloudMapValue(values map[string]string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
