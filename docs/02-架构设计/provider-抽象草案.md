# Provider 抽象草案

## 目标

当前阶段只为阿里云 ECS/RDS 与华为云 ECS/RDS 的只读查询沉淀共同边界，不提前引入复杂调度、写操作或完整多租户模型。

## 最小接口边界

- `ListInstances`：返回云服务器共同字段，复用 `internal/cloud/common.Instance`
- `MetricWithDimensions`：按实例、地域、指标名和维度返回 `internal/cloud/common.MetricSeries`
- `MetricDimensions`：用于华为云这类按挂载点、设备名扩展维度的指标发现
- `ProviderName`：用于前端和脚本识别云平台

## 暂不抽象

- 云账号加密存储
- RDS 专属性能参数
- 告警、AI、知识库联动
- 云资源写操作
- 后台采集调度和缓存

## 下一步落地顺序

1. 先把阿里云 ECS 与华为云 ECS 的只读接口适配到同一组小接口。
2. 保留 RDS 作为数据库资源扩展，不强行塞进 ECS provider。
3. 等真实华为云样本稳定后，再考虑把监控指标候选表按 provider 收敛。
4. 告警和知识库只消费统一后的资源标识，不反向依赖具体云 SDK。
