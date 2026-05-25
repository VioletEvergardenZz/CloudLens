# CloudLens Backend

`cloudlens-backend/` 是云镜后端主服务目录。

## 现在做什么

- 云账号管理
- 阿里云 ECS/RDS 资源和监控查询
- 华为云 ECS/RDS 资源和 CES 云监控查询
- 告警、AI、知识库辅助能力
- 文件入云扩展链路

当前真正落地的主线是多云资源监控，文件监听上传已经退到扩展位。

## 运行

```bash
cp .env.example .env
go run ./cmd
```

或构建后运行：

```bash
go build -o bin/cloudlens-server ./cmd
./bin/cloudlens-server
```

`-config` 现在是可选参数。不传时后端会使用内置默认值、环境变量和当前目录下的 `.env`，云账号、云资源和监控接口可以直接启动；需要加载 `config.yaml` 里的文件入云、日志文件等静态配置时再显式传 `-config config.yaml`。

## 配置要点

云监控主线主要看这些字段：

- `cloud_assets_enabled`
- `aliyun_access_key_id`
- `aliyun_access_key_secret`
- `aliyun_region` / `aliyun_regions`：可选，仅用于环境变量单账号兜底或临时限定调试；控制台云账号默认自动发现全部地域
- `aliyun_metric_period`
- `huawei_access_key_id`
- `huawei_access_key_secret`
- `huawei_region` / `huawei_regions`：可选，仅用于环境变量单账号兜底或临时限定调试；控制台云账号默认自动发现全部地域
- `huawei_metric_period`
- `api_cors_origins`：可选，未配置时默认放开跨域；生产环境建议设置为前端域名白名单。预检允许 `GET,POST,PUT,PATCH,DELETE,OPTIONS`

阿里云 RAM 权限建议至少包含：

- `AliyunECSReadOnlyAccess`
- `AliyunCloudMonitorReadOnlyAccess`
- `AliyunRDSReadOnlyAccess`

华为云 IAM 权限建议至少包含：

- ECS 只读权限
- RDS 只读权限
- CES 云监控只读权限

文件入云扩展主要看这些字段：

- `watch_dir`
- `file_ext`
- `bucket`
- `ak`
- `sk`
- `endpoint`
- `region`

默认规则：

- 不配 OSS 也能启动
- 不配 AI 也能启动
- `watch_dir` 为空时 watcher 保持 idle
- `system_resource_enabled` 保留为历史兼容开关，当前主线不使用

## 存储

默认直接用 SQLite：

- `data/cloud/cloud.db`
- `data/control/control.db`
- `data/kb/knowledge.db`

云账号密钥本机加密使用 `data/cloud/secret.key`，后端会把历史权限自动收紧到 `0600`。ECS/RDS 列表成功同步后，会在 `cloud_resource_snapshots` 表保存最近一次资源快照；云厂商 API 临时失败时，列表接口会返回 `degraded: true` 的旧快照，避免前端资源页直接断流。

当前不建议单独起数据库容器。先用 SQLite，等以后真有多实例或协同写入需求，再迁 PostgreSQL。

## 常用接口

- `GET /api/health`
- `GET /api/cloud/accounts`
- `GET /api/cloud/aliyun/instances`：返回 ECS 基础信息，并附带 `chargeType`、`isSpot`、`spotStrategy`、`expiredAt`、`expiresInDays`、`expirationStatus`、`expirationMessage` 用于判断到期剩余天数；按量付费、抢占式实例和云厂商远期占位时间会标注为无固定到期日
- `GET /api/cloud/aliyun/overview`：返回 ECS 常用监控指标，采样周期会兜底为有效秒数，并兼容云监控不同采样时间字段
- `GET /api/cloud/aliyun/rds/instances`：返回 RDS 实例基础信息、规格、存储、连接端点、到期状态和 `DescribeResourceUsage` 官方空间用量；详情或空间接口失败时会保留基础实例并在 `detailErrors` 标注局部错误
- `GET /api/cloud/aliyun/rds/overview`：按 RDS 引擎分批查询性能参数；单个性能 Key 不支持时只进入 `errors`，不影响其它指标返回
- `GET /api/cloud/huawei/instances`：返回华为云 ECS 基础信息，字段与阿里云 ECS 对齐；华为云 ECS 详情接口未返回包年包月到期时间时会标注为 `unknown`
- `GET /api/cloud/huawei/overview`：优先返回华为云 CES 官方 `SYS.ECS` 指标，内存、分区磁盘和负载在官方基础监控无数据时再使用 `AGT.ECS` 指标兜底；单个指标失败不会影响其它指标
- `GET /api/cloud/huawei/rds/instances`：返回华为云 RDS 实例基础信息、节点 ID、规格、存储、连接端点、到期状态和官方空间用量；空间接口失败时会保留基础实例并在 `detailErrors` 标注局部错误
- `GET /api/cloud/huawei/rds/overview`：从 CES `SYS.RDS` 查询 CPU、内存、QPS、连接数和 IOPS；单个指标失败不会影响其它指标返回
- `GET /api/cloud/huawei/metrics`：返回单个 CES 指标序列，可通过 `namespace` 和 `metric` 指定指标
- `GET /api/cloud/snapshots`：返回账号维度的 ECS/RDS 最近快照摘要
- `GET /api/cloud/diagnostics`：返回云账号启用状态、最近测试结果、期望权限和快照覆盖
- `GET /api/cloud/risks`：基于快照识别到期、公网暴露、RDS 存储水位、资源状态异常和快照陈旧
- `GET /api/cloud/inspection-report`：返回轻量巡检报告，支持 `?format=markdown`
- `GET /api/runtime/checks`：返回本地数据目录、密钥权限、CORS、Dashboard 降级和告警闭环检查

## 验证

```bash
go test ./... -count=1
```

K8s 清单一致性检查：

```bash
scripts/ops/k8s-manifest-check.sh --k8s-dir ../deploy/k8s
```

主线接口轻量回归：

```bash
scripts/ops/cloud-mainline-check.sh --base-url http://localhost:8082 --output-file ../reports/cloud-mainline-check-$(date +%F).json
```

华为云真实账号回归：

```bash
scripts/ops/huawei-real-account-check.sh --base-url http://localhost:8082 --output-file ../reports/huawei-real-account-check-$(date +%F).json
```
