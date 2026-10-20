# k8s-monitor

[English](README.md) | [中文](README_zh.md)

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> A lightweight, read-only terminal UI monitoring tool for Kubernetes clusters

## 📋 Overview

k8s-monitor is a terminal-based monitoring tool for Kubernetes clusters, designed for operations engineers who need quick insights into cluster health via SSH. It provides real-time monitoring with an intuitive keyboard-driven interface.

![k8s-monitor Demo](images/k8s-monitor.gif)

### Key Features

- **🎯 Comprehensive Views**: 8 core views plus Volcano Queues and SuperPod Topology when the cluster provides them
- **📊 Resource Monitoring**: Real-time CPU/Memory/Network metrics with visual progress bars
- **🚀 NPU Monitoring**: Huawei Ascend NPU support with detailed chip metrics
- **📈 Trend Analysis**: Historical metrics tracking with trend indicators
- **🔍 Smart Diagnostics**: Automatic detection of CrashLoops, failed pods, node pressure
- **📝 Log Viewer**: View and search pod logs in real-time
- **🛡️ Read-only**: Safe to use in production - no cluster modifications
- **⚡ Fast & Lightweight**: Single binary, minimal dependencies
- **🌍 i18n Support**: English and Chinese interface

## 🚀 Quick Start

### Installation

#### From Source

```bash
git clone https://github.com/chenpu/k8s-monitor.git
cd k8s-monitor
make build
sudo make install
```

#### Using Go Install

```bash
go install github.com/chenpu/k8s-monitor/cmd/k8s-monitor@latest
```

### Usage

```bash
# Start the interactive console (running without arguments also works)
k8s-monitor [console]

# Specify kubeconfig
k8s-monitor console --kubeconfig ~/.kube/config

# Use specific context
k8s-monitor console --context my-cluster

# Monitor specific namespace
k8s-monitor console --namespace production

# Set language (en/zh)
k8s-monitor console --locale zh

# See all options
k8s-monitor --help
```

## ✨ Features

### Core Functionality (v0.1.1)

#### 📊 Cluster Overview
- Cluster-wide resource summary (CPU, Memory, Pods)
- Color-coded progress bars for capacity, allocatable, requests, and usage
- Automatic utilization percentage calculation
- Recent events and alerts summary

#### 🖥️ Node Monitoring
- Real-time node metrics (CPU, Memory, Network)
- Pod distribution per node
- Node conditions and taints
- Sorting by name, CPU, memory, or pod count
- Trend indicators for resource usage

#### 🚀 NPU Monitoring (Huawei Ascend)
- NPU capacity and allocation tracking
- Per-chip detailed metrics:
  - AI Core utilization
  - Vector utilization
  - HBM memory usage
  - Temperature and power consumption
  - Voltage and frequency
  - Link status
  - RoCE network statistics
  - ECC error tracking
- Topology information (SuperPod, HyperNode)
- Integration with NPU-Exporter for runtime metrics

#### 📦 Pod Management
- Pod list with status, restarts, resource usage
- Filter by namespace, status, or search by name
- Container-level details
- Resource requests and limits tracking
- Network metrics per pod

#### ⚙️ Workload Management
- Jobs, Deployments, StatefulSets, DaemonSets, CronJobs
- Status tracking and replica counts
- Detailed resource specifications
- Navigation to related pods

#### 🌐 Network View
- Services with type, cluster IP, and ports
- Endpoint tracking
- Network traffic monitoring (RX/TX rates)

#### 💾 Storage View
- PersistentVolumes and PersistentVolumeClaims
- Capacity, status, and access modes
- Storage class information

#### 📋 Events & Alerts
- Kubernetes events with filtering (Warning/Normal)
- System-generated health alerts
- Event search and sorting

#### 📝 Pod Logs
- Real-time log viewing with auto-refresh
- Log search with highlighting
- Auto-scroll to latest logs
- Support for multi-container pods

#### 🎬 Action Menu
- Quick actions for pods and nodes
- Execute kubectl commands
- Copy resource information to clipboard

### Advanced Features

- **Vim-style Navigation**: `j/k` for up/down, `Enter` for details, `Esc` to go back
- **Fast View Switching**: Number keys `1-8` for the core views, `9` for Queues (requires Volcano), `0` for Topology (requires SuperPod)
- **Flexible Filtering**: Filter by namespace, status, labels
- **Full-text Search**: Search resources by name
- **Data Export**: Export view data to CSV/JSON
- **Auto-refresh**: Configurable background refresh interval
- **Metric History**: 10-snapshot sliding window for trend calculation
- **Network Rate Calculation**: 20-second time-based sliding window for stable metrics

