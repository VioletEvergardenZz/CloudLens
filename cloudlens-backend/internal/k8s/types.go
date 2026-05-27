// 本文件用于定义 Kubernetes 只读巡检模型。
// 文件职责：把 client-go 原生对象收敛成 CloudLens 控制台可直接消费的轻量结构。
// 边界与容错：只表达查询结果，不包含创建、删除、扩缩容等写操作参数。

package k8s

type Options struct {
	KubeconfigPath string
	Context        string
}

type Overview struct {
	OK          bool           `json:"ok"`
	Source      string         `json:"source"`
	CollectedAt string         `json:"collectedAt"`
	Cluster     ClusterSummary `json:"cluster"`
	Summary     Summary        `json:"summary"`
	Nodes       []Node         `json:"nodes"`
	Namespaces  []Namespace    `json:"namespaces"`
	Pods        []Pod          `json:"pods"`
	Deployments []Deployment   `json:"deployments"`
	Events      []Event        `json:"events"`
	Issues      []Issue        `json:"issues"`
}

type ClusterSummary struct {
	Context string `json:"context,omitempty"`
	Server  string `json:"server,omitempty"`
	Version string `json:"version,omitempty"`
}

type Summary struct {
	NodeTotal             int `json:"nodeTotal"`
	NodeReady             int `json:"nodeReady"`
	NamespaceTotal        int `json:"namespaceTotal"`
	PodTotal              int `json:"podTotal"`
	PodRunning            int `json:"podRunning"`
	PodPending            int `json:"podPending"`
	PodFailed             int `json:"podFailed"`
	DeploymentTotal       int `json:"deploymentTotal"`
	DeploymentUnavailable int `json:"deploymentUnavailable"`
	WarningEventTotal     int `json:"warningEventTotal"`
	IssueTotal            int `json:"issueTotal"`
}

type Node struct {
	Name             string            `json:"name"`
	Ready            bool              `json:"ready"`
	Status           string            `json:"status"`
	ProviderID       string            `json:"providerId,omitempty"`
	InternalIP       string            `json:"internalIp,omitempty"`
	ExternalIP       string            `json:"externalIp,omitempty"`
	HostName         string            `json:"hostName,omitempty"`
	KubeletVersion   string            `json:"kubeletVersion,omitempty"`
	OSImage          string            `json:"osImage,omitempty"`
	ContainerRuntime string            `json:"containerRuntime,omitempty"`
	CPU              string            `json:"cpu,omitempty"`
	Memory           string            `json:"memory,omitempty"`
	PodCIDR          string            `json:"podCidr,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Taints           []string          `json:"taints,omitempty"`
	CreatedAt        string            `json:"createdAt,omitempty"`
}

type Namespace struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Pod struct {
	Namespace    string   `json:"namespace"`
	Name         string   `json:"name"`
	NodeName     string   `json:"nodeName,omitempty"`
	Phase        string   `json:"phase"`
	Ready        bool     `json:"ready"`
	Reason       string   `json:"reason,omitempty"`
	Message      string   `json:"message,omitempty"`
	RestartCount int32    `json:"restartCount"`
	Images       []string `json:"images,omitempty"`
	CreatedAt    string   `json:"createdAt,omitempty"`
}

type Deployment struct {
	Namespace           string `json:"namespace"`
	Name                string `json:"name"`
	Replicas            int32  `json:"replicas"`
	ReadyReplicas       int32  `json:"readyReplicas"`
	AvailableReplicas   int32  `json:"availableReplicas"`
	UnavailableReplicas int32  `json:"unavailableReplicas"`
	Status              string `json:"status"`
	Reason              string `json:"reason,omitempty"`
	Message             string `json:"message,omitempty"`
	CreatedAt           string `json:"createdAt,omitempty"`
}

type Event struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	InvolvedKind   string `json:"involvedKind"`
	InvolvedName   string `json:"involvedName"`
	Count          int32  `json:"count"`
	LastObservedAt string `json:"lastObservedAt,omitempty"`
}

type Issue struct {
	ID           string `json:"id"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	ResourceType string `json:"resourceType"`
	Namespace    string `json:"namespace,omitempty"`
	ResourceName string `json:"resourceName"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion"`
	Evidence     string `json:"evidence,omitempty"`
}
