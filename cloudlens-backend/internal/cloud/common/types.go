// 本文件用于定义多云资源控制台的通用只读模型。
// 文件职责：收敛不同云厂商 ECS/云监控返回给前端的共同字段。
// 边界与容错：只表达查询结果，不包含任何会修改云资源的操作参数。

package common

type Instance struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	HostName     string   `json:"hostName"`
	Provider     string   `json:"provider"`
	RegionID     string   `json:"regionId"`
	ZoneID       string   `json:"zoneId"`
	Status       string   `json:"status"`
	OSName       string   `json:"osName"`
	OSType       string   `json:"osType"`
	Type         string   `json:"type"`
	ChargeType   string   `json:"chargeType"`
	IsSpot       bool     `json:"isSpot"`
	SpotStrategy string   `json:"spotStrategy,omitempty"`
	CPU          int      `json:"cpu"`
	MemoryMB     int      `json:"memoryMb"`
	PublicIPs    []string `json:"publicIps"`
	EipAddress   string   `json:"eipAddress,omitempty"`
	EipID        string   `json:"eipId,omitempty"`
	PrivateIPs   []string `json:"privateIps"`
	VpcID        string   `json:"vpcId"`
	VSwitchID    string   `json:"vSwitchId"`
	SecurityIDs  []string `json:"securityGroupIds"`
	CreatedAt    string   `json:"createdAt"`
	ExpiredAt    string   `json:"expiredAt"`
	// ExpiresInDays 为不高估的完整剩余天数；按量、抢占式或云厂商未返回时为空。
	ExpiresInDays     *int   `json:"expiresInDays,omitempty"`
	ExpirationStatus  string `json:"expirationStatus"`
	ExpirationMessage string `json:"expirationMessage"`
}

type MetricPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
	Raw       any     `json:"raw,omitempty"`
}

type MetricSeries struct {
	InstanceID  string        `json:"instanceId"`
	RegionID    string        `json:"regionId"`
	Namespace   string        `json:"namespace"`
	MetricName  string        `json:"metricName"`
	Label       string        `json:"label,omitempty"`
	SubKey      string        `json:"subKey,omitempty"`
	Unit        string        `json:"unit,omitempty"`
	ValueFormat string        `json:"valueFormat,omitempty"`
	Period      string        `json:"period"`
	Points      []MetricPoint `json:"points"`
}
