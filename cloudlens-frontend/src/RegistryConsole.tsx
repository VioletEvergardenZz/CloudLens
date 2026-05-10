/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于域名接入控制台 负责展示域名探测结果与待确认项 */

import { useCallback, useMemo, useState } from "react";
import "./RegistryConsole.css";
import type {
  RegistryDomainHealthCandidate,
  RegistryDomainHTTPProbe,
  RegistryDomainPendingItem,
  RegistryDomainProbeResult,
  RegistryDomainTLSProbe,
} from "./types";
import { USE_MOCK, postRegistryDomainProbe } from "./console/dashboardApi";

const SAMPLE_DOMAIN = "mch.58victory.com";

const mockResult: RegistryDomainProbeResult = {
  domain: SAMPLE_DOMAIN,
  normalizedTarget: SAMPLE_DOMAIN,
  probedAt: "2026-03-20T09:00:00Z",
  reachable: true,
  recommendedBaseUrl: `https://${SAMPLE_DOMAIN}`,
  suggestedAccessType: "frontend_backend_integrated",
  suggestedAccessTypeLabel: "前后端一体域名",
  dns: {
    host: SAMPLE_DOMAIN,
    cname: "gwf-demo.example.com",
    addresses: ["203.0.113.11", "203.0.113.12"],
    publiclyRoutable: true,
  },
  httpRoot: {
    url: `http://${SAMPLE_DOMAIN}/`,
    finalUrl: `https://${SAMPLE_DOMAIN}/`,
    reachable: true,
    statusCode: 301,
    contentType: "text/html; charset=utf-8",
    contentKind: "html",
    pageKind: "spa",
    title: "Marketing Content Hub",
    hasApiHint: true,
    redirected: true,
    redirectChain: [
      {
        url: `http://${SAMPLE_DOMAIN}/`,
        statusCode: 301,
        location: `https://${SAMPLE_DOMAIN}/`,
      },
      {
        url: `https://${SAMPLE_DOMAIN}/`,
        statusCode: 200,
      },
    ],
  },
  httpsRoot: {
    url: `https://${SAMPLE_DOMAIN}/`,
    finalUrl: `https://${SAMPLE_DOMAIN}/`,
    reachable: true,
    statusCode: 200,
    contentType: "text/html; charset=utf-8",
    contentKind: "html",
    pageKind: "spa",
    title: "Marketing Content Hub",
    hasApiHint: true,
    redirected: false,
  },
  tls: {
    status: "expiring",
    subjectCommonName: SAMPLE_DOMAIN,
    issuerCommonName: "Demo Edge CA",
    notBefore: "2026-01-15T00:00:00Z",
    notAfter: "2026-04-08T00:00:00Z",
    daysRemaining: 19,
    serverNameMatched: true,
    timeValid: true,
    dnsNames: [SAMPLE_DOMAIN, `api.${SAMPLE_DOMAIN}`],
  },
  healthCandidates: [
    {
      path: "/health",
      url: `https://${SAMPLE_DOMAIN}/health`,
      finalUrl: `https://${SAMPLE_DOMAIN}/health`,
      reachable: false,
      statusCode: 404,
      contentType: "text/html; charset=utf-8",
      contentKind: "html",
      likelyHealth: false,
    },
    {
      path: "/actuator/health",
      url: `https://${SAMPLE_DOMAIN}/actuator/health`,
      finalUrl: `https://${SAMPLE_DOMAIN}/actuator/health`,
      reachable: true,
      statusCode: 200,
      contentType: "application/json",
      contentKind: "json",
      likelyHealth: true,
    },
    {
      path: "/api/health",
      url: `https://${SAMPLE_DOMAIN}/api/health`,
      finalUrl: `https://${SAMPLE_DOMAIN}/api/health`,
      reachable: true,
      statusCode: 200,
      contentType: "application/json",
      contentKind: "json",
      likelyHealth: true,
    },
    {
      path: "/api/actuator/health",
      url: `https://${SAMPLE_DOMAIN}/api/actuator/health`,
      finalUrl: `https://${SAMPLE_DOMAIN}/api/actuator/health`,
      reachable: false,
      statusCode: 404,
      contentType: "application/json",
      contentKind: "json",
      likelyHealth: false,
    },
  ],
  pendingItems: [
    {
      code: "confirm_environment",
      title: "确认环境归属",
      detail: "请确认该域名属于 prod、test 还是临时环境，并补充环境用途说明。",
      required: true,
    },
    {
      code: "confirm_backend_runtime",
      title: "确认后端部署方式",
      detail: "请补充后端运行方式（docker、jar、systemd 等）以及对应服务名。",
      required: true,
    },
    {
      code: "confirm_health_endpoint",
      title: "确认正式健康接口",
      detail: "已探测到候选健康接口 /actuator/health，请确认它是否作为正式健康检查入口。",
      required: true,
    },
    {
      code: "confirm_tls_alert",
      title: "纳入证书到期告警",
      detail: "当前证书剩余 19 天，请确认是否纳入证书到期告警。",
      required: false,
    },
  ],
};

