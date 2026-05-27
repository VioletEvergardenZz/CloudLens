# API 目录地图

`api/` 是后端 HTTP 入口层，只负责请求参数、响应格式、路由分发和降级边界。业务编排尽量下沉到 `internal/app`、`internal/service`、`internal/cloud` 或 `internal/store`。

## 先看哪里

1. `server.go`：API 服务的创建、启动和关闭。
2. `routes.go`：所有接口路由地图，按业务域分组。
3. `middleware.go`：JSON、CORS、panic recovery 等横向逻辑。
4. 对应领域 handler：例如云资源看 `cloud_*.go` 和 `resource_handlers.go`，知识库看 `kb_handlers.go`。

## 文件分组

| 文件 | 作用 |
| --- | --- |
| `routes.go` | 路由地图，使用 `chi` 分组，保持 `net/http` handler 风格 |
| `server.go` | 服务生命周期和 handler 依赖聚合 |
| `middleware.go` | JSON 响应、CORS、Recovery |
| `dashboard_handlers.go` | `/api/dashboard` 和短缓存 |
| `config_handlers.go` | 运行态配置更新 |
| `health_handlers.go` | `/api/health` |
| `file_handlers.go` | 文件入云扩展接口 |
| `alert_config_handlers.go`、`alert_handlers.go` | 告警总览、配置、规则和人工处置 |
| `cloud_accounts_handlers.go` | 云账号管理 |
| `cloud_aliyun.go`、`cloud_huawei.go` | 云厂商专属只读接口 |
| `cloud_resources_unified.go`、`resource_handlers.go` | 统一资源索引和指标入口 |
| `cloud_ops_handlers.go` | 云体检 HTTP 入口 |
| `cloud_ops_types.go` | 云体检共享类型 |
| `cloud_ops_diagnostics.go` | 云账号诊断和资源快照摘要 |
| `cloud_ops_risks.go` | 快照风险识别 |
| `cloud_ops_runtime.go` | 本地运行体检 |
| `cloud_ops_report.go` | 轻量巡检报告和 Markdown 渲染 |
| `control_*.go` | 控制面探针和任务调度 |
| `kb_handlers.go` | 知识库接口 |
| `k8s_handlers.go` | Kubernetes 只读巡检接口 |
| `registry_domain_probe.go` | 镜像仓库域名探测 |

## 维护规则

- 新增路由必须先放进 `routes.go` 的对应分组。
- handler 内只做参数校验、错误映射和响应组装。
- `/api/health` 和 `/api/dashboard?mode=light` 要保持可降级，避免前端启动即报错。
- 路径解析和安全检查优先复用现有 helper，不在 handler 内手写危险路径拼接。
