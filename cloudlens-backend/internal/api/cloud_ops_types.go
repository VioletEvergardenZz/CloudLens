// 本文件用于云资产运维体检共享类型。
// 文件职责：集中定义诊断、风险、报告和快照解析结构。

package api

import "time"

const (
	cloudSnapshotFreshDuration = 30 * time.Minute
	cloudSnapshotStaleDuration = 6 * time.Hour
)

type cloudOpsCheck struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type cloudSnapshotSummary struct {
	AccountID     int64  `json:"accountId"`
	Provider      string `json:"provider"`
	ResourceType  string `json:"resourceType"`
	Total         int    `json:"total"`
	Source        string `json:"source"`
	LastSuccessAt string `json:"lastSuccessAt"`
	LastError     string `json:"lastError,omitempty"`
	AgeSeconds    int64  `json:"ageSeconds"`
	Status        string `json:"status"`
}

type cloudAccountDiagnostic struct {
	Account             cloudAccountRecord     `json:"account"`
	Status              string                 `json:"status"`
	ExpectedPermissions []string               `json:"expectedPermissions"`
	Checks              []cloudOpsCheck        `json:"checks"`
	Snapshots           []cloudSnapshotSummary `json:"snapshots"`
}

type cloudRiskItem struct {
	ID           string `json:"id"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Provider     string `json:"provider"`
	AccountID    int64  `json:"accountId"`
	AccountName  string `json:"accountName"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	ResourceName string `json:"resourceName"`
	Region       string `json:"region"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion"`
	Evidence     string `json:"evidence,omitempty"`
	DetectedAt   string `json:"detectedAt"`
}

type cloudRiskSummary struct {
	Total      int            `json:"total"`
	Critical   int            `json:"critical"`
	Warning    int            `json:"warning"`
	Info       int            `json:"info"`
	ByAccount  map[string]int `json:"byAccount"`
	ByCategory map[string]int `json:"byCategory"`
}

type cloudInspectionReport struct {
	GeneratedAt string                   `json:"generatedAt"`
	Status      string                   `json:"status"`
	Summary     map[string]int           `json:"summary"`
	Diagnostics []cloudAccountDiagnostic `json:"diagnostics"`
	Risks       []cloudRiskItem          `json:"risks"`
	Runtime     []cloudOpsCheck          `json:"runtime"`
	NextActions []string                 `json:"nextActions"`
}

type cloudSnapshotResource struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Provider          string                  `json:"provider"`
	RegionID          string                  `json:"regionId"`
	Status            string                  `json:"status"`
	PublicIPs         []string                `json:"publicIps"`
	EipAddress        string                  `json:"eipAddress"`
	ExpiredAt         string                  `json:"expiredAt"`
	ExpiresInDays     *int                    `json:"expiresInDays"`
	ExpirationStatus  string                  `json:"expirationStatus"`
	ExpirationMessage string                  `json:"expirationMessage"`
	ResourceUsage     *cloudSnapshotUsage     `json:"resourceUsage"`
	ConnectionString  string                  `json:"connectionString"`
	Endpoints         []cloudSnapshotEndpoint `json:"endpoints"`
	DetailErrors      []string                `json:"detailErrors"`
}

type cloudSnapshotUsage struct {
	StorageUsagePercent *float64 `json:"storageUsagePercent"`
	DiskUsedBytes       int64    `json:"diskUsedBytes"`
	Source              string   `json:"source"`
}

type cloudSnapshotEndpoint struct {
	ConnectionString string `json:"connectionString"`
	IPType           string `json:"ipType"`
	VPCID            string `json:"vpcId"`
}
