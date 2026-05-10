import type { ConsoleView } from "../types";

export type PlatformDomainId = "duty" | "incident" | "ingest" | "system" | "knowledge" | "execution";

export type WorkspaceMeta = {
  id: ConsoleView;
  label: string;
  desc: string;
};

export type PlatformDomainMeta = {
  id: PlatformDomainId;
  tier: "primary" | "support";
  title: string;
  desc: string;
  objective: string;
  badge: string;
  views: WorkspaceMeta[];
};

export const PLATFORM_DOMAINS: PlatformDomainMeta[] = [
  {
    id: "duty",
    tier: "primary",
    title: "总览",
    desc: "先看当前风险、事件队列、系统热点和动作积压，再决定进入哪个工作域。",
    objective: "总览 -> 事件 -> 系统",
    badge: "主线入口",
    views: [
      {
        id: "overview",
        label: "统一总览",
        desc: "标准后台首页，只保留当班最需要的风险与动作信息。",
      },
    ],
  },
  {
    id: "incident",
    tier: "primary",
    title: "事件",
    desc: "事件域拆成列表页和详情页，避免在多张概念卡里回跳。",
    objective: "事件中心 -> 事件详情 -> 动作回链",
    badge: "事件主线",
    views: [
      {
        id: "alert",
        label: "事件中心",
        desc: "筛选、排序和分诊事件，作为进入单事件详情的入口。",
      },
      {
        id: "events",
        label: "事件详情",
        desc: "围绕单条事件完成诊断、动作和复盘沉淀。",
      },
    ],
  },
  {
    id: "ingest",
    tier: "support",
    title: "接入",
    desc: "把文件和域名接入收拢成支撑域，服务于事件证据链与系统纳管。",
    objective: "接入 -> 确认 -> 纳管",
    badge: "支撑域",
    views: [
      {
        id: "console",
        label: "文件接入",
        desc: "围绕目录、上传、日志与 AI 分析组织文件入云主线。",
      },
      {
        id: "registry",
        label: "域名接入",
        desc: "输入域名并生成接入草案、健康候选和待确认项。",
      },
    ],
  },
  {
    id: "system",
    tier: "primary",
    title: "系统",
    desc: "系统目录负责定位，系统详情负责承接负责人、入口、服务与 Runbook。",
    objective: "系统目录 -> 单系统详情 -> 事件回链",
    badge: "系统主线",
    views: [
      {
        id: "registryCatalog",
        label: "系统台账",
        desc: "用目录表格和详情页签管理系统、环境、服务和负责人。",
      },
      {
        id: "system",
        label: "主机资源",
        desc: "查看资源、进程和主机排障信息，作为系统详情的技术补面。",
      },
    ],
  },
  {
    id: "knowledge",
    tier: "support",
    title: "知识",
    desc: "把知识库从文档仓库升级为事件复用和 SOP 沉淀中心。",
    objective: "事件 -> SOP -> 知识复用",
    badge: "沉淀域",
    views: [
      {
        id: "knowledge",
        label: "知识中心",
        desc: "统一检索、编辑、审核和复用运维知识。",
      },
    ],
  },
  {
    id: "execution",
    tier: "support",
    title: "执行",
    desc: "围绕任务、Agent 和审计组织动作落地，把执行结果挂回事件闭环。",
    objective: "动作下发 -> 执行追踪 -> 审计回链",
    badge: "执行域",
    views: [
      {
        id: "control",
        label: "执行中心",
        desc: "查看任务队列、Agent 状态、失败原因和动作审计。",
      },
    ],
  },
];

export const WORKSPACE_META: Record<ConsoleView, WorkspaceMeta> = Object.fromEntries(
  PLATFORM_DOMAINS.flatMap((domain) => domain.views.map((view) => [view.id, view]))
) as Record<ConsoleView, WorkspaceMeta>;

export const resolveDomainByView = (view: ConsoleView): PlatformDomainMeta => {
  return PLATFORM_DOMAINS.find((domain) => domain.views.some((item) => item.id === view)) ?? PLATFORM_DOMAINS[0];
};
