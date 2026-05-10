/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于前端应用入口组件 当前默认进入云资产运维首页 */

import { CloudResourceConsole } from "./CloudResourceConsole";

function App() {
  return <CloudResourceConsole />;
}

export default App;
