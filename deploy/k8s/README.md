# Kubernetes 部署

这个目录是 CloudLens 的 K8s 部署清单。新手优先看一个文件：

```bash
kubectl apply -f deploy/k8s/all-in-one.yaml
```

需要按模块维护时再使用 Kustomize：

```bash
kubectl apply -k deploy/k8s
```

修改清单后先做离线一致性检查，确认单文件入口和拆分清单没有漂移：

```bash
cloudlens-backend/scripts/ops/k8s-manifest-check.sh --k8s-dir deploy/k8s
```

部署完成后建议把后端 Service 临时转发到本地，再访问 `/api/runtime/checks`、`/api/cloud/diagnostics`、`/api/cloud/risks` 和 `/api/cloud/inspection-report` 做一次运维体检确认。

部署前需要先构建镜像：

```bash
docker build -t cloudlens-backend:local ./cloudlens-backend
docker build -t cloudlens-frontend:local ./cloudlens-frontend
```

kind 本地集群还需要执行：

```bash
kind load docker-image cloudlens-backend:local
kind load docker-image cloudlens-frontend:local
```

没有云账号也能启动。要接入真实云资源时，用 `cloud-credentials.example.yaml` 查看字段，再创建 `cloudlens-cloud-credentials` Secret，真实密钥不要提交到仓库。地域变量默认不配置，后端会按账号自动发现全部可见地域；只有临时排查或限定范围时再把 `ALIYUN_REGIONS`、`HUAWEI_REGIONS` 放进 Secret。

当前清单已经包含探针、资源限制、PVC、Secret 引用、ServiceAccount 和 NetworkPolicy。后端保持单副本是因为当前默认使用 SQLite。

## Kubernetes 只读巡检

CloudLens 新增了 `/api/k8s/overview`，用于读取 kind 或真实集群中的 Node、Namespace、Pod、Deployment 和 Warning Event。`/api/k8s/node-links` 会尝试把 K8s Node 与 CloudLens 统一资源索引里的 ECS 资源关联。这个能力只做巡检，不创建、不删除、不扩缩容 Kubernetes 资源。

本地开发时，后端优先读取：

```bash
CLOUDLENS_K8S_KUBECONFIG=/path/to/kubeconfig
CLOUDLENS_K8S_CONTEXT=kind-kind
```

如果未配置 `CLOUDLENS_K8S_KUBECONFIG`，会继续尝试 `KUBECONFIG` 和默认的 `~/.kube/config`。

集群内部署时，默认清单仍保持 `automountServiceAccountToken: false`，避免在未确认边界前让后端自动拿到集群凭据。确实需要让 CloudLens 巡检当前集群时，再显式执行：

```bash
kubectl apply -f deploy/k8s/k8s-readonly-rbac.yaml
```

同时需要把后端 Deployment 的 `automountServiceAccountToken` 调整为 `true`。这个改动会让后端 Pod 获取只读 Kubernetes API token，生产环境应先确认访问边界和网络策略。

更完整的启动和配置说明见：

```text
docs/02-开发运维/启动与配置.md
```
