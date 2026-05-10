// 本文件用于定义阿里云只读云资产模型
// 文件职责：收敛 ECS 与云监控返回给控制台的最小字段
// 边界与容错：只表达查询结果，不包含任何会修改云资源的操作参数

package aliyun

type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	Region          string
	Regions         []string
	MetricPeriod    string
}

type Instance struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	HostName    string   `json:"hostName"`
	Provider    string   `json:"provider"`
	RegionID    string   `json:"regionId"`
	ZoneID      string   `json:"zoneId"`
	Status      string   `json:"status"`
	OSName      string   `json:"osName"`
	OSType      string   `json:"osType"`
	Type        string   `json:"type"`
	CPU         int      `json:"cpu"`
	MemoryMB    int      `json:"memoryMb"`
	PublicIPs   []string `json:"publicIps"`
	EipAddress  string   `json:"eipAddress,omitempty"`
	EipID       string   `json:"eipId,omitempty"`
	PrivateIPs  []string `json:"privateIps"`
	VpcID       string   `json:"vpcId"`
	VSwitchID   string   `json:"vSwitchId"`
	SecurityIDs []string `json:"securityGroupIds"`
	CreatedAt   string   `json:"createdAt"`
	ExpiredAt   string   `json:"expiredAt"`
}

type MetricPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
	Raw       any     `json:"raw,omitempty"`
}

type MetricSeries struct {
	InstanceID string        `json:"instanceId"`
	RegionID   string        `json:"regionId"`
	Namespace  string        `json:"namespace"`
	MetricName string        `json:"metricName"`
	Period     string        `json:"period"`
	Points     []MetricPoint `json:"points"`
}
