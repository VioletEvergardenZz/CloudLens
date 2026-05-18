// 本文件用于华为云 ECS 与云监控 CES 只读查询。
// 文件职责：封装 ListServersDetails 与 ShowMetricData，避免 API 层直接依赖 SDK 细节。
// 边界与容错：只调用查询类 OpenAPI，不提供启动、停止、释放、修改等写操作。

package huawei

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	coreconfig "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	coreregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/region"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	cesregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/region"
	ecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	ecsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/region"
	rds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	rdsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/region"
)

const (
	ProviderName = "huawei"
	NamespaceECS = "SYS.ECS"
	NamespaceAGT = "AGT.ECS"
	NamespaceRDS = "SYS.RDS"

	expirationStatusNormal       = "normal"
	expirationStatusExpiring     = "expiring"
	expirationStatusExpired      = "expired"
	expirationStatusNoExpiration = "no_expiration"
	expirationStatusUnknown      = "unknown"
	expiringSoonDays             = 30
	chargeTypePostPaid           = "PostPaid"
	chargeTypePrePaid            = "PrePaid"
	chargeTypeSpot               = "Spot"
	maxRegionQueryConcurrency    = 8
	huaweiSDKRequestTimeout      = 5 * time.Second
)

var defaultECSRegions = []string{
	"af-north-1",
	"af-south-1",
	"ae-ad-1",
	"ap-southeast-1",
	"ap-southeast-2",
	"ap-southeast-3",
	"ap-southeast-4",
	"ap-southeast-5",
	"cn-east-2",
	"cn-east-3",
	"cn-east-4",
	"cn-east-5",
	"cn-north-1",
	"cn-north-2",
	"cn-north-4",
	"cn-north-9",
	"cn-north-11",
	"cn-north-12",
	"cn-south-1",
	"cn-south-2",
	"cn-south-4",
	"cn-southwest-2",
	"cn-southwest-3",
	"eu-west-0",
	"eu-west-101",
	"la-north-2",
	"la-south-2",
	"me-east-1",
	"my-kualalumpur-1",
	"na-mexico-1",
	"ru-moscow-1",
	"sa-brazil-1",
	"tr-west-1",
}

type Client struct {
	config Config
}

func NewClient(config Config) (*Client, error) {
	config.AccessKeyID = strings.TrimSpace(config.AccessKeyID)
	config.AccessKeySecret = strings.TrimSpace(config.AccessKeySecret)
	config.ProjectID = strings.TrimSpace(config.ProjectID)
	config.Region = strings.TrimSpace(config.Region)
	config.MetricPeriod = strings.TrimSpace(config.MetricPeriod)
	if config.AccessKeyID == "" || config.AccessKeySecret == "" {
		return nil, fmt.Errorf("华为云 AccessKey 未配置，请在云账号管理中新增账号，或临时设置 HUAWEI_ACCESS_KEY_ID 和 HUAWEI_ACCESS_KEY_SECRET")
	}
	if config.MetricPeriod == "" {
		config.MetricPeriod = "60"
	}
	return &Client{config: config}, nil
}

