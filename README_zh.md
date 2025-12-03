# k8s-monitor

[English](README.md) | [中文](README_zh.md)

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> 轻量级、只读的 Kubernetes 集群终端监控工具

## 📋 概述

k8s-monitor 是一个基于终端的 Kubernetes 集群监控工具，专为需要通过 SSH 快速了解集群健康状态的运维工程师设计。它提供实时监控和直观的键盘驱动界面。

![k8s-monitor 演示](images/k8s-monitor.gif)

### 核心特性

- **🎯 全面视图**：8 个专门视图覆盖所有集群资源
- **📊 资源监控**：实时 CPU/内存/网络指标，带可视化进度条
- **🚀 NPU 监控**：华为昇腾 NPU 支持，提供详细芯片指标
- **📈 趋势分析**：历史指标跟踪和趋势指示器
- **🔍 智能诊断**：自动检测 CrashLoop、失败 Pod、节点压力
- **📝 日志查看器**：实时查看和搜索 Pod 日志
- **🛡️ 只读模式**：生产环境安全 - 不修改集群
- **⚡ 快速轻量**：单一二进制，最小依赖
- **🌍 国际化支持**：中英文界面

## 🚀 快速开始

### 安装

#### 从源码构建

```bash
git clone https://github.com/yourusername/k8s-monitor.git
cd k8s-monitor
make build
sudo make install
```

#### 使用 Go Install

```bash
go install github.com/yourusername/k8s-monitor/cmd/k8s-monitor@latest
```

### 使用方法

```bash
# 启动交互式控制台
k8s-monitor console

# 指定 kubeconfig 文件
k8s-monitor console --kubeconfig ~/.kube/config

# 使用特定上下文
k8s-monitor console --context my-cluster

# 监控特定命名空间
k8s-monitor console --namespace production

# 设置语言（en/zh）
k8s-monitor console --locale zh

# 查看所有选项
k8s-monitor --help
```

## ✨ 功能特性

### 核心功能 (v0.1.1)

#### 📊 集群概览
- 集群范围资源汇总（CPU、内存、Pod）
- 彩色进度条显示容量、可分配量、请求量和使用量
- 自动计算利用率百分比
- 最近事件和告警摘要

#### 🖥️ 节点监控
- 实时节点指标（CPU、内存、网络）
- 每个节点的 Pod 分布
- 节点状态和污点
- 按名称、CPU、内存或 Pod 数量排序
- 资源使用趋势指示器

#### 🚀 NPU 监控（华为昇腾）
- NPU 容量和分配跟踪
- 每个芯片的详细指标：
  - AI Core 利用率
  - Vector 利用率
  - HBM 内存使用
  - 温度和功耗
  - 电压和频率
  - 链路状态
  - RoCE 网络统计
  - ECC 错误跟踪
- 拓扑信息（SuperPod、HyperNode）
- 与 NPU-Exporter 集成获取运行时指标

#### 📦 Pod 管理
- Pod 列表显示状态、重启次数、资源使用
- 按命名空间、状态过滤或按名称搜索
- 容器级别详细信息
- 资源请求和限制跟踪
- 每个 Pod 的网络指标

#### ⚙️ 工作负载管理
- Jobs、Deployments、StatefulSets、DaemonSets、CronJobs
- 状态跟踪和副本数
- 详细的资源规格
- 导航到相关 Pod

#### 🌐 网络视图
- 服务类型、集群 IP 和端口
- 端点跟踪
- 网络流量监控（接收/发送速率）

#### 💾 存储视图
- PersistentVolumes 和 PersistentVolumeClaims
- 容量、状态和访问模式
- 存储类信息

#### 📋 事件与告警
- Kubernetes 事件过滤（警告/正常）
- 系统生成的健康告警
- 事件搜索和排序

#### 📝 Pod 日志
- 实时日志查看，自动刷新
- 日志搜索和高亮
- 自动滚动到最新日志
- 支持多容器 Pod

#### 🎬 操作菜单
- Pod 和节点的快速操作
- 执行 kubectl 命令
- 复制资源信息到剪贴板

### 高级功能

- **Vim 风格导航**：`j/k` 上下移动，`Enter` 查看详情，`Esc` 返回
- **快速视图切换**：数字键 `1-8` 快速导航
- **灵活过滤**：按命名空间、状态、标签过滤
- **全文搜索**：按名称搜索资源
- **数据导出**：导出视图数据为 CSV/JSON
- **自动刷新**：可配置的后台刷新间隔
- **指标历史**：10 个快照滑动窗口用于趋势计算
- **网络速率计算**：20 秒时间窗口滑动平均，确保指标稳定

## 🎮 键盘快捷键

