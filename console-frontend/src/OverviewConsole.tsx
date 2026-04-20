/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于值班驾驶舱原型 强调当班判断、主线收口与跨域协同入口 */

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

type PriorityLaneItem = {
  id: string;
  title: string;
  owner: string;
  summary: string;
  tone: "danger" | "warning" | "info";
  view: ConsoleView;
  actionLabel: string;
};

type SystemHotspot = {
  system: string;
  owner: string;
  signal: string;
  trend: string;
  view: ConsoleView;
};

const POLL_MS = 5000;

const formatTime = (value: string | undefined) => {
  if (!value) return "--";
  const ts = Date.parse(value);
  if (!Number.isFinite(ts)) return value;
  return new Date(ts).toLocaleString();
};

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

const inferSystemProfile = (decision: AlertDecision) => {
  const keyword = `${decision.rule} ${decision.message}`.toLowerCase();
  if (keyword.includes("连接") || keyword.includes("订单") || keyword.includes("hikari")) {
    return {
      system: "订单风控中心",
      owner: "交易稳定性组",
      surface: "风控 API / 订单写入链路",
    };
  }
  if (keyword.includes("磁盘") || keyword.includes("io") || keyword.includes("文件")) {
    return {
      system: "文件接入网关",
      owner: "平台运维组",
      surface: "接入落盘 / 上传回放链路",
    };
  }
  if (keyword.includes("线程池") || keyword.includes("worker")) {
    return {
      system: "营销内容中心",
      owner: "内容平台组",
      surface: "任务消费 / 内容同步链路",
    };
  }
  return {
    system: "统一运维平台",
    owner: "平台稳定性组",
    surface: "公共控制面",
  };
};

const mapDecisionTone = (decision: AlertDecision): "danger" | "warning" | "info" => {
  if (decision.level === "fatal") return "danger";
  if (decision.level === "system") return "warning";
  return "info";
};