func (c *Client) ListInstances(regions []string) ([]Instance, error) {
	if c == nil {
		return nil, fmt.Errorf("华为云客户端未初始化")
	}
	targetRegions, autoDiscovered := c.resolveECSRegions(regions)
	out := make([]Instance, 0)
	regionErrors := make([]string, 0)
	successfulRegions := 0
	for _, result := range c.listRegionInstancesConcurrently(targetRegions) {
		if result.err != nil {
			// 自动全地域采集时，单地域失败不能阻断其它地域，避免 Project ID 或局部权限问题遮住已有资源。
			if autoDiscovered {
				regionErrors = append(regionErrors, fmt.Sprintf("%s: %v", result.region, result.err))
				continue
			}
			return nil, result.err
		}
		successfulRegions++
		out = append(out, result.items...)
	}
	if successfulRegions == 0 && len(regionErrors) > 0 {
		return nil, summarizeRegionErrors("华为云 ECS 全地域采集失败", regionErrors)
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
	client, err := c.newECSClient(region)
	if err != nil {
		return nil, err
	}
	const pageSize int32 = 100
	out := make([]Instance, 0)
	for offset := int32(0); ; offset += pageSize {
		req := &ecsmodel.ListServersDetailsRequest{
			Limit:  int32Ptr(pageSize),
			Offset: int32Ptr(offset),
		}
		resp, err := client.ListServersDetails(req)
		if err != nil {
			return nil, fmt.Errorf("查询华为云 ECS 实例失败 region=%s: %w", region, err)
		}
		if resp.Servers == nil || len(*resp.Servers) == 0 {
			break
		}
		for _, item := range *resp.Servers {
			out = append(out, mapInstance(item, region))
		}
		if resp.Count != nil && int32(len(out)) >= *resp.Count {
			break
		}
		if len(*resp.Servers) < int(pageSize) {
			break
		}
	}
	return out, nil
}

type instanceRegionResult struct {
	region string
	items  []Instance
	err    error
}

func (c *Client) listRegionInstancesConcurrently(regions []string) []instanceRegionResult {
	if len(regions) == 0 {
		return nil
	}
	results := make([]instanceRegionResult, 0, len(regions))
	resultCh := make(chan instanceRegionResult, len(regions))
	guard := make(chan struct{}, maxRegionQueryConcurrency)
	var wg sync.WaitGroup

	// 华为云全地域巡检会触发多个 Project/Region 查询；这里限流并发，避免一个刷新动作造成过高 API 压力。
	for _, region := range regions {
		region := region
		wg.Add(1)
		go func() {
			defer wg.Done()
			guard <- struct{}{}
			defer func() { <-guard }()
			items, err := c.listRegionInstances(region)
			resultCh <- instanceRegionResult{region: region, items: items, err: err}
		}()
	}
	wg.Wait()
	close(resultCh)
	for result := range resultCh {
		results = append(results, result)
	}
	return results
}

func (c *Client) ListRDSInstances(regions []string) ([]RDSInstance, error) {
	if c == nil {
		return nil, fmt.Errorf("华为云客户端未初始化")
	}
	targetRegions, autoDiscovered := c.resolveRDSRegions(regions)
	out := make([]RDSInstance, 0)
	regionErrors := make([]string, 0)
	successfulRegions := 0
	for _, result := range c.listRegionRDSInstancesConcurrently(targetRegions) {
		if result.err != nil {
			// RDS 全地域采集同样允许单地域失败，避免一个未开通地域遮住其它地域已有数据库。
			if autoDiscovered {
				regionErrors = append(regionErrors, fmt.Sprintf("%s: %v", result.region, result.err))
				continue
			}
			return nil, result.err
		}
		successfulRegions++
		out = append(out, result.items...)
	}
	if successfulRegions == 0 && len(regionErrors) > 0 {
		return nil, summarizeRegionErrors("华为云 RDS 全地域采集失败", regionErrors)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RegionID == out[j].RegionID {
			return out[i].Name < out[j].Name
		}
		return out[i].RegionID < out[j].RegionID
	})
	return out, nil
}

func (c *Client) listRegionRDSInstances(region string) ([]RDSInstance, error) {
	client, err := c.newRDSClient(region)
	if err != nil {
		return nil, err
	}
	const pageSize int32 = 100
	out := make([]RDSInstance, 0)
	for offset := int32(0); ; offset += pageSize {
		req := &rdsmodel.ListInstancesRequest{
			Limit:  int32Ptr(pageSize),
			Offset: int32Ptr(offset),
		}
		resp, err := client.ListInstances(req)
		if err != nil {
			return nil, fmt.Errorf("查询华为云 RDS 实例失败 region=%s: %w", region, err)
		}
		if resp.Instances == nil || len(*resp.Instances) == 0 {
			break
		}
		for _, item := range *resp.Instances {
			instance := mapRDSInstance(item, region)
			if usage, err := c.rdsStorageUsage(client, instance.ID, instance.StorageGB, instance.Engine); err == nil {
				instance.ResourceUsage = usage
			} else {
				instance.DetailErrors = append(instance.DetailErrors, fmt.Sprintf("读取 RDS 空间用量失败: %v", err))
			}
			out = append(out, instance)
		}
		if resp.TotalCount != nil && int32(len(out)) >= *resp.TotalCount {
			break
		}
		if len(*resp.Instances) < int(pageSize) {
			break
		}
	}
	return out, nil
}

type rdsRegionResult struct {
	region string
	items  []RDSInstance
	err    error
}

func (c *Client) listRegionRDSInstancesConcurrently(regions []string) []rdsRegionResult {
	if len(regions) == 0 {
		return nil
	}
	results := make([]rdsRegionResult, 0, len(regions))
	resultCh := make(chan rdsRegionResult, len(regions))
	guard := make(chan struct{}, maxRegionQueryConcurrency)
	var wg sync.WaitGroup

	// RDS 列表查询按地域限流并发，避免全地域刷新时对云厂商 API 造成过高压力。
	for _, region := range regions {
		region := region
		wg.Add(1)
		go func() {
			defer wg.Done()
			guard <- struct{}{}
			defer func() { <-guard }()
			items, err := c.listRegionRDSInstances(region)
			resultCh <- rdsRegionResult{region: region, items: items, err: err}
		}()
	}
	wg.Wait()
	close(resultCh)
	for result := range resultCh {
		results = append(results, result)
	}
	return results
}

func (c *Client) Metric(metricName, instanceID, region string, minutes int, period string) (*MetricSeries, error) {
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		metricName = "cpu_util"
	}
	return c.MetricWithDimensions(NamespaceECS, metricName, map[string]string{"instance_id": strings.TrimSpace(instanceID)}, region, minutes, period)
}