### 全局快捷键
| 按键 | 操作 |
|-----|------|
| `q` / `Ctrl+C` | 退出应用 |
| `r` | 手动刷新 |
| `1-8` | 切换到特定视图（1=概览，2=节点，3=Pod，等） |
| `Tab` | 循环切换视图 |

### 列表视图快捷键
| 按键 | 操作 |
|-----|------|
| `↑` / `k` | 向上移动选择 |
| `↓` / `j` | 向下移动选择 |
| `PgUp` / `Ctrl+U` | 向上翻页 |
| `PgDn` / `Ctrl+D` | 向下翻页 |
| `Enter` | 查看详情 |
| `f` | 打开过滤面板 |
| `c` | 清除所有过滤器 |
| `s` | 循环排序顺序 |
| `/` | 按名称搜索 |
| `e` | 导出当前视图数据 |

### 详情视图快捷键
| 按键 | 操作 |
|-----|------|
| `↑` / `↓` | 滚动内容 |
| `PgUp` / `PgDn` | 向上/向下翻页 |
| `Esc` / `Backspace` | 返回列表视图 |
| `l` | 查看日志（仅 Pod 详情） |
| `a` | 打开操作菜单（Pod/节点详情） |

### 日志视图快捷键
| 按键 | 操作 |
|-----|------|
| `↑` / `↓` | 滚动日志 |
| `PgUp` / `PgDn` | 向上/向下翻页 |
| `/` | 在日志中搜索 |
| `Esc` | 退出日志视图 |

### 搜索/过滤模式快捷键
| 按键 | 操作 |
|-----|------|
| `文本` | 输入过滤文本 |
| `Backspace` | 删除字符 |
| `Esc` | 取消 |
| `Enter` | 应用过滤器 |

## ⚙️ 配置

配置文件查找顺序：
1. `./config/config.yaml`
2. `$HOME/.k8s-monitor/config.yaml`
3. `/etc/k8s-monitor/config.yaml`

配置示例：

```yaml
cluster:
  kubeconfig: ~/.kube/config
  context: ""
  namespace: ""

refresh:
  interval: 2s        # 自动刷新间隔
  cache_ttl: 10s      # 缓存过期时间

performance:
  max_concurrent: 10  # 最大并发 kubelet 查询数
  log_tail_lines: 200 # 获取的日志行数

ui:
  locale: zh          # 界面语言（en/zh）
  color_mode: auto    # 颜色模式（auto/always/never）
  default_view: overview

logging:
  level: info         # 日志级别（debug/info/warn/error）
  file: /tmp/k8s-monitor.log

# NPU 监控（华为昇腾）
# npu_exporter: ""    # 自定义 NPU-Exporter 端点（默认：通过 K8s API 代理自动检测）

# 仅用于测试环境 - 跳过 kubelet TLS 验证
# insecure_kubelet: false
```

### NPU 监控配置

启用华为昇腾加速器的 NPU 监控：

1. **前置条件**：集群中必须部署 NPU-Exporter
   ```bash
   # 检查 NPU-Exporter 是否可用
   kubectl get svc -n kube-system npu-exporter
   ```

2. **自动检测**：k8s-monitor 通过 Kubernetes API 代理自动连接 NPU-Exporter

3. **自定义端点**（可选）：如果 NPU-Exporter 部署在不同位置
   ```bash
   k8s-monitor console --npu-exporter http://custom-npu-exporter:8082
   ```

**NPU-Exporter 镜像**：`swr.cn-north-12.myhuaweicloud.com/hwofficial/npu-exporter:2.3.2`

## 🏗️ 架构

```
┌─────────────────────────────────────┐
│    CLI 界面层（Bubble Tea）         │
│         用户交互                    │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│      应用核心层                     │
│  ┌──────────┐    ┌──────────┐      │
│  │   视图   │    │   数据   │      │
│  │  管理器  │    │  管理器  │      │
│  └──────────┘    └──────────┘      │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│      数据源层                       │
│  ┌──────────┐    ┌──────────┐      │
│  │   API    │    │ Kubelet  │      │
│  │  Server  │    │  客户端  │      │
│  └──────────┘    └──────────┘      │
│        │              │             │
│  ┌──────────┐    ┌──────────┐      │
│  │   NPU    │    │ Volcano  │      │
│  │ Exporter │    │  客户端  │      │
│  └──────────┘    └──────────┘      │
│         ↓              ↓            │
│  ┌─────────────────────────┐       │
│  │    缓存与刷新机制       │       │
│  └─────────────────────────┘       │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│      Kubernetes 集群                │
│ (API Server, Kubelet, NPU-Exporter) │
└─────────────────────────────────────┘
```

