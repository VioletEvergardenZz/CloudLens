/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于事件中心页原型，把事件分诊、规则配置和运行设置整理成更像成熟后台的结构。 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./Alert.css";
import { alertConfigSnapshot, alertDashboard, alertRulesSnapshot } from "./mockData";
import type {
  AiLogSummary,
  AlertConfigResponse,
  AlertConfigSnapshot,
  AlertDashboard,
  AlertDecision,
  AlertDecisionStatus,
  AlertKnowledgeTrace,
  AlertLevel,
  AlertResponse,
  AlertRulesResponse,
  AlertRulesSaveResponse,
  AlertRuleset,
  KnowledgeArticle,
} from "./types";
import { buildApiHeaders, fetchKBRecommendations, postAiLogSummary } from "./console/dashboardApi";

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
const USE_MOCK = ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";
const POLL_MS = 3000;
const DECISIONS_PAGE_SIZE = 8;

type AlertConsoleProps = {
  embedded?: boolean;
};

type MatchCaseMode = "inherit" | "case" | "nocase";
type NotifyMode = "inherit" | "send" | "record";
type AlertPanel = "alerts" | "rules" | "config";

type RuleDraft = {
  id: string;
  title: string;
  level: AlertLevel;
  keywordsText: string;
  excludesText: string;
  suppressWindow: string;
  matchCaseMode: MatchCaseMode;
  notifyMode: NotifyMode;
};

type RulesetDraft = {
  version: number;
  defaults: {
    suppressWindow: string;
    matchCase: boolean;
  };
  escalation: {
    enabled: boolean;
    level: AlertLevel;
    window: string;
    threshold: number;
    suppressWindow: string;
    ruleId: string;
    title: string;
    message: string;
  };
  rules: RuleDraft[];
};

