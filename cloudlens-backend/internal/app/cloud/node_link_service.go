// 本文件用于云主机与 Kubernetes Node 的关联判断。
// 文件职责：按 providerID、Node 标签和 InternalIP 建立只读匹配结果。

package cloud

import (
	"fmt"
	"net"
	"strings"

	k8smodel "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/k8s"
)

type NodeLinkService struct{}

func NewNodeLinkService() *NodeLinkService {
	return &NodeLinkService{}
}

func (s *NodeLinkService) BuildLinks(nodes []k8smodel.Node, resources []Resource, cluster string) []NodeLink {
	links := make([]NodeLink, 0, len(nodes))
	for _, node := range nodes {
		link := matchNode(node, resources, cluster)
		links = append(links, link)
	}
	return links
}

func matchNode(node k8smodel.Node, resources []Resource, cluster string) NodeLink {
	base := NodeLink{
		NodeName:   node.Name,
		Cluster:    cluster,
		ProviderID: node.ProviderID,
		InternalIP: node.InternalIP,
		MatchType:  "unmatched",
		Confidence: "low",
	}
	if resource, evidence := matchByLabel(node, resources); resource != nil {
		return fillNodeLink(base, *resource, "label", "high", evidence)
	}
	if resource, evidence := matchByProviderID(node.ProviderID, resources); resource != nil {
		return fillNodeLink(base, *resource, "provider_id", "high", evidence)
	}
	if resource, evidence := matchByInternalIP(node.InternalIP, resources); resource != nil {
		return fillNodeLink(base, *resource, "internal_ip", "medium", evidence)
	}
	if resource, evidence := matchByHostName(node, resources); resource != nil {
		return fillNodeLink(base, *resource, "hostname", "low", evidence)
	}
	return base
}

func matchByLabel(node k8smodel.Node, resources []Resource) (*Resource, string) {
	if len(node.Labels) == 0 {
		return nil, ""
	}
	instanceID := strings.TrimSpace(firstLabel(node.Labels, "cloudlens.io/instance-id", "node.cloudlens.io/instance-id"))
	if instanceID == "" {
		return nil, ""
	}
	for index := range resources {
		if resources[index].ResourceType == "ecs" && strings.EqualFold(resources[index].ResourceID, instanceID) {
			return &resources[index], "label cloudlens.io/instance-id=" + instanceID
		}
	}
	return nil, ""
}

func matchByProviderID(providerID string, resources []Resource) (*Resource, string) {
	instanceID := parseProviderInstanceID(providerID)
	if instanceID == "" {
		return nil, ""
	}
	for index := range resources {
		if resources[index].ResourceType == "ecs" && strings.EqualFold(resources[index].ResourceID, instanceID) {
			return &resources[index], "providerID=" + strings.TrimSpace(providerID)
		}
	}
	return nil, ""
}

func matchByInternalIP(internalIP string, resources []Resource) (*Resource, string) {
	ip := strings.TrimSpace(internalIP)
	if net.ParseIP(ip) == nil {
		return nil, ""
	}
	for index := range resources {
		if resources[index].ResourceType != "ecs" {
			continue
		}
		for _, privateIP := range resources[index].PrivateIPs {
			if strings.TrimSpace(privateIP) == ip {
				return &resources[index], "InternalIP=" + ip
			}
		}
	}
	return nil, ""
}

func matchByHostName(node k8smodel.Node, resources []Resource) (*Resource, string) {
	candidates := []string{node.Name, node.HostName}
	for index := range resources {
		if resources[index].ResourceType != "ecs" {
			continue
		}
		for _, candidate := range candidates {
			if candidate != "" && strings.EqualFold(candidate, resources[index].Name) {
				return &resources[index], "hostname=" + candidate
			}
		}
	}
	return nil, ""
}

func fillNodeLink(base NodeLink, resource Resource, matchType, confidence, evidence string) NodeLink {
	base.Provider = resource.Provider
	base.AccountID = resource.AccountID
	base.AccountName = resource.AccountName
	base.ResourceID = resource.ResourceID
	base.ResourceName = resource.Name
	base.Region = resource.Region
	base.MatchType = matchType
	base.Confidence = confidence
	base.Evidence = evidence
	return base
}

func parseProviderInstanceID(providerID string) string {
	trimmed := strings.TrimSpace(providerID)
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '/' || r == ':' || r == '#'
	})
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func firstLabel(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

func NodeLinkSummary(links []NodeLink) map[string]int {
	summary := map[string]int{"total": len(links), "matched": 0, "unmatched": 0}
	for _, link := range links {
		if link.MatchType == "unmatched" {
			summary["unmatched"]++
		} else {
			summary["matched"]++
			summary[fmt.Sprintf("match_%s", link.MatchType)]++
		}
	}
	return summary
}
