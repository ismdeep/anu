# anu

支持把当前目录同步到远端服务器，然后按配置执行部署动作。

目前支持的 `job.type`:

- `docker-compose`
- `make`
- `shell`
- `k8s` / `k8s-yaml` / `kubernetes`

## Kubernetes YAML 部署

当 `type` 为 `k8s` 时，`anu` 会：

1. 把当前目录同步到远端 `workdir`
2. 在远端 `workdir` 下查找所有 `*.yaml` 和 `*.yml`
3. 按文件名排序后逐个执行 `kubectl apply -f`

示例配置文件：`.lemuria/default.yaml`

```yaml
jobs:
  - type: k8s
    host: 1.2.3.4
    user: root
    port: 22
    workdir: /data/apps/my-service
```

部署时可直接执行：

```bash
anu apply
```