func (c *Client) MetricWithDimensions(namespace, metricName string, dimensions map[string]string, region string, minutes int, period string) (*MetricSeries, error) {
	if c == nil {
		return nil, fmt.Errorf("华为云客户端未初始化")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.config.Region
	}
	if region == "" {
		return nil, fmt.Errorf("查询华为云监控指标必须指定 region")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = NamespaceECS
	}
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		metricName = "cpu_util"
	}
	periodSeconds := parseHuaweiPeriodSeconds(firstNonEmpty(period, c.config.MetricPeriod))
	if minutes <= 0 || minutes > 24*60 {
		minutes = 30
	}
	dimensionValues := metricDimensions(dimensions)
	if len(dimensionValues) == 0 {
		return nil, fmt.Errorf("华为云监控维度不能为空")
	}
	instanceID := firstNonEmpty(dimensions["instance_id"], dimensions["instanceId"], dimensions["server_id"], dimensions["rds_cluster_id"], dimensions["rds_instance_id"], dimensionValues[0])
	client, err := c.newCESClient(region)
	if err != nil {
		return nil, err
	}
	end := time.Now().Add(-time.Duration(periodSeconds) * time.Second)
	start := end.Add(-time.Duration(minutes) * time.Minute)
	periodEnum := huaweiPeriodEnum(periodSeconds)
	req := &cesmodel.ShowMetricDataRequest{
		Namespace:  namespace,
		MetricName: metricName,
		Dim0:       dimensionValues[0],
		Filter:     cesmodel.GetShowMetricDataRequestFilterEnum().AVERAGE,
		Period:     periodEnum,
		From:       start.UnixMilli(),
		To:         end.UnixMilli(),
	}
	if len(dimensionValues) > 1 {
		req.Dim1 = stringPtr(dimensionValues[1])
	}
	if len(dimensionValues) > 2 {
		req.Dim2 = stringPtr(dimensionValues[2])
	}
	if len(dimensionValues) > 3 {
		req.Dim3 = stringPtr(dimensionValues[3])
	}
	resp, err := client.ShowMetricData(req)
	if err != nil {
		return nil, fmt.Errorf("查询华为云 CES 指标失败 region=%s instance=%s metric=%s: %w", region, instanceID, metricName, err)
	}
	points := make([]MetricPoint, 0)
	if resp.Datapoints != nil {
		for _, item := range *resp.Datapoints {
			points = append(points, mapMetricPoint(item))
		}
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})
	return &MetricSeries{
		InstanceID: instanceID,
		RegionID:   region,
		Namespace:  namespace,
		MetricName: firstNonEmpty(stringPtrValue(resp.MetricName), metricName),
		Unit:       resolveMetricSeriesUnit(points, huaweiMetricUnit(namespace, metricName)),
		Period:     strconv.Itoa(int(periodEnum.Value())),
		Points:     points,
	}, nil
}

