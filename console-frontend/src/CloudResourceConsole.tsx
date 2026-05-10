/**
 * 文件职责：承载多云资产管理后台首页
 * 关键交互：左侧菜单切换页面，云资产页按云平台、账号、地域聚合服务器
 * 边界处理：后端云账号同步未接入前使用 mock 数据，接口接入后只替换数据来源
 */

/* 本文件用于普通后台 Web 页面，融合 Komari 的探针监控视角和 GWF 的闭环能力 */

import { useEffect, useMemo, useState } from "react";
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
type AgentStatus = "online" | "stale" | "missing";
type ScopeKind = "all" | "provider" | "account" | "accountRegion" | "region";
type ThemeMode = "light" | "dark";

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

const providerLabels: Record<Provider, string> = {
  aliyun: "阿里云",
  huawei: "华为云",
  tencent: "腾讯云",
};

const SIDEBAR_STORAGE_KEY = "gwf-cloud-sidebar-hidden";
const THEME_STORAGE_KEY = "gwf-cloud-theme";

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
  online: "在线",
  stale: "心跳过期",
  missing: "未接入",
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
    title: "GWF 闭环",
    items: [
      { key: "files", label: "文件入云", desc: "上传与队列" },
      { key: "ai", label: "AI 分析", desc: "诊断与降级" },
      { key: "knowledge", label: "知识库", desc: "复用与沉淀" },
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

const accounts: CloudAccount[] = [
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

const servers: CloudServer[] = [
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
    agentStatus: "stale",
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

const getAccount = (accountId: string) => accounts.find((account) => account.id === accountId);

const getServer = (serverId: string) => servers.find((server) => server.id === serverId);

const buildScopeKey = (kind: ScopeKind, value = "") => `${kind}:${value}`;

const parseScope = (scope: string): { kind: ScopeKind; value: string } => {
  const [kind, ...rest] = scope.split(":");
  if (kind === "provider" || kind === "account" || kind === "accountRegion" || kind === "region") {
    return { kind, value: rest.join(":") };
  }
  return { kind: "all", value: "" };
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
      <b>{value}%</b>
    </span>
  );
}

export function CloudResourceConsole() {
  const [activePage, setActivePage] = useState<PageKey>("servers");
  const [scope, setScope] = useState(buildScopeKey("all"));
  const [keyword, setKeyword] = useState("");
  const [statusFilter, setStatusFilter] = useState<ServerStatus | "all">("all");
  const [agentFilter, setAgentFilter] = useState<AgentStatus | "all">("all");
  const [selectedServerId, setSelectedServerId] = useState(servers[0]?.id ?? "");
  const [sidebarHidden, setSidebarHidden] = useState(() => resolveInitialSidebarHidden());
  const [theme, setTheme] = useState<ThemeMode>(() => resolveInitialTheme());

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarHidden));
  }, [sidebarHidden]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  const parsedScope = parseScope(scope);

  const filteredServers = useMemo(() => {
    const text = keyword.trim().toLowerCase();
    return servers.filter((server) => {
      if (parsedScope.kind === "provider" && server.provider !== parsedScope.value) return false;
      if (parsedScope.kind === "account" && server.accountId !== parsedScope.value) return false;
      if (parsedScope.kind === "accountRegion" && `${server.accountId}:${server.region}` !== parsedScope.value) return false;
      if (parsedScope.kind === "region" && `${server.provider}:${server.region}` !== parsedScope.value) return false;
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
  }, [agentFilter, keyword, parsedScope.kind, parsedScope.value, statusFilter]);

  const selectedServer = useMemo(() => {
    return servers.find((server) => server.id === selectedServerId) ?? filteredServers[0] ?? servers[0];
  }, [filteredServers, selectedServerId]);

  const selectedAccount = selectedServer ? getAccount(selectedServer.accountId) : undefined;

  const summary = useMemo(() => {
    const warningServers = servers.filter((server) => server.status === "warning" || server.status === "offline").length;
    const agentIssues = servers.filter((server) => server.agentStatus !== "online").length;
    const alertCount = servers.reduce((total, server) => total + server.alerts, 0);
    const fileCount = servers.reduce((total, server) => total + server.files, 0);
    return { warningServers, agentIssues, alertCount, fileCount };
  }, []);

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
  }, []);

  const renderServerTable = (rows: CloudServer[]) => (
    <table className="data-table server-list-table">
      <thead>
        <tr>
          <th>服务器</th>
          <th>云平台 / 账号</th>
          <th>地域 / 可用区</th>
          <th>IP</th>
          <th>状态</th>
          <th>CPU</th>
          <th>内存</th>
          <th>磁盘</th>
          <th>探针</th>
          <th>GWF 闭环</th>
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
              <td>
                <strong>{server.name}</strong>
                <span>{server.instanceId}</span>
                <span>{server.business}</span>
              </td>
              <td>
                <strong>{providerLabels[server.provider]}</strong>
                <span>{account?.alias ?? "--"}</span>
              </td>
              <td>
                <strong>{server.region}</strong>
                <span>{server.zone}</span>
              </td>
              <td>
                <span>公网：{server.publicIp}</span>
                <span>内网：{server.privateIp}</span>
              </td>
              <td>
                <StatusText status={server.status}>{serverStatusLabels[server.status]}</StatusText>
              </td>
              <td><Utilization value={server.cpu} /></td>
              <td><Utilization value={server.memory} /></td>
              <td><Utilization value={server.disk} /></td>
              <td>
                <StatusText status={server.agentStatus}>{agentStatusLabels[server.agentStatus]}</StatusText>
                <span>{server.lastSeen}</span>
              </td>
              <td>
                <span>告警 {server.alerts} / 文件 {server.files}</span>
                <span>AI {server.ai} / KB {server.kb}</span>
              </td>
            </tr>
          );
        })}
        {!rows.length ? (
          <tr>
            <td className="empty-cell" colSpan={10}>当前筛选条件下没有服务器</td>
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
      <button className={scope === buildScopeKey("all") ? "tree-node root active" : "tree-node root"} onClick={() => setScope(buildScopeKey("all"))}>
        <span className="tree-label">全部云平台</span>
        <em>{servers.length}</em>
      </button>
      {(["aliyun", "huawei", "tencent"] as Provider[]).map((provider) => {
        const providerServers = servers.filter((server) => server.provider === provider);
        const providerAccounts = accounts.filter((account) => account.provider === provider);
        if (!providerServers.length && !providerAccounts.length) return null;
        return (
          <div className="tree-group" key={provider}>
            <button
              className={scope === buildScopeKey("provider", provider) ? "tree-node provider active" : "tree-node provider"}
              onClick={() => setScope(buildScopeKey("provider", provider))}
            >
              <span className="tree-label">{providerLabels[provider]}</span>
              <em>{providerServers.length}</em>
            </button>
            {providerAccounts.map((account) => {
              const accountServers = servers.filter((server) => server.accountId === account.id);
              const accountRegions = [...new Set(accountServers.map((server) => server.region))];
              return (
                <div className="account-branch" key={account.id}>
                  <button
                    className={scope === buildScopeKey("account", account.id) ? "tree-node account active" : "tree-node account"}
                    onClick={() => setScope(buildScopeKey("account", account.id))}
                  >
                    <span className="tree-label">{account.alias}</span>
                    <em>{accountServers.length}</em>
                  </button>
                  {accountRegions.map((region) => (
                    <button
                      className={scope === buildScopeKey("accountRegion", `${account.id}:${region}`) ? "tree-node region active" : "tree-node region"}
                      key={`${account.id}:${region}`}
                      onClick={() => setScope(buildScopeKey("accountRegion", `${account.id}:${region}`))}
                    >
                      <span className="tree-label">{region}</span>
                      <em>{accountServers.filter((server) => server.region === region).length}</em>
                    </button>
                  ))}
                </div>
              );
            })}
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
            <option value="all">全部探针</option>
            <option value="online">在线</option>
            <option value="stale">心跳过期</option>
            <option value="missing">未接入</option>
          </select>
          <button type="button">同步资产</button>
          <button type="button">导出</button>
        </div>

        <div className="simple-summary">
          <span>云账号：{accounts.length}</span>
          <span>服务器：{servers.length}</span>
          <span>异常服务器：{summary.warningServers}</span>
          <span>探针异常：{summary.agentIssues}</span>
          <span>今日文件入云：{summary.fileCount}</span>
          <span>待关注告警：{summary.alertCount}</span>
        </div>

        <div className="table-panel">{renderServerTable(filteredServers)}</div>

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
              <tr><th>公网 IP</th><td>{selectedServer.publicIp}</td><th>内网 IP</th><td>{selectedServer.privateIp}</td></tr>
              <tr><th>系统</th><td>{selectedServer.os}</td><th>规格</th><td>{selectedServer.spec}</td></tr>
              <tr><th>运行状态</th><td>{serverStatusLabels[selectedServer.status]}</td><th>探针状态</th><td>{agentStatusLabels[selectedServer.agentStatus]} / {selectedServer.lastSeen}</td></tr>
              <tr><th>负载</th><td>{selectedServer.load}</td><th>运行时长</th><td>{selectedServer.uptime}</td></tr>
              <tr><th>GWF 闭环</th><td colSpan={3}>文件 {selectedServer.files}，告警 {selectedServer.alerts}，AI {selectedServer.ai}，知识库命中 {selectedServer.kb}</td></tr>
            </tbody>
          </table>
        </section>
      </section>
    </div>
  );

  const renderAccountsPage = () => (
    <section className="page-main-section">
      <div className="toolbar">
        <button type="button">新增云账号</button>
        <button type="button">校验凭据</button>
        <button type="button">同步全部账号</button>
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
            </tr>
          </thead>
          <tbody>
            {accounts.map((account) => (
              <tr key={account.id}>
                <td>{providerLabels[account.provider]}</td>
                <td><strong>{account.name}</strong><span>{account.alias}</span></td>
                <td>{account.uid}</td>
                <td>{account.owner}</td>
                <td>{account.env}</td>
                <td>{account.regions.join("、")}</td>
                <td>{servers.filter((server) => server.accountId === account.id).length}</td>
                <td><StatusText status={account.status}>{accountStatusLabels[account.status]}</StatusText></td>
                <td>{account.lastSync}</td>
              </tr>
            ))}
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
                  setScope(buildScopeKey("region", `${row.provider}:${row.region}`));
                  setActivePage("servers");
                }}>查看服务器</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );

  const renderMonitorPage = () => (
    <section className="page-main-section">
      <div className="table-panel">{renderServerTable(servers)}</div>
    </section>
  );

  const renderAgentsPage = () => (
    <section className="page-main-section">
      <div className="toolbar">
        <button type="button">生成安装命令</button>
        <button type="button">批量检查心跳</button>
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
                  <td>{server.agentStatus === "online" ? "保持现有采集策略" : "检查 Agent 服务或补装探针"}</td>
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
                  <td>{server.agentStatus === "online" ? "采集正常" : "等待探针恢复"}</td>
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
          <tr><th>GWF 闭环</th><td>按服务器关联文件入云、告警决策、AI 分析、知识库命中</td></tr>
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
            <strong>GWF</strong>
            <span>多云运维管理后台</span>
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
            <p>聚合阿里云、华为云等多个云平台账号下的服务器，并关联监控、探针、告警、文件入云、AI 分析和知识库复用。</p>
          </div>
          <div className="header-meta">
            <span>当前为前端样例数据</span>
            <button type="button" onClick={() => setSidebarHidden((current) => !current)}>
              {sidebarHidden ? "展开菜单" : "隐藏菜单"}
            </button>
            <button type="button" onClick={() => setTheme((current) => (current === "light" ? "dark" : "light"))}>
              {theme === "light" ? "深色模式" : "浅色模式"}
            </button>
            <button type="button">刷新</button>
          </div>
        </header>
        <div className="admin-content">{renderPage()}</div>
      </section>
    </main>
  );
}
