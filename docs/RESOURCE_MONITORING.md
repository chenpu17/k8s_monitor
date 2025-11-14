# 资源监控增强 - v0.1.1

## 概述

v0.1.1 版本大幅增强了 Overview 视图，现在可以显示**集群级别的详细资源使用情况**。

## 新增监控指标

### 1. CPU 资源监控

显示内容：
- **Capacity（总容量）** - 集群所有节点的 CPU 总和
- **Allocatable（可分配）** - 扣除系统预留后可供 Pod 使用的 CPU
- **Requested（已请求）** - 所有 Pod 的 CPU requests 总和
  - 显示请求利用率：Requested / Allocatable * 100%
  - 彩色进度条可视化
- **Used（实际使用）** - 从 kubelet metrics 获取的实际 CPU 使用量
  - 显示使用利用率：Used / Capacity * 100%
  - 彩色进度条可视化

单位：
- CPU 以 **cores** 为单位显示
- 大于 1.0: 显示为 `45.2` (cores)
- 小于 1.0: 显示为 `800m` (millicores)

### 2. 内存资源监控

显示内容：
- **Capacity（总容量）** - 集群所有节点的内存总和
- **Allocatable（可分配）** - 扣除系统预留后可供 Pod 使用的内存
- **Requested（已请求）** - 所有 Pod 的 memory requests 总和
  - 显示请求利用率：Requested / Allocatable * 100%
  - 彩色进度条可视化
- **Used（实际使用）** - 从 kubelet metrics 获取的实际内存使用量
  - 显示使用利用率：Used / Capacity * 100%
  - 彩色进度条可视化

单位：
- 内存自动选择最合适单位：`Ti` / `Gi` / `Mi` / `Ki` / `B`
- 示例：`688.2Gi`、`123.4Gi`、`45.8Mi`

### 3. Pod 容量监控

显示内容：
- **Allocatable（可容纳）** - 集群最多可以运行的 Pod 数量
- **Used（已使用）** - 当前运行的 Pod 数量
- **Utilization（利用率）** - Used / Allocatable * 100%
- 彩色进度条可视化

### 4. 节点和 Pod 状态统计

**节点状态**：
- Total - 总节点数
- Ready - 就绪节点数（绿色）
- NotReady - 未就绪节点数（红色）

**Pod 状态**：
- Total - 总 Pod 数
- Running - 运行中（蓝色）
- Pending - 等待中（黄色）
- Failed - 失败（红色）
- Unknown - 未知状态（灰色）

### 5. 事件统计

- Total - 总事件数
- Warnings - 警告事件数（黄色）
- Errors - 错误事件数（红色）
- Recent - 最近 5 条事件预览

## 进度条颜色说明

进度条根据利用率自动着色：

| 利用率 | 颜色 | 含义 |
|--------|------|------|
| **90% 以上** | 🔴 红色 | 资源紧张，需要扩容 |
| **75-90%** | 🟡 黄色 | 资源偏高，需要关注 |
| **50-75%** | 🟠 橙色 | 资源正常，使用良好 |
| **50% 以下** | 🟢 绿色 | 资源充足 |

## 示例输出

```
╭─────────────────────────────────────────────────────────────────────────╮
│ 📊 Cluster Resources                                                    │
│                                                                          │
│ CPU (cores):                                                             │
│   Capacity:    172.0                                                     │
│   Allocatable: 168.0                                                     │
│   Requested:   45.2 (26.9%)                                              │
│   ████████░░░░░░░░░░░░░░░░░░░░                                           │
│   Usage: metrics unavailable                                             │
│                                                                          │
│ Memory:                                                                  │
│   Capacity:    688.2Gi                                                   │
│   Allocatable: 671.5Gi                                                   │
│   Requested:   123.4Gi (18.4%)                                           │
│   █████░░░░░░░░░░░░░░░░░░░░░░░                                           │
│   Usage: metrics unavailable                                             │
│                                                                          │
│ Pod Capacity:                                                            │
│   Allocatable: 4300                                                      │
│   Used:        607 (14.1%)                                               │
│   ████░░░░░░░░░░░░░░░░░░░░░░░░                                           │
╰─────────────────────────────────────────────────────────────────────────╯

╭───────────────────────────────╮╭───────────────────────────────╮
│ 🖥️  Nodes & 📦 Pods          ││ ⚠️  Events                    │
│                               ││                               │
│ Nodes:                        ││ Total:     10                 │
│   Total:     43               ││ Warnings:  10                 │
│   Ready:     43               ││ Errors:    0                  │
│   NotReady:  0                ││                               │
│                               ││ Recent:                       │
│ Pods:                         ││ • Node/192.168.23.51: Pro...  │
│   Total:     607              ││ • Node/192.168.25.50: Pro...  │
│   Running:   607              ││ • Node/192.168.16.91: Pro...  │
│   Pending:   0                ││ • Pod/prometheus-0: Sch...    │
│   Failed:    0                ││ • Pod/grafana-0: Started      │
│   Unknown:   0                ││                               │
╰───────────────────────────────╯╰───────────────────────────────╯
```