const emptyDashboard: AlertDashboard = {
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

const emptyAlertConfig: AlertConfigSnapshot = {
  enabled: false,
  suppressEnabled: true,
  rulesFile: "",
  logPaths: "",
  pollInterval: "2s",
  startFromEnd: true,
};

const LEVEL_LABELS: Record<AlertLevel, string> = {
  ignore: "忽略",
  business: "P3",
  system: "P2",
  fatal: "P1",
};

const STATUS_LABELS: Record<AlertDecisionStatus, string> = {
  sent: "处理中",
  suppressed: "已抑制",
  recorded: "仅记录",
};

const splitTokens = (value: string) =>
  value
    .split(/[\n,;，；\t]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);

const joinTokens = (values: string[]) => values.join("\n");

const normalizeLevel = (value: string | undefined): AlertLevel => {
  if (value === "fatal" || value === "system" || value === "business" || value === "ignore") {
    return value;
  }
  return "business";
};

const resolveRiskTone = (risk: string) => {
  if (risk.includes("严重")) return "critical";
  if (risk.includes("高")) return "high";
  if (risk.includes("中")) return "medium";
  if (risk.includes("低")) return "low";
  return "muted";
};

const resolveLevelTone = (level: AlertLevel) => {
  if (level === "fatal") return "danger";
  if (level === "system") return "warning";
  if (level === "business") return "info";
  return "ghost";
};

const resolveSuppressedByLabel = (suppressedBy?: string) => {
  if (suppressedBy === "rule_window") return "规则抑制窗口";
  if (suppressedBy === "escalation_window") return "升级抑制窗口";
  return "";
};

const buildDecisionExplainText = (decision: AlertDecision) => {
  const explain = decision.explain;
  if (!explain) return "当前没有补充说明。";

  const parts: string[] = [];
  if (explain.decisionKind === "escalation") {
    const threshold = typeof explain.escalationThreshold === "number" ? explain.escalationThreshold : 0;
    const count = typeof explain.escalationCount === "number" ? explain.escalationCount : 0;
    const window = explain.escalationWindow?.trim();
    let escalationText = "来源：异常升级";
    if (threshold > 0 && count > 0) {
      escalationText += `（${count}/${threshold}）`;
    }
    if (window) {
      escalationText += ` ${window}`;
    }
    parts.push(escalationText);
  } else {
    parts.push("来源：规则匹配");
  }

  parts.push(explain.notify ? "通知：发送" : "通知：仅记录");
  if (explain.suppressionEnabled) {
    const suppressWindow = explain.suppressWindow?.trim();
    parts.push(suppressWindow ? `抑制：开启（${suppressWindow}）` : "抑制：开启");
  } else {
    parts.push("抑制：关闭");
  }

  if (decision.status === "suppressed") {
    const suppressedBy = resolveSuppressedByLabel(explain.suppressedBy);
    if (suppressedBy) {
      parts.push(`抑制来源：${suppressedBy}`);
    }
  }

  return parts.join(" · ");
};

const formatAiSummary = (analysis: AiLogSummary) => {
  const parts: string[] = [];
  if (analysis.summary?.trim()) {
    parts.push(`摘要：${analysis.summary.trim()}`);
  }
  if (analysis.causes?.length) {
    parts.push(`可能原因：${analysis.causes.join("；")}`);
  }
  if (analysis.suggestions?.length) {
    parts.push(`建议动作：${analysis.suggestions.join("；")}`);
  }
  return parts.join("\n");
};

const createDefaultRuleset = (): RulesetDraft => ({
  version: 1,
  defaults: {
    suppressWindow: "5m",
    matchCase: false,
  },
  escalation: {
    enabled: true,
    level: "fatal",
    window: "5m",
    threshold: 20,
    suppressWindow: "5m",
    ruleId: "system_spike",
    title: "系统异常激增",
    message: "系统异常在 5 分钟内达到 20 次",
  },
  rules: [],
});

const createRuleDraft = (seed: Partial<RuleDraft> = {}): RuleDraft => ({
  id: seed.id ?? "",
  title: seed.title ?? "",
  level: seed.level ?? "business",
  keywordsText: seed.keywordsText ?? "",
  excludesText: seed.excludesText ?? "",
  suppressWindow: seed.suppressWindow ?? "",
  matchCaseMode: seed.matchCaseMode ?? "inherit",
  notifyMode: seed.notifyMode ?? "inherit",
});

const buildRulesetDraft = (ruleset: AlertRuleset | null | undefined): RulesetDraft => {
  const fallback = createDefaultRuleset();
  if (!ruleset) return fallback;

  const defaults = ruleset.defaults ?? {};
  const escalation = ruleset.escalation ?? {};
  const threshold = typeof escalation.threshold === "number" ? escalation.threshold : fallback.escalation.threshold;
  const enabled = escalation.enabled ?? threshold > 0;

  return {
    version: ruleset.version ?? fallback.version,
    defaults: {
      suppressWindow: defaults.suppress_window?.trim() || fallback.defaults.suppressWindow,
      matchCase: defaults.match_case ?? fallback.defaults.matchCase,
    },
    escalation: {
      enabled,
      level: normalizeLevel(escalation.level),
      window: escalation.window?.trim() || fallback.escalation.window,
      threshold,
      suppressWindow: escalation.suppress_window?.trim() || fallback.escalation.suppressWindow,
      ruleId: escalation.rule_id?.trim() || fallback.escalation.ruleId,
      title: escalation.title?.trim() || fallback.escalation.title,
      message: escalation.message?.trim() || fallback.escalation.message,
    },
    rules: (ruleset.rules ?? []).map((rule) => {
      const matchCaseMode: MatchCaseMode =
        rule.match_case === undefined ? "inherit" : rule.match_case ? "case" : "nocase";
      const notifyMode: NotifyMode = rule.notify === undefined ? "inherit" : rule.notify ? "send" : "record";
      return createRuleDraft({
        id: rule.id ?? "",
        title: rule.title ?? "",
        level: normalizeLevel(rule.level),
        keywordsText: joinTokens(rule.keywords ?? []),
        excludesText: joinTokens(rule.excludes ?? []),
        suppressWindow: rule.suppress_window ?? "",
        matchCaseMode,
        notifyMode,
      });
    }),
  };
};

const toApiRuleset = (draft: RulesetDraft): AlertRuleset => ({
  version: draft.version,
  defaults: {
    suppress_window: draft.defaults.suppressWindow.trim(),
    match_case: draft.defaults.matchCase,
  },
  escalation: {
    enabled: draft.escalation.enabled,
    level: draft.escalation.level,
    window: draft.escalation.window.trim(),
    threshold: Math.max(0, Math.floor(draft.escalation.threshold)),
    suppress_window: draft.escalation.suppressWindow.trim(),
    rule_id: draft.escalation.ruleId.trim(),
    title: draft.escalation.title.trim(),
    message: draft.escalation.message.trim(),
  },
  rules: draft.rules.map((rule) => {
    const keywords = splitTokens(rule.keywordsText);
    const excludes = splitTokens(rule.excludesText);
    const suppressWindow = rule.suppressWindow.trim();
    const matchCase = rule.matchCaseMode === "inherit" ? undefined : rule.matchCaseMode === "case";
    const notify =
      rule.level === "ignore" ? undefined : rule.notifyMode === "inherit" ? undefined : rule.notifyMode === "send";
    return {
      id: rule.id.trim(),
      title: rule.title.trim(),
      level: rule.level,
      keywords,
      excludes: excludes.length > 0 ? excludes : undefined,
      suppress_window: suppressWindow || undefined,
      match_case: matchCase,
      notify,
    };
  }),
});

export function AlertConsole({ embedded = false }: AlertConsoleProps) {
  const [dashboard, setDashboard] = useState<AlertDashboard>(USE_MOCK ? alertDashboard : emptyDashboard);
  const [loading, setLoading] = useState(!USE_MOCK);
  const [error, setError] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(true);
  const [lastUpdated, setLastUpdated] = useState(() =>
    USE_MOCK ? new Date().toLocaleTimeString("zh-CN", { hour12: false }) : "--"
  );
  const [alertConfig, setAlertConfig] = useState<AlertConfigSnapshot>(USE_MOCK ? alertConfigSnapshot : emptyAlertConfig);
  const [configLoading, setConfigLoading] = useState(!USE_MOCK);
  const [configSaving, setConfigSaving] = useState(false);
  const [configMessage, setConfigMessage] = useState<string | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);
  const [aiTargetPath, setAiTargetPath] = useState("");
  const [aiQuery, setAiQuery] = useState("error");
  const [aiLoading, setAiLoading] = useState(false);
  const [aiSummary, setAiSummary] = useState<string | null>(null);
  const [aiError, setAiError] = useState<string | null>(null);
  const [ruleset, setRuleset] = useState<RulesetDraft>(() =>
    USE_MOCK ? buildRulesetDraft(alertRulesSnapshot) : createDefaultRuleset()
  );
  const [rulesLoading, setRulesLoading] = useState(!USE_MOCK);
  const [rulesSaving, setRulesSaving] = useState(false);
  const [rulesMessage, setRulesMessage] = useState<string | null>(null);
  const [rulesError, setRulesError] = useState<string | null>(null);
  const [activePanel, setActivePanel] = useState<AlertPanel>("alerts");
  const [query, setQuery] = useState("");
  const [levelFilter, setLevelFilter] = useState<"all" | AlertLevel>("all");
  const [statusFilter, setStatusFilter] = useState<"all" | AlertDecisionStatus>("all");
  const [decisionPage, setDecisionPage] = useState(1);
  const [selectedDecisionID, setSelectedDecisionID] = useState("");
  const [recommendationLoading, setRecommendationLoading] = useState(false);
  const [recommendationError, setRecommendationError] = useState<string | null>(null);
  const [recommendations, setRecommendations] = useState<KnowledgeArticle[]>([]);
  const [recommendationTrace, setRecommendationTrace] = useState<AlertKnowledgeTrace | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<Record<AlertLevel, boolean>>({
    fatal: true,
    system: true,
    business: false,
    ignore: false,
  });
  const [expandedRules, setExpandedRules] = useState<Record<number, boolean>>({});
  const fetchingRef = useRef(false);
  const configFetchingRef = useRef(false);
  const rulesFetchingRef = useRef(false);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  const refreshDashboard = async () => {
    if (USE_MOCK || fetchingRef.current || !aliveRef.current) return;
    fetchingRef.current = true;
    try {
      const resp = await fetch(`${API_BASE}/api/alerts`, { cache: "no-store", headers: buildApiHeaders() });
      if (!resp.ok) {
        throw new Error(`接口异常 ${resp.status}`);
      }
      const payload = (await resp.json()) as AlertResponse;
      if (!payload.ok || !payload.data) {
        if (!aliveRef.current) return;
        setEnabled(false);
        setDashboard(emptyDashboard);
        setError(payload.error ?? "告警未启用");
        return;
      }
      if (!aliveRef.current) return;
      setEnabled(payload.enabled ?? true);
      setDashboard(payload.data);
      setError(null);
      setLastUpdated(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
    } catch (err) {
      if (!aliveRef.current) return;
      setError(err instanceof Error ? err.message : "获取事件失败");
    } finally {
      fetchingRef.current = false;
      if (aliveRef.current) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (USE_MOCK) return;
    void refreshDashboard();
    const timer = window.setInterval(() => void refreshDashboard(), POLL_MS);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (USE_MOCK) return;
    const fetchConfig = async () => {
      if (configFetchingRef.current || !aliveRef.current) return;
      configFetchingRef.current = true;
      setConfigLoading(true);
      setConfigError(null);
      try {
        const resp = await fetch(`${API_BASE}/api/alert-config`, { cache: "no-store", headers: buildApiHeaders() });
        if (!resp.ok) {
          throw new Error(`接口异常 ${resp.status}`);
        }
        const payload = (await resp.json()) as AlertConfigResponse;
        if (payload.config && aliveRef.current) {
          setAlertConfig(payload.config);
        }
      } catch (err) {
        if (!aliveRef.current) return;
        setConfigError(err instanceof Error ? err.message : "获取配置失败");
      } finally {
        configFetchingRef.current = false;
        if (aliveRef.current) {
          setConfigLoading(false);
        }
      }
    };
    void fetchConfig();
  }, []);

  useEffect(() => {
    const candidates = splitTokens(alertConfig.logPaths);
    setAiTargetPath((prev) => (prev.trim() ? prev : candidates[0] ?? ""));
  }, [alertConfig.logPaths]);

  const fetchRules = async () => {
    if (USE_MOCK || rulesFetchingRef.current || !aliveRef.current) return;
    rulesFetchingRef.current = true;
    setRulesLoading(true);
    setRulesError(null);
    try {
      const resp = await fetch(`${API_BASE}/api/alert-rules`, { cache: "no-store", headers: buildApiHeaders() });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(text || `接口异常 ${resp.status}`);
      }
      const payload = (await resp.json()) as AlertRulesResponse;
      if (!payload.ok || !payload.rules) {
        if (!aliveRef.current) return;
        setRulesError(payload.error ?? "获取规则失败");
        return;
      }
      if (!aliveRef.current) return;
      setRuleset(buildRulesetDraft(payload.rules));
      setRulesMessage(null);
    } catch (err) {
      if (!aliveRef.current) return;
      setRulesError(err instanceof Error ? err.message : "获取规则失败");
    } finally {
      rulesFetchingRef.current = false;
      if (aliveRef.current) {
        setRulesLoading(false);
      }
    }
  };

  useEffect(() => {
    if (USE_MOCK) return;
    void fetchRules();
  }, []);

  const handleConfigSave = async () => {
    setConfigMessage(null);
    setConfigError(null);
    if (alertConfig.enabled && !alertConfig.logPaths.trim()) {
      setConfigError("启用事件决策时必须填写日志路径。");
      return;
    }

    setConfigSaving(true);
    try {
      const resp = await fetch(`${API_BASE}/api/alert-config`, {
        method: "POST",
        headers: buildApiHeaders(true),
        body: JSON.stringify({
          enabled: alertConfig.enabled,
          suppressEnabled: alertConfig.suppressEnabled,
          rulesFile: alertConfig.rulesFile,
          logPaths: alertConfig.logPaths,
          pollInterval: alertConfig.pollInterval,
          startFromEnd: alertConfig.startFromEnd,
        }),
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(text || `保存失败，状态码 ${resp.status}`);
      }
      const payload = (await resp.json()) as AlertConfigResponse;
      if (payload.config) {
        setAlertConfig(payload.config);
      }
      setConfigMessage("运行配置已保存并立即生效。");
      await refreshDashboard();
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : "保存配置失败");
    } finally {
      setConfigSaving(false);
    }
  };

  const handleAiAnalyze = async () => {
    setAiSummary(null);
    setAiError(null);

    const path = aiTargetPath.trim();
    if (!path) {
      setAiError("请先填写需要分析的日志路径。");
      return;
    }

    setAiLoading(true);
    try {
      const payload = await postAiLogSummary({
        path,
        mode: aiQuery.trim() ? "search" : "tail",
        query: aiQuery.trim() || undefined,
        limit: 200,
      });
      if (!payload.ok || !payload.analysis) {
        setAiError(payload.error ?? "AI 分析失败");
        return;
      }
      const summary = formatAiSummary(payload.analysis);
      if (!summary) {
        setAiError("AI 返回为空，请调整关键词或检查日志内容。");
        return;
      }
      setAiSummary(summary);
    } catch (err) {
      setAiError(err instanceof Error ? err.message : "AI 分析失败");
    } finally {
      setAiLoading(false);
    }
  };

  const updateRule = (index: number, patch: Partial<RuleDraft>) => {
    setRuleset((prev) => {
      const nextRules = [...prev.rules];
      if (!nextRules[index]) return prev;
      nextRules[index] = { ...nextRules[index], ...patch };
      return { ...prev, rules: nextRules };
    });
  };

  const addRule = (seed?: Partial<RuleDraft>) => {
    setRuleset((prev) => ({ ...prev, rules: [...prev.rules, createRuleDraft(seed)] }));
  };

  const addRuleInGroup = (level: AlertLevel) => {
    const nextIndex = ruleset.rules.length;
    addRule({ level });
    setExpandedGroups((prev) => ({ ...prev, [level]: true }));
    setExpandedRules((prev) => ({ ...prev, [nextIndex]: true }));
  };

  const moveRule = (index: number, direction: -1 | 1) => {
    setRuleset((prev) => {
      const nextRules = [...prev.rules];
      const target = index + direction;
      if (target < 0 || target >= nextRules.length) return prev;
      [nextRules[index], nextRules[target]] = [nextRules[target], nextRules[index]];
      return { ...prev, rules: nextRules };
    });
  };

  const duplicateRule = (index: number) => {
    setRuleset((prev) => {
      const nextRules = [...prev.rules];
      const current = nextRules[index];
      if (!current) return prev;
      nextRules.splice(
        index + 1,
        0,
        createRuleDraft({
          ...current,
          id: "",
          title: current.title ? `${current.title} 副本` : "",
        })
      );
      return { ...prev, rules: nextRules };
    });
  };

  const removeRule = (index: number) => {
    setRuleset((prev) => ({ ...prev, rules: prev.rules.filter((_, idx) => idx !== index) }));
    setExpandedRules((prev) => {
      const next: Record<number, boolean> = {};
      Object.keys(prev).forEach((key) => {
        const idx = Number(key);
        if (Number.isNaN(idx) || idx === index) return;
        next[idx > index ? idx - 1 : idx] = prev[idx];
      });
      return next;
    });
  };

  const handleRulesSave = async () => {
    setRulesMessage(null);
    setRulesError(null);
    const payload = toApiRuleset(ruleset);

    if (!payload.rules || payload.rules.length === 0) {
      setRulesError("请至少保留一条规则。");
      return;
    }
    const invalidIndex = payload.rules.findIndex((rule) => !rule.keywords || rule.keywords.length === 0);
    if (invalidIndex >= 0) {
      setRulesError(`规则 ${invalidIndex + 1} 缺少关键词。`);
      return;
    }
    if (ruleset.escalation.enabled && (!ruleset.escalation.window.trim() || ruleset.escalation.threshold <= 0)) {
      setRulesError("已开启异常升级，请填写窗口和阈值。");
      return;
    }

    setRulesSaving(true);
    try {
      const resp = await fetch(`${API_BASE}/api/alert-rules`, {
        method: "POST",
        headers: buildApiHeaders(true),
        body: JSON.stringify({ rules: payload }),
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(text || `保存失败，状态码 ${resp.status}`);
      }
      const result = (await resp.json()) as AlertRulesSaveResponse;
      if (!result.ok || !result.rules) {
        setRulesError(result.error ?? "保存规则失败");
        return;
      }
      setRuleset(buildRulesetDraft(result.rules));
      setRulesMessage("规则已保存，下一轮询自动生效。");
      await refreshDashboard();
    } catch (err) {
      setRulesError(err instanceof Error ? err.message : "保存规则失败");
    } finally {
      setRulesSaving(false);
    }
  };

  const loadRecommendations = useCallback(async (decision: AlertDecision) => {
    setRecommendationLoading(true);
    setRecommendationError(null);
    try {
      if (USE_MOCK) {
        setRecommendations([]);
        setRecommendationTrace(decision.knowledgeTrace ?? null);
        return;
      }
      const payload = await fetchKBRecommendations({
        rule: decision.rule,
        message: decision.message,
        alertId: decision.id,
        limit: 3,
      });
      setRecommendations(payload.items ?? []);
      setRecommendationTrace(payload.trace ?? decision.knowledgeTrace ?? null);
    } catch (err) {
      setRecommendationError(err instanceof Error ? err.message : "知识推荐加载失败");
      setRecommendations([]);
      setRecommendationTrace(decision.knowledgeTrace ?? null);
    } finally {
      setRecommendationLoading(false);
    }
  }, []);

  const decisions = useMemo(() => dashboard.decisions ?? [], [dashboard.decisions]);
  const filteredDecisions = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return decisions.filter((decision) => {
      if (levelFilter !== "all" && decision.level !== levelFilter) return false;
      if (statusFilter !== "all" && decision.status !== statusFilter) return false;
      if (!keyword) return true;
      const haystack = `${decision.message} ${decision.rule} ${decision.file || ""}`.toLowerCase();
      return haystack.includes(keyword);
    });
  }, [decisions, levelFilter, query, statusFilter]);

  const totalDecisionPages = Math.max(1, Math.ceil(filteredDecisions.length / DECISIONS_PAGE_SIZE));
  const pagedDecisions = useMemo(
    () => filteredDecisions.slice((decisionPage - 1) * DECISIONS_PAGE_SIZE, decisionPage * DECISIONS_PAGE_SIZE),
    [decisionPage, filteredDecisions]
  );

  const selectedDecision = useMemo(
    () => filteredDecisions.find((decision) => decision.id === selectedDecisionID) ?? filteredDecisions[0] ?? null,
    [filteredDecisions, selectedDecisionID]
  );

  useEffect(() => {
    if (decisionPage > totalDecisionPages) {
      setDecisionPage(totalDecisionPages);
    }
  }, [decisionPage, totalDecisionPages]);

  useEffect(() => {
    setDecisionPage(1);
  }, [levelFilter, query, statusFilter]);

  useEffect(() => {
    if (!filteredDecisions.length) {
      setSelectedDecisionID("");
      setRecommendations([]);
      setRecommendationTrace(null);
      return;
    }
    const exists = filteredDecisions.some((decision) => decision.id === selectedDecisionID);
    if (!exists) {
      setSelectedDecisionID(filteredDecisions[0].id);
    }
  }, [filteredDecisions, selectedDecisionID]);

  useEffect(() => {
    if (activePanel !== "alerts") return;
    if (!selectedDecision) {
      setRecommendations([]);
      setRecommendationTrace(null);
      return;
    }
    void loadRecommendations(selectedDecision);
  }, [activePanel, loadRecommendations, selectedDecision]);

  const summaryMetrics = useMemo(
    () => [
      { label: "P1", value: dashboard.overview.fatal, detail: "最高优先级" },
      { label: "P2", value: dashboard.overview.system, detail: "系统级风险" },
      { label: "已发送", value: dashboard.stats.sent, detail: "已进入处理链路" },
      { label: "已抑制", value: dashboard.stats.suppressed, detail: "已在抑制窗口内" },
    ],
    [dashboard.overview.fatal, dashboard.overview.system, dashboard.stats.sent, dashboard.stats.suppressed]
  );

  const rulesStats = useMemo(() => {
    const stats = {
      total: ruleset.rules.length,
      notify: 0,
      record: 0,
      ignore: 0,
      business: 0,
      system: 0,
      fatal: 0,
    };

    for (const rule of ruleset.rules) {
      stats[rule.level] = (stats[rule.level] ?? 0) + 1;
      if (rule.level === "ignore") continue;
      const notify =
        rule.notifyMode === "inherit" ? rule.level === "system" || rule.level === "fatal" : rule.notifyMode === "send";
      if (notify) {
        stats.notify += 1;
      } else {
        stats.record += 1;
      }
    }
    return stats;
  }, [ruleset.rules]);

  const groupedRules = useMemo(() => {
    const groups: Record<AlertLevel, Array<{ rule: RuleDraft; index: number }>> = {
      fatal: [],
      system: [],
      business: [],
      ignore: [],
    };
    ruleset.rules.forEach((rule, index) => {
      groups[rule.level].push({ rule, index });
    });
    return groups;
  }, [ruleset.rules]);

  const tabItems: Array<{ id: AlertPanel; label: string; desc: string }> = [
    { id: "alerts", label: "事件中心", desc: `${filteredDecisions.length} 条事件` },
    { id: "rules", label: "规则配置", desc: `${rulesStats.total} 条规则` },
    { id: "config", label: "运行设置", desc: alertConfig.enabled ? "已启用" : "未启用" },
  ];

  const levelOrder: AlertLevel[] = ["fatal", "system", "business", "ignore"];
  const riskLabel = enabled ? dashboard.overview.risk : "未启用";
  const riskTone = resolveRiskTone(riskLabel);

  return (
    <div className={`alert-shell${embedded ? " alert-embedded" : ""}`}>
      <section className="panel alert-page-header">
        <div className="alert-header-top">
          <div className="alert-header-copy">
            <p className="eyebrow">事件中心</p>
            <h2>先分诊事件，再进入单事件详情</h2>
            <p>事件页负责筛选、排序和选择当前要处理的事件，规则与运行设置退回二级页签。</p>
          </div>

          <div className="alert-header-side">
            <div className={`alert-risk-badge tone-${riskTone}`}>{riskLabel}</div>
            <div className="alert-header-meta">
              <span>窗口 {dashboard.overview.window || "--"}</span>
              <span>刷新 {lastUpdated}</span>
              <span>{loading ? "同步中…" : "已同步"}</span>
            </div>
          </div>
        </div>

        {error ? <div className="alert-banner">{error}</div> : null}

        <div className="alert-tab-strip" role="tablist" aria-label="事件中心工作域">
          {tabItems.map((item) => (
            <button
              key={item.id}
              className={`alert-tab-button ${activePanel === item.id ? "active" : ""}`}
              type="button"
              onClick={() => setActivePanel(item.id)}
              role="tab"
              aria-selected={activePanel === item.id}
            >
              <strong>{item.label}</strong>
              <span>{item.desc}</span>
            </button>
          ))}
        </div>
      </section>

      {activePanel === "alerts" ? (
        <>
          <div className="alert-kpi-grid">
            {summaryMetrics.map((item) => (
              <div className="alert-kpi-card" key={item.label}>
                <span>{item.label}</span>
                <strong>{item.value}</strong>
                <small>{item.detail}</small>
              </div>
            ))}
          </div>

          <div className="alert-main-grid">
            <section className="panel">
              <div className="section-title">
                <div>
                  <h2>事件列表</h2>
                  <span>用列表页承担筛选与分诊，详情和配置各自独立</span>
                </div>
              </div>

              <div className="toolbar">
                <input
                  className="search"
                  placeholder="搜索事件内容 / 规则 / 文件"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <select className="select" value={levelFilter} onChange={(event) => setLevelFilter(event.target.value as "all" | AlertLevel)}>
                  <option value="all">全部级别</option>
                  <option value="fatal">P1</option>
                  <option value="system">P2</option>
                  <option value="business">P3</option>
                  <option value="ignore">忽略</option>
                </select>
                <select
                  className="select"
                  value={statusFilter}
                  onChange={(event) => setStatusFilter(event.target.value as "all" | AlertDecisionStatus)}
                >
                  <option value="all">全部状态</option>
                  <option value="sent">处理中</option>
                  <option value="suppressed">已抑制</option>
                  <option value="recorded">仅记录</option>
                </select>
              </div>

              {pagedDecisions.length ? (
                <div className="table-wrap">
                  <table className="table alert-table">
                    <thead>
                      <tr>
                        <th>级别</th>
                        <th>事件</th>
                        <th>规则</th>
                        <th>状态</th>
                        <th>时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {pagedDecisions.map((decision) => (
                        <tr
                          key={decision.id}
                          className={decision.id === selectedDecision?.id ? "alert-row-selected" : ""}
                          onClick={() => setSelectedDecisionID(decision.id)}
                        >
                          <td>
                            <span className={`pill ${resolveLevelTone(decision.level)}`}>{LEVEL_LABELS[decision.level]}</span>
                          </td>
                          <td>
                            <div className="alert-row-main">
                              <div className="row-title">{decision.message}</div>
                              <div className="row-sub">{decision.file || "--"}</div>
                            </div>
                          </td>
                          <td>{decision.rule}</td>
                          <td>
                            <span className="badge ghost">{STATUS_LABELS[decision.status]}</span>
                          </td>
                          <td>{decision.time}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <div className="empty-state">当前筛选条件下没有事件。</div>
              )}

              {filteredDecisions.length > DECISIONS_PAGE_SIZE ? (
                <div className="pagination">
                  <span className="muted">
                    第 {decisionPage} / {totalDecisionPages} 页
                  </span>
                  <button
                    className="btn secondary"
                    type="button"
                    onClick={() => setDecisionPage((prev) => Math.max(1, prev - 1))}
                    disabled={decisionPage <= 1}
                  >
                    上一页
                  </button>
                  <button
                    className="btn secondary"
                    type="button"
                    onClick={() => setDecisionPage((prev) => Math.min(totalDecisionPages, prev + 1))}
                    disabled={decisionPage >= totalDecisionPages}
                  >
                    下一页
                  </button>
                </div>
              ) : null}
            </section>

            <aside className="panel alert-detail-panel">
              <div className="section-title">
                <div>
                  <h2>当前事件</h2>
                  <span>右侧固定承接选中事件的判断和知识联动</span>
                </div>
              </div>

              {selectedDecision ? (
                <div className="alert-detail-stack">
                  <div className="alert-detail-card">
                    <span>事件内容</span>
                    <strong>{selectedDecision.message}</strong>
                    <small>
                      {LEVEL_LABELS[selectedDecision.level]} · {STATUS_LABELS[selectedDecision.status]} · {selectedDecision.time}
                    </small>
                  </div>

                  <div className="alert-detail-card">
                    <span>规则与判断</span>
                    <strong>{selectedDecision.rule}</strong>
                    <small>{selectedDecision.reason || "当前没有人工补充判断。"}</small>
                    <div className="row-sub alert-detail-text">{buildDecisionExplainText(selectedDecision)}</div>
                  </div>

                  {selectedDecision.analysis ? (
                    <div className="alert-detail-card">
                      <span>AI 摘要</span>
                      <div className="alert-detail-text">{selectedDecision.analysis}</div>
                    </div>
                  ) : null}

                  <div className="alert-detail-card">
                    <span>知识推荐</span>
                    {recommendationLoading ? <div className="row-sub">知识推荐加载中…</div> : null}
                    {recommendationError ? <div className="row-sub">{recommendationError}</div> : null}
                    {!recommendationLoading && !recommendations.length ? (
                      <div className="row-sub">当前没有匹配知识，建议在事件详情页结案后补回 SOP。</div>
                    ) : null}
                    <div className="alert-detail-list">
                      {recommendations.map((item) => (
                        <div className="alert-mini-card" key={item.id}>
                          <strong>{item.title}</strong>
                          <span>{item.summary || "暂无摘要"}</span>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="alert-detail-card">
                    <span>运行态摘要</span>
                    <div className="alert-detail-list">
                      <div className="alert-mini-card">
                        <strong>规则源</strong>
                        <span>{dashboard.rules.source || "--"}</span>
                      </div>
                      <div className="alert-mini-card">
                        <strong>轮询</strong>
                        <span>{dashboard.polling.interval || "--"}</span>
                      </div>
                      <div className="alert-mini-card">
                        <strong>知识链路</strong>
                        <span>{recommendationTrace?.linkId || selectedDecision.knowledgeTrace?.linkId || "--"}</span>
                      </div>
                    </div>
                  </div>
                </div>
              ) : (
                <div className="empty-state">请先在左侧选择一条事件。</div>
              )}
            </aside>
          </div>
        </>
      ) : null}

      {activePanel === "config" ? (
        <div className="alert-config-grid">
          <section className="panel">
            <div className="section-title">
              <div>
                <h2>运行配置</h2>
                <span>只保留和事件决策直接相关的运行开关与轮询参数</span>
              </div>
            </div>

            <div className="alert-form-grid">
              <div className="input">
                <label>事件决策开关</label>
                <div className="switch-group">
                  <span className="muted small">{alertConfig.enabled ? "已启用" : "已关闭"}</span>
                  <label className="switch">
                    <input
                      type="checkbox"
                      checked={alertConfig.enabled}
                      disabled={configLoading || configSaving}
                      onChange={(event) => setAlertConfig((prev) => ({ ...prev, enabled: event.target.checked }))}
                    />
                    <span className="slider" />
                  </label>
                </div>
              </div>

              <div className="input">
                <label>抑制开关</label>
                <div className="switch-group">
                  <span className="muted small">{alertConfig.suppressEnabled ? "已开启" : "已关闭"}</span>
                  <label className="switch">
                    <input
                      type="checkbox"
                      checked={alertConfig.suppressEnabled}
                      disabled={configLoading || configSaving}
                      onChange={(event) => setAlertConfig((prev) => ({ ...prev, suppressEnabled: event.target.checked }))}
                    />
                    <span className="slider" />
                  </label>
                </div>
              </div>

              <div className="input">
                <label>从末尾开始</label>
                <div className="switch-group">
                  <span className="muted small">{alertConfig.startFromEnd ? "是" : "否"}</span>
                  <label className="switch">
                    <input
                      type="checkbox"
                      checked={alertConfig.startFromEnd}
                      disabled={configLoading || configSaving}
                      onChange={(event) => setAlertConfig((prev) => ({ ...prev, startFromEnd: event.target.checked }))}
                    />
                    <span className="slider" />
                  </label>
                </div>
              </div>

              <div className="input">
                <label>日志路径</label>
                <input
                  value={alertConfig.logPaths}
                  disabled={configLoading || configSaving}
                  onChange={(event) => setAlertConfig((prev) => ({ ...prev, logPaths: event.target.value }))}
                />
              </div>

              <div className="input">
                <label>轮询间隔</label>
                <input
                  value={alertConfig.pollInterval}
                  disabled={configLoading || configSaving}
                  onChange={(event) => setAlertConfig((prev) => ({ ...prev, pollInterval: event.target.value }))}
                />
              </div>
            </div>

            <div className="toolbar config-actions">
              <button className="btn" type="button" onClick={() => void handleConfigSave()} disabled={configLoading || configSaving}>
                {configSaving ? "保存中…" : "保存运行配置"}
              </button>
            </div>

            {configMessage ? <div className="badge">{configMessage}</div> : null}
            {configError ? <div className="warn-text">{configError}</div> : null}
          </section>

          <section className="panel">
            <div className="section-title">
              <div>
                <h2>AI 快速分析</h2>
                <span>用于对当前日志路径做一次快速诊断，帮助补充规则或事件判断</span>
              </div>
            </div>

            <div className="alert-form-grid">
              <div className="input">
                <label>日志路径</label>
                <input value={aiTargetPath} disabled={aiLoading} onChange={(event) => setAiTargetPath(event.target.value)} />
              </div>
              <div className="input">
                <label>检索关键词</label>
                <input value={aiQuery} disabled={aiLoading} onChange={(event) => setAiQuery(event.target.value)} />
              </div>
            </div>

            <div className="toolbar config-actions">
              <button className="btn secondary" type="button" onClick={() => void handleAiAnalyze()} disabled={aiLoading}>
                {aiLoading ? "分析中…" : "开始分析"}
              </button>
            </div>

            {aiSummary ? <pre className="alert-ai-result">{aiSummary}</pre> : null}
            {aiError ? <div className="warn-text">{aiError}</div> : null}
          </section>
        </div>
      ) : null}

      {activePanel === "rules" ? (
        <div className="alert-rules-stack">
          <div className="alert-rule-metric-grid">
            <div className="alert-rule-metric">
              <span>规则总数</span>
              <strong>{rulesStats.total}</strong>
            </div>
            <div className="alert-rule-metric">
              <span>P1 / P2</span>
              <strong>
                {rulesStats.fatal} / {rulesStats.system}
              </strong>
            </div>
            <div className="alert-rule-metric">
              <span>发送</span>
              <strong>{rulesStats.notify}</strong>
            </div>
            <div className="alert-rule-metric">
              <span>仅记录</span>
              <strong>{rulesStats.record}</strong>
            </div>
          </div>

          <div className="alert-rule-top-grid">
            <section className="panel">
              <div className="section-title">
                <div>
                  <h2>默认策略</h2>
                  <span>未单独设置的规则继承这里的抑制窗口与大小写策略</span>
                </div>
              </div>

              <div className="alert-form-grid">
                <div className="input">
                  <label>默认抑制窗口</label>
                  <input
                    value={ruleset.defaults.suppressWindow}
                    disabled={rulesLoading || rulesSaving}
                    onChange={(event) =>
                      setRuleset((prev) => ({
                        ...prev,
                        defaults: { ...prev.defaults, suppressWindow: event.target.value },
                      }))
                    }
                  />
                </div>

                <div className="input">
                  <label>默认大小写</label>
                  <select
                    value={ruleset.defaults.matchCase ? "true" : "false"}
                    disabled={rulesLoading || rulesSaving}
                    onChange={(event) =>
                      setRuleset((prev) => ({
                        ...prev,
                        defaults: { ...prev.defaults, matchCase: event.target.value === "true" },
                      }))
                    }
                  >
                    <option value="false">不区分大小写</option>
                    <option value="true">区分大小写</option>
                  </select>
                </div>
              </div>
            </section>

            <section className="panel">
              <div className="section-title">
                <div>
                  <h2>异常升级</h2>
                  <span>当系统级事件在短时间内激增时，自动提升优先级</span>
                </div>
              </div>

              <div className="alert-form-grid">
                <div className="input">
                  <label>升级开关</label>
                  <div className="switch-group">
                    <span className="muted small">{ruleset.escalation.enabled ? "已启用" : "已关闭"}</span>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={ruleset.escalation.enabled}
                        disabled={rulesLoading || rulesSaving}
                        onChange={(event) =>
                          setRuleset((prev) => ({
                            ...prev,
                            escalation: { ...prev.escalation, enabled: event.target.checked },
                          }))
                        }
                      />
                      <span className="slider" />
                    </label>
                  </div>
                </div>

                <div className="input">
                  <label>窗口</label>
                  <input
                    value={ruleset.escalation.window}
                    disabled={rulesLoading || rulesSaving || !ruleset.escalation.enabled}
                    onChange={(event) =>
                      setRuleset((prev) => ({
                        ...prev,
                        escalation: { ...prev.escalation, window: event.target.value },
                      }))
                    }
                  />
                </div>

                <div className="input">
                  <label>阈值</label>
                  <input
                    type="number"
                    min={1}
                    value={ruleset.escalation.threshold}
                    disabled={rulesLoading || rulesSaving || !ruleset.escalation.enabled}
                    onChange={(event) =>
                      setRuleset((prev) => ({
                        ...prev,
                        escalation: { ...prev.escalation, threshold: Number(event.target.value) || 0 },
                      }))
                    }
                  />
                </div>

                <div className="input">
                  <label>升级级别</label>
                  <select
                    value={ruleset.escalation.level}
                    disabled={rulesLoading || rulesSaving || !ruleset.escalation.enabled}
                    onChange={(event) =>
                      setRuleset((prev) => ({
                        ...prev,
                        escalation: { ...prev.escalation, level: event.target.value as AlertLevel },
                      }))
                    }
                  >
                    <option value="business">P3</option>
                    <option value="system">P2</option>
                    <option value="fatal">P1</option>
                  </select>
                </div>
              </div>
            </section>
          </div>

          <section className="panel">
            <div className="section-title">
              <div>
                <h2>规则列表</h2>
                <span>按级别分组管理规则，页面重点在可读和可维护，而不是花哨效果</span>
              </div>
              <div className="toolbar-actions">
                <button className="btn secondary" type="button" onClick={() => void fetchRules()} disabled={rulesLoading || rulesSaving}>
                  刷新规则
                </button>
                <button className="btn" type="button" onClick={() => void handleRulesSave()} disabled={rulesLoading || rulesSaving}>
                  {rulesSaving ? "保存中…" : "保存规则"}
                </button>
              </div>
            </div>

            <div className="alert-rule-group-list">
              {levelOrder.map((level) => {
                const items = groupedRules[level];
                return (
                  <div className="alert-rule-group" key={level}>
                    <div className="alert-rule-group-head">
                      <div>
                        <strong>
                          {LEVEL_LABELS[level]} · {items.length} 条
                        </strong>
                        <div className="row-sub">
                          {level === "fatal"
                            ? "直接影响故障升级链"
                            : level === "system"
                              ? "需要运维介入的系统级事件"
                              : level === "business"
                                ? "建议更多记录与沉淀"
                                : "默认忽略的噪声规则"}
                        </div>
                      </div>
                      <div className="toolbar-actions">
                        <button className="btn secondary" type="button" onClick={() => addRuleInGroup(level)} disabled={rulesLoading || rulesSaving}>
                          新增
                        </button>
                        <button
                          className="btn secondary"
                          type="button"
                          onClick={() => setExpandedGroups((prev) => ({ ...prev, [level]: !prev[level] }))}
                        >
                          {expandedGroups[level] ? "收起" : "展开"}
                        </button>
                      </div>
                    </div>

                    {expandedGroups[level] ? (
                      <div className="alert-rule-card-list">
                        {items.length ? (
                          items.map(({ rule, index }) => (
                            <div className="alert-rule-card" key={`${level}-${index}`}>
                              <div className="alert-rule-card-head">
                                <div>
                                  <strong>{rule.title || "未命名规则"}</strong>
                                  <div className="row-sub">
                                    {rule.id || "自动生成 ID"} · {splitTokens(rule.keywordsText).length} 个关键词
                                  </div>
                                </div>
                                <div className="toolbar-actions">
                                  <button className="btn secondary" type="button" onClick={() => setExpandedRules((prev) => ({ ...prev, [index]: !prev[index] }))}>
                                    {expandedRules[index] ? "收起" : "编辑"}
                                  </button>
                                  <button className="btn secondary" type="button" onClick={() => moveRule(index, -1)} disabled={index === 0}>
                                    上移
                                  </button>
                                  <button
                                    className="btn secondary"
                                    type="button"
                                    onClick={() => moveRule(index, 1)}
                                    disabled={index === ruleset.rules.length - 1}
                                  >
                                    下移
                                  </button>
                                  <button className="btn secondary" type="button" onClick={() => duplicateRule(index)}>
                                    复制
                                  </button>
                                  <button className="btn secondary" type="button" onClick={() => removeRule(index)}>
                                    删除
                                  </button>
                                </div>
                              </div>

                              {expandedRules[index] ? (
                                <div className="alert-form-grid">
                                  <div className="input">
                                    <label>规则标题</label>
                                    <input value={rule.title} onChange={(event) => updateRule(index, { title: event.target.value })} />
                                  </div>
                                  <div className="input">
                                    <label>规则 ID</label>
                                    <input value={rule.id} onChange={(event) => updateRule(index, { id: event.target.value })} />
                                  </div>
                                  <div className="input">
                                    <label>级别</label>
                                    <select value={rule.level} onChange={(event) => updateRule(index, { level: event.target.value as AlertLevel })}>
                                      <option value="ignore">忽略</option>
                                      <option value="business">P3</option>
                                      <option value="system">P2</option>
                                      <option value="fatal">P1</option>
                                    </select>
                                  </div>
                                  <div className="input">
                                    <label>通知策略</label>
                                    <select value={rule.notifyMode} onChange={(event) => updateRule(index, { notifyMode: event.target.value as NotifyMode })}>
                                      <option value="inherit">自动</option>
                                      <option value="send">发送</option>
                                      <option value="record">仅记录</option>
                                    </select>
                                  </div>
                                  <div className="input">
                                    <label>抑制窗口</label>
                                    <input
                                      value={rule.suppressWindow}
                                      placeholder={`默认 ${ruleset.defaults.suppressWindow}`}
                                      onChange={(event) => updateRule(index, { suppressWindow: event.target.value })}
                                    />
                                  </div>
                                  <div className="input">
                                    <label>大小写</label>
                                    <select value={rule.matchCaseMode} onChange={(event) => updateRule(index, { matchCaseMode: event.target.value as MatchCaseMode })}>
                                      <option value="inherit">跟随默认</option>
                                      <option value="case">区分大小写</option>
                                      <option value="nocase">不区分大小写</option>
                                    </select>
                                  </div>
                                  <div className="input alert-input-full">
                                    <label>关键词</label>
                                    <textarea value={rule.keywordsText} onChange={(event) => updateRule(index, { keywordsText: event.target.value })} />
                                  </div>
                                  <div className="input alert-input-full">
                                    <label>排除词</label>
                                    <textarea value={rule.excludesText} onChange={(event) => updateRule(index, { excludesText: event.target.value })} />
                                  </div>
                                </div>
                              ) : null}
                            </div>
                          ))
                        ) : (
                          <div className="empty-state">当前级别还没有规则。</div>
                        )}
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>

            {rulesMessage ? <div className="badge">{rulesMessage}</div> : null}
            {rulesError ? <div className="warn-text">{rulesError}</div> : null}
          </section>
        </div>
      ) : null}
    </div>
  );
}
