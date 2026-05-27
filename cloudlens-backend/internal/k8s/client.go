// 本文件用于 Kubernetes 只读查询。
// 文件职责：基于 client-go 读取 kind 或真实集群中的 Node、Pod、Deployment 和 Event。
// 边界与容错：只使用 List/Get 类接口，避免 CloudLens 变成 Kubernetes 写操作管理面。

package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultIssueRestartThreshold = int32(3)
	maxWarningEvents             = 50
)

type Client struct {
	clientset *kubernetes.Clientset
	source    string
	context   string
	server    string
}

func NewClient(options Options) (*Client, error) {
	cfg, source, contextName, err := buildRestConfig(options)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Kubernetes 客户端失败: %w", err)
	}
	return &Client{
		clientset: clientset,
		source:    source,
		context:   contextName,
		server:    strings.TrimSpace(cfg.Host),
	}, nil
}

func (c *Client) Overview(ctx context.Context, namespace string) (*Overview, error) {
	if c == nil || c.clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = metav1.NamespaceAll
	}

	version := ""
	if serverVersion, err := c.clientset.Discovery().ServerVersion(); err == nil && serverVersion != nil {
		version = serverVersion.GitVersion
	}

	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Kubernetes Node 失败: %w", err)
	}
	namespaceList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Kubernetes Namespace 失败: %w", err)
	}
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Kubernetes Pod 失败: %w", err)
	}
	deploymentList, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Kubernetes Deployment 失败: %w", err)
	}
	eventList, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取 Kubernetes Event 失败: %w", err)
	}

	nodes := mapNodes(nodeList.Items)
	namespaces := mapNamespaces(namespaceList.Items)
	pods := mapPods(podList.Items)
	deployments := mapDeployments(deploymentList.Items)
	events := mapEvents(eventList.Items)
	issues := buildIssues(nodes, pods, deployments, events)
	summary := buildSummary(nodes, namespaces, pods, deployments, events, issues)

	return &Overview{
		OK:          true,
		Source:      c.source,
		CollectedAt: time.Now().UTC().Format(time.RFC3339),
		Cluster: ClusterSummary{
			Context: c.context,
			Server:  c.server,
			Version: version,
		},
		Summary:     summary,
		Nodes:       nodes,
		Namespaces:  namespaces,
		Pods:        pods,
		Deployments: deployments,
		Events:      events,
		Issues:      issues,
	}, nil
}

func buildRestConfig(options Options) (*rest.Config, string, string, error) {
	kubeconfigPath := strings.TrimSpace(options.KubeconfigPath)
	if kubeconfigPath == "" {
		kubeconfigPath = strings.TrimSpace(os.Getenv("CLOUDLENS_K8S_KUBECONFIG"))
	}
	if kubeconfigPath == "" {
		kubeconfigPath = strings.TrimSpace(os.Getenv("KUBECONFIG"))
	}
	contextName := strings.TrimSpace(options.Context)
	if contextName == "" {
		contextName = strings.TrimSpace(os.Getenv("CLOUDLENS_K8S_CONTEXT"))
	}

	if kubeconfigPath != "" || defaultKubeconfigExists() {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		if kubeconfigPath != "" {
			rules.ExplicitPath = kubeconfigPath
		}
		overrides := &clientcmd.ConfigOverrides{}
		if contextName != "" {
			overrides.CurrentContext = contextName
		}
		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
		rawConfig, _ := clientConfig.RawConfig()
		if contextName == "" {
			contextName = rawConfig.CurrentContext
		}
		cfg, err := clientConfig.ClientConfig()
		if err == nil {
			source := "kubeconfig"
			if rules.ExplicitPath != "" {
				source = rules.ExplicitPath
			}
			return cfg, source, contextName, nil
		}
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, "", "", fmt.Errorf("未找到可用 Kubernetes 配置，请设置 KUBECONFIG、CLOUDLENS_K8S_KUBECONFIG，或在集群内启用只读 ServiceAccount: %w", err)
	}
	return cfg, "in-cluster", contextName, nil
}

