/**
 * 文件职责：承接当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态，再调用接口同步；失败时给出可见反馈
 * 边界处理：对空数据、异常数据和超时请求提供兜底展示
 */

/* 本文件用于文件接入工作台的头部区域，统一呈现上下文、刷新状态与主题切换 */

type ConsoleHeaderProps = {
  agent: string;
  loading: boolean;
  error: string | null;
  timeframe: "realtime" | "24h";
  onTimeframeChange: (value: "realtime" | "24h") => void;
  theme: "dark" | "light";
  onThemeChange: (value: "dark" | "light") => void;
  eyebrow?: string;
  title?: string;
  description?: string;
};

export function ConsoleHeader({
  agent,
  loading,
  error,
  timeframe,
  onTimeframeChange,
  theme,
  onThemeChange,
  eyebrow = "接入工作域",
  title = "文件接入工作台",
  description = "统一查看监控目录、上传队列、日志检索与 AI 分析。",
}: ConsoleHeaderProps) {
  return (
    <header className="page-header">
      <div className="brand">
        <div className="title">
          <p className="eyebrow">{eyebrow}</p>
          <h1>{title}</h1>
          <p>{description}</p>
          <div className="title-meta">
            <span className="badge ghost">主机 {agent}</span>
          </div>
        </div>
      </div>
      <div className="controls">
        {loading ? <span className="badge">刷新中...</span> : null}
        {error ? (
          <>
            <span className="pill danger">接口异常</span>
            <span className="badge ghost">{error}</span>
          </>
        ) : null}
        <div className={`chip ${timeframe === "realtime" ? "active" : ""}`} onClick={() => onTimeframeChange("realtime")}>
          实时
        </div>
        <div className="theme-toggle">
          <span className="muted small">主题</span>
          <label className="switch mini">
            <input
              type="checkbox"
              aria-label="切换浅色和深色主题"
              checked={theme === "light"}
              onChange={(event) => onThemeChange(event.target.checked ? "light" : "dark")}
            />
            <span className="slider" />
          </label>
          <span className="badge ghost">{theme === "light" ? "浅色" : "深色"}</span>
        </div>
      </div>
    </header>
  );
}
