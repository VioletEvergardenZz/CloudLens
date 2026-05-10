// 本文件用于阿里云 ECS 与云监控只读查询
// 文件职责：封装 DescribeInstances 与 DescribeMetricList，避免 API 层直接依赖 SDK 细节
// 边界与容错：只调用查询类 OpenAPI，不提供启动、停止、释放、修改等写操作

package aliyun

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
)

const (
	ProviderName = "aliyun"
	NamespaceECS = "acs_ecs_dashboard"
)

type Client struct {
	config Config
}

func NewClient(config Config) (*Client, error) {
	config.AccessKeyID = strings.TrimSpace(config.AccessKeyID)
	config.AccessKeySecret = strings.TrimSpace(config.AccessKeySecret)
	config.Region = strings.TrimSpace(config.Region)
	config.MetricPeriod = strings.TrimSpace(config.MetricPeriod)
	if config.AccessKeyID == "" || config.AccessKeySecret == "" {
		return nil, fmt.Errorf("阿里云 AccessKey 未配置，请在云账号管理中新增账号，或临时设置 ALIYUN_ACCESS_KEY_ID 和 ALIYUN_ACCESS_KEY_SECRET")
	}
	if config.Region == "" && len(config.Regions) == 0 {
		return nil, fmt.Errorf("阿里云地域未配置，请在云账号管理中填写地域，或临时设置 ALIYUN_REGION/ALIYUN_REGIONS")
	}
	if config.MetricPeriod == "" {
		config.MetricPeriod = "60"
	}
	return &Client{config: config}, nil
}

func (c *Client) ListInstances(regions []string) ([]Instance, error) {
	if c == nil {
		return nil, fmt.Errorf("阿里云客户端未初始化")
	}
	targetRegions := normalizeRegions(regions, c.config)
	out := make([]Instance, 0)
	for _, region := range targetRegions {
		items, err := c.listRegionInstances(region)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RegionID == out[j].RegionID {
			return out[i].Name < out[j].Name
		}
		return out[i].RegionID < out[j].RegionID
	})
	return out, nil
}

func (c *Client) listRegionInstances(region string) ([]Instance, error) {
	client, err := ecs.NewClientWithAccessKey(region, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建 ECS 只读客户端失败: %w", err)
	}
	out := make([]Instance, 0)
	for pageNumber := 1; ; pageNumber++ {
		req := ecs.CreateDescribeInstancesRequest()
		req.PageNumber = requests.Integer(strconv.Itoa(pageNumber))
		req.PageSize = requests.Integer("50")
		resp, err := client.DescribeInstances(req)
		if err != nil {
			return nil, fmt.Errorf("查询 ECS 实例失败 region=%s: %w", region, err)
		}
		for _, item := range resp.Instances.Instance {
			out = append(out, mapInstance(item, region))
		}
		if len(out) >= resp.TotalCount || len(resp.Instances.Instance) == 0 {
			break
		}
	}
	return out, nil
}

func (c *Client) Metric(metricName, instanceID, region string, minutes int, period string) (*MetricSeries, error) {
	return c.MetricWithDimensions(metricName, map[string]string{"instanceId": strings.TrimSpace(instanceID)}, region, minutes, period)
}

