// 本文件用于华为云云资产只读接口。
// 文件职责：把华为云 ECS 实例列表和 CES 监控指标暴露给控制台。
// 边界与容错：只调用查询类接口，不承载任何会修改云资源的动作。

package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	huaweicloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/huawei"
	"github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/models"
)

type huaweiMetricCandidate struct {
	Namespace string
	Name      string
	Dimension map[string]string
	Scale     float64
	Unit      string
}

var huaweiCommonMountPoints = []string{"/", "/data", "/home", "/var", "C:", "D:"}

var defaultHuaweiOverviewMetrics = map[string][]huaweiMetricCandidate{
	"cpu": {
		{Namespace: huaweicloud.NamespaceECS, Name: "cpu_util"},
	},
	"memory": {
		{Namespace: huaweicloud.NamespaceECS, Name: "mem_util"},
		{Namespace: huaweicloud.NamespaceAGT, Name: "mem_usedPercent"},
		{Namespace: huaweicloud.NamespaceAGT, Name: "mem_util"},
	},
	"disk": {
		{Namespace: huaweicloud.NamespaceECS, Name: "disk_util_inband"},
		{Namespace: huaweicloud.NamespaceAGT, Name: "disk_usedPercent", Dimension: map[string]string{"mount_point": "/"}},
		{Namespace: huaweicloud.NamespaceAGT, Name: "disk_util", Dimension: map[string]string{"mount_point": "/"}},
	},
	"load1m": {
		{Namespace: huaweicloud.NamespaceAGT, Name: "load_average1"},
		{Namespace: huaweicloud.NamespaceAGT, Name: "load_average_1m"},
	},
	"internetIn": {
		{Namespace: huaweicloud.NamespaceECS, Name: "network_incoming_bytes_aggregate_rate", Scale: 8, Unit: "bit/s"},
	},
	"internetOut": {
		{Namespace: huaweicloud.NamespaceECS, Name: "network_outgoing_bytes_aggregate_rate", Scale: 8, Unit: "bit/s"},
	},
	"intranetIn": {
		{Namespace: huaweicloud.NamespaceECS, Name: "network_incoming_bytes_rate_inband", Scale: 8, Unit: "bit/s"},
	},
	"intranetOut": {
		{Namespace: huaweicloud.NamespaceECS, Name: "network_outgoing_bytes_rate_inband", Scale: 8, Unit: "bit/s"},
	},
	"diskReadBps": {
		{Namespace: huaweicloud.NamespaceECS, Name: "disk_read_bytes_rate"},
		{Namespace: huaweicloud.NamespaceAGT, Name: "disk_read_bytes_rate"},
	},
	"diskWriteBps": {
		{Namespace: huaweicloud.NamespaceECS, Name: "disk_write_bytes_rate"},
		{Namespace: huaweicloud.NamespaceAGT, Name: "disk_write_bytes_rate"},
	},
}

// cloudHuaweiInstances 返回当前只读账号下的华为云 ECS 实例列表。
// regions 参数可用于临时限定地域，未传时使用账号或 HUAWEI_REGIONS/HUAWEI_REGION 配置。
func (h *handler) cloudHuaweiInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	client, account, err := h.newHuaweiCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	regions := splitCloudList(firstCloudQuery(r, "regions", "region"))
	items, err := client.ListInstances(regions)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": humanizeHuaweiCloudError(err),
			"code":  classifyHuaweiCloudError(err),
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
		"provider":  huaweicloud.ProviderName,
		"items":     items,
		"total":     len(items),
	})
}

