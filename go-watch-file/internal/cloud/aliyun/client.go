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
		return nil, fmt.Errorf("阿里云 AccessKey 未配置，请设置 ALIYUN_ACCESS_KEY_ID 和 ALIYUN_ACCESS_KEY_SECRET")
	}
	if config.Region == "" && len(config.Regions) == 0 {
		return nil, fmt.Errorf("阿里云地域未配置，请设置 ALIYUN_REGION 或 ALIYUN_REGIONS")
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
	req.Dimensions = fmt.Sprintf(`[{"instanceId":%q}]`, instanceID)
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

func mapInstance(item ecs.Instance, fallbackRegion string) Instance {
	region := firstNonEmpty(item.RegionId, fallbackRegion)
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
		PublicIPs:   compactStrings(append([]string{}, item.PublicIpAddress.IpAddress...)),
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
