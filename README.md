# 云镜 CloudLens

云镜是一个面向个人运维和小团队的多云资源监控小工具。

当前主线很简单：

- 云账号接入
- 云服务器和数据库查看
- ECS 主机指标与 RDS 性能指标查看

扩展能力继续保留：

- 告警
- AI 摘要
- 知识库
- 文件入云

目前真正落地的是阿里云 ECS/RDS 只读监控，以及华为云 ECS 只读监控。

## 特性

- 轻量部署，默认使用本地 SQLite
- 已落地阿里云 ECS/RDS 与华为云 ECS 只读接入
- ECS 列表会统一汇总实例规格、网络、计费和到期状态
- RDS 会同步实例详情、连接端点，并按引擎查询性能参数
- 前端按账号、地域、资源和监控视图组织
- 文件入云能力按需开启，不强绑 OSS
- 系统资源监控不再作为现行主功能继续推进

## 快速开始

### 后端

```bash
cd cloudlens-backend
cp .env.example .env
go run ./cmd
```

如果你当前就在仓库根目录，也可以直接运行：

```bash
go run ./cloudlens-backend/cmd
```

后端默认可以不指定配置文件：会使用内置默认值、环境变量和当前工作目录下的 `.env`。如果你还想启用旧的文件入云静态配置，再显式传入 `-config ./cloudlens-backend/config.yaml`。

### 前端

```bash
cd cloudlens-frontend
npm install
npm run dev
```

### Docker Compose

```bash
cp .env.example .env
docker compose up --build -d
```

- 前端：`http://localhost:8081`
- 后端：`http://localhost:8082`

## 文档入口

- [主线文档](./docs/README.md)
- [后端说明](./cloudlens-backend/README.md)
- [前端说明](./cloudlens-frontend/README.md)

## 当前说明

- 当前真正落地的云厂商能力包含阿里云 ECS/RDS 和华为云 ECS
- 当前更像“多云资源监控主线先跑通”的版本，告警、AI、知识库会继续围绕资源异常排查收口
- 默认数据库是 SQLite，不需要单独起数据库容器
- 如果后面继续融合旧能力，建议把文件入云、AI、知识库都挂到“实例异常排查”场景里，而不是再拆成独立产品线