## 关于 "metrics unavailable"

如果您看到 `Usage: metrics unavailable`，说明：

1. **kubelet metrics 获取失败** - 通常是由于 TLS 证书验证问题
2. **不影响基本功能** - 您仍然可以看到：
   - ✅ Capacity（容量）
   - ✅ Allocatable（可分配）
   - ✅ Requested（请求量）
   - ❌ Used（实际使用量）- 这个需要 metrics

### 解决方法

**临时方案（测试环境）**：

```bash
# 跳过 kubelet TLS 验证
kubectl config set-cluster <cluster-name> --insecure-skip-tls-verify=true
```

**长期方案（等待 v0.2）**：

我们将在 v0.2 版本添加 `--insecure-kubelet` 配置选项，无需修改 kubeconfig。

## 数据来源

| 指标 | 数据源 | 说明 |
|------|--------|------|
| Capacity | API Server | Node.Status.Capacity |
| Allocatable | API Server | Node.Status.Allocatable |
| Requested | API Server | Pod.Spec.Containers[].Resources.Requests |
| Limited | API Server | Pod.Spec.Containers[].Resources.Limits |
| **Used** | **kubelet** | **/stats/summary endpoint** |

## 计算公式

```go
// 集群总容量
CPUCapacity = Σ(Node.Status.Capacity.CPU)
MemoryCapacity = Σ(Node.Status.Capacity.Memory)

// 集群可分配
CPUAllocatable = Σ(Node.Status.Allocatable.CPU)
MemoryAllocatable = Σ(Node.Status.Allocatable.Memory)

// 集群请求量（仅计算 Running 和 Pending 的 Pod）
CPURequested = Σ(Pod.Spec.Containers[].Resources.Requests.CPU)
MemoryRequested = Σ(Pod.Spec.Containers[].Resources.Requests.Memory)

// 集群实际使用（来自 kubelet metrics）
CPUUsed = Σ(Node.CPUUsage)
MemoryUsed = Σ(Node.MemoryUsage)

// 利用率
CPURequestUtilization = CPURequested / CPUAllocatable * 100%
CPUUsageUtilization = CPUUsed / CPUCapacity * 100%
MemRequestUtilization = MemoryRequested / MemoryAllocatable * 100%
MemUsageUtilization = MemoryUsed / MemoryCapacity * 100%
PodUtilization = TotalPods / PodAllocatable * 100%
```

## 与其他监控工具对比

| 功能 | k8s-monitor | kubectl top | Prometheus | Grafana |
|------|-------------|-------------|------------|---------|
| 集群总容量 | ✅ | ❌ | ✅ | ✅ |
| 集群可分配 | ✅ | ❌ | ✅ | ✅ |
| 集群请求量 | ✅ | ❌ | ✅ | ✅ |
| 实际使用量 | ✅* | ✅ | ✅ | ✅ |
| 可视化进度条 | ✅ | ❌ | ❌ | ✅ |
| 实时 TUI | ✅ | ❌ | ❌ | ❌ |
| 无需部署 | ✅ | ✅ | ❌ | ❌ |

\* 需要 kubelet metrics 可访问

## 使用建议

### 场景 1：容量规划

查看集群整体资源使用情况，决定是否需要扩容：

```bash
./bin/k8s-monitor console
# 按 1 查看 Overview
# 检查 CPU/Memory 的 Requested 利用率
# 如果 > 80%，考虑添加节点
```

### 场景 2：资源分配检查

验证 Pod 的 resource requests 是否合理：

```bash
# 如果 Requested 很低 (<30%) 但实际集群很慢
# 说明很多 Pod 没有设置 requests，导致过度调度
# 需要为 Pod 添加 resource requests
```

### 场景 3：性能问题排查

对比 Requested 和 Used：

```bash
# 如果 Requested 低但 Used 高
# 说明 Pod 实际使用超过了请求量
# 可能需要增加 requests 或添加 limits
```

## 下一步改进（v0.2）

计划在下一个版本添加：

- [ ] 按 namespace 分组的资源使用统计
- [ ] Top N 最消耗资源的 Pods
- [ ] 历史资源使用趋势图（sparkline）
- [ ] 导出资源报告（CSV/JSON）
- [ ] `--insecure-kubelet` 配置选项
- [ ] 支持 Metrics Server 作为数据源

---

**版本**: v0.1.1
**日期**: 2025-11-06
**改进**: 从简单计数到完整的资源监控系统
