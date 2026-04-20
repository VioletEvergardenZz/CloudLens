/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于事件工作台原型页 将证据、判断、动作和复盘素材收口到单页 */

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
      impact: "下单链路成功率下降，风控决策响应时间拉长",
      route: "https://risk.example.com/api",
      health: "127.0.0.1:8081/actuator/health",
      rollback: "优先扩容连接池并核查最近变更，再考虑回滚近 30 分钟数据库发布",
      evidence: [
        "连接池耗尽与请求超时应同时核对，避免只看单条错误日志。",
        "先确认慢 SQL 或事务堆积，再决定是否需要回滚应用版本。",
        "把数据库、线程池和调用方重试放在同一张证据板里观察。",
      ],
      nextActions: [
        "查看系统详情，确认 prod 环境服务和健康规则是否完整。",
        "从执行中心下发连接池扩容或重启 worker 的标准动作。",
        "补录本次根因与恢复动作，生成 SOP 草稿。",
      ],
    };
  }

  if (keyword.includes("线程池") || keyword.includes("rejectedexecution")) {
    return {
      system: "营销内容中心",
      service: "content-worker / scheduler",
      owner: "内容平台组",
      impact: "异步任务处理堆积，延迟扩散到内容发布链路",
      route: "https://mch-test.example.com/",
      health: "worker metrics / queue backlog",
      rollback: "优先降并发或暂停高成本任务，再评估最近任务模板发布是否需要回滚",
      evidence: [
        "线程池拒绝通常不是根因，更多是上游流量、慢依赖或任务设计问题的结果。",
        "重点核查 backlog、平均执行时长和最近任务模板变更。",
        "如果没有 SOP，至少把临时止损动作写入复盘草稿。",
      ],
      nextActions: [
        "先收敛任务队列和 backlog，再决定是否重启 worker。",
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
      impact: "上传与回放材料落盘变慢，AI 分析与回放链路同时受影响",
      route: "https://gwf.example.com/api",
      health: "127.0.0.1:8082/api/health",
      rollback: "优先释放磁盘压力和回收失败文件，再评估是否回退最近接入配置",
      evidence: [
        "磁盘写入异常需要同时看 backlog、上传失败样本和系统资源页的磁盘分区。",
        "如果 AI 或回放链路降级，不要只在事件页结案，要回写到接入和知识面。",
        "排障前先确认是不是单机磁盘热点，而不是业务流量骤增。",
      ],
      nextActions: [
        "联动接入工作台核对失败文件和重试策略。",
        "联动系统资源页检查 /data 与 /var/lib/docker 的空间风险。",
        "把这次事件沉淀成“上传失败 / 磁盘热点”的 SOP 模板。",
      ],
    };
  }

  return {
    system: "统一运维平台",
    service: "公共控制面",
    owner: "平台稳定性组",
    impact: "当前事件影响面未完全识别，需要先补系统和服务上下文",
    route: "--",
    health: "--",
    rollback: "优先从系统详情补齐上下文，再决定是否执行动作或升级",
    evidence: [
      "没有系统、服务、入口和健康规则的上下文时，任何动作都容易误判。",
      "先确认责任边界，再决定要不要升级或指派任务。",
    ],
    nextActions: [
      "先进入系统详情补齐上下文。",
      "再根据事件类型选择执行中心或知识中心。",
    ],
  };
};

