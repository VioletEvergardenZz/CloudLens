/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于系统台账页原型，将系统目录和单系统详情收敛为更接近成熟后台的页面结构。 */

import { useMemo, useState } from "react";
import "./RegistryCatalogConsole.css";

type RegistryEnvironment = {
  key: string;
  type: "prod" | "test" | "temp";
  health: "healthy" | "degraded" | "unavailable" | "unknown";
  changeWindow: string;
  services: Array<{
    key: string;
    runtimeType: string;
    route?: string;
    healthTarget?: string;
    owner: string;
    configCompleteness: string;
  }>;
};

type RegistryIncident = {
  title: string;
  level: "P1" | "P2" | "P3";
  status: "处理中" | "已恢复" | "待复盘";
  time: string;
};

type RegistryRunbook = {
  title: string;
  summary: string;
  updatedAt: string;
};

type RegistrySystem = {
  key: string;
  name: string;
  owner: string;
  ownerTeam: string;
  importance: "core" | "normal" | "temp";
  status: "active" | "maintenance" | "offline";
  summary: string;
  repo: string;
  docs: string;
  routes: string[];
  coverage: {
    health: string;
    runbook: string;
    ownership: string;
  };
  pendingGaps: string[];
  incidents: RegistryIncident[];
  runbooks: RegistryRunbook[];
  environments: RegistryEnvironment[];
};

type RegistryDetailTab = "overview" | "services" | "incidents" | "runbooks" | "gaps";

