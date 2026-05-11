// 本文件用于定义阿里云只读云资产模型
// 文件职责：收敛 ECS、RDS 与监控返回给控制台的最小字段
// 边界与容错：只表达查询结果，不包含任何会修改云资源的操作参数

package aliyun

import "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/common"

type Config struct {
	AccessKeyID     string
	AccessKeySecret string
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
	DataSizeBytes       int64    `json:"dataSizeBytes,omitempty"`
	LogSizeBytes        int64    `json:"logSizeBytes,omitempty"`
	SQLSizeBytes        int64    `json:"sqlSizeBytes,omitempty"`
	BackupSizeBytes     int64    `json:"backupSizeBytes,omitempty"`
	StorageUsagePercent *float64 `json:"storageUsagePercent,omitempty"`
	Source              string   `json:"source,omitempty"`
}

type RDSInstance struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Provider           string            `json:"provider"`
	RegionID           string            `json:"regionId"`
	ZoneID             string            `json:"zoneId"`
	Engine             string            `json:"engine"`
	EngineVersion      string            `json:"engineVersion"`
	Status             string            `json:"status"`
	LockMode           string            `json:"lockMode,omitempty"`
	LockReason         string            `json:"lockReason,omitempty"`
	Type               string            `json:"type,omitempty"`
	Category           string            `json:"category,omitempty"`
	Class              string            `json:"class,omitempty"`
	ClassType          string            `json:"classType,omitempty"`
	CPU                int               `json:"cpu,omitempty"`
	CPURaw             string            `json:"cpuRaw,omitempty"`
	MemoryMB           int64             `json:"memoryMb,omitempty"`
	StorageGB          int               `json:"storageGb,omitempty"`
	StorageType        string            `json:"storageType,omitempty"`
	MaxConnections     int               `json:"maxConnections,omitempty"`
	MaxIOPS            int               `json:"maxIops,omitempty"`
	MaxIOMBPS          int               `json:"maxIombps,omitempty"`
	NetworkType        string            `json:"networkType,omitempty"`
	NetType            string            `json:"netType,omitempty"`
	ConnectionMode     string            `json:"connectionMode,omitempty"`
	ConnectionString   string            `json:"connectionString,omitempty"`
	Port               string            `json:"port,omitempty"`
	VpcID              string            `json:"vpcId,omitempty"`
	VSwitchID          string            `json:"vSwitchId,omitempty"`
	ResourceGroupID    string            `json:"resourceGroupId,omitempty"`
	AccountType        string            `json:"accountType,omitempty"`
	DeletionProtection bool              `json:"deletionProtection"`
	CreatedAt          string            `json:"createdAt"`
	ExpiredAt          string            `json:"expiredAt"`
	PayType            string            `json:"payType"`
	Endpoints          []RDSEndpoint     `json:"endpoints,omitempty"`
	ResourceUsage      *RDSResourceUsage `json:"resourceUsage,omitempty"`
	DetailErrors       []string          `json:"detailErrors,omitempty"`
	ExpiresInDays      *int              `json:"expiresInDays,omitempty"`
	ExpirationStatus   string            `json:"expirationStatus"`
	ExpirationMessage  string            `json:"expirationMessage"`
}

type MetricPoint = common.MetricPoint
type MetricSeries = common.MetricSeries
