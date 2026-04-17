/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于事件工作台原型页 聚合告警 AI 知识与控制面信息 */

import { useEffect, useMemo, useState } from "react";
import "./EventWorkbench.css";
import { alertDashboard as alertDashboardMock } from "./mockData";
import type { AlertDecision, AlertDashboard, ConsoleView, ControlTask, KnowledgeArticle } from "./types";
import {
  USE_MOCK,
  fetchAlertDashboard,
  fetchControlTasks,
  fetchKBRecommendations,
} from "./console/dashboardApi";

type EventWorkbenchProps = {
  onViewChange: (view: ConsoleView) => void;
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

const buildEventTimeline = (decision: AlertDecision) => {
  const items = [
    { title: "告警生成", detail: decision.time },
    { title: "规则命中", detail: decision.rule },
  ];
  if (decision.explain?.decisionKind === "escalation") {
    items.push({
      title: "升级触发",
      detail: `${decision.explain.escalationCount ?? 0}/${decision.explain.escalationThreshold ?? 0}`,
    });
  }
  if (decision.reason) {
    items.push({ title: "当前状态说明", detail: decision.reason });
  }
  if (decision.knowledgeTrace?.linkedAt) {
    items.push({ title: "知识推荐关联", detail: decision.knowledgeTrace.linkedAt });
  }
  return items;
};

export function EventWorkbench({ onViewChange }: EventWorkbenchProps) {
  const [dashboard, setDashboard] = useState<AlertDashboard>(USE_MOCK ? alertDashboardMock : emptyAlertDashboard);
  const [tasks, setTasks] = useState<ControlTask[]>([]);
  const [recommendations, setRecommendations] = useState<KnowledgeArticle[]>([]);
  const [selectedEventId, setSelectedEventId] = useState<string>(USE_MOCK ? alertDashboardMock.decisions[0]?.id ?? "" : "");
  const [loading, setLoading] = useState(!USE_MOCK);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let disposed = false;
    const refresh = async () => {
      if (USE_MOCK) return;
      try {
        setLoading(true);
        const [alertResp, taskResp] = await Promise.all([fetchAlertDashboard(), fetchControlTasks({ limit: 20 })]);
        if (disposed) return;
        setDashboard(alertResp.data ?? emptyAlertDashboard);
        setTasks(taskResp.items ?? []);
        setSelectedEventId((prev) => prev || alertResp.data?.decisions?.[0]?.id || "");
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
    return () => {
      disposed = true;
    };
  }, []);

  const selectedEvent = useMemo(
    () => dashboard.decisions.find((item) => item.id === selectedEventId) ?? dashboard.decisions[0] ?? null,
    [dashboard.decisions, selectedEventId]
  );

  useEffect(() => {
    let disposed = false;
    const refreshRecommendations = async () => {
      if (!selectedEvent) {
        setRecommendations([]);
        return;
      }
      if (USE_MOCK) {
        setRecommendations([]);
        return;
      }
      try {
        const resp = await fetchKBRecommendations({
          alertId: selectedEvent.id,
          rule: selectedEvent.rule,
          message: selectedEvent.message,
          limit: 3,
        });
        if (!disposed) {
          setRecommendations(resp.items ?? []);
        }
      } catch {
        if (!disposed) {
          setRecommendations([]);
        }
      }
    };
    void refreshRecommendations();
    return () => {
      disposed = true;
    };
  }, [selectedEvent]);

  const relatedTasks = useMemo(() => tasks.slice(0, 4), [tasks]);
  const timeline = useMemo(() => (selectedEvent ? buildEventTimeline(selectedEvent) : []), [selectedEvent]);

  return (
    <div className="event-shell">
      <section className="panel" id="event-summary">
        <div className="section-title">
          <h2>事件工作台</h2>
          <span>{loading ? "加载中..." : "聚合告警、AI、知识与控制面动作"}</span>
        </div>
        {error ? <div className="event-banner">接口异常：{error}</div> : null}
        <div className="event-master-grid">
          <div className="event-master-card">
            <div className="event-master-label">当前事件</div>
            <div className="event-master-title">{selectedEvent?.message || "暂无事件"}</div>
            <div className="event-master-meta">
              <span className="badge ghost">规则 {selectedEvent?.rule || "--"}</span>
              <span className="badge ghost">级别 {selectedEvent?.level || "--"}</span>
              <span className="badge ghost">状态 {selectedEvent?.status || "--"}</span>
            </div>
          </div>
          <div className="event-master-card compact">
            <div className="event-master-label">建议路径</div>
            <div className="event-master-value">发现 → 诊断 → 处置 → 复盘</div>
            <div className="event-master-hint">当前页用于承接单事件处置工作流。</div>
          </div>
        </div>
      </section>

      <div className="event-grid">
        <section className="panel" id="event-timeline">
          <div className="section-title">
            <h2>事件时间线</h2>
            <span>最近告警与流转痕迹</span>
          </div>
          <div className="event-list">
            {(dashboard.decisions ?? []).slice(0, 6).map((item) => (
              <button
                className={`event-list-item ${item.id === selectedEvent?.id ? "active" : ""}`}
                key={item.id}
                type="button"
                onClick={() => setSelectedEventId(item.id)}
              >
                <div className="event-list-time">{item.time}</div>
                <div className="event-list-main">
                  <strong>{item.message}</strong>
                  <span>{item.rule}</span>
                </div>
              </button>
            ))}
          </div>
          <div className="event-timeline-detail">
            {timeline.length ? (
              timeline.map((item, index) => (
                <div className="event-timeline-item" key={`${item.title}-${index}`}>
                  <span className="event-timeline-index">{index + 1}</span>
                  <div>
                    <strong>{item.title}</strong>
                    <div className="row-sub">{item.detail}</div>
                  </div>
                </div>
              ))
            ) : (
              <div className="empty-state">暂无事件时间线</div>
            )}
          </div>
        </section>

        <section className="panel" id="event-analysis">
          <div className="section-title">
            <h2>分析区</h2>
            <span>AI / 根因 / 日志摘要</span>
          </div>
          {selectedEvent ? (
            <div className="event-analysis-grid">
              <div className="event-analysis-card">
                <div className="event-analysis-label">告警摘要</div>
                <div className="event-analysis-main">{selectedEvent.message}</div>
                <div className="row-sub">文件：{selectedEvent.file || "--"}</div>
              </div>
              <div className="event-analysis-card">
                <div className="event-analysis-label">决策解释</div>
                <div className="event-analysis-main">
                  {selectedEvent.explain
                    ? `${selectedEvent.explain.decisionKind} · notify=${selectedEvent.explain.notify}`
                    : "当前没有解释字段"}
                </div>
                <div className="row-sub">{selectedEvent.reason || "无额外说明"}</div>
              </div>
              <div className="event-analysis-card">
                <div className="event-analysis-label">AI / 关联分析</div>
                <div className="event-analysis-main">{selectedEvent.analysis || "当前事件暂无 AI 分析结果，可由告警详情或日志分析补充。"}</div>
              </div>
            </div>
          ) : (
            <div className="empty-state">暂无可分析事件</div>
          )}
        </section>
      </div>

      <div className="event-grid">
        <section className="panel" id="event-actions">
          <div className="section-title">
            <h2>动作与知识</h2>
            <span>推荐 SOP / 控制面任务 / 审计入口</span>
          </div>
          <div className="event-action-grid">
            <div className="event-action-card">
              <div className="event-analysis-label">建议动作</div>
              <div className="event-action-buttons">
                <button className="btn secondary" type="button" onClick={() => onViewChange("control")}>
                  打开控制面
                </button>
                <button className="btn secondary" type="button" onClick={() => onViewChange("knowledge")}>
                  打开知识库
                </button>
                <button className="btn secondary" type="button" onClick={() => onViewChange("system")}>
                  查看系统资源
                </button>
              </div>
            </div>

            <div className="event-action-card">
              <div className="event-analysis-label">知识推荐</div>
              {recommendations.length ? (
                <div className="event-reco-list">
                  {recommendations.map((item) => (
                    <div className="event-reco-item" key={item.id}>
                      <strong>{item.title}</strong>
                      <span>{item.summary || "暂无摘要"}</span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="row-sub">当前未命中知识推荐，后续可直接从此页发起补录。</div>
              )}
            </div>

            <div className="event-action-card">
              <div className="event-analysis-label">最近控制面任务</div>
              {relatedTasks.length ? (
                <div className="event-reco-list">
                  {relatedTasks.map((task) => (
                    <div className="event-reco-item" key={task.id}>
                      <strong>{task.type}</strong>
                      <span>{task.target} · {task.status}</span>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="row-sub">当前没有关联控制面任务。</div>
              )}
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