const SYSTEMS: RegistrySystem[] = [
  {
    key: "marketing-content-hub",
    name: "营销内容中心",
    owner: "宋景",
    ownerTeam: "内容平台组",
    importance: "core",
    status: "active",
    summary: "负责内容发布、素材同步和异步任务调度，是内容链路的关键系统。",
    repo: "git@example.com:marketing-content-hub.git",
    docs: "docs/systems/marketing-content-hub.md",
    routes: ["https://mch.58victory.com/", "https://mch-test.example.com/"],
    coverage: {
      health: "2/3 环境已补齐",
      runbook: "线程池 / 发布失败已覆盖",
      ownership: "负责人、值班组、升级链完整",
    },
    pendingGaps: ["test 环境缺少统一健康探针", "回放材料与变更记录尚未自动回链到同一页"],
    incidents: [
      { title: "线程池拒绝导致异步任务堆积", level: "P2", status: "处理中", time: "2026-04-20 10:18" },
      { title: "内容发布回调超时", level: "P3", status: "待复盘", time: "2026-04-19 18:42" },
    ],
    runbooks: [
      {
        title: "线程池拒绝标准处置",
        summary: "先看 backlog、平均执行时长和最近任务模板变更，再决定扩容还是重启。",
        updatedAt: "2026-04-18",
      },
      {
        title: "内容发布失败回滚说明",
        summary: "回退最近发布批次，核对素材同步与回调链路。",
        updatedAt: "2026-04-12",
      },
    ],
    environments: [
      {
        key: "prod",
        type: "prod",
        health: "healthy",
        changeWindow: "工作日 10:00-18:00",
        services: [
          {
            key: "marketing-frontend",
            runtimeType: "static",
            route: "https://mch.58victory.com/",
            owner: "前端值班组",
            configCompleteness: "80%",
          },
          {
            key: "marketing-api",
            runtimeType: "docker",
            route: "https://mch.58victory.com/api",
            healthTarget: "http://localhost:8080/actuator/health",
            owner: "内容后端组",
            configCompleteness: "70%",
          },
        ],
      },
      {
        key: "test",
        type: "test",
        health: "degraded",
        changeWindow: "随时可发，但需要保留回滚材料",
        services: [
          {
            key: "marketing-frontend-test",
            runtimeType: "static",
            route: "https://mch-test.example.com/",
            owner: "前端值班组",
            configCompleteness: "68%",
          },
          {
            key: "marketing-api-test",
            runtimeType: "docker",
            healthTarget: "http://localhost:18080/actuator/health",
            owner: "内容后端组",
            configCompleteness: "62%",
          },
        ],
      },
    ],
  },
  {
    key: "order-risk-center",
    name: "订单风控中心",
    owner: "叶舟",
    ownerTeam: "交易稳定性组",
    importance: "core",
    status: "active",
    summary: "负责订单准入、风控判定和风险事件升级，是交易链路最敏感的系统之一。",
    repo: "git@example.com:order-risk-center.git",
    docs: "docs/systems/order-risk-center.md",
    routes: ["https://risk.example.com/api"],
    coverage: {
      health: "prod 关键健康规则完整",
      runbook: "连接池 / 慢 SQL / 回滚已覆盖",
      ownership: "负责人和值班升级链完整",
    },
    pendingGaps: ["worker 缺少独立服务级可观测入口", "最近事件与变更记录尚未自动聚合"],
    incidents: [
      { title: "数据库连接池耗尽", level: "P1", status: "处理中", time: "2026-04-20 10:31" },
      { title: "订单写入慢 SQL 抖动", level: "P2", status: "已恢复", time: "2026-04-19 22:06" },
    ],
    runbooks: [
      {
        title: "连接池耗尽处置卡",
        summary: "先看 DB 连接数、线程池和上游重试，再决定扩容还是回滚。",
        updatedAt: "2026-04-17",
      },
      {
        title: "高等级交易故障升级链",
        summary: "15 分钟内未收敛时自动升级到平台主管和值班负责人。",
        updatedAt: "2026-04-08",
      },
    ],
    environments: [
      {
        key: "prod",
        type: "prod",
        health: "degraded",
        changeWindow: "核心时段禁止无回滚发布",
        services: [
          {
            key: "risk-api",
            runtimeType: "jar",
            route: "https://risk.example.com/api",
            healthTarget: "http://127.0.0.1:8081/actuator/health",
            owner: "交易稳定性组",
            configCompleteness: "74%",
          },
          {
            key: "risk-worker",
            runtimeType: "script",
            owner: "交易稳定性组",
            configCompleteness: "58%",
          },
        ],
      },
    ],
  },
  {
    key: "ops-file-gateway",
    name: "文件接入网关",
    owner: "周临",
    ownerTeam: "平台运维组",
    importance: "normal",
    status: "maintenance",
    summary: "承接文件入云、回放与 AI 分析输入，是事件证据链的入口系统。",
    repo: "git@example.com:ops-file-gateway.git",
    docs: "docs/systems/ops-file-gateway.md",
    routes: ["https://gwf.example.com/api"],
    coverage: {
      health: "prod 健康探针完整",
      runbook: "上传失败 / 磁盘热点已覆盖",
      ownership: "负责人已明确，值班升级链待补充",
    },
    pendingGaps: ["控制面与系统资源仍偏技术视角，缺少事件回链", "域名接入草案尚未自动转成系统详情"],
    incidents: [
      { title: "磁盘写入异常导致上传回放变慢", level: "P2", status: "待复盘", time: "2026-04-18 15:04" },
    ],
    runbooks: [
      {
        title: "上传失败排查 SOP",
        summary: "先看 backlog 和失败样本，再核对对象存储与本地磁盘空间。",
        updatedAt: "2026-04-16",
      },
    ],
    environments: [
      {
        key: "prod",
        type: "prod",
        health: "healthy",
        changeWindow: "非高峰期可发，必须保留回滚材料",
        services: [
          {
            key: "gwf-api",
            runtimeType: "docker",
            route: "https://gwf.example.com/api",
            healthTarget: "http://127.0.0.1:8082/api/health",
            owner: "平台运维组",
            configCompleteness: "92%",
          },
        ],
      },
    ],
  },
];

const healthLabel = (health: RegistryEnvironment["health"]) => {
  switch (health) {
    case "healthy":
      return "健康";
    case "degraded":
      return "降级";
    case "unavailable":
      return "不可用";
    default:
      return "未知";
  }
};

const healthTone = (health: RegistryEnvironment["health"]) => {
  switch (health) {
    case "healthy":
      return "success";
    case "degraded":
      return "warning";
    case "unavailable":
      return "danger";
    default:
      return "info";
  }
};

