/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于事件详情页原型，采用“队列 + 详情 + 动作”的标准运维页面结构。 */

import { useEffect, useMemo, useState } from "react";
import "./EventWorkbench.css";
import { alertDashboard as alertDashboardMock } from "./mockData";
import type { AlertDashboard, AlertDecision, ConsoleView, ControlTask, KnowledgeArticle } from "./types";
import { USE_MOCK, fetchAlertDashboard, fetchControlTasks, fetchKBRecommendations } from "./console/dashboardApi";

type EventWorkbenchProps = {
  onViewChange: (view: ConsoleView) => void;
};

type IncidentProfile = {
  system: string;
  service: string;
  owner: string;
  impact: string;
  route: string;
  health: string;
  rollback: string;
  evidence: string[];
  nextActions: string[];
};

type EventTab = "summary" | "evidence" | "postmortem";

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
    items.push({ title: "当前判断", detail: decision.reason });
  }

  if (decision.knowledgeTrace?.linkedAt) {
    items.push({ title: "知识关联", detail: decision.knowledgeTrace.linkedAt });
  }

  return items;
};

const inferIncidentProfile = (decision: AlertDecision | null): IncidentProfile => {
  if (!decision) {
    return {
      system: "--",
      service: "--",
      owner: "--",
      impact: "--",
      route: "--",
      health: "--",
      rollback: "--",
      evidence: [],
      nextActions: [],
    };
  }

  const keyword = `${decision.rule} ${decision.message}`.toLowerCase();

  if (keyword.includes("连接") || keyword.includes("hikari") || keyword.includes("order")) {
    return {
      system: "订单风控中心",
      service: "risk-api / order-writer",
      owner: "交易稳定性组",
      impact: "下单链路成功率下降，风控判定响应时间拉长。",
      route: "https://risk.example.com/api",
      health: "127.0.0.1:8081/actuator/health",
      rollback: "优先扩容连接池并核查最近变更，再评估是否回滚近 30 分钟的数据库或应用发布。",
      evidence: [
        "连接池耗尽与请求超时应同时核对，避免只看单条错误日志。",
        "优先确认慢 SQL、事务堆积和上游重试，而不是先回滚全部应用。",
        "数据库、线程池和调用方重试应放在同一张证据板里观察。",
      ],
      nextActions: [
        "先进入系统台账确认 prod 服务、健康检查和负责人边界。",
        "从执行中心下发连接池扩容或 worker 限流动作。",
        "补齐本次根因与恢复动作，沉淀成标准 SOP。",
      ],
    };
  }

  if (keyword.includes("线程池") || keyword.includes("rejectedexecution")) {
    return {
      system: "营销内容中心",
      service: "content-worker / scheduler",
      owner: "内容平台组",
      impact: "异步任务处理堆积，延迟扩散到内容发布和同步链路。",
      route: "https://mch-test.example.com/",
      health: "worker metrics / queue backlog",
      rollback: "先降并发或暂停高成本任务，再评估最近任务模板发布是否需要回滚。",
      evidence: [
        "线程池拒绝通常不是根因，更常见是上游流量、慢依赖或任务设计问题。",
        "重点核查 backlog、平均执行时长和最近任务模板变更。",
        "如果当前没有 SOP，至少把临时止损动作写入复盘草稿。",
      ],
      nextActions: [
        "先收敛队列 backlog，再决定是否重启 worker。",
        "检查接入链路最近上传或回放是否触发了任务洪峰。",
        "同步知识库，补齐线程池告警的标准处置步骤。",
      ],
    };
  }

  if (keyword.includes("磁盘") || keyword.includes("io")) {
    return {
      system: "文件接入网关",
      service: "gwf-api / upload-worker",
      owner: "平台运维组",
      impact: "上传与回放材料落盘变慢，AI 分析和回放链路同时受影响。",
      route: "https://gwf.example.com/api",
      health: "127.0.0.1:8082/api/health",
      rollback: "优先释放磁盘压力并清理失败文件，再评估是否回退最近接入配置。",
      evidence: [
        "磁盘写入异常需要同时看 backlog、失败样本和系统资源页的磁盘分区。",
        "如果 AI 或回放链路降级，不能只在事件页结案，应回写到接入与知识面。",
        "排障前先确认是不是单机磁盘热点，而不是业务流量激增。",
      ],
      nextActions: [
        "联动接入工作台核对失败文件和重试策略。",
        "联动主机资源页检查 /data 与容器卷的空间风险。",
        "把这次事件沉淀成“上传失败 / 磁盘热点”SOP 模板。",
      ],
    };
  }

  return {
    system: "统一运维平台",
    service: "公共控制面",
    owner: "平台稳定性组",
    impact: "当前影响面尚未完全识别，需要先补齐系统与服务上下文。",
    route: "--",
    health: "--",
    rollback: "先从系统台账补齐上下文，再决定是执行动作还是升级。",
    evidence: [
      "没有系统、服务、入口和健康规则的上下文时，任何动作都容易误判。",
      "先确认责任边界，再决定是否升级或指派任务。",
    ],
    nextActions: ["进入系统台账补齐上下文。", "再根据事件类型选择执行中心或知识中心。"],
  };
};