func (c *Client) MetricDimensions(namespace, metricName string, dimensions map[string]string, region string) ([]map[string]string, error) {
	if c == nil {
		return nil, fmt.Errorf("华为云客户端未初始化")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = c.config.Region
	}
	if region == "" {
		return nil, fmt.Errorf("查询华为云监控指标维度必须指定 region")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = NamespaceECS
	}
	metricName = strings.TrimSpace(metricName)
	if metricName == "" {
		return nil, fmt.Errorf("metricName 不能为空")
	}
	metricDims := metricDimensions(dimensions)
	client, err := c.newCESClient(region)
	if err != nil {
		return nil, err
	}
	req := &cesmodel.ListMetricsRequest{
		Namespace:  stringPtr(namespace),
		MetricName: stringPtr(metricName),
		Limit:      int32Ptr(1000),
	}
	if len(metricDims) > 0 {
		req.Dim0 = stringPtr(metricDims[0])
	}
	if len(metricDims) > 1 {
		req.Dim1 = stringPtr(metricDims[1])
	}
	if len(metricDims) > 2 {
		req.Dim2 = stringPtr(metricDims[2])
	}
	resp, err := client.ListMetrics(req)
	if err != nil {
		return nil, fmt.Errorf("查询华为云 CES 指标维度失败 region=%s metric=%s: %w", region, metricName, err)
	}
	out := make([]map[string]string, 0)
	if resp.Metrics == nil {
		return out, nil
	}
	for _, metric := range *resp.Metrics {
		if metric.Namespace != namespace || metric.MetricName != metricName {
			continue
		}
		item := make(map[string]string)
		for _, dim := range metric.Dimensions {
			name := strings.TrimSpace(dim.Name)
			value := strings.TrimSpace(dim.Value)
			if name != "" && value != "" {
				item[name] = value
			}
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out, nil
}

func (c *Client) RDSPerformance(dbInstanceID, nodeID, region string, minutes int, period string) (map[string]*MetricSeries, map[string]string, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("华为云客户端未初始化")
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
		return nil, nil, fmt.Errorf("查询华为云 RDS 监控必须指定 region")
	}
	if minutes <= 0 || minutes > 24*60 {
		minutes = 30
	}
	candidateGroups := huaweiRDSMetricCandidates(dbInstanceID, nodeID)
	resultCh := make(chan huaweiRDSMetricResult, len(candidateGroups))
	var wg sync.WaitGroup
	// RDS 概览同时查多个指标组；每组内部仍按候选维度兜底，避免一个慢指标拖住整个详情页。
	for key, candidates := range candidateGroups {
		key, candidates := key, candidates
		wg.Add(1)
		go func() {
			defer wg.Done()
			series, err := c.queryFirstRDSMetric(candidates, region, minutes, period)
			resultCh <- huaweiRDSMetricResult{key: key, series: series, err: err}
		}()
	}
	wg.Wait()
	close(resultCh)
	metrics := make(map[string]*MetricSeries)
	errorsByMetric := make(map[string]string)
	for result := range resultCh {
		if result.err != nil {
			errorsByMetric[result.key] = result.err.Error()
			continue
		}
		metrics[result.key] = result.series
	}
	return metrics, errorsByMetric, nil
}

type huaweiRDSMetricResult struct {
	key    string
	series *MetricSeries
	err    error
}

type huaweiRDSMetricCandidate struct {
	Name       string
	Label      string
	Dimensions map[string]string
	Unit       string
}

func huaweiRDSMetricCandidates(dbInstanceID, nodeID string) map[string][]huaweiRDSMetricCandidate {
	clusterDimension := map[string]string{"rds_cluster_id": strings.TrimSpace(dbInstanceID)}
	instanceDimension := map[string]string{"rds_instance_id": strings.TrimSpace(firstNonEmpty(nodeID, dbInstanceID))}
	bothDimension := map[string]string{
		"rds_cluster_id":  strings.TrimSpace(dbInstanceID),
		"rds_instance_id": strings.TrimSpace(firstNonEmpty(nodeID, dbInstanceID)),
	}
	return map[string][]huaweiRDSMetricCandidate{
		"cpu": {
			{Name: "rds001_cpu_util", Label: "CPU 使用率", Dimensions: clusterDimension, Unit: "%"},
			{Name: "rds001_cpu_util", Label: "CPU 使用率", Dimensions: bothDimension, Unit: "%"},
			{Name: "rds001_cpu_util", Label: "CPU 使用率", Dimensions: instanceDimension, Unit: "%"},
		},
		"memory": {
			{Name: "rds002_mem_util", Label: "内存使用率", Dimensions: clusterDimension, Unit: "%"},
			{Name: "rds002_mem_util", Label: "内存使用率", Dimensions: bothDimension, Unit: "%"},
			{Name: "rds002_mem_util", Label: "内存使用率", Dimensions: instanceDimension, Unit: "%"},
		},
		"qps": {
			{Name: "rds008_qps", Label: "QPS", Dimensions: clusterDimension},
			{Name: "rds008_qps", Label: "QPS", Dimensions: bothDimension},
			{Name: "rds008_qps", Label: "QPS", Dimensions: instanceDimension},
		},
		"connections": {
			{Name: "rds072_conn_usage", Label: "连接数使用率", Dimensions: clusterDimension, Unit: "%"},
			{Name: "rds072_conn_usage", Label: "连接数使用率", Dimensions: bothDimension, Unit: "%"},
			{Name: "rds072_conn_usage", Label: "连接数使用率", Dimensions: instanceDimension, Unit: "%"},
		},
		"iops": {
			{Name: "rds039_iops", Label: "IOPS", Dimensions: clusterDimension},
			{Name: "rds039_iops", Label: "IOPS", Dimensions: bothDimension},
			{Name: "rds039_iops", Label: "IOPS", Dimensions: instanceDimension},
		},
	}
}

func (c *Client) queryFirstRDSMetric(candidates []huaweiRDSMetricCandidate, region string, minutes int, period string) (*MetricSeries, error) {
	var firstErr error
	var firstEmpty *MetricSeries
	for _, candidate := range candidates {
		series, err := c.MetricWithDimensions(NamespaceRDS, candidate.Name, candidate.Dimensions, region, minutes, period)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if series != nil {
			series.Label = firstNonEmpty(candidate.Label, series.Label)
			series.Unit = firstNonEmpty(candidate.Unit, series.Unit)
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
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("没有可用的华为云 RDS 指标候选")
}

func (c *Client) newECSClient(regionID string) (*ecs.EcsClient, error) {
	credentials, err := c.credentials()
	if err != nil {
		return nil, err
	}
	reg, err := ecsregion.SafeValueOf(regionID)
	if err != nil {
		reg = coreregion.NewRegion(regionID, fmt.Sprintf("https://ecs.%s.myhuaweicloud.com", regionID))
	}
	hcClient, err := ecs.EcsClientBuilder().
		WithRegion(reg).
		WithCredential(credentials).
		WithHttpConfig(huaweiHTTPConfig()).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("创建华为云 ECS 只读客户端失败 region=%s: %w", regionID, err)
	}
	return ecs.NewEcsClient(hcClient), nil
}

func (c *Client) newCESClient(regionID string) (*ces.CesClient, error) {
	credentials, err := c.credentials()
	if err != nil {
		return nil, err
	}
	reg, err := cesregion.SafeValueOf(regionID)
	if err != nil {
		reg = coreregion.NewRegion(regionID, fmt.Sprintf("https://ces.%s.myhuaweicloud.com", regionID))
	}
	hcClient, err := ces.CesClientBuilder().
		WithRegion(reg).
		WithCredential(credentials).
		WithHttpConfig(huaweiHTTPConfig()).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("创建华为云 CES 只读客户端失败 region=%s: %w", regionID, err)
	}
	return ces.NewCesClient(hcClient), nil
}

func (c *Client) newRDSClient(regionID string) (*rds.RdsClient, error) {
	credentials, err := c.credentials()
	if err != nil {
		return nil, err
	}
	reg, err := rdsregion.SafeValueOf(regionID)
	if err != nil {
		reg = coreregion.NewRegion(regionID, fmt.Sprintf("https://rds.%s.myhuaweicloud.com", regionID))
	}
	hcClient, err := rds.RdsClientBuilder().
		WithRegion(reg).
		WithCredential(credentials).
		WithHttpConfig(huaweiHTTPConfig()).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("创建华为云 RDS 只读客户端失败 region=%s: %w", regionID, err)
	}
	return rds.NewRdsClient(hcClient), nil
}

