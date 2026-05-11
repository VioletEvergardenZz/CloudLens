// 本文件用于华为云 ECS 与云监控 CES 只读查询。
// 文件职责：封装 ListServersDetails 与 ShowMetricData，避免 API 层直接依赖 SDK 细节。
// 边界与容错：只调用查询类 OpenAPI，不提供启动、停止、释放、修改等写操作。

package huawei

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	coreregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/region"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
	cesregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/region"
	ecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	ecsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/region"
)

const (
	ProviderName = "huawei"
	NamespaceECS = "SYS.ECS"
	NamespaceAGT = "AGT.ECS"

	expirationStatusNoExpiration = "no_expiration"
	expirationStatusUnknown      = "unknown"
	chargeTypePostPaid           = "PostPaid"
	chargeTypePrePaid            = "PrePaid"
	chargeTypeSpot               = "Spot"
)

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
	if config.Region == "" && len(config.Regions) == 0 {
		return nil, fmt.Errorf("华为云地域未配置，请在云账号管理中填写地域，或临时设置 HUAWEI_REGION/HUAWEI_REGIONS")
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
	instanceID := firstNonEmpty(dimensions["instance_id"], dimensions["instanceId"], dimensions["server_id"])
	if instanceID == "" {
		return nil, fmt.Errorf("instanceId 不能为空")
	}
	dimensionValues := metricDimensions(dimensions)
	if len(dimensionValues) == 0 {
		return nil, fmt.Errorf("华为云监控维度不能为空")
	}
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

func (c *Client) newECSClient(regionID string) (*ecs.EcsClient, error) {
	credentials, err := c.credentials()
	if err != nil {
		return nil, err
	}
	reg, err := ecsregion.SafeValueOf(regionID)
	if err != nil {
		reg = coreregion.NewRegion(regionID, fmt.Sprintf("https://ecs.%s.myhuaweicloud.com", regionID))
	}
	hcClient, err := ecs.EcsClientBuilder().WithRegion(reg).WithCredential(credentials).SafeBuild()
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
	hcClient, err := ces.CesClientBuilder().WithRegion(reg).WithCredential(credentials).SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("创建华为云 CES 只读客户端失败 region=%s: %w", regionID, err)
	}
	return ces.NewCesClient(hcClient), nil
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
