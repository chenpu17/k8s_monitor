# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

k8s-monitor is a lightweight, read-only CLI monitoring console for Kubernetes clusters built with Go. It provides a terminal-based UI (TUI) using Bubble Tea framework for real-time cluster monitoring.

**Current Version**: v0.1.1
**Language**: Go 1.24+
**Architecture**: Terminal User Interface (TUI) with Bubble Tea, client-go for Kubernetes API

## Common Commands

### Build and Run
```bash
# Build the binary
make build

# Run locally (builds and runs)
make run

# Run in development mode (no build)
make dev

# Install to $GOPATH/bin
make install
```

### Testing
```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run tests with coverage report
make test-coverage
```

### Code Quality
```bash
# Format code
make fmt

# Run go vet
make vet

# Run linters (requires golangci-lint)
make lint

# Run all checks (format, vet, test)
make check
```

### Dependencies
```bash
# Download and tidy dependencies
make deps

# Verify dependencies
make mod-verify
```

### Multi-platform Build
```bash
# Build for multiple platforms (linux/darwin, amd64/arm64)
make build-all
```

## Architecture

### High-Level Structure

The codebase follows a layered architecture:

```
CLI Layer (Bubble Tea TUI)
    ↓
Application Core (Data Manager, Config)
    ↓
Data Source Layer (API Server, Kubelet, Aggregated)
    ↓
Kubernetes Cluster
```

### Key Components

1. **UI Layer** (`internal/ui/`)
   - `model.go`: Main Bubble Tea model, handles all UI state and events
   - View files (`overview.go`, `nodes.go`, `pods.go`, `workloads.go`, etc.): Render different views
   - Detail files (`*_detail.go`): Render detailed views for specific resources
   - `logs.go`: Pod log viewer with search functionality
   - `action_menu.go`: Action menu system for resource operations
   - `export.go`: Export functionality for data

2. **Application Core** (`internal/app/`)
   - `app.go`: Main application orchestrator
   - `config.go`: Configuration loading and management

3. **Data Source Layer** (`internal/datasource/`)
   - `apiserver.go`: Kubernetes API Server client (using client-go)
   - `kubelet.go`: Kubelet Summary API client for metrics
   - `aggregated.go`: Aggregates data from multiple sources
   - `interface.go`: Data source interfaces and helper functions

4. **Cache Layer** (`internal/cache/`)
   - `cache.go`: TTL-based cache implementation
   - `refresher.go`: Background data refresh mechanism

5. **Internationalization** (`internal/i18n/`)
   - `i18n.go`: Supports English and Chinese locales
   - Translation files in subdirectories

### Data Flow

1. **Background Refresh**: `cache.Refresher` periodically fetches data from `AggregatedDataSource`
2. **AggregatedDataSource** combines:
   - API Server data (pods, nodes, events, workloads)
   - Kubelet metrics (CPU, memory, network per pod/node)
3. **Data cached** in `TTLCache` with configurable TTL
4. **UI** requests data via `DataProvider` interface, gets cached or fresh data
5. **Bubble Tea** handles UI updates via messages (`clusterDataMsg`, `logsMsg`, etc.)

## Important Design Patterns

### Bubble Tea Architecture (Elm-inspired)

The UI follows the Model-Update-View pattern:
- **Model**: `ui.Model` struct holds all UI state
- **Update**: `Model.Update()` handles messages and returns updated model + commands
- **View**: `Model.View()` renders the current state to string

### Message Passing

Communication happens via typed messages:
- `clusterDataMsg`: Cluster data refresh complete
- `logsMsg`: Pod logs fetched
- `refreshTickMsg`: Auto-refresh timer tick
- `logsRefreshTickMsg`: Log auto-refresh tick
- `exportSuccessMsg`/`exportErrorMsg`: Export operation results
- `commandOutputMsg`: Display command output

### View Types

The UI supports multiple views (see `ViewType` enum in `internal/ui/model.go`):
- `ViewOverview`: Cluster health summary
- `ViewNodes`: Node list with metrics
- `ViewPods`: Pod list with filtering
- `ViewWorkloads`: Jobs, Deployments, StatefulSets, DaemonSets, CronJobs
- `ViewNetwork`: Services and network info
- `ViewStorage`: PersistentVolumes and PersistentVolumeClaims
- `ViewEvents`: Cluster events
- `ViewAlerts`: Health alerts
- Detail views for each resource type

### Keyboard Navigation

- **Global**: `q`/`Ctrl+C` quit, `r` refresh, `1-8` switch views, `tab` cycle views
- **List views**: `↑`/`k` up, `↓`/`j` down, `PgUp`/`PgDn` page, `enter` detail, `s` sort, `/` search
- **Detail views**: `↑`/`↓` scroll, `esc` back, `l` logs (pods), `a` actions
- **Search mode**: Type to filter, `backspace` delete, `esc` cancel
- **Logs mode**: `↑`/`↓` scroll, `/` search, `esc` back