func huaweiHTTPConfig() *coreconfig.HttpConfig {
	return coreconfig.DefaultHttpConfig().WithTimeout(huaweiSDKRequestTimeout)
}

func (c *Client) credentials() (*basic.Credentials, error) {
	builder := basic.NewCredentialsBuilder().
		WithAk(c.config.AccessKeyID).
		WithSk(c.config.AccessKeySecret)
	if strings.TrimSpace(c.config.ProjectID) != "" {
		builder.WithProjectId(c.config.ProjectID)
	}
	credentials, err := builder.SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("创建华为云访问凭据失败: %w", err)
	}
	return credentials, nil
}

func mapInstance(item ecsmodel.ServerDetail, fallbackRegion string) Instance {
	metadata := item.Metadata
	chargeType, isSpot := resolveChargeType(metadata)
	expiration := resolveExpiration(chargeType, isSpot)
	publicIPs, privateIPs := resolveIPs(item)
	securityIDs := make([]string, 0, len(item.SecurityGroups))
	for _, group := range item.SecurityGroups {
		securityIDs = append(securityIDs, firstNonEmpty(group.Id, group.Name))
	}
	return Instance{
		ID:                item.Id,
		Name:              firstNonEmpty(item.Name, item.OSEXTSRVATTRinstanceName, item.Id),
		HostName:          firstNonEmpty(item.OSEXTSRVATTRhostname, item.OSEXTSRVATTRhost),
		Provider:          ProviderName,
		RegionID:          fallbackRegion,
		ZoneID:            item.OSEXTAZavailabilityZone,
		Status:            item.Status,
		OSName:            firstNonEmpty(metadata["image_name"]),
		OSType:            firstNonEmpty(metadata["os_type"]),
		Type:              resolveFlavorName(item.Flavor),
		ChargeType:        chargeType,
		IsSpot:            isSpot,
		SpotStrategy:      resolveSpotStrategy(isSpot),
		CPU:               parsePositiveInt(resolveFlavorCPU(item.Flavor), 0),
		MemoryMB:          parsePositiveInt(resolveFlavorMemory(item.Flavor), 0),
		PublicIPs:         compactStrings(publicIPs),
		EipAddress:        firstNonEmpty(publicIPs...),
		PrivateIPs:        compactStrings(privateIPs),
		VpcID:             firstNonEmpty(metadata["vpc_id"]),
		VSwitchID:         resolveSubnetID(item.NetworkInterfaces),
		SecurityIDs:       compactStrings(securityIDs),
		CreatedAt:         item.Created,
		ExpiredAt:         "",
		ExpiresInDays:     expiration.ExpiresInDays,
		ExpirationStatus:  expiration.Status,
		ExpirationMessage: expiration.Message,
	}
}

func mapRDSInstance(item rdsmodel.InstanceResponse, fallbackRegion string) RDSInstance {
	engine, engineVersion := resolveRDSDatastore(item.Datastore)
	nodeID, zoneID := resolveRDSNodeInfo(item.Nodes)
	volumeType, volumeSize := resolveRDSVolume(item.Volume)
	chargeType := resolveRDSChargeMode(item.ChargeInfo)
	expiredAt := stringPtrValue(item.ExpirationTime)
	expiration := resolveRDSExpiration(expiredAt, chargeType, time.Now())
	cpuRaw := stringPtrValue(item.Cpu)
	memoryGB := parsePositiveFloat(stringPtrValue(item.Mem), 0)
	endpoints := resolveRDSEndpoints(item)
	connectionString, port := resolveRDSConnection(endpoints, item.Port)
	return RDSInstance{
		ID:                strings.TrimSpace(item.Id),
		NodeID:            nodeID,
		Name:              firstNonEmpty(stringPtrValue(item.Alias), item.Name, item.Id),
		Provider:          ProviderName,
		RegionID:          firstNonEmpty(item.Region, fallbackRegion),
		ZoneID:            zoneID,
		Engine:            engine,
		EngineVersion:     engineVersion,
		Status:            strings.TrimSpace(item.Status),
		Type:              strings.TrimSpace(item.Type),
		Class:             strings.TrimSpace(item.FlavorRef),
		CPU:               parsePositiveInt(cpuRaw, 0),
		CPURaw:            cpuRaw,
		MemoryMB:          int64(math.Round(memoryGB * 1024)),
		StorageGB:         int(volumeSize),
		StorageType:       volumeType,
		NetworkType:       "VPC",
		ConnectionString:  connectionString,
		Port:              port,
		VpcID:             strings.TrimSpace(item.VpcId),
		VSwitchID:         strings.TrimSpace(item.SubnetId),
		CreatedAt:         strings.TrimSpace(item.Created),
		ExpiredAt:         expiredAt,
		PayType:           chargeType,
		Endpoints:         endpoints,
		ExpiresInDays:     expiration.ExpiresInDays,
		ExpirationStatus:  expiration.Status,
		ExpirationMessage: expiration.Message,
	}
}

