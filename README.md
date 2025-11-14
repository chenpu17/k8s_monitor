# k8s-monitor

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> A lightweight, read-only CLI monitoring console for Kubernetes clusters

## 📋 Overview

k8s-monitor is a terminal-based monitoring tool for Kubernetes clusters, designed for operations engineers who need quick insights into cluster health via SSH. It provides:

- **🎯 One-screen Overview**: Cluster-wide resource usage with visual progress bars
- **📊 Resource Monitoring**: CPU/Memory capacity, requests, limits, and actual usage
- **📈 Utilization Metrics**: Automatic calculation of request and usage percentages
- **🔍 Quick Diagnostics**: Automatic detection of CrashLoops, failed pods, node pressure
- **🛡️ Read-only**: No cluster modifications, safe to use in production
- **⚡ Fast & Lightweight**: Single binary, minimal dependencies

### 🆕 v0.1.1 Highlights

**详细的集群资源视图**：
- 显示集群总 CPU 容量（如 `172.0 cores`）、可分配量、请求量、实际使用量
- 显示集群总内存容量（如 `688.2Gi`）、可分配量、请求量、实际使用量
- 彩色进度条实时可视化资源利用率
- 自动汇总所有节点和 Pod 的资源指标

```
📊 Cluster Resources

CPU (cores):
  Capacity:    172.0
  Allocatable: 168.0
  Requested:   45.2 (26.9%)
  ████████░░░░░░░░░░░░░░░░░░░░    <-- 彩色进度条

Memory:
  Capacity:    688.2Gi
  Allocatable: 671.5Gi
  Requested:   123.4Gi (18.4%)
  █████░░░░░░░░░░░░░░░░░░░░░░░
```

详见 [docs/RESOURCE_MONITORING.md](docs/RESOURCE_MONITORING.md)

## 🚀 Quick Start

### Installation

#### From Source

```bash
git clone https://github.com/yourusername/k8s-monitor.git
cd k8s-monitor
make build
sudo make install
```

#### Using Go Install

```bash
go install github.com/yourusername/k8s-monitor/cmd/k8s-monitor@latest
```

### Usage

```bash
# Start the interactive console
k8s-monitor console

# Specify kubeconfig
k8s-monitor console --kubeconfig ~/.kube/config

# Use specific context
k8s-monitor console --context my-cluster

# Monitor specific namespace
k8s-monitor console --namespace production

# See all options
k8s-monitor --help
```

## ✨ Features

### v0.1.1 (Latest)
- ✅ **详细资源监控**: 集群级别的 CPU/Memory 容量、分配、使用情况
- ✅ **可视化进度条**: 彩色进度条显示资源利用率（自动根据 90%/75%/50% 着色）
- ✅ **Pod 容量监控**: 显示集群最多可运行的 Pod 数和当前使用情况
- ✅ **请求量统计**: 汇总所有 Pod 的 resource requests 和 limits
- ✅ **实际使用量**: 从 kubelet metrics 获取真实的 CPU/Memory 使用情况
- ✅ **利用率计算**: 自动计算请求利用率和使用利用率百分比

### v0.1 MVP (Complete ✅)
- ✅ **Overview view**: Cluster health summary, node/pod statistics, recent events
- ✅ **Node view**: Detailed node metrics, resource usage, pod distribution
- ✅ **Pod view**: Pod list with namespace, status, restart count
- ✅ **Detail views**: Deep dive into node and pod information
- ✅ **Fast navigation**: Number keys (1/2/3) for instant view switching
- ✅ **Interactive filtering**: Filter pods by namespace with live preview
- ✅ **vim-style navigation**: j/k for up/down, Enter/Esc for drilling down/up
- ✅ **Manual refresh**: R key to refresh data on demand
- ✅ **Auto-refresh**: Background refresh with configurable interval
- ✅ **Color-coded status**: Visual indicators for Ready/NotReady/Pending/Failed

### v0.2 (Planned)
- ⏳ **Pod logs viewing**: View container logs from the TUI
- ⏳ **Resource editing**: Quick edits via kubectl edit integration
- ⏳ **Advanced filtering**: Filter by labels, status, and custom queries
- ⏳ **Search functionality**: Quick search across all resources
- ⏳ **Performance metrics**: CPU/Memory usage trends over time