const formatTime = (value?: string) => {
  if (!value) return "--";
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  return new Date(timestamp).toLocaleString();
};

const formatBool = (value: boolean) => (value ? "是" : "否");

const formatProbeStatus = (probe: RegistryDomainHTTPProbe) => {
  if (!probe.reachable) return "不可达";
  if (!probe.statusCode) return "已响应";
  return `HTTP ${probe.statusCode}`;
};

const formatTLSStatus = (tls?: RegistryDomainTLSProbe) => {
  switch (tls?.status) {
    case "ok":
      return "正常";
    case "expiring":
      return "即将到期";
    case "expired":
      return "已过期";
    case "hostname_mismatch":
      return "域名不匹配";
    default:
      return "--";
  }
};

const formatAccessTypeTone = (reachable: boolean, requiredCount: number) => {
  if (!reachable) return "danger";
  if (requiredCount > 2) return "warning";
  return "success";
};

const renderCandidateTone = (candidate: RegistryDomainHealthCandidate) => {
  if (candidate.likelyHealth) return "success";
  if (!candidate.reachable) return "danger";
  return "warning";
};

const renderProbeTone = (probe: RegistryDomainHTTPProbe) => {
  if (!probe.reachable) return "danger";
  if ((probe.statusCode ?? 0) >= 400) return "warning";
  return "success";
};

