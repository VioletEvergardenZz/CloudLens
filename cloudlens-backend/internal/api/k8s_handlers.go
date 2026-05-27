// 本文件用于 Kubernetes 只读巡检接口。
// 文件职责：把 kind 或真实集群的只读资源快照暴露给控制台。
// 边界与容错：接口不执行任何 Kubernetes 写操作，无法连接集群时返回可读降级信息。

package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	appcloud "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/app/cloud"
	k8sclient "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/k8s"
)

const defaultK8sRequestTimeout = 8 * time.Second

func (h *handler) k8sOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CLOUDLENS_K8S_ENABLED")), "false") {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"enabled": false,
			"error":   "Kubernetes 巡检未启用",
		})
		return
	}

	timeout := defaultK8sRequestTimeout
	if raw := strings.TrimSpace(os.Getenv("CLOUDLENS_K8S_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	client, err := k8sclient.NewClient(k8sclient.Options{})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"enabled":    true,
			"source":     "unavailable",
			"error":      err.Error(),
			"suggestion": "本地 kind 场景请确认 KUBECONFIG 指向可用集群；集群内部署场景请按文档启用只读 ServiceAccount。",
		})
		return
	}

	overview, err := client.Overview(ctx, firstCloudQuery(r, "namespace", "ns"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"enabled":    true,
			"source":     "kubernetes",
			"error":      err.Error(),
			"suggestion": "请检查 kubeconfig 当前 context、集群连通性以及只读权限是否包含 Node、Pod、Deployment 和 Event。",
		})
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (h *handler) k8sNodeLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if h == nil || h.nodeLinkService == nil || h.resourceService == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "K8s 关联服务未初始化",
			"items": []any{},
		})
		return
	}
	if err := h.rebuildCloudResourceIndexFromSnapshots(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"items": []any{},
		})
		return
	}
	resources, err := h.resourceService.List(parseCloudResourceFilter(r))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"items": []any{},
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), defaultK8sRequestTimeout)
	defer cancel()
	client, err := k8sclient.NewClient(k8sclient.Options{})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"items": []any{},
		})
		return
	}
	overview, err := client.Overview(ctx, firstCloudQuery(r, "namespace", "ns"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"items": []any{},
		})
		return
	}
	clusterName := firstCloudString(overview.Cluster.Context, overview.Cluster.Server)
	links := h.nodeLinkService.BuildLinks(overview.Nodes, resources, clusterName)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"items":   links,
		"summary": appcloud.NodeLinkSummary(links),
	})
}
