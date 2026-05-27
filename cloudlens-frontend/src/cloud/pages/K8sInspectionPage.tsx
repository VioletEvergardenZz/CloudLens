/**
 * 文件职责：承载 Kubernetes 只读巡检页面。
 * 关键交互：展示集群概览、异常项、Pod 明细和云主机关联结果。
 * 边界处理：K8s 不可用时展示后端返回的可读错误，不提供写操作入口。
 */

import type { ReactElement } from "react";

type K8sSummary = {
  nodeTotal: number;
  nodeReady: number;
  namespaceTotal: number;
  podTotal: number;
  podRunning: number;
  podPending: number;
  podFailed: number;
  deploymentTotal: number;
  deploymentUnavailable: number;
  warningEventTotal: number;
  issueTotal: number;
};

type K8sClusterSummary = {
  context?: string;
  server?: string;
  version?: string;
};

type K8sNode = {
  name: string;
  ready: boolean;
  status: string;
  providerId?: string;
  internalIp?: string;
  externalIp?: string;
  hostName?: string;
  kubeletVersion?: string;
  osImage?: string;
  containerRuntime?: string;
  cpu?: string;
  memory?: string;
  podCidr?: string;
  labels?: Record<string, string>;
  taints?: string[];
  createdAt?: string;
};

type K8sPod = {
  namespace: string;
  name: string;
  nodeName?: string;
  phase: string;
  ready: boolean;
  reason?: string;
  message?: string;
  restartCount: number;
  images?: string[];
  createdAt?: string;
};

type K8sDeployment = {
  namespace: string;
  name: string;
  replicas: number;
  readyReplicas: number;
  availableReplicas: number;
  unavailableReplicas: number;
  status: string;
  reason?: string;
  message?: string;
  createdAt?: string;
};

type K8sIssue = {
  id: string;
  severity: string;
  category: string;
  resourceType: string;
  namespace?: string;
  resourceName: string;
  message: string;
  suggestion: string;
  evidence?: string;
};

export type K8sNodeLink = {
  nodeName: string;
  cluster?: string;
  providerId?: string;
  internalIp?: string;
  provider?: string;
  accountId?: number;
  accountName?: string;
  resourceId?: string;
  resourceName?: string;
  region?: string;
  matchType: string;
  confidence: string;
  evidence?: string;
};

export type K8sOverviewResponse = {
  ok?: boolean;
  enabled?: boolean;
  source?: string;
  collectedAt?: string;
  cluster?: K8sClusterSummary;
  summary?: K8sSummary;
  nodes?: K8sNode[];
  pods?: K8sPod[];
  deployments?: K8sDeployment[];
  issues?: K8sIssue[];
  error?: string;
  suggestion?: string;
};

export type K8sInspectionPageProps = {
  activeSection: string;
  overview: K8sOverviewResponse;
  nodeLinks: K8sNodeLink[];
  loading: boolean;
  error: string;
  onRefresh: () => void;
  formatDisplayTime: (raw?: string) => string;
  cloudRiskSeverityLabel: (severity?: string) => string;
  StatusText: ({ children, status }: { children: string; status: string }) => ReactElement;
};

const emptyK8sSummary = (): K8sSummary => ({
  nodeTotal: 0,
  nodeReady: 0,
  namespaceTotal: 0,
  podTotal: 0,
  podRunning: 0,
  podPending: 0,
  podFailed: 0,
  deploymentTotal: 0,
  deploymentUnavailable: 0,
  warningEventTotal: 0,
  issueTotal: 0,
});

const linkConfidenceLabel: Record<string, string> = {
  high: "高",
  medium: "中",
  low: "低",
};

const linkMatchTypeLabel: Record<string, string> = {
  label: "Node 标签",
  provider_id: "ProviderID",
  internal_ip: "内网 IP",
  hostname: "主机名",
  unmatched: "未匹配",
};