const resolveLevelLabel = (decision: AlertDecision | null) => {
  if (!decision) return "--";
  if (decision.level === "fatal") return "P1";
  if (decision.level === "system") return "P2";
  if (decision.level === "business") return "P3";
  return "记录";
};

export function EventWorkbench({ onViewChange }: EventWorkbenchProps) {
  const [dashboard, setDashboard] = useState<AlertDashboard>(USE_MOCK ? alertDashboardMock : emptyAlertDashboard);
  const [tasks, setTasks] = useState<ControlTask[]>([]);
  const [recommendations, setRecommendations] = useState<KnowledgeArticle[]>([]);
  const [selectedEventId, setSelectedEventId] = useState<string>(USE_MOCK ? alertDashboardMock.decisions[0]?.id ?? "" : "");
  const [detailTab, setDetailTab] = useState<EventTab>("summary");
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

  const incidentProfile = useMemo(() => inferIncidentProfile(selectedEvent), [selectedEvent]);
  const timeline = useMemo(() => (selectedEvent ? buildEventTimeline(selectedEvent) : []), [selectedEvent]);
  const relatedTasks = useMemo(() => tasks.slice(0, 5), [tasks]);

  const postmortemDraft = useMemo(
    () => [
      `事件级别：${resolveLevelLabel(selectedEvent)}，当前状态 ${selectedEvent?.status || "--"}。`,
      `影响对象：${incidentProfile.system} / ${incidentProfile.service}。`,
      `临时动作：${incidentProfile.nextActions[0] ?? "待补充动作"}`,
      `恢复或回滚判断：${incidentProfile.rollback}`,
    ],
    [incidentProfile.nextActions, incidentProfile.rollback, incidentProfile.service, incidentProfile.system, selectedEvent]
  );

  const detailTabs: Array<{ id: EventTab; label: string; desc: string }> = [
    { id: "summary", label: "概览", desc: "系统、影响和恢复判断" },
    { id: "evidence", label: "证据", desc: "关键证据和知识联动" },
    { id: "postmortem", label: "复盘", desc: "复盘草稿和沉淀内容" },
  ];

  return (
    <div className="event-shell">
      <section className="panel event-header" id="event-header">
        <div className="event-header-top">
          <div className="event-header-copy">
            <p className="eyebrow">事件详情</p>
            <h2>{selectedEvent?.message || "暂无事件"}</h2>
            <p>事件详情页应该服务于单条事件的诊断、动作和复盘，而不是继续堆积概念卡片。</p>
          </div>

          <div className="event-header-actions">
            <button className="btn secondary" type="button" onClick={() => onViewChange("alert")}>
              返回事件中心
            </button>
            <button className="btn" type="button" onClick={() => onViewChange("registryCatalog")}>
              查看系统台账
            </button>
          </div>
        </div>

        {error ? <div className="event-banner">接口异常：{error}</div> : null}

        <div className="event-kpi-grid">
          <div className="event-kpi-card">
            <span>事件级别</span>
            <strong>{resolveLevelLabel(selectedEvent)}</strong>
            <small>{selectedEvent?.rule || "--"}</small>
          </div>
          <div className="event-kpi-card">
            <span>所属系统</span>
            <strong>{incidentProfile.system}</strong>
            <small>{incidentProfile.service}</small>
          </div>
          <div className="event-kpi-card">
            <span>责任团队</span>
            <strong>{incidentProfile.owner}</strong>
            <small>{selectedEvent?.status || "--"}</small>
          </div>
          <div className="event-kpi-card">
            <span>最近刷新</span>
            <strong>{loading ? "刷新中…" : "已加载"}</strong>
            <small>{selectedEvent?.time || "--"}</small>
          </div>
        </div>
      </section>

      <div className="event-main-grid">
        <section className="panel" id="event-context">
          <div className="section-title">
            <div>
              <h2>事件队列</h2>
              <span>左侧保留最近事件切换，右侧固定展示当前事件的详情</span>
            </div>
          </div>

          {dashboard.decisions.length ? (
            <div className="table-wrap">
              <table className="table event-queue-table">
                <thead>
                  <tr>
                    <th>级别</th>
                    <th>事件</th>
                    <th>规则</th>
                    <th>时间</th>
                  </tr>
                </thead>
                <tbody>
                  {dashboard.decisions.slice(0, 8).map((item) => (
                    <tr
                      key={item.id}
                      className={item.id === selectedEvent?.id ? "event-row-selected" : ""}
                      onClick={() => setSelectedEventId(item.id)}
                    >
                      <td>
                        <span
                          className={`pill ${
                            item.level === "fatal" ? "danger" : item.level === "system" ? "warning" : "info"
                          }`}
                        >
                          {resolveLevelLabel(item)}
                        </span>
                      </td>
                      <td>
                        <div className="event-row-main">
                          <div className="row-title">{item.message}</div>
                          <div className="row-sub">{item.file || "--"}</div>
                        </div>
                      </td>
                      <td>{item.rule}</td>
                      <td>{item.time}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="empty-state">暂无事件队列。</div>
          )}

          <div className="event-timeline-block">
            <div className="section-title">
              <div>
                <h2>事件时间线</h2>
                <span>把关键节点留在详情页里，不再散落到多个模块</span>
              </div>
            </div>
            <div className="event-timeline-list">
              {timeline.length ? (
                timeline.map((item, index) => (
                  <div className="event-timeline-item" key={`${item.title}-${index}`}>
                    <span className="event-index">{index + 1}</span>
                    <div>
                      <strong>{item.title}</strong>
                      <div className="row-sub">{item.detail}</div>
                    </div>
                  </div>
                ))
              ) : (
                <div className="empty-state">暂无时间线数据。</div>
              )}
            </div>
          </div>
        </section>

        <section className="panel" id="event-analysis">
          <div className="section-title">
            <div>
              <h2>事件分析</h2>
              <span>用页内标签切换诊断内容，更接近成熟运维产品的事件详情页</span>
            </div>
          </div>

          <div className="event-tab-strip">
            {detailTabs.map((tab) => (
              <button
                key={tab.id}
                className={`event-tab-button ${detailTab === tab.id ? "active" : ""}`}
                type="button"
                onClick={() => setDetailTab(tab.id)}
              >
                <strong>{tab.label}</strong>
                <span>{tab.desc}</span>
              </button>
            ))}
          </div>

          {detailTab === "summary" ? (
            <div className="event-summary-grid">
              <div className="event-detail-card">
                <span>影响面</span>
                <strong>{incidentProfile.impact}</strong>
                <small>系统 {incidentProfile.system} · 服务 {incidentProfile.service}</small>
              </div>
              <div className="event-detail-card">
                <span>访问入口</span>
                <strong>{incidentProfile.route}</strong>
                <small>健康检查 {incidentProfile.health}</small>
              </div>
              <div className="event-detail-card full">
                <span>恢复判断</span>
                <strong>{incidentProfile.rollback}</strong>
                <small>先补齐上下文，再决定扩容、重启还是回滚。</small>
              </div>
            </div>
          ) : null}

          {detailTab === "evidence" ? (
            <div className="event-evidence-stack">
              <div className="event-list-block">
                <h3>关键证据</h3>
                <div className="event-evidence-list">
                  {incidentProfile.evidence.map((item, index) => (
                    <div className="event-evidence-item" key={`${item}-${index}`}>
                      <span className="event-index">{index + 1}</span>
                      <span>{item}</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className="event-list-block">
                <h3>知识联动</h3>
                {recommendations.length ? (
                  <div className="event-recommend-list">
                    {recommendations.map((item) => (
                      <div className="event-recommend-item" key={item.id}>
                        <strong>{item.title}</strong>
                        <span>{item.summary || "暂无摘要"}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="empty-state">当前没有命中知识推荐，建议在结案后回写 SOP。</div>
                )}
              </div>
            </div>
          ) : null}

          {detailTab === "postmortem" ? (
            <div className="event-postmortem-list">
              {postmortemDraft.map((item, index) => (
                <div className="event-postmortem-item" key={`${item}-${index}`}>
                  <span className="event-index">{index + 1}</span>
                  <span>{item}</span>
                </div>
              ))}
            </div>
          ) : null}
        </section>
      </div>

      <section className="panel" id="event-actions">
        <div className="section-title">
          <div>
            <h2>动作与执行</h2>
            <span>把责任人、执行入口、推荐动作和关联任务收拢在一起</span>
          </div>
        </div>

        <div className="event-action-grid">
          <div className="event-action-card">
            <span>当前负责人</span>
            <strong>{incidentProfile.owner}</strong>
            <small>{selectedEvent?.status || "--"} · {selectedEvent?.time || "--"}</small>
            <div className="event-button-row">
              <button className="btn secondary" type="button" onClick={() => onViewChange("registryCatalog")}>
                系统详情
              </button>
              <button className="btn secondary" type="button" onClick={() => onViewChange("control")}>
                执行中心
              </button>
              <button className="btn secondary" type="button" onClick={() => onViewChange("knowledge")}>
                知识中心
              </button>
            </div>
          </div>

          <div className="event-action-card">
            <span>建议动作</span>
            <div className="event-recommend-list">
              {incidentProfile.nextActions.map((item, index) => (
                <div className="event-recommend-item" key={`${item}-${index}`}>
                  <strong>动作 {index + 1}</strong>
                  <span>{item}</span>
                </div>
              ))}
            </div>
          </div>

          <div className="event-action-card">
            <span>关联任务</span>
            {relatedTasks.length ? (
              <div className="event-recommend-list">
                {relatedTasks.map((task) => (
                  <div className="event-recommend-item" key={task.id}>
                    <strong>{task.type}</strong>
                    <span>
                      {task.target} · {task.status}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <div className="empty-state">当前没有关联执行任务。</div>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