func (c *Client) MetricWithDimensions(metricName string, dimensions map[string]string, region string, minutes int, period string) (*MetricSeries, error) {
	if c == nil {
		return nil, fmt.Errorf("阿里云客户端未初始化")
	}
	instanceID := strings.TrimSpace(dimensions["instanceId"])
	if instanceID == "" {
		return nil, fmt.Errorf("instanceId 不能为空")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.config.Region
	}
	if region == "" {
		return nil, fmt.Errorf("查询监控指标必须指定 region")
	}
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		metricName = "cpu_total"
	}
	period = strings.TrimSpace(period)
	if period == "" {
		period = c.config.MetricPeriod
	}
	if minutes <= 0 || minutes > 24*60 {
		minutes = 30
	}
	client, err := cms.NewClientWithAccessKey(region, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建云监控只读客户端失败: %w", err)
	}
	end := time.Now()
	start := end.Add(-time.Duration(minutes) * time.Minute)
	req := cms.CreateDescribeMetricListRequest()
	req.Namespace = NamespaceECS
	req.MetricName = metricName
	req.Period = period
	req.StartTime = strconv.FormatInt(start.UnixMilli(), 10)
	req.EndTime = strconv.FormatInt(end.UnixMilli(), 10)
	req.Dimensions = metricDimensionsJSON(dimensions)
	resp, err := client.DescribeMetricList(req)
	if err != nil {
		return nil, fmt.Errorf("查询云监控指标失败 region=%s instance=%s metric=%s: %w", region, instanceID, metricName, err)
	}
	if !resp.Success && strings.TrimSpace(resp.Code) != "" {
		return nil, fmt.Errorf("云监控返回失败 code=%s message=%s", resp.Code, resp.Message)
	}
	points, err := parseMetricPoints(resp.Datapoints)
	if err != nil {
		return nil, err
	}
	return &MetricSeries{
		InstanceID: instanceID,
		RegionID:   region,
		Namespace:  NamespaceECS,
		MetricName: metricName,
		Period:     firstNonEmpty(resp.Period, period),
		Points:     points,
	}, nil
}

func (c *Client) InstanceMonitorMetrics(instanceID, region string, minutes int, period string) (map[string]*MetricSeries, error) {
	if c == nil {
		return nil, fmt.Errorf("阿里云客户端未初始化")
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("instanceId 不能为空")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.config.Region
	}
	if region == "" {
		return nil, fmt.Errorf("查询 ECS 基础监控必须指定 region")
	}
	periodSeconds := parsePositiveInt(firstNonEmpty(period, c.config.MetricPeriod), 60)
	if minutes <= 0 || minutes > 24*60 {
		minutes = 30
	}
	client, err := ecs.NewClientWithAccessKey(region, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建 ECS 只读客户端失败: %w", err)
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(minutes) * time.Minute)
	req := ecs.CreateDescribeInstanceMonitorDataRequest()
	req.InstanceId = instanceID
	req.Period = requests.Integer(strconv.Itoa(periodSeconds))
	req.StartTime = start.Format(time.RFC3339)
	req.EndTime = end.Format(time.RFC3339)
	resp, err := client.DescribeInstanceMonitorData(req)
	if err != nil {
		return nil, fmt.Errorf("查询 ECS 基础监控失败 region=%s instance=%s: %w", region, instanceID, err)
	}
	items := append([]ecs.InstanceMonitorData{}, resp.MonitorData.InstanceMonitorData...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].TimeStamp < items[j].TimeStamp
	})
	out := map[string]*MetricSeries{
		"cpu":               newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.CPU", period),
		"internetIn":        newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.InternetRX", period),
		"internetOut":       newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.InternetTX", period),
		"intranetIn":        newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.IntranetRX", period),
		"intranetOut":       newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.IntranetTX", period),
		"internetBandwidth": newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.InternetBandwidth", period),
		"intranetBandwidth": newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.IntranetBandwidth", period),
		"diskReadBps":       newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.BPSRead", period),
		"diskWriteBps":      newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.BPSWrite", period),
	}
	for _, item := range items {
		timestamp := parseAliyunTimeMillis(item.TimeStamp)
		if timestamp == 0 {
			continue
		}
		appendMetricPoint(out["cpu"], timestamp, float64(item.CPU))
		appendMetricPoint(out["internetIn"], timestamp, trafficKbitToBitRate(item.InternetRX, periodSeconds))
		appendMetricPoint(out["internetOut"], timestamp, trafficKbitToBitRate(item.InternetTX, periodSeconds))
		appendMetricPoint(out["intranetIn"], timestamp, trafficKbitToBitRate(item.IntranetRX, periodSeconds))
		appendMetricPoint(out["intranetOut"], timestamp, trafficKbitToBitRate(item.IntranetTX, periodSeconds))
		appendMetricPoint(out["internetBandwidth"], timestamp, float64(item.InternetBandwidth*1000))
		appendMetricPoint(out["intranetBandwidth"], timestamp, float64(item.IntranetBandwidth*1000))
		appendMetricPoint(out["diskReadBps"], timestamp, float64(item.BPSRead))
		appendMetricPoint(out["diskWriteBps"], timestamp, float64(item.BPSWrite))
	}
	return out, nil
}