## 🎮 Keyboard Shortcuts

### Global Keys
| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit application |
| `r` | Manual refresh |
| `1-8` | Switch to a core view (1=Overview, 2=Nodes, 3=Pods, 4=Workloads, 5=Network, 6=Storage, 7=Events, 8=Alerts) |
| `9` | Switch to Queues view (only when Volcano Queues are detected) |
| `0` | Switch to Topology view (only when SuperPod topology is detected) |
| `Tab` | Cycle through the currently available views |

### List View Keys
| Key | Action |
|-----|--------|
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `PgUp` / `Ctrl+U` | Page up |
| `PgDn` / `Ctrl+D` | Page down |
| `Enter` | View details |
| `f` | Open filter panel |
| `c` | Clear all filters |
| `s` | Cycle sort order |
| `/` | Search by name |
| `e` | Export current view data |

### Detail View Keys
| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll content |
| `PgUp` / `PgDn` | Page up/down |
| `Esc` / `Backspace` | Back to list view |
| `l` | View logs (Pod detail only) |
| `a` | Open action menu (Pod/Node detail) |

### Logs View Keys
| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll logs |
| `PgUp` / `PgDn` | Page up/down |
| `/` | Search in logs |
| `Esc` | Exit logs view |

### Search/Filter Mode Keys
| Key | Action |
|-----|--------|
| `text` | Type to filter |
| `Backspace` | Delete character |
| `Esc` | Cancel |
| `Enter` | Apply filter |

## ⚙️ Configuration

Configuration file locations (searched in order):
1. `./config/config.yaml`
2. `$HOME/.k8s-monitor/config.yaml`
3. `/etc/k8s-monitor/config.yaml`

The file must be named `config.yaml`; `config/default.yaml` in this repository is only a sample. Copy it into place to use it:

```bash
cp config/default.yaml config/config.yaml
```

Example configuration (all keys are optional; values shown are the built-in defaults):

```yaml
cluster:
  # Use an absolute path - "~" is not expanded. Leave empty to fall back to
  # $HOME/.kube/config (or in-cluster config when running inside a pod).
  kubeconfig: ""
  context: ""                  # Kubernetes context (empty = current context)
  namespace: ""                # Namespace to monitor (empty = all namespaces)

refresh:
  interval: 2s                 # Auto-refresh interval
  timeout: 30s                 # Timeout for API requests to the cluster
  max_concurrent: 10           # Max concurrent kubelet queries

cache:
  ttl: 60s                     # Cache time-to-live

ui:
  locale: en                   # Interface language (en/zh)
  log_tail_lines: 200          # Number of log lines to fetch

kubelet:
  # Test environments only - skip kubelet TLS verification
  insecure: false

# NPU monitoring (Huawei Ascend)
npu_exporter:
  endpoint: ""                 # Custom NPU-Exporter endpoint (default: auto-detect via K8s API proxy)

logging:
  level: info                  # Log level (debug/info/warn/error)
  file: /tmp/k8s-monitor.log
```

### NPU Monitoring Setup

To enable NPU monitoring for Huawei Ascend accelerators:

1. **Prerequisites**: NPU-Exporter must be deployed in your cluster
   ```bash
   # Check if NPU-Exporter is available
   kubectl get svc -n kube-system npu-exporter
   ```

2. **Automatic Detection**: k8s-monitor automatically connects to NPU-Exporter via Kubernetes API proxy

3. **Custom Endpoint** (optional): If NPU-Exporter is deployed in a different location
   ```bash
   k8s-monitor console --npu-exporter http://custom-npu-exporter:8082
   ```

**NPU-Exporter Image**: `swr.cn-north-12.myhuaweicloud.com/hwofficial/npu-exporter:2.3.2`

## 🏗️ Architecture