export function K8sInspectionPage({
  activeSection,
  overview,
  nodeLinks,
  loading,
  error,
  onRefresh,
  formatDisplayTime,
  cloudRiskSeverityLabel,
  StatusText,
}: K8sInspectionPageProps) {
  const summary = overview.summary ?? emptyK8sSummary();
  const nodes = overview.nodes ?? [];
  const pods = overview.pods ?? [];
  const deployments = overview.deployments ?? [];
  const issues = overview.issues ?? [];
  return (
    <section className="page-main-section k8s-page">
      <div className="section-title k8s-page-header">
        <div>
          <h3>K8s 巡检</h3>
          <span>{overview.cluster?.context || overview.cluster?.server || "等待连接集群"} / {overview.cluster?.version || "版本未知"}</span>
        </div>
        <div className="k8s-page-actions">
          <span>{overview.collectedAt ? `采集于 ${formatDisplayTime(overview.collectedAt)}` : "尚未采集"}</span>
          <button className="k8s-refresh-button" type="button" onClick={onRefresh} disabled={loading}>
            {loading ? "刷新中" : "刷新"}
          </button>
        </div>
      </div>
      {error ? <div className="inline-message">{error}</div> : null}
      {activeSection === "overview" ? (
        <>
          <div className="agent-overview-grid">
            <article className="agent-overview-card primary">
              <span>节点就绪</span>
              <strong>{summary.nodeReady}/{summary.nodeTotal}</strong>
              <em>{overview.source || "kubernetes"}</em>
            </article>
            <article className="agent-overview-card">
              <span>Pod 状态</span>
              <strong>{summary.podRunning}/{summary.podTotal}</strong>
              <em>Pending {summary.podPending} / Failed {summary.podFailed}</em>
            </article>
            <article className="agent-overview-card">
              <span>工作负载</span>
              <strong>{summary.deploymentTotal}</strong>
              <em>不可用 {summary.deploymentUnavailable}</em>
            </article>
            <article className="agent-overview-card">
              <span>云主机关联</span>
              <strong>{nodeLinks.filter((item) => item.matchType !== "unmatched").length}/{nodeLinks.length}</strong>
              <em>按标签、ProviderID、内网 IP 匹配</em>
            </article>
          </div>
          <div className="table-panel">
            <table className="data-table">
              <thead>
                <tr>
                  <th>节点</th>
                  <th>状态</th>
                  <th>内网 IP</th>
                  <th>ProviderID</th>
                  <th>规格</th>
                  <th>Kubelet</th>
                  <th>运行时</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((node) => (
                  <tr key={node.name}>
                    <td><strong>{node.name}</strong></td>
                    <td><StatusText status={node.status}>{node.status}</StatusText></td>
                    <td>{node.internalIp || "--"}</td>
                    <td>{node.providerId || "--"}</td>
                    <td>{node.cpu || "--"} / {node.memory || "--"}</td>
                    <td>{node.kubeletVersion || "--"}</td>
                    <td>{node.containerRuntime || "--"}</td>
                  </tr>
                ))}
                {!nodes.length ? <tr><td colSpan={7}>暂无节点数据</td></tr> : null}
              </tbody>
            </table>
          </div>
          <div className="table-panel">
            <table className="data-table">
              <thead>
                <tr>
                  <th>命名空间</th>
                  <th>Deployment</th>
                  <th>状态</th>
                  <th>副本</th>
                  <th>原因</th>
                </tr>
              </thead>
              <tbody>
                {deployments.slice(0, 20).map((deployment) => (
                  <tr key={`${deployment.namespace}:${deployment.name}`}>
                    <td>{deployment.namespace}</td>
                    <td><strong>{deployment.name}</strong></td>
                    <td><StatusText status={deployment.status}>{deployment.status}</StatusText></td>
                    <td>{deployment.availableReplicas}/{deployment.replicas}</td>
                    <td>{deployment.reason || deployment.message || "--"}</td>
                  </tr>
                ))}
                {!deployments.length ? <tr><td colSpan={5}>暂无工作负载数据</td></tr> : null}
              </tbody>
            </table>
          </div>
        </>
      ) : null}
      {activeSection === "issues" ? (
        <div className="table-panel">
          <table className="data-table">
            <thead>
              <tr>
                <th>级别</th>
                <th>类型</th>
                <th>资源</th>
                <th>问题</th>
                <th>建议</th>
              </tr>
            </thead>
            <tbody>
              {issues.map((issue) => (
                <tr key={issue.id}>
                  <td><StatusText status={issue.severity}>{cloudRiskSeverityLabel(issue.severity)}</StatusText></td>
                  <td>{issue.category}</td>
                  <td>{issue.namespace ? `${issue.namespace} / ` : ""}{issue.resourceType} / {issue.resourceName}</td>
                  <td>{issue.message}</td>
                  <td>{issue.suggestion}</td>
                </tr>
              ))}
              {!issues.length ? <tr><td colSpan={5}>暂无 K8s 异常项</td></tr> : null}
            </tbody>
          </table>
        </div>
      ) : null}
      {activeSection === "pods" ? (
        <div className="table-panel">
          <table className="data-table">
            <thead>
              <tr>
                <th>命名空间</th>
                <th>Pod</th>
                <th>状态</th>
                <th>节点</th>
                <th>重启</th>
                <th>原因</th>
                <th>创建时间</th>
              </tr>
            </thead>
            <tbody>
              {pods.slice(0, 80).map((pod) => (
                <tr key={`${pod.namespace}:${pod.name}`}>
                  <td>{pod.namespace}</td>
                  <td><strong>{pod.name}</strong></td>
                  <td><StatusText status={pod.phase}>{pod.ready ? `${pod.phase} / Ready` : pod.phase}</StatusText></td>
                  <td>{pod.nodeName || "--"}</td>
                  <td>{pod.restartCount}</td>
                  <td>{pod.reason || pod.message || "--"}</td>
                  <td>{formatDisplayTime(pod.createdAt)}</td>
                </tr>
              ))}
              {!pods.length ? <tr><td colSpan={7}>暂无 Pod 数据</td></tr> : null}
            </tbody>
          </table>
        </div>
      ) : null}
      {activeSection === "links" ? (
        <div className="table-panel">
          <table className="data-table">
            <thead>
              <tr>
                <th>K8s Node</th>
                <th>匹配方式</th>
                <th>可信度</th>
                <th>云主机</th>
                <th>账号 / 地域</th>
                <th>证据</th>
              </tr>
            </thead>
            <tbody>
              {nodeLinks.map((link) => (
                <tr key={link.nodeName}>
                  <td><strong>{link.nodeName}</strong></td>
                  <td>{linkMatchTypeLabel[link.matchType] ?? link.matchType}</td>
                  <td><StatusText status={link.confidence === "high" ? "ok" : link.confidence === "medium" ? "warning" : "unknown"}>{linkConfidenceLabel[link.confidence] ?? link.confidence}</StatusText></td>
                  <td>{link.resourceName ? `${link.resourceName} / ${link.resourceId}` : "--"}</td>
                  <td>{link.accountName ? `${link.accountName} / ${link.region || "--"}` : "--"}</td>
                  <td>{link.evidence || link.internalIp || link.providerId || "--"}</td>
                </tr>
              ))}
              {!nodeLinks.length ? <tr><td colSpan={6}>暂无 Node 关联结果</td></tr> : null}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