const levelLabel = (decision: AlertDecision) => {
  if (decision.level === "fatal") return "P1";
  if (decision.level === "system") return "P2";
  if (decision.level === "business") return "P3";
  return "记录";
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
        if (disposed) return;
        setError((err as Error).message);
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
  const uploadRecords = dashboard.uploadRecords ?? [];

  const controlSummary = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const task of tasks) {
      const status = task.status || "unknown";
      counts[status] = (counts[status] ?? 0) + 1;
    }
    return counts;
  }, [tasks]);

  const priorityLane = useMemo<PriorityLaneItem[]>(() => {
    const items: PriorityLaneItem[] = (alerts.decisions ?? []).slice(0, 3).map((decision) => {
      const profile = inferSystemProfile(decision);
      return {
        id: decision.id,
        title: `${levelLabel(decision)} · ${decision.message}`,
        owner: profile.owner,
        summary: `${profile.system} · ${profile.surface} · 规则 ${decision.rule}`,
        tone: mapDecisionTone(decision),
        view: "events",
        actionLabel: "进入事件工作台",
      };
    });

    if ((controlSummary.pending ?? 0) > 0) {
      items.push({
        id: "control-backlog",
        title: `执行积压 ${controlSummary.pending} 条`,
        owner: "自动化执行域",
        summary: `运行中 ${controlSummary.running ?? 0} 条，失败 Top1：${failureReasons[0]?.reason ?? "暂无"}`,
        tone: "warning",
        view: "control",
        actionLabel: "查看执行中心",
      });
    }

    return items;
  }, [alerts.decisions, controlSummary.pending, controlSummary.running, failureReasons]);

  const uniqueHotSystems = useMemo<SystemHotspot[]>(() => {
    const result: SystemHotspot[] = [];
    const seen = new Set<string>();

    for (const decision of alerts.decisions ?? []) {
      const profile = inferSystemProfile(decision);
      if (seen.has(profile.system)) continue;
      seen.add(profile.system);
      result.push({
        system: profile.system,
        owner: profile.owner,
        signal: `${decision.rule} · ${decision.message}`,
        trend: `最近告警时间 ${decision.time}`,
        view: "registryCatalog",
      });
      if (result.length >= 3) break;
    }

    if (!result.length) {
      result.push(
        {
          system: "订单风控中心",
          owner: "交易稳定性组",
          signal: "数据库连接池与慢 SQL 是当前最高风险面",
          trend: "建议从系统详情进入，再跳转事件工作台",
          view: "registryCatalog",
        },
        {
          system: "文件接入网关",
          owner: "平台运维组",
          signal: "上传失败与重试堆积会直接拉长 MTTR",
          trend: "建议联动接入工作台和执行中心",
          view: "console",
        }
      );
    }

    return result;
  }, [alerts.decisions]);

  const shiftCards = useMemo(
    () => [
      {
        label: "高风险事件",
        value: String((alerts.overview.fatal ?? 0) + (alerts.overview.system ?? 0)),
        hint: `P1 ${alerts.overview.fatal ?? 0} · P2 ${alerts.overview.system ?? 0}`,
      },
      {
        label: "待认领动作",
        value: String(controlSummary.pending ?? 0),
        hint: `运行中 ${controlSummary.running ?? 0} · 超时 ${controlSummary.timeout ?? 0}`,
      },
      {
        label: "在线 Agent",
        value: String(agents.filter((item) => item.status === "online").length),
        hint: `总数 ${agents.length} · draining ${agents.filter((item) => item.status === "draining").length}`,
      },
      {
        label: "主线健康",
        value: alerts.overview.risk || "--",
        hint: `监控目录 ${hero.watchDirs?.length ?? 0} 个 · 最近刷新 ${formatTime(new Date().toISOString())}`,
      },
    ],
    [agents, alerts.overview.fatal, alerts.overview.risk, alerts.overview.system, controlSummary.pending, controlSummary.running, controlSummary.timeout, hero.watchDirs]
  );

  const handoffItems = useMemo(
    () => [
      {
        title: "班次交接判断",
        detail: `当前窗口 ${alerts.overview.window || "--"} 内，建议先盯住 ${alerts.overview.fatal ?? 0} 条 P1 和 ${alerts.overview.system ?? 0} 条系统级事件。`,
      },
      {
        title: "最可能拉长 MTTR 的点",
        detail: failureReasons[0]?.reason
          ? `执行失败 Top1：${failureReasons[0].reason}`
          : "当前没有明显失败热点，优先保持事件工作台和系统详情的链路可用。",
      },
      {
        title: "最近变化面",
        detail: uploadRecords[0]?.file
          ? `最近上传 ${uploadRecords[0].file}，如事件与发布或回放相关，优先回看接入链路。`
          : "暂无新的上传或回放材料，必要时从事件页补充证据。",
      },
      {
        title: "知识沉淀提醒",
        detail: `当前已发送告警 ${alerts.stats.sent ?? 0} 条，建议把高频案例整理成 SOP，减少下次处理成本。`,
      },
    ],
    [alerts.overview.fatal, alerts.overview.system, alerts.overview.window, alerts.stats.sent, failureReasons, uploadRecords]
  );

  const workflowLanes = useMemo(
    () => [
      {
        title: "发现",
        summary: `近窗口命中 ${alerts.stats.sent + alerts.stats.suppressed + alerts.stats.recorded} 条`,
        detail: `来源 ${alerts.rules.source || "--"} · 轮询 ${alerts.polling.interval || "--"}`,
      },
      {
        title: "分诊",
        summary: `已通知 ${alerts.stats.sent ?? 0} / 已抑制 ${alerts.stats.suppressed ?? 0}`,
        detail: `默认抑制窗口 ${alerts.rules.defaultSuppress || "--"}`,
      },
      {
        title: "处置",
        summary: `待执行 ${controlSummary.pending ?? 0} / 运行中 ${controlSummary.running ?? 0}`,
        detail: `失败热点 ${failureReasons[0]?.reason ?? "暂无"}`,
      },
      {
        title: "沉淀",
        summary: `系统主线 ${uniqueHotSystems.length} 个热点面`,
        detail: "建议把事件页中的诊断和动作直接回写为 SOP 草稿",
      },
    ],
    [alerts.polling.interval, alerts.rules.defaultSuppress, alerts.rules.source, alerts.stats.recorded, alerts.stats.sent, alerts.stats.suppressed, controlSummary.pending, controlSummary.running, failureReasons, uniqueHotSystems.length]
  );

  const quickActions = useMemo(
    () => [
      {
        title: "进入事件工作台",
        desc: "处理当前最高优先级事件",
        view: "events" as const,
      },
      {
        title: "查看系统详情",
        desc: "先把排障上下文补齐",
        view: "registryCatalog" as const,
      },
      {
        title: "检查接入链路",
        desc: "上传、回放、探测与待确认项",
        view: "console" as const,
      },
      {
        title: "打开执行中心",
        desc: "确认动作是否真正落下去",
        view: "control" as const,
      },
    ],
    []
  );

  const opsMoments = useMemo(
    () => [
      {
        title: "文件入云",
        detail: metricCards[1] ? `${metricCards[1].value} · ${metricCards[1].trend}` : "等待接入链路数据",
      },
      {
        title: "告警决策",
        detail: `规则总数 ${alerts.rules.total ?? 0} · 升级策略 ${alerts.rules.escalation || "--"}`,
      },
      {
        title: "AI / 分析",
        detail: monitorSummary[2] ? `${monitorSummary[2].label} ${monitorSummary[2].value}` : "等待日志和 AI 摘要进入闭环",
      },
      {
        title: "知识 / 复用",
        detail: `当前更适合把“高频故障 + 动作链”沉淀为 SOP，而不是继续增加散页。`,
      },
    ],
    [alerts.rules.escalation, alerts.rules.total, metricCards, monitorSummary]
  );

  return (
    <div className="overview-shell">
      <section className="panel overview-command" id="overview-summary">
        <div className="overview-command-main">
          <div className="overview-command-head">
            <div>
              <p className="eyebrow">值班驾驶舱</p>
              <h2>让值班人先判断“现在最危险的是什么”，再决定跳去哪一页。</h2>
            </div>
            <span className="overview-refresh">{loading ? "刷新中..." : `最近刷新 ${formatTime(new Date().toISOString())}`}</span>
          </div>
          {error ? <div className="overview-banner">接口异常：{error}</div> : null}
          <p className="overview-command-copy">
            当前不是再去找某个模块，而是先盯住高风险事件、临近超时动作和热点系统。
            只有这三件事明确了，接入、知识、执行这些支撑面才知道该为谁服务。
          </p>
          <div className="overview-signal-strip">
            <span className="overview-signal-pill danger">P1 {alerts.overview.fatal ?? 0}</span>
            <span className="overview-signal-pill warning">系统级 {alerts.overview.system ?? 0}</span>
            <span className="overview-signal-pill info">业务级 {alerts.overview.business ?? 0}</span>
            <span className="overview-signal-pill neutral">未恢复 {alerts.stats.sent ?? 0}</span>
            <span className="overview-signal-pill neutral">抑制 {alerts.stats.suppressed ?? 0}</span>
          </div>
          <div className="overview-quick-actions">
            {quickActions.map((item) => (
              <button className="overview-quick-button" key={item.title} type="button" onClick={() => onViewChange(item.view)}>
                <strong>{item.title}</strong>
                <span>{item.desc}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="overview-command-side">
          {shiftCards.map((card) => (
            <div className="overview-shift-card" key={card.label}>
              <div className="overview-shift-label">{card.label}</div>
              <div className="overview-shift-value">{card.value}</div>
              <div className="overview-shift-hint">{card.hint}</div>
            </div>
          ))}
        </div>
      </section>

      <div className="overview-grid overview-grid-primary" id="overview-timeline">
        <section className="panel">
          <div className="section-title">
            <h2>当前优先队列</h2>
            <span>值班先做什么，不再靠脑补拼流程</span>
          </div>
          <div className="overview-priority-list">
            {priorityLane.length ? (
              priorityLane.map((item) => (
                <div className={`overview-priority-item tone-${item.tone}`} key={item.id}>
                  <div className="overview-priority-main">
                    <div className="overview-priority-owner">{item.owner}</div>
                    <strong>{item.title}</strong>
                    <span>{item.summary}</span>
                  </div>
                  <button className="btn secondary" type="button" onClick={() => onViewChange(item.view)}>
                    {item.actionLabel}
                  </button>
                </div>
              ))
            ) : (
              <div className="empty-state">当前没有待收口的高优先级事项</div>
            )}
          </div>
        </section>

        <section className="panel">
          <div className="section-title">
            <h2>交接与判断</h2>
            <span>把“为什么先处理这个”说清楚</span>
          </div>
          <div className="overview-handoff-list">
            {handoffItems.map((item) => (
              <div className="overview-handoff-item" key={item.title}>
                <strong>{item.title}</strong>
                <span>{item.detail}</span>
              </div>
            ))}
          </div>
        </section>
      </div>

      <div className="overview-grid" id="overview-runtime">
        <section className="panel">
          <div className="section-title">
            <h2>系统热点</h2>
            <span>参考 NetBox / Backstage 的思路，把排障上下文挂回系统对象</span>
          </div>
          <div className="overview-hotspot-list">
            {uniqueHotSystems.map((item) => (
              <button className="overview-hotspot-card" key={item.system} type="button" onClick={() => onViewChange(item.view)}>
                <div className="overview-hotspot-head">
                  <strong>{item.system}</strong>
                  <span className="badge ghost">{item.owner}</span>
                </div>
                <div className="overview-hotspot-signal">{item.signal}</div>
                <div className="overview-hotspot-trend">{item.trend}</div>
              </button>
            ))}
          </div>
        </section>

        <section className="panel">
          <div className="section-title">
            <h2>闭环主线</h2>
            <span>参考 OneUptime / GoAlert / Rundeck，把发现到执行压成一条线</span>
          </div>
          <div className="overview-lane-list">
            {workflowLanes.map((item) => (
              <div className="overview-lane-card" key={item.title}>
                <div className="overview-lane-title">{item.title}</div>
                <strong>{item.summary}</strong>
                <span>{item.detail}</span>
              </div>
            ))}
          </div>
        </section>
      </div>

      <div className="overview-grid" id="overview-actions">
        <section className="panel">
          <div className="section-title">
            <h2>主线运行观察</h2>
            <span>不是铺更多组件，而是确认每条主线是否真的连上了</span>
          </div>
          <div className="overview-moment-list">
            {opsMoments.map((item) => (
              <div className="overview-moment-item" key={item.title}>
                <strong>{item.title}</strong>
                <span>{item.detail}</span>
              </div>
            ))}
          </div>
        </section>

        <section className="panel">
          <div className="section-title">
            <h2>当班备注</h2>
            <span>把重要上下文留在首页，而不是散在多个模块里</span>
          </div>
          <div className="overview-owner-card">
            <strong>当前班次建议默认路径</strong>
            <p>
              先从值班页认清风险，再进入事件工作台完成处置，最后用系统详情补齐系统、服务、入口、SOP 和责任人上下文。
            </p>
            <div className="overview-owner-tags">
              <span className="badge ghost">值班先看风险</span>
              <span className="badge ghost">事件页收口动作</span>
              <span className="badge ghost">系统页承接上下文</span>
              <span className="badge ghost">支撑域围绕主线服务</span>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
