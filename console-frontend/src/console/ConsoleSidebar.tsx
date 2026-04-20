/**
 * 文件职责：承接当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于统一运维平台侧边导航，采用更接近常规 Web 后台的左侧栏结构 */

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
    case "overview-header":
      return { title: "首页摘要", desc: "风险、动作和总览指标", badge: "摘要" };
    case "overview-incidents":
      return { title: "事件队列", desc: "当前优先处理的事件列表", badge: "事件" };
    case "overview-systems":
      return { title: "系统热点", desc: "按系统对象收敛当前风险面", badge: "系统" };
    default:
      return { title: "支撑动作", desc: "动作积压与闭环主线状态", badge: "动作" };
  }
};

const resolveEventMeta = (id: string): NavMeta => {
  switch (id) {
    case "event-header":
      return { title: "事件摘要", desc: "当前事件级别、系统与责任人", badge: "摘要" };
    case "event-context":
      return { title: "事件队列", desc: "左侧切换事件并查看时间线", badge: "队列" };
    case "event-analysis":
      return { title: "事件分析", desc: "概览、证据和复盘内容", badge: "分析" };
    default:
      return { title: "动作执行", desc: "负责人、推荐动作和关联任务", badge: "动作" };
  }
};

const resolveRegistryCatalogMeta = (id: string): NavMeta => {
  switch (id) {
    case "registry-header":
      return { title: "台账摘要", desc: "系统、环境、服务和热点统计", badge: "摘要" };
    case "registry-directory":
      return { title: "系统目录", desc: "用表格筛选和定位系统对象", badge: "目录" };
    default:
      return { title: "系统详情", desc: "页签化查看服务、事件和 Runbook", badge: "详情" };
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
  const primaryDomains = PLATFORM_DOMAINS.filter((domain) => domain.tier === "primary");
  const supportDomains = PLATFORM_DOMAINS.filter((domain) => domain.tier === "support");
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
    <aside className="sidebar platform-sidebar">
      <div className="sidebar-brand">
        <div className="brand-logo brand-logo-small">
          <div className="brand-logo-mark">GWF</div>
          <div className="brand-logo-sub">Go Watch File</div>
        </div>
      <div className="sidebar-brand-copy">
        <strong>统一运维工作台</strong>
        <span>先收住主线信息结构，再让接入、知识和执行围绕事件闭环服务。</span>
      </div>
      </div>

      <div className="sidebar-summary">
        <span className="badge ghost">{activeDomain.badge}</span>
        <strong>{activeDomain.title}</strong>
        <p>{activeDomain.desc}</p>
        <div className="sidebar-summary-objective">{activeDomain.objective}</div>
      </div>

      <div className="sidebar-group">
        <div className="sidebar-group-title">主线工作域</div>
        <div className="sidebar-nav-list">
          {primaryDomains.map((domain) => {
            const active = domain.id === activeDomain.id;
            return (
              <button
                key={domain.id}
                className={`sidebar-nav-button ${active ? "active" : ""}`}
                type="button"
                onClick={() => onViewChange(domain.views[0].id)}
              >
                <span className="sidebar-nav-label">{domain.title}</span>
                <span className="sidebar-nav-desc">{domain.desc}</span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="sidebar-group">
        <div className="sidebar-group-title">支撑工作域</div>
        <div className="sidebar-nav-list">
          {supportDomains.map((domain) => {
            const active = domain.id === activeDomain.id;
            return (
              <button
                key={domain.id}
                className={`sidebar-nav-button support ${active ? "active" : ""}`}
                type="button"
                onClick={() => onViewChange(domain.views[0].id)}
              >
                <span className="sidebar-nav-label">{domain.title}</span>
                <span className="sidebar-nav-desc">{domain.desc}</span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="sidebar-group">
        <div className="sidebar-group-title">当前页面</div>
        <div className="sidebar-workspace-list">
          {activeDomain.views.map((workspace) => (
            <button
              key={workspace.id}
              className={`sidebar-workspace-button ${view === workspace.id ? "active" : ""}`}
              type="button"
              onClick={() => onViewChange(workspace.id)}
            >
              <span className="sidebar-nav-label">{workspace.label}</span>
              <span className="sidebar-nav-desc">{workspace.desc}</span>
            </button>
          ))}
        </div>
      </div>

      {sectionConfig.ids.length ? (
        <div className="sidebar-group sidebar-section-group">
          <div className="sidebar-group-title">页内结构</div>
          <nav className="sidebar-section-nav">
            {sectionConfig.ids.map((id) => {
              const meta = sectionConfig.resolver(id);
              return (
                <a key={id} className={`sidebar-section-link ${activeSection === id ? "active" : ""}`} href={`#${id}`}>
                  <div className="sidebar-section-main">
                    <span className="sidebar-section-title">{meta.title}</span>
                    <span className="sidebar-section-desc">{meta.desc}</span>
                  </div>
                  <span className="badge ghost">{meta.badge}</span>
                </a>
              );
            })}
          </nav>
        </div>
      ) : null}
    </aside>
  );
}
