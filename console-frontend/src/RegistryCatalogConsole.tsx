/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于系统台账原型页 展示系统 环境 服务 路由 与健康规则的骨架 */

import { useMemo, useState } from "react";
import "./RegistryCatalogConsole.css";

type RegistryEnvironment = {
  key: string;
  type: "prod" | "test" | "temp";
  health: "healthy" | "degraded" | "unavailable" | "unknown";
  services: Array<{
    key: string;
    runtimeType: string;
    route?: string;
    healthTarget?: string;
    configCompleteness: string;
  }>;
};

type RegistrySystem = {
  key: string;
  name: string;
  owner: string;
  importance: "core" | "normal" | "temp";
  status: "active" | "maintenance" | "offline";
  repo: string;
  environments: RegistryEnvironment[];
};

const SYSTEMS: RegistrySystem[] = [
  {
    key: "marketing-content-hub",
    name: "营销内容中心",
    owner: "内容平台组",
    importance: "core",
    status: "active",
    repo: "git@example.com:marketing-content-hub.git",
    environments: [
      {
        key: "prod",
        type: "prod",
        health: "healthy",
        services: [
          {
            key: "marketing-frontend",
            runtimeType: "static",
            route: "https://mch.58victory.com/",
            configCompleteness: "80%",
          },
          {
            key: "marketing-api",
            runtimeType: "docker",
            route: "https://mch.58victory.com/api",
            healthTarget: "http://localhost:8080/actuator/health",
            configCompleteness: "70%",
          },
        ],
      },
      {
        key: "test",
        type: "test",
        health: "degraded",
        services: [
          {
            key: "marketing-frontend-test",
            runtimeType: "static",
            route: "https://mch-test.example.com/",
            configCompleteness: "68%",
          },
          {
            key: "marketing-api-test",
            runtimeType: "docker",
            healthTarget: "http://localhost:18080/actuator/health",
            configCompleteness: "62%",
          },
        ],
      },
    ],
  },
  {
    key: "order-risk-center",
    name: "订单风控中心",
    owner: "交易稳定性组",
    importance: "core",
    status: "active",
    repo: "git@example.com:order-risk-center.git",
    environments: [
      {
        key: "prod",
        type: "prod",
        health: "degraded",
        services: [
          {
            key: "risk-api",
            runtimeType: "jar",
            route: "https://risk.example.com/api",
            healthTarget: "http://127.0.0.1:8081/actuator/health",
            configCompleteness: "74%",
          },
          {
            key: "risk-worker",
            runtimeType: "script",
            configCompleteness: "58%",
          },
        ],
      },
    ],
  },
  {
    key: "ops-file-gateway",
    name: "文件接入网关",
    owner: "平台运维组",
    importance: "normal",
    status: "maintenance",
    repo: "git@example.com:ops-file-gateway.git",
    environments: [
      {
        key: "prod",
        type: "prod",
        health: "healthy",
        services: [
          {
            key: "gwf-api",
            runtimeType: "docker",
            route: "https://gwf.example.com/api",
            healthTarget: "http://127.0.0.1:8082/api/health",
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
        item.owner.toLowerCase().includes(keyword);
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
    return {
      systemCount: filteredSystems.length,
      envCount,
      serviceCount,
    };
  }, [filteredSystems]);

  return (
    <div className="registry-catalog-shell">
      <section className="panel" id="registry-catalog-overview">
        <div className="section-title">
          <h2>系统台账</h2>
          <span>当前为纳管台账原型，后续接入 Git 事实源 / Agent 巡检结果</span>
        </div>
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
        </div>
      </section>

      <section className="panel" id="registry-catalog-list">
        <div className="section-title">
          <h2>系统列表</h2>
          <span>系统 / 环境 / 服务 / 配置完整度</span>
        </div>
        <div className="toolbar">
          <input
            className="search"
            placeholder="搜索系统名 / key / 负责人"
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
                  <span className="badge ghost">{item.status}</span>
                  <span className="badge ghost">{item.owner}</span>
                  <span className="badge ghost">{item.environments.length} 环境</span>
                </div>
              </button>
            ))}
          </div>

          <div className="registry-catalog-detail panel" id="registry-catalog-detail">
            {selectedSystem ? (
              <>
                <div className="section-title">
                  <h2>{selectedSystem.name}</h2>
                  <span>{selectedSystem.owner} · {selectedSystem.repo}</span>
                </div>
                <div className="registry-detail-grid">
                  {selectedSystem.environments.map((env) => (
                    <div className="registry-detail-card" key={env.key}>
                      <div className="registry-detail-head">
                        <div>
                          <div className="row-title">{env.key}</div>
                          <div className="row-sub">类型 {env.type}</div>
                        </div>
                        <span className={`pill ${healthTone(env.health)}`}>{healthLabel(env.health)}</span>
                      </div>
                      <div className="registry-service-list">
                        {env.services.map((service) => (
                          <div className="registry-service-item" key={service.key}>
                            <div className="row-title">{service.key}</div>
                            <div className="row-sub">运行形态 {service.runtimeType}</div>
                            {service.route ? <div className="row-sub">入口 {service.route}</div> : null}
                            {service.healthTarget ? <div className="row-sub">健康 {service.healthTarget}</div> : null}
                            <div className="row-sub">配置完整度 {service.configCompleteness}</div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ))}
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
