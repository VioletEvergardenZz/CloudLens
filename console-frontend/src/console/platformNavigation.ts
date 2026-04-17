import type { ConsoleView } from "../types";

export type PlatformDomainId = "duty" | "incident" | "ingest" | "system" | "knowledge" | "execution";

export type WorkspaceMeta = {
  id: ConsoleView;
  label: string;
  desc: string;
};

export type PlatformDomainMeta = {
  id: PlatformDomainId;
  title: string;
  desc: string;
  objective: string;
  badge: string;
  views: WorkspaceMeta[];
};

export const PLATFORM_DOMAINS: PlatformDomainMeta[] = [
  {
    id: "duty",
    title: "值班",
    desc: "先看风险、待办和交接动态，值班页是所有动作的起点。",
    objective: "发现 -> 分诊 -> 进入工作台",
    badge: "值班主线",
    views: [
      {
        id: "overview",
        label: "值班驾驶舱",
        desc: "统一查看风险条、待办、时间线和主线健康。",
      },
    ],
  },
  {
    id: "incident",
    title: "事件",
    desc: "把批量分诊和单事件处置放到同一个工作域里，不再来回跳页。",
    objective: "分诊 -> 分析 -> 处置 -> 复盘",
    badge: "事件主线",
    views: [
      {
        id: "alert",
        label: "事件中心",
        desc: "批量筛选告警与事件，作为进入单事件处置的入口。",
      },
      {
        id: "events",
        label: "事件工作台",
        desc: "围绕单条事件完成 AI 分析、任务动作与知识联动。",
      },
    ],
  },
  {
    id: "ingest",
    title: "接入",
    desc: "把文件接入和域名接入收束到一条纳管主线上，减少孤立入口。",
    objective: "接入 -> 确认 -> 纳管草案",
    badge: "接入主线",
    views: [
      {
        id: "console",
        label: "文件接入",
        desc: "目录、上传队列、日志与 AI 分析的统一入口。",
      },
      {
        id: "registry",
        label: "域名接入",
        desc: "输入域名后生成接入草案、健康候选和待确认事项。",
      },
    ],
  },
  {
    id: "system",
    title: "系统",
    desc: "从系统目录进入单系统详情，承接纳管事实、健康状态和最近事件。",
    objective: "纳管目录 -> 系统详情 -> 排障上下文",
    badge: "系统主线",
    views: [
      {
        id: "registryCatalog",
        label: "系统台账",
        desc: "统一查看系统、环境、服务和配置完整度。",
      },
      {
        id: "system",
        label: "系统详情",
        desc: "查看资源、进程与主机排障信息，作为单系统排障上下文。",
      },
    ],
  },
  {
    id: "knowledge",
    title: "知识",
    desc: "把知识库从文档存放区变成事件沉淀、SOP 复用和经验回写中心。",
    objective: "事件 -> SOP -> 知识复用",
    badge: "沉淀主线",
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
    title: "执行",
    desc: "围绕任务执行、结果追踪和审计，把动作真正挂回事件闭环。",
    objective: "动作下发 -> 执行追踪 -> 审计回链",
    badge: "执行主线",
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
