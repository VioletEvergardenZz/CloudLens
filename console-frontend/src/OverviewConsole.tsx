/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于统一运维总览页，将风险、事件、系统热点和动作积压收敛成标准后台首页。 */

import { useEffect, useMemo, useState } from "react";
import "./OverviewConsole.css";
import {
  alertDashboard as alertDashboardMock,
  heroCopy as heroCopyMock,
  metricCards as metricCardsMock,
  monitorSummary as monitorSummaryMock,
} from "./mockData";
import type {
  AlertDashboard,
  AlertDecision,
  AlertDecisionStatus,
  ConsoleView,
  ControlAgent,
  ControlTask,
  ControlTaskFailureReason,
  DashboardPayload,
} from "./types";
import {
  USE_MOCK,
  fetchAlertDashboard,
  fetchControlAgents,
  fetchControlTaskFailureReasons,
  fetchControlTasks,
  fetchDashboardLite,
} from "./console/dashboardApi";

type OverviewConsoleProps = {
  onViewChange: (view: ConsoleView) => void;
};

type SystemProfile = {
  system: string;
  service: string;
  owner: string;
};

type IncidentRow = {
  id: string;
  levelLabel: string;
  levelTone: "danger" | "warning" | "info";
  title: string;
  system: string;
  owner: string;
  statusLabel: string;
  reason: string;
  time: string;
};

type HotspotRow = {
  system: string;
  owner: string;
  service: string;
  latestSignal: string;
  latestTime: string;
  priority: string;
};

type ActionCard = {
  label: string;
  value: string;
  detail: string;
  view: ConsoleView;
};

const POLL_MS = 5000;

const emptyAlertDashboard: AlertDashboard = {
  overview: {
    window: "--",
    risk: "--",
    fatal: 0,
    system: 0,
    business: 0,
    sent: 0,
    suppressed: 0,
    latest: "--",
  },
  decisions: [],
  stats: {
    sent: 0,
    suppressed: 0,
    recorded: 0,
  },
  rules: {
    source: "--",
    lastLoaded: "--",
    total: 0,
    defaultSuppress: "--",
    escalation: "--",
    levels: {
      ignore: 0,
      business: 0,
      system: 0,
      fatal: 0,
    },
  },
  polling: {
    interval: "--",
    logFiles: [],
    lastPoll: "--",
    nextPoll: "--",
  },
};

const emptyDashboardLite: Partial<DashboardPayload> = {
  heroCopy: heroCopyMock,
  metricCards: metricCardsMock,
  monitorSummary: monitorSummaryMock,
  uploadRecords: [],
};

const formatTime = (value?: string) => {
  if (!value) return "--";
  const ts = Date.parse(value);
  if (!Number.isFinite(ts)) return value;
  return new Date(ts).toLocaleString();
};

const inferSystemProfile = (decision: AlertDecision): SystemProfile => {
  const keyword = `${decision.rule} ${decision.message}`.toLowerCase();

  if (keyword.includes("连接") || keyword.includes("hikari") || keyword.includes("order")) {
    return {
      system: "订单风控中心",
      service: "risk-api / order-writer",
      owner: "交易稳定性组",
    };
  }

  if (keyword.includes("磁盘") || keyword.includes("io") || keyword.includes("文件")) {
    return {
      system: "文件接入网关",
      service: "gwf-api / upload-worker",
      owner: "平台运维组",
    };
  }

  if (keyword.includes("线程池") || keyword.includes("worker")) {
    return {
      system: "营销内容中心",
      service: "content-worker / scheduler",
      owner: "内容平台组",
    };
  }

  return {
    system: "统一运维平台",
    service: "公共控制面",
    owner: "平台稳定性组",
  };
};

const resolveLevelLabel = (decision: AlertDecision) => {
  if (decision.level === "fatal") return "P1";
  if (decision.level === "system") return "P2";
  if (decision.level === "business") return "P3";
  return "记录";
};

const resolveLevelTone = (decision: AlertDecision): IncidentRow["levelTone"] => {
  if (decision.level === "fatal") return "danger";
  if (decision.level === "system") return "warning";
  return "info";
};

const resolveStatusLabel = (status: AlertDecisionStatus) => {
  if (status === "sent") return "处理中";
  if (status === "suppressed") return "已抑制";
  return "仅记录";
};

