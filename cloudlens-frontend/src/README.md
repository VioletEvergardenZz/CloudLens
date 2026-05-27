# 前端 src 目录地图

`src/` 是控制台前端代码。当前优先保证云资源监控主线可用，页面组织暂时以控制台功能为中心。

## 先看哪里

1. `main.tsx`：前端入口。
2. `App.tsx`：页面组合和主导航。
3. `console/`：控制台通用导航、头部、Dashboard API 和工具函数。
4. 云资源主线页面优先看 `CloudResourceConsole.tsx` 和 `cloud/pages/K8sInspectionPage.tsx`。

## 文件分组

| 路径 | 作用 |
| --- | --- |
| `App.tsx` / `App.css` | 应用入口和整体布局 |
| `console/` | 通用控制台骨架、导航和 Dashboard 数据工具 |
| `CloudResourceConsole.tsx` | 云账号、资源、监控概览主线页面 |
| `cloud/pages/` | 云资源相关的独立页面 |
| `AlertConsole.tsx` | 告警辅助能力 |
| `KnowledgeConsole.tsx` | 知识库辅助能力 |
| `ControlConsole.tsx` | 探针控制面 |
| `OverviewConsole.tsx` | 总览页 |
| `Registry*.tsx` | 镜像仓库与域名探测相关页面 |
| `types.ts` | 前端共享类型 |
| `mockData.ts` | 本地演示数据 |

## 维护规则

- 新增主线页面优先放进清晰目录，不继续无限平铺根目录。
- 控制台常用导航、API 工具和格式化函数优先放到 `console/`。
- 前端展示要和后端降级语义一致，不能把后端 `200` 降级数据当成异常直接打断页面。
