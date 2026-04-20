/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于系统台账与系统详情原型 把纳管事实、最近事件与 SOP 放回同一页 */

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

const SYSTEMS: RegistrySystem[] = [
  {
    key: "marketing-content-hub",
    name: "营销内容中心",
    owner: "宋辰",
    ownerTeam: "内容平台组",
    importance: "core",
    status: "active",
    summary: "负责内容发布、素材同步和异步任务调度，是内容链路的关键系统。",
    repo: "git@example.com:marketing-content-hub.git",
    docs: "docs/systems/marketing-content-hub.md",
    routes: ["https://mch.58victory.com/", "https://mch-test.example.com/"],
    coverage: {
      health: "2/3 环境已补齐",
      runbook: "线程池 / 发布失败 已覆盖",
      ownership: "负责人、值班组、升级链已补齐",
    },
    pendingGaps: ["test 环境缺少统一健康探针", "回放材料与变更记录还未回链到同一页"],
    incidents: [
      {
        title: "线程池拒绝导致异步任务堆积",
        level: "P2",
        status: "处理中",
        time: "2026-04-20 10:18",
      },
      {
        title: "内容发布回调超时",
        level: "P3",
        status: "待复盘",
        time: "2026-04-19 18:42",
      },
    ],
    runbooks: [
      {
        title: "线程池拒绝标准处置",
        summary: "先看 backlog、平均执行时长和最近任务模板变更，再决定是否扩容或重启。",
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
        changeWindow: "随时可发，但需保留回滚材料",
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
      runbook: "连接池 / 慢 SQL / 回滚 已覆盖",
      ownership: "负责人、值班升级链完整",
    },
    pendingGaps: ["worker 缺少独立服务级可观测入口", "最近事件与变更记录尚未自动聚合"],
    incidents: [
      {
        title: "数据库连接池耗尽",
        level: "P1",
        status: "处理中",
        time: "2026-04-20 10:31",
      },
      {
        title: "订单写入慢 SQL 抖动",
        level: "P2",
        status: "已恢复",
        time: "2026-04-19 22:06",
      },
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
      runbook: "上传失败 / 磁盘热点 已覆盖",
      ownership: "负责人已明确，值班升级链待补充",
    },
    pendingGaps: ["控制面与系统资源仍偏技术视角，缺少事件回链", "域名接入草案还未自动转成系统详情"],
    incidents: [
      {
        title: "磁盘写入异常导致上传回放变慢",
        level: "P2",
        status: "待复盘",
        time: "2026-04-18 15:04",
      },
    ],
    runbooks: [
      {
        title: "上传失败排查 SOP",
        summary: "先看 backlog、失败文件样本，再核对对象存储与本地磁盘空间。",
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

export function RegistryCatalogConsole() {
  const [query, setQuery] = useState("");
  const [envFilter, setEnvFilter] = useState<"all" | "prod" | "test" | "temp">("all");
  const [selectedSystemKey, setSelectedSystemKey] = useState(SYSTEMS[0]?.key ?? "");

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

  return (
    <div className="registry-catalog-shell">
      <section className="panel registry-catalog-hero" id="registry-catalog-overview">
        <div className="section-title">
          <div>
            <h2>系统台账 / 系统详情</h2>
            <span>参考 NetBox 与 Backstage，把“事实源 + 关系 + 责任 + 最近事件”合成单系统入口</span>
          </div>
          <span className="badge ghost">系统主线</span>
        </div>
        <p className="registry-catalog-copy">
          系统页不应该只是资产清单，而应该成为排障上下文的承接面。
          当事件页需要系统、服务、入口、责任人和 SOP 时，应该能在这里一次性拿到。
        </p>
        <div className="registry-catalog-summary">
          <div className="registry-catalog-card">
            <div className="registry-catalog-label">纳管系统</div>
            <div className="registry-catalog-value">{summary.systemCount}</div>
          </div>
          <div className="registry-catalog-card">
            <div className="registry-catalog-label">环境总数</div>
            <div className="registry-catalog-value">{summary.envCount}</div>
          </div>
          <div className="registry-catalog-card">
            <div className="registry-catalog-label">服务总数</div>
            <div className="registry-catalog-value">{summary.serviceCount}</div>
          </div>
          <div className="registry-catalog-card">
            <div className="registry-catalog-label">热点系统</div>
            <div className="registry-catalog-value">{summary.hotspotCount}</div>
          </div>
        </div>
      </section>

      <section className="panel" id="registry-catalog-list">
        <div className="section-title">
          <h2>系统目录</h2>
          <span>先从系统对象进入，而不是从零散模块里拼上下文</span>
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

        <div className="registry-catalog-layout">
          <div className="registry-catalog-list">
            {filteredSystems.map((item) => (
              <button
                className={`registry-catalog-item ${item.key === selectedSystem?.key ? "active" : ""}`}
                key={item.key}
                type="button"
                onClick={() => setSelectedSystemKey(item.key)}
              >
                <div className="row-title">{item.name}</div>
                <div className="row-sub">{item.key}</div>
                <div className="registry-catalog-badges">
                  <span className="badge ghost">{importanceLabel(item.importance)}</span>
                  <span className="badge ghost">{item.ownerTeam}</span>
                  <span className="badge ghost">{item.environments.length} 环境</span>
                </div>
              </button>
            ))}
          </div>

          <div className="registry-catalog-detail panel" id="registry-catalog-detail">
            {selectedSystem ? (
              <>
                <div className="registry-detail-headline">
                  <div>
                    <div className="registry-detail-title-row">
                      <h2>{selectedSystem.name}</h2>
                      <span className="badge ghost">{importanceLabel(selectedSystem.importance)}</span>
                    </div>
                    <p>{selectedSystem.summary}</p>
                  </div>
                  <div className="registry-detail-meta">
                    <span className="badge ghost">{selectedSystem.ownerTeam}</span>
                    <span className="badge ghost">负责人 {selectedSystem.owner}</span>
                    <span className="badge ghost">{selectedSystem.repo}</span>
                  </div>
                </div>

                <div className="registry-detail-ribbon">
                  <div className="registry-ribbon-card">
                    <small>健康覆盖</small>
                    <strong>{selectedSystem.coverage.health}</strong>
                  </div>
                  <div className="registry-ribbon-card">
                    <small>Runbook 覆盖</small>
                    <strong>{selectedSystem.coverage.runbook}</strong>
                  </div>
                  <div className="registry-ribbon-card">
                    <small>责任边界</small>
                    <strong>{selectedSystem.coverage.ownership}</strong>
                  </div>
                </div>

                <div className="registry-detail-layout">
                  <div className="registry-detail-main">
                    <div className="registry-detail-grid">
                      {selectedSystem.environments.map((env) => (
                        <div className="registry-detail-card" key={env.key}>
                          <div className="registry-detail-head">
                            <div>
                              <div className="row-title">{env.key}</div>
                              <div className="row-sub">变更窗口 {env.changeWindow}</div>
                            </div>
                            <span className={`pill ${healthTone(env.health)}`}>{healthLabel(env.health)}</span>
                          </div>
                          <div className="registry-service-list">
                            {env.services.map((service) => (
                              <div className="registry-service-item" key={service.key}>
                                <div className="row-title">{service.key}</div>
                                <div className="row-sub">运行形态 {service.runtimeType}</div>
                                <div className="row-sub">负责人 {service.owner}</div>
                                {service.route ? <div className="row-sub">入口 {service.route}</div> : null}
                                {service.healthTarget ? <div className="row-sub">健康 {service.healthTarget}</div> : null}
                                <div className="row-sub">配置完整度 {service.configCompleteness}</div>
                              </div>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="registry-detail-side">
                    <div className="registry-side-card">
                      <div className="registry-side-title">最近事件</div>
                      <div className="registry-side-list">
                        {selectedSystem.incidents.map((incident) => (
                          <div className="registry-side-item" key={`${incident.title}-${incident.time}`}>
                            <strong>{incident.title}</strong>
                            <span>
                              {incident.level} · {incident.status} · {incident.time}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>

                    <div className="registry-side-card">
                      <div className="registry-side-title">SOP / Runbook</div>
                      <div className="registry-side-list">
                        {selectedSystem.runbooks.map((runbook) => (
                          <div className="registry-side-item" key={runbook.title}>
                            <strong>{runbook.title}</strong>
                            <span>{runbook.summary}</span>
                            <span>最近更新 {runbook.updatedAt}</span>
                          </div>
                        ))}
                      </div>
                    </div>

                    <div className="registry-side-card">
                      <div className="registry-side-title">待补齐项</div>
                      <div className="registry-side-list">
                        {selectedSystem.pendingGaps.map((item) => (
                          <div className="registry-side-item" key={item}>
                            <strong>Gap</strong>
                            <span>{item}</span>
                          </div>
                        ))}
                      </div>
                    </div>

                    <div className="registry-side-card">
                      <div className="registry-side-title">入口与文档</div>
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
                </div>
              </>
            ) : (
              <div className="empty-state">当前没有匹配系统</div>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