export function OverviewConsole({ onViewChange }: OverviewConsoleProps) {
  const [dashboard, setDashboard] = useState<Partial<DashboardPayload>>(USE_MOCK ? emptyDashboardLite : {});
  const [alerts, setAlerts] = useState<AlertDashboard>(USE_MOCK ? alertDashboardMock : emptyAlertDashboard);
  const [agents, setAgents] = useState<ControlAgent[]>([]);
  const [tasks, setTasks] = useState<ControlTask[]>([]);
  const [failureReasons, setFailureReasons] = useState<ControlTaskFailureReason[]>([]);
  const [loading, setLoading] = useState(!USE_MOCK);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let disposed = false;

    const refresh = async () => {
      if (USE_MOCK) return;
      try {
        setLoading(true);
        const [dashboardResp, alertResp, agentResp, taskResp, failureResp] = await Promise.all([
          fetchDashboardLite(),
          fetchAlertDashboard(),
          fetchControlAgents(),
          fetchControlTasks({ limit: 50 }),
          fetchControlTaskFailureReasons({ status: "failed,timeout", limit: 5 }),
        ]);

        if (disposed) return;
        setDashboard(dashboardResp);
        setAlerts(alertResp.data ?? emptyAlertDashboard);
        setAgents(agentResp.items ?? []);
        setTasks(taskResp.items ?? []);
        setFailureReasons(failureResp.items ?? []);
        setError(null);
      } catch (err) {
        if (!disposed) {
          setError((err as Error).message);
        }
      } finally {
        if (!disposed) {
          setLoading(false);
        }
      }
    };

    void refresh();
    if (!USE_MOCK) {
      const timer = window.setInterval(() => void refresh(), POLL_MS);
      return () => {
        disposed = true;
        window.clearInterval(timer);
      };
    }

    return () => {
      disposed = true;
    };
  }, []);

  const hero = dashboard.heroCopy ?? heroCopyMock;
  const metricCards = dashboard.metricCards ?? metricCardsMock;
  const monitorSummary = dashboard.monitorSummary ?? monitorSummaryMock;

  const controlSummary = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const task of tasks) {
      const status = task.status || "unknown";
      counts[status] = (counts[status] ?? 0) + 1;
    }
    return counts;
  }, [tasks]);

  const incidentRows = useMemo<IncidentRow[]>(() => {
    return (alerts.decisions ?? []).slice(0, 8).map((decision) => {
      const profile = inferSystemProfile(decision);
      return {
        id: decision.id,
        levelLabel: resolveLevelLabel(decision),
        levelTone: resolveLevelTone(decision),
        title: decision.message,
        system: profile.system,
        owner: profile.owner,
        statusLabel: resolveStatusLabel(decision.status),
        reason: decision.reason || decision.rule,
        time: decision.time,
      };
    });
  }, [alerts.decisions]);

  const hotspotRows = useMemo<HotspotRow[]>(() => {
    const rows: HotspotRow[] = [];
    const seen = new Set<string>();

    for (const decision of alerts.decisions ?? []) {
      const profile = inferSystemProfile(decision);
      if (seen.has(profile.system)) continue;
      seen.add(profile.system);
      rows.push({
        system: profile.system,
        owner: profile.owner,
        service: profile.service,
        latestSignal: `${decision.rule} · ${decision.message}`,
        latestTime: decision.time,
        priority: resolveLevelLabel(decision),
      });
    }

    if (!rows.length) {
      rows.push(
        {
          system: "订单风控中心",
          owner: "交易稳定性组",
          service: "risk-api / order-writer",
          latestSignal: "数据库连接池和慢 SQL 仍是当前最高频风险面",
          latestTime: "--",
          priority: "P1",
        },
        {
          system: "文件接入网关",
          owner: "平台运维组",
          service: "gwf-api / upload-worker",
          latestSignal: "上传失败与回放重试容易拉长 MTTR",
          latestTime: "--",
          priority: "P2",
        }
      );
    }

    return rows.slice(0, 6);
  }, [alerts.decisions]);

  const summaryCards = useMemo(
    () => [
      {
        label: "待处理事件",
        value: String((alerts.overview.fatal ?? 0) + (alerts.overview.system ?? 0) + (alerts.overview.business ?? 0)),
        detail: `P1 ${alerts.overview.fatal ?? 0} · P2 ${alerts.overview.system ?? 0} · P3 ${alerts.overview.business ?? 0}`,
      },
      {
        label: "动作积压",
        value: String((controlSummary.pending ?? 0) + (controlSummary.running ?? 0)),
        detail: `待执行 ${controlSummary.pending ?? 0} · 运行中 ${controlSummary.running ?? 0}`,
      },
      {
        label: "在线 Agent",
        value: String(agents.filter((item) => item.status === "online").length),
        detail: `总数 ${agents.length} · draining ${agents.filter((item) => item.status === "draining").length}`,
      },
      {
        label: "系统热点",
        value: String(hotspotRows.length),
        detail: `规则 ${alerts.rules.total ?? 0} · 轮询 ${alerts.polling.interval || "--"}`,
      },
    ],
    [
      agents,
      alerts.overview.business,
      alerts.overview.fatal,
      alerts.overview.system,
      alerts.polling.interval,
      alerts.rules.total,
      controlSummary.pending,
      controlSummary.running,
      hotspotRows.length,
    ]
  );

  const actionCards = useMemo<ActionCard[]>(
    () => [
      {
        label: "执行积压",
        value: `${controlSummary.pending ?? 0} 条待处理`,
        detail: `失败热点：${failureReasons[0]?.reason ?? "暂无明显热点"}`,
        view: "control",
      },
      {
        label: "文件入云",
        value: metricCards[1]?.value ?? "--",
        detail: metricCards[1]?.trend ?? "等待接入链路数据",
        view: "console",
      },
      {
        label: "AI / 分析",
        value: monitorSummary[2]?.value ?? "--",
        detail: monitorSummary[2]?.desc ?? "等待更多日志与摘要进入闭环",
        view: "events",
      },
      {
        label: "知识复用",
        value: `${alerts.stats.sent ?? 0} 条已通知`,
        detail: "高频事件应及时沉淀为 SOP，而不是继续散落在多个模块里。",
        view: "knowledge",
      },
    ],
    [alerts.stats.sent, controlSummary.pending, failureReasons, metricCards, monitorSummary]
  );

  const handoffNotes = useMemo(
    () => [
      {
        title: "当班先看什么",
        detail: `当前窗口 ${alerts.overview.window || "--"} 内，优先关注 ${alerts.overview.fatal ?? 0} 条 P1 和 ${alerts.overview.system ?? 0} 条 P2。`,
      },
      {
        title: "最可能拖慢恢复的点",
        detail: failureReasons[0]?.reason
          ? `执行失败 Top1：${failureReasons[0].reason}`
          : "当前没有明显失败热点，建议先保证事件详情页和系统详情页的上下文完整。",
      },
      {
        title: "当前主线状态",
        detail: `监控目录 ${hero.watchDirs?.length ?? 0} 个，风险等级 ${alerts.overview.risk || "--"}，最近事件 ${formatTime(alerts.overview.latest)}。`,
      },
    ],
    [alerts.overview.fatal, alerts.overview.latest, alerts.overview.risk, alerts.overview.system, alerts.overview.window, failureReasons, hero.watchDirs]
  );

  const workflowStatus = useMemo(
    () => [
      {
        title: "文件入云",
        detail: metricCards[1] ? `${metricCards[1].value} · ${metricCards[1].trend}` : "等待接入链路数据",
      },
      {
        title: "告警决策",
        detail: `规则 ${alerts.rules.total ?? 0} 条 · 已发送 ${alerts.stats.sent ?? 0} · 抑制 ${alerts.stats.suppressed ?? 0}`,
      },
      {
        title: "AI 分析",
        detail: monitorSummary[2] ? `${monitorSummary[2].label} ${monitorSummary[2].value}` : "等待日志和 AI 摘要进入闭环",
      },
      {
        title: "知识复用",
        detail: "把高频事件的诊断和动作沉淀为 SOP，减少下次处理成本。",
      },
    ],
    [alerts.rules.total, alerts.stats.sent, alerts.stats.suppressed, metricCards, monitorSummary]
  );

  return (
    <div className="overview-shell">
      <section className="panel overview-page-header" id="overview-header">
        <div className="overview-header-top">
          <div className="overview-header-copy">
            <p className="eyebrow">统一运维总览</p>
            <h2>先判断当前风险，再进入事件或系统详情</h2>
            <p>
              总览页只保留值班必须知道的四类信息：风险、事件队列、系统热点和动作积压。
              不再把概念卡片堆成首页，而是收敛成标准后台入口。
            </p>
          </div>

          <div className="overview-header-side">
            <div className="overview-meta-card">
              <span>风险等级</span>
              <strong>{alerts.overview.risk || "--"}</strong>
              <small>{loading ? "正在刷新…" : `最近刷新 ${formatTime(new Date().toISOString())}`}</small>
            </div>
            <div className="overview-header-actions">
              <button className="btn" type="button" onClick={() => onViewChange("alert")}>
                进入事件中心
              </button>
              <button className="btn secondary" type="button" onClick={() => onViewChange("registryCatalog")}>
                查看系统台账
              </button>
            </div>
          </div>
        </div>

        {error ? <div className="overview-banner">接口异常：{error}</div> : null}

        <div className="overview-stat-grid">
          {summaryCards.map((item) => (
            <div className="overview-stat-card" key={item.label}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
              <small>{item.detail}</small>
            </div>
          ))}
        </div>
      </section>

      <div className="overview-primary-grid">
        <section className="panel" id="overview-incidents">
          <div className="section-title">
            <div>
              <h2>当前事件队列</h2>
              <span>首页应该先回答“现在最值得处理的是哪几件事”</span>
            </div>
            <div className="overview-toolbar">
              <span className="badge ghost">窗口 {alerts.overview.window || "--"}</span>
              <span className="badge ghost">来源 {alerts.rules.source || "--"}</span>
              <span className="badge ghost">已发送 {alerts.stats.sent ?? 0}</span>
            </div>
          </div>

          {incidentRows.length ? (
            <div className="table-wrap">
              <table className="table overview-table">
                <thead>
                  <tr>
                    <th>级别</th>
                    <th>事件</th>
                    <th>系统</th>
                    <th>负责人</th>
                    <th>状态</th>
                    <th>时间</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {incidentRows.map((row) => (
                    <tr key={row.id}>
                      <td>
                        <span className={`pill ${row.levelTone}`}>{row.levelLabel}</span>
                      </td>
                      <td>
                        <div className="overview-row-main">
                          <div className="row-title">{row.title}</div>
                          <div className="row-sub">{row.reason}</div>
                        </div>
                      </td>
                      <td>
                        <div className="row-title">{row.system}</div>
                      </td>
                      <td>{row.owner}</td>
                      <td>
                        <span className="badge ghost">{row.statusLabel}</span>
                      </td>
                      <td>{row.time}</td>
                      <td>
                        <button className="btn secondary overview-table-action" type="button" onClick={() => onViewChange("events")}>
                          处置
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="empty-state">当前没有需要收口的事件。</div>
          )}
        </section>

        <div className="overview-side-stack">
          <section className="panel">
            <div className="section-title">
              <div>
                <h2>当班判断</h2>
                <span>把交接需要的上下文固定留在首页</span>
              </div>
            </div>
            <div className="overview-note-list">
              {handoffNotes.map((item) => (
                <div className="overview-note-item" key={item.title}>
                  <strong>{item.title}</strong>
                  <span>{item.detail}</span>
                </div>
              ))}
            </div>
          </section>

          <section className="panel" id="overview-actions">
            <div className="section-title">
              <div>
                <h2>动作与支撑域</h2>
                <span>接入、AI、知识和执行都应该围绕主线服务</span>
              </div>
            </div>
            <div className="overview-action-list">
              {actionCards.map((item) => (
                <button className="overview-action-card" key={item.label} type="button" onClick={() => onViewChange(item.view)}>
                  <div className="overview-action-top">
                    <strong>{item.label}</strong>
                    <span className="badge ghost">查看</span>
                  </div>
                  <div className="overview-action-value">{item.value}</div>
                  <div className="row-sub">{item.detail}</div>
                </button>
              ))}
            </div>
          </section>
        </div>
      </div>

      <div className="overview-secondary-grid">
        <section className="panel" id="overview-systems">
          <div className="section-title">
            <div>
              <h2>系统热点</h2>
              <span>参考 Backstage / NetBox 的思路，把排障上下文挂回系统对象</span>
            </div>
            <button className="btn secondary" type="button" onClick={() => onViewChange("registryCatalog")}>
              打开系统台账
            </button>
          </div>

          <div className="table-wrap">
            <table className="table overview-table">
              <thead>
                <tr>
                  <th>系统</th>
                  <th>服务</th>
                  <th>负责人</th>
                  <th>最新信号</th>
                  <th>优先级</th>
                  <th>时间</th>
                </tr>
              </thead>
              <tbody>
                {hotspotRows.map((row) => (
                  <tr key={`${row.system}-${row.latestSignal}`}>
                    <td>
                      <div className="row-title">{row.system}</div>
                    </td>
                    <td>{row.service}</td>
                    <td>{row.owner}</td>
                    <td>
                      <div className="overview-row-main">
                        <div className="row-title">{row.latestSignal}</div>
                      </div>
                    </td>
                    <td>
                      <span className={`pill ${row.priority === "P1" ? "danger" : row.priority === "P2" ? "warning" : "info"}`}>
                        {row.priority}
                      </span>
                    </td>
                    <td>{row.latestTime}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel">
          <div className="section-title">
            <div>
              <h2>闭环主线状态</h2>
              <span>文件入云、告警决策、AI 分析和知识复用要能连成一条线</span>
            </div>
          </div>
          <div className="overview-workflow-list">
            {workflowStatus.map((item) => (
              <div className="overview-workflow-item" key={item.title}>
                <strong>{item.title}</strong>
                <span>{item.detail}</span>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
