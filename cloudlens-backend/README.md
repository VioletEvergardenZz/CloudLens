# CloudLens Backend

`cloudlens-backend/` 是云镜后端主服务目录。

## 现在做什么

- 云账号管理
- 阿里云 ECS/RDS 资源和监控查询
- 华为云 ECS 资源和 CES 云监控查询
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
- `aliyun_region`
- `aliyun_regions`
- `aliyun_metric_period`
- `huawei_access_key_id`
- `huawei_access_key_secret`
- `huawei_project_id`：可选；部分账号或企业项目场景可显式填写，控制台云账号表单也支持按账号保存 Project ID
- `huawei_region`
- `huawei_regions`
- `huawei_metric_period`

阿里云 RAM 权限建议至少包含：

- `AliyunECSReadOnlyAccess`
- `AliyunCloudMonitorReadOnlyAccess`
- `AliyunRDSReadOnlyAccess`

华为云 IAM 权限建议至少包含：

- ECS 只读权限
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

当前不建议单独起数据库容器。先用 SQLite，等以后真有多实例或协同写入需求，再迁 PostgreSQL。

## 常用接口

- `GET /api/health`
- `GET /api/cloud/accounts`
- `GET /api/cloud/aliyun/instances`：返回 ECS 基础信息，并附带 `chargeType`、`isSpot`、`spotStrategy`、`expiredAt`、`expiresInDays`、`expirationStatus`、`expirationMessage` 用于判断到期剩余天数；按量付费、抢占式实例和云厂商远期占位时间会标注为无固定到期日
- `GET /api/cloud/aliyun/overview`
- `GET /api/cloud/aliyun/rds/instances`：返回 RDS 实例基础信息、规格、存储、连接端点、到期状态和 `DescribeResourceUsage` 官方空间用量；详情或空间接口失败时会保留基础实例并在 `detailErrors` 标注局部错误
- `GET /api/cloud/aliyun/rds/overview`：按 RDS 引擎分批查询性能参数；单个性能 Key 不支持时只进入 `errors`，不影响其它指标返回
- `GET /api/cloud/huawei/instances`：返回华为云 ECS 基础信息，字段与阿里云 ECS 对齐；华为云 ECS 详情接口未返回包年包月到期时间时会标注为 `unknown`
- `GET /api/cloud/huawei/overview`：优先返回华为云 CES 官方 `SYS.ECS` 指标，内存、分区磁盘和负载在官方基础监控无数据时再使用 `AGT.ECS` 指标兜底；单个指标失败不会影响其它指标
- `GET /api/cloud/huawei/metrics`：返回单个 CES 指标序列，可通过 `namespace` 和 `metric` 指定指标

## 验证

```bash
go test ./... -count=1
```

主线接口轻量回归：

```bash
scripts/ops/cloud-mainline-check.sh --base-url http://localhost:8082 --output-file ../reports/cloud-mainline-check-$(date +%F).json
```

华为云真实账号回归：

```bash
scripts/ops/huawei-real-account-check.sh --base-url http://localhost:8082 --output-file ../reports/huawei-real-account-check-$(date +%F).json
```