func (c *Client) rdsStorageUsage(client *rds.RdsClient, instanceID string, storageGB int, engine string) (*RDSResourceUsage, error) {
	if client == nil {
		return nil, fmt.Errorf("华为云 RDS 客户端未初始化")
	}
	resp, err := client.ShowStorageUsedSpace(&rdsmodel.ShowStorageUsedSpaceRequest{
		InstanceId: strings.TrimSpace(instanceID),
	})
	if err != nil {
		return nil, err
	}
	usedGB := parsePositiveFloat(stringPtrValue(resp.Used), 0)
	usage := &RDSResourceUsage{
		DBInstanceID:  strings.TrimSpace(instanceID),
		Engine:        strings.TrimSpace(engine),
		DiskUsedBytes: int64(usedGB * 1024 * 1024 * 1024),
		Source:        "rds.ShowStorageUsedSpace",
	}
	if storageGB > 0 && usedGB > 0 {
		percent := (usedGB / float64(storageGB)) * 100
		usage.StorageUsagePercent = &percent
	}
	return usage, nil
}

func resolveRDSDatastore(datastore *rdsmodel.Datastore) (string, string) {
	if datastore == nil {
		return "", ""
	}
	return datastore.Type.Value(), firstNonEmpty(stringPtrValue(datastore.CompleteVersion), datastore.Version)
}

func resolveRDSNodeInfo(nodes []rdsmodel.NodeResponse) (string, string) {
	if len(nodes) == 0 {
		return "", ""
	}
	zones := make([]string, 0, len(nodes))
	nodeID := ""
	for _, node := range nodes {
		if nodeID == "" {
			nodeID = strings.TrimSpace(node.Id)
		}
		zones = append(zones, node.AvailabilityZone)
	}
	return nodeID, strings.Join(uniqueStrings(zones), ",")
}

func resolveRDSVolume(volume *rdsmodel.VolumeForInstanceResponse) (string, int32) {
	if volume == nil {
		return "", 0
	}
	return volume.Type.Value(), volume.Size
}

func resolveRDSChargeMode(chargeInfo *rdsmodel.ChargeInfoResponse) string {
	if chargeInfo == nil {
		return ""
	}
	return chargeInfo.ChargeMode.Value()
}

func resolveRDSEndpoints(item rdsmodel.InstanceResponse) []RDSEndpoint {
	out := make([]RDSEndpoint, 0)
	for _, dnsName := range stringPtrSliceValue(item.PrivateDnsNames) {
		out = append(out, RDSEndpoint{
			ConnectionString:     dnsName,
			Port:                 strconv.Itoa(int(item.Port)),
			IPType:               "Private",
			ConnectionStringType: "Private",
			VPCID:                strings.TrimSpace(item.VpcId),
			VSwitchID:            strings.TrimSpace(item.SubnetId),
		})
	}
	for _, ip := range item.PrivateIps {
		out = append(out, RDSEndpoint{
			ConnectionString:     strings.TrimSpace(ip),
			Port:                 strconv.Itoa(int(item.Port)),
			IPAddress:            strings.TrimSpace(ip),
			IPType:               "Private",
			ConnectionStringType: "Private",
			VPCID:                strings.TrimSpace(item.VpcId),
			VSwitchID:            strings.TrimSpace(item.SubnetId),
		})
	}
	for _, dnsName := range stringPtrSliceValue(item.PublicDnsNames) {
		out = append(out, RDSEndpoint{
			ConnectionString:     dnsName,
			Port:                 strconv.Itoa(int(item.Port)),
			IPType:               "Public",
			ConnectionStringType: "Public",
		})
	}
	for _, ip := range item.PublicIps {
		out = append(out, RDSEndpoint{
			ConnectionString:     strings.TrimSpace(ip),
			Port:                 strconv.Itoa(int(item.Port)),
			IPAddress:            strings.TrimSpace(ip),
			IPType:               "Public",
			ConnectionStringType: "Public",
		})
	}
	return out
}

