# CloudLens Backend

`cloudlens-backend/` 是云镜后端主服务目录。

## 现在做什么

- 云账号管理
- 阿里云 ECS 资源和监控查询
- 告警、AI、知识库辅助能力
- 文件入云扩展链路

当前真正落地的主线是多云资源监控，文件监听上传已经退到扩展位。

## 运行

```bash
cp .env.example .env
go run ./cmd -config config.yaml
```

或构建后运行：

```bash
go build -o bin/cloudlens-server ./cmd
./bin/cloudlens-server -config config.yaml
```

## 配置要点

云监控主线主要看这些字段：

- `cloud_assets_enabled`
- `aliyun_access_key_id`
- `aliyun_access_key_secret`
- `aliyun_region`
- `aliyun_regions`

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
- `GET /api/cloud/aliyun/instances`
- `GET /api/cloud/aliyun/overview`

## 验证

```bash
go test ./... -count=1
```
