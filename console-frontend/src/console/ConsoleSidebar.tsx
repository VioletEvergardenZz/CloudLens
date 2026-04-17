/**
 * 文件职责：承接当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于统一运维平台壳层导航，按工作域组织入口并保留页内分区跳转 */

import type { ConsoleView } from "../types";
import { PLATFORM_DOMAINS, resolveDomainByView } from "./platformNavigation";

type ConsoleSidebarProps = {
  view: ConsoleView;
  activeSection: string;
  sectionIds: string[];
  systemSectionIds: string[];
  registrySectionIds: string[];
  overviewSectionIds: string[];
  eventSectionIds: string[];
  registryCatalogSectionIds: string[];
  onViewChange: (view: ConsoleView) => void;
};

type NavMeta = {
  title: string;
  desc: string;
  badge: string;
};

const resolveConsoleMeta = (id: string): NavMeta => {
  switch (id) {
    case "overview":
      return { title: "总览", desc: "心跳、指标与风险摘要", badge: "状态" };
    case "config":
      return { title: "配置", desc: "上传、路由与降级配置", badge: "配置" };
    case "directory":
      return { title: "目录", desc: "监控范围与自动上传策略", badge: "目录" };
    case "files":
      return { title: "文件", desc: "文件列表、状态与队列", badge: "文件" };
    case "tail":
      return { title: "日志", desc: "Tail、检索与 AI 摘要", badge: "分析" };
    case "failures":
      return { title: "上传记录", desc: "最近动作、结果与失败样本", badge: "记录" };
    default:
      return { title: "监控", desc: "趋势、队列与失败原因", badge: "图表" };
  }
};

const resolveSystemMeta = (id: string): NavMeta => {
  switch (id) {
    case "system-overview":
      return { title: "概览", desc: "主机、负载与连接状态", badge: "概览" };
    case "system-resources":
      return { title: "资源", desc: "CPU、内存与磁盘占用", badge: "资源" };
    case "system-volumes":
      return { title: "分区", desc: "容量、使用率与空间风险", badge: "磁盘" };
    case "system-processes":
      return { title: "进程", desc: "进程筛选、排序与监听端口", badge: "进程" };
    default:
      return { title: "详情", desc: "指标、环境变量与处置入口", badge: "详情" };
  }
};

const resolveRegistryMeta = (id: string): NavMeta => {
  switch (id) {
    case "registry-overview":
      return { title: "接入说明", desc: "范围、边界与使用方式", badge: "说明" };
    case "registry-probe":
      return { title: "开始探测", desc: "域名输入、探测与导出", badge: "探测" };
    case "registry-summary":
      return { title: "接入摘要", desc: "类型、入口、TLS 与待确认项", badge: "摘要" };
    case "registry-details":
      return { title: "探测详情", desc: "DNS、HTTP、HTTPS 与 TLS 结果", badge: "详情" };
    case "registry-health":
      return { title: "健康候选", desc: "候选接口与命中状态", badge: "健康" };
    case "registry-raw":
      return { title: "原始结果", desc: "完整 JSON 输出与核对", badge: "原始" };
    default:
      return { title: "待确认", desc: "人工补充信息与下一步动作", badge: "下一步" };
  }
};

const resolveOverviewMeta = (id: string): NavMeta => {
  switch (id) {
    case "overview-summary":
      return { title: "全局状态", desc: "今日风险、待办与当班入口", badge: "摘要" };
    case "overview-timeline":
      return { title: "事件态势", desc: "告警趋势与事件时间线", badge: "趋势" };
    case "overview-runtime":
      return { title: "运行健康", desc: "文件入云、告警、AI 与执行主线", badge: "健康" };
    default:
      return { title: "待办与变化", desc: "待处理事项与最近变化", badge: "待办" };
  }
};

const resolveEventMeta = (id: string): NavMeta => {
  switch (id) {
    case "event-summary":
      return { title: "事件摘要", desc: "等级、状态与责任归属", badge: "摘要" };
    case "event-timeline":
      return { title: "时间线", desc: "事件流转与处置轨迹", badge: "时间线" };
    case "event-analysis":
      return { title: "分析区", desc: "AI、根因与日志摘要", badge: "分析" };
    default:
      return { title: "动作与知识", desc: "SOP、任务与审计入口", badge: "动作" };
  }
};

const resolveRegistryCatalogMeta = (id: string): NavMeta => {
  switch (id) {
    case "registry-catalog-overview":
      return { title: "台账总览", desc: "系统、环境与完整度", badge: "总览" };
    case "registry-catalog-list":
      return { title: "系统列表", desc: "系统筛选与目录列表", badge: "列表" };
    default:
      return { title: "系统详情", desc: "服务、路由、健康与配置", badge: "详情" };
  }
};

