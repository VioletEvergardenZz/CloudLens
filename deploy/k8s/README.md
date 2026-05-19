# Kubernetes 部署

这个目录是 CloudLens 的 K8s 部署清单。新手优先看一个文件：

```bash
kubectl apply -f deploy/k8s/all-in-one.yaml
```

需要按模块维护时再使用 Kustomize：

```bash
kubectl apply -k deploy/k8s
```

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

没有云账号也能启动。要接入真实云资源时，用 `cloud-credentials.example.yaml` 查看字段，再创建 `cloudlens-cloud-credentials` Secret，真实密钥不要提交到仓库。

当前清单已经包含探针、资源限制、PVC、Secret 引用、ServiceAccount 和 NetworkPolicy。后端保持单副本是因为当前默认使用 SQLite。

更完整的启动和配置说明见：

```text
docs/02-开发运维/启动与配置.md
```
