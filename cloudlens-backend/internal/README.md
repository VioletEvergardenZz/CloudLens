# internal 目录地图

`internal/` 放后端私有代码。新同学阅读时建议按这条路径走：

```text
cmd/main.go
  ↓
internal/api/routes.go
  ↓
internal/api/*_handlers.go
  ↓
internal/app/
  ↓
internal/cloud/ 或 internal/store/
```

## 分层说明

| 目录 | 作用 | 阅读建议 |
| --- | --- | --- |
| `api/` | HTTP 入口、路由、中间件、接口参数和响应 | 先看 `routes.go`，再按接口领域看 handler |
| `app/` | 应用服务层，承接跨存储、跨云厂商的业务编排 | 云资源主线优先看 `app/cloud` |
| `cloud/` | 云厂商只读适配器，封装阿里云、华为云 SDK | 只放云 API 调用和字段转换 |
| `store/` | SQLite 存储封装 | 只处理本地数据读写 |
| `service/` | 长生命周期运行态服务 | 文件监听、上传、告警热更新等放这里 |
| `alert/` | 告警规则、状态和通知闭环 | 作为辅助能力维护 |
| `kb/` | 知识库服务 | 作为辅助能力维护 |
| `k8s/` | Kubernetes 只读巡检适配器 | 只做只读查询 |
| `config/` | 配置加载和运行时配置写入 | 配置行为变更优先同步文档 |
| `models/` | 跨包共享模型 | 谨慎修改，避免扩大影响面 |
| `metrics/` | Prometheus 指标输出 | 可观测相关改动看这里 |
| `pathutil/` | 路径安全与目录处理 | 文件路径相关逻辑优先复用这里 |

## 当前约定

- 云资源主线优先走 `api -> app/cloud -> cloud/{aliyun,huawei} -> store`。
- 文件入云是扩展能力，集中在 `service/`、`watcher/`、`upload/`、`oss/`。
- 新增接口先在 `api/routes.go` 分组登记，再新建或复用对应 handler 文件。
- 不为了统一形式强行搬代码；当一个文件超过阅读负担时，再按业务职责拆分。