const exportRegistryProbe = (result: RegistryDomainProbeResult) => {
  if (typeof window === "undefined" || typeof document === "undefined") return;
  const fileNameSafe = result.normalizedTarget.replace(/[^a-zA-Z0-9.-]+/g, "_");
  const payload = {
    exportedAt: new Date().toISOString(),
    result,
  };
  const blob = new Blob([JSON.stringify(payload, null, 2)], {
    type: "application/json;charset=utf-8",
  });
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `registry-domain-probe-${fileNameSafe || "result"}.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
};

const stringifyRegistryResult = (result: RegistryDomainProbeResult) => JSON.stringify(result, null, 2);

export function RegistryConsole() {
  const [domain, setDomain] = useState(SAMPLE_DOMAIN);
  const [result, setResult] = useState<RegistryDomainProbeResult | null>(USE_MOCK ? mockResult : null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(USE_MOCK ? "当前为示例模式，可直接查看接入草案结构。" : null);

  const requiredPendingCount = useMemo(
    () => result?.pendingItems.filter((item) => item.required).length ?? 0,
    [result]
  );

  const handleProbe = useCallback(async () => {
    const trimmed = domain.trim();
    if (!trimmed) {
      setError("请输入域名或带协议的入口地址");
      setMessage(null);
      return;
    }
    setLoading(true);
    setError(null);
    setMessage(null);
    try {
      if (USE_MOCK) {
        setResult({
          ...mockResult,
          domain: trimmed,
          normalizedTarget: trimmed.replace(/^https?:\/\//i, "").replace(/\/.*$/, ""),
        });
        setMessage("已生成示例探测结果，可继续联调前端展示。");
        return;
      }
      const payload = await postRegistryDomainProbe(trimmed);
      setResult(payload.result);
      setMessage("探测完成，建议先核对“待确认事项”后再补台账。");
    } catch (err) {
      setResult(null);
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }, [domain]);

  const handleUseExample = useCallback(() => {
    setDomain(SAMPLE_DOMAIN);
    setError(null);
    setMessage("已填充示例域名，可直接点击“开始探测”。");
  }, []);

  return (
    <div className="registry-shell">
      <section className="registry-hero" id="registry-overview">
        <div className="registry-hero-main">
          <p className="eyebrow">域名驱动接入原型</p>
          <h1>填写域名，先拿到接入草案</h1>
          <p className="subtitle">
            先做入口探测，再补环境、负责人、部署方式和配置来源，避免每次新系统接入都从零手工梳理。
          </p>
          <div className="registry-intro">
            <span className="badge ghost">范围：域名入口、TLS、候选健康接口、待确认事项</span>
            <span className="badge ghost">边界：不自动确认负责人、服务器、部署方式</span>
            <span className="badge ghost">安全：默认拒绝 IP / localhost / 仅内网目标</span>
          </div>
        </div>
        <div className="registry-hero-side">
          <div className="registry-kpi">
            <small>当前建议切口</small>
            <div className="kpi-value">域名接入页</div>
            <span className="muted small">先闭环“探测结果 + 待确认项”</span>
          </div>
          <div className="registry-kpi">
            <small>推荐输入</small>
            <div className="kpi-value">域名 / URL</div>
            <span className="muted small">支持 `mch.example.com` 或 `https://mch.example.com/app`</span>
          </div>
        </div>
      </section>

      <section className="panel registry-actions" id="registry-probe">
        <div className="section-title">
          <h2>开始探测</h2>
          <span>{loading ? "正在访问入口、证书和健康路径..." : "支持 POST /api/registry/domain-probe"}</span>
        </div>
        <div className="registry-toolbar">
          <input
            className="search registry-domain-input"
            placeholder="输入域名或带协议的入口地址，例如 mch.58victory.com"
            value={domain}
            onChange={(event) => setDomain(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                void handleProbe();
              }
            }}
          />
          <button className="btn" type="button" onClick={() => void handleProbe()} disabled={loading}>
            {loading ? "探测中..." : "开始探测"}
          </button>
          <button className="btn secondary" type="button" onClick={handleUseExample} disabled={loading}>
            使用示例
          </button>
          <button className="btn secondary" type="button" onClick={() => result && exportRegistryProbe(result)} disabled={!result}>
            导出 JSON
          </button>
        </div>
        {error ? <div className="registry-banner error">探测失败：{error}</div> : null}
        {message ? <div className="registry-banner">{message}</div> : null}
      </section>

      {result ? (
        <>
          <section className="panel registry-summary" id="registry-summary">
            <div className="section-title">
              <h2>接入摘要</h2>
              <span>探测时间 {formatTime(result.probedAt)}</span>
            </div>
            <div className="registry-summary-grid">
              <div className="registry-summary-card">
                <div className="registry-summary-label">建议接入类型</div>
                <div className="registry-summary-value">
                  <span className={`pill ${formatAccessTypeTone(result.reachable, requiredPendingCount)}`}>
                    {result.suggestedAccessTypeLabel}
                  </span>
                </div>
                <div className="row-sub">代码标识：{result.suggestedAccessType}</div>
              </div>
              <div className="registry-summary-card">
                <div className="registry-summary-label">建议入口</div>
                <div className="registry-summary-value registry-full-value" title={result.recommendedBaseUrl}>
                  {result.recommendedBaseUrl || "--"}
                </div>
                <div className="row-sub">归一化目标：{result.normalizedTarget}</div>
              </div>
              <div className="registry-summary-card">
                <div className="registry-summary-label">可达性</div>
                <div className="registry-summary-value">
                  <span className={`pill ${result.reachable ? "success" : "danger"}`}>{result.reachable ? "可达" : "不可达"}</span>
                </div>
                <div className="row-sub">待确认事项 {result.pendingItems.length} 条</div>
              </div>
              <div className="registry-summary-card">
                <div className="registry-summary-label">TLS 摘要</div>
                <div className="registry-summary-value">
                  <span className={`pill ${result.tls?.status === "ok" ? "success" : result.tls ? "warning" : "danger"}`}>
                    {formatTLSStatus(result.tls)}
                  </span>
                </div>
                <div className="row-sub">
                  {result.tls ? `剩余 ${result.tls.daysRemaining} 天 · SAN 匹配 ${formatBool(result.tls.serverNameMatched)}` : "未获取到证书信息"}
                </div>
              </div>
            </div>
          </section>

          <section className="panel registry-details" id="registry-details">
            <div className="section-title">
              <h2>探测详情</h2>
              <span>DNS + HTTP/HTTPS 根路径 + TLS</span>
            </div>
            <div className="registry-probe-grid">
              <div className="registry-card">
                <div className="registry-card-head">
                  <h3>DNS</h3>
                  <span className={`pill ${result.dns.publiclyRoutable ? "success" : "warning"}`}>
                    {result.dns.publiclyRoutable ? "公网可达" : "待确认"}
                  </span>
                </div>
                <div className="registry-kv-list">
                  <div className="registry-kv-row">
                    <span>Host</span>
                    <strong>{result.dns.host || "--"}</strong>
                  </div>
                  <div className="registry-kv-row">
                    <span>CNAME</span>
                    <strong>{result.dns.cname || "--"}</strong>
                  </div>
                  <div className="registry-kv-row">
                    <span>地址</span>
                    <strong>{result.dns.addresses.length ? result.dns.addresses.join(", ") : "--"}</strong>
                  </div>
                </div>
                {result.dns.error ? <div className="row-sub">错误：{result.dns.error}</div> : null}
              </div>

              <RegistryProbeCard title="HTTP 根路径" probe={result.httpRoot} />
              <RegistryProbeCard title="HTTPS 根路径" probe={result.httpsRoot} />

              <div className="registry-card">
                <div className="registry-card-head">
                  <h3>TLS</h3>
                  <span className={`pill ${result.tls?.status === "ok" ? "success" : result.tls ? "warning" : "danger"}`}>
                    {formatTLSStatus(result.tls)}
                  </span>
                </div>
                {result.tls ? (
                  <>
                    <div className="registry-kv-list">
                      <div className="registry-kv-row">
                        <span>证书主题</span>
                        <strong>{result.tls.subjectCommonName || "--"}</strong>
                      </div>
                      <div className="registry-kv-row">
                        <span>颁发者</span>
                        <strong>{result.tls.issuerCommonName || "--"}</strong>
                      </div>
                      <div className="registry-kv-row">
                        <span>有效期</span>
                        <strong>{formatTime(result.tls.notBefore)} ~ {formatTime(result.tls.notAfter)}</strong>
                      </div>
                      <div className="registry-kv-row">
                        <span>SAN 匹配</span>
                        <strong>{formatBool(result.tls.serverNameMatched)}</strong>
                      </div>
                    </div>
                    {result.tls.dnsNames?.length ? (
                      <div className="registry-tag-list">
                        {result.tls.dnsNames.map((item) => (
                          <span className="hero-tag" key={item}>
                            {item}
                          </span>
                        ))}
                      </div>
                    ) : null}
                  </>
                ) : (
                  <div className="row-sub">未获取到证书信息，通常说明 HTTPS 不可达或未启用。</div>
                )}
              </div>
            </div>
          </section>

          <section className="panel registry-health" id="registry-health">
            <div className="section-title">
              <h2>候选健康接口</h2>
              <span>优先确认 JSON 响应且状态稳定的路径</span>
            </div>
            <div className="registry-health-list">
              {result.healthCandidates.length ? (
                result.healthCandidates.map((candidate) => (
                  <div className="registry-health-item" key={candidate.path}>
                    <div className="registry-health-head">
                      <div>
                        <div className="row-title">{candidate.path}</div>
                        <div className="row-sub" title={candidate.url}>
                          {candidate.finalUrl || candidate.url}
                        </div>
                      </div>
                      <span className={`pill ${renderCandidateTone(candidate)}`}>
                        {candidate.likelyHealth ? "候选健康接口" : candidate.reachable ? "已响应" : "未命中"}
                      </span>
                    </div>
                    <div className="registry-health-meta">
                      <span className="badge ghost">状态 {candidate.statusCode ?? "--"}</span>
                      <span className="badge ghost">内容 {candidate.contentKind || "--"}</span>
                      {candidate.error ? <span className="badge ghost">错误 {candidate.error}</span> : null}
                    </div>
                  </div>
                ))
              ) : (
                <div className="empty-state">当前没有候选健康接口结果</div>
              )}
            </div>
          </section>

          <section className="panel registry-next" id="registry-next">
            <div className="section-title">
              <h2>待确认事项</h2>
              <span>先确认必填项，再补系统台账和配置来源</span>
            </div>
            <div className="registry-next-list">
              {result.pendingItems.length ? (
                result.pendingItems.map((item) => <RegistryPendingCard key={item.code} item={item} />)
              ) : (
                <div className="empty-state">当前没有待确认事项，可以开始沉淀系统台账。</div>
              )}
            </div>
          </section>

          <section className="panel registry-raw" id="registry-raw">
            <div className="section-title">
              <h2>原始探测结果</h2>
              <span>直接对应接口返回，便于联调与核对</span>
            </div>
            <pre className="registry-json-box">{stringifyRegistryResult(result)}</pre>
          </section>
        </>
      ) : (
        <section className="panel registry-empty" id="registry-next">
          <div className="empty-state">
            输入一个域名后，页面会展示入口探测结果、候选健康接口，以及“下一步需要人工确认什么”。
          </div>
        </section>
      )}
    </div>
  );
}

