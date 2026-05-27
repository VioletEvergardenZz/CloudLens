# service 目录地图

`service/` 放长生命周期服务。这里不是普通业务 handler，而是负责协调 watcher、上传池、运行态状态、通知和告警热更新。

## FileService 阅读顺序

| 文件 | 作用 |
| --- | --- |
| `file_service.go` | `FileService` 结构体、常量和构造函数 |
| `dependencies.go` | OSS、钉钉、邮件、上传池、watcher 的依赖初始化 |
| `lifecycle.go` | `Start` / `Stop` 生命周期 |
| `runtime_config.go` | 控制台配置热更新、watcher 和上传池重建 |
| `upload_flow.go` | 文件入队、上传重试、通知发送、队列限流 |
| `health.go` | `/api/health` 所需的队列和失败原因快照 |
| `alert_runtime.go` | 告警状态暴露、告警配置和规则热更新 |
| `upload_ai.go` | 文件上传通知中的 AI 摘要辅助能力 |

## 维护规则

- 文件上传主路径只从 `upload_flow.go` 进入。
- 运行态配置变更只从 `runtime_config.go` 修改，避免多个文件各自重建组件。
- 与并发、热更新、回滚、限流有关的逻辑必须保留中文注释，说明原因和边界。
- 文件入云当前是扩展能力，不再扩大为产品主线；新增能力应优先服务云资源监控主线。
