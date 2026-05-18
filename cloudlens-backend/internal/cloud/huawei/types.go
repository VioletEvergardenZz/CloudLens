// 本文件用于定义华为云只读云资产模型。
// 文件职责：让华为云 ECS 与 CES 监控复用控制台已有的通用字段合同。
// 边界与容错：只表达查询结果，不包含任何会修改云资源的操作参数。

package huawei

import "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/common"

type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	ProjectID       string
	Region          string
	Regions         []string
	MetricPeriod    string
}

type Instance = common.Instance

type RDSEndpoint struct {
	ConnectionString     string `json:"connectionString"`
	Port                 string `json:"port"`
	IPAddress            string `json:"ipAddress,omitempty"`
	IPType               string `json:"ipType,omitempty"`
	ConnectionStringType string `json:"connectionStringType,omitempty"`
	Availability         string `json:"availability,omitempty"`
	VPCID                string `json:"vpcId,omitempty"`
	VSwitchID            string `json:"vSwitchId,omitempty"`
}

type RDSResourceUsage struct {
	DBInstanceID        string   `json:"dbInstanceId,omitempty"`
	Engine              string   `json:"engine,omitempty"`
	DiskUsedBytes       int64    `json:"diskUsedBytes,omitempty"`
	StorageUsagePercent *float64 `json:"storageUsagePercent,omitempty"`
	Source              string   `json:"source,omitempty"`
}

type RDSInstance struct {
	ID                string            `json:"id"`
	NodeID            string            `json:"nodeId,omitempty"`
	Name              string            `json:"name"`
	Provider          string            `json:"provider"`
	RegionID          string            `json:"regionId"`
	ZoneID            string            `json:"zoneId"`
	Engine            string            `json:"engine"`
	EngineVersion     string            `json:"engineVersion"`
	Status            string            `json:"status"`
	Type              string            `json:"type,omitempty"`
	Category          string            `json:"category,omitempty"`
	Class             string            `json:"class,omitempty"`
	CPU               int               `json:"cpu,omitempty"`
	CPURaw            string            `json:"cpuRaw,omitempty"`
	MemoryMB          int64             `json:"memoryMb,omitempty"`
	StorageGB         int               `json:"storageGb,omitempty"`
	StorageType       string            `json:"storageType,omitempty"`
	NetworkType       string            `json:"networkType,omitempty"`
	ConnectionString  string            `json:"connectionString,omitempty"`
	Port              string            `json:"port,omitempty"`
	VpcID             string            `json:"vpcId,omitempty"`
	VSwitchID         string            `json:"vSwitchId,omitempty"`
	CreatedAt         string            `json:"createdAt"`
	ExpiredAt         string            `json:"expiredAt"`
	PayType           string            `json:"payType"`
	Endpoints         []RDSEndpoint     `json:"endpoints,omitempty"`
	ResourceUsage     *RDSResourceUsage `json:"resourceUsage,omitempty"`
	DetailErrors      []string          `json:"detailErrors,omitempty"`
	ExpiresInDays     *int              `json:"expiresInDays,omitempty"`
	ExpirationStatus  string            `json:"expirationStatus"`
	ExpirationMessage string            `json:"expirationMessage"`
}

type MetricPoint = common.MetricPoint
type MetricSeries = common.MetricSeries
