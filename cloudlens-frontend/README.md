# CloudLens Console Frontend

`cloudlens-frontend/` 是云镜的控制台前端。

## 现在做什么

- 云账号管理
- 阿里云 ECS/RDS 与华为云 ECS 云资源列表
- ECS 到期剩余天数汇总
- ECS 主机监控与 RDS 性能概览
- 告警、知识库、文件工作台

默认入口已经切到 `CloudResourceConsole`，产品叙事也以多云资源监控为主。

## 运行

```bash
npm install
npm run dev
```

默认代理：

- `/api` -> `http://localhost:8080`

如需改后端地址，可设置 `VITE_API_BASE`。

## 构建

```bash
npm run lint
npm run build
```

## 重点文件

- `src/App.tsx`
- `src/CloudResourceConsole.tsx`
- `src/AlertConsole.tsx`
- `src/KnowledgeConsole.tsx`
- `src/console/dashboardApi.ts`

补充说明：

- 当前主界面支持阿里云 ECS/RDS 和华为云 ECS 监控
- RDS 指标页会按后端返回的性能参数动态展开，展示当前值、趋势、最近采样、窗口统计和原始 Key 信息
- 文件、AI、知识库页面后续更适合围绕实例异常排查来融合
- 系统资源控制台属于旧支线，不再作为现行主功能推进
