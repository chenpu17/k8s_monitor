# NPU Metrics Collector DaemonSet

这个 DaemonSet 用于收集华为昇腾 NPU 的运行时指标，并将其写入 Kubernetes Node Annotations，供 k8s-monitor 读取显示。

## 部署

```bash
# 部署到集群
kubectl apply -f npu-metrics-collector.yaml

# 检查运行状态
kubectl get daemonset -n kube-system npu-metrics-collector
kubectl get pods -n kube-system -l app=npu-metrics-collector

# 查看日志
kubectl logs -n kube-system -l app=npu-metrics-collector -f
```

## 验证

部署后，可以检查 Node annotations 是否被正确更新：

```bash
kubectl get node <node-name> -o jsonpath='{.metadata.annotations}' | jq . | grep npu
```

预期输出类似：
```json
{
  "npu.huawei.com/utilization": "45",
  "npu.huawei.com/hbm-total": "68719476736",
  "npu.huawei.com/hbm-used": "34359738368",
  "npu.huawei.com/hbm-utilization": "50",
  "npu.huawei.com/temperature": "55",
  "npu.huawei.com/power": "180",
  "npu.huawei.com/health": "Healthy",
  "npu.huawei.com/error-count": "0",
  "npu.huawei.com/aicore-count": "8",
  "npu.huawei.com/metrics-updated": "2024-01-01T12:00:00Z"
}
```

## 配置

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `INTERVAL` | `10` | 指标收集间隔（秒） |
| `NODE_NAME` | 自动获取 | 节点名称（自动从 Pod spec 获取） |

### 节点选择器

默认情况下，DaemonSet 会在所有节点上运行。如果只想在 NPU 节点上运行，可以修改 `nodeSelector`：

```yaml
nodeSelector:
  accelerator/huawei-npu: "true"
  # 或使用其他标签
  # node.kubernetes.io/instance-type: "npu-instance"
```

### 自定义镜像

默认使用 `bitnami/kubectl:latest` 镜像。如果需要使用包含 `npu-smi` 的自定义镜像，可以：

1. 构建包含 `npu-smi` 的镜像：

```dockerfile
FROM ubuntu:22.04

# 安装 kubectl
RUN apt-get update && apt-get install -y curl && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/arm64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# 复制 npu-smi（需要从宿主机获取）
COPY npu-smi /usr/local/bin/npu-smi
RUN chmod +x /usr/local/bin/npu-smi

# 复制 Ascend 驱动库（可选）
COPY Ascend /usr/local/Ascend
ENV LD_LIBRARY_PATH=/usr/local/Ascend/driver/lib64:$LD_LIBRARY_PATH
```

2. 更新 DaemonSet 使用自定义镜像：

```yaml
image: your-registry/npu-metrics-collector:latest
```

## 收集的指标

| 指标 | Annotation Key | 说明 |
|------|---------------|------|
| NPU 利用率 | `npu.huawei.com/utilization` | AI Core 利用率百分比 (0-100) |
| HBM 总量 | `npu.huawei.com/hbm-total` | HBM 显存总量（字节） |
| HBM 已用 | `npu.huawei.com/hbm-used` | HBM 显存已用（字节） |
| HBM 利用率 | `npu.huawei.com/hbm-utilization` | HBM 利用率百分比 |
| 温度 | `npu.huawei.com/temperature` | NPU 温度（摄氏度） |
| 功耗 | `npu.huawei.com/power` | NPU 功耗（瓦特） |
| 健康状态 | `npu.huawei.com/health` | Healthy/Warning/Unhealthy |
| 错误计数 | `npu.huawei.com/error-count` | NPU 错误数量 |
| AI Core 数量 | `npu.huawei.com/aicore-count` | 每节点 AI Core 数量 |
| 更新时间 | `npu.huawei.com/metrics-updated` | 最后更新时间戳 |

## 测试模式

如果节点上没有 `npu-smi` 命令，收集器会自动切换到测试模式，生成模拟数据。这在开发和测试环境中很有用。

## 清理

```bash
kubectl delete -f npu-metrics-collector.yaml
```

## 故障排除

### Pod 无法启动

1. 检查节点是否有 NPU 驱动：
   ```bash
   ls -la /usr/local/Ascend
   ls -la /usr/local/bin/npu-smi
   ```

2. 检查 RBAC 权限：
   ```bash
   kubectl auth can-i patch nodes --as=system:serviceaccount:kube-system:npu-metrics-collector
   ```

### 指标未更新

1. 查看 Pod 日志：
   ```bash
   kubectl logs -n kube-system -l app=npu-metrics-collector
   ```

2. 手动测试 npu-smi：
   ```bash
   kubectl exec -n kube-system <pod-name> -- npu-smi info
   ```