func defaultKubeconfigExists() bool {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(home, ".kube", "config"))
	return err == nil && !info.IsDir()
}

func mapNodes(items []corev1.Node) []Node {
	out := make([]Node, 0, len(items))
	for _, item := range items {
		ready := nodeReady(item)
		out = append(out, Node{
			Name:             item.Name,
			Ready:            ready,
			Status:           nodeStatusLabel(ready),
			ProviderID:       strings.TrimSpace(item.Spec.ProviderID),
			InternalIP:       nodeAddress(item.Status.Addresses, corev1.NodeInternalIP),
			ExternalIP:       nodeAddress(item.Status.Addresses, corev1.NodeExternalIP),
			HostName:         nodeAddress(item.Status.Addresses, corev1.NodeHostName),
			KubeletVersion:   item.Status.NodeInfo.KubeletVersion,
			OSImage:          item.Status.NodeInfo.OSImage,
			ContainerRuntime: item.Status.NodeInfo.ContainerRuntimeVersion,
			CPU:              quantityString(item.Status.Capacity.Cpu()),
			Memory:           quantityString(item.Status.Capacity.Memory()),
			PodCIDR:          strings.TrimSpace(item.Spec.PodCIDR),
			Labels:           stableMapCopy(item.Labels),
			Taints:           mapTaints(item.Spec.Taints),
			CreatedAt:        item.CreationTimestamp.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func mapNamespaces(items []corev1.Namespace) []Namespace {
	out := make([]Namespace, 0, len(items))
	for _, item := range items {
		out = append(out, Namespace{
			Name:      item.Name,
			Status:    string(item.Status.Phase),
			CreatedAt: item.CreationTimestamp.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func mapPods(items []corev1.Pod) []Pod {
	out := make([]Pod, 0, len(items))
	for _, item := range items {
		reason, message := podReason(item)
		out = append(out, Pod{
			Namespace:    item.Namespace,
			Name:         item.Name,
			NodeName:     item.Spec.NodeName,
			Phase:        string(item.Status.Phase),
			Ready:        podReady(item),
			Reason:       reason,
			Message:      message,
			RestartCount: podRestartCount(item),
			Images:       podImages(item),
			CreatedAt:    item.CreationTimestamp.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out
}

func mapDeployments(items []appsv1.Deployment) []Deployment {
	out := make([]Deployment, 0, len(items))
	for _, item := range items {
		replicas := int32(1)
		if item.Spec.Replicas != nil {
			replicas = *item.Spec.Replicas
		}
		reason, message := deploymentReason(item)
		status := "available"
		if item.Status.AvailableReplicas < replicas {
			status = "unavailable"
		}
		out = append(out, Deployment{
			Namespace:           item.Namespace,
			Name:                item.Name,
			Replicas:            replicas,
			ReadyReplicas:       item.Status.ReadyReplicas,
			AvailableReplicas:   item.Status.AvailableReplicas,
			UnavailableReplicas: item.Status.UnavailableReplicas,
			Status:              status,
			Reason:              reason,
			Message:             message,
			CreatedAt:           item.CreationTimestamp.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace == out[j].Namespace {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out
}

func mapEvents(items []corev1.Event) []Event {
	out := make([]Event, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Type) != string(corev1.EventTypeWarning) {
			continue
		}
		out = append(out, Event{
			Namespace:      item.Namespace,
			Name:           item.Name,
			Type:           item.Type,
			Reason:         item.Reason,
			Message:        item.Message,
			InvolvedKind:   item.InvolvedObject.Kind,
			InvolvedName:   item.InvolvedObject.Name,
			Count:          item.Count,
			LastObservedAt: eventObservedAt(item),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastObservedAt > out[j].LastObservedAt
	})
	if len(out) > maxWarningEvents {
		out = out[:maxWarningEvents]
	}
	return out
}

func buildSummary(nodes []Node, namespaces []Namespace, pods []Pod, deployments []Deployment, events []Event, issues []Issue) Summary {
	summary := Summary{
		NodeTotal:         len(nodes),
		NamespaceTotal:    len(namespaces),
		PodTotal:          len(pods),
		DeploymentTotal:   len(deployments),
		WarningEventTotal: len(events),
		IssueTotal:        len(issues),
	}
	for _, node := range nodes {
		if node.Ready {
			summary.NodeReady++
		}
	}
	for _, pod := range pods {
		switch strings.ToLower(pod.Phase) {
		case "running":
			summary.PodRunning++
		case "pending":
			summary.PodPending++
		case "failed":
			summary.PodFailed++
		}
	}
	for _, deployment := range deployments {
		if deployment.Status != "available" {
			summary.DeploymentUnavailable++
		}
	}
	return summary
}

func buildIssues(nodes []Node, pods []Pod, deployments []Deployment, events []Event) []Issue {
	issues := make([]Issue, 0)
	for _, node := range nodes {
		if node.Ready {
			continue
		}
		issues = append(issues, Issue{
			ID:           "node:" + node.Name + ":not-ready",
			Severity:     "critical",
			Category:     "node_status",
			ResourceType: "node",
			ResourceName: node.Name,
			Message:      "Kubernetes 节点未处于 Ready 状态",
			Suggestion:   "先检查节点 kubelet、容器运行时、磁盘压力和网络连通性，再回到云主机视图确认实例状态。",
			Evidence:     node.Status,
		})
	}
	for _, pod := range pods {
		if pod.Phase == string(corev1.PodRunning) && pod.Ready && pod.RestartCount < defaultIssueRestartThreshold {
			continue
		}
		severity := "warning"
		if pod.Phase == string(corev1.PodFailed) || isHardPodReason(pod.Reason) {
			severity = "critical"
		}
		message := fmt.Sprintf("Pod 当前状态为 %s", pod.Phase)
		if strings.TrimSpace(pod.Reason) != "" {
			message = fmt.Sprintf("Pod 当前状态为 %s，原因：%s", pod.Phase, pod.Reason)
		}
		if pod.RestartCount >= defaultIssueRestartThreshold {
			message = fmt.Sprintf("%s，容器累计重启 %d 次", message, pod.RestartCount)
		}
		issues = append(issues, Issue{
			ID:           fmt.Sprintf("pod:%s:%s:%s", pod.Namespace, pod.Name, pod.Reason),
			Severity:     severity,
			Category:     "pod_status",
			ResourceType: "pod",
			Namespace:    pod.Namespace,
			ResourceName: pod.Name,
			Message:      message,
			Suggestion:   "查看 Pod 事件、镜像拉取、资源请求和挂载配置；如果集中在同一 Node，再关联云主机状态继续排查。",
			Evidence:     strings.TrimSpace(pod.Message),
		})
	}
	for _, deployment := range deployments {
		if deployment.Status == "available" {
			continue
		}
		issues = append(issues, Issue{
			ID:           fmt.Sprintf("deployment:%s:%s:unavailable", deployment.Namespace, deployment.Name),
			Severity:     "warning",
			Category:     "workload_status",
			ResourceType: "deployment",
			Namespace:    deployment.Namespace,
			ResourceName: deployment.Name,
			Message:      fmt.Sprintf("Deployment 可用副本 %d/%d", deployment.AvailableReplicas, deployment.Replicas),
			Suggestion:   "检查副本调度、镜像、探针和资源限制，确认异常 Pod 是否集中在同一个节点。",
			Evidence:     firstNonEmpty(deployment.Message, deployment.Reason),
		})
	}
	for _, event := range events {
		issues = append(issues, Issue{
			ID:           fmt.Sprintf("event:%s:%s:%s", event.Namespace, event.InvolvedName, event.Reason),
			Severity:     "info",
			Category:     "warning_event",
			ResourceType: strings.ToLower(firstNonEmpty(event.InvolvedKind, "event")),
			Namespace:    event.Namespace,
			ResourceName: firstNonEmpty(event.InvolvedName, event.Name),
			Message:      fmt.Sprintf("K8s Warning 事件：%s", event.Reason),
			Suggestion:   "结合事件时间和相关 Pod/Node 状态判断是否仍在影响当前巡检。",
			Evidence:     strings.TrimSpace(event.Message),
		})
	}
	sort.Slice(issues, func(i, j int) bool {
		leftRank := issueSeverityRank(issues[i].Severity)
		rightRank := issueSeverityRank(issues[j].Severity)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return issues[i].ID < issues[j].ID
	})
	return issues
}

func nodeReady(item corev1.Node) bool {
	for _, condition := range item.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func nodeStatusLabel(ready bool) string {
	if ready {
		return "Ready"
	}
	return "NotReady"
}

func nodeAddress(addresses []corev1.NodeAddress, addressType corev1.NodeAddressType) string {
	for _, address := range addresses {
		if address.Type == addressType && strings.TrimSpace(address.Address) != "" {
			return strings.TrimSpace(address.Address)
		}
	}
	return ""
}

func quantityString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func mapTaints(taints []corev1.Taint) []string {
	out := make([]string, 0, len(taints))
	for _, taint := range taints {
		value := taint.Key
		if strings.TrimSpace(taint.Value) != "" {
			value += "=" + taint.Value
		}
		if strings.TrimSpace(string(taint.Effect)) != "" {
			value += ":" + string(taint.Effect)
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stableMapCopy(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func podReady(item corev1.Pod) bool {
	for _, condition := range item.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podRestartCount(item corev1.Pod) int32 {
	var total int32
	for _, status := range item.Status.ContainerStatuses {
		total += status.RestartCount
	}
	for _, status := range item.Status.InitContainerStatuses {
		total += status.RestartCount
	}
	return total
}

func podImages(item corev1.Pod) []string {
	out := make([]string, 0, len(item.Spec.Containers)+len(item.Spec.InitContainers))
	seen := make(map[string]struct{})
	for _, container := range append(item.Spec.InitContainers, item.Spec.Containers...) {
		image := strings.TrimSpace(container.Image)
		if image == "" {
			continue
		}
		if _, ok := seen[image]; ok {
			continue
		}
		seen[image] = struct{}{}
		out = append(out, image)
	}
	sort.Strings(out)
	return out
}

func podReason(item corev1.Pod) (string, string) {
	if strings.TrimSpace(item.Status.Reason) != "" || strings.TrimSpace(item.Status.Message) != "" {
		return strings.TrimSpace(item.Status.Reason), strings.TrimSpace(item.Status.Message)
	}
	for _, status := range append(item.Status.InitContainerStatuses, item.Status.ContainerStatuses...) {
		if status.State.Waiting != nil {
			return strings.TrimSpace(status.State.Waiting.Reason), strings.TrimSpace(status.State.Waiting.Message)
		}
		if status.State.Terminated != nil {
			return strings.TrimSpace(status.State.Terminated.Reason), strings.TrimSpace(status.State.Terminated.Message)
		}
	}
	return "", ""
}

func deploymentReason(item appsv1.Deployment) (string, string) {
	for _, condition := range item.Status.Conditions {
		if condition.Status == corev1.ConditionFalse || condition.Status == corev1.ConditionUnknown {
			return strings.TrimSpace(condition.Reason), strings.TrimSpace(condition.Message)
		}
	}
	return "", ""
}

func eventObservedAt(item corev1.Event) string {
	if !item.EventTime.IsZero() {
		return item.EventTime.UTC().Format(time.RFC3339)
	}
	if !item.LastTimestamp.IsZero() {
		return item.LastTimestamp.UTC().Format(time.RFC3339)
	}
	return item.CreationTimestamp.UTC().Format(time.RFC3339)
}

func isHardPodReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "CreateContainerError", "RunContainerError":
		return true
	default:
		return false
	}
}

func issueSeverityRank(severity string) int {
	switch strings.TrimSpace(severity) {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
