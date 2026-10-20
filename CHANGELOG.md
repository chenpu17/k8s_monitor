# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2026-10-20

### Fixed
- Overview workload counts (Deployments/StatefulSets/DaemonSets/Jobs/CronJobs) now use real list results instead of non-existent label conventions
- Volcano job NPU resource parsing uses `resource.ParseQuantity`, fixing fractional cores ("1.5" -> 1000m) and fractional memory ("1.5Gi" -> 0)
- NPU-Exporter client: concurrent access is mutex-protected; clusters with multiple NPU nodes skip per-node enrichment instead of fabricating data from a single node
- Kubelet metrics: node and pod metrics now come from a single Summary API request per node (previously two)
- API Server client: explicit REST timeout plus tuned QPS/Burst; background refresh rounds are bounded by a context deadline (`refresh.timeout`, default 30s)
- `--refresh` only overrides the config file when actually passed on the command line
- Detail view scrolling could stop one line short of the bottom; map-based detail sections render in sorted order so content no longer jitters on every refresh
- Export actions snapshot the current view/data before the async command runs
- `<no value>` placeholder in the network stats line; digit and Tab keys no longer switch views while a filter/search is active
- Describe/YAML actions in resource detail views work again (App now proxies the data source)

### Added
- Running `k8s-monitor` without arguments launches the interactive console (the `console` subcommand still works)
- MIT LICENSE file; module path moved to `github.com/chenpu/k8s-monitor`
- New test suites for app/cache/datasource/ui, including a rendering-determinism oracle for detail scrolling

### Changed
- Documentation (README/README_zh/CLAUDE.md/sample config) aligned with the real viper configuration keys

---

## [0.1.0] - 2025-01-14

### Overview

This is the first public release (MVP) of k8s-monitor - a lightweight, read-only CLI monitoring console for Kubernetes clusters. The tool provides a terminal-based interface for operations engineers who need quick insights into cluster health via SSH.

### Added

#### Core Infrastructure
- **CLI Framework**: Built with Cobra, supporting `k8s-monitor console` command
- **Configuration System**: Viper-based config loading from files and environment variables
- **Logging System**: Structured logging with Zap, file rotation with lumberjack
- **Background Refresh**: Automatic data refresh with configurable interval (default: 10s)
- **TTL Cache**: In-memory caching with time-to-live to minimize API calls

#### Data Sources
- **API Server Client**: Fetch nodes, pods, and events via client-go
- **Kubelet Client**: Retrieve real-time metrics from kubelet Summary API
- **Aggregated Data Source**: Combine multiple data sources with parallel fetching
- **Graceful Degradation**: Continue with basic data if kubelet is unavailable

#### User Interface (Bubble Tea)
- **Overview View**:
  - Cluster health summary (nodes online/ready)
  - Node statistics (CPU/Memory/Pods capacity and usage)
  - Pod statistics by status (Running/Pending/Failed/Unknown)
  - Recent events (last 3 events with timestamp and description)

- **Node View**:
  - Node list table with resource usage
  - CPU and Memory usage with percentages
  - Pod count per node
  - Status indicators (Ready/NotReady)
  - Node detail view with basic info and running pods

- **Pod View**:
  - Pod list table with namespace, status, node, restart count
  - Interactive namespace filtering (F key)
  - Filter panel with all available namespaces
  - Clear filter option (C key)
  - Pod detail view with container information

- **Detail Views**:
  - Node details: basic info, resource usage, pods on node
  - Pod details: basic info, container states, images, restart counts
  - Container status visualization with color-coded indicators

#### Keyboard Controls
- **Fast Navigation**: Number keys 1/2/3 for instant view switching
- **vim-style Movement**: j/k keys for up/down navigation
- **View Cycling**: Tab key to cycle through views
- **Detail Drilling**: Enter to view details, Esc to go back
- **Filtering**: F key to open filter panel, C key to clear filter
- **Manual Refresh**: R key to refresh data immediately
- **Quit**: q or Ctrl+C to exit application

#### Visual Design
- **Color-coded Status**: Green (Ready/Running), Yellow (Pending), Red (Failed/NotReady)
- **Responsive Layout**: Adapts to terminal size with lipgloss
- **Selection Highlighting**: Selected row highlighted in list views
- **Scroll Indicators**: Shows current position in long lists (e.g., [1-10 of 50])
- **Context-aware Help**: Footer shows relevant keys for current mode