const resolveSectionConfig = (
  view: ConsoleView,
  sectionIds: string[],
  systemSectionIds: string[],
  registrySectionIds: string[],
  overviewSectionIds: string[],
  eventSectionIds: string[],
  registryCatalogSectionIds: string[]
) => {
  switch (view) {
    case "console":
      return { ids: sectionIds, resolver: resolveConsoleMeta };
    case "system":
      return { ids: systemSectionIds, resolver: resolveSystemMeta };
    case "registry":
      return { ids: registrySectionIds, resolver: resolveRegistryMeta };
    case "overview":
      return { ids: overviewSectionIds, resolver: resolveOverviewMeta };
    case "events":
      return { ids: eventSectionIds, resolver: resolveEventMeta };
    case "registryCatalog":
      return { ids: registryCatalogSectionIds, resolver: resolveRegistryCatalogMeta };
    default:
      return { ids: [] as string[], resolver: resolveConsoleMeta };
  }
};

export function ConsoleSidebar({
  view,
  activeSection,
  sectionIds,
  systemSectionIds,
  registrySectionIds,
  overviewSectionIds,
  eventSectionIds,
  registryCatalogSectionIds,
  onViewChange,
}: ConsoleSidebarProps) {
  const activeDomain = resolveDomainByView(view);
  const sectionConfig = resolveSectionConfig(
    view,
    sectionIds,
    systemSectionIds,
    registrySectionIds,
    overviewSectionIds,
    eventSectionIds,
    registryCatalogSectionIds
  );

  return (
    <aside className="sidebar platform-shell-nav">
      <div className="platform-brand-row">
        <div className="brand-logo brand-logo-small">
          <div className="brand-logo-mark">GWF</div>
          <div className="brand-logo-sub">Go Watch File</div>
        </div>
        <div className="platform-context">
          <div className="platform-context-title">统一运维工作台</div>
          <div className="platform-context-sub">值班驱动、事件收口、接入纳管、知识沉淀、执行回链</div>
        </div>
      </div>

      <div className="platform-domain-grid" role="tablist" aria-label="统一运维工作域导航">
        {PLATFORM_DOMAINS.map((domain) => {
          const active = domain.id === activeDomain.id;
          return (
            <button
              key={domain.id}
              className={`domain-card ${active ? "active" : ""}`}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => onViewChange(domain.views[0].id)}
            >
              <div className="domain-card-head">
                <span className="badge ghost">{domain.badge}</span>
                <span className="domain-card-step">{domain.views.length} 个工作台</span>
              </div>
              <div className="domain-card-title">{domain.title}</div>
              <div className="domain-card-desc">{domain.desc}</div>
              <div className="domain-card-objective">{domain.objective}</div>
            </button>
          );
        })}
      </div>

      <div className="platform-workspace-row">
        <div className="platform-workspace-copy">
          <div className="platform-workspace-eyebrow">{activeDomain.badge}</div>
          <div className="platform-workspace-title">{activeDomain.title}</div>
          <div className="platform-workspace-sub">{activeDomain.desc}</div>
        </div>

        <div className="platform-workspace-nav" role="tablist" aria-label={`${activeDomain.title}工作台切换`}>
          {activeDomain.views.map((workspace) => (
            <button
              key={workspace.id}
              className={`workspace-pill ${view === workspace.id ? "active" : ""}`}
              type="button"
              role="tab"
              aria-selected={view === workspace.id}
              onClick={() => onViewChange(workspace.id)}
            >
              <span className="workspace-pill-title">{workspace.label}</span>
              <span className="workspace-pill-desc">{workspace.desc}</span>
            </button>
          ))}
        </div>
      </div>

      {sectionConfig.ids.length ? (
        <nav className="platform-section-nav">
          {sectionConfig.ids.map((id) => {
            const meta = sectionConfig.resolver(id);
            return (
              <a key={id} className={`nav-item ${activeSection === id ? "active" : ""}`} href={`#${id}`}>
                <div className="nav-label">
                  <span className={`nav-dot ${activeSection === id ? "live" : ""}`} />
                  <div>
                    <div className="nav-label-title">{meta.title}</div>
                    <small>{meta.desc}</small>
                  </div>
                </div>
                <span className="badge ghost">{meta.badge}</span>
              </a>
            );
          })}
        </nav>
      ) : null}
    </aside>
  );
}