func newMetricSeries(instanceID, region, metricName, period string) *MetricSeries {
	return &MetricSeries{
		InstanceID: instanceID,
		RegionID:   region,
		Namespace:  "acs_ecs",
		MetricName: metricName,
		Period:     period,
		Points:     []MetricPoint{},
	}
}

func appendMetricPoint(series *MetricSeries, timestamp int64, value float64) {
	if series == nil {
		return
	}
	series.Points = append(series.Points, MetricPoint{Timestamp: timestamp, Value: value})
}

func trafficKbitToBitRate(valueKbit int, periodSeconds int) float64 {
	if periodSeconds <= 0 {
		periodSeconds = 60
	}
	return float64(valueKbit*1000) / float64(periodSeconds)
}

func parseAliyunTimeMillis(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}

func metricDimensionsJSON(dimensions map[string]string) string {
	cleaned := make(map[string]string, len(dimensions))
	for key, value := range dimensions {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		cleaned[key] = value
	}
	if len(cleaned) == 0 {
		return "[]"
	}
	raw, err := json.Marshal([]map[string]string{cleaned})
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func mapInstance(item ecs.Instance, fallbackRegion string) Instance {
	region := firstNonEmpty(item.RegionId, fallbackRegion)
	publicIPs := compactStrings(append([]string{}, item.PublicIpAddress.IpAddress...))
	eipAddress := strings.TrimSpace(item.EipAddress.IpAddress)
	return Instance{
		ID:          item.InstanceId,
		Name:        firstNonEmpty(item.InstanceName, item.HostName, item.Hostname, item.InstanceId),
		HostName:    firstNonEmpty(item.HostName, item.Hostname),
		Provider:    ProviderName,
		RegionID:    region,
		ZoneID:      item.ZoneId,
		Status:      item.Status,
		OSName:      firstNonEmpty(item.OSName, item.OSNameEn),
		OSType:      firstNonEmpty(item.OSType, item.OsType),
		Type:        item.InstanceType,
		CPU:         firstPositive(item.CPU, item.Cpu),
		MemoryMB:    item.Memory,
		PublicIPs:   publicIPs,
		EipAddress:  eipAddress,
		EipID:       strings.TrimSpace(item.EipAddress.AllocationId),
		PrivateIPs:  compactStrings(append([]string{}, item.VpcAttributes.PrivateIpAddress.IpAddress...)),
		VpcID:       item.VpcAttributes.VpcId,
		VSwitchID:   item.VpcAttributes.VSwitchId,
		SecurityIDs: compactStrings(append([]string{}, item.SecurityGroupIds.SecurityGroupId...)),
		CreatedAt:   item.CreationTime,
		ExpiredAt:   item.ExpiredTime,
	}
}

func parseMetricPoints(raw string) ([]MetricPoint, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []MetricPoint{}, nil
	}
	var records []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &records); err != nil {
		return nil, fmt.Errorf("解析云监控数据失败: %w", err)
	}
	points := make([]MetricPoint, 0, len(records))
	for _, record := range records {
		timestamp := numberAsInt64(record["timestamp"])
		value := firstMetricValue(record)
		points = append(points, MetricPoint{
			Timestamp: timestamp,
			Value:     value,
			Raw:       record,
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})
	return points, nil
}

func normalizeRegions(requested []string, config Config) []string {
	regions := make([]string, 0)
	regions = append(regions, requested...)
	regions = append(regions, config.Regions...)
	if strings.TrimSpace(config.Region) != "" {
		regions = append(regions, config.Region)
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(regions))
	for _, region := range regions {
		trimmed := strings.TrimSpace(region)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func firstMetricValue(record map[string]any) float64 {
	for _, key := range []string{"Average", "average", "Value", "value", "Maximum", "maximum", "Minimum", "minimum"} {
		if value, ok := numberAsFloat64(record[key]); ok {
			return value
		}
	}
	return 0
}

func numberAsInt64(value any) int64 {
	if num, ok := numberAsFloat64(value); ok {
		return int64(num)
	}
	return 0
}

func numberAsFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
