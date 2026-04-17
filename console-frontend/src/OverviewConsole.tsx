/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于平台总览页 统一展示全局状态 待办 与关键链路概况 */

import { useEffect, useMemo, useState } from "react";
import "./OverviewConsole.css";
import {
  alertDashboard as alertDashboardMock,
  heroCopy as heroCopyMock,
  metricCards as metricCardsMock,
  monitorSummary as monitorSummaryMock,
} from "./mockData";
import type { AlertDashboard, ConsoleView, ControlAgent, ControlTask, ControlTaskFailureReason, DashboardPayload } from "./types";
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

  const globalCards = useMemo(
    () => [
      {
        label: "未恢复告警",
        value: String((alerts.overview.fatal ?? 0) + (alerts.overview.system ?? 0) + (alerts.overview.business ?? 0)),
        hint: `风险 ${alerts.overview.risk || "--"}`,
        action: () => onViewChange("alert"),
      },
      {
        label: "在线 Agent",
        value: String(agents.filter((item) => item.status === "online").length),
        hint: `总数 ${agents.length}`,
        action: () => onViewChange("control"),
      },
      {
        label: "控制面 backlog",
        value: String(controlSummary.pending ?? 0),
        hint: `执行中 ${controlSummary.running ?? 0}`,
        action: () => onViewChange("control"),
      },
      {
        label: "监控目录",
        value: String(hero.watchDirs?.length ?? 0),
        hint: `当前节点 ${hero.agent || "--"}`,
        action: () => onViewChange("console"),
      },
      {
        label: "接入原型",
        value: "已接入",
        hint: "域名探测 / 待确认项",
        action: () => onViewChange("registry"),
      },
      {
        label: "知识库状态",
        value: `${alerts.stats.sent ?? 0} 条已通知`,
        hint: "支持告警知识联动",
        action: () => onViewChange("knowledge"),
      },
    ],
    [agents, alerts, controlSummary.pending, controlSummary.running, hero.agent, hero.watchDirs, onViewChange]
  );

  const todoItems = useMemo(
    () => [
      `高等级告警待处理：${alerts.overview.fatal ?? 0} 条`,
      `控制面待分配任务：${controlSummary.pending ?? 0} 条`,
      `最近失败原因：${failureReasons[0]?.reason ?? "暂无"}`,
      `最近上传：${uploadRecords[0]?.file ?? "暂无记录"}`,
    ],
    [alerts.overview.fatal, controlSummary.pending, failureReasons, uploadRecords]
  );

  const timelineItems = useMemo(() => {
    return (alerts.decisions ?? []).slice(0, 5).map((item) => ({
      id: item.id,
      time: item.time,
      title: `${item.rule} · ${item.message}`,
      status: item.status,
    }));
  }, [alerts.decisions]);

  return (
    <div className="overview-shell">
      <section className="panel overview-hero" id="overview-summary">
        <div className="section-title">
          <div>
            <h2>平台总览</h2>
            <span>统一查看当前运维事件闭环、接入状态与待处理事项</span>
          </div>
          <span>{loading ? "刷新中..." : `最近刷新 ${formatTime(new Date().toISOString())}`}</span>
        </div>
        {error ? <div className="overview-banner">接口异常：{error}</div> : null}
        <div className="overview-card-grid">
          {globalCards.map((card) => (
            <button className="overview-stat-card" key={card.label} type="button" onClick={card.action}>
              <div className="overview-stat-label">{card.label}</div>
              <div className="overview-stat-value">{card.value}</div>
              <div className="overview-stat-hint">{card.hint}</div>
            </button>
          ))}
        </div>
      </section>

      <div className="overview-grid">
        <section className="panel" id="overview-timeline">
          <div className="section-title">
            <h2>事件态势</h2>
            <span>最近告警 / 时间线</span>
          </div>
          <div className="overview-timeline-list">
            {timelineItems.length ? (
              timelineItems.map((item) => (
                <button className="overview-timeline-item" key={item.id} type="button" onClick={() => onViewChange("events")}>
                  <div className="overview-timeline-time">{item.time}</div>
                  <div className="overview-timeline-main">
                    <strong>{item.title}</strong>
                    <span>{item.status}</span>
                  </div>
                </button>
              ))
            ) : (
              <div className="empty-state">暂无事件时间线</div>
            )}
          </div>
        </section>

        <section className="panel" id="overview-runtime">
          <div className="section-title">
            <h2>主线运行健康</h2>
            <span>文件入云 / 告警 / AI / 控制面</span>
          </div>
          <div className="overview-health-grid">
            {metricCards.map((item) => (
              <div className="overview-health-card" key={item.label}>
                <div className="overview-health-label">{item.label}</div>
                <div className="overview-health-value">{item.value}</div>
                <div className="overview-health-hint">{item.trend}</div>
              </div>
            ))}
            {monitorSummary.map((item) => (
              <div className="overview-health-card compact" key={item.label}>
                <div className="overview-health-label">{item.label}</div>
                <div className="overview-health-value">{item.value}</div>
                <div className="overview-health-hint">{item.desc}</div>
              </div>
            ))}
          </div>
        </section>
      </div>

      <div className="overview-grid">
        <section className="panel" id="overview-actions">
          <div className="section-title">
            <h2>当前待办</h2>
            <span>值班优先级</span>
          </div>
          <div className="overview-todo-list">
            {todoItems.map((item, index) => (
              <div className="overview-todo-item" key={`${item}-${index}`}>
                <span className="overview-todo-index">{index + 1}</span>
                <span>{item}</span>
              </div>
            ))}
          </div>
        </section>

        <section className="panel">
          <div className="section-title">
            <h2>最近变化</h2>
            <span>接入 / 回放 / 失败原因</span>
          </div>
          <div className="overview-change-list">
            <div className="overview-change-item">
              <strong>文件接入</strong>
              <span>监控目录 {hero.watchDirs?.join(" / ") || "--"}</span>
            </div>
            <div className="overview-change-item">
              <strong>域名接入</strong>
              <span>已具备域名探测、候选健康接口、待确认项原型</span>
            </div>
            <div className="overview-change-item">
              <strong>失败原因 Top1</strong>
              <span>{failureReasons[0]?.reason ?? "暂无失败样本"}</span>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