### v0.3+ (Future)
- ⏳ **Multi-cluster support**: Switch between multiple clusters
- ⏳ **Historical data**: Track metrics over time with trends
- ⏳ **Alerts and notifications**: Custom alert rules
- ⏳ **Plugin system**: Extensible architecture for custom views

## 🎮 Keyboard Shortcuts

### Global Keys
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit application |
| `r` | Manual refresh |
| `1` | Switch to Overview view |
| `2` | Switch to Node view |
| `3` | Switch to Pod view |
| `Tab` | Cycle through views |
| `?` | Show help (future) |

### List View Keys
| Key | Action |
|-----|--------|
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `Enter` | View details |
| `f` | Open filter panel (Pod view only) |
| `c` | Clear filter (Pod view only) |

### Detail View Keys
| Key | Action |
|-----|--------|
| `Esc` / `Backspace` | Back to list view |

### Filter Mode Keys
| Key | Action |
|-----|--------|
| `↑` / `↓` | Select namespace |
| `Enter` | Apply filter |
| `Esc` | Cancel filter |

## ⚙️ Configuration

Configuration file locations (searched in order):
1. `./config/config.yaml`
2. `$HOME/.k8s-monitor/config.yaml`
3. `/etc/k8s-monitor/config.yaml`

Example configuration:

```yaml
cluster:
  kubeconfig: ~/.kube/config
  context: ""

refresh:
  interval: 10s
  timeout: 5s

ui:
  color_mode: auto
  default_view: overview

logging:
  level: info
  file: /tmp/k8s-monitor.log
```

See [config/default.yaml](config/default.yaml) for all options.

## 🏗️ Architecture

```
CLI Interface (Bubble Tea)
    ↓
Application Core (Data Manager, View Manager)
    ↓
Data Sources (API Server, kubelet, Metrics Server)
    ↓
Kubernetes Cluster
```

- **UI Layer**: Terminal rendering with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **Application Core**: Business logic, data aggregation, caching
- **Data Sources**: client-go for API Server, HTTP client for kubelet Summary API

## 📖 Documentation

- [Product Plan](docs/product_plan.md) - Product vision and roadmap
- [Technical Design](docs/technical_design.md) - Architecture and implementation details
- [Development Plan](docs/development_plan.md) - Development progress tracking

## 🛠️ Development

### Prerequisites

- Go 1.21+
- Access to a Kubernetes cluster (for testing)
- kubectl configured

### Build

```bash
# Install dependencies
make deps

# Build binary
make build

# Run tests
make test

# Run locally
make run
```

### Project Structure

```
k8s-monitor/
├── cmd/k8s-monitor/     # Main entry point
├── internal/            # Private application code
│   ├── app/             # Application core
│   ├── ui/              # UI layer (views, components)
│   ├── datasource/      # Data source clients
│   ├── model/           # Data models
│   └── utils/           # Utilities
├── pkg/                 # Public libraries
├── config/              # Configuration files
├── docs/                # Documentation
└── scripts/             # Build scripts
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please follow the [Conventional Commits](https://www.conventionalcommits.org/) specification.

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Excellent TUI framework
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes Go client
- [Cobra](https://github.com/spf13/cobra) - CLI framework

## 📊 Status

**Current Version**: v0.1.0 (MVP)

**Development Status**: ✅ Day 9 Complete - Ready for Release

- ✅ Project initialization
- ✅ CLI framework (Cobra + Viper + Zap)
- ✅ API Server client (client-go)
- ✅ Kubelet client (Summary API)
- ✅ Cache layer + background refresh
- ✅ **Overview view**: Cluster summary, node/pod stats, recent events
- ✅ **Node view**: Node list, resource usage, detail view
- ✅ **Pod view**: Pod list, namespace filtering, detail view
- ✅ **Detail views**: Node details, Pod details, container info
- ✅ **Fast navigation**: Number keys 1/2/3 for quick view switching
- ✅ **Interactive filtering**: Namespace filter with live preview
- ✅ **vim-style navigation**: j/k for up/down, Enter for details, Esc to go back

**Next Steps**: Documentation and integration testing (Day 10)

---

Made with ❤️ for Kubernetes operators