// cloudHuaweiMetrics 返回单台华为云 ECS 的一个 CES 指标序列。
func (h *handler) cloudHuaweiMetrics(w http.ResponseWriter, r *http.Request) {
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
	client, account, err := h.newHuaweiCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	namespace := firstCloudQuery(r, "namespace")
	if strings.TrimSpace(namespace) == "" {
		namespace = huaweicloud.NamespaceECS
	}
	metricName := firstCloudQuery(r, "metric", "metricName")
	if strings.TrimSpace(metricName) == "" {
		metricName = "cpu_util"
	}
	series, err := client.MetricWithDimensions(
		namespace,
		metricName,
		map[string]string{"instance_id": instanceID},
		firstCloudQuery(r, "region", "regionId"),
		minutes,
		query.Get("period"),
	)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": humanizeHuaweiCloudError(err),
			"code":  classifyHuaweiCloudError(err),
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
		"provider":  huaweicloud.ProviderName,
		"series":    series,
	})
}

// cloudHuaweiOverview 返回单台华为云 ECS 的常用监控指标集合。
// 单个指标无数据或查询失败不影响其它指标，便于控制台先展示可用数据。
func (h *handler) cloudHuaweiOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	query := r.URL.Query()
	instanceID := firstCloudQuery(r, "instanceId", "instanceID", "id")
	region := firstCloudQuery(r, "region", "regionId")
	if strings.TrimSpace(instanceID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "instanceId 不能为空"})
		return
	}
	minutes := parseCloudInt(query.Get("minutes"), 30, 1, 24*60)
	client, account, err := h.newHuaweiCloudClientFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	metrics := make(map[string]*huaweicloud.MetricSeries, len(defaultHuaweiOverviewMetrics))
	errorsByMetric := make(map[string]string)
	for key, candidates := range buildHuaweiOverviewMetricCandidates(instanceID) {
		series, metricErr := queryFirstHuaweiMetric(client, candidates, instanceID, region, minutes, query.Get("period"))
		if metricErr != nil {
			errorsByMetric[key] = humanizeHuaweiCloudError(metricErr)
			continue
		}
		metrics[key] = series
	}
	availableMetricCount := countHuaweiMetricSeries(metrics)
	status := "ok"
	message := ""
	if availableMetricCount == 0 {
		status = "no_metric_data"
		message = "未查询到华为云 ECS 监控数据，请检查 CES 只读权限、实例地域和监控采样状态"
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
		"provider":             huaweicloud.ProviderName,
		"status":               status,
		"message":              message,
		"availableMetricCount": availableMetricCount,
		"metrics":              metrics,
		"errors":               errorsByMetric,
	})
}

func countHuaweiMetricSeries(metrics map[string]*huaweicloud.MetricSeries) int {
	count := 0
	for _, series := range metrics {
		if series != nil && len(series.Points) > 0 {
			count++
		}
	}
	return count
}

func buildHuaweiOverviewMetricCandidates(instanceID string) map[string][]huaweiMetricCandidate {
	out := make(map[string][]huaweiMetricCandidate, len(defaultHuaweiOverviewMetrics))
	instanceID = strings.TrimSpace(instanceID)
	for key, candidates := range defaultHuaweiOverviewMetrics {
		out[key] = make([]huaweiMetricCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if len(candidate.Dimension) == 0 {
				candidate.Dimension = map[string]string{"instance_id": instanceID}
			}
			out[key] = append(out[key], candidate)
		}
	}
	return out
}