const levelLabel = (decision: AlertDecision | null) => {
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
  const incidentProfile = useMemo(() => inferIncidentProfile(selectedEvent), [selectedEvent]);

  const diagnosisCards = useMemo(
    () => [
      {
        label: "告警摘要",
        value: selectedEvent?.message || "暂无事件",
        hint: `规则 ${selectedEvent?.rule || "--"} · 文件 ${selectedEvent?.file || "--"}`,
      },
      {
        label: "当前判断",
        value:
          selectedEvent?.analysis ||
          "当前没有 AI 结果时，也要把事件、系统、动作和知识放在一页内判断，避免跳页丢上下文。",
        hint: selectedEvent?.reason || "还没有人工判断记录，建议先补一句当前判断。",
      },
      {
        label: "影响面",
        value: incidentProfile.impact,
        hint: `${incidentProfile.system} · ${incidentProfile.service}`,
      },
    ],
    [incidentProfile.impact, incidentProfile.service, incidentProfile.system, selectedEvent?.analysis, selectedEvent?.file, selectedEvent?.message, selectedEvent?.reason, selectedEvent?.rule]
  );

  const postmortemDraft = useMemo(
    () => [
      `事件级别：${levelLabel(selectedEvent)}，当前状态 ${selectedEvent?.status || "--"}。`,
      `影响对象：${incidentProfile.system} / ${incidentProfile.service}。`,
      `临时动作：${incidentProfile.nextActions[0] ?? "待补充动作"}`,
      `回滚或恢复判断：${incidentProfile.rollback}`,
    ],
    [incidentProfile.nextActions, incidentProfile.rollback, incidentProfile.service, incidentProfile.system, selectedEvent]
  );

  return (
    <div className="event-shell">
      <section className="panel event-command" id="event-summary">
        <div className="section-title">
          <div>
            <h2>事件工作台</h2>
            <span>{loading ? "加载中..." : "把证据、判断、动作与复盘收口到单页"}</span>
          </div>
          <span className="badge ghost">单事件收口页</span>
        </div>
        {error ? <div className="event-banner">接口异常：{error}</div> : null}
        <div className="event-command-grid">
          <div className="event-command-card featured">
            <div className="event-command-label">当前事件</div>
            <div className="event-command-title">{selectedEvent?.message || "暂无事件"}</div>
            <div className="event-command-meta">
              <span className="badge ghost">{levelLabel(selectedEvent)}</span>
              <span className="badge ghost">{selectedEvent?.status || "--"}</span>
              <span className="badge ghost">{incidentProfile.owner}</span>
            </div>
          </div>
          <div className="event-command-card">
            <div className="event-command-label">系统上下文</div>
            <div className="event-command-main">{incidentProfile.system}</div>
            <div className="event-command-sub">{incidentProfile.service}</div>
          </div>
          <div className="event-command-card">
            <div className="event-command-label">恢复路径</div>
            <div className="event-command-main">诊断 → 动作 → 回写</div>
            <div className="event-command-sub">不要把动作散落到多个模块后再回来补记录。</div>
          </div>
        </div>
      </section>

      <div className="event-workbench-grid">
        <section className="panel event-column" id="event-timeline">
          <div className="section-title">
            <h2>事件队列</h2>
            <span>选中一条事件，右侧整页跟着切换</span>
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

        <section className="panel event-column" id="event-analysis">
          <div className="section-title">
            <h2>诊断面</h2>
            <span>把“为什么判断成这样”留在工作台中间</span>
          </div>
          <div className="event-diagnosis-grid">
            {diagnosisCards.map((item) => (
              <div className="event-analysis-card" key={item.label}>
                <div className="event-analysis-label">{item.label}</div>
                <div className="event-analysis-main">{item.value}</div>
                <div className="row-sub">{item.hint}</div>
              </div>
            ))}
          </div>

          <div className="event-evidence-list">
            {incidentProfile.evidence.map((item, index) => (
              <div className="event-evidence-item" key={`${item}-${index}`}>
                <span className="event-evidence-index">{index + 1}</span>
                <span>{item}</span>
              </div>
            ))}
          </div>
        </section>

        <section className="panel event-column" id="event-actions">
          <div className="section-title">
            <h2>动作面</h2>
            <span>参考 GoAlert / Rundeck，让 owner、SOP 和任务联动站到右侧</span>
          </div>
          <div className="event-action-card">
            <div className="event-analysis-label">当前负责人</div>
            <div className="event-action-main">{incidentProfile.owner}</div>
            <div className="row-sub">健康检查 {incidentProfile.health}</div>
            <div className="event-action-buttons">
              <button className="btn secondary" type="button" onClick={() => onViewChange("registryCatalog")}>
                查看系统详情
              </button>
              <button className="btn secondary" type="button" onClick={() => onViewChange("control")}>
                打开执行中心
              </button>
              <button className="btn secondary" type="button" onClick={() => onViewChange("knowledge")}>
                打开知识中心
              </button>
            </div>
          </div>

          <div className="event-action-card">
            <div className="event-analysis-label">建议动作</div>
            <div className="event-reco-list">
              {incidentProfile.nextActions.map((item, index) => (
                <div className="event-reco-item" key={`${item}-${index}`}>
                  <strong>动作 {index + 1}</strong>
                  <span>{item}</span>
                </div>
              ))}
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
              <div className="row-sub">当前未命中知识推荐，建议在事件结束后直接回写 SOP 草稿。</div>
            )}
          </div>

          <div className="event-action-card">
            <div className="event-analysis-label">最近控制面任务</div>
            {relatedTasks.length ? (
              <div className="event-reco-list">
                {relatedTasks.map((task) => (
                  <div className="event-reco-item" key={task.id}>
                    <strong>{task.type}</strong>
                    <span>
                      {task.target} · {task.status}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <div className="row-sub">当前没有关联控制面任务。</div>
            )}
          </div>
        </section>
      </div>

      <div className="event-bottom-grid">
        <section className="panel">
          <div className="section-title">
            <h2>系统上下文</h2>
            <span>把系统、服务、入口、健康规则放回同一页</span>
          </div>
          <div className="event-context-grid">
            <div className="event-context-card">
              <div className="event-analysis-label">系统</div>
              <strong>{incidentProfile.system}</strong>
              <span>{incidentProfile.service}</span>
            </div>
            <div className="event-context-card">
              <div className="event-analysis-label">入口</div>
              <strong>{incidentProfile.route}</strong>
              <span>建议从系统详情继续核对环境、路由和责任人。</span>
            </div>
            <div className="event-context-card">
              <div className="event-analysis-label">恢复判断</div>
              <strong>{incidentProfile.rollback}</strong>
              <span>这块应该直接成为复盘草稿的一部分。</span>
            </div>
          </div>
        </section>

        <section className="panel">
          <div className="section-title">
            <h2>复盘草稿</h2>
            <span>事件页不只是结束动作，也要给知识沉淀留出口</span>
          </div>
          <div className="event-postmortem-list">
            {postmortemDraft.map((item, index) => (
              <div className="event-postmortem-item" key={`${item}-${index}`}>
                <span className="event-postmortem-index">{index + 1}</span>
                <span>{item}</span>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