func resolveRDSConnection(endpoints []RDSEndpoint, port int32) (string, string) {
	portText := ""
	if port > 0 {
		portText = strconv.Itoa(int(port))
	}
	for _, endpoint := range endpoints {
		if strings.TrimSpace(endpoint.ConnectionString) == "" {
			continue
		}
		return strings.TrimSpace(endpoint.ConnectionString), firstNonEmpty(endpoint.Port, portText)
	}
	return "", portText
}

type expirationInfo struct {
	ExpiresInDays *int
	Status        string
	Message       string
}

func resolveExpiration(chargeType string, isSpot bool) expirationInfo {
	if isSpot {
		return expirationInfo{
			Status:  expirationStatusNoExpiration,
			Message: "竞价实例按量计费，无固定到期日",
		}
	}
	if strings.EqualFold(chargeType, chargeTypePostPaid) {
		return expirationInfo{
			Status:  expirationStatusNoExpiration,
			Message: "按需计费资源无固定到期日",
		}
	}
	return expirationInfo{
		Status:  expirationStatusUnknown,
		Message: "华为云 ECS 详情接口未返回到期时间",
	}
}

func resolveRDSExpiration(expiredAt, chargeType string, now time.Time) expirationInfo {
	expiredAt = strings.TrimSpace(expiredAt)
	chargeType = strings.TrimSpace(chargeType)
	if strings.EqualFold(chargeType, "postPaid") || strings.EqualFold(chargeType, chargeTypePostPaid) {
		return expirationInfo{
			Status:  expirationStatusNoExpiration,
			Message: "按需计费资源无固定到期日",
		}
	}
	if expiredAt == "" {
		return expirationInfo{
			Status:  expirationStatusUnknown,
			Message: "华为云 RDS 未返回到期时间",
		}
	}
	parsed, ok := parseHuaweiTime(expiredAt)
	if !ok {
		return expirationInfo{
			Status:  expirationStatusUnknown,
			Message: "华为云 RDS 到期时间格式暂未识别",
		}
	}
	days := int(math.Floor(parsed.Sub(now).Hours() / 24))
	switch {
	case days < 0:
		return expirationInfo{
			ExpiresInDays: &days,
			Status:        expirationStatusExpired,
			Message:       "资源已到期",
		}
	case days <= expiringSoonDays:
		return expirationInfo{
			ExpiresInDays: &days,
			Status:        expirationStatusExpiring,
			Message:       fmt.Sprintf("%d 天后到期", days),
		}
	default:
		return expirationInfo{
			ExpiresInDays: &days,
			Status:        expirationStatusNormal,
			Message:       fmt.Sprintf("%d 天后到期", days),
		}
	}
}

func parseHuaweiTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if value, err := time.Parse(layout, raw); err == nil {
			return value, true
		}
	}
	return time.Time{}, false
}

func resolveChargeType(metadata map[string]string) (string, bool) {
	mode := strings.TrimSpace(metadata["charging_mode"])
	switch mode {
	case "0":
		return chargeTypePostPaid, false
	case "1":
		return chargeTypePrePaid, false
	case "2":
		return chargeTypeSpot, true
	default:
		return "", false
	}
}

func resolveSpotStrategy(isSpot bool) string {
	if isSpot {
		return "Spot"
	}
	return ""
}

func resolveFlavorName(flavor *ecsmodel.ServerFlavor) string {
	if flavor == nil {
		return ""
	}
	return firstNonEmpty(flavor.Name, flavor.Id)
}

func resolveFlavorCPU(flavor *ecsmodel.ServerFlavor) string {
	if flavor == nil {
		return ""
	}
	return flavor.Vcpus
}

func resolveFlavorMemory(flavor *ecsmodel.ServerFlavor) string {
	if flavor == nil {
		return ""
	}
	return flavor.Ram
}

func resolveIPs(item ecsmodel.ServerDetail) ([]string, []string) {
	publicIPs := make([]string, 0)
	privateIPs := make([]string, 0)
	for _, addresses := range item.Addresses {
		for _, address := range addresses {
			if strings.TrimSpace(address.Addr) == "" || address.Version != "4" {
				continue
			}
			if address.OSEXTIPStype != nil && address.OSEXTIPStype.Value() == "floating" {
				publicIPs = append(publicIPs, address.Addr)
				continue
			}
			privateIPs = append(privateIPs, address.Addr)
		}
	}
	if item.NetworkInterfaces != nil {
		for _, nic := range *item.NetworkInterfaces {
			if nic.IpAddresses != nil {
				privateIPs = append(privateIPs, *nic.IpAddresses...)
			}
			if nic.Association != nil && nic.Association.PublicIpAddress != nil {
				publicIPs = append(publicIPs, *nic.Association.PublicIpAddress)
			}
		}
	}
	return uniqueStrings(publicIPs), uniqueStrings(privateIPs)
}

func resolveSubnetID(interfaces *[]ecsmodel.NetworkInterfaces) string {
	if interfaces == nil {
		return ""
	}
	for _, item := range *interfaces {
		if item.SubnetId != nil && strings.TrimSpace(*item.SubnetId) != "" {
			return strings.TrimSpace(*item.SubnetId)
		}
	}
	return ""
}

