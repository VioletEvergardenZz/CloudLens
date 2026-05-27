// 本文件用于定义云资源应用层模型。
// 文件职责：沉淀 API、SQLite 和云厂商适配器之间共享的统一资源与风险合同。
// 边界与容错：只表达只读巡检数据，不包含任何云资源写操作。

package cloud

type Resource struct {
	ID                string            `json:"id"`
	AccountID         int64             `json:"accountId"`
	AccountName       string            `json:"accountName"`
	Provider          string            `json:"provider"`
	ResourceType      string            `json:"resourceType"`
	ResourceID        string            `json:"resourceId"`
	Name              string            `json:"name"`
	Region            string            `json:"region"`
	Zone              string            `json:"zone,omitempty"`
	Status            string            `json:"status"`
	PrivateIPs        []string          `json:"privateIps,omitempty"`
	PublicIPs         []string          `json:"publicIps,omitempty"`
	ChargeType        string            `json:"chargeType,omitempty"`
	ExpiredAt         string            `json:"expiredAt,omitempty"`
	ExpiresInDays     *int              `json:"expiresInDays,omitempty"`
	ExpirationStatus  string            `json:"expirationStatus,omitempty"`
	ExpirationMessage string            `json:"expirationMessage,omitempty"`
	Engine            string            `json:"engine,omitempty"`
	EngineVersion     string            `json:"engineVersion,omitempty"`
	NodeID            string            `json:"nodeId,omitempty"`
	Source            string            `json:"source"`
	SnapshotAt        string            `json:"snapshotAt"`
	LastError         string            `json:"lastError,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	RawJSON           string            `json:"-"`
}

type ResourceFilter struct {
	Provider     string
	ResourceType string
	AccountID    int64
	Region       string
}

type Risk struct {
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

type RiskSummary struct {
	Total      int            `json:"total"`
	Critical   int            `json:"critical"`
	Warning    int            `json:"warning"`
	Info       int            `json:"info"`
	ByAccount  map[string]int `json:"byAccount"`
	ByCategory map[string]int `json:"byCategory"`
}

type NodeLink struct {
	NodeName     string `json:"nodeName"`
	Cluster      string `json:"cluster,omitempty"`
	ProviderID   string `json:"providerId,omitempty"`
	InternalIP   string `json:"internalIp,omitempty"`
	Provider     string `json:"provider,omitempty"`
	AccountID    int64  `json:"accountId,omitempty"`
	AccountName  string `json:"accountName,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`
	ResourceName string `json:"resourceName,omitempty"`
	Region       string `json:"region,omitempty"`
	MatchType    string `json:"matchType"`
	Confidence   string `json:"confidence"`
	Evidence     string `json:"evidence,omitempty"`
}
