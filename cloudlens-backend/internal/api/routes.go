// 本文件用于集中维护 HTTP 路由地图。
// 文件职责：把接口按业务域分组，降低 server.go 的阅读负担。

package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *handler) routes() http.Handler {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		h.registerCoreRoutes(r)
		h.registerFileRoutes(r)
		h.registerAlertRoutes(r)
		h.registerKnowledgeRoutes(r)
		h.registerControlRoutes(r)
		h.registerCloudRoutes(r)
		h.registerInspectionRoutes(r)
		h.registerK8sRoutes(r)
	})
	r.HandleFunc("/metrics", h.prometheusMetrics)

	return r
}

func (h *handler) registerCoreRoutes(r chi.Router) {
	r.HandleFunc("/dashboard", h.dashboard)
	r.HandleFunc("/config", h.updateConfig)
	r.HandleFunc("/system", h.systemDashboard)
	r.HandleFunc("/system/terminate", h.systemTerminate)
	r.HandleFunc("/runtime/checks", h.runtimeChecks)
	r.HandleFunc("/health", h.health)
}

func (h *handler) registerFileRoutes(r chi.Router) {
	r.HandleFunc("/auto-upload", h.toggleAutoUpload)
	r.HandleFunc("/manual-upload", h.manualUpload)
	r.HandleFunc("/file-log", h.fileLog)
	r.HandleFunc("/ai/log-summary", h.aiLogSummary)
}

func (h *handler) registerAlertRoutes(r chi.Router) {
	r.HandleFunc("/alerts", h.alertDashboard)
	r.HandleFunc("/alerts/report", h.alertReport)
	r.HandleFunc("/alerts/*", h.alertDecisionWorkflow)
	r.HandleFunc("/alert-config", h.alertConfig)
	r.HandleFunc("/alert-rules", h.alertRules)
}

func (h *handler) registerKnowledgeRoutes(r chi.Router) {
	r.HandleFunc("/kb/articles", h.kbArticles)
	r.HandleFunc("/kb/articles/*", h.kbArticleByID)
	r.HandleFunc("/kb/search", h.kbSearch)
	r.HandleFunc("/kb/ask", h.kbAsk)
	r.HandleFunc("/kb/import/docs", h.kbImportDocs)
	r.HandleFunc("/kb/recommendations", h.kbRecommendations)
	r.HandleFunc("/kb/reviews/pending", h.kbPendingReviews)
	r.HandleFunc("/kb/gates", h.kbGates)
}

func (h *handler) registerControlRoutes(r chi.Router) {
	r.HandleFunc("/control/agents", h.controlAgentsHandler)
	r.HandleFunc("/control/agents/*", h.controlAgentByIDHandler)
	r.HandleFunc("/control/tasks", h.controlTasksHandler)
	r.HandleFunc("/control/tasks/*", h.controlTaskByIDHandler)
	r.HandleFunc("/control/dispatch/pull", h.controlDispatchPullHandler)
	r.HandleFunc("/control/audit", h.controlAuditHandler)
	r.HandleFunc("/registry/domain-probe", h.registryDomainProbe)
}

func (h *handler) registerCloudRoutes(r chi.Router) {
	r.HandleFunc("/cloud/accounts", h.cloudAccountsHandler)
	r.HandleFunc("/cloud/accounts/*", h.cloudAccountByIDHandler)
	r.HandleFunc("/cloud/snapshots", h.cloudSnapshots)
	r.HandleFunc("/cloud/resources", h.cloudResources)
	r.HandleFunc("/cloud/diagnostics", h.cloudDiagnostics)
	r.HandleFunc("/cloud/risks", h.cloudRisks)
	r.HandleFunc("/cloud/inspection-report", h.cloudInspectionReport)

	r.HandleFunc("/cloud/aliyun/instances", h.cloudAliyunInstances)
	r.HandleFunc("/cloud/aliyun/rds/instances", h.cloudAliyunRDSInstances)
	r.HandleFunc("/cloud/aliyun/rds/overview", h.cloudAliyunRDSOverview)
	r.HandleFunc("/cloud/aliyun/metrics", h.cloudAliyunMetrics)
	r.HandleFunc("/cloud/aliyun/overview", h.cloudAliyunOverview)

	r.HandleFunc("/cloud/huawei/instances", h.cloudHuaweiInstances)
	r.HandleFunc("/cloud/huawei/rds/instances", h.cloudHuaweiRDSInstances)
	r.HandleFunc("/cloud/huawei/rds/overview", h.cloudHuaweiRDSOverview)
	r.HandleFunc("/cloud/huawei/metrics", h.cloudHuaweiMetrics)
	r.HandleFunc("/cloud/huawei/overview", h.cloudHuaweiOverview)

	r.HandleFunc("/resources", h.cloudResources)
	r.HandleFunc("/resources/*", h.cloudResourceByID)
}

func (h *handler) registerInspectionRoutes(r chi.Router) {
	r.HandleFunc("/inspection/risks", h.inspectionRisks)
}

func (h *handler) registerK8sRoutes(r chi.Router) {
	r.HandleFunc("/k8s/overview", h.k8sOverview)
	r.HandleFunc("/k8s/node-links", h.k8sNodeLinks)
}