## Critical Implementation Details

### Data Provider Interface

The UI depends on `DataProvider` interface (defined in `internal/ui/model.go`):
```go
type DataProvider interface {
    GetClusterData() (*model.ClusterData, error)
    ForceRefresh() error
}
```

The `app.App` implements this interface and provides access to cached/fresh data.

### Kubelet Metrics Access

The kubelet client uses the Summary API via API Server proxy:
- Path: `/api/v1/nodes/{node}/proxy/stats/summary`
- Requires proper RBAC permissions
- Falls back gracefully if kubelet metrics unavailable
- Can skip TLS verification with `--insecure-kubelet` flag (test environments only)

### Concurrency Control

- Kubelet queries are concurrent but limited by `MaxConcurrent` (default: 10)
- Uses semaphore pattern in `aggregated.go` to prevent overwhelming the cluster
- Background refresh runs in separate goroutine via `cache.Refresher`

### Internationalization

- Uses go-i18n library with message bundles
- Locale determined by `--locale` flag or config file
- All user-facing strings should use `m.T()`, `m.TP()`, or `m.TF()` methods
- Translation keys defined in `internal/i18n/` subdirectories

### Logging

- Uses uber/zap for structured logging
- Logs to file (default: `/tmp/k8s-monitor.log`) with rotation
- NEVER logs to stdout/stderr (conflicts with TUI)
- Log level configurable via config or `--verbose` flag

### State Management

The UI model maintains extensive state:
- Current view and detail mode
- Scroll positions and selected indices
- Filter and search text
- Logs viewer state (auto-refresh, auto-scroll)
- Action menu state
- Metric history for trend calculation (last 10 snapshots)
- Cached sorted data to maintain selection consistency

### Trend Calculation

Network rates use 20-second time-based sliding window:
- Collects multiple rate measurements within window
- Averages valid rates for stable metrics
- Handles counter resets gracefully
- Uses kubelet-provided timestamps for accuracy

## Testing Practices

- Unit tests for cache, datasource, and conversion functions
- Test files follow `*_test.go` naming convention
- Use table-driven tests where appropriate
- Mock interfaces for isolated component testing

## Configuration

Configuration sources (in order of precedence):
1. Command-line flags
2. Config file (YAML): `./config/config.yaml`, `~/.k8s-monitor/config.yaml`, `/etc/k8s-monitor/config.yaml`
3. Default values

Key configuration options:
- `kubeconfig`: Path to kubeconfig file
- `context`: Kubernetes context to use
- `namespace`: Namespace filter (empty = all)
- `refresh_interval`: Auto-refresh interval (default: 2s)
- `cache_ttl`: Cache TTL (default: 10s)
- `max_concurrent`: Max concurrent kubelet queries (default: 10)
- `log_tail_lines`: Number of log lines to fetch (default: 200)
- `locale`: UI language (`en` or `zh`)
- `insecure_kubelet`: Skip kubelet TLS verification (test only)

## Common Pitfalls

1. **TUI Output Pollution**: Never use `fmt.Print*` or log to stdout/stderr - use zap logger to file
2. **Client-go Verbosity**: klog is configured to suppress output in `cmd/k8s-monitor/main.go`
3. **State Sync**: When adding new views, update `getMaxIndex()`, `renderViewTabs()`, and key handlers
4. **Index Bounds**: Always clamp `selectedIndex` when data changes or view switches
5. **Metric Snapshots**: Only record when `LastRefreshTime` changes to avoid duplicates
6. **Detail Mode**: Set `detailMode = true` when entering detail views for proper keyboard handling
7. **Search Mode**: Block shortcut keys in search mode except navigation keys
8. **Logs Auto-scroll**: Pause auto-refresh during search to avoid performance issues

## Development Workflow

When adding a new feature:

1. **Define Model Changes**: Update `internal/ui/model.go` if new state needed
2. **Add Messages**: Define new message types for async operations
3. **Implement Update Logic**: Handle messages in `Model.Update()`
4. **Create View Renderer**: Add view rendering function (e.g., `renderNewView()`)
5. **Wire Key Bindings**: Update key handlers in `Update()` switch cases
6. **Add Translations**: Update i18n message bundles for both locales
7. **Test**: Write unit tests for business logic
8. **Document**: Update README.md if user-facing feature

## Commit Message Convention

Follow [Conventional Commits](https://www.conventionalcommits.org/):
- `feat:` new feature
- `fix:` bug fix
- `refactor:` code refactoring
- `docs:` documentation changes
- `test:` test additions/modifications
- `chore:` maintenance tasks

## Resources

- [Bubble Tea Documentation](https://github.com/charmbracelet/bubbletea)
- [client-go Documentation](https://github.com/kubernetes/client-go)
- [Kubernetes API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)
- [Kubelet Summary API](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/apis/stats/v1alpha1/types.go)