function RegistryProbeCard({ title, probe }: { title: string; probe: RegistryDomainHTTPProbe }) {
  return (
    <div className="registry-card">
      <div className="registry-card-head">
        <h3>{title}</h3>
        <span className={`pill ${renderProbeTone(probe)}`}>{formatProbeStatus(probe)}</span>
      </div>
      <div className="registry-kv-list">
        <div className="registry-kv-row">
          <span>请求地址</span>
          <strong>{probe.url}</strong>
        </div>
        <div className="registry-kv-row">
          <span>最终地址</span>
          <strong>{probe.finalUrl || "--"}</strong>
        </div>
        <div className="registry-kv-row">
          <span>页面标题</span>
          <strong>{probe.title || "--"}</strong>
        </div>
        <div className="registry-kv-row">
          <span>内容类型</span>
          <strong>{probe.contentType || "--"}</strong>
        </div>
        <div className="registry-kv-row">
          <span>内容识别</span>
          <strong>{probe.contentKind || "--"}</strong>
        </div>
        <div className="registry-kv-row">
          <span>页面识别</span>
          <strong>{probe.pageKind || "--"}</strong>
        </div>
        <div className="registry-kv-row">
          <span>API 提示</span>
          <strong>{formatBool(probe.hasApiHint)}</strong>
        </div>
      </div>
      <div className="registry-health-meta">
        {probe.redirected ? <span className="badge ghost">发生跳转 {probe.redirectChain?.length ?? 0} 次</span> : null}
      </div>
      {probe.redirectChain?.length ? (
        <div className="registry-redirect-list">
          {probe.redirectChain.map((hop, index) => (
            <div className="registry-redirect-item" key={`${hop.url}-${hop.statusCode}-${index}`}>
              <div className="registry-kv-row">
                <span>跳转 {index + 1}</span>
                <strong>{hop.url}</strong>
              </div>
              <div className="registry-health-meta">
                <span className="badge ghost">状态 {hop.statusCode}</span>
                {hop.location ? <span className="badge ghost">Location {hop.location}</span> : null}
              </div>
            </div>
          ))}
        </div>
      ) : null}
      {probe.error ? <div className="row-sub">错误：{probe.error}</div> : null}
    </div>
  );
}

function RegistryPendingCard({ item }: { item: RegistryDomainPendingItem }) {
  return (
    <div className={`registry-next-item ${item.required ? "required" : ""}`}>
      <div className="registry-next-head">
        <div className="row-title">{item.title}</div>
        <span className={`pill ${item.required ? "warning" : "info"}`}>{item.required ? "必填" : "建议"}</span>
      </div>
      <div className="row-sub">{item.detail}</div>
      <div className="registry-next-code">标识：{item.code}</div>
    </div>
  );
}
