# 云镜 CloudLens

<p align="center">
  <strong>面向个人运维和小团队的多云资源监控工具</strong>
</p>

<p align="center">
  <a href="./cloudlens-backend/go.mod"><img alt="Go" src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white"></a>
  <a href="./cloudlens-frontend/package.json"><img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=20232A"></a>
  <a href="./cloudlens-frontend/package.json"><img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-5.9-3178C6?logo=typescript&logoColor=white"></a>
  <a href="./cloudlens-frontend/package.json"><img alt="Vite" src="https://img.shields.io/badge/Vite-7.2-646CFF?logo=vite&logoColor=white"></a>
  <a href="./cloudlens-backend/go.mod"><img alt="SQLite" src="https://img.shields.io/badge/SQLite-local-003B57?logo=sqlite&logoColor=white"></a>
  <a href="./docker-compose.yml"><img alt="Docker" src="https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white"></a>
</p>

<p align="center">
  <img alt="阿里云 ECS" src="https://img.shields.io/badge/Alibaba%20Cloud-ECS%20%7C%20RDS-FF6A00?logo=alibabacloud&logoColor=white">
  <img alt="华为云 ECS" src="https://img.shields.io/badge/Huawei%20Cloud-ECS-C7000B?logo=huawei&logoColor=white">
  <img alt="当前阶段" src="https://img.shields.io/badge/stage-%E5%A4%9A%E4%BA%91%E7%9B%91%E6%8E%A7%E4%B8%BB%E7%BA%BF-2F855A">
</p>

---

CloudLens 当前聚焦一条主线：**云账号接入、云资源查看、主机与数据库指标查看**。

它不是一个复杂的一体化运维平台，而是先把个人和小团队最常用的多云资源可见性跑顺：接入云账号，看清 ECS/RDS 资源，快速判断实例状态、计费到期和监控指标。告警、AI、知识库和文件入云仍保留为辅助能力，后续会围绕资源异常排查继续完善入口和流程。

## 能力概览

| 能力 | 当前状态 | 说明 |
| --- | --- | --- |
| 云账号接入 | 已落地 | 支持从配置和控制台管理云账号 |
| 阿里云 ECS | 已落地 | 实例列表、规格、网络、计费、到期状态、主机指标 |
| 阿里云 RDS | 已落地 | 实例详情、连接端点、引擎性能参数与监控概览 |
| 华为云 ECS | 已落地 | 实例列表、到期状态、CES 云监控指标 |
| 本地 SQLite | 默认使用 | 不需要单独启动数据库，适合轻量部署 |
| 告警 | 辅助能力 | 后续围绕资源异常排查继续增强 |
| AI 摘要 | 辅助能力 | 默认关闭，可通过环境变量接入 |
| 知识库 | 辅助能力 | 保留排查资料沉淀能力 |
| 文件入云 | 扩展能力 | 当前不再作为产品主线推进 |

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 后端 | Go 1.24、标准库 HTTP、SQLite、阿里云 SDK、华为云 SDK |
| 前端 | React 19、TypeScript、Vite、Chart.js |
| 存储 | 本地 SQLite，默认落盘到 `data/` |
| 部署 | 本地运行、Docker Compose、Kubernetes |
| 配置 | 优先看 `docs/02-开发运维/启动与配置.md`，再按场景使用 `.env`、`config.yaml` 或 K8s Secret |

## 快速开始

如果你第一次接手项目，建议先看：[启动与配置](./docs/02-开发运维/启动与配置.md)。

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

后端默认可以不指定配置文件。它会使用内置默认值、环境变量和当前工作目录下的 `.env`。如果要启用旧的文件入云静态配置，再显式传入 `-config ./cloudlens-backend/config.yaml`。

### 前端

```bash
cd cloudlens-frontend
npm install
npm run dev
```

### Docker Compose

```bash
cp cloudlens-backend/.env.example .env
docker compose up --build -d
```

默认访问地址：

| 服务 | 地址 |
| --- | --- |
| 前端控制台 | `http://localhost:8081` |
| 后端接口 | `http://localhost:8082` |

## 配置入口

云监控主线主要关注这些环境变量：

```env
ALIYUN_ACCESS_KEY_ID=your-ak
ALIYUN_ACCESS_KEY_SECRET=your-secret
ALIYUN_REGION=cn-hangzhou
ALIYUN_REGIONS=cn-hangzhou

HUAWEI_ACCESS_KEY_ID=your-ak
HUAWEI_ACCESS_KEY_SECRET=your-secret
HUAWEI_PROJECT_ID=
HUAWEI_REGION=cn-south-1
HUAWEI_REGIONS=cn-south-1
```

完整配置示例见：[cloudlens-backend/.env.example](./cloudlens-backend/.env.example)。

## 常用接口

| 接口 | 用途 |
| --- | --- |
| `GET /api/health` | 健康检查，始终允许匿名访问 |
| `GET /api/cloud/accounts` | 云账号列表 |
| `GET /api/cloud/aliyun/instances` | 阿里云 ECS 实例列表 |
| `GET /api/cloud/aliyun/overview` | 阿里云 ECS 监控概览 |
| `GET /api/cloud/aliyun/rds/instances` | 阿里云 RDS 实例列表 |
| `GET /api/cloud/aliyun/rds/overview` | 阿里云 RDS 性能概览 |
| `GET /api/cloud/huawei/instances` | 华为云 ECS 实例列表 |
| `GET /api/cloud/huawei/overview` | 华为云 ECS 监控概览 |
| `GET /api/dashboard?mode=light` | 控制台轻量总览，运行态未就绪时返回降级数据 |

## 仓库结构

```text
CloudLens/
├── cloudlens-backend/     # Go 后端服务、云资源接口、告警、知识库与运维脚本
├── cloudlens-frontend/    # React + TypeScript 控制台
├── docs/                  # 当前主线文档
├── deploy/k8s/            # Kubernetes 部署清单
├── reports/               # 阶段回放与复盘产物
├── legacy/                # 历史方案归档
└── docker-compose.yml     # 本地容器化启动入口
```

## 文档入口

| 文档 | 说明 |
| --- | --- |
| [启动与配置](./docs/02-开发运维/启动与配置.md) | 本地、Docker Compose、Kubernetes 的启动入口和最小配置说明 |
| [主线文档](./docs/README.md) | 项目定位、架构、模块和计划入口 |
| [后端说明](./cloudlens-backend/README.md) | 后端运行、配置、接口和验证说明 |
| [前端说明](./cloudlens-frontend/README.md) | 前端运行、构建和重点文件说明 |
| [开发指南](./docs/02-开发运维/开发指南.md) | 本地开发、运维和回归要求 |

## 验证命令

```bash
cd cloudlens-backend && go test ./... -count=1
cd cloudlens-frontend && npm run build
```

macOS + nvm 场景下，如果 `node` 或 `npm` 命令不存在，先执行：

```bash
source /etc/profile
```

## 当前边界

- 当前真正落地的云厂商能力包含阿里云 ECS/RDS 和华为云 ECS。
- 当前版本优先保证资源可见性、监控可用性、可维护性和安全性。
- 默认数据库是 SQLite，不需要单独起数据库容器。
- 文件入云、AI、知识库会继续围绕“实例异常排查”场景融合，而不是拆成独立产品主线。