func mapMetricPoint(item cesmodel.Datapoint) MetricPoint {
	raw := map[string]any{
		"timestamp": item.Timestamp,
	}
	value := 0.0
	if item.Average != nil {
		value = *item.Average
		raw["average"] = *item.Average
	} else if item.Max != nil {
		value = *item.Max
		raw["max"] = *item.Max
	} else if item.Min != nil {
		value = *item.Min
		raw["min"] = *item.Min
	} else if item.Sum != nil {
		value = *item.Sum
		raw["sum"] = *item.Sum
	}
	if item.Unit != nil {
		raw["unit"] = *item.Unit
	}
	return MetricPoint{
		Timestamp: item.Timestamp,
		Value:     value,
		Raw:       raw,
	}
}

func resolveMetricSeriesUnit(points []MetricPoint, fallback string) string {
	for _, point := range points {
		raw, ok := point.Raw.(map[string]any)
		if !ok {
			continue
		}
		unit, ok := raw["unit"].(string)
		if ok && strings.TrimSpace(unit) != "" {
			return strings.TrimSpace(unit)
		}
	}
	return strings.TrimSpace(fallback)
}

func huaweiMetricUnit(namespace, metricName string) string {
	namespace = strings.TrimSpace(namespace)
	metricName = strings.TrimSpace(metricName)
	switch metricName {
	case "cpu_util", "mem_util", "mem_usedPercent", "disk_util_inband", "disk_usedPercent", "disk_util":
		return "%"
	case "rds001_cpu_util", "rds002_mem_util", "rds072_conn_usage":
		return "%"
	case "network_incoming_bytes_aggregate_rate", "network_outgoing_bytes_aggregate_rate",
		"network_incoming_bytes_rate_inband", "network_outgoing_bytes_rate_inband",
		"disk_read_bytes_rate", "disk_write_bytes_rate":
		return "Byte/s"
	default:
		if namespace == NamespaceAGT && strings.HasPrefix(metricName, "load_average") {
			return ""
		}
		return ""
	}
}

func metricDimensions(dimensions map[string]string) []string {
	keys := make([]string, 0, len(dimensions))
	for key := range dimensions {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(dimensions[key]) != "" {
			keys = append(keys, normalizeDimensionKey(key))
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		value := ""
		for rawKey, rawValue := range dimensions {
			if normalizeDimensionKey(rawKey) == key {
				value = strings.TrimSpace(rawValue)
				break
			}
		}
		if value != "" {
			out = append(out, key+","+value)
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func normalizeDimensionKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "instanceId" || key == "server_id" {
		return "instance_id"
	}
	return key
}

func huaweiPeriodEnum(periodSeconds int) cesmodel.ShowMetricDataRequestPeriod {
	enum := cesmodel.GetShowMetricDataRequestPeriodEnum()
	switch periodSeconds {
	case 1:
		return enum.E_1
	case 300:
		return enum.E_300
	case 1200:
		return enum.E_1200
	case 3600:
		return enum.E_3600
	case 14400:
		return enum.E_14400
	case 86400:
		return enum.E_86400
	default:
		return enum.E_60
	}
}

func parseHuaweiPeriodSeconds(raw string) int {
	value := parsePositiveInt(raw, 60)
	switch {
	case value <= 1:
		return 1
	case value <= 60:
		return 60
	case value <= 300:
		return 300
	case value <= 1200:
		return 1200
	case value <= 3600:
		return 3600
	case value <= 14400:
		return 14400
	default:
		return 86400
	}
}

func (c *Client) resolveECSRegions(requested []string) ([]string, bool) {
	explicitRegions := normalizeRegions(requested, Config{})
	if len(explicitRegions) > 0 {
		return explicitRegions, false
	}
	seedRegions := normalizeRegions(nil, c.config)
	return mergeRegionLists(seedRegions, defaultECSRegions), true
}

func (c *Client) resolveRDSRegions(requested []string) ([]string, bool) {
	explicitRegions := normalizeRegions(requested, Config{})
	if len(explicitRegions) > 0 {
		return explicitRegions, false
	}
	seedRegions := normalizeRegions(nil, c.config)
	return mergeRegionLists(seedRegions, defaultECSRegions), true
}

func summarizeRegionErrors(prefix string, regionErrors []string) error {
	if len(regionErrors) == 0 {
		return fmt.Errorf("%s", prefix)
	}
	message := regionErrors[0]
	if len(regionErrors) > 1 {
		message = fmt.Sprintf("%s；另有 %d 个地域失败", message, len(regionErrors)-1)
	}
	return fmt.Errorf("%s: %s", prefix, message)
}

func mergeRegionLists(lists ...[]string) []string {
	total := 0
	for _, values := range lists {
		total += len(values)
	}
	out := make([]string, 0, total)
	seen := make(map[string]struct{}, total)
	for _, values := range lists {
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

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range compactStrings(values) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
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

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parsePositiveFloat(raw string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func int32Ptr(value int32) *int32 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPtrSliceValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return compactStrings(*values)
}
