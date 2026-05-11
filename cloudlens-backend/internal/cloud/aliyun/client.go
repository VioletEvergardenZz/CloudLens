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
	"github.com/aliyun/alibaba-cloud-sdk-go/services/rds"
)

const (
	ProviderName             = "aliyun"
	NamespaceECS             = "acs_ecs_dashboard"
	NamespaceRDSPerformance  = "rds.DescribeDBInstancePerformance"
	maxRDSPerformanceKeySize = 30

	expirationStatusNormal       = "normal"
	expirationStatusExpiring     = "expiring"
	expirationStatusExpired      = "expired"
	expirationStatusNoExpiration = "no_expiration"
	expirationStatusUnknown      = "unknown"
	expiringSoonDays             = 30
	placeholderExpirationDays    = 20 * 365
	spotStrategyNone             = "NoSpot"
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

func (c *Client) ListRDSInstances(regions []string) ([]RDSInstance, error) {
	if c == nil {
		return nil, fmt.Errorf("阿里云客户端未初始化")
	}
	targetRegions := normalizeRegions(regions, c.config)
	out := make([]RDSInstance, 0)
	for _, region := range targetRegions {
		items, err := c.listRegionRDSInstances(region)
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

func (c *Client) listRegionRDSInstances(region string) ([]RDSInstance, error) {
	client, err := rds.NewClientWithAccessKey(region, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建 RDS 只读客户端失败: %w", err)
	}
	out := make([]RDSInstance, 0)
	for pageNumber := 1; ; pageNumber++ {
		req := rds.CreateDescribeDBInstancesRequest()
		req.PageNumber = requests.Integer(strconv.Itoa(pageNumber))
		req.PageSize = requests.Integer("50")
		resp, err := client.DescribeDBInstances(req)
		if err != nil {
			return nil, fmt.Errorf("查询 RDS 实例失败 region=%s: %w", region, err)
		}
		pageItems := resp.Items.DBInstance
		for _, item := range pageItems {
			instance := mapRDSInstance(item, region)
			if attr, err := describeRDSInstanceAttribute(client, item.DBInstanceId); err == nil {
				mergeRDSInstanceAttribute(&instance, attr)
			} else {
				instance.DetailErrors = append(instance.DetailErrors, fmt.Sprintf("读取 RDS 详情失败: %v", err))
			}
			if endpoints, err := describeRDSInstanceNetInfo(client, item.DBInstanceId); err == nil {
				instance.Endpoints = endpoints
				mergeRDSPrimaryEndpoint(&instance)
			} else {
				instance.DetailErrors = append(instance.DetailErrors, fmt.Sprintf("读取 RDS 连接信息失败: %v", err))
			}
			if usage, err := describeRDSResourceUsage(client, item.DBInstanceId, instance.StorageGB); err == nil {
				instance.ResourceUsage = usage
			} else {
				instance.DetailErrors = append(instance.DetailErrors, fmt.Sprintf("读取 RDS 官方空间用量失败: %v", err))
			}
			out = append(out, instance)
		}
		if len(out) >= resp.TotalRecordCount || len(pageItems) == 0 {
			break
		}
	}
	return out, nil
}

func describeRDSInstanceAttribute(client *rds.Client, instanceID string) (rds.DBInstanceAttribute, error) {
	req := rds.CreateDescribeDBInstanceAttributeRequest()
	req.DBInstanceId = strings.TrimSpace(instanceID)
	resp, err := client.DescribeDBInstanceAttribute(req)
	if err != nil {
		return rds.DBInstanceAttribute{}, err
	}
	if len(resp.Items.DBInstanceAttribute) == 0 {
		return rds.DBInstanceAttribute{}, fmt.Errorf("RDS 未返回实例详情")
	}
	return resp.Items.DBInstanceAttribute[0], nil
}

func describeRDSResourceUsage(client *rds.Client, instanceID string, storageGB int) (*RDSResourceUsage, error) {
	req := rds.CreateDescribeResourceUsageRequest()
	req.DBInstanceId = strings.TrimSpace(instanceID)
	resp, err := client.DescribeResourceUsage(req)
	if err != nil {
		return nil, err
	}
	usage := &RDSResourceUsage{
		DBInstanceID:    strings.TrimSpace(resp.DBInstanceId),
		Engine:          strings.TrimSpace(resp.Engine),
		DiskUsedBytes:   nonNegativeBytes(resp.DiskUsed),
		DataSizeBytes:   nonNegativeBytes(resp.DataSize),
		LogSizeBytes:    nonNegativeBytes(resp.LogSize),
		SQLSizeBytes:    nonNegativeBytes(resp.SQLSize),
		BackupSizeBytes: nonNegativeBytes(resp.BackupSize),
		Source:          "rds.DescribeResourceUsage",
	}
	if usage.DBInstanceID == "" {
		usage.DBInstanceID = strings.TrimSpace(instanceID)
	}
	if storageGB > 0 && usage.DiskUsedBytes > 0 {
		percent := (float64(usage.DiskUsedBytes) / (float64(storageGB) * 1024 * 1024 * 1024)) * 100
		usage.StorageUsagePercent = &percent
	}
	return usage, nil
}

func describeRDSInstanceNetInfo(client *rds.Client, instanceID string) ([]RDSEndpoint, error) {
	req := rds.CreateDescribeDBInstanceNetInfoRequest()
	req.DBInstanceId = strings.TrimSpace(instanceID)
	resp, err := client.DescribeDBInstanceNetInfo(req)
	if err != nil {
		return nil, err
	}
	items := resp.DBInstanceNetInfos.DBInstanceNetInfo
	endpoints := make([]RDSEndpoint, 0, len(items))
	for _, item := range items {
		endpoints = append(endpoints, RDSEndpoint{
			ConnectionString:     strings.TrimSpace(item.ConnectionString),
			Port:                 strings.TrimSpace(item.Port),
			IPAddress:            strings.TrimSpace(item.IPAddress),
			IPType:               strings.TrimSpace(item.IPType),
			ConnectionStringType: strings.TrimSpace(item.ConnectionStringType),
			Availability:         strings.TrimSpace(item.Availability),
			VPCID:                strings.TrimSpace(item.VPCId),
			VSwitchID:            strings.TrimSpace(item.VSwitchId),
		})
	}
	return endpoints, nil
}

func nonNegativeBytes(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return value
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
		Unit:       aliyunMetricUnit(metricName),
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
		"cpu":               newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.CPU", period, "%"),
		"internetIn":        newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.InternetRX", period, "bit/s"),
		"internetOut":       newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.InternetTX", period, "bit/s"),
		"intranetIn":        newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.IntranetRX", period, "bit/s"),
		"intranetOut":       newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.IntranetTX", period, "bit/s"),
		"internetBandwidth": newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.InternetBandwidth", period, "bit/s"),
		"intranetBandwidth": newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.IntranetBandwidth", period, "bit/s"),
		"diskReadBps":       newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.BPSRead", period, "Byte/s"),
		"diskWriteBps":      newMetricSeries(instanceID, region, "ecs.DescribeInstanceMonitorData.BPSWrite", period, "Byte/s"),
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

func (c *Client) RDSPerformance(dbInstanceID, region, engine string, minutes int, period string) (map[string]*MetricSeries, map[string]string, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("阿里云客户端未初始化")
	}
	dbInstanceID = strings.TrimSpace(dbInstanceID)
	if dbInstanceID == "" {
		return nil, nil, fmt.Errorf("dbInstanceId 不能为空")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.config.Region
	}
	if region == "" {
		return nil, nil, fmt.Errorf("查询 RDS 监控必须指定 region")
	}
	period = strings.TrimSpace(period)
	if period == "" {
		period = c.config.MetricPeriod
	}
	if minutes <= 0 || minutes > 24*60 {
		minutes = 30
	}
	client, err := rds.NewClientWithAccessKey(region, c.config.AccessKeyID, c.config.AccessKeySecret)
	if err != nil {
		return nil, nil, fmt.Errorf("创建 RDS 只读客户端失败: %w", err)
	}
	keys := rdsPerformanceKeys(engine)
	out := make(map[string]*MetricSeries)
	errorsByKey := make(map[string]string)
	for _, batch := range chunkStrings(keys, maxRDSPerformanceKeySize) {
		series, err := queryRDSPerformanceBatch(client, batch, dbInstanceID, region, engine, minutes, period)
		if err == nil {
			mergeMetricSeries(out, series)
			continue
		}
		for _, key := range batch {
			series, keyErr := queryRDSPerformanceBatch(client, []string{key}, dbInstanceID, region, engine, minutes, period)
			if keyErr != nil {
				errorsByKey[key] = keyErr.Error()
				continue
			}
			mergeMetricSeries(out, series)
		}
	}
	return out, errorsByKey, nil
}

func queryRDSPerformanceBatch(client *rds.Client, keys []string, dbInstanceID, region, engine string, minutes int, period string) (map[string]*MetricSeries, error) {
	if len(keys) == 0 {
		return map[string]*MetricSeries{}, nil
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(minutes) * time.Minute)
	req := rds.CreateDescribeDBInstancePerformanceRequest()
	req.DBInstanceId = dbInstanceID
	req.Key = strings.Join(keys, ",")
	req.StartTime = start.Format("2006-01-02T15:04Z")
	req.EndTime = end.Format("2006-01-02T15:04Z")
	req.UseNullWhenMissingPoint = requests.Boolean("true")
	resp, err := client.DescribeDBInstancePerformance(req)
	if err != nil {
		return nil, fmt.Errorf("查询 RDS 性能指标失败 region=%s instance=%s keys=%s: %w", region, dbInstanceID, req.Key, err)
	}
	return buildRDSMetricSeries(resp.PerformanceKeys.PerformanceKey, dbInstanceID, region, firstNonEmpty(resp.Engine, engine), period), nil
}

func buildRDSMetricSeries(performanceKeys []rds.PerformanceKey, dbInstanceID, region, engine, period string) map[string]*MetricSeries {
	out := make(map[string]*MetricSeries)
	for _, performanceKey := range performanceKeys {
		key := strings.TrimSpace(performanceKey.Key)
		if key == "" {
			continue
		}
		for _, value := range performanceKey.Values.PerformanceValue {
			rawParts := splitRDSMetricParts(value.Value)
			if len(rawParts) == 0 {
				continue
			}
			subKeys := splitRDSMetricFormat(performanceKey.ValueFormat, len(rawParts))
			timestamp := parseAliyunTimeMillis(value.Date)
			if timestamp == 0 {
				continue
			}
			for index, rawPart := range rawParts {
				metricValue, ok := numberAsFloat64(rawPart)
				if !ok {
					continue
				}
				subKey := rdsMetricSubKey(subKeys, key, index)
				seriesID := rdsMetricSeriesID(key, subKey, index)
				series := out[seriesID]
				if series == nil {
					series = &MetricSeries{
						InstanceID:  dbInstanceID,
						RegionID:    region,
						Namespace:   NamespaceRDSPerformance,
						MetricName:  key,
						Label:       rdsMetricLabel(key, subKey),
						SubKey:      subKey,
						Unit:        strings.TrimSpace(performanceKey.Unit),
						ValueFormat: strings.TrimSpace(performanceKey.ValueFormat),
						Period:      period,
						Points:      []MetricPoint{},
					}
					out[seriesID] = series
				}
				series.Points = append(series.Points, MetricPoint{
					Timestamp: timestamp,
					Value:     metricValue,
					Raw: map[string]string{
						"engine": engine,
						"key":    key,
						"subKey": subKey,
						"raw":    strings.TrimSpace(rawPart),
						"date":   value.Date,
					},
				})
			}
		}
	}
	for _, series := range out {
		sort.Slice(series.Points, func(i, j int) bool {
			return series.Points[i].Timestamp < series.Points[j].Timestamp
		})
	}
	return out
}

func newMetricSeries(instanceID, region, metricName, period, unit string) *MetricSeries {
	return &MetricSeries{
		InstanceID: instanceID,
		RegionID:   region,
		Namespace:  "acs_ecs",
		MetricName: metricName,
		Unit:       unit,
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

func aliyunMetricUnit(metricName string) string {
	metricName = strings.TrimSpace(metricName)
	switch metricName {
	case "CPUUtilization", "cpu_total", "memory_usedutilization", "diskusage_utilization":
		return "%"
	case "InternetInRate", "InternetOutRate", "IntranetInRate", "IntranetOutRate",
		"VPC_PublicIP_InternetInRate", "VPC_PublicIP_InternetOutRate":
		return "bit/s"
	case "DiskReadBPS", "DiskWriteBPS", "disk_readbytes", "disk_writebytes":
		return "Byte/s"
	case "load_1m":
		return ""
	default:
		return ""
	}
}

func parseAliyunTimeMillis(raw string) int64 {
	parsed, ok := parseAliyunTime(raw)
	if !ok {
		return 0
	}
	return parsed.UnixMilli()
}

func parseAliyunTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04Z", "2006-01-02T15:04:05Z"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
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
	isSpot := isSpotInstance(item.IsSpot, item.SpotStrategy)
	expiration := resolveExpiration(item.ExpiredTime, item.InstanceChargeType, isSpot, time.Now())
	return Instance{
		ID:                item.InstanceId,
		Name:              firstNonEmpty(item.InstanceName, item.HostName, item.Hostname, item.InstanceId),
		HostName:          firstNonEmpty(item.HostName, item.Hostname),
		Provider:          ProviderName,
		RegionID:          region,
		ZoneID:            item.ZoneId,
		Status:            item.Status,
		OSName:            firstNonEmpty(item.OSName, item.OSNameEn),
		OSType:            firstNonEmpty(item.OSType, item.OsType),
		Type:              item.InstanceType,
		ChargeType:        item.InstanceChargeType,
		IsSpot:            isSpot,
		SpotStrategy:      item.SpotStrategy,
		CPU:               firstPositive(item.CPU, item.Cpu),
		MemoryMB:          item.Memory,
		PublicIPs:         publicIPs,
		EipAddress:        eipAddress,
		EipID:             strings.TrimSpace(item.EipAddress.AllocationId),
		PrivateIPs:        compactStrings(append([]string{}, item.VpcAttributes.PrivateIpAddress.IpAddress...)),
		VpcID:             item.VpcAttributes.VpcId,
		VSwitchID:         item.VpcAttributes.VSwitchId,
		SecurityIDs:       compactStrings(append([]string{}, item.SecurityGroupIds.SecurityGroupId...)),
		CreatedAt:         item.CreationTime,
		ExpiredAt:         item.ExpiredTime,
		ExpiresInDays:     expiration.ExpiresInDays,
		ExpirationStatus:  expiration.Status,
		ExpirationMessage: expiration.Message,
	}
}

func mapRDSInstance(item rds.DBInstance, fallbackRegion string) RDSInstance {
	region := firstNonEmpty(item.RegionId, fallbackRegion)
	cpuRaw := strings.TrimSpace(item.DBInstanceCPU)
	payType := strings.TrimSpace(item.PayType)
	expiration := resolveExpiration(item.ExpireTime, payType, false, time.Now())
	return RDSInstance{
		ID:                 strings.TrimSpace(item.DBInstanceId),
		Name:               firstNonEmpty(item.DBInstanceDescription, item.DBInstanceName, item.DBInstanceId),
		Provider:           ProviderName,
		RegionID:           region,
		ZoneID:             strings.TrimSpace(item.ZoneId),
		Engine:             strings.TrimSpace(item.Engine),
		EngineVersion:      strings.TrimSpace(item.EngineVersion),
		Status:             firstNonEmpty(item.DBInstanceStatus, item.Status),
		LockMode:           strings.TrimSpace(item.LockMode),
		LockReason:         strings.TrimSpace(item.LockReason),
		Type:               strings.TrimSpace(item.DBInstanceType),
		Category:           strings.TrimSpace(item.Category),
		Class:              strings.TrimSpace(item.DBInstanceClass),
		CPU:                parsePositiveInt(cpuRaw, 0),
		CPURaw:             cpuRaw,
		MemoryMB:           int64(item.DBInstanceMemory),
		StorageType:        strings.TrimSpace(item.DBInstanceStorageType),
		NetworkType:        strings.TrimSpace(item.InstanceNetworkType),
		NetType:            strings.TrimSpace(item.DBInstanceNetType),
		ConnectionMode:     strings.TrimSpace(item.ConnectionMode),
		ConnectionString:   strings.TrimSpace(item.ConnectionString),
		VpcID:              strings.TrimSpace(item.VpcId),
		VSwitchID:          strings.TrimSpace(item.VSwitchId),
		ResourceGroupID:    strings.TrimSpace(item.ResourceGroupId),
		DeletionProtection: item.DeletionProtection,
		CreatedAt:          strings.TrimSpace(item.CreateTime),
		ExpiredAt:          strings.TrimSpace(item.ExpireTime),
		PayType:            payType,
		ExpiresInDays:      expiration.ExpiresInDays,
		ExpirationStatus:   expiration.Status,
		ExpirationMessage:  expiration.Message,
	}
}

func mergeRDSInstanceAttribute(instance *RDSInstance, attr rds.DBInstanceAttribute) {
	if instance == nil {
		return
	}
	if value := strings.TrimSpace(attr.DBInstanceId); value != "" {
		instance.ID = value
	}
	instance.Name = firstNonEmpty(attr.DBInstanceDescription, instance.Name, attr.DBInstanceId)
	instance.RegionID = firstNonEmpty(attr.RegionId, instance.RegionID)
	instance.ZoneID = firstNonEmpty(attr.ZoneId, instance.ZoneID)
	instance.Engine = firstNonEmpty(attr.Engine, instance.Engine)
	instance.EngineVersion = firstNonEmpty(attr.EngineVersion, instance.EngineVersion)
	instance.Status = firstNonEmpty(attr.DBInstanceStatus, instance.Status)
	instance.LockMode = firstNonEmpty(attr.LockMode, instance.LockMode)
	instance.LockReason = firstNonEmpty(attr.LockReason, instance.LockReason)
	instance.Type = firstNonEmpty(attr.DBInstanceType, instance.Type)
	instance.Category = firstNonEmpty(attr.Category, instance.Category)
	instance.Class = firstNonEmpty(attr.DBInstanceClass, instance.Class)
	instance.ClassType = firstNonEmpty(attr.DBInstanceClassType, instance.ClassType)
	instance.CPURaw = firstNonEmpty(attr.DBInstanceCPU, instance.CPURaw)
	instance.CPU = firstPositive(parsePositiveInt(attr.DBInstanceCPU, 0), instance.CPU)
	instance.MemoryMB = firstPositiveInt64(attr.DBInstanceMemory, instance.MemoryMB)
	instance.StorageGB = firstPositive(attr.DBInstanceStorage, instance.StorageGB)
	instance.StorageType = firstNonEmpty(attr.DBInstanceStorageType, instance.StorageType)
	instance.MaxConnections = firstPositive(attr.MaxConnections, instance.MaxConnections)
	instance.MaxIOPS = firstPositive(attr.MaxIOPS, instance.MaxIOPS)
	instance.MaxIOMBPS = firstPositive(attr.MaxIOMBPS, instance.MaxIOMBPS)
	instance.NetworkType = firstNonEmpty(attr.InstanceNetworkType, instance.NetworkType)
	instance.NetType = firstNonEmpty(attr.DBInstanceNetType, instance.NetType)
	instance.ConnectionMode = firstNonEmpty(attr.ConnectionMode, instance.ConnectionMode)
	instance.ConnectionString = firstNonEmpty(attr.ConnectionString, instance.ConnectionString)
	instance.Port = firstNonEmpty(attr.Port, instance.Port)
	instance.VpcID = firstNonEmpty(attr.VpcId, instance.VpcID)
	instance.VSwitchID = firstNonEmpty(attr.VSwitchId, instance.VSwitchID)
	instance.ResourceGroupID = firstNonEmpty(attr.ResourceGroupId, instance.ResourceGroupID)
	instance.AccountType = firstNonEmpty(attr.AccountType, instance.AccountType)
	instance.DeletionProtection = attr.DeletionProtection
	instance.CreatedAt = firstNonEmpty(attr.CreationTime, instance.CreatedAt)
	instance.ExpiredAt = firstNonEmpty(attr.ExpireTime, instance.ExpiredAt)
	instance.PayType = firstNonEmpty(attr.PayType, instance.PayType)
	expiration := resolveExpiration(instance.ExpiredAt, instance.PayType, false, time.Now())
	instance.ExpiresInDays = expiration.ExpiresInDays
	instance.ExpirationStatus = expiration.Status
	instance.ExpirationMessage = expiration.Message
}

func mergeRDSPrimaryEndpoint(instance *RDSInstance) {
	if instance == nil || len(instance.Endpoints) == 0 {
		return
	}
	for _, endpoint := range instance.Endpoints {
		if strings.EqualFold(endpoint.ConnectionStringType, "Normal") || strings.EqualFold(endpoint.IPType, "Private") {
			instance.ConnectionString = firstNonEmpty(instance.ConnectionString, endpoint.ConnectionString)
			instance.Port = firstNonEmpty(instance.Port, endpoint.Port)
			instance.VpcID = firstNonEmpty(instance.VpcID, endpoint.VPCID)
			instance.VSwitchID = firstNonEmpty(instance.VSwitchID, endpoint.VSwitchID)
			return
		}
	}
	endpoint := instance.Endpoints[0]
	instance.ConnectionString = firstNonEmpty(instance.ConnectionString, endpoint.ConnectionString)
	instance.Port = firstNonEmpty(instance.Port, endpoint.Port)
	instance.VpcID = firstNonEmpty(instance.VpcID, endpoint.VPCID)
	instance.VSwitchID = firstNonEmpty(instance.VSwitchID, endpoint.VSwitchID)
}

type expirationInfo struct {
	ExpiresInDays *int
	Status        string
	Message       string
}

func resolveExpiration(expiredAt, chargeType string, isSpot bool, now time.Time) expirationInfo {
	expiredAt = strings.TrimSpace(expiredAt)
	chargeType = strings.TrimSpace(chargeType)
	if isSpot {
		return expirationInfo{
			Status:  expirationStatusNoExpiration,
			Message: "抢占式实例按量计费，无固定到期日",
		}
	}
	if strings.EqualFold(chargeType, "PostPaid") {
		return expirationInfo{
			Status:  expirationStatusNoExpiration,
			Message: "按量付费资源无固定到期日",
		}
	}
	if expiredAt == "" {
		return expirationInfo{
			Status:  expirationStatusUnknown,
			Message: "云厂商未返回到期时间",
		}
	}
	expiresAt, ok := parseAliyunTime(expiredAt)
	if !ok {
		return expirationInfo{
			Status:  expirationStatusUnknown,
			Message: "到期时间格式无法识别",
		}
	}
	if now.IsZero() {
		now = time.Now()
	}
	if isPlaceholderExpiration(expiresAt, now) {
		return expirationInfo{
			Status:  expirationStatusNoExpiration,
			Message: "云厂商返回远期占位时间，视为无固定到期日",
		}
	}
	if !expiresAt.After(now) {
		zero := 0
		return expirationInfo{
			ExpiresInDays: &zero,
			Status:        expirationStatusExpired,
			Message:       "已到期",
		}
	}
	days := int(expiresAt.Sub(now).Hours() / 24)
	status := expirationStatusNormal
	if days <= expiringSoonDays {
		status = expirationStatusExpiring
	}
	message := fmt.Sprintf("剩余 %d 天", days)
	if days == 0 {
		message = "不足 1 天到期"
	}
	return expirationInfo{
		ExpiresInDays: &days,
		Status:        status,
		Message:       message,
	}
}

func isPlaceholderExpiration(expiresAt, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	if !expiresAt.After(now) {
		return false
	}
	days := int(expiresAt.Sub(now).Hours() / 24)
	return days >= placeholderExpirationDays || expiresAt.Year() >= 2099
}

func isSpotInstance(isSpot bool, spotStrategy string) bool {
	if isSpot {
		return true
	}
	spotStrategy = strings.TrimSpace(spotStrategy)
	return spotStrategy != "" && !strings.EqualFold(spotStrategy, spotStrategyNone)
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

func rdsPerformanceKeys(engine string) []string {
	normalized := strings.ToLower(strings.TrimSpace(engine))
	switch {
	case strings.Contains(normalized, "sqlserver") || strings.Contains(normalized, "mssql"):
		return append([]string{}, rdsSQLServerPerformanceKeys...)
	case strings.Contains(normalized, "postgres") || strings.Contains(normalized, "pgsql"):
		return append([]string{}, rdsPostgreSQLPerformanceKeys...)
	case strings.Contains(normalized, "mariadb"):
		return append([]string{}, rdsMySQLPerformanceKeys...)
	case strings.Contains(normalized, "mysql"):
		return append([]string{}, rdsMySQLPerformanceKeys...)
	default:
		keys := append([]string{}, rdsMySQLPerformanceKeys...)
		keys = append(keys, rdsPostgreSQLPerformanceKeys...)
		keys = append(keys, rdsSQLServerPerformanceKeys...)
		return uniqueStrings(keys)
	}
}

var rdsMySQLPerformanceKeys = []string{
	"MySQL_NetworkTraffic",
	"MySQL_QPSTPS",
	"MySQL_Sessions",
	"MySQL_InnoDBBufferRatio",
	"MySQL_InnoDBDataReadWriten",
	"MySQL_InnoDBLogRequests",
	"MySQL_InnoDBLogWrites",
	"MySQL_TempDiskTableCreates",
	"MySQL_MyISAMKeyBufferRatio",
	"MySQL_MyISAMKeyReadWrites",
	"MySQL_COMDML",
	"MySQL_RowDML",
	"MySQL_MemCpuUsage",
	"MySQL_IOPS",
	"MySQL_DetailedSpaceUsage",
	"slavestat",
	"MySQL_ThreadStatus",
	"MySQL_ReplicationDelay",
	"MySQL_ReplicationThread",
	"MySQL_ROW_LOCK",
	"MySQL_SelectScan",
	"MySQL_MBPS",
	"MySQL_RCU_MemCpuUsage",
	"MySQL_RCU_IOPS",
	"MySQL_RCU_DetailedSpaceUsage",
}

var rdsPostgreSQLPerformanceKeys = []string{
	"CpuUsage",
	"MemoryUsage",
	"PgSQL_SpaceUsage",
	"PgSQL_IOPS",
	"PgSQL_Session",
	"PolarDBConnections",
	"PolarDBRowDML",
	"PolarDBQPSTPS",
	"PolarDBSwellTime",
	"PolarDBCPU",
	"PolarDBMemory",
	"PolarDBReplication",
	"PolarDBLongSQL",
	"PolarDBLongIdleTransaction",
	"PolarDBLongTransaction",
	"PolarDBLongTwoPCTransaction",
	"PolarDBLocalIOSTAT",
	"PolarDBLocalDiskUsage",
}

var rdsSQLServerPerformanceKeys = []string{
	"SQLServer_DetailedSpaceUsage",
	"SQLServer_InstanceDiskUsage",
	"SQLServer_BufferHit",
	"SQLServer_InstanceMemUsage",
	"SQLServer_InstanceCPUUsage",
	"SQLServer_NetworkTraffic",
	"SQLServer_Sessions",
	"SQLServer_Transactions",
	"SQLServer_AGPerf",
	"SQLServer_SQLCompilations",
	"SQLServer_InstanceIOPSUsage",
	"SQLServer_LockTimeout",
	"SQLServer_MirrorPerf",
	"SQLServer_PageLife",
	"SQLServer_Block",
	"SQLServer_FullScans",
	"SQLServer_InstanceMBPSUsage",
	"SQLServer_IOPS",
	"SQLServer_QPS",
	"SQLServer_CheckPoint",
	"SQLServer_Deadlock",
	"SQLServer_PagePerf",
	"SQLServer_MBPS",
	"SQLServer_Logins",
	"SQLServer_RCU",
	"SQLServer_IndexUsage",
	"SQLServer_Cache",
	"SQLServer_AdvancedMemUsage",
	"SQLServer_BackupPerf",
	"SQLServer_MemUsage",
	"SQLServer_LogGrowth",
	"SQLServer_OptimizeConcurrent",
	"SQLServer_LogPerf",
	"SQLServer_LockWaits",
}

func chunkStrings(values []string, size int) [][]string {
	if size <= 0 || len(values) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		chunks = append(chunks, values[start:end])
	}
	return chunks
}

func mergeMetricSeries(dst, src map[string]*MetricSeries) {
	for key, series := range src {
		dst[key] = series
	}
}

func splitRDSMetricParts(raw string) []string {
	fields := strings.Split(strings.TrimSpace(raw), "&")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" || strings.EqualFold(value, "null") {
			out = append(out, "")
			continue
		}
		out = append(out, value)
	}
	return out
}

func splitRDSMetricFormat(format string, valueCount int) []string {
	fields := strings.Split(strings.TrimSpace(format), "&")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) >= valueCount {
		return out
	}
	for len(out) < valueCount {
		out = append(out, fmt.Sprintf("value%d", len(out)+1))
	}
	return out
}

func rdsMetricSubKey(subKeys []string, key string, index int) string {
	if index >= 0 && index < len(subKeys) && strings.TrimSpace(subKeys[index]) != "" {
		return strings.TrimSpace(subKeys[index])
	}
	if index == 0 {
		return key
	}
	return fmt.Sprintf("value%d", index+1)
}

func rdsMetricSeriesID(key, subKey string, index int) string {
	cleanSubKey := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-':
			return r
		default:
			return '_'
		}
	}, strings.TrimSpace(subKey))
	if cleanSubKey == "" {
		cleanSubKey = fmt.Sprintf("value%d", index+1)
	}
	return strings.TrimSpace(key) + "." + cleanSubKey
}

func rdsMetricLabel(key, subKey string) string {
	if strings.TrimSpace(subKey) == "" || strings.EqualFold(strings.TrimSpace(subKey), strings.TrimSpace(key)) {
		return strings.TrimSpace(key)
	}
	return strings.TrimSpace(key) + " / " + strings.TrimSpace(subKey)
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
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

func firstPositiveInt64(values ...int64) int64 {
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
