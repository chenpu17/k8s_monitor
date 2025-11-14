# k8s-monitor 快速启动指南

## 🚀 启动应用

### 方法 1：默认配置启动（推荐）

```bash
cd /root/git/Temps/k8s_monitor
./bin/k8s-monitor console
```

**效果**：干净的 TUI 界面，无任何日志干扰。

### 方法 2：自定义配置启动

```bash
./bin/k8s-monitor console --config config/default.yaml --refresh 15 -v
```

### 方法 3：监控特定 namespace

```bash
./bin/k8s-monitor console -n kube-system
```

---

## 📊 界面说明

启动后您会看到干净的 4 宫格界面：

```
╭─────────────────╮╭─────────────────╮
│ 📊 Cluster      ││ 🖥️  Nodes       │
│ Overview        ││                 │
╰─────────────────╯╰─────────────────╯
╭─────────────────╮╭─────────────────╮
│ 📦 Pods         ││ ⚠️  Events      │
│                 ││                 │
╰─────────────────╯╰─────────────────╯

q quit • r refresh • 1/2/3 views
```

### 快捷键

| 快捷键 | 功能 |
|--------|------|
| `1` | 📊 Overview（概览视图）|
| `2` | 🖥️  Nodes 列表 |
| `3` | 📦 Pods 列表 |
| `j` / `↓` | 向下滚动 |
| `k` / `↑` | 向上滚动 |
| `Enter` | 查看详情 |
| `Esc` | 返回列表 |
| `/` | 过滤（输入关键词）|
| `Tab` | 切换数据源 |
| `r` | 手动刷新 |
| `q` | 退出 |

---

## 📝 日志查看

**TUI 界面不会显示日志**（设计如此），所有日志输出到文件：

```bash
# 实时查看日志
tail -f /tmp/k8s-monitor.log

# 查看最近的错误
grep ERROR /tmp/k8s-monitor.log | tail -20

# 查看调试信息（需要 --verbose 启动）
grep DEBUG /tmp/k8s-monitor.log | tail -20
```

### 自定义日志路径

编辑 `config/default.yaml`：

```yaml
logging:
  level: info       # debug | info | warn | error
  file: /tmp/k8s-monitor.log
```

或使用环境变量：

```bash
# 启动应用时会创建日志文件
./bin/k8s-monitor console --config my-config.yaml
```

---

## ⚠️ 常见问题

### 1. 界面显示日志混乱（已修复 v0.1.1）

**症状**：TUI 界面被日志输出覆盖，无法正常显示。

**原因**：v0.1.0 版本错误地将日志同时输出到 stderr 和文件。

**解决**：已在 v0.1.1 修复，现在日志**仅**输出到文件。

### 2. TLS 证书警告

**症状**：日志中出现大量 `tls: failed to verify certificate` 警告。

**影响**：仅影响 Node/Pod 的 CPU/Memory 指标获取，不影响基本功能。

**临时解决**（测试环境）：

```bash
kubectl config set-cluster <cluster-name> --insecure-skip-tls-verify=true
```

**长期解决**：配置正确的 CA 证书或等待 v0.2 添加 `--insecure-kubelet` 选项。

### 3. 无法连接到集群

**检查 kubeconfig**：

```bash
kubectl cluster-info
kubectl get nodes
```

**指定 kubeconfig**：

```bash
./bin/k8s-monitor console -k ~/.kube/config
```

### 4. "could not open a new TTY" 错误

**原因**：在非终端环境（如脚本、CI/CD）中运行 TUI 应用。

**解决**：必须在**真实的终端**中运行，不能通过管道或后台运行。

---

## 🔧 配置调优

### 高频刷新（监控实时变化）

```bash
./bin/k8s-monitor console --refresh 5  # 每 5 秒刷新
```

### 降低资源占用

```yaml
# config/default.yaml
refresh:
  interval: 30s        # 降低刷新频率
  max_concurrent: 3    # 减少并发请求

cache:
  ttl: 60s             # 延长缓存时间
```

### 大集群优化

```yaml
refresh:
  timeout: 10s         # 增加超时时间
  max_concurrent: 20   # 增加并发数

cache:
  max_entries: 2000    # 增加缓存条目
```

---

## 🎯 使用场景

### 场景 1：健康检查

```bash
# 快速查看集群状态
./bin/k8s-monitor console

# 按 1 查看概览
# 检查 NotReady 节点数、Failed Pods 数
```

### 场景 2：问题排查

```bash
# 监控特定 namespace
./bin/k8s-monitor console -n production

# 按 3 查看 Pods
# 按 / 过滤: "Error" 或 "CrashLoop"
# 按 Enter 查看详情
```

### 场景 3：部署验证

```bash
# 高频刷新监控部署进度
./bin/k8s-monitor console --refresh 5 -n my-app

# 按 3 查看 Pods 列表
# 观察 Running 数量变化
```

---

## 📦 下一步

- 查看 [EXAMPLES.md](EXAMPLES.md) 了解更多使用示例
- 查看 [CHANGELOG.md](../CHANGELOG.md) 了解版本历史
- 查看 [development_plan.md](development_plan.md) 了解未来计划

---

**提示**：如遇到任何问题，请先查看日志文件 `/tmp/k8s-monitor.log`！