**关键组件：**
- **UI 层**：使用 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 终端渲染（Elm 架构）
- **应用核心**：业务逻辑、视图管理、状态处理
- **数据源**：
  - 通过 [client-go](https://github.com/kubernetes/client-go) 访问 API Server
  - Kubelet Summary API 获取实时指标
  - NPU-Exporter 获取华为昇腾 NPU 指标（通过 K8s API 代理）
  - Volcano 客户端获取 HyperNode 拓扑（可选）
- **缓存层**：基于 TTL 的缓存和后台刷新

## 📖 文档

- [CLAUDE.md](CLAUDE.md) - 开发者指南
- [产品计划](docs/product_plan.md) - 产品愿景和路线图
- [技术设计](docs/technical_design.md) - 架构和实现细节
- [开发计划](docs/development_plan.md) - 开发进度跟踪
- [资源监控](docs/RESOURCE_MONITORING.md) - 详细资源监控指南

## 🛠️ 开发

### 前置要求

- Go 1.21+（推荐 Go 1.24+）
- 访问 Kubernetes 集群（用于测试）
- kubectl 配置有效的 kubeconfig

### 构建命令

```bash
# 安装依赖
make deps

# 构建二进制
make build

# 运行测试和覆盖率
make test

# 运行代码检查
make lint

# 格式化代码
make fmt

# 运行所有检查（格式化、vet、测试）
make check

# 多平台构建
make build-all

# 本地运行
make run

# 清理构建产物
make clean
```

### 项目结构

```
k8s-monitor/
├── cmd/k8s-monitor/        # 主入口
├── internal/               # 私有应用代码
│   ├── app/                # 应用核心（配置、生命周期）
│   ├── ui/                 # UI 层（Bubble Tea 模型和视图）
│   ├── datasource/         # 数据源客户端（API Server、Kubelet）
│   ├── cache/              # 缓存和刷新逻辑
│   ├── model/              # 数据模型
│   ├── i18n/               # 国际化
│   └── diagnostic/         # 诊断工具
├── config/                 # 配置文件
├── docs/                   # 文档
├── images/                 # 截图和演示
└── scripts/                # 构建和工具脚本
```

### 代码统计

| 组件 | 代码行数 | 说明 |
|------|---------|------|
| **总计** | **~22,300** | 纯 Go 实现 |
| `internal/ui` | ~15,200 | TUI 层（Bubble Tea 模型、视图、渲染） |
| `internal/datasource` | ~4,600 | 数据源（API Server、Kubelet、NPU-Exporter、Volcano） |
| `internal/model` | ~700 | 数据模型和类型定义 |
| `internal/cache` | ~550 | TTL 缓存和后台刷新 |
| `internal/app` | ~420 | 应用核心和配置 |
| `internal/i18n` | ~120 | 国际化支持 |
| `cmd/` | ~190 | CLI 入口 |

- **51 个 Go 源文件**
- **零运行时外部依赖** - 单一静态二进制
- **支持两种语言** - 中文和英文

### 运行测试

```bash
# 运行所有测试
make test

# 仅运行单元测试
make test-unit

# 生成覆盖率报告
make test-coverage
```

## 🤝 贡献

欢迎贡献！请随时提交问题和拉取请求。

### 如何贡献

1. Fork 仓库
2. 创建特性分支（`git checkout -b feature/amazing-feature`）
3. 提交更改（`git commit -m 'feat: add amazing feature'`）
4. 推送到分支（`git push origin feature/amazing-feature`）
5. 打开 Pull Request

### 提交规范

请遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：
- `feat:` 新功能
- `fix:` 错误修复
- `refactor:` 代码重构
- `docs:` 文档变更
- `test:` 测试添加/修改
- `chore:` 维护任务

## 🗺️ 路线图

### v0.2（进行中）
- [ ] 多集群支持
- [ ] 自定义告警规则
- [ ] 历史指标图表
- [ ] 通过 kubectl 集成编辑资源
- [ ] 高级标签过滤

### v0.3+（未来）
- [ ] 自定义视图插件系统
- [ ] 随时间聚合指标
- [ ] 导出到监控系统
- [ ] 配置文件

## 📝 许可证

本项目基于 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - 优秀的 TUI 框架
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - 终端样式
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes Go 客户端
- [Cobra](https://github.com/spf13/cobra) - CLI 框架
- [Viper](https://github.com/spf13/viper) - 配置管理

## 📧 联系方式

如有问题、建议或问题，请在 GitHub 上[提交 issue](https://github.com/yourusername/k8s-monitor/issues)。

---

用 ❤️ 为 Kubernetes 运维人员打造