### Technical Features

#### Performance
- **Concurrent Data Fetching**: Parallel kubelet queries for multiple nodes
- **Efficient Rendering**: Only renders visible portion of lists
- **Background Processing**: Non-blocking data refresh in separate goroutine
- **Cache Hit Optimization**: Serves cached data while refreshing in background

#### Architecture
- **Interface-based Design**: DataProvider interface for clean separation
- **Thread-safe Operations**: sync.RWMutex for concurrent access
- **Graceful Shutdown**: Proper cleanup of goroutines and resources
- **Error Recovery**: Continues operation despite partial failures

#### Code Quality
- **Modular Structure**: Clear separation between UI, app, datasource layers
- **Unit Tests**: 10+ tests covering core logic (all passing)
- **Structured Logging**: JSON logs for production, human-readable for console
- **Configuration Validation**: Validates config on startup

### Configuration

Default configuration options:

```yaml
cluster:
  kubeconfig: ~/.kube/config
  context: ""

refresh:
  interval: 10s
  timeout: 5s

cache:
  ttl: 30s

ui:
  color_mode: auto
  default_view: overview

logging:
  level: info
  file: /tmp/k8s-monitor.log
```

### Command-line Options

```bash
k8s-monitor console [flags]

Flags:
  -k, --kubeconfig string    Path to kubeconfig file
  -c, --context string       Kubernetes context to use
  -n, --namespace string     Default namespace filter
      --refresh duration     Refresh interval (default 10s)
      --no-color            Disable colored output
  -v, --verbose             Enable verbose logging
      --config string       Config file path
```

### Known Limitations

- **Read-only**: No cluster modifications (by design)
- **Metrics Dependency**: Resource usage requires kubelet access or Metrics Server
- **Single Cluster**: Only monitors one cluster at a time (multi-cluster support planned for v0.3)
- **Limited History**: No historical data tracking (planned for v0.3)
- **Basic Filtering**: Only supports namespace filtering (advanced filters planned for v0.2)

### Requirements

- **Go Version**: 1.21+
- **Kubernetes**: 1.26+ (tested on 1.28)
- **Access**: Valid kubeconfig with read permissions
- **Optional**: Kubelet access for real-time metrics (falls back to API Server if unavailable)

### Installation

```bash
# From source
git clone https://github.com/chenpu/k8s-monitor.git
cd k8s-monitor
make build
sudo make install

# Using Go install
go install github.com/chenpu/k8s-monitor/cmd/k8s-monitor@latest
```

### Development Timeline

- **Day 1-2** (2025-01-06 ~ 2025-01-07): Project initialization, CLI/config/logging
- **Day 3-4** (2025-01-08 ~ 2025-01-09): API Server + kubelet clients
- **Day 5** (2025-01-10): Cache layer + background refresh
- **Day 6** (2025-01-11): Bubble Tea framework + Overview view
- **Day 7** (2025-01-12): Node + Pod list views with scrolling
- **Day 8** (2025-01-13): Detail views (Node + Pod)
- **Day 9** (2025-01-14): Fast navigation + filtering
- **Day 10** (2025-01-15): Testing + documentation

### Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Excellent TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes Go client
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Zap](https://github.com/uber-go/zap) - High-performance logging

### Contributors

- Initial development and MVP release

---

## [Unreleased]

### Added
- Kubelet access preflight using `SelfSubjectAccessReview` to detect missing `nodes/proxy` RBAC, cache the result, and avoid repeatedly querying metrics when access is denied.
- Network panels now surface contextual hints (RBAC vs TLS) instead of always suggesting `--insecure-kubelet`, making it clear when credentials need additional permissions.

### Planned for v0.2 (2025-01-20 ~ 2025-02-09)

- Pod logs viewing from the TUI
- Resource editing via kubectl edit integration
- Advanced filtering (labels, status, custom queries)
- Search functionality across all resources
- Performance metrics with CPU/Memory usage trends

### Planned for v0.3+ (2025-02-10 ~ 2025-03-09)

- Multi-cluster support with context switching
- Historical data tracking with trends
- Custom alert rules and notifications
- Plugin system for extensibility
- Network and service monitoring
- Snapshot and diff capabilities

---

[0.1.0]: https://github.com/chenpu/k8s-monitor/releases/tag/v0.1.0

[0.1.1]: https://github.com/chenpu/k8s-monitor/releases/tag/v0.1.1
