/**
 * 文件职责：承载多云资产管理后台首页
 * 关键交互：左侧菜单切换页面，云资产页按云平台、账号、地域聚合 ECS/RDS 资源
 * 边界处理：后端云账号同步未接入前使用 mock 数据，接口接入后只替换数据来源
 */

/* 本文件用于普通后台 Web 页面，融合 Komari 的探针监控视角和云镜当前的多云监控主线 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { API_BASE, USE_MOCK, buildApiHeaders } from "./console/dashboardApi";
import "./CloudResourceConsole.css";

type PageKey =
  | "servers"
  | "accounts"
  | "regions"
  | "monitor"
  | "agents"
  | "alerts"
  | "ai"
  | "knowledge"
  | "sync"
  | "settings";

type AlertSectionKey = "health" | "reminders" | "rules" | "coverage";

type MenuItem = {
  key: PageKey;
  label: string;
  desc: string;
  children?: Array<{ key: string; label: string; desc: string }>;
};

type Provider = "aliyun" | "huawei" | "tencent";
type AccountStatus = "normal" | "warning" | "syncing" | "disabled";
type ServerStatus = "running" | "warning" | "offline" | "maintenance";
type AgentStatus = "online" | "stale" | "missing" | "offline";
type ScopeKind = "all" | "product" | "productProvider" | "productAccount" | "productAccountRegion" | "region" | "server";
type ThemeMode = "light" | "dark";
type PublicIpType = "public" | "eip" | "none";
type ResourceType = "ecs" | "rds";
type ServerSortKey = "cpu" | "memory" | "disk" | "expiration";
type SortDirection = "asc" | "desc";
type ExpirationStatus = "normal" | "expiring" | "expired" | "no_expiration" | "unknown";

type CloudAccount = {
  id: string;
  provider: Provider;
  name: string;
  alias: string;
  uid: string;
  owner: string;
  scope: string;
  status: AccountStatus;
  lastSync: string;
  regions: string[];
  enabled?: boolean;
  metricPeriod?: string;
  checkMessage?: string;
};

type CloudServer = {
  id: string;
  accountId: string;
  provider: Provider;
  resourceType?: ResourceType;
  region: string;
  zone: string;
  instanceId: string;
  name: string;
  business: string;
  publicIp: string;
  publicIpType?: PublicIpType;
  publicIpId?: string;
  privateIp: string;
  os: string;
  spec: string;
  chargeType?: string;
  isSpot?: boolean;
  spotStrategy?: string;
  createdAt?: string;
  expiredAt?: string;
  expiresInDays?: number;
  expirationStatus?: ExpirationStatus;
  expirationText?: string;
  engine?: string;
  engineVersion?: string;
  dbInstanceType?: string;
  dbCategory?: string;
  dbClass?: string;
  dbStorageGb?: number;
  dbStorageType?: string;
  dbConnectionString?: string;
  dbPort?: string;
  dbMaxConnections?: number;
  dbMaxIops?: number;
  dbMaxIombps?: number;
  dbEndpointCount?: number;
  dbLockMode?: string;
  dbLockReason?: string;
  status: ServerStatus;
  agentStatus: AgentStatus;
  cpu?: number;
  memory?: number;
  disk?: number;
  load: string;
  uptime: string;
  lastSeen: string;
  alerts: number;
  files: number;
  ai: number;
  kb: number;
};

type SyncTask = {
  id: string;
  accountId: string;
  name: string;
  status: "完成" | "执行中" | "等待" | "失败";
  progress: string;
  updatedAt: string;
};

type AliyunInstance = {
  id: string;
  name: string;
  hostName: string;
  provider: Provider;
  regionId: string;
  zoneId: string;
  status: string;
  osName: string;
  osType: string;
  type: string;
  chargeType?: string;
  isSpot?: boolean;
  spotStrategy?: string;
  cpu: number;
  memoryMb: number;
  publicIps: string[];
  eipAddress?: string;
  eipId?: string;
  privateIps: string[];
  vpcId: string;
  vSwitchId: string;
  securityGroupIds: string[];
  createdAt: string;
  expiredAt: string;
  expiresInDays?: number;
  expirationStatus?: ExpirationStatus;
  expirationMessage?: string;
};

type AliyunInstancesResponse = {
  ok?: boolean;
  accountId?: number;
  provider?: string;
  items?: AliyunInstance[];
  total?: number;
  error?: string;
};

type AliyunMetricPoint = {
  timestamp: number;
  value: number;
  raw?: Record<string, unknown>;
};

type AliyunMetricSeries = {
  namespace?: string;
  metricName: string;
  label?: string;
  subKey?: string;
  unit?: string;
  valueFormat?: string;
  period?: string;
  points?: AliyunMetricPoint[];
};

type AliyunOverviewResponse = {
  ok?: boolean;
  accountId?: number;
  resource?: ResourceType | string;
  status?: string;
  message?: string;
  availableMetricCount?: number;
  metrics?: Record<string, AliyunMetricSeries>;
  errors?: Record<string, string>;
  error?: string;
};

type AliyunRDSEndpoint = {
  connectionString: string;
  port: string;
  ipAddress?: string;
  ipType?: string;
  connectionStringType?: string;
  availability?: string;
  vpcId?: string;
  vSwitchId?: string;
};

type AliyunRDSResourceUsage = {
  dbInstanceId?: string;
  engine?: string;
  diskUsedBytes?: number;
  dataSizeBytes?: number;
  logSizeBytes?: number;
  sqlSizeBytes?: number;
  backupSizeBytes?: number;
  storageUsagePercent?: number;
  source?: string;
};

type AliyunRDSInstance = {
  id: string;
  nodeId?: string;
  name: string;
  provider: Provider;
  regionId: string;
  zoneId: string;
  engine: string;
  engineVersion: string;
  status: string;
  lockMode?: string;
  lockReason?: string;
  type?: string;
  category?: string;
  class?: string;
  classType?: string;
  cpu?: number;
  cpuRaw?: string;
  memoryMb?: number;
  storageGb?: number;
  storageType?: string;
  maxConnections?: number;
  maxIops?: number;
  maxIombps?: number;
  networkType?: string;
  netType?: string;
  connectionMode?: string;
  connectionString?: string;
  port?: string;
  vpcId?: string;
  vSwitchId?: string;
  createdAt: string;
  expiredAt: string;
  payType: string;
  endpoints?: AliyunRDSEndpoint[];
  resourceUsage?: AliyunRDSResourceUsage;
  detailErrors?: string[];
  expiresInDays?: number;
  expirationStatus?: ExpirationStatus;
  expirationMessage?: string;
};

type AliyunRDSInstancesResponse = {
  ok?: boolean;
  accountId?: number;
  provider?: string;
  resource?: string;
  items?: AliyunRDSInstance[];
  total?: number;
  error?: string;
};

type RDSStorageUsage = {
  percent?: number;
  metricName?: string;
  subKey?: string;
  unit?: string;
  rawValue?: number;
  storageGb?: number;
  source: "official" | "percent" | "size" | "missing";
};

type ApiCloudAccount = {
  id: number;
  provider: Provider;
  name: string;
  accessKeyIdMasked: string;
  regions: string[];
  metricPeriod: string;
  enabled: boolean;
  lastCheckStatus?: string;
  lastCheckMessage?: string;
  lastCheckedAt?: string;
};

type CloudAccountsResponse = {
  ok?: boolean;
  items?: ApiCloudAccount[];
  error?: string;
};

type CloudAccountCheck = {
  name: string;
  status: string;
  ok: boolean;
  message: string;
};

type CloudAccountTestResponse = {
  ok?: boolean;
  account?: ApiCloudAccount;
  checks?: CloudAccountCheck[];
  error?: string;
};

type CloudAccountForm = {
  provider: Provider;
  name: string;
  accessKeyId: string;
  accessKeySecret: string;
  metricPeriod: string;
};

type MonitorMetricKey = string;

type MonitorMetric = {
  key: MonitorMetricKey;
  label: string;
  unit: string;
  source: string;
  metricName: string;
  period: string;
  value: string;
  range: string;
  average: string;
  sampledAt: string;
  trend: string;
  sparklinePoints: AliyunMetricPoint[];
  recentSamples: string[];
  points: number;
  status: "ok" | "empty" | "error";
  note: string;
};

type RefreshState = "idle" | "syncing" | "ok" | "error";

type CloudAssetIssue = {
  id: string;
  provider: Provider;
  accountId: string;
  accountName: string;
  resourceType: ResourceType;
  scope: string;
  message: string;
};

type CloudOverviewRefreshResult = {
  server: CloudServer;
  overview: AliyunOverviewResponse;
  issue?: CloudAssetIssue;
};

type CloudOverviewRefreshTask = () => Promise<CloudOverviewRefreshResult>;

type AgentStatusCounter = Record<AgentStatus, number>;

type AgentAccountSummary = {
  key: string;
  provider: Provider;
  accountName: string;
  total: number;
  covered: number;
  statusCounts: AgentStatusCounter;
  pointCount: number;
  latestTimestamp: number;
  resourceTypes: ResourceType[];
  regionCount: number;
  realSampleResourceCount: number;
  sources: string[];
  riskCount: number;
};

type AgentRegionSummary = {
  key: string;
  provider: Provider;
  region: string;
  total: number;
  covered: number;
  statusCounts: AgentStatusCounter;
  accountCount: number;
  pointCount: number;
  latestTimestamp: number;
  realSampleResourceCount: number;
  riskCount: number;
};

type AgentSourceSummary = {
  source: string;
  resourceCount: number;
  pointCount: number;
  latestTimestamp: number;
  periods: string[];
};

type AgentSourceSnapshot = {
  source: string;
  pointCount: number;
  latestTimestamp: number;
  periods: string[];
};

type AgentGovernanceItem = {
  title: string;
  detail: string;
  status: AgentStatus | "normal";
};

type ResourceAccountNode = {
  key: string;
  resourceType: ResourceType;
  provider: Provider;
  accountId: string;
  accountName: string;
  rows: CloudServer[];
};

type ResourceProviderNode = {
  key: string;
  resourceType: ResourceType;
  provider: Provider;
  rows: CloudServer[];
  accounts: ResourceAccountNode[];
};

type ResourceProductNode = {
  key: string;
  resourceType: ResourceType;
  rows: CloudServer[];
  providers: ResourceProviderNode[];
};

type CloudAlertSeverity = "info" | "warning" | "critical";

type CloudAlertRuleResult = {
  severity?: CloudAlertSeverity;
  title?: string;
  currentValue: string;
  condition?: string;
  message: string;
  suggestion: string;
};

type CloudAlertRule = {
  id: string;
  title: string;
  resourceTypes: ResourceType[];
  providers: Provider[];
  metric: string;
  condition: string;
  severity: CloudAlertSeverity;
  enabled: boolean;
  notifyChannels: string[];
  evaluate: (server: CloudServer, config: CloudAlertRuleConfig) => CloudAlertRuleResult | null;
};

type CloudAlertRuleConfig = {
  enabled: boolean;
  severity: CloudAlertSeverity;
  warningThreshold?: number;
  criticalThreshold?: number;
  notifyChannels: string[];
};

type CloudAlertReminder = {
  id: string;
  ruleId: string;
  serverId: string;
  accountId: string;
  provider: Provider;
  resourceType: ResourceType;
  resourceName: string;
  instanceId: string;
  region: string;
  severity: CloudAlertSeverity;
  title: string;
  metric: string;
  condition: string;
  currentValue: string;
  message: string;
  suggestion: string;
  notifyChannels: string[];
  updatedAt: string;
};

type AlertWorkbenchFilter = {
  severity?: CloudAlertSeverity;
  ruleId?: string;
  coverageKey?: string;
  serverId?: string;
};

type ResourceHealthLevel = "healthy" | "watch" | "risk" | "critical";

type ResourceHealthScore = {
  score: number;
  level: ResourceHealthLevel;
  reasons: string[];
};

type ResourceHealthRow = ResourceHealthScore & {
  server: CloudServer;
  reminders: CloudAlertReminder[];
};

const providerLabels: Record<Provider, string> = {
  aliyun: "阿里云",
  huawei: "华为云",
  tencent: "腾讯云",
};

const supportedAccountProviders: Provider[] = ["aliyun", "huawei"];

const resourceTypeLabels: Record<ResourceType, string> = {
  ecs: "ECS",
  rds: "RDS",
};

const resourceTypes: ResourceType[] = ["ecs", "rds"];

const getResourceType = (server: CloudServer): ResourceType => server.resourceType ?? "ecs";

const emptyCloudServer: CloudServer = {
  id: "",
  accountId: "aliyun-runtime",
  provider: "aliyun",
  resourceType: "ecs",
  region: "--",
  zone: "--",
  instanceId: "--",
  name: "--",
  business: "未选择资源",
  publicIp: "--",
  publicIpType: "none",
  privateIp: "--",
  os: "--",
  spec: "--",
  chargeType: "",
  isSpot: false,
  spotStrategy: "",
  expiredAt: "",
  expirationStatus: "unknown",
  expirationText: "未返回到期时间",
  status: "maintenance",
  agentStatus: "missing",
  cpu: 0,
  memory: 0,
  disk: 0,
  load: "--",
  uptime: "--",
  lastSeen: "--",
  alerts: 0,
  files: 0,
  ai: 0,
  kb: 0,
};

const SIDEBAR_STORAGE_KEY = "gwf-cloud-sidebar-hidden";
const THEME_STORAGE_KEY = "gwf-cloud-theme";
const ASSET_REFRESH_INTERVAL_MS = 30_000;
const CLOUD_ASSET_FETCH_TIMEOUT_MS = 60_000;
const CLOUD_OVERVIEW_FETCH_TIMEOUT_MS = 30_000;

const emptyAccountForm: CloudAccountForm = {
  provider: "aliyun",
  name: "",
  accessKeyId: "",
  accessKeySecret: "",
  metricPeriod: "60",
};

const fetchWithTimeout = async (input: RequestInfo | URL, init: RequestInit = {}, timeoutMs = CLOUD_ASSET_FETCH_TIMEOUT_MS) => {
  const controller = new AbortController();
  const timer = globalThis.setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(input, { ...init, signal: controller.signal });
  } finally {
    globalThis.clearTimeout(timer);
  }
};

const resolveInitialSidebarHidden = () => {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === "true";
};

const resolveInitialTheme = (): ThemeMode => {
  if (typeof window === "undefined") return "light";
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === "light" || stored === "dark") return stored;
  if (window.matchMedia?.("(prefers-color-scheme: dark)").matches) return "dark";
  return "light";
};

const accountStatusLabels: Record<AccountStatus, string> = {
  normal: "正常",
  warning: "需关注",
  syncing: "同步中",
  disabled: "停用",
};

const serverStatusLabels: Record<ServerStatus, string> = {
  running: "运行中",
  warning: "异常",
  offline: "离线",
  maintenance: "维护中",
};

const agentStatusLabels: Record<AgentStatus, string> = {
  online: "正常",
  stale: "数据异常",
  missing: "未同步",
  offline: "离线",
};

const agentStatusOrder: AgentStatus[] = ["online", "stale", "missing", "offline"];

const agentStatusDescriptions: Record<AgentStatus, string> = {
  online: "最新采样可用",
  stale: "采样偏旧或接口异常",
  missing: "尚未形成采样",
  offline: "资源离线不可采样",
};

const emptyAgentStatusCounter = (): AgentStatusCounter => ({
  online: 0,
  stale: 0,
  missing: 0,
  offline: 0,
});

const menuGroups: Array<{
  title: string;
  items: MenuItem[];
}> = [
  {
    title: "云资产",
    items: [
      { key: "servers", label: "资源总览", desc: "ECS/RDS 资源", children: [
        { key: "overview", label: "资源看板", desc: "数量和风险概览" },
        { key: "list", label: "资源清单", desc: "筛选和查看资源" },
        { key: "detail", label: "资源详情", desc: "选中资源上下文" },
      ] },
      { key: "accounts", label: "云账号管理", desc: "账号与同步状态", children: [
        { key: "list", label: "已接入账号", desc: "状态与操作" },
        { key: "form", label: "账号配置", desc: "新增或编辑" },
      ] },
      { key: "regions", label: "地域视图", desc: "按地域聚合" },
    ],
  },
  {
    title: "监控运维",
    items: [
      { key: "monitor", label: "监控概览", desc: "ECS/RDS 指标", children: [
        { key: "workspace", label: "监控工作台", desc: "选资源看指标" },
      ] },
      { key: "agents", label: "探针管理", desc: "采样覆盖与质量", children: [
        { key: "overview", label: "治理总览", desc: "覆盖率和缺口" },
        { key: "actions", label: "治理提醒", desc: "优先处理项" },
        { key: "accounts", label: "账号覆盖", desc: "按账号查看" },
        { key: "regions", label: "地域缺口", desc: "按地域查看" },
        { key: "sources", label: "采样来源", desc: "来源质量" },
      ] },
      {
        key: "alerts",
        label: "告警事件",
        desc: "告警与处置",
        children: [
          { key: "reminders", label: "云产品提醒", desc: "当前命中事件" },
          { key: "health", label: "高风险资源", desc: "健康分排序" },
          { key: "rules", label: "规则配置", desc: "阈值与渠道" },
          { key: "coverage", label: "覆盖情况", desc: "账号产品分组" },
        ],
      },
    ],
  },
  {
    title: "异常排查",
    items: [
      { key: "ai", label: "辅助判断", desc: "异常证据摘要", children: [
        { key: "scope", label: "能力定位", desc: "输入和输出" },
        { key: "analysis", label: "资源判断", desc: "异常辅助结论" },
      ] },
      { key: "knowledge", label: "处置知识", desc: "SOP 与复盘", children: [
        { key: "scope", label: "沉淀规则", desc: "来源和回流" },
        { key: "entries", label: "知识条目", desc: "处置场景" },
      ] },
    ],
  },
  {
    title: "接入治理",
    items: [
      { key: "sync", label: "同步健康", desc: "账号与资源同步", children: [
        { key: "scope", label: "同步说明", desc: "定位和重点" },
        { key: "tasks", label: "任务列表", desc: "同步进度" },
      ] },
      { key: "settings", label: "接入边界", desc: "配置与安全约束", children: [
        { key: "config", label: "配置边界", desc: "接入和安全" },
      ] },
    ],
  },
];

const mockAccounts: CloudAccount[] = [
  {
    id: "aliyun-prod-cn",
    provider: "aliyun",
    name: "阿里云生产账号",
    alias: "阿里云 / 生产",
    uid: "19****6721",
    owner: "生产运维组",
    scope: "生产环境资源",
    status: "normal",
    lastSync: "13:18:22",
    regions: ["华东1 杭州", "华北2 北京"],
  },
  {
    id: "aliyun-hk",
    provider: "aliyun",
    name: "阿里云海外账号",
    alias: "阿里云 / 海外",
    uid: "13****9026",
    owner: "海外业务组",
    scope: "生产环境资源",
    status: "warning",
    lastSync: "13:16:04",
    regions: ["中国香港"],
  },
  {
    id: "huawei-prod",
    provider: "huawei",
    name: "华为云生产账号",
    alias: "华为云 / 生产",
    uid: "hw-****-2871",
    owner: "基础平台组",
    scope: "生产环境资源",
    status: "normal",
    lastSync: "13:15:33",
    regions: ["华北-北京四", "华南-广州"],
  },
  {
    id: "huawei-test",
    provider: "huawei",
    name: "华为云测试账号",
    alias: "华为云 / 测试",
    uid: "hw-****-1098",
    owner: "研发测试组",
    scope: "测试环境资源",
    status: "syncing",
    lastSync: "同步中",
    regions: ["华东-上海一"],
  },
  {
    id: "tencent-lab",
    provider: "tencent",
    name: "腾讯云实验账号",
    alias: "腾讯云 / 实验",
    uid: "tc-****-0516",
    owner: "个人实验",
    scope: "实验环境资源",
    status: "normal",
    lastSync: "13:08:10",
    regions: ["广州"],
  },
];

const mockServers: CloudServer[] = [
  {
    id: "ecs-order-01",
    accountId: "aliyun-prod-cn",
    provider: "aliyun",
    region: "华东1 杭州",
    zone: "cn-hangzhou-h",
    instanceId: "i-bp1-order-01",
    name: "order-api-prod-01",
    business: "订单核心",
    publicIp: "47.98.12.31",
    privateIp: "10.18.4.21",
    os: "Alibaba Cloud Linux 3",
    spec: "8C16G",
    chargeType: "PrePaid",
    expiredAt: "2026-08-05T00:00:00Z",
    status: "running",
    agentStatus: "online",
    cpu: 42,
    memory: 61,
    disk: 48,
    load: "1.24",
    uptime: "48 天",
    lastSeen: "12 秒前",
    alerts: 1,
    files: 36,
    ai: 4,
    kb: 3,
  },
  {
    id: "ecs-pay-01",
    accountId: "aliyun-prod-cn",
    provider: "aliyun",
    region: "华东1 杭州",
    zone: "cn-hangzhou-i",
    instanceId: "i-bp1-pay-01",
    name: "payment-worker-prod",
    business: "支付异步任务",
    publicIp: "121.40.86.17",
    privateIp: "10.18.7.18",
    os: "Ubuntu 22.04",
    spec: "2C8G",
    chargeType: "PrePaid",
    expiredAt: "2026-05-29T00:00:00Z",
    status: "warning",
    agentStatus: "online",
    cpu: 78,
    memory: 73,
    disk: 67,
    load: "3.84",
    uptime: "19 天",
    lastSeen: "9 秒前",
    alerts: 4,
    files: 19,
    ai: 5,
    kb: 2,
  },
  {
    id: "ecs-pre-01",
    accountId: "aliyun-prod-cn",
    provider: "aliyun",
    region: "华北2 北京",
    zone: "cn-beijing-k",
    instanceId: "i-bp1-pre-api",
    name: "pre-api-01",
    business: "预发 API",
    publicIp: "39.105.12.44",
    privateIp: "10.38.2.19",
    os: "Alibaba Cloud Linux 3",
    spec: "2C4G",
    chargeType: "PostPaid",
    isSpot: true,
    spotStrategy: "SpotAsPriceGo",
    status: "running",
    agentStatus: "missing",
    cpu: 18,
    memory: 44,
    disk: 33,
    load: "0.22",
    uptime: "11 天",
    lastSeen: "未接入",
    alerts: 0,
    files: 0,
    ai: 0,
    kb: 0,
  },
  {
    id: "ecs-gateway-hk",
    accountId: "aliyun-hk",
    provider: "aliyun",
    region: "中国香港",
    zone: "cn-hongkong-b",
    instanceId: "i-j6c-gateway-01",
    name: "gateway-hk-01",
    business: "海外网关",
    publicIp: "8.210.42.19",
    privateIp: "172.19.2.11",
    os: "Debian 12",
    spec: "2C4G",
    chargeType: "PrePaid",
    expiredAt: "2026-05-17T00:00:00Z",
    status: "running",
    agentStatus: "online",
    cpu: 49,
    memory: 58,
    disk: 38,
    load: "0.72",
    uptime: "63 天",
    lastSeen: "21 秒前",
    alerts: 2,
    files: 12,
    ai: 2,
    kb: 2,
  },
  {
    id: "ecs-web-hk",
    accountId: "aliyun-hk",
    provider: "aliyun",
    region: "中国香港",
    zone: "cn-hongkong-c",
    instanceId: "i-j6c-web-02",
    name: "web-hk-02",
    business: "海外 Web",
    publicIp: "47.242.16.87",
    privateIp: "172.19.3.22",
    os: "Alibaba Cloud Linux 3",
    spec: "2C4G",
    chargeType: "PrePaid",
    expiredAt: "2026-05-01T00:00:00Z",
    status: "offline",
    agentStatus: "offline",
    cpu: 0,
    memory: 0,
    disk: 52,
    load: "--",
    uptime: "--",
    lastSeen: "38 分钟前",
    alerts: 3,
    files: 0,
    ai: 1,
    kb: 0,
  },
  {
    id: "huawei-app-01",
    accountId: "huawei-prod",
    provider: "huawei",
    region: "华北-北京四",
    zone: "cn-north-4a",
    instanceId: "ecs-hw-app-01",
    name: "hw-app-prod-01",
    business: "核心应用",
    publicIp: "119.3.42.18",
    privateIp: "10.42.1.11",
    os: "EulerOS 2.0",
    spec: "4C8G",
    chargeType: "PostPaid",
    status: "running",
    agentStatus: "online",
    cpu: 31,
    memory: 52,
    disk: 41,
    load: "0.68",
    uptime: "72 天",
    lastSeen: "16 秒前",
    alerts: 0,
    files: 24,
    ai: 1,
    kb: 1,
  },
  {
    id: "huawei-db-01",
    accountId: "huawei-prod",
    provider: "huawei",
    region: "华南-广州",
    zone: "cn-south-1b",
    instanceId: "ecs-hw-db-01",
    name: "hw-db-readonly-01",
    business: "只读数据库",
    publicIp: "-",
    privateIp: "10.56.9.21",
    os: "EulerOS 2.0",
    spec: "8C32G",
    chargeType: "PrePaid",
    expiredAt: "2026-07-20T00:00:00Z",
    status: "warning",
    agentStatus: "online",
    cpu: 55,
    memory: 81,
    disk: 84,
    load: "2.14",
    uptime: "91 天",
    lastSeen: "19 秒前",
    alerts: 2,
    files: 6,
    ai: 2,
    kb: 1,
  },
  {
    id: "huawei-test-01",
    accountId: "huawei-test",
    provider: "huawei",
    region: "华东-上海一",
    zone: "cn-east-3a",
    instanceId: "ecs-hw-test-01",
    name: "hw-test-runner",
    business: "自动化测试",
    publicIp: "124.70.16.89",
    privateIp: "10.66.2.33",
    os: "Ubuntu 22.04",
    spec: "2C4G",
    chargeType: "PostPaid",
    status: "maintenance",
    agentStatus: "stale",
    cpu: 8,
    memory: 29,
    disk: 58,
    load: "0.10",
    uptime: "维护中",
    lastSeen: "2 小时前",
    alerts: 1,
    files: 2,
    ai: 0,
    kb: 0,
  },
  {
    id: "tencent-lab-01",
    accountId: "tencent-lab",
    provider: "tencent",
    region: "广州",
    zone: "ap-guangzhou-6",
    instanceId: "ins-lab-01",
    name: "lab-observer-01",
    business: "实验观察",
    publicIp: "43.139.21.7",
    privateIp: "10.70.1.10",
    os: "Debian 12",
    spec: "2C2G",
    chargeType: "PrePaid",
    expiredAt: "2026-06-02T00:00:00Z",
    status: "running",
    agentStatus: "online",
    cpu: 16,
    memory: 36,
    disk: 27,
    load: "0.31",
    uptime: "5 天",
    lastSeen: "28 秒前",
    alerts: 0,
    files: 8,
    ai: 1,
    kb: 0,
  },
];

const syncTasks: SyncTask[] = [
  {
    id: "sync-aliyun-prod",
    accountId: "aliyun-prod-cn",
    name: "同步 ECS/RDS 实例、地域、安全组",
    status: "完成",
    progress: "100%",
    updatedAt: "13:18:22",
  },
  {
    id: "sync-aliyun-hk",
    accountId: "aliyun-hk",
    name: "同步香港账号 ECS/RDS 与监控采样",
    status: "完成",
    progress: "100%",
    updatedAt: "13:16:04",
  },
  {
    id: "sync-huawei-prod",
    accountId: "huawei-prod",
    name: "同步华为云 ECS 与 VPC 信息",
    status: "完成",
    progress: "100%",
    updatedAt: "13:15:33",
  },
  {
    id: "sync-huawei-test",
    accountId: "huawei-test",
    name: "同步测试账号资源标签",
    status: "执行中",
    progress: "68%",
    updatedAt: "13:19:01",
  },
];

const getCloudServerStatus = (status: string): ServerStatus => {
  const normalized = status.trim().toLowerCase();
  if (normalized === "running" || normalized === "active") return "running";
  if (normalized === "stopped" || normalized === "deleted" || normalized === "shutoff" || normalized === "shelved" || normalized === "shelved_offloaded") return "offline";
  if (["starting", "stopping", "build", "reboot", "hard_reboot", "migrating", "resize", "revert_resize", "verify_resize"].includes(normalized)) {
    return "maintenance";
  }
  return "warning";
};

const getCloudRDSStatus = (status: string): ServerStatus => {
  const normalized = status.trim().toLowerCase();
  if (normalized === "running" || normalized === "active" || normalized === "normal") return "running";
  if (normalized === "stopped" || normalized === "deleted" || normalized === "released" || normalized === "shutdown") return "offline";
  if (["creating", "build", "rebooting", "restoring", "backingup", "migrating", "classchanging", "netmodifying"].includes(normalized)) {
    return "maintenance";
  }
  return "warning";
};

const formatUptimeFromCreatedAt = (createdAt: string, status: string) => {
  if (getCloudServerStatus(status) !== "running") return "--";
  const created = new Date(createdAt);
  if (Number.isNaN(created.getTime())) return "--";
  const diffMs = Date.now() - created.getTime();
  if (diffMs <= 0) return "--";
  const days = Math.floor(diffMs / 86_400_000);
  if (days > 0) return `${days} 天`;
  const hours = Math.floor(diffMs / 3_600_000);
  if (hours > 0) return `${hours} 小时`;
  const minutes = Math.max(1, Math.floor(diffMs / 60_000));
  return `${minutes} 分钟`;
};

const formatMemorySpec = (memoryMb: number) => {
  if (!memoryMb || memoryMb <= 0) return "内存未知";
  if (memoryMb >= 1024) return `${Math.round(memoryMb / 1024)}G`;
  return `${memoryMb}M`;
};

const formatStorageSpec = (storageGb?: number, storageType?: string) => {
  const storageText = storageGb && storageGb > 0 ? `${storageGb}GB` : "存储未知";
  return storageType?.trim() ? `${storageText} / ${storageType.trim()}` : storageText;
};

const formatRDSCpuSpec = (instance: AliyunRDSInstance) => {
  if (instance.cpu && instance.cpu > 0) return `${instance.cpu}C`;
  if (instance.cpuRaw?.trim()) return `${instance.cpuRaw.trim()}C`;
  return "CPU未知";
};

const formatPercentValue = (value: number) => `${value.toFixed(2)}%`;

const resolvePublicIp = (instance: AliyunInstance): Pick<CloudServer, "publicIp" | "publicIpType" | "publicIpId"> => {
  const eipAddress = instance.eipAddress?.trim();
  if (eipAddress) {
    return { publicIp: eipAddress, publicIpType: "eip", publicIpId: instance.eipId?.trim() || "" };
  }
  const publicIp = instance.publicIps?.find((ip) => ip.trim())?.trim();
  if (publicIp) {
    return { publicIp, publicIpType: "public", publicIpId: "" };
  }
  return { publicIp: "-", publicIpType: "none", publicIpId: "" };
};

const publicIpLabel = (server: CloudServer) => {
  if (getResourceType(server) === "rds") return "连接地址";
  return server.publicIpType === "eip" ? "弹性公网 IP" : "公网";
};

const renderResourceAddressCell = (server: CloudServer) => {
  const resourceType = getResourceType(server);
  if (resourceType === "rds") {
    return <span>{server.publicIp}</span>;
  }
  return (
    <>
      <span>{publicIpLabel(server)}：{server.publicIp}</span>
      {server.publicIpType === "eip" && server.publicIpId ? <span>EIP ID：{server.publicIpId}</span> : null}
      <span>内网：{server.privateIp}</span>
    </>
  );
};

const isSpotServer = (source: { isSpot?: boolean; spotStrategy?: string }) => {
  const spotStrategy = (source.spotStrategy ?? "").trim().toLowerCase();
  return Boolean(source.isSpot) || (spotStrategy !== "" && spotStrategy !== "nospot");
};

const formatChargeType = (chargeType?: string, isSpot?: boolean) => {
  if (isSpot) return "抢占式实例";
  const normalized = (chargeType ?? "").trim().toLowerCase();
  if (normalized === "prepaid") return "包年包月";
  if (normalized === "postpaid") return "按量付费";
  return chargeType?.trim() || "--";
};

const PLACEHOLDER_EXPIRATION_DAYS = 20 * 365;

const isPlaceholderExpiration = (expiredAt?: string, expiresInDays?: number) => {
  if (typeof expiresInDays === "number" && expiresInDays >= PLACEHOLDER_EXPIRATION_DAYS) return true;
  const date = new Date(expiredAt ?? "");
  if (Number.isNaN(date.getTime())) return false;
  if (date.getFullYear() >= 2099) return true;
  const diffMs = date.getTime() - Date.now();
  return diffMs > PLACEHOLDER_EXPIRATION_DAYS * 86_400_000;
};

const calculateExpiresInDays = (expiredAt?: string) => {
  if (isPlaceholderExpiration(expiredAt)) return undefined;
  const date = new Date(expiredAt ?? "");
  if (Number.isNaN(date.getTime())) return undefined;
  const diffMs = date.getTime() - Date.now();
  if (diffMs <= 0) return 0;
  return Math.floor(diffMs / 86_400_000);
};

const normalizeExpirationStatus = (
  status?: string,
  chargeType?: string,
  isSpot?: boolean,
  expiresInDays?: number
): ExpirationStatus => {
  if (status === "normal" || status === "expiring" || status === "expired" || status === "no_expiration" || status === "unknown") {
    return status;
  }
  if (isSpot) return "no_expiration";
  if ((chargeType ?? "").trim().toLowerCase() === "postpaid") return "no_expiration";
  if (typeof expiresInDays === "number") {
    if (expiresInDays <= 0) return "expired";
    if (expiresInDays <= 30) return "expiring";
    return "normal";
  }
  return "unknown";
};

const formatExpirationDate = (expiredAt?: string) => {
  const date = new Date(expiredAt ?? "");
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit" });
};

const resolveExpirationInfo = (source: {
  chargeType?: string;
  isSpot?: boolean;
  spotStrategy?: string;
  expiredAt?: string;
  expiresInDays?: number;
  expirationStatus?: string;
  expirationMessage?: string;
  expirationText?: string;
}) => {
  const expiresInDays = typeof source.expiresInDays === "number" ? source.expiresInDays : calculateExpiresInDays(source.expiredAt);
  const isSpot = isSpotServer(source);
  const isPlaceholder = isPlaceholderExpiration(source.expiredAt, source.expiresInDays);
  const status = isPlaceholder ? "no_expiration" : normalizeExpirationStatus(source.expirationStatus, source.chargeType, isSpot, expiresInDays);
  const message = isPlaceholder ? "云厂商返回远期占位时间，视为无固定到期日" : source.expirationText || source.expirationMessage || "";
  const chargeText = formatChargeType(source.chargeType, isSpot);
  const text =
    status === "expired"
      ? "已到期"
      : status === "no_expiration"
        ? chargeText === "--" ? "无固定到期" : chargeText
        : status === "unknown"
          ? message || "未返回到期时间"
          : typeof expiresInDays === "number" && expiresInDays === 0 && message
            ? message
            : typeof expiresInDays === "number"
            ? `剩余 ${expiresInDays} 天`
            : message || "未返回到期时间";
  return {
    status,
    expiresInDays,
    text,
    dateText: formatExpirationDate(source.expiredAt),
    chargeText,
    message,
  };
};

const compactExpirationText = (expiration: ReturnType<typeof resolveExpirationInfo>) => {
  if (expiration.status === "unknown") return "未知";
  if (expiration.status === "no_expiration") return "无固定";
  if (expiration.status === "expired") return "已到期";
  if (typeof expiration.expiresInDays === "number") return `${expiration.expiresInDays}天`;
  return expiration.text || "--";
};

const metricSortLabels: Record<ServerSortKey, string> = {
  cpu: "CPU",
  memory: "内存",
  disk: "磁盘",
  expiration: "到期",
};

const expirationSortValue = (server: CloudServer) => {
  const expiration = resolveExpirationInfo(server);
  if (typeof expiration.expiresInDays === "number" && expiration.status !== "no_expiration" && expiration.status !== "unknown") {
    return { rank: 0, value: expiration.expiresInDays };
  }
  if (expiration.status === "no_expiration") return { rank: 1, value: Number.POSITIVE_INFINITY };
  return { rank: 2, value: Number.POSITIVE_INFINITY };
};

const sortServersByMetric = (
  rows: CloudServer[],
  metricSort: { key: ServerSortKey; direction: SortDirection } | null
) => {
  if (!metricSort) return rows;
  return [...rows].sort((left, right) => {
    if (metricSort.key === "expiration") {
      const leftExpiration = expirationSortValue(left);
      const rightExpiration = expirationSortValue(right);
      if (leftExpiration.rank !== rightExpiration.rank) return leftExpiration.rank - rightExpiration.rank;
      const result = leftExpiration.value - rightExpiration.value;
      if (result !== 0) return metricSort.direction === "asc" ? result : -result;
      return left.name.localeCompare(right.name, "zh-CN");
    }
    const leftMetric = left[metricSort.key];
    const rightMetric = right[metricSort.key];
    if (typeof leftMetric !== "number" && typeof rightMetric !== "number") return left.name.localeCompare(right.name, "zh-CN");
    if (typeof leftMetric !== "number") return 1;
    if (typeof rightMetric !== "number") return -1;
    const result = leftMetric - rightMetric;
    if (result !== 0) return metricSort.direction === "asc" ? result : -result;
    return left.name.localeCompare(right.name, "zh-CN");
  });
};

const buildResourceProductTree = (
  rows: CloudServer[],
  accounts: CloudAccount[],
  metricSort: { key: ServerSortKey; direction: SortDirection } | null
): ResourceProductNode[] => {
  const accountMap = new Map(accounts.map((account) => [account.id, account]));
  const typeOrder: Record<ResourceType, number> = { ecs: 0, rds: 1 };
  return resourceTypes
    .map((resourceType) => {
      const productRows = rows.filter((server) => getResourceType(server) === resourceType);
      const providers = (["aliyun", "huawei", "tencent"] as Provider[])
        .map((provider) => {
          const providerRows = productRows.filter((server) => server.provider === provider);
          const accountIDs = [...new Set(providerRows.map((server) => server.accountId))];
          const accountNodes = accountIDs
            .map((accountId) => {
              const account = accountMap.get(accountId);
              const accountRows = providerRows.filter((server) => server.accountId === accountId);
              return {
                key: `${resourceType}:${provider}:${accountId}`,
                resourceType,
                provider,
                accountId,
                accountName: account?.name ?? account?.alias ?? "未匹配账号",
                rows: sortServersByMetric(accountRows, metricSort),
              };
            })
            .sort((left, right) => left.accountName.localeCompare(right.accountName, "zh-CN"));
          return {
            key: `${resourceType}:${provider}`,
            resourceType,
            provider,
            rows: providerRows,
            accounts: accountNodes,
          };
        })
        .filter((provider) => provider.rows.length > 0);
      return {
        key: resourceType,
        resourceType,
        rows: productRows,
        providers,
      };
    })
    .filter((product) => product.rows.length > 0)
    .sort((left, right) => typeOrder[left.resourceType] - typeOrder[right.resourceType]);
};

const mapApiCloudAccount = (account: ApiCloudAccount): CloudAccount => {
  const provider = supportedAccountProviders.includes(account.provider) ? account.provider : "aliyun";
  const status = !account.enabled
    ? "disabled"
    : account.lastCheckStatus === "ok"
      ? "normal"
      : account.lastCheckStatus
        ? "warning"
        : "syncing";
  return {
    id: String(account.id),
    provider,
    name: account.name,
    alias: `${providerLabels[provider]} / ${account.name}`,
    uid: account.accessKeyIdMasked || "--",
    owner: "控制台配置",
    scope: account.enabled ? "ECS/RDS 资源与指标" : "已停用",
    status,
    lastSync: formatDisplayTime(account.lastCheckedAt),
    regions: account.regions ?? [],
    enabled: account.enabled,
    metricPeriod: account.metricPeriod || "60",
    checkMessage: account.lastCheckMessage || "",
  };
};

const mapAliyunServer = (account: CloudAccount, instance: AliyunInstance, overview?: AliyunOverviewResponse): CloudServer => {
  const cpuSpec = instance.cpu > 0 ? `${instance.cpu}C` : "CPU未知";
  const serverStatus = getCloudServerStatus(instance.status);
  const publicIpInfo = resolvePublicIp(instance);
  const expiration = resolveExpirationInfo(instance);
  const cpuPoint = latestPoint(overview?.metrics?.cpu?.points);
  const memoryPoint = latestPoint(overview?.metrics?.memory?.points);
  const diskPoint = latestPoint(overview?.metrics?.disk?.points);
  const loadPoint = latestPoint(overview?.metrics?.load1m?.points);
  const freshness = resolveOverviewFreshness(overview);
  const cloudMonitorStatus: AgentStatus =
    serverStatus === "offline"
      ? "offline"
      : freshness.status;
  const lastSeen =
    serverStatus === "offline"
      ? "实例已离线，探针不可用"
      : freshness.message || overview?.message || (overview?.availableMetricCount ? "云监控已同步" : "等待云监控数据");
  return {
    id: `${account.id}:${instance.id}`,
    accountId: account.id,
    provider: account.provider,
    region: instance.regionId || "--",
    zone: instance.zoneId || "--",
    instanceId: instance.id,
    name: instance.name || instance.hostName || instance.id,
    business: `${providerLabels[account.provider]} ECS`,
    ...publicIpInfo,
    privateIp: instance.privateIps?.[0] ?? "-",
    os: instance.osName || instance.osType || "--",
    spec: `${cpuSpec}${formatMemorySpec(instance.memoryMb)}`,
    chargeType: instance.chargeType || "",
    isSpot: instance.isSpot,
    spotStrategy: instance.spotStrategy || "",
    createdAt: instance.createdAt || "",
    expiredAt: instance.expiredAt || "",
    expiresInDays: expiration.expiresInDays,
    expirationStatus: expiration.status,
    expirationText: expiration.text,
    status: serverStatus,
    agentStatus: cloudMonitorStatus,
    cpu: clampPercentMetric(cpuPoint?.value),
    memory: clampPercentMetric(memoryPoint?.value),
    disk: clampPercentMetric(diskPoint?.value),
    load: loadPoint ? loadPoint.value.toFixed(2) : "--",
    uptime: formatUptimeFromCreatedAt(instance.createdAt, instance.status),
    lastSeen,
    alerts: 0,
    files: 0,
    ai: 0,
    kb: 0,
  };
};

const metricIdentity = (series?: AliyunMetricSeries) =>
  [series?.metricName, series?.subKey, series?.label, series?.valueFormat]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();

const latestRDSMetricValue = (overview: AliyunOverviewResponse | undefined, includes: string[], excludes: string[] = []) => {
  const normalizedIncludes = includes.map((item) => item.toLowerCase());
  const normalizedExcludes = excludes.map((item) => item.toLowerCase());
  let selected: { series: AliyunMetricSeries; point: AliyunMetricPoint } | undefined;
  for (const series of Object.values(overview?.metrics ?? {})) {
    const identity = metricIdentity(series);
    if (!normalizedIncludes.some((item) => identity.includes(item))) continue;
    if (normalizedExcludes.some((item) => identity.includes(item))) continue;
    const point = latestPoint(series?.points);
    if (!point) continue;
    if (!selected || point.timestamp > selected.point.timestamp) {
      selected = { series, point };
    }
  }
  return selected;
};

const isPercentSeries = (series: AliyunMetricSeries) => {
  const unit = series.unit?.trim().toLowerCase() ?? "";
  const identity = metricIdentity(series);
  return unit.includes("%") || unit.includes("percent") || identity.includes("percent") || identity.includes("ratio");
};

const isStorageSizeSeries = (series: AliyunMetricSeries) => {
  const unit = series.unit?.trim().toLowerCase() ?? "";
  return (
    unit.includes("byte") ||
    unit.includes("kb") ||
    unit.includes("mb") ||
    unit.includes("gb") ||
    unit.includes("kilobyte") ||
    unit.includes("megabyte") ||
    unit.includes("gigabyte")
  );
};

const isRDSStorageTotalSubKey = (series: AliyunMetricSeries) => {
  const identity = metricIdentity(series);
  return ["ins_size", "instance_size", "total_size", "totalspace", "total_space"].some((key) => identity.includes(key));
};

const rdsStoragePriority = (series: AliyunMetricSeries) => {
  const identity = metricIdentity(series);
  if (isRDSStorageTotalSubKey(series)) return 0;
  if (identity.includes("data_size") || identity.includes("datausage") || identity.includes("data_space")) return 1;
  if (identity.includes("used") || identity.includes("usage")) return 2;
  return 3;
};

const latestRDSStorageSizeMetric = (overview: AliyunOverviewResponse | undefined) => {
  let selected: { series: AliyunMetricSeries; point: AliyunMetricPoint; priority: number } | undefined;
  for (const series of Object.values(overview?.metrics ?? {})) {
    const identity = metricIdentity(series);
    if (!identity.includes("spaceusage") && !identity.includes("diskusage")) continue;
    if (identity.includes("iops") || identity.includes("mbps") || identity.includes("rate")) continue;
    if (isPercentSeries(series) || !isStorageSizeSeries(series)) continue;
    const point = latestPoint(series.points);
    if (!point) continue;
    const priority = rdsStoragePriority(series);
    if (!selected || priority < selected.priority || (priority === selected.priority && point.timestamp > selected.point.timestamp)) {
      selected = { series, point, priority };
    }
  }
  return selected;
};

const rdsStorageValueToMb = (series: AliyunMetricSeries, value: number) => {
  const unit = series.unit?.trim().toLowerCase() ?? "";
  if (unit.includes("gb") || unit.includes("gbyte") || unit.includes("gigabyte")) return value * 1024;
  if (unit.includes("kb") || unit.includes("kbyte") || unit.includes("kilobyte")) return value / 1024;
  if (unit.includes("byte") && !unit.includes("mbyte") && !unit.includes("megabyte")) return value / 1024 / 1024;
  return value;
};

const resolveRDSStorageUsage = (overview: AliyunOverviewResponse | undefined, instance: AliyunRDSInstance): RDSStorageUsage => {
  if (typeof instance.resourceUsage?.storageUsagePercent === "number") {
    return {
      percent: clampPercentMetric(instance.resourceUsage.storageUsagePercent),
      metricName: instance.resourceUsage.source || "rds.DescribeResourceUsage",
      subKey: "DiskUsed",
      unit: "%",
      rawValue: instance.resourceUsage.diskUsedBytes,
      storageGb: instance.storageGb,
      source: "official",
    };
  }
  const direct = latestRDSMetricValue(overview, ["diskusage", "storageusage", "spaceusage_percent"], ["iops", "mbps"]);
  if (direct && isPercentSeries(direct.series)) {
    return {
      percent: clampPercentMetric(direct.point.value),
      metricName: direct.series.metricName,
      subKey: direct.series.subKey,
      unit: direct.series.unit,
      rawValue: direct.point.value,
      storageGb: instance.storageGb,
      source: "percent",
    };
  }
  const usedSpace = latestRDSStorageSizeMetric(overview);
  if (usedSpace && instance.storageGb && instance.storageGb > 0) {
    const usedMb = rdsStorageValueToMb(usedSpace.series, usedSpace.point.value);
    return {
      percent: clampPercentMetric((usedMb / (instance.storageGb * 1024)) * 100),
      metricName: usedSpace.series.metricName,
      subKey: usedSpace.series.subKey,
      unit: usedSpace.series.unit,
      rawValue: usedSpace.point.value,
      storageGb: instance.storageGb,
      source: "size",
    };
  }
  return { storageGb: instance.storageGb, source: "missing" };
};

const mapCloudRDSResource = (account: CloudAccount, instance: AliyunRDSInstance, overview?: AliyunOverviewResponse): CloudServer => {
  const serverStatus = getCloudRDSStatus(instance.status);
  const expiration = resolveExpirationInfo({
    chargeType: instance.payType,
    expiredAt: instance.expiredAt,
    expiresInDays: instance.expiresInDays,
    expirationStatus: instance.expirationStatus,
    expirationMessage: instance.expirationMessage,
  });
  const freshness = resolveOverviewFreshness(overview);
  const cpuMetric = latestRDSMetricValue(overview, ["cpu"], ["proxy"]);
  const memoryMetric = latestRDSMetricValue(overview, ["mem", "memory"], ["proxy"]);
  const qpsMetric = latestRDSMetricValue(overview, ["qps"]);
  const storageUsage = resolveRDSStorageUsage(overview, instance);
  const primaryEndpoint = instance.endpoints?.find((endpoint) => endpoint.connectionString?.trim()) ?? instance.endpoints?.[0];
  const connectionString = instance.connectionString?.trim() || primaryEndpoint?.connectionString?.trim() || "-";
  const port = instance.port?.trim() || primaryEndpoint?.port?.trim() || "";
  const storageText = formatStorageSpec(instance.storageGb, instance.storageType);
  const classText = instance.class?.trim() || `${formatRDSCpuSpec(instance)}${formatMemorySpec(instance.memoryMb ?? 0)}`;
  const lastSeen =
    serverStatus === "offline"
      ? "实例已离线，性能数据不可用"
      : freshness.message || overview?.message || (overview?.availableMetricCount ? "RDS 性能数据已同步" : "等待 RDS 性能数据");
  return {
    id: `${account.id}:rds:${instance.id}`,
    accountId: account.id,
    provider: account.provider,
    resourceType: "rds",
    region: instance.regionId || "--",
    zone: instance.zoneId || "--",
    instanceId: instance.id,
    name: instance.name || instance.id,
    business: `${instance.engine || "RDS"} 数据库`,
    publicIp: port ? `${connectionString}:${port}` : connectionString,
    publicIpType: "none",
    publicIpId: "",
    privateIp: primaryEndpoint?.ipAddress || instance.vpcId || "-",
    os: [instance.engine, instance.engineVersion].filter(Boolean).join(" ") || "--",
    spec: `${classText} / ${storageText}`,
    chargeType: instance.payType || "",
    createdAt: instance.createdAt || "",
    expiredAt: instance.expiredAt || "",
    expiresInDays: expiration.expiresInDays,
    expirationStatus: expiration.status,
    expirationText: expiration.text,
    engine: instance.engine,
    engineVersion: instance.engineVersion,
    dbInstanceType: instance.type,
    dbCategory: instance.category,
    dbClass: instance.class,
    dbStorageGb: instance.storageGb,
    dbStorageType: instance.storageType,
    dbConnectionString: connectionString,
    dbPort: port,
    dbMaxConnections: instance.maxConnections,
    dbMaxIops: instance.maxIops,
    dbMaxIombps: instance.maxIombps,
    dbEndpointCount: instance.endpoints?.length ?? 0,
    dbLockMode: instance.lockMode,
    dbLockReason: instance.lockReason,
    status: serverStatus,
    agentStatus: serverStatus === "offline" ? "offline" : freshness.status,
    cpu: clampPercentMetric(cpuMetric?.point.value),
    memory: clampPercentMetric(memoryMetric?.point.value),
    disk: storageUsage.percent,
    load: qpsMetric ? qpsMetric.point.value.toFixed(2) : "--",
    uptime: formatUptimeFromCreatedAt(instance.createdAt, instance.status),
    lastSeen,
    alerts: 0,
    files: 0,
    ai: 0,
    kb: 0,
  };
};

const monitorMetricMeta: Array<{ key: MonitorMetricKey; label: string; unit: string }> = [
  { key: "cpu", label: "CPU", unit: "%" },
  { key: "memory", label: "内存", unit: "%" },
  { key: "disk", label: "磁盘", unit: "%" },
  { key: "load1m", label: "1 分钟负载", unit: "" },
  { key: "internetIn", label: "公网入", unit: "bit/s" },
  { key: "internetOut", label: "公网出", unit: "bit/s" },
  { key: "internetTotal", label: "公网总带宽", unit: "bit/s" },
  { key: "internetBandwidth", label: "公网带宽", unit: "bit/s" },
  { key: "intranetIn", label: "内网入", unit: "bit/s" },
  { key: "intranetOut", label: "内网出", unit: "bit/s" },
  { key: "intranetTotal", label: "内网总带宽", unit: "bit/s" },
  { key: "intranetBandwidth", label: "内网带宽", unit: "bit/s" },
  { key: "networkTotal", label: "总带宽", unit: "bit/s" },
  { key: "diskReadBps", label: "磁盘读速率", unit: "B/s" },
  { key: "diskWriteBps", label: "磁盘写速率", unit: "B/s" },
];

const formatRateValue = (value: number, unit: "bit/s" | "B/s") => {
  if (unit === "bit/s") {
    if (value >= 1000 * 1000) return `${(value / 1000 / 1000).toFixed(2)} Mbit/s`;
    if (value >= 1000) return `${(value / 1000).toFixed(2)} Kbit/s`;
    return `${value.toFixed(2)} bit/s`;
  }
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(2)} MB/s`;
  if (value >= 1024) return `${(value / 1024).toFixed(2)} KB/s`;
  return `${value.toFixed(2)} B/s`;
};

const normalizeMetricUnit = (unit?: string) => {
  const raw = (unit ?? "").trim();
  const lower = raw.toLowerCase();
  if (!raw) return "";
  if (lower === "bit/s" || lower === "bits/s" || lower === "bps") return "bit/s";
  if (lower === "byte/s" || lower === "bytes/s" || lower === "b/s") return "B/s";
  if (raw === "%") return "%";
  return raw;
};

const displayMetricUnit = (series: AliyunMetricSeries | undefined, fallback: string) => normalizeMetricUnit(series?.unit) || fallback;

const isByteRateUnit = (unit: string) => unit === "B/s";
const isBitRateUnit = (unit: string) => unit === "bit/s";

const formatMetricRateByUnit = (value: number, unit: string) => {
  if (isByteRateUnit(unit)) return formatRateValue(value, "B/s");
  return formatRateValue(value, "bit/s");
};

const formatMonitorMetricValue = (key: MonitorMetricKey, value: number, unit: string) => {
  if (unit === "%") return formatPercentValue(value);
  if (key === "load1m") return value.toFixed(2);
  if (isBitRateUnit(unit) || isByteRateUnit(unit)) return formatMetricRateByUnit(value, unit);
  if (unit) return `${value.toFixed(2)} ${unit}`;
  return value.toFixed(2);
};

const latestPoint = (points?: AliyunMetricPoint[]) => {
  if (!points?.length) return undefined;
  return points.at(-1);
};

const hasMetricPoint = (series?: AliyunMetricSeries) => Boolean(latestPoint(series?.points));

const clampPercentMetric = (value?: number) => {
  if (typeof value !== "number" || Number.isNaN(value)) return undefined;
  return Math.max(0, Math.min(100, value));
};

const parseMetricPeriodSeconds = (period?: string) => {
  const parsed = Number.parseInt((period ?? "").trim(), 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
};

const summarizeMetricPoints = (points?: AliyunMetricPoint[]) => {
  if (!points?.length) return undefined;
  let min = Number.POSITIVE_INFINITY;
  let max = Number.NEGATIVE_INFINITY;
  let sum = 0;
  for (const point of points) {
    min = Math.min(min, point.value);
    max = Math.max(max, point.value);
    sum += point.value;
  }
  return {
    latest: points.at(-1),
    min,
    max,
    average: sum / points.length,
  };
};

const recentMetricPoints = (points?: AliyunMetricPoint[], limit = 8) => {
  if (!points?.length) return [];
  return points.slice(Math.max(0, points.length - limit));
};

const mergeServerRowsWithPrevious = (
  freshRows: CloudServer[],
  previousRows: CloudServer[],
  overviewMap: Record<string, AliyunOverviewResponse>
) => {
  const previousMap = new Map(previousRows.map((server) => [server.id, server]));
  return freshRows.map((server) => {
    const previous = previousMap.get(server.id);
    if (!previous || server.status === "offline") return server;
    const overview = overviewMap[server.id];
    if (getResourceType(server) === "rds") {
      if (hasAnyMetricPoint(overview) || previous.agentStatus === "missing") return server;
      return {
        ...server,
        agentStatus: previous.agentStatus,
        cpu: previous.cpu,
        memory: previous.memory,
        disk: previous.disk,
        load: previous.load,
        lastSeen: previous.lastSeen,
      };
    }

    const metrics = overview?.metrics ?? {};
    const next = { ...server };
    let reusedPreviousMetric = false;

    if (!hasMetricPoint(metrics.cpu) && typeof previous.cpu === "number" && previous.cpu > 0) {
      next.cpu = previous.cpu;
      reusedPreviousMetric = true;
    }
    if (!hasMetricPoint(metrics.memory) && typeof previous.memory === "number" && previous.memory > 0) {
      next.memory = previous.memory;
      reusedPreviousMetric = true;
    }
    if (!hasMetricPoint(metrics.disk) && typeof previous.disk === "number" && previous.disk > 0) {
      next.disk = previous.disk;
      reusedPreviousMetric = true;
    }
    if (!hasMetricPoint(metrics.load1m) && previous.load !== "--") {
      next.load = previous.load;
      reusedPreviousMetric = true;
    }
    if (reusedPreviousMetric) {
      next.agentStatus = previous.agentStatus;
      next.lastSeen = previous.lastSeen || "沿用上一轮监控状态，等待本轮采样返回";
    }
    return next;
  });
};

const mergePendingServerRowsWithPrevious = (freshRows: CloudServer[], previousRows: CloudServer[]) => {
  const previousMap = new Map(previousRows.map((server) => [server.id, server]));
  return freshRows.map((server) => {
    const previous = previousMap.get(server.id);
    if (!previous || server.status === "offline") return server;
    return {
      ...server,
      agentStatus: previous.agentStatus,
      cpu: previous.cpu,
      memory: previous.memory,
      disk: previous.disk,
      load: previous.load,
      lastSeen: previous.lastSeen,
    };
  });
};

const hasAnyMetricPoint = (overview?: AliyunOverviewResponse) =>
  Object.values(overview?.metrics ?? {}).some((series) => hasMetricPoint(series));

const mergeOverviewMapWithPrevious = (
  freshMap: Record<string, AliyunOverviewResponse>,
  previousMap: Record<string, AliyunOverviewResponse>,
  freshRows: CloudServer[]
) => {
  const rowMap = new Map(freshRows.map((server) => [server.id, server]));
  return Object.fromEntries(
    Object.entries(freshMap).map(([serverId, overview]) => {
      const server = rowMap.get(serverId);
      const previous = previousMap[serverId];
      if (server?.status === "offline" || !previous) return [serverId, overview];
      if (hasAnyMetricPoint(overview) || !hasAnyMetricPoint(previous)) return [serverId, overview];
      return [
        serverId,
        {
          ...overview,
          ok: previous.ok,
          status: previous.status,
          message: "本轮暂未返回新采样，沿用上一轮监控值",
          availableMetricCount: previous.availableMetricCount,
          metrics: previous.metrics,
          errors: overview.errors,
        },
      ];
    })
  );
};

const filterOverviewMapForRows = (
  overviewMap: Record<string, AliyunOverviewResponse>,
  rows: CloudServer[]
) => {
  const rowIDs = new Set(rows.map((server) => server.id));
  return Object.fromEntries(Object.entries(overviewMap).filter(([serverId]) => rowIDs.has(serverId)));
};

const buildDerivedSeries = (metricName: string, seriesList: Array<AliyunMetricSeries | undefined>): AliyunMetricSeries | undefined => {
  const validSeries = seriesList.filter((series): series is AliyunMetricSeries => Boolean(series?.points?.length));
  if (validSeries.length !== seriesList.length || !validSeries.length) return undefined;
  const baseline = validSeries[0]?.points ?? [];
  const unit = normalizeMetricUnit(validSeries[0]?.unit);
  if (validSeries.some((series) => normalizeMetricUnit(series.unit) !== unit)) return undefined;
  const extraMaps = validSeries.slice(1).map((series) => new Map((series.points ?? []).map((point) => [point.timestamp, point.value])));
  const points = baseline.flatMap((point) => {
    let total = point.value;
    for (const extraMap of extraMaps) {
      const next = extraMap.get(point.timestamp);
      if (typeof next !== "number") return [];
      total += next;
    }
    return [{ timestamp: point.timestamp, value: total }];
  });
  if (!points.length) return undefined;
  return { namespace: "derived", metricName, unit, points };
};

const derivedMetricKeys = new Set<MonitorMetricKey>(["internetTotal", "intranetTotal", "networkTotal"]);

const pluginMetricKeys = new Set<MonitorMetricKey>(["memory", "disk", "load1m"]);

const formatMetricTime = (timestamp?: number) => {
  if (!timestamp) return "";
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString("zh-CN", { hour12: false });
};

const formatMetricShortTime = (timestamp?: number) => {
  if (!timestamp) return "";
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString("zh-CN", { hour12: false });
};

const formatAgeText = (ageMs: number) => {
  if (!Number.isFinite(ageMs) || ageMs < 0) return "";
  const totalSeconds = Math.floor(ageMs / 1000);
  if (totalSeconds < 60) return `${Math.max(1, totalSeconds)} 秒`;
  const totalMinutes = Math.floor(totalSeconds / 60);
  if (totalMinutes < 60) return `${totalMinutes} 分钟`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes > 0 ? `${hours} 小时 ${minutes} 分钟` : `${hours} 小时`;
};

const latestMetricSnapshot = (overview?: AliyunOverviewResponse) => {
  let latestTimestamp = 0;
  let maxPeriodSeconds = 0;
  let pointCount = 0;
  for (const series of Object.values(overview?.metrics ?? {})) {
    const point = latestPoint(series?.points);
    if (point?.timestamp && point.timestamp > latestTimestamp) {
      latestTimestamp = point.timestamp;
    }
    maxPeriodSeconds = Math.max(maxPeriodSeconds, parseMetricPeriodSeconds(series?.period));
    pointCount += series?.points?.length ?? 0;
  }
  return {
    latestTimestamp,
    periodSeconds: maxPeriodSeconds,
    pointCount,
  };
};

const collectMetricSeries = (overview?: AliyunOverviewResponse) =>
  Object.values(overview?.metrics ?? {}).filter((series): series is AliyunMetricSeries => Boolean(series));

const collectAvailableMetricSeries = (overview?: AliyunOverviewResponse) =>
  collectMetricSeries(overview).filter((series) => hasMetricPoint(series));

const summarizeMetricSourceSnapshots = (overview?: AliyunOverviewResponse): AgentSourceSnapshot[] => {
  const sourceMap = new Map<string, AgentSourceSnapshot>();
  collectAvailableMetricSeries(overview).forEach((series) => {
    const source = metricSourceLabel(series);
    if (!source) return;
    const row = sourceMap.get(source) ?? {
      source,
      pointCount: 0,
      latestTimestamp: 0,
      periods: [],
    };
    const points = series.points ?? [];
    const latest = latestPoint(points);
    row.pointCount += points.length;
    if (latest?.timestamp) row.latestTimestamp = Math.max(row.latestTimestamp, latest.timestamp);
    if (series.period) {
      const period = `${series.period}s`;
      if (!row.periods.includes(period)) row.periods.push(period);
    }
    sourceMap.set(source, row);
  });
  return [...sourceMap.values()];
};

const formatCoveragePercent = (covered: number, total: number) => {
  if (total <= 0) return "0%";
  return `${Math.round((covered / total) * 100)}%`;
};

const formatCompactNumber = (value: number) => value.toLocaleString("zh-CN");

const resolveOverviewFreshness = (overview?: AliyunOverviewResponse): { status: AgentStatus; message: string } => {
  const snapshot = latestMetricSnapshot(overview);
  if (!snapshot.latestTimestamp) {
    if (overview?.status === "metric_error" || overview?.error) {
      return { status: "stale", message: overview.message || overview.error || "监控接口异常，暂未拿到采样值" };
    }
    return {
      status: "missing",
      message: overview?.message || "当前未拿到监控采样，请检查云监控插件、地域和权限",
    };
  }
  const now = Date.now();
  const ageMs = Math.max(0, now - snapshot.latestTimestamp);
  const ageText = formatAgeText(ageMs);
  const sampledAt = formatMetricTime(snapshot.latestTimestamp);
  const thresholdMs = Math.max(snapshot.periodSeconds * 3 * 1000, 180_000);
  if (overview?.message?.includes("沿用上一轮监控值")) {
    return {
      status: "stale",
      message: `本轮未返回新采样，当前展示为 ${sampledAt}${ageText ? `（${ageText}前）` : ""}`,
    };
  }
  if (ageMs > thresholdMs) {
    return {
      status: "stale",
      message: `最新采样 ${sampledAt}${ageText ? `（${ageText}前，数据偏旧）` : "（数据偏旧）"}`,
    };
  }
  return {
    status: "online",
    message: `最新采样 ${sampledAt}${ageText ? `（${ageText}前）` : ""}`,
  };
};

const metricSourceLabel = (series?: AliyunMetricSeries) => {
  const namespace = series?.namespace ?? "";
  const metricName = series?.metricName ?? "";
  if (namespace === "derived") return "本页面计算";
  if (metricName.startsWith("ecs.DescribeInstanceMonitorData.")) return "ECS 基础监控";
  if (metricName.startsWith("VPC_PublicIP_")) return "弹性公网 IP 云监控";
  if (namespace === "acs_ecs_dashboard") return "云监控 ECS 指标";
  if (namespace === "SYS.ECS") return "华为云 CES ECS 指标";
  if (namespace === "AGT.ECS") return "华为云 Agent 指标";
  return namespace || "未知来源";
};

const formatMonitorMetricDelta = (key: MonitorMetricKey, value: number, unit: string) => {
  const sign = value > 0 ? "+" : value < 0 ? "-" : "";
  const abs = Math.abs(value);
  if (unit === "%") return `${sign}${abs.toFixed(2)} pct`;
  if (key === "load1m") return `${sign}${abs.toFixed(2)}`;
  if (isBitRateUnit(unit) || isByteRateUnit(unit)) return `${sign}${formatMetricRateByUnit(abs, unit)}`;
  if (unit) return `${sign}${abs.toFixed(2)} ${unit}`;
  return `${sign}${abs.toFixed(2)}`;
};

const buildMonitorMetricTrend = (key: MonitorMetricKey, unit: string, points?: AliyunMetricPoint[]) => {
  const recentPoints = recentMetricPoints(points);
  if (!recentPoints.length) return "--";
  if (recentPoints.length === 1) return "单点采样";
  const first = recentPoints[0];
  const latest = recentPoints.at(-1);
  if (!first || !latest) return "--";
  const delta = latest.value - first.value;
  if (Math.abs(delta) < 0.0001) return "近窗基本持平";
  return `${delta > 0 ? "上升" : "下降"} ${formatMonitorMetricDelta(key, delta, unit)}`;
};

const buildMonitorRecentSamples = (key: MonitorMetricKey, unit: string, points?: AliyunMetricPoint[]) =>
  recentMetricPoints(points, 6).map((point) => `${formatMetricShortTime(point.timestamp)} ${formatMonitorMetricValue(key, point.value, unit)}`);

const metricSourceDetail = (series?: AliyunMetricSeries) => {
  const source = metricSourceLabel(series);
  const metricName = series?.metricName ? [series.metricName, series.subKey].filter(Boolean).join(" / ") : "--";
  const period = series?.period ? `${series.period}s` : "--";
  return { source, metricName, period };
};

const buildMetricPointCoordinates = (points: AliyunMetricPoint[], width: number, height: number, padding = 0) => {
  if (points.length < 2) return [];
  const chartWidth = Math.max(1, width - padding * 2);
  const chartHeight = Math.max(1, height - padding * 2);
  const values = points.map((point) => point.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  return points.map((point, index) => ({
    x: padding + (index / Math.max(1, points.length - 1)) * chartWidth,
    y: padding + chartHeight - ((point.value - min) / range) * chartHeight,
  }));
};

const buildSparklinePath = (points: AliyunMetricPoint[], width = 112, height = 28, padding = 0) => {
  if (points.length < 2) return "";
  return buildMetricPointCoordinates(points, width, height, padding)
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`)
    .join(" ");
};

const buildMetricNote = (
  item: { key: MonitorMetricKey },
  latest: AliyunMetricPoint | undefined,
  series: AliyunMetricSeries | undefined,
  error?: string
) => {
  const source = metricSourceLabel(series);
  const period = series?.period ? `，周期 ${series.period}s` : "";
  const metricName = series?.metricName ? ` / ${series.metricName}` : "";
  const unit = displayMetricUnit(series, "");
  const unitText = unit ? `，单位 ${unit}` : "";
  if (latest) {
    return `${source}${metricName}${unitText}${period}`;
  }
  if (error) return error;
  if (derivedMetricKeys.has(item.key)) return "依赖的入/出方向指标没有同时返回";
  if (item.key === "disk" && series?.namespace === "SYS.ECS") return "华为云基础监控返回成功但没有磁盘采样点；请确认云服务器镜像内 UVP VMTools 正常，或安装/启用主机监控 Agent 后查看挂载点维度的磁盘指标";
  if (item.key === "disk" && series?.namespace === "AGT.ECS") return "华为云 Agent 磁盘指标通常按挂载点上报，当前实例维度没有采样点；后续可按挂载点维度扩展查询";
  if (pluginMetricKeys.has(item.key)) return "ECS 基础监控不包含该项，需云监控插件或 Agent 上报";
  if (series) return `${source}${metricName}${period}，接口返回成功但没有采样点`;
  return "云厂商未返回该指标";
};

const formatRDSMetricValue = (series: AliyunMetricSeries | undefined, value: number) => {
  const unit = series?.unit?.trim() ?? "";
  const unitLower = unit.toLowerCase();
  if (unit.includes("%")) return formatPercentValue(value);
  if (unitLower === "kb" || unitLower === "kbyte") return `${(value / 1024).toFixed(2)} MB`;
  if (unitLower === "mb" || unitLower === "mbyte") return `${value.toFixed(2)} MB`;
  if (unitLower === "gb" || unitLower === "gbyte") return `${value.toFixed(2)} GB`;
  if (unitLower.includes("kbps")) return `${value.toFixed(2)} KB/s`;
  if (unitLower.includes("mbps")) return `${value.toFixed(2)} MB/s`;
  if (unitLower.includes("second") && /qps|tps|iops|cps/i.test(`${series?.metricName ?? ""} ${series?.subKey ?? ""}`)) {
    return `${value.toFixed(2)}/s`;
  }
  if (!unit || unit === "Count") return value.toFixed(2);
  return `${value.toFixed(2)} ${unit}`;
};

const formatRDSMetricDelta = (series: AliyunMetricSeries | undefined, value: number) => {
  const sign = value > 0 ? "+" : value < 0 ? "-" : "";
  return `${sign}${formatRDSMetricValue(series, Math.abs(value))}`;
};

const buildRDSMetricTrend = (series: AliyunMetricSeries | undefined, points?: AliyunMetricPoint[]) => {
  const recentPoints = recentMetricPoints(points);
  if (!recentPoints.length) return "--";
  if (recentPoints.length === 1) return "单点采样";
  const first = recentPoints[0];
  const latest = recentPoints.at(-1);
  if (!first || !latest) return "--";
  const delta = latest.value - first.value;
  if (Math.abs(delta) < 0.0001) return "近窗基本持平";
  return `${delta > 0 ? "上升" : "下降"} ${formatRDSMetricDelta(series, delta)}`;
};

const buildRDSRecentSamples = (series: AliyunMetricSeries | undefined, points?: AliyunMetricPoint[]) =>
  recentMetricPoints(points, 6).map((point) => `${formatMetricShortTime(point.timestamp)} ${formatRDSMetricValue(series, point.value)}`);

const buildRDSMetricNote = (series: AliyunMetricSeries | undefined, latest: AliyunMetricPoint | undefined, error?: string) => {
  if (error) return error;
  const key = series?.metricName ?? "未知指标";
  const subKey = series?.subKey ? ` / ${series.subKey}` : "";
  const unit = series?.unit ? `，单位 ${series.unit}` : "";
  const valueFormat = series?.valueFormat ? `，格式 ${series.valueFormat}` : "";
  if (latest) return `RDS 性能参数 ${key}${subKey}${unit}${valueFormat}`;
  return `RDS 性能参数 ${key}${subKey} 暂无采样点`;
};

const buildRDSMonitorRows = (payload?: AliyunOverviewResponse): MonitorMetric[] => {
  const rows = Object.entries(payload?.metrics ?? {})
    .sort(([, left], [, right]) => `${left.metricName}.${left.subKey ?? ""}`.localeCompare(`${right.metricName}.${right.subKey ?? ""}`))
    .map(([key, series]) => {
      const points = series?.points ?? [];
      const summary = summarizeMetricPoints(points);
      const latest = summary?.latest;
      return {
        key,
        label: series.label || [series.metricName, series.subKey].filter(Boolean).join(" / "),
        unit: series.unit || "无单位",
        ...metricSourceDetail(series),
        value: latest ? formatRDSMetricValue(series, latest.value) : "--",
        range: summary ? `${formatRDSMetricValue(series, summary.min)} ~ ${formatRDSMetricValue(series, summary.max)}` : "--",
        average: summary ? formatRDSMetricValue(series, summary.average) : "--",
        sampledAt: formatMetricTime(latest?.timestamp) || "--",
        trend: buildRDSMetricTrend(series, points),
        sparklinePoints: points,
        recentSamples: buildRDSRecentSamples(series, points),
        points: points.length,
        status: latest ? "ok" : "empty",
        note: buildRDSMetricNote(series, latest),
      } satisfies MonitorMetric;
    });
  const existingMetricNames = new Set(rows.map((row) => String(row.key).split(".")[0]));
  const errorRows = Object.entries(payload?.errors ?? {})
    .filter(([key]) => !existingMetricNames.has(key))
    .map(([key, error]) => ({
      key,
      label: key,
      unit: "无单位",
      source: "RDS 性能接口",
      metricName: key,
      period: "--",
      value: "--",
      range: "--",
      average: "--",
      sampledAt: "--",
      trend: "--",
      sparklinePoints: [],
      recentSamples: [],
      points: 0,
      status: "error" as const,
      note: buildRDSMetricNote(undefined, undefined, error),
    }));
  return [...rows, ...errorRows];
};

const buildMonitorRows = (payload?: AliyunOverviewResponse): MonitorMetric[] => {
  const metrics: Record<string, AliyunMetricSeries | undefined> = {
    ...(payload?.metrics ?? {}),
  };
  metrics.internetTotal = buildDerivedSeries("InternetInRate + InternetOutRate", [metrics.internetIn, metrics.internetOut]);
  metrics.intranetTotal = buildDerivedSeries("IntranetInRate + IntranetOutRate", [metrics.intranetIn, metrics.intranetOut]);
  metrics.networkTotal = buildDerivedSeries("公网 + 内网总带宽", [
    metrics.internetIn,
    metrics.internetOut,
    metrics.intranetIn,
    metrics.intranetOut,
  ]);
  return monitorMetricMeta.map((item) => {
    const series = metrics[item.key];
    const unit = displayMetricUnit(series, item.unit);
    const points = series?.points ?? [];
    const summary = summarizeMetricPoints(points);
    const latest = summary?.latest;
    const error = payload?.errors?.[item.key];
    const detail = metricSourceDetail(series);
    return {
      ...item,
      unit,
      ...detail,
      value: latest ? formatMonitorMetricValue(item.key, latest.value, unit) : "--",
      range: summary ? `${formatMonitorMetricValue(item.key, summary.min, unit)} ~ ${formatMonitorMetricValue(item.key, summary.max, unit)}` : "--",
      average: summary ? formatMonitorMetricValue(item.key, summary.average, unit) : "--",
      sampledAt: formatMetricTime(latest?.timestamp) || "--",
      trend: buildMonitorMetricTrend(item.key, unit, points),
      sparklinePoints: points,
      recentSamples: buildMonitorRecentSamples(item.key, unit, points),
      points: points.length,
      status: latest ? "ok" : error ? "error" : "empty",
      note: buildMetricNote(item, latest, series, error),
    };
  });
};

const buildScopeKey = (kind: ScopeKind, value = "") => `${kind}:${value}`;

const parseScope = (scope: string): { kind: ScopeKind; value: string } => {
  const [kind, ...rest] = scope.split(":");
  if (
    kind === "product" ||
    kind === "productProvider" ||
    kind === "productAccount" ||
    kind === "productAccountRegion" ||
    kind === "region" ||
    kind === "server"
  ) {
    return { kind, value: rest.join(":") };
  }
  return { kind: "all", value: "" };
};

const serverMatchesScope = (server: CloudServer, parsedScope: { kind: ScopeKind; value: string }) => {
  const resourceType = getResourceType(server);
  if (parsedScope.kind === "product") return resourceType === parsedScope.value;
  if (parsedScope.kind === "productProvider") return `${resourceType}:${server.provider}` === parsedScope.value;
  if (parsedScope.kind === "productAccount") return `${resourceType}:${server.provider}:${server.accountId}` === parsedScope.value;
  if (parsedScope.kind === "productAccountRegion") return `${resourceType}:${server.provider}:${server.accountId}:${server.region}` === parsedScope.value;
  if (parsedScope.kind === "region") return `${server.provider}:${server.region}` === parsedScope.value;
  if (parsedScope.kind === "server") return server.id === parsedScope.value;
  return true;
};

const buildCloudAssetIssue = (
  account: CloudAccount,
  resourceType: ResourceType,
  scope: string,
  message: string
): CloudAssetIssue => ({
  id: `${account.id}:${resourceType}:${scope}:${message}`,
  provider: account.provider,
  accountId: account.id,
  accountName: account.name,
  resourceType,
  scope,
  message,
});

const isCloudAssetIssue = (value: unknown): value is CloudAssetIssue => {
  if (!value || typeof value !== "object") return false;
  const issue = value as Partial<CloudAssetIssue>;
  return typeof issue.accountName === "string" && typeof issue.resourceType === "string" && typeof issue.message === "string";
};

const summarizeCloudAssetIssues = (issues: CloudAssetIssue[]) => {
  if (!issues.length) return "";
  const first = issues[0];
  const firstText = `${first.accountName} ${resourceTypeLabels[first.resourceType]} ${first.scope}: ${first.message}`;
  return issues.length === 1 ? firstText : `${firstText}，另有 ${issues.length - 1} 项局部问题`;
};

const formatDisplayTime = (raw?: string) => {
  if (!raw) return "--";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString("zh-CN", { hour12: false });
};

const alertSeverityLabels: Record<CloudAlertSeverity, string> = {
  info: "提示",
  warning: "预警",
  critical: "严重",
};

const alertSeverityRank: Record<CloudAlertSeverity, number> = {
  critical: 0,
  warning: 1,
  info: 2,
};

const healthLevelLabels: Record<ResourceHealthLevel, string> = {
  healthy: "健康",
  watch: "关注",
  risk: "风险",
  critical: "高危",
};

const healthLevelStatus: Record<ResourceHealthLevel, string> = {
  healthy: "normal",
  watch: "warning",
  risk: "warning",
  critical: "critical",
};

const alertSeverityPenalty: Record<CloudAlertSeverity, number> = {
  critical: 35,
  warning: 18,
  info: 8,
};

const daysSince = (raw?: string) => {
  if (!raw) return null;
  const timestamp = new Date(raw).getTime();
  if (Number.isNaN(timestamp)) return null;
  return Math.max(0, Math.floor((Date.now() - timestamp) / 86400000));
};

const isPostPaidCharge = (chargeType?: string) => {
  const normalized = (chargeType ?? "").trim().toLowerCase();
  return normalized.includes("postpaid") || normalized.includes("post_paid") || normalized.includes("按量");
};

const calculateResourceHealthScore = (server: CloudServer, reminders: CloudAlertReminder[]): ResourceHealthScore => {
  const penalty = reminders.reduce((total, reminder) => total + alertSeverityPenalty[reminder.severity], 0);
  const score = Math.max(0, Math.min(100, 100 - penalty));
  const reasons = reminders.map((item) => item.title);
  if (!reasons.length && server.agentStatus !== "online") {
    reasons.push(`监控状态${agentStatusLabels[server.agentStatus]}`);
  }
  const level: ResourceHealthLevel =
    score < 50 ? "critical" :
    score < 70 ? "risk" :
    score < 90 ? "watch" :
    "healthy";
  return { score, level, reasons: [...new Set(reasons)].slice(0, 3) };
};

const cloudAlertRules: CloudAlertRule[] = [
  {
    id: "resource-status",
    title: "实例运行状态异常",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "资源状态",
    condition: "非运行中触发",
    severity: "critical",
    enabled: true,
    notifyChannels: ["控制台", "后续可接入钉钉/邮件"],
    evaluate: (server, config) => {
      if (server.status === "running") return null;
      const resourceType = getResourceType(server);
      const severity: CloudAlertSeverity = server.status === "offline" ? "critical" : config.severity;
      return {
        severity,
        currentValue: serverStatusLabels[server.status],
        condition: "状态不是运行中",
        message: `${resourceTypeLabels[resourceType]} ${server.name} 当前为${serverStatusLabels[server.status]}`,
        suggestion: server.status === "offline" ? "先确认实例是否被关停、释放或云厂商状态异常" : "关注变配、重启、迁移或锁定中的实例状态",
      };
    },
  },
  {
    id: "metric-cpu",
    title: "CPU 使用率过高",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "CPU",
    condition: ">= 75% 预警，>= 85% 严重",
    severity: "warning",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      const warningThreshold = config.warningThreshold ?? 75;
      const criticalThreshold = config.criticalThreshold ?? 85;
      if (typeof server.cpu !== "number" || server.cpu < warningThreshold) return null;
      const severity: CloudAlertSeverity = server.cpu >= criticalThreshold ? "critical" : config.severity;
      return {
        severity,
        currentValue: formatPercentValue(server.cpu),
        condition: `>= ${warningThreshold}% 预警，>= ${criticalThreshold}% 严重`,
        message: `${server.name} CPU 已达到 ${formatPercentValue(server.cpu)}`,
        suggestion: getResourceType(server) === "rds" ? "排查慢 SQL、连接数和突增查询，必要时评估规格升级" : "排查进程热点、定时任务和突发流量，必要时扩容或限流",
      };
    },
  },
  {
    id: "metric-memory",
    title: "内存使用率过高",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "内存",
    condition: ">= 75% 预警，>= 85% 严重",
    severity: "warning",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      const warningThreshold = config.warningThreshold ?? 75;
      const criticalThreshold = config.criticalThreshold ?? 85;
      if (typeof server.memory !== "number" || server.memory < warningThreshold) return null;
      const severity: CloudAlertSeverity = server.memory >= criticalThreshold ? "critical" : config.severity;
      return {
        severity,
        currentValue: formatPercentValue(server.memory),
        condition: `>= ${warningThreshold}% 预警，>= ${criticalThreshold}% 严重`,
        message: `${server.name} 内存已达到 ${formatPercentValue(server.memory)}`,
        suggestion: "观察最近采样趋势，排查内存泄漏、缓存膨胀或数据库连接堆积",
      };
    },
  },
  {
    id: "metric-disk",
    title: "磁盘或存储水位过高",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "磁盘 / 存储",
    condition: ">= 80% 预警，>= 90% 严重",
    severity: "warning",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      const warningThreshold = config.warningThreshold ?? 80;
      const criticalThreshold = config.criticalThreshold ?? 90;
      if (typeof server.disk !== "number" || server.disk < warningThreshold) return null;
      const resourceType = getResourceType(server);
      const severity: CloudAlertSeverity = server.disk >= criticalThreshold ? "critical" : config.severity;
      return {
        severity,
        currentValue: formatPercentValue(server.disk),
        condition: `>= ${warningThreshold}% 预警，>= ${criticalThreshold}% 严重`,
        message: `${server.name} ${resourceType === "rds" ? "存储" : "磁盘"}水位 ${formatPercentValue(server.disk)}`,
        suggestion: resourceType === "rds" ? "确认表空间、日志和备份增长，准备扩容或清理策略" : "清理日志与临时文件，确认是否需要扩容数据盘",
      };
    },
  },
  {
    id: "billing-expiration",
    title: "包年包月资源即将到期",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "到期时间",
    condition: "<= 30 天预警，已到期严重",
    severity: "warning",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      const warningThreshold = config.warningThreshold ?? 30;
      const expiration = resolveExpirationInfo(server);
      if (expiration.status !== "expired" && expiration.status !== "expiring") return null;
      if (typeof expiration.expiresInDays === "number" && expiration.expiresInDays > warningThreshold) return null;
      const severity: CloudAlertSeverity = expiration.status === "expired" ? "critical" : "warning";
      return {
        severity,
        currentValue: expiration.dateText ? `${expiration.text} / ${expiration.dateText}` : expiration.text,
        condition: expiration.status === "expired" ? "已到期" : `剩余天数小于等于 ${warningThreshold} 天`,
        message: `${server.name} ${expiration.status === "expired" ? "已经到期" : `将在 ${expiration.text} 后到期`}`,
        suggestion: "确认续费策略、预算归属和是否需要释放闲置资源",
      };
    },
  },
  {
    id: "metric-freshness",
    title: "云监控采样不可用",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "监控采样",
    condition: "监控离线、偏旧或未同步",
    severity: "warning",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      if (server.agentStatus === "online") return null;
      const severity: CloudAlertSeverity = server.agentStatus === "offline" ? "critical" : config.severity;
      return {
        severity,
        currentValue: agentStatusLabels[server.agentStatus],
        message: `${server.name} 监控状态为${agentStatusLabels[server.agentStatus]}`,
        suggestion: "检查云监控权限、插件采集项和实例当前运行状态",
      };
    },
  },
  {
    id: "security-public-ip",
    title: "公网访问面暴露",
    resourceTypes: ["ecs"],
    providers: ["aliyun", "huawei"],
    metric: "公网入口",
    condition: "存在公网 IP 或 EIP",
    severity: "info",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      if (server.publicIpType === "none" || !server.publicIp || server.publicIp === "-") return null;
      return {
        severity: config.severity,
        currentValue: server.publicIp,
        message: `${server.name} 存在${server.publicIpType === "eip" ? "弹性公网 IP" : "公网 IP"}`,
        suggestion: "确认安全组只开放必要端口，并为公网入口补齐负责人和告警策略",
      };
    },
  },
  {
    id: "cost-postpaid-running",
    title: "长期运行按量资源",
    resourceTypes: ["ecs", "rds"],
    providers: ["aliyun", "huawei"],
    metric: "计费方式",
    condition: "按量资源运行超过 30 天",
    severity: "info",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      const warningThreshold = config.warningThreshold ?? 30;
      if (!isPostPaidCharge(server.chargeType)) return null;
      const runningDays = daysSince(server.createdAt);
      if (runningDays === null || runningDays < warningThreshold) return null;
      return {
        severity: config.severity,
        currentValue: `${server.chargeType} / ${runningDays} 天`,
        message: `${server.name} 是长期运行的按量资源`,
        suggestion: "确认是否需要转包年包月、降配或释放闲置资源",
      };
    },
  },
  {
    id: "rds-lock",
    title: "RDS 锁定或保护状态",
    resourceTypes: ["rds"],
    providers: ["aliyun"],
    metric: "RDS 锁定状态",
    condition: "出现非空锁定模式",
    severity: "warning",
    enabled: true,
    notifyChannels: ["控制台"],
    evaluate: (server, config) => {
      if (getResourceType(server) !== "rds") return null;
      const lockMode = server.dbLockMode?.trim();
      if (!lockMode || lockMode.toLowerCase() === "normal") return null;
      return {
        severity: config.severity,
        currentValue: lockMode,
        message: `${server.name} 当前锁定状态为 ${lockMode}`,
        suggestion: server.dbLockReason ? `查看锁定原因：${server.dbLockReason}` : "进入 RDS 控制台确认锁定原因和恢复动作",
      };
    },
  },
];

const defaultAlertRuleThresholds: Record<string, Pick<CloudAlertRuleConfig, "warningThreshold" | "criticalThreshold">> = {
  "metric-cpu": { warningThreshold: 75, criticalThreshold: 85 },
  "metric-memory": { warningThreshold: 75, criticalThreshold: 85 },
  "metric-disk": { warningThreshold: 80, criticalThreshold: 90 },
  "billing-expiration": { warningThreshold: 30 },
  "cost-postpaid-running": { warningThreshold: 30 },
};

const buildInitialAlertRuleConfigs = (): Record<string, CloudAlertRuleConfig> =>
  Object.fromEntries(
    cloudAlertRules.map((rule) => [
      rule.id,
      {
        enabled: rule.enabled,
        severity: rule.severity,
        notifyChannels: [...rule.notifyChannels],
        ...defaultAlertRuleThresholds[rule.id],
      },
    ])
  );

const getAlertRuleConfig = (configs: Record<string, CloudAlertRuleConfig>, rule: CloudAlertRule): CloudAlertRuleConfig => {
  const saved = configs[rule.id];
  return {
    enabled: saved?.enabled ?? rule.enabled,
    severity: saved?.severity ?? rule.severity,
    notifyChannels: saved?.notifyChannels ?? rule.notifyChannels,
    warningThreshold: saved?.warningThreshold ?? defaultAlertRuleThresholds[rule.id]?.warningThreshold,
    criticalThreshold: saved?.criticalThreshold ?? defaultAlertRuleThresholds[rule.id]?.criticalThreshold,
  };
};

const formatAlertRuleCondition = (rule: CloudAlertRule, config: CloudAlertRuleConfig) => {
  if (rule.id === "metric-cpu" || rule.id === "metric-memory" || rule.id === "metric-disk") {
    return `>= ${config.warningThreshold ?? 0}% 预警，>= ${config.criticalThreshold ?? 0}% 严重`;
  }
  if (rule.id === "billing-expiration") return `<= ${config.warningThreshold ?? 0} 天预警，已到期严重`;
  if (rule.id === "cost-postpaid-running") return `按量资源运行超过 ${config.warningThreshold ?? 0} 天`;
  return rule.condition;
};

const buildCloudAlertReminders = (servers: CloudServer[], configs: Record<string, CloudAlertRuleConfig>): CloudAlertReminder[] => {
  const nowText = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  const reminders = servers.flatMap((server) =>
    cloudAlertRules.flatMap((rule) => {
      const resourceType = getResourceType(server);
      const config = getAlertRuleConfig(configs, rule);
      if (!config.enabled || !rule.resourceTypes.includes(resourceType) || !rule.providers.includes(server.provider)) return [];
      const result = rule.evaluate(server, config);
      if (!result) return [];
      return [
        {
          id: `${rule.id}:${server.id}`,
          ruleId: rule.id,
          serverId: server.id,
          accountId: server.accountId,
          provider: server.provider,
          resourceType,
          resourceName: server.name,
          instanceId: server.instanceId,
          region: server.region,
          severity: result.severity ?? rule.severity,
          title: result.title ?? rule.title,
          metric: rule.metric,
          condition: result.condition ?? formatAlertRuleCondition(rule, config),
          currentValue: result.currentValue,
          message: result.message,
          suggestion: result.suggestion,
          notifyChannels: config.notifyChannels,
          updatedAt: nowText,
        },
      ];
    })
  );
  return reminders.sort((left, right) => {
    const severityDiff = alertSeverityRank[left.severity] - alertSeverityRank[right.severity];
    if (severityDiff !== 0) return severityDiff;
    return left.resourceName.localeCompare(right.resourceName, "zh-CN");
  });
};

const getStatusClass = (status: string) => {
  if (status === "running" || status === "online" || status === "normal" || status === "info" || status === "完成") {
    return "ok";
  }
  if (status === "warning" || status === "stale" || status === "syncing" || status === "expiring" || status === "执行中" || status === "等待") {
    return "warn";
  }
  if (status === "maintenance" || status === "disabled" || status === "no_expiration" || status === "unknown") {
    return "muted";
  }
  return "bad";
};

const getPageTitle = (page: PageKey) => menuGroups.flatMap((group) => group.items).find((item) => item.key === page)?.label ?? "";
const defaultPageSections: Partial<Record<PageKey, string>> = {
  servers: "overview",
  accounts: "list",
  monitor: "workspace",
  agents: "overview",
  alerts: "reminders",
  ai: "analysis",
  knowledge: "entries",
  sync: "tasks",
  settings: "config",
};
const getPageSectionTitle = (page: PageKey, section: string) =>
  menuGroups
    .flatMap((group) => group.items)
    .find((item) => item.key === page)
    ?.children?.find((item) => item.key === section)?.label ?? "";

function StatusText({ children, status }: { children: string; status: string }) {
  return <span className={`status-text ${getStatusClass(status)}`}>{children}</span>;
}

function Utilization({ value }: { value?: number }) {
  if (typeof value !== "number") {
    return (
      <span className="usage usage-empty">
        <i style={{ width: "0%" }} />
        <b>--</b>
      </span>
    );
  }
  const tone = value >= 80 ? "bad" : value >= 70 ? "warn" : "ok";
  return (
    <span className={`usage usage-${tone}`}>
      <i style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
      <b>{formatPercentValue(value)}</b>
    </span>
  );
}

function MetricLineChart({ points, status }: { points: AliyunMetricPoint[]; status: MonitorMetric["status"] }) {
  if (points.length < 2) {
    return (
      <div className="metric-line-chart metric-line-chart-empty">
        <span>{points.length === 1 ? "单点采样" : "暂无趋势"}</span>
      </div>
    );
  }
  const first = points[0];
  const latest = points.at(-1);
  const delta = (latest?.value ?? 0) - (first?.value ?? 0);
  const trendClass = status !== "ok" ? "muted" : delta > 0 ? "up" : delta < 0 ? "down" : "flat";
  const chartWidth = 480;
  const chartHeight = 168;
  const chartPadding = 18;
  const path = buildSparklinePath(points, chartWidth, chartHeight, chartPadding);
  const coordinates = buildMetricPointCoordinates(points, chartWidth, chartHeight, chartPadding);
  const latestCoordinate = coordinates.at(-1);
  return (
    <div className={`metric-line-chart metric-line-chart-${trendClass}`}>
      <svg viewBox={`0 0 ${chartWidth} ${chartHeight}`} aria-hidden="true" focusable="false">
        <line x1="0" y1="42" x2={chartWidth} y2="42" />
        <line x1="0" y1="84" x2={chartWidth} y2="84" />
        <line x1="0" y1="126" x2={chartWidth} y2="126" />
        <path d={path} pathLength={100} />
        {latestCoordinate ? <circle cx={latestCoordinate.x} cy={latestCoordinate.y} r="3.8" /> : null}
      </svg>
    </div>
  );
}

function MetricChartCard({ item }: { item: MonitorMetric }) {
  return (
    <article className={`monitor-chart-card monitor-chart-card-${item.status}`}>
      <div className="monitor-chart-card-head">
        <div>
          <h4>{item.label}</h4>
          <span>{item.unit || "无单位"} / {item.points} 个点</span>
        </div>
        <StatusText status={item.status === "ok" ? "normal" : item.status === "error" ? "warning" : "disabled"}>
          {item.status === "ok" ? "正常" : item.status === "error" ? "异常" : "暂无数据"}
        </StatusText>
      </div>
      <div className="monitor-chart-value-row">
        <strong>{item.value}</strong>
        <span>{item.trend}</span>
      </div>
      <MetricLineChart points={item.sparklinePoints} status={item.status} />
      <div className="monitor-chart-facts">
        <span><b>范围</b>{item.range}</span>
        <span><b>均值</b>{item.average}</span>
        <span><b>来源</b>{item.source}</span>
        <span><b>原始指标</b>{item.metricName}</span>
        <span><b>周期</b>{item.period}</span>
      </div>
      <p className={item.status === "ok" ? "monitor-chart-note muted" : "monitor-chart-note"}>{item.note}</p>
    </article>
  );
}

export function CloudResourceConsole() {
  const [activePage, setActivePage] = useState<PageKey>("servers");
  const [accounts, setAccounts] = useState<CloudAccount[]>(() => (USE_MOCK ? mockAccounts : []));
  const [servers, setServers] = useState<CloudServer[]>(() => (USE_MOCK ? mockServers : []));
  const [activePageSections, setActivePageSections] = useState<Partial<Record<PageKey, string>>>(defaultPageSections);
  const [activeAlertSection, setActiveAlertSection] = useState<AlertSectionKey>("reminders");
  const [assetLoading, setAssetLoading] = useState(!USE_MOCK);
  const [assetError, setAssetError] = useState("");
  const [assetIssues, setAssetIssues] = useState<CloudAssetIssue[]>([]);
  const [lastSyncAt, setLastSyncAt] = useState("--");
  const [scope, setScope] = useState(buildScopeKey("all"));
  const [keyword, setKeyword] = useState("");
  const [resourceFilter, setResourceFilter] = useState<ResourceType | "all">("all");
  const [statusFilter, setStatusFilter] = useState<ServerStatus | "all">("all");
  const [agentFilter, setAgentFilter] = useState<AgentStatus | "all">("all");
  const [selectedServerId, setSelectedServerId] = useState(servers[0]?.id ?? "");
  const [sidebarHidden, setSidebarHidden] = useState(() => resolveInitialSidebarHidden());
  const [theme, setTheme] = useState<ThemeMode>(() => resolveInitialTheme());
  const [overviewByServerId, setOverviewByServerId] = useState<Record<string, AliyunOverviewResponse>>({});
  const [accountForm, setAccountForm] = useState<CloudAccountForm>(emptyAccountForm);
  const [editingAccountId, setEditingAccountId] = useState("");
  const [accountSaving, setAccountSaving] = useState(false);
  const [testingAccountId, setTestingAccountId] = useState("");
  const [togglingAccountId, setTogglingAccountId] = useState("");
  const [accountNotice, setAccountNotice] = useState("");
  const [expandedTree, setExpandedTree] = useState<Record<string, boolean>>({});
  const [metricSort, setMetricSort] = useState<{ key: ServerSortKey; direction: SortDirection } | null>(null);
  const [refreshState, setRefreshState] = useState<RefreshState>(() => (USE_MOCK ? "ok" : "syncing"));
  const [alertRuleConfigs, setAlertRuleConfigs] = useState<Record<string, CloudAlertRuleConfig>>(() => buildInitialAlertRuleConfigs());
  const [selectedAlertRuleId, setSelectedAlertRuleId] = useState(cloudAlertRules[0]?.id ?? "");
  const [alertFilter, setAlertFilter] = useState<AlertWorkbenchFilter>({});
  const assetLoadingRef = useRef(false);

  const getActiveSection = useCallback((page: PageKey) => {
    if (page === "alerts") return activeAlertSection;
    return activePageSections[page] ?? defaultPageSections[page] ?? "";
  }, [activeAlertSection, activePageSections]);

  const setActiveSection = useCallback((page: PageKey, section: string) => {
    if (page === "alerts") {
      setActiveAlertSection(section as AlertSectionKey);
      return;
    }
    setActivePageSections((current) => ({ ...current, [page]: section }));
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarHidden));
  }, [sidebarHidden]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  const refreshCloudResourceOverviews = async (tasks: CloudOverviewRefreshTask[], baseIssues: CloudAssetIssue[]) => {
    if (!tasks.length) return;
    const results = await Promise.allSettled(tasks.map((task) => task()));
    const overviewMap: Record<string, AliyunOverviewResponse> = {};
    const nextServers: CloudServer[] = [];
    const nextIssues = [...baseIssues];
    for (const result of results) {
      if (result.status !== "fulfilled") {
        continue;
      }
      overviewMap[result.value.server.id] = result.value.overview;
      nextServers.push(result.value.server);
      if (result.value.issue) {
        nextIssues.push(result.value.issue);
      }
    }
    if (!nextServers.length) return;
    const nextServerMap = new Map(nextServers.map((server) => [server.id, server]));
    setOverviewByServerId((current) => mergeOverviewMapWithPrevious(overviewMap, current, nextServers));
    setServers((current) => {
      const candidateRows = current.map((server) => nextServerMap.get(server.id) ?? server);
      return mergeServerRowsWithPrevious(candidateRows, current, overviewMap);
    });
    setAssetIssues(nextIssues);
    setAssetError(summarizeCloudAssetIssues(nextIssues));
    setRefreshState(nextIssues.length === 0 ? "ok" : "error");
  };

  const loadCloudAssets = async (options: { silent?: boolean } = {}) => {
    if (assetLoadingRef.current) return false;
    assetLoadingRef.current = true;
    setRefreshState("syncing");
    if (USE_MOCK) {
      setAssetLoading(false);
      setLastSyncAt("示例模式");
      setRefreshState("ok");
      assetLoadingRef.current = false;
      return true;
    }
    if (!options.silent) setAssetLoading(true);
    setAssetError("");
    try {
      const accountResp = await fetchWithTimeout(`${API_BASE}/api/cloud/accounts`, {
        cache: "no-store",
        headers: buildApiHeaders(),
      });
      const accountPayload = (await accountResp.json().catch(() => ({}))) as CloudAccountsResponse;
      if (!accountResp.ok) {
        throw new Error(accountPayload.error || `云账号接口返回 ${accountResp.status}`);
      }
      const apiAccounts = accountPayload.items ?? [];
      const mappedAccounts = apiAccounts.map(mapApiCloudAccount);
      setAccounts(mappedAccounts);
      if (!mappedAccounts.length) {
        setServers([]);
        setOverviewByServerId({});
        setAssetIssues([]);
        setSelectedServerId("");
        setLastSyncAt("等待配置云账号");
        setRefreshState("ok");
        return true;
      }

      const overviewTasks: CloudOverviewRefreshTask[] = [];
      const issues: CloudAssetIssue[] = [];
      const accountSyncResults = new Map<string, Pick<CloudAccount, "status" | "lastSync" | "checkMessage">>();
      const appendAssetIssue = (issue: CloudAssetIssue) => {
        issues.push(issue);
        setAssetIssues([...issues]);
        setAssetError(summarizeCloudAssetIssues(issues));
      };
      const applyAccountResourceRows = (account: CloudAccount, resourceType: ResourceType, accountRows: CloudServer[]) => {
        setServers((current) => {
          const rowsWithoutResource = current.filter((server) => server.accountId !== account.id || getResourceType(server) !== resourceType);
          return mergePendingServerRowsWithPrevious([...rowsWithoutResource, ...accountRows], current);
        });
        setSelectedServerId((current) => current || accountRows[0]?.id || "");
      };
      const accountResults = await Promise.allSettled(
        mappedAccounts
          .filter((account) => account.enabled !== false)
          .map(async (account) => {
            const providerPath = encodeURIComponent(account.provider);
            const accountIssueMessages: string[] = [];
            const loadECSRows = async () => {
              const resp = await fetchWithTimeout(`${API_BASE}/api/cloud/${providerPath}/instances?accountId=${encodeURIComponent(account.id)}`, {
                cache: "no-store",
                headers: buildApiHeaders(),
              });
              const payload = (await resp.json().catch(() => ({}))) as AliyunInstancesResponse;
              if (!resp.ok) {
                throw buildCloudAssetIssue(account, "ecs", "实例列表", payload.error || `接口返回 ${resp.status}`);
              }
              const instances = payload.items ?? [];
              return instances.map((instance) => {
                const publicIp = instance.eipAddress?.trim() || instance.publicIps?.find((ip) => ip.trim())?.trim() || "";
                const server = mapAliyunServer(account, instance);
                if (server.status === "offline") {
                  return server;
                }
                overviewTasks.push(async () => {
                  const overviewResp = await fetchWithTimeout(
                    `${API_BASE}/api/cloud/${providerPath}/overview?accountId=${encodeURIComponent(account.id)}&instanceId=${encodeURIComponent(instance.id)}&region=${encodeURIComponent(instance.regionId)}&minutes=30&period=${encodeURIComponent(account.metricPeriod || "60")}&publicIp=${encodeURIComponent(publicIp)}`,
                    { cache: "no-store", headers: buildApiHeaders() },
                    CLOUD_OVERVIEW_FETCH_TIMEOUT_MS
                  );
                  const overview = (await overviewResp.json().catch(() => ({}))) as AliyunOverviewResponse;
                  if (overviewResp.ok) {
                    return { server: mapAliyunServer(account, instance, overview), overview };
                  }
                  const message = overview.error || "ECS 监控数据读取失败";
                  return {
                    server,
                    overview: { ok: false, error: message },
                    issue: buildCloudAssetIssue(account, "ecs", `${instance.regionId || "--"} / ${instance.id}`, message),
                  };
                });
                return server;
              });
            };
            const loadRDSRows = async () => {
              const resp = await fetchWithTimeout(`${API_BASE}/api/cloud/${providerPath}/rds/instances?accountId=${encodeURIComponent(account.id)}`, {
                cache: "no-store",
                headers: buildApiHeaders(),
              });
              const payload = (await resp.json().catch(() => ({}))) as AliyunRDSInstancesResponse;
              if (!resp.ok) {
                throw buildCloudAssetIssue(account, "rds", "实例列表", payload.error || `接口返回 ${resp.status}`);
              }
              const instances = payload.items ?? [];
              return instances.map((instance) => {
                const server = mapCloudRDSResource(account, instance);
                if (server.status === "offline") {
                  return server;
                }
                overviewTasks.push(async () => {
                  const overviewResp = await fetchWithTimeout(
                    `${API_BASE}/api/cloud/${providerPath}/rds/overview?accountId=${encodeURIComponent(account.id)}&dbInstanceId=${encodeURIComponent(instance.id)}&nodeId=${encodeURIComponent(instance.nodeId || "")}&region=${encodeURIComponent(instance.regionId)}&engine=${encodeURIComponent(instance.engine || "")}&minutes=30&period=${encodeURIComponent(account.metricPeriod || "60")}`,
                    { cache: "no-store", headers: buildApiHeaders() },
                    CLOUD_OVERVIEW_FETCH_TIMEOUT_MS
                  );
                  const overview = (await overviewResp.json().catch(() => ({}))) as AliyunOverviewResponse;
                  if (overviewResp.ok) {
                    return { server: mapCloudRDSResource(account, instance, overview), overview };
                  }
                  const message = overview.error || "RDS 性能数据读取失败";
                  return {
                    server,
                    overview: { ok: false, resource: "rds", error: message },
                    issue: buildCloudAssetIssue(account, "rds", `${instance.regionId || "--"} / ${instance.id}`, message),
                  };
                });
                return server;
              });
            };
            const resourceLoaders: Array<{ type: ResourceType; load: () => Promise<CloudServer[]> }> =
              [
                { type: "ecs", load: loadECSRows },
                { type: "rds", load: loadRDSRows },
              ];
            const resourceRows = await Promise.all(
              resourceLoaders.map(async (loader) => {
                try {
                  const rows = await loader.load();
                  applyAccountResourceRows(account, loader.type, rows);
                  return rows;
                } catch (reason) {
                  const issue = isCloudAssetIssue(reason)
                    ? reason
                    : buildCloudAssetIssue(account, loader.type, "资源同步", reason instanceof Error ? reason.message : "云资源同步失败");
                  accountIssueMessages.push(issue.message);
                  appendAssetIssue(issue);
                  applyAccountResourceRows(account, loader.type, []);
                  return [];
                }
              })
            );
            const accountRows = resourceRows.flat();
            const syncedAt = new Date().toLocaleTimeString("zh-CN", { hour12: false });
            accountSyncResults.set(account.id, {
              status: accountIssueMessages.length ? "warning" : "normal",
              lastSync: syncedAt,
              checkMessage: accountIssueMessages[0] || `资源同步完成，发现 ${accountRows.length} 个 ECS/RDS 资源`,
            });
            if (!accountRows.length) {
              return [];
            }
            setServers((current) => {
              const rowsWithoutAccount = current.filter((server) => server.accountId !== account.id);
              return mergePendingServerRowsWithPrevious([...rowsWithoutAccount, ...accountRows], current);
            });
            setSelectedServerId((current) => current || accountRows[0]?.id || "");
            return accountRows;
          })
      );
      const rows = accountResults.flatMap((result) => {
        if (result.status === "fulfilled") return result.value;
        if (isCloudAssetIssue(result.reason)) {
          appendAssetIssue(result.reason);
        } else {
          appendAssetIssue({
            id: `unknown:${Date.now()}:${issues.length}`,
            provider: "aliyun",
            accountId: "",
            accountName: "未知账号",
            resourceType: "ecs",
            scope: "资源同步",
            message: result.reason instanceof Error ? result.reason.message : "云资源同步失败",
          });
        }
        return [];
      });
      setOverviewByServerId((current) => filterOverviewMapForRows(current, rows));
      setServers((current) => mergePendingServerRowsWithPrevious(rows, current));
      setAccounts((current) => current.map((account) => {
        const syncResult = accountSyncResults.get(account.id);
        return syncResult ? { ...account, ...syncResult } : account;
      }));
      setSelectedServerId((current) => (rows.some((server) => server.id === current) ? current : rows[0]?.id ?? ""));
      setLastSyncAt(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
      setAssetIssues(issues);
      setAssetError(summarizeCloudAssetIssues(issues));
      setRefreshState(issues.length === 0 ? "ok" : "error");
      void refreshCloudResourceOverviews(overviewTasks, issues);
      return issues.length === 0;
    } catch (error) {
      setAssetError(error instanceof Error ? error.message : "云资源同步失败");
      setAssetIssues([]);
      setRefreshState("error");
      return false;
    } finally {
      assetLoadingRef.current = false;
      if (!options.silent) setAssetLoading(false);
    }
  };

  const refreshAccountsAndResources = async () => {
    setAccountNotice("正在刷新账号列表、ECS/RDS 实例和监控概览");
    const ok = await loadCloudAssets();
    setAccountNotice(ok ? "账号列表、ECS/RDS 实例和监控概览已刷新" : "刷新未完成，请查看页面顶部的云资源同步提示");
  };

  const saveAccount = async () => {
    if (USE_MOCK) return;
    if (!accountForm.name.trim()) {
      setAccountNotice("请填写账号名称");
      return;
    }
    if (!editingAccountId && (!accountForm.accessKeyId.trim() || !accountForm.accessKeySecret.trim())) {
      setAccountNotice("新增账号需要填写 AccessKey ID 和 Secret");
      return;
    }
    setAccountSaving(true);
    setAccountNotice("");
    try {
      const endpoint = editingAccountId
        ? `${API_BASE}/api/cloud/accounts/${encodeURIComponent(editingAccountId)}`
        : `${API_BASE}/api/cloud/accounts`;
      const resp = await fetch(endpoint, {
        method: editingAccountId ? "PUT" : "POST",
        headers: buildApiHeaders(true),
        body: JSON.stringify({
          provider: accountForm.provider,
          name: accountForm.name.trim(),
          accessKeyId: accountForm.accessKeyId.trim(),
          accessKeySecret: accountForm.accessKeySecret.trim(),
          regions: [],
          metricPeriod: accountForm.metricPeriod.trim() || "60",
        }),
      });
      const payload = (await resp.json().catch(() => ({}))) as { error?: string };
      if (!resp.ok) {
        throw new Error(payload.error || `保存云账号失败 ${resp.status}`);
      }
      setAccountForm(emptyAccountForm);
      setEditingAccountId("");
      setAccountNotice(editingAccountId ? "账号已更新" : "账号已保存");
      await loadCloudAssets();
    } catch (error) {
      setAccountNotice(error instanceof Error ? error.message : "保存云账号失败");
    } finally {
      setAccountSaving(false);
    }
  };

  const testAccount = async (accountId: string) => {
    if (USE_MOCK) return;
    setTestingAccountId(accountId);
    setAccountNotice("");
    try {
      const resp = await fetch(`${API_BASE}/api/cloud/accounts/${encodeURIComponent(accountId)}/test`, {
        method: "POST",
        headers: buildApiHeaders(),
      });
      const payload = (await resp.json().catch(() => ({}))) as CloudAccountTestResponse;
      if (!resp.ok) {
        throw new Error(payload.error || `校验云账号失败 ${resp.status}`);
      }
      const detail = payload.checks?.map((item) => `${item.name}: ${item.message}`).join("；") || "校验完成";
      setAccountNotice(detail);
      await loadCloudAssets();
    } catch (error) {
      setAccountNotice(error instanceof Error ? error.message : "校验云账号失败");
    } finally {
      setTestingAccountId("");
    }
  };

  const deleteAccount = async (accountId: string) => {
    if (USE_MOCK) return;
    setAccountNotice("");
    try {
      const resp = await fetch(`${API_BASE}/api/cloud/accounts/${encodeURIComponent(accountId)}`, {
        method: "DELETE",
        headers: buildApiHeaders(),
      });
      const payload = (await resp.json().catch(() => ({}))) as { error?: string };
      if (!resp.ok) {
        throw new Error(payload.error || `删除云账号失败 ${resp.status}`);
      }
      if (editingAccountId === accountId) {
        setEditingAccountId("");
        setAccountForm(emptyAccountForm);
      }
      setAccountNotice("账号已删除");
      await loadCloudAssets();
    } catch (error) {
      setAccountNotice(error instanceof Error ? error.message : "删除云账号失败");
    }
  };

  const toggleAccountEnabled = async (account: CloudAccount) => {
    const nextEnabled = account.enabled === false;
    if (USE_MOCK) {
      setAccounts((current) =>
        current.map((item) =>
          item.id === account.id
            ? {
                ...item,
                enabled: nextEnabled,
                scope: nextEnabled ? "ECS/RDS 资源与指标" : "已停用",
                status: nextEnabled ? "normal" : "disabled",
              }
            : item
        )
      );
      setAccountNotice(nextEnabled ? "示例账号已启用" : "示例账号已停用");
      return;
    }
    setTogglingAccountId(account.id);
    setAccountNotice("");
    try {
      const resp = await fetch(`${API_BASE}/api/cloud/accounts/${encodeURIComponent(account.id)}`, {
        method: "PUT",
        headers: buildApiHeaders(true),
        body: JSON.stringify({
          provider: account.provider,
          enabled: nextEnabled,
        }),
      });
      const payload = (await resp.json().catch(() => ({}))) as { error?: string };
      if (!resp.ok) {
        throw new Error(payload.error || `更新云账号状态失败 ${resp.status}`);
      }
      setAccountNotice(nextEnabled ? "账号已启用，后续刷新会同步该账号资源" : "账号已停用，后续刷新会跳过该账号资源");
      await loadCloudAssets();
    } catch (error) {
      setAccountNotice(error instanceof Error ? error.message : "更新云账号状态失败");
    } finally {
      setTogglingAccountId("");
    }
  };

  const editAccount = (account: CloudAccount) => {
    setEditingAccountId(account.id);
    setAccountForm({
      provider: account.provider,
      name: account.name,
      accessKeyId: "",
      accessKeySecret: "",
      metricPeriod: account.metricPeriod || "60",
    });
    setActiveSection("accounts", "form");
    setAccountNotice("编辑模式下 AccessKey 留空表示沿用原值");
  };

  const cancelAccountEdit = () => {
    setEditingAccountId("");
    setAccountForm(emptyAccountForm);
    setAccountNotice("");
  };

  const updateAccountForm = <K extends keyof CloudAccountForm>(key: K, value: CloudAccountForm[K]) => {
    setAccountForm((current) => {
      if (key === "provider") {
        const nextProvider = value as Provider;
        return {
          ...current,
          provider: nextProvider,
        };
      }
      return { ...current, [key]: value };
    });
  };

  const isTreeExpanded = (key: string) => expandedTree[key] === true;

  const toggleTreeNode = (key: string, defaultExpanded = false) => {
    setExpandedTree((current) => {
      const currentExpanded = current[key] ?? defaultExpanded;
      return { ...current, [key]: !currentExpanded };
    });
  };

  const toggleMetricSort = (key: ServerSortKey) => {
    setMetricSort((current) => {
      if (current?.key !== key) return { key, direction: "desc" };
      return { key, direction: current.direction === "desc" ? "asc" : "desc" };
    });
  };

  useEffect(() => {
    void loadCloudAssets();
    if (USE_MOCK || typeof window === "undefined") return;
    const timer = window.setInterval(() => {
      void loadCloudAssets({ silent: true });
    }, ASSET_REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, []);

  const parsedScope = useMemo(() => parseScope(scope), [scope]);

  const getAccount = useCallback((accountId: string) => accounts.find((account) => account.id === accountId), [accounts]);

  const selectScope = (nextScope: string) => {
    const nextParsedScope = parseScope(nextScope);
    const firstServer = servers.find((server) => serverMatchesScope(server, nextParsedScope));
    setScope(nextScope);
    setSelectedServerId(firstServer?.id ?? "");
    if (nextParsedScope.kind === "product" || nextParsedScope.kind === "productProvider" || nextParsedScope.kind === "productAccount" || nextParsedScope.kind === "productAccountRegion") {
      setResourceFilter("all");
    }
  };

  const openResourceContext = (page: PageKey, serverId = selectedServer.id) => {
    setSelectedServerId(serverId);
    setActivePage(page);
    if (page === "monitor") setActiveSection("monitor", "workspace");
    if (page === "alerts") {
      setActiveSection("alerts", "reminders");
      setAlertFilter({ serverId });
    }
    if (page === "ai") setActiveSection("ai", "analysis");
    if (page === "knowledge") setActiveSection("knowledge", "entries");
  };

  const visibleServers = useMemo(() => {
    const text = keyword.trim().toLowerCase();
    const rows = servers.filter((server) => {
      if (!serverMatchesScope(server, parsedScope)) return false;
      if (resourceFilter !== "all" && getResourceType(server) !== resourceFilter) return false;
      if (statusFilter !== "all" && server.status !== statusFilter) return false;
      if (agentFilter !== "all" && server.agentStatus !== agentFilter) return false;
      if (!text) return true;
      const account = getAccount(server.accountId);
      return [
        server.name,
        server.instanceId,
        server.business,
        server.region,
        server.zone,
        server.publicIp,
        server.privateIp,
        server.expiredAt,
        server.expirationText,
        resolveExpirationInfo(server).text,
        account?.name,
        account?.alias,
        providerLabels[server.provider],
      ]
        .join(" ")
        .toLowerCase()
        .includes(text);
    });
    return sortServersByMetric(rows, metricSort);
  }, [agentFilter, getAccount, keyword, metricSort, parsedScope, resourceFilter, servers, statusFilter]);

  const monitorResourceTree = useMemo(
    () => buildResourceProductTree(servers, accounts, metricSort),
    [accounts, metricSort, servers]
  );

  const selectedServer = useMemo(() => {
    if (activePage === "monitor" || activePage === "agents") return servers.find((server) => server.id === selectedServerId) ?? servers[0] ?? emptyCloudServer;
    return visibleServers.find((server) => server.id === selectedServerId) ?? visibleServers[0] ?? servers[0] ?? emptyCloudServer;
  }, [activePage, selectedServerId, servers, visibleServers]);

  const selectedAccount = selectedServer ? getAccount(selectedServer.accountId) : undefined;

  const summary = useMemo(() => {
    const warningServers = servers.filter((server) => server.status === "warning" || server.status === "offline").length;
    const agentIssues = servers.filter((server) => server.agentStatus !== "online").length;
    const ecsCount = servers.filter((server) => getResourceType(server) === "ecs").length;
    const rdsCount = servers.filter((server) => getResourceType(server) === "rds").length;
    return { warningServers, agentIssues, ecsCount, rdsCount };
  }, [servers]);

  const expirationSummary = useMemo(() => {
    let expiring = 0;
    servers.forEach((server) => {
      const status = resolveExpirationInfo(server).status;
      if (status === "expiring") expiring += 1;
    });
    return { expiring };
  }, [servers]);

  const refreshSummary = useMemo(() => {
    if (USE_MOCK) return "示例模式";
    if (refreshState === "syncing") return "同步中";
    if (refreshState === "error") return "上次异常";
    return "自动刷新";
  }, [refreshState]);

  const monitorSummary = useMemo(() => {
    const online = servers.filter((server) => server.agentStatus === "online").length;
    const stale = servers.filter((server) => server.agentStatus === "stale").length;
    const missing = servers.filter((server) => server.agentStatus === "missing").length;
    const offline = servers.filter((server) => server.agentStatus === "offline").length;
    return { online, stale, missing, offline };
  }, [servers]);

  const cloudAlertReminders = useMemo(() => buildCloudAlertReminders(servers, alertRuleConfigs), [alertRuleConfigs, servers]);

  const cloudAlertSummary = useMemo(() => {
    const critical = cloudAlertReminders.filter((item) => item.severity === "critical").length;
    const warning = cloudAlertReminders.filter((item) => item.severity === "warning").length;
    const info = cloudAlertReminders.filter((item) => item.severity === "info").length;
    const affectedResources = new Set(cloudAlertReminders.map((item) => item.serverId)).size;
    return { critical, warning, info, affectedResources };
  }, [cloudAlertReminders]);

  const alertRuleRows = useMemo(() => {
    const counts = new Map<string, number>();
    cloudAlertReminders.forEach((item) => counts.set(item.ruleId, (counts.get(item.ruleId) ?? 0) + 1));
    return cloudAlertRules.map((rule) => {
      const config = getAlertRuleConfig(alertRuleConfigs, rule);
      return {
        ...rule,
        enabled: config.enabled,
        severity: config.severity,
        condition: formatAlertRuleCondition(rule, config),
        notifyChannels: config.notifyChannels,
        warningThreshold: config.warningThreshold,
        criticalThreshold: config.criticalThreshold,
        activeCount: counts.get(rule.id) ?? 0,
      };
    });
  }, [alertRuleConfigs, cloudAlertReminders]);

  const alertRuleSummary = useMemo(() => {
    const enabledRules = alertRuleRows.filter((rule) => rule.enabled).length;
    const hitRules = alertRuleRows.filter((rule) => rule.activeCount > 0).length;
    const activeReminders = alertRuleRows.reduce((total, rule) => total + rule.activeCount, 0);
    return { enabledRules, hitRules, activeReminders };
  }, [alertRuleRows]);

  const selectedAlertRule = alertRuleRows.find((rule) => rule.id === selectedAlertRuleId) ?? alertRuleRows[0];

  const updateAlertRuleConfig = useCallback((ruleId: string, patch: Partial<CloudAlertRuleConfig>) => {
    const rule = cloudAlertRules.find((item) => item.id === ruleId);
    if (!rule) return;
    setAlertRuleConfigs((current) => ({
      ...current,
      [ruleId]: {
        ...getAlertRuleConfig(current, rule),
        ...patch,
      },
    }));
  }, []);

  const updateAlertRuleThreshold = useCallback((ruleId: string, key: "warningThreshold" | "criticalThreshold", rawValue: string) => {
    const nextValue = Number(rawValue);
    if (!Number.isFinite(nextValue)) return;
    updateAlertRuleConfig(ruleId, { [key]: Math.max(0, nextValue) });
  }, [updateAlertRuleConfig]);

  const updateAlertRuleChannels = useCallback((ruleId: string, rawValue: string) => {
    const channels = rawValue.split(/[，,]/).map((item) => item.trim()).filter(Boolean);
    updateAlertRuleConfig(ruleId, { notifyChannels: channels.length ? channels : ["控制台"] });
  }, [updateAlertRuleConfig]);

  const resourceHealthRows = useMemo<ResourceHealthRow[]>(() => {
    return servers
      .map((server) => {
        const reminders = cloudAlertReminders.filter((item) => item.serverId === server.id);
        return {
          server,
          reminders,
          ...calculateResourceHealthScore(server, reminders),
        };
      })
      .filter((row) => row.reminders.length > 0 || row.score < 100)
      .sort((left, right) => {
        if (left.score !== right.score) return left.score - right.score;
        return left.server.name.localeCompare(right.server.name, "zh-CN");
      })
      .slice(0, 8);
  }, [cloudAlertReminders, servers]);

  const alertCoverageRows = useMemo(() => {
    const accountMap = new Map(accounts.map((account) => [account.id, account]));
    const groups = new Map<string, {
      key: string;
      provider: Provider;
      accountId: string;
      resourceType: ResourceType;
      accountName: string;
      total: number;
      affected: number;
      critical: number;
      warning: number;
    }>();
    servers.forEach((server) => {
      const resourceType = getResourceType(server);
      const key = `${server.provider}:${resourceType}:${server.accountId}`;
      const account = accountMap.get(server.accountId);
      const current = groups.get(key) ?? {
        key,
        provider: server.provider,
        accountId: server.accountId,
        resourceType,
        accountName: account?.name ?? account?.alias ?? "未匹配账号",
        total: 0,
        affected: 0,
        critical: 0,
        warning: 0,
      };
      const reminders = cloudAlertReminders.filter((item) => item.serverId === server.id);
      current.total += 1;
      if (reminders.length) current.affected += 1;
      current.critical += reminders.filter((item) => item.severity === "critical").length;
      current.warning += reminders.filter((item) => item.severity === "warning").length;
      groups.set(key, current);
    });
    return [...groups.values()].sort((left, right) => {
      const providerDiff = providerLabels[left.provider].localeCompare(providerLabels[right.provider], "zh-CN");
      if (providerDiff !== 0) return providerDiff;
      const typeDiff = resourceTypeLabels[left.resourceType].localeCompare(resourceTypeLabels[right.resourceType], "zh-CN");
      if (typeDiff !== 0) return typeDiff;
      return left.accountName.localeCompare(right.accountName, "zh-CN");
    });
  }, [accounts, cloudAlertReminders, servers]);

  const filteredCloudAlertReminders = useMemo(() => {
    return cloudAlertReminders.filter((item) => {
      if (alertFilter.severity && item.severity !== alertFilter.severity) return false;
      if (alertFilter.ruleId && item.ruleId !== alertFilter.ruleId) return false;
      if (alertFilter.serverId && item.serverId !== alertFilter.serverId) return false;
      if (alertFilter.coverageKey && `${item.provider}:${item.resourceType}:${item.accountId}` !== alertFilter.coverageKey) return false;
      return true;
    });
  }, [alertFilter, cloudAlertReminders]);

  const alertFilterText = useMemo(() => {
    if (alertFilter.severity) return `级别：${alertSeverityLabels[alertFilter.severity]}`;
    if (alertFilter.ruleId) return `规则：${alertRuleRows.find((rule) => rule.id === alertFilter.ruleId)?.title ?? alertFilter.ruleId}`;
    if (alertFilter.serverId) return `资源：${servers.find((server) => server.id === alertFilter.serverId)?.name ?? alertFilter.serverId}`;
    if (alertFilter.coverageKey) {
      const row = alertCoverageRows.find((item) => item.key === alertFilter.coverageKey);
      return row ? `覆盖：${providerLabels[row.provider]} / ${resourceTypeLabels[row.resourceType]} / ${row.accountName}` : "覆盖分组";
    }
    return "";
  }, [alertCoverageRows, alertFilter, alertRuleRows, servers]);

  const agentGovernance = useMemo(() => {
    const total = servers.length;
    const statusCounts = emptyAgentStatusCounter();
    let covered = 0;
    let realSampleResourceCount = 0;
    let latestTimestamp = 0;
    let pointCount = 0;
    const sourceMap = new Map<string, AgentSourceSummary>();
    const accountMap = new Map<string, AgentAccountSummary>();
    const regionMap = new Map<string, AgentRegionSummary>();

    servers.forEach((server) => {
      const overview = overviewByServerId[server.id];
      const snapshot = latestMetricSnapshot(overview);
      const resourceType = getResourceType(server);
      const account = getAccount(server.accountId);
      const sourceSnapshots = summarizeMetricSourceSnapshots(overview);
      const sources = sourceSnapshots.map((item) => item.source);
      const hasRealSample = snapshot.pointCount > 0 && snapshot.latestTimestamp > 0;
      const hasAvailableSample = server.agentStatus === "online" && hasRealSample;

      statusCounts[server.agentStatus] += 1;
      if (hasAvailableSample) {
        covered += 1;
      }
      if (hasRealSample) {
        realSampleResourceCount += 1;
        latestTimestamp = Math.max(latestTimestamp, snapshot.latestTimestamp);
        pointCount += snapshot.pointCount;
      }

      const accountKey = `${server.provider}:${server.accountId}`;
      const accountRow: AgentAccountSummary = accountMap.get(accountKey) ?? {
        key: accountKey,
        provider: server.provider,
        accountName: account?.name ?? account?.alias ?? server.accountId,
        total: 0,
        covered: 0,
        statusCounts: emptyAgentStatusCounter(),
        pointCount: 0,
        latestTimestamp: 0,
        resourceTypes: [],
        regionCount: 0,
        realSampleResourceCount: 0,
        sources: [],
        riskCount: 0,
      };
      accountRow.total += 1;
      accountRow.statusCounts[server.agentStatus] += 1;
      if (hasAvailableSample) {
        accountRow.covered += 1;
      }
      if (hasRealSample) {
        accountRow.realSampleResourceCount += 1;
        accountRow.pointCount += snapshot.pointCount;
        accountRow.latestTimestamp = Math.max(accountRow.latestTimestamp, snapshot.latestTimestamp);
      }
      if (server.agentStatus !== "online") accountRow.riskCount += 1;
      if (!accountRow.resourceTypes.includes(resourceType)) accountRow.resourceTypes.push(resourceType);
      sources.forEach((source) => {
        if (!accountRow.sources.includes(source)) accountRow.sources.push(source);
      });
      accountMap.set(accountKey, accountRow);

      const regionKey = `${server.provider}:${server.region}`;
      const regionRow: AgentRegionSummary = regionMap.get(regionKey) ?? {
        key: regionKey,
        provider: server.provider,
        region: server.region,
        total: 0,
        covered: 0,
        statusCounts: emptyAgentStatusCounter(),
        accountCount: 0,
        pointCount: 0,
        latestTimestamp: 0,
        realSampleResourceCount: 0,
        riskCount: 0,
      };
      regionRow.total += 1;
      regionRow.statusCounts[server.agentStatus] += 1;
      if (hasAvailableSample) {
        regionRow.covered += 1;
      }
      if (hasRealSample) {
        regionRow.realSampleResourceCount += 1;
        regionRow.pointCount += snapshot.pointCount;
        regionRow.latestTimestamp = Math.max(regionRow.latestTimestamp, snapshot.latestTimestamp);
      }
      if (server.agentStatus !== "online") regionRow.riskCount += 1;
      regionMap.set(regionKey, regionRow);

      sourceSnapshots.forEach((sourceSnapshot) => {
        const sourceRow: AgentSourceSummary = sourceMap.get(sourceSnapshot.source) ?? {
          source: sourceSnapshot.source,
          resourceCount: 0,
          pointCount: 0,
          latestTimestamp: 0,
          periods: [],
        };
        sourceRow.resourceCount += 1;
        sourceRow.pointCount += sourceSnapshot.pointCount;
        sourceRow.latestTimestamp = Math.max(sourceRow.latestTimestamp, sourceSnapshot.latestTimestamp);
        sourceSnapshot.periods.forEach((period) => {
          if (!sourceRow.periods.includes(period)) sourceRow.periods.push(period);
        });
        sourceMap.set(sourceSnapshot.source, sourceRow);
      });
    });

    const accountRegionSets = new Map<string, Set<string>>();
    servers.forEach((server) => {
      const key = `${server.provider}:${server.accountId}`;
      const current = accountRegionSets.get(key) ?? new Set<string>();
      current.add(server.region);
      accountRegionSets.set(key, current);
    });
    accountMap.forEach((row) => {
      row.regionCount = accountRegionSets.get(row.key)?.size ?? 0;
      row.resourceTypes.sort((left, right) => resourceTypeLabels[left].localeCompare(resourceTypeLabels[right], "zh-CN"));
      row.sources.sort((left, right) => left.localeCompare(right, "zh-CN"));
    });

    const regionAccountSets = new Map<string, Set<string>>();
    servers.forEach((server) => {
      const key = `${server.provider}:${server.region}`;
      const current = regionAccountSets.get(key) ?? new Set<string>();
      current.add(server.accountId);
      regionAccountSets.set(key, current);
    });
    regionMap.forEach((row) => {
      row.accountCount = regionAccountSets.get(row.key)?.size ?? 0;
    });

    const accountRows = [...accountMap.values()].sort((left, right) => {
      if (right.riskCount !== left.riskCount) return right.riskCount - left.riskCount;
      return left.accountName.localeCompare(right.accountName, "zh-CN");
    });
    const regionRows = [...regionMap.values()].sort((left, right) => {
      if (right.riskCount !== left.riskCount) return right.riskCount - left.riskCount;
      return `${providerLabels[left.provider]}${left.region}`.localeCompare(`${providerLabels[right.provider]}${right.region}`, "zh-CN");
    });
    const sourceRows = [...sourceMap.values()].sort((left, right) => {
      if (right.resourceCount !== left.resourceCount) return right.resourceCount - left.resourceCount;
      return left.source.localeCompare(right.source, "zh-CN");
    });

    const governanceItems: AgentGovernanceItem[] = [];
    if (realSampleResourceCount === 0) {
      governanceItems.push({
        title: "暂无真实监控采样",
        detail: "当前没有可用于统计的真实采样点，请先确认云账号、地域、权限和云监控接口返回。",
        status: "missing",
      });
    }
    if (statusCounts.missing > 0) {
      governanceItems.push({
        title: "存在未同步资源",
        detail: `${statusCounts.missing} 个资源尚未形成采样，优先检查账号权限、地域发现和云监控插件状态。`,
        status: "missing",
      });
    }
    if (statusCounts.stale > 0) {
      governanceItems.push({
        title: "存在偏旧采样",
        detail: `${statusCounts.stale} 个资源最近一轮未拿到新采样，需要关注云监控接口返回和采样周期。`,
        status: "stale",
      });
    }
    if (statusCounts.offline > 0) {
      governanceItems.push({
        title: "存在离线资源",
        detail: `${statusCounts.offline} 个资源离线，需要结合资源状态判断是否继续纳入监控统计。`,
        status: "offline",
      });
    }
    if (!governanceItems.length) {
      governanceItems.push({
        title: "全局采样状态正常",
        detail: "当前资源已有可用采样，后续重点保持账号权限和地域发现稳定。",
        status: "normal",
      });
    }

    return {
      total,
      covered,
      actionable: statusCounts.stale + statusCounts.missing,
      statusCounts,
      coverageText: formatCoveragePercent(covered, Math.max(1, total)),
      realSampleResourceCount,
      hasRealSamples: realSampleResourceCount > 0,
      latestTimestamp,
      pointCount,
      accountRows,
      regionRows,
      sourceRows,
      governanceItems,
    };
  }, [getAccount, overviewByServerId, servers]);

  const regionRows = useMemo(() => {
    const map = new Map<string, { provider: Provider; region: string; total: number; abnormal: number; agents: number }>();
    servers.forEach((server) => {
      const key = `${server.provider}:${server.region}`;
      const current = map.get(key) ?? { provider: server.provider, region: server.region, total: 0, abnormal: 0, agents: 0 };
      current.total += 1;
      if (server.status !== "running") current.abnormal += 1;
      if (server.agentStatus === "online") current.agents += 1;
      map.set(key, current);
    });
    return [...map.values()];
  }, [servers]);

  const renderMetricSortButton = (key: ServerSortKey) => {
    const active = metricSort?.key === key;
    const directionText = active ? (metricSort.direction === "asc" ? "升序" : "降序") : "排序";
    return (
      <button
        className={active ? "sort-button active" : "sort-button"}
        type="button"
        onClick={() => toggleMetricSort(key)}
        title={`按${metricSortLabels[key]}${directionText}`}
      >
        <span>{metricSortLabels[key]}</span>
        <b>{active ? (metricSort.direction === "asc" ? "↑" : "↓") : "↕"}</b>
      </button>
    );
  };

  const renderServerTable = (rows: CloudServer[]) => (
    <table className="data-table server-list-table">
      <colgroup>
        <col className="server-col" />
        <col className="account-col" />
        <col className="region-col" />
        <col className="ip-col" />
        <col className="status-col" />
        <col className="usage-col" />
        <col className="usage-col" />
        <col className="usage-col" />
        <col className="monitor-col" />
        <col className="expiration-col" />
      </colgroup>
      <thead>
        <tr>
          <th>资源</th>
          <th>云平台 / 账号</th>
          <th>地域 / 可用区</th>
          <th>IP</th>
          <th>状态</th>
          <th>{renderMetricSortButton("cpu")}</th>
          <th>{renderMetricSortButton("memory")}</th>
          <th>{renderMetricSortButton("disk")}</th>
          <th>监控</th>
          <th>{renderMetricSortButton("expiration")}</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((server) => {
          const account = getAccount(server.accountId);
          const resourceType = getResourceType(server);
          const expiration = resolveExpirationInfo(server);
          const expirationShortText = compactExpirationText(expiration);
          const expirationSubText = expiration.dateText
            ? expiration.dateText
            : expiration.status === "no_expiration"
              ? expiration.chargeText
              : expiration.status === "unknown"
                ? "待云厂商返回"
                : expiration.chargeText;
          return (
            <tr
              className={selectedServer.id === server.id ? "selected" : ""}
              key={server.id}
              onClick={() => {
                setSelectedServerId(server.id);
              }}
            >
              <td className="server-name-cell">
                <strong>{server.name}</strong>
                <span>{server.instanceId}</span>
                <span>{resourceTypeLabels[resourceType]} / {server.business}</span>
              </td>
              <td className="account-cell">
                <strong>{providerLabels[server.provider]}</strong>
                <span>{account?.alias ?? "--"}</span>
              </td>
              <td className="region-cell">
                <strong>{server.region}</strong>
                <span>{server.zone}</span>
              </td>
              <td className="ip-cell">
                {renderResourceAddressCell(server)}
              </td>
              <td>
                <StatusText status={server.status}>{serverStatusLabels[server.status]}</StatusText>
              </td>
              <td><Utilization value={server.cpu} /></td>
              <td><Utilization value={server.memory} /></td>
              <td><Utilization value={server.disk} /></td>
              <td className="monitor-cell" title={`${agentStatusLabels[server.agentStatus]} / ${server.lastSeen}`}>
                <StatusText status={server.agentStatus}>{agentStatusLabels[server.agentStatus]}</StatusText>
              </td>
              <td className="expiration-cell" title={expiration.dateText ? `${expiration.text}，到期日 ${expiration.dateText}` : expiration.message || expiration.text}>
                <StatusText status={expiration.status}>{expirationShortText}</StatusText>
                <span>{expirationSubText}</span>
              </td>
            </tr>
          );
        })}
        {!rows.length ? (
          <tr>
            <td className="empty-cell" colSpan={10}>当前筛选条件下没有云资源</td>
          </tr>
        ) : null}
      </tbody>
    </table>
  );

  const renderResourcePicker = (products: ResourceProductNode[], emptyText: string) => (
    <div className="resource-picker">
      {products.map((product) => {
        const productKey = `monitorPicker:product:${product.key}`;
        const productHasSelected = product.rows.some((server) => server.id === selectedServer.id);
        const productExpanded = expandedTree[productKey] ?? productHasSelected;
        return (
          <div className="tree-group" key={product.key}>
            <div className="tree-line">
              <button className={productExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(productKey, productHasSelected)} aria-label={productExpanded ? "收起产品" : "展开产品"}>
                <span className="tree-caret" />
              </button>
              <button
                className="tree-node product"
                type="button"
                onClick={() => toggleTreeNode(productKey, productHasSelected)}
              >
                <span className="tree-label">{resourceTypeLabels[product.resourceType]}</span>
                <em>{product.rows.length}</em>
              </button>
            </div>
            {productExpanded
              ? product.providers.map((providerNode) => {
                  const providerKey = `monitorPicker:provider:${providerNode.key}`;
                  const providerHasSelected = providerNode.rows.some((server) => server.id === selectedServer.id);
                  const providerExpanded = expandedTree[providerKey] ?? providerHasSelected;
                  return (
                    <div className="provider-branch" key={providerNode.key}>
                      <div className="tree-line">
                        <button className={providerExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(providerKey, providerHasSelected)} aria-label={providerExpanded ? "收起云平台" : "展开云平台"}>
                          <span className="tree-caret" />
                        </button>
                        <button
                          className="tree-node provider"
                          type="button"
                          onClick={() => toggleTreeNode(providerKey, providerHasSelected)}
                        >
                          <span className="tree-label">{providerLabels[providerNode.provider]}</span>
                          <em>{providerNode.rows.length}</em>
                        </button>
                      </div>
                      {providerExpanded
                        ? providerNode.accounts.map((accountNode) => {
                            const accountKey = `monitorPicker:account:${accountNode.key}`;
                            const accountHasSelected = accountNode.rows.some((server) => server.id === selectedServer.id);
                            const accountExpanded = expandedTree[accountKey] ?? accountHasSelected;
                            return (
                              <div className="account-branch" key={accountNode.key}>
                                <div className="tree-line">
                                  <button className={accountExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(accountKey, accountHasSelected)} aria-label={accountExpanded ? "收起账号" : "展开账号"}>
                                    <span className="tree-caret" />
                                  </button>
                                  <button
                                    className="tree-node account"
                                    type="button"
                                    onClick={() => toggleTreeNode(accountKey, accountHasSelected)}
                                  >
                                    <span className="tree-label">{accountNode.accountName}</span>
                                    <em>{accountNode.rows.length}</em>
                                  </button>
                                </div>
                                {accountExpanded
                                  ? [...new Set(accountNode.rows.map((server) => server.region))].map((region) => {
                                      const regionKey = `monitorPicker:region:${accountNode.key}:${region}`;
                                      const regionRows = accountNode.rows.filter((server) => server.region === region);
                                      const regionHasSelected = regionRows.some((server) => server.id === selectedServer.id);
                                      const regionExpanded = expandedTree[regionKey] ?? regionHasSelected;
                                      return (
                                        <div className="region-branch" key={`${accountNode.key}:${region}`}>
                                          <div className="tree-line">
                                            <button className={regionExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(regionKey, regionHasSelected)} aria-label={regionExpanded ? "收起地域" : "展开地域"}>
                                              <span className="tree-caret" />
                                            </button>
                                            <button
                                              className="tree-node region"
                                              type="button"
                                              onClick={() => toggleTreeNode(regionKey, regionHasSelected)}
                                            >
                                              <span className="tree-label">{region}</span>
                                              <em>{regionRows.length}</em>
                                            </button>
                                          </div>
                                          {regionExpanded
                                            ? regionRows.map((server) => {
                                                const isSelected = selectedServer.id === server.id;
                                                return (
                                                  <button
                                                    className={[
                                                      "tree-node",
                                                      "server",
                                                      activePage === "agents" ? "with-state" : "",
                                                      isSelected ? "active" : "",
                                                    ].filter(Boolean).join(" ")}
                                                    type="button"
                                                    key={server.id}
                                                    onClick={() => {
                                                      setSelectedServerId(server.id);
                                                    }}
                                                  >
                                                    <span className="tree-label">{server.name}</span>
                                                    {activePage === "agents" ? <StatusText status={server.agentStatus}>{agentStatusLabels[server.agentStatus]}</StatusText> : null}
                                                  </button>
                                                );
                                              })
                                            : null}
                                        </div>
                                      );
                                    })
                                  : null}
                              </div>
                            );
                          })
                        : null}
                    </div>
                  );
                })
              : null}
          </div>
        );
      })}
      {!products.length ? <div className="empty-card">{emptyText}</div> : null}
    </div>
  );

  const renderScopeTree = () => (
    <aside className="resource-tree" aria-label="资源范围">
      <div className="resource-tree-title">
        <strong>资源范围</strong>
        <span>按云资产 / 云平台 / 账号 / 地域筛选</span>
      </div>
      <button className={scope === buildScopeKey("all") ? "tree-node root active" : "tree-node root"} onClick={() => selectScope(buildScopeKey("all"))}>
        <span className="tree-label">全部云资产</span>
      </button>
      {resourceTypes.map((resourceType) => {
        const productKey = `product:${resourceType}`;
        const productExpanded = isTreeExpanded(productKey);
        const productServers = servers.filter((server) => getResourceType(server) === resourceType);
        if (!productServers.length) return null;
        return (
          <div className="tree-group" key={resourceType}>
            <div className="tree-line">
              <button className={productExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(productKey)} aria-label={productExpanded ? "收起云资产" : "展开云资产"}>
                <span className="tree-caret" />
              </button>
              <button
                className={scope === buildScopeKey("product", resourceType) ? "tree-node product active" : "tree-node product"}
                onClick={() => selectScope(buildScopeKey("product", resourceType))}
              >
                <span className="tree-label">{resourceTypeLabels[resourceType]}</span>
              </button>
            </div>
            {productExpanded
              ? (
                (["aliyun", "huawei", "tencent"] as Provider[]).map((provider) => {
                  const providerKey = `productProvider:${resourceType}:${provider}`;
                  const providerExpanded = isTreeExpanded(providerKey);
                  const providerServers = productServers.filter((server) => server.provider === provider);
                  const providerAccounts = accounts.filter((account) => account.provider === provider && providerServers.some((server) => server.accountId === account.id));
                  if (!providerServers.length) return null;
                  return (
                    <div className="provider-branch" key={`${resourceType}:${provider}`}>
                      <div className="tree-line">
                        <button className={providerExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(providerKey)} aria-label={providerExpanded ? "收起云平台" : "展开云平台"}>
                          <span className="tree-caret" />
                        </button>
                        <button
                          className={scope === buildScopeKey("productProvider", `${resourceType}:${provider}`) ? "tree-node provider active" : "tree-node provider"}
                          onClick={() => selectScope(buildScopeKey("productProvider", `${resourceType}:${provider}`))}
                        >
                          <span className="tree-label">{providerLabels[provider]}</span>
                        </button>
                      </div>
                      {providerExpanded
                        ? providerAccounts.map((account) => {
                            const accountKey = `productAccount:${resourceType}:${provider}:${account.id}`;
                            const accountExpanded = isTreeExpanded(accountKey);
                            const accountServers = providerServers.filter((server) => server.accountId === account.id);
                            const accountRegions = [...new Set(accountServers.map((server) => server.region))];
                            return (
                              <div className="account-branch" key={`${resourceType}:${provider}:${account.id}`}>
                                <div className="tree-line">
                                  <button className={accountExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(accountKey)} aria-label={accountExpanded ? "收起账号" : "展开账号"}>
                                    <span className="tree-caret" />
                                  </button>
                                  <button
                                    className={scope === buildScopeKey("productAccount", `${resourceType}:${provider}:${account.id}`) ? "tree-node account active" : "tree-node account"}
                                    onClick={() => selectScope(buildScopeKey("productAccount", `${resourceType}:${provider}:${account.id}`))}
                                  >
                                    <span className="tree-label">{account.name}</span>
                                  </button>
                                </div>
                                {accountExpanded
                                  ? accountRegions.map((region) => {
                                      const regionKey = `productAccountRegion:${resourceType}:${provider}:${account.id}:${region}`;
                                      const regionExpanded = isTreeExpanded(regionKey);
                                      const regionServers = accountServers.filter((server) => server.region === region);
                                      return (
                                        <div className="region-branch" key={`${resourceType}:${provider}:${account.id}:${region}`}>
                                          <div className="tree-line">
                                            <button className={regionExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(regionKey)} aria-label={regionExpanded ? "收起地域" : "展开地域"}>
                                              <span className="tree-caret" />
                                            </button>
                                            <button
                                              className={scope === buildScopeKey("productAccountRegion", `${resourceType}:${provider}:${account.id}:${region}`) ? "tree-node region active" : "tree-node region"}
                                              onClick={() => selectScope(buildScopeKey("productAccountRegion", `${resourceType}:${provider}:${account.id}:${region}`))}
                                            >
                                              <span className="tree-label">{region}</span>
                                            </button>
                                          </div>
                                          {regionExpanded
                                            ? regionServers.map((server) => (
                                                <button
                                                  className={selectedServer.id === server.id ? "tree-node server active" : "tree-node server"}
                                                  key={server.id}
                                                  onClick={() => {
                                                    selectScope(buildScopeKey("server", server.id));
                                                  }}
                                                >
                                                  <span className="tree-label">{server.name}</span>
                                                </button>
                                              ))
                                            : null}
                                        </div>
                                      );
                                    })
                                  : null}
                              </div>
                            );
                          })
                        : null}
                    </div>
                  );
                })
              )
              : null}
          </div>
        );
      })}
    </aside>
  );

  const renderServersPage = () => {
    const activeSection = getActiveSection("servers");
    const selectedExpiration = resolveExpirationInfo(selectedServer);
    const selectedResourceType = getResourceType(selectedServer);
    const selectedExpirationDetail = selectedExpiration.dateText
      ? `${selectedExpiration.text} / 到期 ${selectedExpiration.dateText}`
      : selectedExpiration.message || selectedExpiration.text;
    return (
    <div className={activeSection === "list" ? "asset-page asset-page-with-tree" : "asset-page"}>
      {activeSection === "list" ? renderScopeTree() : null}
      <section className="page-main-section">
        {activeSection === "overview" ? (
        <div className="resource-overview-board">
          <section className="resource-overview-hero">
            <div>
              <span>资源可见性</span>
              <strong>{servers.length}</strong>
              <em>当前接入 ECS/RDS 资源</em>
            </div>
            <button type="button" onClick={() => setActiveSection("servers", "list")}>查看清单</button>
          </section>
          <section className="resource-kpi-grid">
            <button type="button" onClick={() => { setResourceFilter("ecs"); setActiveSection("servers", "list"); }}><b>{summary.ecsCount}</b><span>ECS 实例</span></button>
            <button type="button" onClick={() => { setResourceFilter("rds"); setActiveSection("servers", "list"); }}><b>{summary.rdsCount}</b><span>RDS 实例</span></button>
            <button type="button" onClick={() => { setStatusFilter("warning"); setActiveSection("servers", "list"); }}><b>{summary.warningServers}</b><span>异常资源</span></button>
            <button type="button" onClick={() => { setAgentFilter("stale"); setActiveSection("servers", "list"); }}><b>{summary.agentIssues}</b><span>监控异常</span></button>
            <button type="button" onClick={() => setActiveSection("servers", "list")}><b>{expirationSummary.expiring}</b><span>30 天内到期</span></button>
            <button type="button" onClick={() => setActivePage("accounts")}><b>{accounts.length}</b><span>云账号</span></button>
          </section>
          <section className="resource-overview-grid">
            <article className="resource-overview-panel">
              <div className="section-title">
                <h3>同步状态</h3>
                <span>{assetLoading ? "正在同步云资源" : assetError ? `同步异常：${assetError}` : `${lastSyncAt}，${refreshSummary}`}</span>
              </div>
              {assetIssues.length ? (
                <div className="asset-issue-list compact" aria-label="云资源同步问题">
                  {assetIssues.slice(0, 4).map((issue) => (
                    <span key={issue.id}>
                      <b>{providerLabels[issue.provider]} / {issue.accountName}</b>
                      {resourceTypeLabels[issue.resourceType]}：{issue.message}
                    </span>
                  ))}
                </div>
              ) : <div className="resource-empty-hint">暂无局部同步问题</div>}
            </article>
            <article className="resource-overview-panel">
              <div className="section-title">
                <h3>最近关注</h3>
                <span>按健康分和提醒数量优先处理</span>
              </div>
              <div className="resource-focus-list">
                {resourceHealthRows.slice(0, 5).map((row) => (
                  <button type="button" key={row.server.id} onClick={() => { setSelectedServerId(row.server.id); setActiveSection("servers", "detail"); }}>
                    <strong>{row.server.name}</strong>
                    <span>{healthLevelLabels[row.level]} / {row.score} 分 / {row.reminders.length} 条提醒</span>
                  </button>
                ))}
                {!resourceHealthRows.length ? <div className="resource-empty-hint">当前没有高风险资源</div> : null}
              </div>
            </article>
          </section>
        </div>
        ) : null}

        {activeSection === "list" ? (
        <>
        <div className="toolbar">
          <input value={keyword} onChange={(event) => setKeyword(event.currentTarget.value)} placeholder="搜索资源、账号、IP、业务或地域" />
          <select value={resourceFilter} onChange={(event) => setResourceFilter(event.currentTarget.value as ResourceType | "all")}>
            <option value="all">全部产品</option>
            <option value="ecs">只看 ECS</option>
            <option value="rds">只看 RDS</option>
          </select>
          <select value={statusFilter} onChange={(event) => setStatusFilter(event.currentTarget.value as ServerStatus | "all")}>
            <option value="all">全部状态</option>
            <option value="running">运行中</option>
            <option value="warning">异常</option>
            <option value="offline">离线</option>
            <option value="maintenance">维护中</option>
          </select>
          <select value={agentFilter} onChange={(event) => setAgentFilter(event.currentTarget.value as AgentStatus | "all")}>
            <option value="all">全部监控</option>
            <option value="online">正常</option>
            <option value="stale">数据异常</option>
            <option value="missing">未同步</option>
            <option value="offline">离线</option>
          </select>
        </div>

        <div className="simple-summary">
          <span>云账号：{accounts.length}</span>
          <span>资源：{servers.length}</span>
          <span>ECS：{summary.ecsCount}</span>
          <span>RDS：{summary.rdsCount}</span>
          <span>异常资源：{summary.warningServers}</span>
          <span>监控异常：{summary.agentIssues}</span>
          <span>到期关注：{expirationSummary.expiring}（30 天内）</span>
          <span>云资源同步：{assetLoading ? "同步中" : assetError ? `异常：${assetError}` : `${lastSyncAt}，${refreshSummary}`}</span>
        </div>
        {assetIssues.length ? (
          <div className="asset-issue-list" aria-label="云资源同步问题">
            <strong>本轮局部同步问题</strong>
            {assetIssues.slice(0, 6).map((issue) => (
              <span key={issue.id}>
                <b>{providerLabels[issue.provider]} / {issue.accountName} / {resourceTypeLabels[issue.resourceType]}</b>
                {issue.scope}：{issue.message}
              </span>
            ))}
            {assetIssues.length > 6 ? <em>另有 {assetIssues.length - 6} 项问题，可刷新后继续核对</em> : null}
          </div>
        ) : null}

        <div className="table-panel">{renderServerTable(visibleServers)}</div>
        </>
        ) : null}

        {activeSection === "detail" ? (
        <section className="detail-panel">
          <div className="section-title">
            <h3>资源详情</h3>
            <span>选择表格中的云资源后在这里查看基础信息和监控状态</span>
          </div>
          <table className="detail-table">
            <tbody>
              <tr><th>资源</th><td>{selectedServer.name}</td><th>实例 ID</th><td>{selectedServer.instanceId}</td></tr>
              <tr><th>云平台</th><td>{providerLabels[selectedServer.provider]}</td><th>账号</th><td>{selectedAccount?.name ?? "--"}</td></tr>
              <tr><th>资源类型</th><td>{resourceTypeLabels[selectedResourceType]}</td><th>业务分类</th><td>{selectedServer.business}</td></tr>
              <tr><th>地域</th><td>{selectedServer.region}</td><th>可用区</th><td>{selectedServer.zone}</td></tr>
              {selectedResourceType === "rds" ? (
                <tr><th>{publicIpLabel(selectedServer)}</th><td colSpan={3}>{selectedServer.publicIp}</td></tr>
              ) : (
                <tr><th>{publicIpLabel(selectedServer)}</th><td>{selectedServer.publicIp}{selectedServer.publicIpId ? ` / ${selectedServer.publicIpId}` : ""}</td><th>内网 IP</th><td>{selectedServer.privateIp}</td></tr>
              )}
              <tr><th>{selectedResourceType === "rds" ? "引擎" : "系统"}</th><td>{selectedServer.os}</td><th>规格</th><td>{selectedServer.spec}</td></tr>
              {selectedResourceType === "rds" ? (
                <>
                  <tr><th>RDS 类型</th><td>{selectedServer.dbInstanceType || "--"}</td><th>系列 / 存储</th><td>{[selectedServer.dbCategory, selectedServer.dbStorageType].filter(Boolean).join(" / ") || "--"}</td></tr>
                  <tr><th>连接端点</th><td>{selectedServer.dbEndpointCount ?? 0} 个</td><th>性能上限</th><td>{`连接 ${selectedServer.dbMaxConnections || "--"} / IOPS ${selectedServer.dbMaxIops || "--"} / MBPS ${selectedServer.dbMaxIombps || "--"}`}</td></tr>
                  <tr><th>锁定状态</th><td>{selectedServer.dbLockMode || "--"}</td><th>锁定原因</th><td>{selectedServer.dbLockReason || "--"}</td></tr>
                </>
              ) : null}
              <tr><th>计费方式</th><td>{formatChargeType(selectedServer.chargeType, isSpotServer(selectedServer))}</td><th>到期状态</th><td>{selectedExpirationDetail}</td></tr>
              <tr><th>运行状态</th><td>{serverStatusLabels[selectedServer.status]}</td><th>{selectedResourceType === "rds" ? "性能采样" : "云监控"}</th><td>{agentStatusLabels[selectedServer.agentStatus]} / {selectedServer.lastSeen}</td></tr>
              <tr><th>{selectedResourceType === "rds" ? "QPS" : "负载"}</th><td>{selectedServer.load}</td><th>运行时长</th><td>{selectedServer.uptime}</td></tr>
            </tbody>
          </table>
          <div className="resource-context-actions" aria-label="实例异常排查入口">
            <button type="button" onClick={() => openResourceContext("monitor")}>查看监控</button>
            <button type="button" onClick={() => openResourceContext("alerts")}>关联告警</button>
            <button type="button" onClick={() => openResourceContext("ai")}>AI 摘要</button>
            <button type="button" onClick={() => openResourceContext("knowledge")}>知识库</button>
            <button type="button" onClick={() => setActiveSection("servers", "list")}>返回清单</button>
          </div>
        </section>
        ) : null}
      </section>
    </div>
    );
  };

  const renderAccountsPage = () => {
    const activeSection = getActiveSection("accounts");
    return (
    <section className="page-main-section">
      {activeSection === "list" ? (
      <div className="account-list-panel">
        <div className="section-title account-list-title">
          <div>
            <h3>已接入云平台账号</h3>
            <span>展示当前保存的云账号；刷新会重新读取账号列表、ECS/RDS 实例和监控概览</span>
          </div>
          <div className="account-title-actions">
            <small>{assetLoading ? "刷新中" : `最近刷新 ${lastSyncAt}`}</small>
            <button type="button" onClick={() => void refreshAccountsAndResources()} disabled={assetLoading}>
              {assetLoading ? "刷新中" : "刷新账号资源"}
            </button>
          </div>
        </div>
        {accountNotice ? <div className="inline-message">{accountNotice}</div> : null}
        <div className="table-panel account-table-panel">
          <table className="data-table account-table">
            <colgroup>
              <col className="account-provider-col" />
              <col className="account-name-col" />
              <col className="account-id-col" />
              <col className="account-source-col" />
              <col className="account-scope-col" />
              <col className="account-regions-col" />
              <col className="account-server-col" />
              <col className="account-status-col" />
              <col className="account-sync-col" />
              <col className="account-actions-col" />
            </colgroup>
            <thead>
              <tr>
                <th>云平台</th>
                <th>账号名称</th>
                <th>账号标识</th>
                <th>来源</th>
                <th>接入范围</th>
                <th>地域策略</th>
                <th>资源</th>
                <th>状态</th>
                <th>最近同步</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {accounts.map((account) => {
                const isDisabled = account.enabled === false;
                const isToggling = togglingAccountId === account.id;
                return (
                  <tr key={account.id}>
                    <td>{providerLabels[account.provider]}</td>
                    <td><strong>{account.name}</strong></td>
                    <td>{account.uid}</td>
                    <td>{account.owner}</td>
                    <td>{account.scope}</td>
                    <td>{account.regions.length ? account.regions.join("、") : "自动发现全部地域"}</td>
                    <td>{servers.filter((server) => server.accountId === account.id).length}</td>
                    <td><StatusText status={account.status}>{accountStatusLabels[account.status]}</StatusText></td>
                    <td>{account.lastSync}</td>
                    <td>
                      <div className="row-actions">
                        <button
                          className={isDisabled ? "link-button state-button enable" : "link-button state-button"}
                          type="button"
                          onClick={() => void toggleAccountEnabled(account)}
                          disabled={isToggling}
                        >
                          {isToggling ? "处理中" : isDisabled ? "启用" : "停用"}
                        </button>
                        <button className="link-button" type="button" onClick={() => void testAccount(account.id)} disabled={testingAccountId === account.id || isDisabled}>
                          {testingAccountId === account.id ? "校验中" : "校验"}
                        </button>
                        <button className="link-button" type="button" onClick={() => editAccount(account)}>编辑</button>
                        <button className="link-button danger" type="button" onClick={() => void deleteAccount(account.id)}>删除</button>
                      </div>
                    </td>
                  </tr>
                );
              })}
              {!accounts.length ? (
                <tr>
                  <td className="empty-cell" colSpan={10}>还没有云账号，先在下方新增一个阿里云 ECS/RDS 或华为云 ECS 账号</td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>
      ) : null}

      {activeSection === "form" ? (
      <div className="account-config-panel">
        <div className="section-title">
          <h3>{editingAccountId ? "编辑云平台账号" : "新增云平台账号"}</h3>
          <span>AccessKey Secret 只提交到后端加密保存，不会回显到前端</span>
        </div>
        <div className="account-form">
          <label>
            <span>云平台</span>
            <select value={accountForm.provider} onChange={(event) => updateAccountForm("provider", event.currentTarget.value as Provider)}>
              {supportedAccountProviders.map((provider) => (
                <option key={provider} value={provider}>{providerLabels[provider]}</option>
              ))}
            </select>
          </label>
          <label>
            <span>账号名称</span>
            <input value={accountForm.name} onChange={(event) => updateAccountForm("name", event.currentTarget.value)} placeholder="浙江万盛账号" />
          </label>
          <label>
            <span>AccessKey ID</span>
            <input value={accountForm.accessKeyId} onChange={(event) => updateAccountForm("accessKeyId", event.currentTarget.value)} placeholder={editingAccountId ? "留空沿用原值" : "LTAI..."} />
          </label>
          <label>
            <span>AccessKey Secret</span>
            <input type="password" value={accountForm.accessKeySecret} onChange={(event) => updateAccountForm("accessKeySecret", event.currentTarget.value)} placeholder={editingAccountId ? "留空沿用原值" : "AccessKey Secret"} />
          </label>
          <label>
            <span>采样周期（秒）</span>
            <input value={accountForm.metricPeriod} onChange={(event) => updateAccountForm("metricPeriod", event.currentTarget.value)} placeholder="60" />
          </label>
          <div className="account-form-actions">
            <button type="button" onClick={() => void saveAccount()} disabled={accountSaving}>
              {accountSaving ? "保存中" : editingAccountId ? "更新账号" : "保存账号"}
            </button>
            {editingAccountId ? <button type="button" onClick={cancelAccountEdit}>取消编辑</button> : null}
          </div>
        </div>
      </div>
      ) : null}
    </section>
    );
  };

  const renderRegionsPage = () => (
    <section className="page-main-section">
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>云平台</th>
              <th>地域</th>
              <th>资源数</th>
              <th>异常数</th>
              <th>探针在线</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {regionRows.map((row) => (
              <tr key={`${row.provider}:${row.region}`}>
                <td>{providerLabels[row.provider]}</td>
                <td>{row.region}</td>
                <td>{row.total}</td>
                <td>{row.abnormal}</td>
                <td>{row.agents}</td>
                <td><button className="link-button" type="button" onClick={() => {
                  selectScope(buildScopeKey("region", `${row.provider}:${row.region}`));
                  setActivePage("servers");
                }}>查看资源</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderMonitorPage = () => {
    const selectedResourceOffline = selectedServer.status === "offline";
    const overview = selectedResourceOffline ? undefined : overviewByServerId[selectedServer.id];
    const isRDS = getResourceType(selectedServer) === "rds";
    const metricRows = isRDS ? buildRDSMonitorRows(overview) : buildMonitorRows(overview);
    const freshness = resolveOverviewFreshness(overview);
    const snapshot = latestMetricSnapshot(overview);
    const account = getAccount(selectedServer.accountId);
    const expiration = resolveExpirationInfo(selectedServer);
    const chartRows = selectedResourceOffline ? [] : metricRows.filter((item) => item.status !== "empty" || item.points > 0);
    const freshnessSummary = snapshot.latestTimestamp
      ? `最近 30 分钟 ${snapshot.pointCount} 个采样点，${freshness.message}${snapshot.periodSeconds ? `，周期 ${snapshot.periodSeconds}s` : ""}`
      : selectedResourceOffline
        ? selectedServer.lastSeen || "资源已离线，监控采样不可用"
        : freshness.message;
    return (
      <section className="page-main-section">
        <div className="monitor-topline">
          <div className="monitor-stat-strip">
            <span><b>监控正常</b>{monitorSummary.online}</span>
            <span><b>数据偏旧</b>{monitorSummary.stale}</span>
            <span><b>未同步</b>{monitorSummary.missing}</span>
            <span><b>离线</b>{monitorSummary.offline}</span>
          </div>
          <div className="monitor-refresh-panel">
            <span>{refreshSummary}</span>
            <button type="button" onClick={() => void loadCloudAssets()} disabled={assetLoading}>
              {assetLoading ? "刷新中" : "立即刷新"}
            </button>
          </div>
        </div>
        <div className="monitor-workspace">
          <aside className="monitor-resource-pane">
            <div className="workspace-pane-title">
              <strong>资源选择</strong>
              <span>按产品和账号快速定位</span>
            </div>
            {renderResourcePicker(monitorResourceTree, "当前没有可展示的监控资源")}
          </aside>
          <section className="detail-panel monitor-overview-panel">
            <div className="section-title monitor-overview-title">
              <div>
                <h3>{isRDS ? "RDS 性能监控" : "ECS 云监控"}</h3>
                <span>{selectedServer.name} / {selectedServer.instanceId}</span>
              </div>
              <StatusText status={selectedServer.agentStatus}>{agentStatusLabels[selectedServer.agentStatus]}</StatusText>
            </div>
            {freshnessSummary ? (
              <div className="inline-message">{freshnessSummary}</div>
            ) : null}
            <div className="monitor-resource-summary">
              <span><b>云平台 / 账号</b><em>{providerLabels[selectedServer.provider]} / {account?.name ?? "--"}</em></span>
              <span><b>地域 / 可用区</b><em>{selectedServer.region} / {selectedServer.zone}</em></span>
              <span><b>{isRDS ? "引擎 / 规格" : "系统 / 规格"}</b><em>{selectedServer.os} / {selectedServer.spec}</em></span>
              <span><b>{publicIpLabel(selectedServer)}</b><em>{selectedServer.publicIp}</em></span>
              <span><b>到期状态</b><em>{expiration.text}</em></span>
            </div>
            <div className="monitor-chart-grid">
              {chartRows.map((item) => <MetricChartCard item={item} key={item.key} />)}
              {!chartRows.length ? (
                <div className="monitor-chart-empty">
                  <strong>暂无可展示的监控图表</strong>
                  <span>{freshness.message || "云厂商暂未返回该资源的采样点"}</span>
                </div>
              ) : null}
            </div>
          </section>
        </div>
      </section>
    );
  };

  const renderAgentsPage = () => {
    const activeSection = getActiveSection("agents");
    if (USE_MOCK) {
      return (
        <section className="page-main-section">
          <div className="agent-command-panel">
            <div className="agent-command-copy">
              <span>探针治理</span>
              <strong>看全局接入覆盖、采样质量和监控缺口</strong>
              <p>连接真实云账号后展示监控采样统计。</p>
            </div>
          </div>
          <section className="agent-governance-panel">
            <div className="agent-empty-state">暂无真实云账号数据</div>
          </section>
        </section>
      );
    }

    return (
      <section className="page-main-section">
        <div className="agent-command-panel">
          <div className="agent-command-copy">
            <span>探针治理</span>
            <strong>看全局接入覆盖、采样质量和监控缺口</strong>
            <p>汇总真实监控采样，按账号、地域和采样来源查看接入覆盖与质量。</p>
          </div>
          <div className="agent-command-actions">
            <span>{refreshSummary} / 最近同步 {lastSyncAt}</span>
            <button type="button" onClick={() => void loadCloudAssets()} disabled={assetLoading}>
              {assetLoading ? "刷新中" : "立即刷新"}
            </button>
          </div>
        </div>
        {activeSection === "overview" ? (
        <div className="agent-overview-grid">
          <article className="agent-overview-card primary">
            <span>可用采样覆盖率</span>
            <strong>{agentGovernance.coverageText}</strong>
            <em>{agentGovernance.hasRealSamples ? `${formatCompactNumber(agentGovernance.covered)} / ${formatCompactNumber(agentGovernance.total)} 个资源最新可用` : "暂无真实采样"}</em>
          </article>
          <article className="agent-overview-card">
            <span>治理待处理</span>
            <strong>{formatCompactNumber(agentGovernance.actionable)}</strong>
            <em>数据异常与未同步进入治理清单</em>
          </article>
          <article className="agent-overview-card">
            <span>真实采样点</span>
            <strong>{agentGovernance.hasRealSamples ? formatCompactNumber(agentGovernance.pointCount) : "--"}</strong>
            <em>{agentGovernance.hasRealSamples ? "最近 30 分钟窗口" : "暂无真实采样"}</em>
          </article>
          <article className="agent-overview-card">
            <span>最近采样</span>
            <strong>{agentGovernance.latestTimestamp ? formatMetricShortTime(agentGovernance.latestTimestamp) : "--"}</strong>
            <em>{agentGovernance.latestTimestamp ? formatMetricTime(agentGovernance.latestTimestamp) : "暂无真实采样"}</em>
          </article>
        </div>
        ) : null}
        {activeSection === "overview" ? (
        <div className="agent-status-lanes">
          {agentStatusOrder.map((status) => {
            const count = agentGovernance.statusCounts[status];
            const percent = formatCoveragePercent(count, Math.max(1, agentGovernance.total));
            return (
              <article className={`agent-status-lane agent-status-lane-${status}`} key={status}>
                <div>
                  <StatusText status={status}>{agentStatusLabels[status]}</StatusText>
                  <span>{agentStatusDescriptions[status]}</span>
                </div>
                <strong>{count}</strong>
                <i style={{ width: percent }} />
              </article>
            );
          })}
        </div>
        ) : null}
        <div className="agent-governance-grid">
          {activeSection === "actions" ? (
          <section className="agent-governance-panel agent-governance-panel-wide">
            <div className="agent-panel-title">
              <strong>治理提醒</strong>
              <span>从全局角度判断下一步该先补哪里</span>
            </div>
            <div className="agent-governance-list">
              {agentGovernance.governanceItems.map((item) => (
                <article className="agent-governance-item" key={`${item.status}:${item.title}`}>
                  <StatusText status={item.status}>{item.status === "normal" ? "正常" : agentStatusLabels[item.status]}</StatusText>
                  <div>
                    <strong>{item.title}</strong>
                    <span>{item.detail}</span>
                  </div>
                </article>
              ))}
            </div>
          </section>
          ) : null}
          {activeSection === "accounts" ? (
          <section className="agent-governance-panel">
            <div className="agent-panel-title">
              <strong>账号覆盖</strong>
              <span>按云账号查看采样覆盖和缺口</span>
            </div>
            <div className="agent-summary-list">
              {agentGovernance.accountRows.slice(0, 6).map((row) => (
                <article className="agent-summary-row" key={row.key}>
                  <div>
                    <strong>{row.accountName}</strong>
                    <span>{providerLabels[row.provider]} / {row.regionCount} 个地域 / {row.resourceTypes.map((type) => resourceTypeLabels[type]).join("、")}</span>
                  </div>
                  <div className="agent-summary-metrics">
                    <span><b>{formatCoveragePercent(row.covered, row.total)}</b>覆盖</span>
                    <span><b>{row.riskCount}</b>待处理</span>
                    <span><b>{row.pointCount ? formatCompactNumber(row.pointCount) : "--"}</b>真实采样</span>
                  </div>
                  <div className="agent-progress-track">
                    <i style={{ width: formatCoveragePercent(row.covered, row.total) }} />
                  </div>
                </article>
              ))}
              {!agentGovernance.accountRows.length ? <div className="agent-empty-state">暂无云账号资源</div> : null}
            </div>
          </section>
          ) : null}
          {activeSection === "regions" ? (
          <section className="agent-governance-panel">
            <div className="agent-panel-title">
              <strong>地域缺口</strong>
              <span>按地域看采样覆盖和风险集中度</span>
            </div>
            <div className="agent-summary-list">
              {agentGovernance.regionRows.slice(0, 6).map((row) => (
                <article className="agent-summary-row" key={row.key}>
                  <div>
                    <strong>{row.region}</strong>
                    <span>{providerLabels[row.provider]} / {row.accountCount} 个账号 / {row.total} 个资源</span>
                  </div>
                  <div className="agent-summary-metrics">
                    <span><b>{formatCoveragePercent(row.covered, row.total)}</b>覆盖</span>
                    <span><b>{row.riskCount}</b>待处理</span>
                    <span><b>{row.latestTimestamp ? formatMetricShortTime(row.latestTimestamp) : "--"}</b>最近</span>
                  </div>
                  <div className="agent-progress-track">
                    <i style={{ width: formatCoveragePercent(row.covered, row.total) }} />
                  </div>
                </article>
              ))}
              {!agentGovernance.regionRows.length ? <div className="agent-empty-state">暂无地域资源</div> : null}
            </div>
          </section>
          ) : null}
          {activeSection === "sources" ? (
          <section className="agent-governance-panel agent-governance-panel-wide">
            <div className="agent-panel-title">
              <strong>采样来源质量</strong>
              <span>看监控数据来自哪里，以及是否形成稳定采样</span>
            </div>
            <div className="agent-source-grid">
              {agentGovernance.sourceRows.map((row) => (
                <article className="agent-source-card" key={row.source}>
                  <span>{row.source}</span>
                  <strong>{formatCompactNumber(row.resourceCount)} 个资源</strong>
                  <em>{formatCompactNumber(row.pointCount)} 个真实采样 / {row.periods.length ? row.periods.join("、") : "周期未知"}</em>
                  <small>{row.latestTimestamp ? `最近 ${formatMetricShortTime(row.latestTimestamp)}` : "暂无真实采样"}</small>
                </article>
              ))}
              {!agentGovernance.sourceRows.length ? <div className="agent-empty-state">暂无真实采样来源</div> : null}
            </div>
          </section>
          ) : null}
        </div>
      </section>
    );
  };

  const renderEventsPage = () => (
    <section className="page-main-section alert-management-page">
      <div className="alert-management-topline">
        <div className="monitor-stat-strip alert-stat-strip">
          <button className={alertFilter.severity === "critical" ? "active" : ""} type="button" onClick={() => { setActiveAlertSection("reminders"); setAlertFilter({ severity: "critical" }); }}><b>严重提醒</b>{cloudAlertSummary.critical}</button>
          <button className={alertFilter.severity === "warning" ? "active" : ""} type="button" onClick={() => { setActiveAlertSection("reminders"); setAlertFilter({ severity: "warning" }); }}><b>预警提醒</b>{cloudAlertSummary.warning}</button>
          <button className={alertFilter.severity === "info" ? "active" : ""} type="button" onClick={() => { setActiveAlertSection("reminders"); setAlertFilter({ severity: "info" }); }}><b>提示提醒</b>{cloudAlertSummary.info}</button>
          <button className={alertFilter.serverId ? "active" : ""} type="button" onClick={() => { setActiveAlertSection("reminders"); setAlertFilter({}); }}><b>影响资源</b>{cloudAlertSummary.affectedResources}</button>
        </div>
        <div className="monitor-refresh-panel">
          <span>{refreshSummary}</span>
          <button type="button" onClick={() => void loadCloudAssets()} disabled={assetLoading}>
            {assetLoading ? "刷新中" : "刷新告警数据"}
          </button>
        </div>
      </div>

      {activeAlertSection === "health" ? (
      <section className="detail-panel">
        <div className="section-title">
          <h3>高风险资源</h3>
          <span>按运行状态、监控采样、用量水位、到期和公网暴露计算健康分</span>
        </div>
        <div className="table-panel">
          <table className="data-table resource-health-table">
            <thead>
              <tr>
                <th>健康分</th>
                <th>资源</th>
                <th>云平台 / 账号 / 地域</th>
                <th>状态</th>
                <th>主要原因</th>
                <th>提醒数</th>
              </tr>
            </thead>
            <tbody>
              {resourceHealthRows.map((row) => {
                const account = getAccount(row.server.accountId);
                const resourceType = getResourceType(row.server);
                return (
                  <tr className={alertFilter.serverId === row.server.id ? "clickable-row active" : "clickable-row"} key={row.server.id} onClick={() => { setActiveAlertSection("reminders"); setAlertFilter({ serverId: row.server.id }); }}>
                    <td>
                      <strong>{row.score}</strong>
                      <span> / 100</span>
                    </td>
                    <td className="server-name-cell">
                      <strong>{row.server.name}</strong>
                      <span>{resourceTypeLabels[resourceType]} / {row.server.instanceId}</span>
                    </td>
                    <td className="account-cell">
                      <strong>{providerLabels[row.server.provider]}</strong>
                      <span>{account?.alias ?? "--"} / {row.server.region}</span>
                    </td>
                    <td>
                      <StatusText status={healthLevelStatus[row.level]}>{healthLevelLabels[row.level]}</StatusText>
                    </td>
                    <td>{row.reasons.length ? row.reasons.join("、") : "暂无风险项"}</td>
                    <td>{row.reminders.length}</td>
                  </tr>
                );
              })}
              {!resourceHealthRows.length ? (
                <tr>
                  <td className="empty-cell" colSpan={6}>当前资源健康分均为正常</td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
      ) : null}

      {activeAlertSection === "reminders" ? (
      <section className="detail-panel">
        <div className="section-title">
          <h3>云产品提醒</h3>
          <span>
            {alertFilterText ? `当前筛选：${alertFilterText}，共 ${filteredCloudAlertReminders.length} 条` : "点击顶部统计、规则或覆盖分组筛选提醒"}
            {alertFilterText ? <button className="inline-clear-button" type="button" onClick={() => setAlertFilter({})}>清除筛选</button> : null}
          </span>
        </div>
        <div className="table-panel">
          <table className="data-table cloud-alert-table">
            <colgroup>
              <col className="alert-severity-col" />
              <col className="alert-resource-col" />
              <col className="alert-account-col" />
              <col className="alert-metric-col" />
              <col className="alert-message-col" />
              <col className="alert-suggestion-col" />
              <col className="alert-time-col" />
            </colgroup>
            <thead>
              <tr>
                <th>级别</th>
                <th>资源</th>
                <th>云平台 / 账号 / 地域</th>
                <th>指标 / 当前值</th>
                <th>提醒内容</th>
                <th>建议动作</th>
                <th>更新时间</th>
              </tr>
            </thead>
            <tbody>
              {filteredCloudAlertReminders.map((reminder) => {
                const account = getAccount(reminder.accountId);
                return (
                  <tr key={reminder.id}>
                    <td><StatusText status={reminder.severity}>{alertSeverityLabels[reminder.severity]}</StatusText></td>
                    <td className="server-name-cell">
                      <strong>{reminder.resourceName}</strong>
                      <span>{resourceTypeLabels[reminder.resourceType]} / {reminder.instanceId}</span>
                    </td>
                    <td className="account-cell">
                      <strong>{providerLabels[reminder.provider]}</strong>
                      <span>{account?.alias ?? "--"} / {reminder.region}</span>
                    </td>
                    <td>
                      <strong>{reminder.metric}</strong>
                      <span>{reminder.currentValue}</span>
                      <span>{reminder.condition}</span>
                    </td>
                    <td>{reminder.message}</td>
                    <td>{reminder.suggestion}</td>
                    <td>{reminder.updatedAt}</td>
                  </tr>
                );
              })}
              {!filteredCloudAlertReminders.length ? (
                <tr>
                  <td className="empty-cell" colSpan={7}>{alertFilterText ? "当前筛选下没有提醒" : "当前接入云产品没有命中提醒规则"}</td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>
      ) : null}

      {activeAlertSection === "rules" || activeAlertSection === "coverage" ? (
      <div className="alert-management-grid single">
        {activeAlertSection === "rules" ? (
        <section className="detail-panel">
          <div className="section-title">
            <h3>规则配置</h3>
            <span>在控制台调整启用状态、级别、阈值和通知渠道，当前修改会立即影响本页提醒结果</span>
          </div>
          <div className="alert-rule-summary">
            <span><b>{alertRuleSummary.enabledRules}</b>启用规则</span>
            <span><b>{alertRuleSummary.hitRules}</b>命中规则</span>
            <span><b>{alertRuleSummary.activeReminders}</b>当前提醒</span>
            <button className="link-button" type="button" onClick={() => setAlertRuleConfigs(buildInitialAlertRuleConfigs())}>
              恢复默认
            </button>
          </div>
          <div className="alert-rule-editor">
            <div className="alert-rule-list" aria-label="告警规则列表">
              {alertRuleRows.map((rule) => (
                <button
                  className={selectedAlertRule?.id === rule.id ? "alert-rule-list-item active" : "alert-rule-list-item"}
                  key={rule.id}
                  type="button"
                  onClick={() => {
                    setSelectedAlertRuleId(rule.id);
                    setAlertFilter({ ruleId: rule.id });
                  }}
                >
                  <div>
                    <strong>{rule.title}</strong>
                    <span>{rule.metric} / 命中 {rule.activeCount}</span>
                  </div>
                  <StatusText status={rule.enabled ? rule.severity : "disabled"}>
                    {rule.enabled ? alertSeverityLabels[rule.severity] : "停用"}
                  </StatusText>
                </button>
              ))}
            </div>
            {selectedAlertRule ? (
              <div className="alert-rule-form">
                <div className="alert-rule-form-head">
                  <div>
                    <strong>{selectedAlertRule.title}</strong>
                    <span>{selectedAlertRule.metric} / {selectedAlertRule.condition}</span>
                  </div>
                  <label className="switch-row">
                    <input
                      type="checkbox"
                      checked={selectedAlertRule.enabled}
                      onChange={(event) => updateAlertRuleConfig(selectedAlertRule.id, { enabled: event.currentTarget.checked })}
                    />
                    <span>启用</span>
                  </label>
                </div>
                <div className="alert-rule-form-grid">
                  <label>
                    <span>默认级别</span>
                    <select
                      value={selectedAlertRule.severity}
                      onChange={(event) => updateAlertRuleConfig(selectedAlertRule.id, { severity: event.currentTarget.value as CloudAlertSeverity })}
                    >
                      <option value="info">提示</option>
                      <option value="warning">预警</option>
                      <option value="critical">严重</option>
                    </select>
                  </label>
                  {typeof selectedAlertRule.warningThreshold === "number" ? (
                    <label>
                      <span>{selectedAlertRule.id.startsWith("metric-") ? "预警阈值（%）" : "预警阈值（天）"}</span>
                      <input
                        min={0}
                        type="number"
                        value={selectedAlertRule.warningThreshold}
                        onChange={(event) => updateAlertRuleThreshold(selectedAlertRule.id, "warningThreshold", event.currentTarget.value)}
                      />
                    </label>
                  ) : null}
                  {typeof selectedAlertRule.criticalThreshold === "number" ? (
                    <label>
                      <span>严重阈值（%）</span>
                      <input
                        min={0}
                        type="number"
                        value={selectedAlertRule.criticalThreshold}
                        onChange={(event) => updateAlertRuleThreshold(selectedAlertRule.id, "criticalThreshold", event.currentTarget.value)}
                      />
                    </label>
                  ) : null}
                  <label className="alert-rule-channel-field">
                    <span>通知渠道</span>
                    <input
                      value={selectedAlertRule.notifyChannels.join("，")}
                      onChange={(event) => updateAlertRuleChannels(selectedAlertRule.id, event.currentTarget.value)}
                      placeholder="控制台，钉钉，邮件"
                    />
                  </label>
                </div>
                <div className="alert-rule-item-meta">
                  <span><b>覆盖产品</b>{selectedAlertRule.resourceTypes.map((type) => resourceTypeLabels[type]).join("、")}</span>
                  <span><b>云平台</b>{selectedAlertRule.providers.map((provider) => providerLabels[provider]).join("、")}</span>
                  <span><b>当前命中</b>{selectedAlertRule.activeCount}</span>
                  <span><b>说明</b>当前为页面内配置，刷新后会恢复默认；后续可接后端持久化接口</span>
                </div>
              </div>
            ) : null}
          </div>
        </section>
        ) : null}

        {activeAlertSection === "coverage" ? (
        <section className="detail-panel">
          <div className="section-title">
            <h3>覆盖情况</h3>
            <span>按云产品、云账号聚合当前规则覆盖和提醒数量</span>
          </div>
          <div className="table-panel">
            <table className="data-table alert-coverage-table">
              <thead>
                <tr>
                  <th>云平台</th>
                  <th>产品</th>
                  <th>账号</th>
                  <th>资源数</th>
                  <th>影响资源</th>
                  <th>严重 / 预警</th>
                </tr>
              </thead>
              <tbody>
                {alertCoverageRows.map((row) => (
                  <tr className={alertFilter.coverageKey === row.key ? "clickable-row active" : "clickable-row"} key={row.key} onClick={() => { setActiveAlertSection("reminders"); setAlertFilter({ coverageKey: row.key }); }}>
                    <td>{providerLabels[row.provider]}</td>
                    <td>{resourceTypeLabels[row.resourceType]}</td>
                    <td>{row.accountName}</td>
                    <td>{row.total}</td>
                    <td>{row.affected}</td>
                    <td>{row.critical} / {row.warning}</td>
                  </tr>
                ))}
                {!alertCoverageRows.length ? (
                  <tr>
                    <td className="empty-cell" colSpan={6}>暂无可覆盖的云资源</td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </section>
        ) : null}
      </div>
      ) : null}
    </section>
  );

  const renderAiPage = () => {
    const activeSection = getActiveSection("ai");
    return (
    <section className="page-main-section">
      {activeSection === "scope" ? (
      <div className="simple-summary">
        <span><b>定位</b>只服务异常资源，不做独立 AI 工作台</span>
        <span><b>输入</b>告警、指标、资源状态、采样新鲜度</span>
        <span><b>输出</b>影响判断、可能原因、下一步检查</span>
      </div>
      ) : null}
      {activeSection === "analysis" ? (
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>资源</th>
              <th>触发依据</th>
              <th>辅助结论</th>
              <th>建议动作</th>
            </tr>
          </thead>
          <tbody>
            {servers.filter((server) => server.ai > 0 || server.alerts > 0).map((server) => (
              <tr key={server.id}>
                <td>{server.name}</td>
                <td>{server.alerts > 0 ? `关联 ${server.alerts} 条告警` : `已有 ${server.ai} 次分析记录`}</td>
                <td>{server.alerts > 0 ? "需要结合监控曲线、同步状态和知识命中判断是否真实影响业务" : "暂无持续风险，保留为排查上下文"}</td>
                <td>从资源详情进入关联告警，确认指标时间、账号权限和最近同步结果</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      ) : null}
    </section>
    );
  };

  const renderKnowledgePage = () => {
    const activeSection = getActiveSection("knowledge");
    return (
    <section className="page-main-section">
      {activeSection === "scope" ? (
      <div className="simple-summary">
        <span><b>定位</b>只沉淀可复用处置步骤</span>
        <span><b>来源</b>告警结案、资源治理、故障复盘</span>
        <span><b>回流</b>命中后反哺异常排查页</span>
      </div>
      ) : null}
      {activeSection === "entries" ? (
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>场景</th>
              <th>适用范围</th>
              <th>沉淀内容</th>
              <th>使用方式</th>
            </tr>
          </thead>
          <tbody>
            <tr><td>磁盘水位高</td><td>ECS / RDS</td><td>清理、扩容、回滚和复查步骤</td><td>资源详情命中后展示为处置建议</td></tr>
            <tr><td>监控采样缺失</td><td>多云通用</td><td>账号权限、地域、指标名和采样周期检查</td><td>同步失败或探针异常时优先推荐</td></tr>
            <tr><td>实例状态异常</td><td>ECS</td><td>停机、迁移、锁定、欠费和重启排查路径</td><td>告警确认后写入处理记录和复盘草稿</td></tr>
          </tbody>
        </table>
      </div>
      ) : null}
    </section>
    );
  };

  const renderSyncPage = () => {
    const activeSection = getActiveSection("sync");
    return (
    <section className="page-main-section">
      {activeSection === "tasks" ? (
      <div className="toolbar">
        <button type="button">立即同步</button>
        <button type="button">查看同步日志</button>
      </div>
      ) : null}
      {activeSection === "scope" ? (
      <div className="simple-summary">
        <span><b>定位</b>解释资源为什么没出现或指标为什么不新鲜</span>
        <span><b>重点</b>账号级失败和单资源失败分开展示</span>
        <span><b>回流</b>同步问题进入资源风险与排查入口</span>
      </div>
      ) : null}
      {activeSection === "tasks" ? (
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>账号</th>
              <th>任务</th>
              <th>状态</th>
              <th>进度</th>
              <th>更新时间</th>
            </tr>
          </thead>
          <tbody>
            {syncTasks.map((task) => {
              const account = getAccount(task.accountId);
              return (
                <tr key={task.id}>
                  <td>{account?.alias ?? "--"}</td>
                  <td>{task.name}</td>
                  <td><StatusText status={task.status}>{task.status}</StatusText></td>
                  <td>{task.progress}</td>
                  <td>{task.updatedAt}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      ) : null}
    </section>
    );
  };

  const renderSettingsPage = () => (
    <section className="page-main-section">
      <table className="detail-table settings-table">
        <tbody>
          <tr><th>页面定位</th><td>这里不再做宽泛系统设置，只展示当前阶段影响云账号接入、同步和排障的配置边界</td></tr>
          <tr><th>云账号接入</th><td>后端已接入阿里云 ECS/RDS、华为云 ECS/RDS 的凭据保存、校验和只读查询</td></tr>
          <tr><th>同步策略</th><td>资源列表和监控概览以只读同步为主；局部失败需要保留已有数据并给出账号、地域、资源类型和权限提示</td></tr>
          <tr><th>辅助能力</th><td>AI 摘要、知识沉淀和复盘内容只挂在实例异常排查链路，不作为当前阶段独立主线</td></tr>
          <tr><th>安全边界</th><td>凭据不落前端，后端按账号维度加密保存；当前管理接口默认匿名可访问，部署前需按环境补充访问控制</td></tr>
        </tbody>
      </table>
    </section>
  );

  const renderPage = () => {
    if (activePage === "servers") return renderServersPage();
    if (activePage === "accounts") return renderAccountsPage();
    if (activePage === "regions") return renderRegionsPage();
    if (activePage === "monitor") return renderMonitorPage();
    if (activePage === "agents") return renderAgentsPage();
    if (activePage === "alerts") return renderEventsPage();
    if (activePage === "ai") return renderAiPage();
    if (activePage === "knowledge") return renderKnowledgePage();
    if (activePage === "sync") return renderSyncPage();
    return renderSettingsPage();
  };

  return (
    <main className={`admin-console theme-${theme}${sidebarHidden ? " sidebar-hidden" : ""}`}>
      <aside className="admin-sidebar">
        <div className="admin-logo">
          <div className="admin-logo-copy">
            <strong>CloudLens</strong>
            <span>Multi-cloud console</span>
          </div>
          <button className="sidebar-collapse-button" type="button" onMouseDown={(event) => event.preventDefault()} onClick={() => setSidebarHidden(true)} aria-label="隐藏左侧菜单" title="隐藏菜单">
            <span aria-hidden="true">‹</span>
          </button>
        </div>
        <nav className="admin-menu" aria-label="主菜单">
          {menuGroups.map((group) => (
            <section className="menu-group" key={group.title}>
              <h2>
                <span>{group.title}</span>
              </h2>
              {group.items.map((item) => (
                <div className="menu-item-wrap" key={item.key}>
                <button
                  className={activePage === item.key ? "menu-item active" : "menu-item"}
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => {
                    setActivePage(item.key);
                    if (item.children?.length) setActiveSection(item.key, defaultPageSections[item.key] ?? item.children[0].key);
                  }}
                >
                  <span className="menu-item-copy">
                    <span>{item.label}</span>
                    <small>{item.desc}</small>
                  </span>
                </button>
                {item.children && activePage === item.key ? (
                  <div className="submenu-list">
                    {item.children.map((child) => (
                      <button
                        className={getActiveSection(item.key) === child.key ? "submenu-item active" : "submenu-item"}
                        key={child.key}
                        type="button"
                        onMouseDown={(event) => event.preventDefault()}
                        onClick={() => {
                          setActivePage(item.key);
                          setActiveSection(item.key, child.key);
                        }}
                      >
                        <span>{child.label}</span>
                        <small>{child.desc}</small>
                      </button>
                    ))}
                  </div>
                ) : null}
                </div>
              ))}
            </section>
          ))}
        </nav>
      </aside>

      {sidebarHidden ? (
        <button className="sidebar-reopen-button" type="button" onMouseDown={(event) => event.preventDefault()} onClick={() => setSidebarHidden(false)} aria-label="展开左侧菜单" title="展开菜单">
          <span aria-hidden="true">›</span>
        </button>
      ) : null}

      <section className="admin-main">
        <header className="admin-header">
          <div>
            <h1>{getPageSectionTitle(activePage, getActiveSection(activePage)) ? `${getPageTitle(activePage)} / ${getPageSectionTitle(activePage, getActiveSection(activePage))}` : getPageTitle(activePage)}</h1>
            <p>当前先做好阿里云 ECS/RDS 与华为云 ECS 监控，AI 分析和知识库保留为实例排查扩展能力。</p>
          </div>
          <div className="header-meta">
            <span>{USE_MOCK ? "当前为前端样例数据" : "当前为真实云账号数据"}</span>
            <button type="button" onClick={() => setSidebarHidden((current) => !current)}>
              {sidebarHidden ? "展开菜单" : "隐藏菜单"}
            </button>
            <button type="button" onClick={() => setTheme((current) => (current === "light" ? "dark" : "light"))}>
              {theme === "light" ? "深色模式" : "浅色模式"}
            </button>
            <button type="button" onClick={() => void loadCloudAssets()}>刷新</button>
          </div>
        </header>
        <div className="admin-content">{renderPage()}</div>
      </section>
    </main>
  );
}