func queryFirstHuaweiMetric(client *huaweicloud.Client, candidates []huaweiMetricCandidate, instanceID, region string, minutes int, period string) (*huaweicloud.MetricSeries, error) {
	var firstErr error
	var firstEmpty *huaweicloud.MetricSeries
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Name) == "" {
			continue
		}
		namespace := strings.TrimSpace(candidate.Namespace)
		if namespace == "" {
			namespace = huaweicloud.NamespaceECS
		}
		dimensions := candidate.Dimension
		if len(dimensions) == 0 {
			dimensions = map[string]string{"instance_id": instanceID}
		}
		series, err := client.MetricWithDimensions(namespace, candidate.Name, dimensions, region, minutes, period)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		series = scaleHuaweiMetricSeries(series, candidate.Scale)
		series = annotateHuaweiMetricSeries(series, candidate.Unit)
		if series != nil && len(series.Points) > 0 {
			return series, nil
		}
		if isHuaweiDiskMountMetric(namespace, candidate.Name) && dimensions["mount_point"] == "/" {
			mountSeries, mountErr := queryFirstHuaweiMountMetric(client, namespace, candidate.Name, instanceID, region, minutes, period, candidate.Scale)
			if mountErr != nil && firstErr == nil {
				firstErr = mountErr
			}
			if mountSeries != nil && len(mountSeries.Points) > 0 {
				return mountSeries, nil
			}
			if firstEmpty == nil && mountSeries != nil {
				firstEmpty = mountSeries
			}
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

func queryFirstHuaweiMountMetric(client *huaweicloud.Client, namespace, metricName, instanceID, region string, minutes int, period string, scale float64) (*huaweicloud.MetricSeries, error) {
	mountPoints, err := huaweiMetricMountPoints(client, namespace, metricName, instanceID, region)
	if err != nil {
		mountPoints = huaweiCommonMountPoints
	}
	var firstErr error
	var firstEmpty *huaweicloud.MetricSeries
	for _, mountPoint := range mountPoints {
		dimensions := map[string]string{
			"instance_id": instanceID,
			"mount_point": strings.TrimSpace(mountPoint),
		}
		series, metricErr := client.MetricWithDimensions(namespace, metricName, dimensions, region, minutes, period)
		if metricErr != nil {
			if firstErr == nil {
				firstErr = metricErr
			}
			continue
		}
		series = scaleHuaweiMetricSeries(series, scale)
		series = annotateHuaweiMetricSeries(series, "")
		if series != nil {
			series.SubKey = strings.TrimSpace(mountPoint)
			if len(series.Points) > 0 {
				return series, nil
			}
			if firstEmpty == nil {
				firstEmpty = series
			}
		}
	}
	if firstEmpty != nil {
		return firstEmpty, nil
	}
	return nil, firstErr
}

func huaweiMetricMountPoints(client *huaweicloud.Client, namespace, metricName, instanceID, region string) ([]string, error) {
	dimensions, err := client.MetricDimensions(namespace, metricName, map[string]string{"instance_id": instanceID}, region)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(dimensions))
	seen := make(map[string]struct{}, len(dimensions))
	for _, item := range dimensions {
		mountPoint := strings.TrimSpace(firstCloudString(item["mount_point"], item["disk_name"], item["device"]))
		if mountPoint == "" {
			continue
		}
		if _, ok := seen[mountPoint]; ok {
			continue
		}
		seen[mountPoint] = struct{}{}
		out = append(out, mountPoint)
	}
	if len(out) == 0 {
		return huaweiCommonMountPoints, nil
	}
	return out, nil
}

func isHuaweiDiskMountMetric(namespace, metricName string) bool {
	if strings.TrimSpace(namespace) != huaweicloud.NamespaceAGT {
		return false
	}
	switch strings.TrimSpace(metricName) {
	case "disk_usedPercent", "disk_util":
		return true
	default:
		return false
	}
}

func firstCloudString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func scaleHuaweiMetricSeries(series *huaweicloud.MetricSeries, scale float64) *huaweicloud.MetricSeries {
	if series == nil || scale == 0 || scale == 1 {
		return series
	}
	if series.Unit == "Byte/s" || series.Unit == "byte/s" {
		series.Unit = "bit/s"
	}
	for index := range series.Points {
		series.Points[index].Value *= scale
	}
	return series
}

func annotateHuaweiMetricSeries(series *huaweicloud.MetricSeries, fallbackUnit string) *huaweicloud.MetricSeries {
	if series == nil {
		return nil
	}
	fallbackUnit = strings.TrimSpace(fallbackUnit)
	if fallbackUnit != "" {
		series.Unit = fallbackUnit
		return series
	}
	return series
}

