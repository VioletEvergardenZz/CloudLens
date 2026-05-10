/**
 * 文件职责：承载多云资产管理后台首页
 * 关键交互：左侧菜单切换页面，云资产页按云平台、账号、地域聚合服务器
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
  | "files"
  | "ai"
  | "knowledge"
  | "sync"
  | "settings";

type Provider = "aliyun" | "huawei" | "tencent";
type AccountStatus = "normal" | "warning" | "syncing" | "disabled";
type ServerStatus = "running" | "warning" | "offline" | "maintenance";
type AgentStatus = "online" | "stale" | "missing" | "offline";
type ScopeKind = "all" | "provider" | "account" | "accountRegion" | "region";
type ThemeMode = "light" | "dark";
type PublicIpType = "public" | "eip" | "none";
type MetricSortKey = "cpu" | "memory" | "disk";
type SortDirection = "asc" | "desc";

type CloudAccount = {
  id: string;
  provider: Provider;
  name: string;
  alias: string;
  uid: string;
  owner: string;
  env: string;
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
  status: ServerStatus;
  agentStatus: AgentStatus;
  cpu: number;
  memory: number;
  disk: number;
  load: string;
  uptime: string;
  lastSeen: string;
  alerts: number;
  files: number;
  ai: number;
  kb: number;
};

type OperationEvent = {
  id: string;
  serverId: string;
  time: string;
  type: "告警" | "文件入云" | "AI分析" | "知识库" | "探针";
  level: "info" | "warning" | "critical";
  message: string;
  status: string;
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
  provider: "aliyun";
  regionId: string;
  zoneId: string;
  status: string;
  osName: string;
  osType: string;
  type: string;
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
  period?: string;
  points?: AliyunMetricPoint[];
};

type AliyunOverviewResponse = {
  ok?: boolean;
  accountId?: number;
  status?: string;
  message?: string;
  availableMetricCount?: number;
  metrics?: Record<string, AliyunMetricSeries>;
  errors?: Record<string, string>;
  error?: string;
};

type ApiCloudAccount = {
  id: number;
  provider: "aliyun";
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
  providerName: string;
  name: string;
  accessKeyId: string;
  accessKeySecret: string;
  regions: string;
  metricPeriod: string;
  enabled: boolean;
};

type MonitorMetricKey =
  | "cpu"
  | "memory"
  | "disk"
  | "load1m"
  | "internetIn"
  | "internetOut"
  | "internetTotal"
  | "internetBandwidth"
  | "intranetIn"
  | "intranetOut"
  | "intranetTotal"
  | "intranetBandwidth"
  | "networkTotal"
  | "diskReadBps"
  | "diskWriteBps";

type MonitorMetric = {
  key: MonitorMetricKey;
  label: string;
  unit: string;
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

const providerLabels: Record<Provider, string> = {
  aliyun: "阿里云",
  huawei: "华为云",
  tencent: "腾讯云",
};

const emptyCloudServer: CloudServer = {
  id: "",
  accountId: "aliyun-runtime",
  provider: "aliyun",
  region: "--",
  zone: "--",
  instanceId: "--",
  name: "--",
  business: "未选择服务器",
  publicIp: "--",
  publicIpType: "none",
  privateIp: "--",
  os: "--",
  spec: "--",
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

const emptyAccountForm: CloudAccountForm = {
  providerName: "阿里云 ECS",
  name: "",
  accessKeyId: "",
  accessKeySecret: "",
  regions: "cn-hangzhou",
  metricPeriod: "60",
  enabled: true,
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

const menuGroups: Array<{
  title: string;
  items: Array<{ key: PageKey; label: string; desc: string }>;
}> = [
  {
    title: "云资产",
    items: [
      { key: "servers", label: "服务器总览", desc: "多云账号服务器" },
      { key: "accounts", label: "云账号管理", desc: "账号与同步状态" },
      { key: "regions", label: "地域视图", desc: "按地域聚合" },
    ],
  },
  {
    title: "监控运维",
    items: [
      { key: "monitor", label: "监控概览", desc: "CPU/内存/磁盘" },
      { key: "agents", label: "探针管理", desc: "Agent 接入状态" },
      { key: "alerts", label: "告警事件", desc: "告警与处置" },
    ],
  },
  {
    title: "扩展能力",
    items: [
      { key: "files", label: "日志采样", desc: "文件上传与归档" },
      { key: "ai", label: "AI 摘要", desc: "异常辅助分析" },
      { key: "knowledge", label: "知识沉淀", desc: "经验复用与复盘" },
    ],
  },
  {
    title: "系统",
    items: [
      { key: "sync", label: "同步任务", desc: "云资源同步" },
      { key: "settings", label: "系统设置", desc: "接入配置" },
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
    env: "生产",
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
    env: "生产",
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
    env: "生产",
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
    env: "测试",
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
    env: "实验",
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

const events: OperationEvent[] = [
  {
    id: "evt-001",
    serverId: "ecs-pay-01",
    time: "13:17:42",
    type: "告警",
    level: "critical",
    message: "支付任务队列积压超过阈值",
    status: "AI 已给出处置建议",
  },
  {
    id: "evt-002",
    serverId: "huawei-db-01",
    time: "13:15:03",
    type: "告警",
    level: "critical",
    message: "只读数据库数据盘使用率达到 84%",
    status: "知识库命中 disk-cleanup",
  },
  {
    id: "evt-003",
    serverId: "huawei-app-01",
    time: "13:12:55",
    type: "文件入云",
    level: "info",
    message: "核心应用日志入云完成 24 个",
    status: "OSS 写入正常",
  },
  {
    id: "evt-004",
    serverId: "ecs-web-hk",
    time: "12:39:18",
    type: "探针",
    level: "warning",
    message: "海外 Web 探针心跳过期",
    status: "等待实例侧恢复",
  },
  {
    id: "evt-005",
    serverId: "ecs-order-01",
    time: "12:31:04",
    type: "知识库",
    level: "info",
    message: "订单超时事件复用历史处置记录",
    status: "已关联知识库",
  },
];

const syncTasks: SyncTask[] = [
  {
    id: "sync-aliyun-prod",
    accountId: "aliyun-prod-cn",
    name: "同步 ECS 实例、地域、安全组",
    status: "完成",
    progress: "100%",
    updatedAt: "13:18:22",
  },
  {
    id: "sync-aliyun-hk",
    accountId: "aliyun-hk",
    name: "同步香港账号服务器与探针心跳",
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

const getAliyunStatus = (status: string): ServerStatus => {
  const normalized = status.trim().toLowerCase();
  if (normalized === "running") return "running";
  if (normalized === "stopped" || normalized === "deleted") return "offline";
  if (normalized === "starting" || normalized === "stopping") return "maintenance";
  return "warning";
};

const formatUptimeFromCreatedAt = (createdAt: string, status: string) => {
  if (status.trim().toLowerCase() !== "running") return "--";
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

const publicIpLabel = (server: CloudServer) => (server.publicIpType === "eip" ? "弹性公网 IP" : "公网");

const metricSortLabels: Record<MetricSortKey, string> = {
  cpu: "CPU",
  memory: "内存",
  disk: "磁盘",
};

const sortServersByMetric = (
  rows: CloudServer[],
  metricSort: { key: MetricSortKey; direction: SortDirection } | null
) => {
  if (!metricSort) return rows;
  return [...rows].sort((left, right) => {
    const result = left[metricSort.key] - right[metricSort.key];
    return metricSort.direction === "asc" ? result : -result;
  });
};

const mapApiCloudAccount = (account: ApiCloudAccount): CloudAccount => {
  const status = !account.enabled
    ? "disabled"
    : account.lastCheckStatus === "ok"
      ? "normal"
      : account.lastCheckStatus
        ? "warning"
        : "syncing";
  return {
    id: String(account.id),
    provider: "aliyun",
    name: account.name,
    alias: `阿里云 / ${account.name}`,
    uid: account.accessKeyIdMasked || "--",
    owner: "控制台配置",
    env: account.enabled ? "ECS 监控" : "已停用",
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
  const serverStatus = getAliyunStatus(instance.status);
  const publicIpInfo = resolvePublicIp(instance);
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
    provider: "aliyun",
    region: instance.regionId || "--",
    zone: instance.zoneId || "--",
    instanceId: instance.id,
    name: instance.name || instance.hostName || instance.id,
    business: "阿里云 ECS",
    ...publicIpInfo,
    privateIp: instance.privateIps?.[0] ?? "-",
    os: instance.osName || instance.osType || "--",
    spec: `${cpuSpec}${formatMemorySpec(instance.memoryMb)}`,
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

const monitorMetricMeta: Array<{ key: MonitorMetricKey; label: string; unit: string }> = [
  { key: "cpu", label: "CPU", unit: "%" },
  { key: "memory", label: "内存", unit: "%" },
  { key: "disk", label: "磁盘", unit: "%" },
  { key: "load1m", label: "1 分钟负载", unit: "" },
  { key: "internetIn", label: "公网入", unit: "B/s" },
  { key: "internetOut", label: "公网出", unit: "B/s" },
  { key: "internetTotal", label: "公网总带宽", unit: "B/s" },
  { key: "internetBandwidth", label: "公网带宽", unit: "bit/s" },
  { key: "intranetIn", label: "内网入", unit: "B/s" },
  { key: "intranetOut", label: "内网出", unit: "B/s" },
  { key: "intranetTotal", label: "内网总带宽", unit: "B/s" },
  { key: "intranetBandwidth", label: "内网带宽", unit: "bit/s" },
  { key: "networkTotal", label: "总带宽", unit: "B/s" },
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

const formatMetricValue = (key: MonitorMetricKey, value: number) => {
  if (key === "cpu" || key === "memory" || key === "disk") {
    return formatPercentValue(value);
  }
  if (key === "load1m") return value.toFixed(2);
  if (key === "diskReadBps" || key === "diskWriteBps") return formatRateValue(value, "B/s");
  return formatRateValue(value, "bit/s");
};

const latestPoint = (points?: AliyunMetricPoint[]) => {
  if (!points?.length) return undefined;
  return points.at(-1);
};

const hasMetricPoint = (series?: AliyunMetricSeries) => Boolean(latestPoint(series?.points));

const clampPercentMetric = (value?: number) => {
  if (typeof value !== "number" || Number.isNaN(value)) return 0;
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

    const metrics = overviewMap[server.id]?.metrics ?? {};
    const next = { ...server };
    let reusedPreviousMetric = false;

    if (!hasMetricPoint(metrics.cpu) && previous.cpu > 0) {
      next.cpu = previous.cpu;
      reusedPreviousMetric = true;
    }
    if (!hasMetricPoint(metrics.memory) && previous.memory > 0) {
      next.memory = previous.memory;
      reusedPreviousMetric = true;
    }
    if (!hasMetricPoint(metrics.disk) && previous.disk > 0) {
      next.disk = previous.disk;
      reusedPreviousMetric = true;
    }
    if (!hasMetricPoint(metrics.load1m) && previous.load !== "--") {
      next.load = previous.load;
      reusedPreviousMetric = true;
    }
    if (reusedPreviousMetric && server.agentStatus === "missing") {
      next.agentStatus = previous.agentStatus === "online" ? "stale" : previous.agentStatus;
      next.lastSeen = "本轮暂未返回新采样，沿用上一轮监控值";
    }
    return next;
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

const buildDerivedSeries = (metricName: string, seriesList: Array<AliyunMetricSeries | undefined>): AliyunMetricSeries | undefined => {
  const validSeries = seriesList.filter((series): series is AliyunMetricSeries => Boolean(series?.points?.length));
  if (validSeries.length !== seriesList.length || !validSeries.length) return undefined;
  const baseline = validSeries[0]?.points ?? [];
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
  return { namespace: "derived", metricName, points };
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
  return namespace || "未知来源";
};

const formatMetricDelta = (key: MonitorMetricKey, value: number) => {
  const sign = value > 0 ? "+" : value < 0 ? "-" : "";
  const abs = Math.abs(value);
  if (key === "cpu" || key === "memory" || key === "disk") {
    return `${sign}${abs.toFixed(2)} pct`;
  }
  if (key === "load1m") {
    return `${sign}${abs.toFixed(2)}`;
  }
  if (key === "diskReadBps" || key === "diskWriteBps") {
    return `${sign}${formatRateValue(abs, "B/s")}`;
  }
  return `${sign}${formatRateValue(abs, "bit/s")}`;
};

const buildMetricTrend = (key: MonitorMetricKey, points?: AliyunMetricPoint[]) => {
  const recentPoints = recentMetricPoints(points);
  if (!recentPoints.length) return "--";
  if (recentPoints.length === 1) return "单点采样";
  const first = recentPoints[0];
  const latest = recentPoints.at(-1);
  if (!first || !latest) return "--";
  const delta = latest.value - first.value;
  if (Math.abs(delta) < 0.0001) return "近窗基本持平";
  return `${delta > 0 ? "上升" : "下降"} ${formatMetricDelta(key, delta)}`;
};

const buildRecentSamples = (key: MonitorMetricKey, points?: AliyunMetricPoint[]) =>
  recentMetricPoints(points, 6).map((point) => `${formatMetricShortTime(point.timestamp)} ${formatMetricValue(key, point.value)}`);

const buildSparklinePath = (points: AliyunMetricPoint[], width = 112, height = 28) => {
  if (points.length < 2) return "";
  const values = points.map((point) => point.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  return points
    .map((point, index) => {
      const x = (index / Math.max(1, points.length - 1)) * width;
      const y = height - ((point.value - min) / range) * height;
      return `${index === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)}`;
    })
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
  if (latest) {
    return `${source}${metricName}${period}`;
  }
  if (error) return error;
  if (derivedMetricKeys.has(item.key)) return "依赖的入/出方向指标没有同时返回";
  if (pluginMetricKeys.has(item.key)) return "ECS 基础监控不包含该项，需云监控插件或 Agent 上报";
  if (series) return `${source}${metricName}${period}，接口返回成功但没有采样点`;
  return "阿里云未返回该指标";
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
    const points = series?.points ?? [];
    const summary = summarizeMetricPoints(points);
    const latest = summary?.latest;
    const error = payload?.errors?.[item.key];
    return {
      ...item,
      value: latest ? formatMetricValue(item.key, latest.value) : "--",
      range: summary ? `${formatMetricValue(item.key, summary.min)} ~ ${formatMetricValue(item.key, summary.max)}` : "--",
      average: summary ? formatMetricValue(item.key, summary.average) : "--",
      sampledAt: formatMetricTime(latest?.timestamp) || "--",
      trend: buildMetricTrend(item.key, points),
      sparklinePoints: recentMetricPoints(points),
      recentSamples: buildRecentSamples(item.key, points),
      points: points.length,
      status: latest ? "ok" : error ? "error" : "empty",
      note: buildMetricNote(item, latest, series, error),
    };
  });
};

const buildScopeKey = (kind: ScopeKind, value = "") => `${kind}:${value}`;

const parseScope = (scope: string): { kind: ScopeKind; value: string } => {
  const [kind, ...rest] = scope.split(":");
  if (kind === "provider" || kind === "account" || kind === "accountRegion" || kind === "region") {
    return { kind, value: rest.join(":") };
  }
  return { kind: "all", value: "" };
};

const serverMatchesScope = (server: CloudServer, parsedScope: { kind: ScopeKind; value: string }) => {
  if (parsedScope.kind === "provider") return server.provider === parsedScope.value;
  if (parsedScope.kind === "account") return server.accountId === parsedScope.value;
  if (parsedScope.kind === "accountRegion") return `${server.accountId}:${server.region}` === parsedScope.value;
  if (parsedScope.kind === "region") return `${server.provider}:${server.region}` === parsedScope.value;
  return true;
};

const splitFormList = (raw: string) =>
  raw
    .split(/[,;\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);

const formatDisplayTime = (raw?: string) => {
  if (!raw) return "--";
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString("zh-CN", { hour12: false });
};

const getStatusClass = (status: ServerStatus | AgentStatus | AccountStatus | OperationEvent["level"] | SyncTask["status"]) => {
  if (status === "running" || status === "online" || status === "normal" || status === "info" || status === "完成") {
    return "ok";
  }
  if (status === "warning" || status === "stale" || status === "syncing" || status === "执行中" || status === "等待") {
    return "warn";
  }
  if (status === "maintenance" || status === "disabled") {
    return "muted";
  }
  return "bad";
};

const getPageTitle = (page: PageKey) => menuGroups.flatMap((group) => group.items).find((item) => item.key === page)?.label ?? "";

function StatusText({ children, status }: { children: string; status: string }) {
  return <span className={`status-text ${getStatusClass(status as ServerStatus)}`}>{children}</span>;
}

function Utilization({ value }: { value: number }) {
  const tone = value >= 80 ? "bad" : value >= 70 ? "warn" : "ok";
  return (
    <span className={`usage usage-${tone}`}>
      <i style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
      <b>{formatPercentValue(value)}</b>
    </span>
  );
}

function MetricSparkline({ points, status }: { points: AliyunMetricPoint[]; status: MonitorMetric["status"] }) {
  if (points.length < 2) {
    return <span className="metric-sparkline-empty">{points.length === 1 ? "单点" : "无趋势"}</span>;
  }
  const first = points[0];
  const latest = points.at(-1);
  const delta = (latest?.value ?? 0) - (first?.value ?? 0);
  const trendClass = status !== "ok" ? "muted" : delta > 0 ? "up" : delta < 0 ? "down" : "flat";
  const path = buildSparklinePath(points);
  return (
    <div className={`metric-sparkline metric-sparkline-${trendClass}`}>
      <svg viewBox="0 0 112 28" aria-hidden="true" focusable="false">
        <path d={path} pathLength={100} />
      </svg>
    </div>
  );
}

export function CloudResourceConsole() {
  const [activePage, setActivePage] = useState<PageKey>("servers");
  const [accounts, setAccounts] = useState<CloudAccount[]>(() => (USE_MOCK ? mockAccounts : []));
  const [servers, setServers] = useState<CloudServer[]>(() => (USE_MOCK ? mockServers : []));
  const [assetLoading, setAssetLoading] = useState(!USE_MOCK);
  const [assetError, setAssetError] = useState("");
  const [lastSyncAt, setLastSyncAt] = useState("--");
  const [scope, setScope] = useState(buildScopeKey("all"));
  const [keyword, setKeyword] = useState("");
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
  const [accountNotice, setAccountNotice] = useState("");
  const [expandedTree, setExpandedTree] = useState<Record<string, boolean>>({});
  const [metricSort, setMetricSort] = useState<{ key: MetricSortKey; direction: SortDirection } | null>(null);
  const assetLoadingRef = useRef(false);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarHidden));
  }, [sidebarHidden]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  const loadCloudAssets = async (options: { silent?: boolean } = {}) => {
    if (assetLoadingRef.current) return;
    assetLoadingRef.current = true;
    if (USE_MOCK) {
      setAssetLoading(false);
      setLastSyncAt("示例模式");
      assetLoadingRef.current = false;
      return;
    }
    if (!options.silent) setAssetLoading(true);
    setAssetError("");
    try {
      const accountResp = await fetch(`${API_BASE}/api/cloud/accounts`, {
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
        setSelectedServerId("");
        setLastSyncAt("等待配置云账号");
        return;
      }

      const overviewMap: Record<string, AliyunOverviewResponse> = {};
      const errors: string[] = [];
      const accountResults = await Promise.allSettled(
        mappedAccounts
          .filter((account) => account.enabled !== false)
          .map(async (account) => {
            const resp = await fetch(`${API_BASE}/api/cloud/aliyun/instances?accountId=${encodeURIComponent(account.id)}`, {
              cache: "no-store",
              headers: buildApiHeaders(),
            });
            const payload = (await resp.json().catch(() => ({}))) as AliyunInstancesResponse;
            if (!resp.ok) {
              throw new Error(`${account.name}: ${payload.error || `ECS 接口返回 ${resp.status}`}`);
            }
            const instances = payload.items ?? [];
            const rows = await Promise.all(
              instances.map(async (instance) => {
                const publicIp = instance.eipAddress?.trim() || instance.publicIps?.find((ip) => ip.trim())?.trim() || "";
                const overviewResp = await fetch(
                  `${API_BASE}/api/cloud/aliyun/overview?accountId=${encodeURIComponent(account.id)}&instanceId=${encodeURIComponent(instance.id)}&region=${encodeURIComponent(instance.regionId)}&minutes=30&period=${encodeURIComponent(account.metricPeriod || "60")}&publicIp=${encodeURIComponent(publicIp)}`,
                  { cache: "no-store", headers: buildApiHeaders() }
                );
                const overview = (await overviewResp.json().catch(() => ({}))) as AliyunOverviewResponse;
                const server = mapAliyunServer(account, instance, overviewResp.ok ? overview : undefined);
                if (overviewResp.ok) {
                  overviewMap[server.id] = overview;
                } else {
                  overviewMap[server.id] = { ok: false, error: overview.error || "监控数据读取失败" };
                }
                return server;
              })
            );
            return rows;
          })
      );
      const rows = accountResults.flatMap((result) => {
        if (result.status === "fulfilled") return result.value;
        errors.push(result.reason instanceof Error ? result.reason.message : "云资源同步失败");
        return [];
      });
      setOverviewByServerId((current) => mergeOverviewMapWithPrevious(overviewMap, current, rows));
      setServers((current) => mergeServerRowsWithPrevious(rows, current, overviewMap));
      setSelectedServerId((current) => (rows.some((server) => server.id === current) ? current : rows[0]?.id ?? ""));
      setLastSyncAt(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
      if (errors.length) {
        setAssetError(errors.join("；"));
      }
    } catch (error) {
      setAssetError(error instanceof Error ? error.message : "云资源同步失败");
    } finally {
      assetLoadingRef.current = false;
      if (!options.silent) setAssetLoading(false);
    }
  };

  const saveAccount = async () => {
    if (USE_MOCK) return;
    const regions = splitFormList(accountForm.regions);
    if (!accountForm.name.trim() || !regions.length) {
      setAccountNotice("请填写账号名称和地域");
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
          provider: "aliyun",
          name: accountForm.name.trim(),
          accessKeyId: accountForm.accessKeyId.trim(),
          accessKeySecret: accountForm.accessKeySecret.trim(),
          regions,
          metricPeriod: accountForm.metricPeriod.trim() || "60",
          enabled: accountForm.enabled,
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

  const editAccount = (account: CloudAccount) => {
    setEditingAccountId(account.id);
    setAccountForm({
      providerName: providerLabels[account.provider] ?? "阿里云 ECS",
      name: account.name,
      accessKeyId: "",
      accessKeySecret: "",
      regions: account.regions.join(","),
      metricPeriod: account.metricPeriod || "60",
      enabled: account.enabled !== false,
    });
    setAccountNotice("编辑模式下 AccessKey 留空表示沿用原值");
  };

  const cancelAccountEdit = () => {
    setEditingAccountId("");
    setAccountForm(emptyAccountForm);
    setAccountNotice("");
  };

  const updateAccountForm = <K extends keyof CloudAccountForm>(key: K, value: CloudAccountForm[K]) => {
    setAccountForm((current) => ({ ...current, [key]: value }));
  };

  const isTreeExpanded = (key: string) => expandedTree[key] !== false;

  const toggleTreeNode = (key: string) => {
    setExpandedTree((current) => ({ ...current, [key]: current[key] === false }));
  };

  const toggleMetricSort = (key: MetricSortKey) => {
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

  const getServer = (serverId: string) => servers.find((server) => server.id === serverId);

  const selectScope = (nextScope: string) => {
    const nextParsedScope = parseScope(nextScope);
    const firstServer = servers.find((server) => serverMatchesScope(server, nextParsedScope));
    setScope(nextScope);
    setSelectedServerId(firstServer?.id ?? "");
  };

  const visibleServers = useMemo(() => {
    const text = keyword.trim().toLowerCase();
    const rows = servers.filter((server) => {
      if (!serverMatchesScope(server, parsedScope)) return false;
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
        account?.name,
        account?.alias,
        providerLabels[server.provider],
      ]
        .join(" ")
        .toLowerCase()
        .includes(text);
    });
    return sortServersByMetric(rows, metricSort);
  }, [agentFilter, getAccount, keyword, metricSort, parsedScope, servers, statusFilter]);

  const monitorServers = useMemo(() => sortServersByMetric(servers, metricSort), [metricSort, servers]);

  const selectedServer = useMemo(() => {
    if (activePage === "monitor") return servers.find((server) => server.id === selectedServerId) ?? servers[0] ?? emptyCloudServer;
    return visibleServers.find((server) => server.id === selectedServerId) ?? visibleServers[0] ?? servers[0] ?? emptyCloudServer;
  }, [activePage, selectedServerId, servers, visibleServers]);

  const selectedAccount = selectedServer ? getAccount(selectedServer.accountId) : undefined;

  const summary = useMemo(() => {
    const warningServers = servers.filter((server) => server.status === "warning" || server.status === "offline").length;
    const agentIssues = servers.filter((server) => server.agentStatus !== "online").length;
    const alertCount = servers.reduce((total, server) => total + server.alerts, 0);
    const fileCount = servers.reduce((total, server) => total + server.files, 0);
    return { warningServers, agentIssues, alertCount, fileCount };
  }, [servers]);

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

  const renderMetricSortButton = (key: MetricSortKey) => {
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
      </colgroup>
      <thead>
        <tr>
          <th>服务器</th>
          <th>云平台 / 账号</th>
          <th>地域 / 可用区</th>
          <th>IP</th>
          <th>状态</th>
          <th>{renderMetricSortButton("cpu")}</th>
          <th>{renderMetricSortButton("memory")}</th>
          <th>{renderMetricSortButton("disk")}</th>
          <th>云监控</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((server) => {
          const account = getAccount(server.accountId);
          return (
            <tr
              className={selectedServer.id === server.id ? "selected" : ""}
              key={server.id}
              onClick={() => setSelectedServerId(server.id)}
            >
              <td className="server-name-cell">
                <strong>{server.name}</strong>
                <span>{server.instanceId}</span>
                <span>{server.business}</span>
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
                <span>{publicIpLabel(server)}：{server.publicIp}</span>
                {server.publicIpType === "eip" && server.publicIpId ? <span>EIP ID：{server.publicIpId}</span> : null}
                <span>内网：{server.privateIp}</span>
              </td>
              <td>
                <StatusText status={server.status}>{serverStatusLabels[server.status]}</StatusText>
              </td>
              <td><Utilization value={server.cpu} /></td>
              <td><Utilization value={server.memory} /></td>
              <td><Utilization value={server.disk} /></td>
              <td className="monitor-cell" title={`${agentStatusLabels[server.agentStatus]} / ${server.lastSeen}`}>
                <StatusText status={server.agentStatus}>{agentStatusLabels[server.agentStatus]}</StatusText>
                <span>{server.lastSeen}</span>
              </td>
            </tr>
          );
        })}
        {!rows.length ? (
          <tr>
            <td className="empty-cell" colSpan={9}>当前筛选条件下没有服务器</td>
          </tr>
        ) : null}
      </tbody>
    </table>
  );

  const renderScopeTree = () => (
    <aside className="resource-tree" aria-label="资源范围">
      <div className="resource-tree-title">
        <strong>资源范围</strong>
        <span>按云平台 / 账号 / 地域筛选</span>
      </div>
      <button className={scope === buildScopeKey("all") ? "tree-node root active" : "tree-node root"} onClick={() => selectScope(buildScopeKey("all"))}>
        <span className="tree-label">全部云平台</span>
        <em>{servers.length}</em>
      </button>
      {(["aliyun", "huawei", "tencent"] as Provider[]).map((provider) => {
        const providerKey = `provider:${provider}`;
        const providerExpanded = isTreeExpanded(providerKey);
        const providerServers = servers.filter((server) => server.provider === provider);
        const providerAccounts = accounts.filter((account) => account.provider === provider);
        if (!providerServers.length && !providerAccounts.length) return null;
        return (
          <div className="tree-group" key={provider}>
            <div className="tree-line">
              <button className={providerExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(providerKey)} aria-label={providerExpanded ? "收起云平台" : "展开云平台"}>
                <span className="tree-caret" />
              </button>
              <button
                className={scope === buildScopeKey("provider", provider) ? "tree-node provider active" : "tree-node provider"}
                onClick={() => selectScope(buildScopeKey("provider", provider))}
              >
                <span className="tree-label">{providerLabels[provider]}</span>
                <em>{providerServers.length}</em>
              </button>
            </div>
            {providerExpanded
              ? providerAccounts.map((account) => {
                  const accountKey = `account:${account.id}`;
                  const accountExpanded = isTreeExpanded(accountKey);
                  const accountServers = servers.filter((server) => server.accountId === account.id);
                  const accountRegions = [...new Set(accountServers.map((server) => server.region))];
                  return (
                    <div className="account-branch" key={account.id}>
                      <div className="tree-line">
                        <button className={accountExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(accountKey)} aria-label={accountExpanded ? "收起账号" : "展开账号"}>
                          <span className="tree-caret" />
                        </button>
                        <button
                          className={scope === buildScopeKey("account", account.id) ? "tree-node account active" : "tree-node account"}
                          onClick={() => selectScope(buildScopeKey("account", account.id))}
                        >
                          <span className="tree-label">{account.name}</span>
                          <em>{accountServers.length}</em>
                        </button>
                      </div>
                      {accountExpanded
                        ? accountRegions.map((region) => {
                            const regionKey = `accountRegion:${account.id}:${region}`;
                            const regionExpanded = isTreeExpanded(regionKey);
                            const regionServers = accountServers.filter((server) => server.region === region);
                            return (
                              <div className="region-branch" key={`${account.id}:${region}`}>
                                <div className="tree-line">
                                  <button className={regionExpanded ? "tree-toggle expanded" : "tree-toggle"} type="button" onClick={() => toggleTreeNode(regionKey)} aria-label={regionExpanded ? "收起地域" : "展开地域"}>
                                    <span className="tree-caret" />
                                  </button>
                                  <button
                                    className={scope === buildScopeKey("accountRegion", `${account.id}:${region}`) ? "tree-node region active" : "tree-node region"}
                                    onClick={() => selectScope(buildScopeKey("accountRegion", `${account.id}:${region}`))}
                                  >
                                    <span className="tree-label">{region}</span>
                                    <em>{regionServers.length}</em>
                                  </button>
                                </div>
                                {regionExpanded
                                  ? regionServers.map((server) => (
                                      <button
                                        className={selectedServer.id === server.id ? "tree-node server active" : "tree-node server"}
                                        key={server.id}
                                        onClick={() => {
                                          selectScope(buildScopeKey("accountRegion", `${account.id}:${region}`));
                                          setSelectedServerId(server.id);
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
      })}
    </aside>
  );

  const renderServersPage = () => (
    <div className="asset-page">
      {renderScopeTree()}
      <section className="page-main-section">
        <div className="toolbar">
          <input value={keyword} onChange={(event) => setKeyword(event.currentTarget.value)} placeholder="搜索服务器、账号、IP、业务或地域" />
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
          <button type="button" onClick={() => void loadCloudAssets()} disabled={assetLoading}>{assetLoading ? "同步中" : "同步资产"}</button>
          <button type="button">导出</button>
        </div>

        <div className="simple-summary">
          <span>云账号：{accounts.length}</span>
          <span>服务器：{servers.length}</span>
          <span>异常服务器：{summary.warningServers}</span>
          <span>云监控异常：{summary.agentIssues}</span>
          <span>今日文件入云：{summary.fileCount}</span>
          <span>待关注告警：{summary.alertCount}</span>
          <span>云资源同步：{assetLoading ? "同步中" : assetError ? `异常：${assetError}` : `${lastSyncAt}，自动刷新 30 秒`}</span>
        </div>

        <div className="table-panel">{renderServerTable(visibleServers)}</div>

        <section className="detail-panel">
          <div className="section-title">
            <h3>服务器详情</h3>
            <span>选择表格中的服务器后在这里查看基础信息和闭环状态</span>
          </div>
          <table className="detail-table">
            <tbody>
              <tr><th>服务器</th><td>{selectedServer.name}</td><th>实例 ID</th><td>{selectedServer.instanceId}</td></tr>
              <tr><th>云平台</th><td>{providerLabels[selectedServer.provider]}</td><th>账号</th><td>{selectedAccount?.name ?? "--"}</td></tr>
              <tr><th>地域</th><td>{selectedServer.region}</td><th>可用区</th><td>{selectedServer.zone}</td></tr>
              <tr><th>{publicIpLabel(selectedServer)}</th><td>{selectedServer.publicIp}{selectedServer.publicIpId ? ` / ${selectedServer.publicIpId}` : ""}</td><th>内网 IP</th><td>{selectedServer.privateIp}</td></tr>
              <tr><th>系统</th><td>{selectedServer.os}</td><th>规格</th><td>{selectedServer.spec}</td></tr>
              <tr><th>运行状态</th><td>{serverStatusLabels[selectedServer.status]}</td><th>云监控</th><td>{agentStatusLabels[selectedServer.agentStatus]} / {selectedServer.lastSeen}</td></tr>
              <tr><th>负载</th><td>{selectedServer.load}</td><th>运行时长</th><td>{selectedServer.uptime}</td></tr>
            </tbody>
          </table>
        </section>
      </section>
    </div>
  );

  const renderAccountsPage = () => (
    <section className="page-main-section">
      <div className="account-config-panel">
        <div className="section-title">
          <h3>{editingAccountId ? "编辑云平台账号" : "新增云平台账号"}</h3>
          <span>AccessKey Secret 只提交到后端加密保存，不会回显到前端</span>
        </div>
        <div className="account-form">
          <label>
            <span>云平台</span>
            <input value={accountForm.providerName} onChange={(event) => updateAccountForm("providerName", event.currentTarget.value)} placeholder="阿里云 ECS" />
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
            <span>地域</span>
            <input value={accountForm.regions} onChange={(event) => updateAccountForm("regions", event.currentTarget.value)} placeholder="cn-hangzhou,cn-shanghai" />
          </label>
          <label>
            <span>采样周期（秒）</span>
            <input value={accountForm.metricPeriod} onChange={(event) => updateAccountForm("metricPeriod", event.currentTarget.value)} placeholder="60" />
          </label>
          <label>
            <span>账号状态</span>
            <select value={accountForm.enabled ? "enabled" : "disabled"} onChange={(event) => updateAccountForm("enabled", event.currentTarget.value === "enabled")}>
              <option value="enabled">启用同步和展示</option>
              <option value="disabled">停用</option>
            </select>
          </label>
          <div className="account-form-actions">
            <button type="button" onClick={() => void saveAccount()} disabled={accountSaving}>
              {accountSaving ? "保存中" : editingAccountId ? "更新账号" : "保存账号"}
            </button>
            {editingAccountId ? <button type="button" onClick={cancelAccountEdit}>取消编辑</button> : null}
          </div>
        </div>
        {accountNotice ? <div className="inline-message">{accountNotice}</div> : null}
      </div>
      <div className="toolbar">
        <button type="button" onClick={() => void loadCloudAssets()}>同步全部账号</button>
      </div>
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>云平台</th>
              <th>账号名称</th>
              <th>账号标识</th>
              <th>负责人</th>
              <th>环境</th>
              <th>地域</th>
              <th>服务器</th>
              <th>状态</th>
              <th>最近同步</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {accounts.map((account) => (
              <tr key={account.id}>
                <td>{providerLabels[account.provider]}</td>
                <td><strong>{account.name}</strong><span>{account.checkMessage || account.alias}</span></td>
                <td>{account.uid}</td>
                <td>{account.owner}</td>
                <td>{account.env}</td>
                <td>{account.regions.join("、")}</td>
                <td>{servers.filter((server) => server.accountId === account.id).length}</td>
                <td><StatusText status={account.status}>{accountStatusLabels[account.status]}</StatusText></td>
                <td>{account.lastSync}</td>
                <td>
                  <div className="row-actions">
                    <button className="link-button" type="button" onClick={() => void testAccount(account.id)} disabled={testingAccountId === account.id}>
                      {testingAccountId === account.id ? "校验中" : "校验"}
                    </button>
                    <button className="link-button" type="button" onClick={() => editAccount(account)}>编辑</button>
                    <button className="link-button danger" type="button" onClick={() => void deleteAccount(account.id)}>删除</button>
                  </div>
                </td>
              </tr>
            ))}
            {!accounts.length ? (
              <tr>
                <td className="empty-cell" colSpan={10}>还没有云账号，先在上方新增一个阿里云 ECS 账号</td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderRegionsPage = () => (
    <section className="page-main-section">
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>云平台</th>
              <th>地域</th>
              <th>服务器数</th>
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
                }}>查看服务器</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderMonitorPage = () => {
    const overview = overviewByServerId[selectedServer.id];
    const metricRows = buildMonitorRows(overview);
    const freshness = resolveOverviewFreshness(overview);
    const snapshot = latestMetricSnapshot(overview);
    const freshnessSummary = snapshot.latestTimestamp
      ? `监控窗口最近拿到 ${snapshot.pointCount} 个采样点，${freshness.message}${snapshot.periodSeconds ? `，主采样周期 ${snapshot.periodSeconds}s` : ""}`
      : freshness.message;
    return (
      <section className="page-main-section">
        <div className="table-panel">{renderServerTable(monitorServers)}</div>
        <section className="detail-panel">
          <div className="section-title">
            <h3>ECS 监控详情</h3>
            <span>{selectedServer.name} / {selectedServer.instanceId}</span>
          </div>
          {freshnessSummary ? (
            <div className="inline-message">{freshnessSummary}</div>
          ) : null}
          <table className="data-table">
            <thead>
              <tr>
                  <th>指标</th>
                  <th>当前值</th>
                  <th>30 分钟范围</th>
                  <th>均值</th>
                  <th>趋势</th>
                  <th>最近采样</th>
                  <th>最新采样</th>
                  <th>采样点数</th>
                  <th>状态</th>
                  <th>说明</th>
              </tr>
            </thead>
            <tbody>
              {metricRows.map((item) => (
                <tr key={item.key}>
                  <td>{item.label}</td>
                  <td>{item.value}</td>
                  <td>{item.range}</td>
                  <td>{item.average}</td>
                  <td className="metric-trend-cell">
                    <MetricSparkline points={item.sparklinePoints} status={item.status} />
                    <span>{item.trend}</span>
                  </td>
                  <td className="metric-samples-cell">
                    {item.recentSamples.length ? item.recentSamples.map((sample) => <span key={`${item.key}-${sample}`}>{sample}</span>) : <span>--</span>}
                  </td>
                  <td>{item.sampledAt}</td>
                  <td>{item.points}</td>
                  <td><StatusText status={item.status === "ok" ? "normal" : item.status === "error" ? "warning" : "disabled"}>{item.status === "ok" ? "正常" : item.status === "error" ? "异常" : "暂无数据"}</StatusText></td>
                  <td>{item.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      </section>
    );
  };

  const renderAgentsPage = () => (
    <section className="page-main-section">
      <div className="toolbar">
        <button type="button">生成安装命令</button>
        <button type="button" onClick={() => void loadCloudAssets()} disabled={assetLoading}>
          {assetLoading ? "检查中" : "批量检查心跳"}
        </button>
      </div>
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>服务器</th>
              <th>云账号</th>
              <th>探针状态</th>
              <th>最近心跳</th>
              <th>文件入云</th>
              <th>建议动作</th>
            </tr>
          </thead>
          <tbody>
            {servers.map((server) => {
              const account = getAccount(server.accountId);
              return (
                <tr key={server.id}>
                  <td><strong>{server.name}</strong><span>{server.privateIp}</span></td>
                  <td>{account?.alias ?? "--"}</td>
                  <td><StatusText status={server.agentStatus}>{agentStatusLabels[server.agentStatus]}</StatusText></td>
                  <td>{server.lastSeen}</td>
                  <td>{server.files}</td>
                  <td>
                    {server.agentStatus === "online"
                      ? "保持现有采集策略"
                      : server.agentStatus === "offline"
                        ? "实例离线，恢复运行后再检查探针"
                        : "检查 Agent 服务或补装探针"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderEventsPage = () => (
    <section className="page-main-section">
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>类型</th>
              <th>级别</th>
              <th>服务器</th>
              <th>账号 / 地域</th>
              <th>内容</th>
              <th>状态</th>
            </tr>
          </thead>
          <tbody>
            {events.map((event) => {
              const server = getServer(event.serverId);
              const account = server ? getAccount(server.accountId) : undefined;
              return (
                <tr key={event.id}>
                  <td>{event.time}</td>
                  <td>{event.type}</td>
                  <td><StatusText status={event.level}>{event.level}</StatusText></td>
                  <td>{server?.name ?? "--"}</td>
                  <td>{account?.alias ?? "--"} / {server?.region ?? "--"}</td>
                  <td>{event.message}</td>
                  <td>{event.status}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderFilesPage = () => (
    <section className="page-main-section">
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>服务器</th>
              <th>云平台 / 账号</th>
              <th>今日入云文件</th>
              <th>最近状态</th>
              <th>建议</th>
            </tr>
          </thead>
          <tbody>
            {servers.map((server) => {
              const account = getAccount(server.accountId);
              return (
                <tr key={server.id}>
                  <td>{server.name}</td>
                  <td>{providerLabels[server.provider]} / {account?.alias ?? "--"}</td>
                  <td>{server.files}</td>
                  <td>
                    {server.agentStatus === "online"
                      ? "采集正常"
                      : server.agentStatus === "offline"
                        ? "实例离线，暂停采集判断"
                        : "等待探针恢复"}
                  </td>
                  <td>{server.files === 0 ? "确认日志目录和自动上传配置" : "保持当前队列策略"}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderAiPage = () => (
    <section className="page-main-section">
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>服务器</th>
              <th>AI 分析次数</th>
              <th>知识库命中</th>
              <th>最近结论</th>
            </tr>
          </thead>
          <tbody>
            {servers.filter((server) => server.ai > 0 || server.alerts > 0).map((server) => (
              <tr key={server.id}>
                <td>{server.name}</td>
                <td>{server.ai}</td>
                <td>{server.kb}</td>
                <td>{server.alerts > 0 ? "存在告警，建议优先查看 AI 分析结果和知识库命中" : "暂无高风险结论"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderKnowledgePage = () => (
    <section className="page-main-section">
      <div className="table-panel">
        <table className="data-table">
          <thead>
            <tr>
              <th>知识条目</th>
              <th>适用云平台</th>
              <th>关联服务器</th>
              <th>命中次数</th>
              <th>说明</th>
            </tr>
          </thead>
          <tbody>
            <tr><td>disk-cleanup</td><td>阿里云 / 华为云</td><td>huawei-db-01</td><td>1</td><td>磁盘水位过高时的清理与扩容步骤</td></tr>
            <tr><td>order-timeout</td><td>阿里云</td><td>order-api-prod-01</td><td>3</td><td>订单接口超时的排查流程</td></tr>
            <tr><td>agent-heartbeat-stale</td><td>多云通用</td><td>web-hk-02</td><td>1</td><td>探针心跳过期后的恢复检查</td></tr>
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderSyncPage = () => (
    <section className="page-main-section">
      <div className="toolbar">
        <button type="button">立即同步</button>
        <button type="button">查看同步日志</button>
      </div>
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
    </section>
  );

  const renderSettingsPage = () => (
    <section className="page-main-section">
      <table className="detail-table settings-table">
        <tbody>
          <tr><th>云账号接入</th><td>后端接入后支持阿里云、华为云、腾讯云凭据同步</td></tr>
          <tr><th>探针接入</th><td>后续复用 Komari 的轻量 Agent 思路，统一上报基础监控与心跳</td></tr>
          <tr><th>扩展能力</th><td>后续把日志采样、AI 摘要和知识沉淀都挂到实例异常排查链路</td></tr>
          <tr><th>安全边界</th><td>凭据不落前端，后端按账号维度加密保存和定时同步</td></tr>
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
    if (activePage === "files") return renderFilesPage();
    if (activePage === "ai") return renderAiPage();
    if (activePage === "knowledge") return renderKnowledgePage();
    if (activePage === "sync") return renderSyncPage();
    return renderSettingsPage();
  };

  return (
    <main className={`admin-console theme-${theme}${sidebarHidden ? " sidebar-hidden" : ""}`}>
      <aside className="admin-sidebar">
        <div className="admin-logo">
          <div>
            <strong>云镜</strong>
            <span>CloudLens 多云监控台</span>
          </div>
          <button type="button" onClick={() => setSidebarHidden(true)}>隐藏</button>
        </div>
        <nav className="admin-menu" aria-label="主菜单">
          {menuGroups.map((group) => (
            <section className="menu-group" key={group.title}>
              <h2>{group.title}</h2>
              {group.items.map((item) => (
                <button
                  className={activePage === item.key ? "menu-item active" : "menu-item"}
                  key={item.key}
                  type="button"
                  onClick={() => setActivePage(item.key)}
                >
                  <span>{item.label}</span>
                  <small>{item.desc}</small>
                </button>
              ))}
            </section>
          ))}
        </nav>
      </aside>

      <section className="admin-main">
        <header className="admin-header">
          <div>
            <h1>{getPageTitle(activePage)}</h1>
            <p>当前先收口阿里云单账号 ECS 监控，文件入云、AI 分析和知识库保留为实例排查扩展能力。</p>
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