const importanceLabel = (importance: RegistrySystem["importance"]) => {
  switch (importance) {
    case "core":
      return "核心系统";
    case "temp":
      return "临时系统";
    default:
      return "常规系统";
  }
};

const statusLabel = (status: RegistrySystem["status"]) => {
  switch (status) {
    case "active":
      return "运行中";
    case "maintenance":
      return "维护中";
    default:
      return "离线";
  }
};

const statusTone = (status: RegistrySystem["status"]) => {
  switch (status) {
    case "active":
      return "success";
    case "maintenance":
      return "warning";
    default:
      return "danger";
  }
};

export function RegistryCatalogConsole() {
  const [query, setQuery] = useState("");
  const [envFilter, setEnvFilter] = useState<"all" | "prod" | "test" | "temp">("all");
  const [selectedSystemKey, setSelectedSystemKey] = useState(SYSTEMS[0]?.key ?? "");
  const [detailTab, setDetailTab] = useState<RegistryDetailTab>("overview");

  const filteredSystems = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return SYSTEMS.filter((item) => {
      const matchesKeyword =
        !keyword ||
        item.key.toLowerCase().includes(keyword) ||
        item.name.toLowerCase().includes(keyword) ||
        item.owner.toLowerCase().includes(keyword) ||
        item.ownerTeam.toLowerCase().includes(keyword);

      if (!matchesKeyword) return false;
      if (envFilter === "all") return true;
      return item.environments.some((env) => env.type === envFilter);
    });
  }, [envFilter, query]);

  const selectedSystem =
    filteredSystems.find((item) => item.key === selectedSystemKey) ?? filteredSystems[0] ?? SYSTEMS[0] ?? null;

  const summary = useMemo(() => {
    const envCount = filteredSystems.reduce((acc, item) => acc + item.environments.length, 0);
    const serviceCount = filteredSystems.reduce(
      (acc, item) => acc + item.environments.reduce((serviceAcc, env) => serviceAcc + env.services.length, 0),
      0
    );
    const hotspotCount = filteredSystems.filter((item) =>
      item.incidents.some((incident) => incident.status === "处理中" || incident.status === "待复盘")
    ).length;

    return {
      systemCount: filteredSystems.length,
      envCount,
      serviceCount,
      hotspotCount,
    };
  }, [filteredSystems]);

  const detailTabs: Array<{ id: RegistryDetailTab; label: string; desc: string }> = [
    { id: "overview", label: "概览", desc: "系统摘要、入口和覆盖情况" },
    { id: "services", label: "服务", desc: "按环境查看服务和健康检查" },
    { id: "incidents", label: "事件", desc: "最近事件与变更窗口" },
    { id: "runbooks", label: "Runbook", desc: "处置手册和文档入口" },
    { id: "gaps", label: "待补齐", desc: "当前缺口和下一步建议" },
  ];

  return (
    <div className="registry-catalog-shell">
      <section className="panel registry-page-header" id="registry-header">
        <div className="registry-page-top">
          <div className="registry-page-copy">
            <p className="eyebrow">系统台账</p>
            <h2>先从系统目录进入，再看单系统详情</h2>
            <p>系统页不应该只是资产清单，而应该承接事件上下文、负责人、服务入口和 Runbook。</p>
          </div>
        </div>

        <div className="registry-summary-grid">
          <div className="registry-summary-card">
            <span>系统数</span>
            <strong>{summary.systemCount}</strong>
            <small>目录内已纳管系统</small>
          </div>
          <div className="registry-summary-card">
            <span>环境数</span>
            <strong>{summary.envCount}</strong>
            <small>prod / test / temp</small>
          </div>
          <div className="registry-summary-card">
            <span>服务数</span>
            <strong>{summary.serviceCount}</strong>
            <small>系统详情页应承接服务入口与健康规则</small>
          </div>
          <div className="registry-summary-card">
            <span>热点系统</span>
            <strong>{summary.hotspotCount}</strong>
            <small>仍有处理中或待复盘事件</small>
          </div>
        </div>
      </section>

      <section className="panel" id="registry-directory">
        <div className="section-title">
          <div>
            <h2>系统目录</h2>
            <span>上方目录负责筛选和定位，下方详情负责承接排障上下文</span>
          </div>
        </div>

        <div className="toolbar">
          <input
            className="search"
            placeholder="搜索系统名 / key / 负责人 / 团队"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <select className="select" value={envFilter} onChange={(event) => setEnvFilter(event.target.value as "all" | "prod" | "test" | "temp")}>
            <option value="all">全部环境</option>
            <option value="prod">prod</option>
            <option value="test">test</option>
            <option value="temp">temp</option>
          </select>
        </div>

        {filteredSystems.length ? (
          <div className="table-wrap">
            <table className="table registry-directory-table">
              <thead>
                <tr>
                  <th>系统</th>
                  <th>团队</th>
                  <th>环境</th>
                  <th>最近事件</th>
                  <th>Runbook</th>
                  <th>状态</th>
                </tr>
              </thead>
              <tbody>
                {filteredSystems.map((item) => (
                  <tr
                    key={item.key}
                    className={item.key === selectedSystem?.key ? "registry-row-selected" : ""}
                    onClick={() => setSelectedSystemKey(item.key)}
                  >
                    <td>
                      <div className="registry-row-main">
                        <div className="row-title">{item.name}</div>
                        <div className="row-sub">
                          {item.key} · {importanceLabel(item.importance)}
                        </div>
                      </div>
                    </td>
                    <td>
                      <div className="row-title">{item.ownerTeam}</div>
                      <div className="row-sub">负责人 {item.owner}</div>
                    </td>
                    <td>{item.environments.length} 个</td>
                    <td>{item.incidents[0]?.title || "暂无事件"}</td>
                    <td>{item.runbooks.length} 份</td>
                    <td>
                      <span className={`pill ${statusTone(item.status)}`}>{statusLabel(item.status)}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty-state">当前没有匹配的系统。</div>
        )}
      </section>

      <section className="panel" id="registry-detail">
        {selectedSystem ? (
          <>
            <div className="registry-detail-header">
              <div className="registry-detail-copy">
                <div className="registry-detail-title">
                  <h2>{selectedSystem.name}</h2>
                  <span className="badge ghost">{importanceLabel(selectedSystem.importance)}</span>
                  <span className={`pill ${statusTone(selectedSystem.status)}`}>{statusLabel(selectedSystem.status)}</span>
                </div>
                <p>{selectedSystem.summary}</p>
              </div>

              <div className="registry-detail-meta">
                <span className="badge ghost">{selectedSystem.ownerTeam}</span>
                <span className="badge ghost">负责人 {selectedSystem.owner}</span>
                <span className="badge ghost">{selectedSystem.repo}</span>
              </div>
            </div>

            <div className="registry-tab-strip">
              {detailTabs.map((tab) => (
                <button
                  key={tab.id}
                  className={`registry-tab-button ${detailTab === tab.id ? "active" : ""}`}
                  type="button"
                  onClick={() => setDetailTab(tab.id)}
                >
                  <strong>{tab.label}</strong>
                  <span>{tab.desc}</span>
                </button>
              ))}
            </div>

            {detailTab === "overview" ? (
              <div className="registry-overview-grid">
                <div className="registry-detail-card">
                  <span>健康覆盖</span>
                  <strong>{selectedSystem.coverage.health}</strong>
                  <small>把健康探针与环境对应起来，而不是散落在文档里。</small>
                </div>
                <div className="registry-detail-card">
                  <span>Runbook 覆盖</span>
                  <strong>{selectedSystem.coverage.runbook}</strong>
                  <small>事件页需要能直接跳到处置手册。</small>
                </div>
                <div className="registry-detail-card">
                  <span>责任边界</span>
                  <strong>{selectedSystem.coverage.ownership}</strong>
                  <small>值班时优先明确谁负责、谁升级、谁回写。</small>
                </div>
                <div className="registry-detail-card full">
                  <span>入口与文档</span>
                  <div className="registry-route-list">
                    {selectedSystem.routes.map((route) => (
                      <span className="badge ghost" key={route}>
                        {route}
                      </span>
                    ))}
                    <span className="badge ghost">{selectedSystem.docs}</span>
                  </div>
                </div>
              </div>
            ) : null}

            {detailTab === "services" ? (
              <div className="registry-service-grid">
                {selectedSystem.environments.map((env) => (
                  <div className="registry-env-card" key={env.key}>
                    <div className="registry-env-head">
                      <div>
                        <strong>{env.key}</strong>
                        <div className="row-sub">变更窗口 {env.changeWindow}</div>
                      </div>
                      <span className={`pill ${healthTone(env.health)}`}>{healthLabel(env.health)}</span>
                    </div>

                    <div className="table-wrap">
                      <table className="table registry-service-table">
                        <thead>
                          <tr>
                            <th>服务</th>
                            <th>运行形态</th>
                            <th>负责人</th>
                            <th>入口 / 健康</th>
                            <th>配置完整度</th>
                          </tr>
                        </thead>
                        <tbody>
                          {env.services.map((service) => (
                            <tr key={service.key}>
                              <td>{service.key}</td>
                              <td>{service.runtimeType}</td>
                              <td>{service.owner}</td>
                              <td>
                                <div className="registry-row-main">
                                  {service.route ? <div className="row-title">{service.route}</div> : null}
                                  <div className="row-sub">{service.healthTarget || "暂无健康检查"}</div>
                                </div>
                              </td>
                              <td>{service.configCompleteness}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                ))}
              </div>
            ) : null}

            {detailTab === "incidents" ? (
              <div className="registry-stack">
                <div className="registry-detail-card">
                  <span>最近事件</span>
                  <div className="registry-detail-list">
                    {selectedSystem.incidents.map((incident) => (
                      <div className="registry-list-item" key={`${incident.title}-${incident.time}`}>
                        <strong>{incident.title}</strong>
                        <span>
                          {incident.level} · {incident.status} · {incident.time}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="registry-detail-card">
                  <span>环境变更窗口</span>
                  <div className="registry-detail-list">
                    {selectedSystem.environments.map((env) => (
                      <div className="registry-list-item" key={env.key}>
                        <strong>{env.key}</strong>
                        <span>{env.changeWindow}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            ) : null}

            {detailTab === "runbooks" ? (
              <div className="registry-stack">
                <div className="registry-detail-card">
                  <span>Runbook 列表</span>
                  <div className="registry-detail-list">
                    {selectedSystem.runbooks.map((runbook) => (
                      <div className="registry-list-item" key={runbook.title}>
                        <strong>{runbook.title}</strong>
                        <span>{runbook.summary}</span>
                        <span>最近更新 {runbook.updatedAt}</span>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="registry-detail-card">
                  <span>文档与入口</span>
                  <div className="registry-route-list">
                    {selectedSystem.routes.map((route) => (
                      <span className="badge ghost" key={route}>
                        {route}
                      </span>
                    ))}
                    <span className="badge ghost">{selectedSystem.docs}</span>
                  </div>
                </div>
              </div>
            ) : null}

            {detailTab === "gaps" ? (
              <div className="registry-stack">
                <div className="registry-detail-card">
                  <span>当前缺口</span>
                  <div className="registry-detail-list">
                    {selectedSystem.pendingGaps.map((item) => (
                      <div className="registry-list-item" key={item}>
                        <strong>Gap</strong>
                        <span>{item}</span>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="registry-detail-card">
                  <span>下一步建议</span>
                  <div className="registry-detail-list">
                    <div className="registry-list-item">
                      <strong>先补健康</strong>
                      <span>把缺失环境的健康检查补齐到系统详情页，而不是留在外部文档。</span>
                    </div>
                    <div className="registry-list-item">
                      <strong>再接事件</strong>
                      <span>让事件详情页可以直接回链到系统对象、服务入口和负责人。</span>
                    </div>
                    <div className="registry-list-item">
                      <strong>最后沉淀 SOP</strong>
                      <span>把当前热点事件对应的标准动作回写到 Runbook 区域。</span>
                    </div>
                  </div>
                </div>
              </div>
            ) : null}
          </>
        ) : (
          <div className="empty-state">当前没有可展示的系统详情。</div>
        )}
      </section>
    </div>
  );
}