func (h *handler) newHuaweiCloudClientFromRequest(r *http.Request) (*huaweicloud.Client, *cloudAccountRecord, error) {
	accountIDRaw := firstCloudQuery(r, "accountId", "accountID")
	if strings.TrimSpace(accountIDRaw) == "" {
		client, err := h.newHuaweiCloudClient()
		return client, nil, err
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(accountIDRaw), 10, 64)
	if err != nil || accountID <= 0 {
		return nil, nil, fmt.Errorf("accountId 不合法")
	}
	if h == nil || h.cloudStore == nil {
		return nil, nil, fmt.Errorf("云账号存储未初始化")
	}
	cfg, account, err := h.cloudStore.HuaweiConfig(accountID)
	if err != nil {
		return nil, nil, err
	}
	client, err := huaweicloud.NewClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	return client, account, nil
}

func (h *handler) newHuaweiCloudClient() (*huaweicloud.Client, error) {
	cfg := huaweiCloudConfig(h.cfg)
	return huaweicloud.NewClient(cfg)
}

func huaweiCloudConfig(cfg *models.Config) huaweicloud.Config {
	if cfg == nil {
		return huaweicloud.Config{}
	}
	return huaweicloud.Config{
		AccessKeyID:     strings.TrimSpace(cfg.HuaweiAccessKeyID),
		AccessKeySecret: strings.TrimSpace(cfg.HuaweiAccessKeySecret),
		ProjectID:       strings.TrimSpace(cfg.HuaweiProjectID),
		Region:          strings.TrimSpace(cfg.HuaweiRegion),
		Regions:         splitCloudList(cfg.HuaweiRegions),
		MetricPeriod:    strings.TrimSpace(cfg.HuaweiMetricPeriod),
	}
}

func classifyHuaweiCloudError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "apigw.0301") || strings.Contains(msg, "signature") || strings.Contains(msg, "ak") || strings.Contains(msg, "sk"):
		return "HUAWEI_CREDENTIAL_INVALID"
	case strings.Contains(msg, "forbidden") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "permission") || strings.Contains(msg, "iam"):
		return "HUAWEI_PERMISSION_DENIED"
	case strings.Contains(msg, "listserversdetails") || strings.Contains(msg, "ecs"):
		return "HUAWEI_ECS_PERMISSION_OR_REGION_ERROR"
	case strings.Contains(msg, "showmetricdata") || strings.Contains(msg, "ces"):
		return "HUAWEI_CES_PERMISSION_OR_METRIC_ERROR"
	default:
		return "HUAWEI_API_ERROR"
	}
}

func humanizeHuaweiCloudError(err error) string {
	if err == nil {
		return ""
	}
	switch classifyHuaweiCloudError(err) {
	case "HUAWEI_CREDENTIAL_INVALID":
		return "华为云 AccessKey 无效或 Secret 不匹配，请在云账号管理里重新保存凭据"
	case "HUAWEI_PERMISSION_DENIED":
		raw := strings.ToLower(err.Error())
		if strings.Contains(raw, "showmetricdata") || strings.Contains(raw, "ces") {
			return "当前华为云账号缺少云监控 CES 只读权限，请补充 CES 监控读取权限"
		}
		if strings.Contains(raw, "listserversdetails") || strings.Contains(raw, "ecs") {
			return "当前华为云账号缺少 ECS 只读权限，请补充 ECS 查询权限"
		}
		return "当前华为云账号权限不足，请确认 ECS 只读和 CES 云监控只读权限"
	case "HUAWEI_ECS_PERMISSION_OR_REGION_ERROR":
		return "华为云 ECS 实例读取失败，请检查 ECS 只读权限和账号地域配置"
	case "HUAWEI_CES_PERMISSION_OR_METRIC_ERROR":
		return "华为云云监控指标读取失败，请检查 CES 权限、实例地域和监控采样状态"
	default:
		return err.Error()
	}
}