```
┌─────────────────────────────────────┐
│    CLI Interface (Bubble Tea)       │
│         User Interaction            │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│      Application Core               │
│  ┌──────────┐    ┌──────────┐      │
│  │   View   │    │   Data   │      │
│  │  Manager │    │ Manager  │      │
│  └──────────┘    └──────────┘      │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│      Data Source Layer              │
│  ┌──────────┐    ┌──────────┐      │
│  │   API    │    │ Kubelet  │      │
│  │  Server  │    │  Client  │      │
│  └──────────┘    └──────────┘      │
│        │              │             │
│  ┌──────────┐    ┌──────────┐      │
│  │  NPU     │    │  Volcano │      │
│  │ Exporter │    │  Client  │      │
│  └──────────┘    └──────────┘      │
│         ↓              ↓            │
│  ┌─────────────────────────┐       │
│  │    Cache & Refresh      │       │
│  └─────────────────────────┘       │
└─────────────────────────────────────┘
                 ↓
┌─────────────────────────────────────┐
│      Kubernetes Cluster             │
│  (API Server, Kubelet, NPU-Exporter)│
└─────────────────────────────────────┘
```

**Key Components:**
- **UI Layer**: Terminal rendering with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm architecture)
- **Application Core**: Business logic, view management, state handling
- **Data Sources**:
  - API Server via [client-go](https://github.com/kubernetes/client-go)
  - Kubelet Summary API for real-time metrics
  - NPU-Exporter for Huawei Ascend NPU metrics (via K8s API proxy)
  - Volcano client for HyperNode topology (optional)
- **Cache Layer**: TTL-based caching with background refresh

## 📖 Documentation

- [CLAUDE.md](CLAUDE.md) - Developer guide for working with this codebase
- [Product Plan](docs/product_plan.md) - Product vision and roadmap
- [Technical Design](docs/technical_design.md) - Architecture and implementation details
- [Development Plan](docs/development_plan.md) - Development progress tracking
- [Resource Monitoring](docs/RESOURCE_MONITORING.md) - Detailed resource monitoring guide

## 🛠️ Development

### Prerequisites

- Go 1.24+
- Access to a Kubernetes cluster (for testing)
- kubectl configured with valid kubeconfig

### Build Commands

```bash
# Install dependencies
make deps

# Build binary
make build

# Run tests with coverage
make test

# Run linters
make lint

# Format code
make fmt

# Run all checks (format, vet, test)
make check

# Build for multiple platforms
make build-all

# Run locally
make run

# Clean build artifacts
make clean
```

### Project Structure

```
k8s-monitor/
├── cmd/k8s-monitor/        # Main entry point
├── internal/               # Private application code
│   ├── app/                # Application core (config, lifecycle)
│   ├── ui/                 # UI layer (Bubble Tea models and views)
│   ├── datasource/         # Data source clients (API Server, Kubelet)
│   ├── cache/              # Cache and refresh logic
│   ├── model/              # Data models
│   ├── i18n/               # Internationalization
│   └── diagnostic/         # Diagnostic utilities
├── config/                 # Configuration files
├── docs/                   # Documentation
├── images/                 # Screenshots and demos
└── scripts/                # Build and utility scripts
```

### Code Statistics

| Component | Lines of Code | Description |
|-----------|---------------|-------------|
| **Total** | **~22,300** | Pure Go implementation |
| `internal/ui` | ~15,200 | TUI layer (Bubble Tea models, views, rendering) |
| `internal/datasource` | ~4,600 | Data sources (API Server, Kubelet, NPU-Exporter, Volcano) |
| `internal/model` | ~700 | Data models and types |
| `internal/cache` | ~550 | TTL cache and background refresh |
| `internal/app` | ~540 | Application core and configuration |
| `internal/i18n` | ~120 | Internationalization support |
| `cmd/` | ~190 | CLI entry point |

- **52 Go source files** across the codebase
- **Zero external runtime dependencies** - single static binary
- **Two languages supported** - English and Chinese

### Running Tests

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Generate coverage report
make test-coverage
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

### How to Contribute

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Commit Convention

Please follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:
- `feat:` new feature
- `fix:` bug fix
- `refactor:` code refactoring
- `docs:` documentation changes
- `test:` test additions/modifications
- `chore:` maintenance tasks

## 🗺️ Roadmap

### v0.2 (In Progress)
- [ ] Multi-cluster support
- [ ] Custom alert rules
- [ ] Historical metrics graphs
- [ ] Resource editing via kubectl integration
- [ ] Advanced label filtering

### v0.3+ (Future)
- [ ] Plugin system for custom views
- [ ] Metrics aggregation over time
- [ ] Export to monitoring systems
- [ ] Configuration profiles

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Excellent TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes Go client
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management

## 📧 Contact

For questions, suggestions, or issues, please [open an issue](https://github.com/chenpu/k8s-monitor/issues) on GitHub.

---

Made with ❤️ for Kubernetes operators
