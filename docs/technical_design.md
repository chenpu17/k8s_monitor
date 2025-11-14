# k8s 监控控制台 - 技术设计方案

## 文档说明
本文档描述 k8s-monitor 的技术架构、模块设计、关键技术实现方案。

**维护规则**：
- 本文档相对稳定，重大架构变更时更新
- 设计变更需团队评审
- 版本号跟随产品版本

---

## 目录
- [1. 架构设计](#1-架构设计)
- [2. 技术选型](#2-技术选型)
- [3. 模块设计](#3-模块设计)
- [4. 数据流设计](#4-数据流设计)
- [5. 关键技术实现](#5-关键技术实现)

---

# 1. 架构设计

## 1.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Interface                        │
│  (终端渲染、键盘交互、状态栏、面板管理)                      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Application Core                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ View Manager │  │ Data Manager │  │ Config Mgr   │      │
│  │ (视图切换)   │  │ (数据聚合)   │  │ (配置管理)   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Filter Mgr   │  │ Event Bus    │  │ Error Handler│      │
│  │ (过滤排序)   │  │ (事件通信)   │  │ (错误处理)   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Data Source Layer                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ API Server   │  │ kubelet API  │  │ Metrics Srv  │      │
│  │  Client      │  │  Client      │  │  Client      │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Cache Layer  │  │ Retry Logic  │  │ Fallback Mgr │      │
│  │ (数据缓存)   │  │ (重试机制)   │  │ (降级处理)   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│  API Server  →  kubelet (nodes)  →  Metrics Server          │
└─────────────────────────────────────────────────────────────┘
```

## 1.2 分层职责

| 层级 | 职责 | 关键组件 |
|------|------|----------|
| **CLI 界面层** | 终端渲染、用户交互、快捷键处理 | TUI 框架、Panel 组件、Input Handler |
| **应用核心层** | 业务逻辑、视图管理、数据聚合 | View Manager、Data Manager、Filter Manager |
| **数据源层** | 数据获取、缓存、降级处理 | K8s Client、Cache、Fallback Manager |
| **基础设施层** | 日志、配置、错误处理 | Logger、Config Loader、Error Handler |

## 1.3 设计原则

1. **模块解耦**：各层通过接口通信，便于测试和替换
2. **降级优先**：所有数据源都有降级方案，保证可用性
3. **并发控制**：限制对 K8s API 的并发请求数，避免压力过大
4. **渐进式渲染**：允许部分视图先渲染，避免整体阻塞
5. **错误友好**：提供分级错误提示 + 诊断建议 + 快速操作

---

# 2. 技术选型

## 2.1 编程语言与框架

**推荐方案：Go + Bubble Tea**

| 技术栈 | 选型 | 理由 |
|--------|------|------|
| **编程语言** | Go 1.21+ | - 静态编译，单二进制部署<br>- 优秀的并发支持（goroutine）<br>- 成熟的 K8s 生态（client-go）<br>- 跨平台支持 |
| **TUI 框架** | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | - 现代化 Elm 架构（Model-Update-View）<br>- 活跃维护，社区强大<br>- 配套丰富组件（Lip Gloss、Bubbles）<br>- 优秀的键盘/鼠标事件处理 |
| **K8s 客户端** | [client-go](https://github.com/kubernetes/client-go) | - 官方 SDK，功能完整<br>- 支持 kubeconfig、RBAC<br>- 内置重试、限流机制 |
| **配置管理** | [Viper](https://github.com/spf13/viper) | - 支持多格式（YAML/JSON/TOML）<br>- 环境变量、命令行参数集成 |
| **日志** | [Zap](https://github.com/uber-go/zap) | - 高性能结构化日志<br>- 分级输出（文件 + stderr） |
| **CLI 框架** | [Cobra](https://github.com/spf13/cobra) | - 标准化命令行参数解析<br>- 自动生成 help 文档 |

**备选方案**：Rust + Ratatui（更高性能，但开发周期长）

## 2.2 依赖库清单

```go
// go.mod
module github.com/yourusername/k8s-monitor

go 1.21

require (
    github.com/charmbracelet/bubbletea v0.25.0
    github.com/charmbracelet/lipgloss v0.9.1
    github.com/charmbracelet/bubbles v0.18.0
    k8s.io/client-go v0.29.0
    k8s.io/api v0.29.0
    k8s.io/apimachinery v0.29.0
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    go.uber.org/zap v1.26.0
    github.com/prometheus/client_golang v1.18.0  // Metrics 采集（可选）
)
```

## 2.3 技术栈对比

| 方案 | 优点 | 缺点 | 适用场景 |
|------|------|------|----------|
| **Go + Bubble Tea** | 开发快、生态好、跨平台 | 性能略低于 Rust | 快速迭代、团队熟悉 Go |
| **Rust + Ratatui** | 性能极高、内存安全 | 学习曲线陡、开发慢 | 对性能极致要求 |
| **Python + Rich/Textual** | 原型快、库丰富 | 启动慢、打包复杂 | 原型验证 |

**最终选择**：Go + Bubble Tea（平衡开发效率和性能）

---

# 3. 模块设计

## 3.1 目录结构

```
k8s-monitor/
├── cmd/
│   └── k8s-monitor/
│       └── main.go                 # 程序入口
├── internal/
│   ├── app/
│   │   ├── app.go                  # 应用主逻辑
│   │   └── config.go               # 配置加载
│   ├── ui/
│   │   ├── view_manager.go         # 视图管理器
│   │   ├── views/
│   │   │   ├── overview.go         # 概览视图
│   │   │   ├── node.go             # 节点视图
│   │   │   ├── workload.go         # 工作负载视图
│   │   │   └── diagnostic.go       # 诊断视图
│   │   ├── components/
│   │   │   ├── statusbar.go        # 状态栏组件
│   │   │   ├── table.go            # 表格组件
│   │   │   ├── filter.go           # 过滤面板
│   │   │   └── gauge.go            # 仪表盘组件
│   │   └── styles.go               # 样式定义
│   ├── datasource/
│   │   ├── client.go               # 数据源客户端接口
│   │   ├── apiserver.go            # API Server 客户端
│   │   ├── kubelet.go              # kubelet 客户端
│   │   ├── metrics.go              # Metrics Server 客户端
│   │   ├── cache.go                # 缓存管理
│   │   └── fallback.go             # 降级处理
│   ├── model/
│   │   ├── cluster.go              # 集群数据模型
│   │   ├── node.go                 # 节点数据模型
│   │   ├── pod.go                  # Pod 数据模型
│   │   └── event.go                # 事件数据模型
│   ├── filter/
│   │   ├── filter.go               # 过滤器接口
│   │   ├── namespace.go            # 命名空间过滤
│   │   └── label.go                # 标签过滤
│   ├── diagnostic/
│   │   ├── checker.go              # 诊断检查器
│   │   ├── rules.go                # 诊断规则
│   │   └── report.go               # 诊断报告生成
│   └── utils/
│       ├── logger.go               # 日志工具
│       ├── formatter.go            # 格式化工具
│       └── errors.go               # 错误处理
├── pkg/
│   └── snapshot/
│       ├── snapshot.go             # 快照导出
│       └── diff.go                 # 快照对比
├── config/
│   └── default.yaml                # 默认配置
├── docs/
│   ├── product_plan.md             # 产品方案
│   ├── technical_design.md         # 本文档
│   └── development_plan.md         # 开发计划与进展
├── scripts/
│   ├── build.sh                    # 构建脚本
│   └── install.sh                  # 安装脚本
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 3.2 核心模块设计

### 3.2.1 View Manager（视图管理器）

```go
// internal/ui/view_manager.go
type ViewManager struct {
    currentView View
    views       map[string]View
    eventBus    *EventBus
    dataManager *DataManager
}

type View interface {
    Render(data interface{}) string
    HandleKey(key string) tea.Cmd
    OnEnter()
    OnExit()
}

// 视图切换
func (vm *ViewManager) SwitchView(name string) error
```

### 3.2.2 Data Manager（数据管理器）

```go
// internal/app/data_manager.go
type DataManager struct {
    clients      []DataSourceClient
    cache        *Cache
    fallbackMgr  *FallbackManager
    refreshInterval time.Duration
}

// 数据获取流程（带降级）
func (dm *DataManager) FetchClusterData() (*ClusterData, error) {
    // 1. 尝试从缓存获取
    // 2. 并发请求多个数据源
    // 3. 处理失败降级
    // 4. 更新缓存
}

// 并发获取节点数据（限制并发数）
func (dm *DataManager) FetchNodesDataConcurrent(nodes []string, maxConcurrent int) []NodeData
```

### 3.2.3 Data Source Client（数据源客户端）

```go
// internal/datasource/client.go
type DataSourceClient interface {
    Name() string
    Priority() int
    IsAvailable() bool
    FetchNodes() ([]NodeData, error)
    FetchPods(namespace string) ([]PodData, error)
    FetchEvents(namespace string) ([]EventData, error)
    FetchMetrics() (*MetricsData, error)
}

// API Server 客户端
type APIServerClient struct {
    clientset *kubernetes.Clientset
    config    *rest.Config
}

// kubelet 客户端
type KubeletClient struct {
    nodeIP     string
    port       int
    tlsConfig  *tls.Config
    useProxy   bool  // 是否通过 API Server 代理
}
```

### 3.2.4 Cache Layer（缓存层）

```go
// internal/datasource/cache.go
type Cache struct {
    data      sync.Map  // key: cacheKey, value: CacheEntry
    ttl       time.Duration
}

type CacheEntry struct {
    Data       interface{}
    Timestamp  time.Time
    Source     string  // 数据来源（apiserver/kubelet/metrics）
}

// 带 TTL 的缓存获取
func (c *Cache) Get(key string) (interface{}, bool)
func (c *Cache) Set(key string, data interface{}, source string)
```

### 3.2.5 Fallback Manager（降级管理器）

```go
// internal/datasource/fallback.go
type FallbackManager struct {
    strategies map[string]FallbackStrategy
}

type FallbackStrategy interface {
    // 当主数据源失败时，选择备用方案
    SelectFallback(primarySource string, err error) (fallbackSource string, fallbackFunc func() (interface{}, error))
    // 生成降级提示信息
    GenerateNotice(primarySource string, fallbackSource string) string
}

// 示例：Metrics Server 不可用 → kubelet Summary API
func (fm *FallbackManager) HandleMetricsFailure(err error) (*MetricsData, Notice)
```

---

# 4. 数据流设计

## 4.1 启动流程

```
1. main.go
   ↓
2. 加载配置（kubeconfig、刷新间隔、过滤规则）
   ↓
3. 初始化 K8s 客户端（API Server、kubelet、Metrics Server）
   ↓
4. 健康检查（检测数据源可用性）
   ↓
5. 初始化缓存、日志、错误处理器
   ↓
6. 启动 Bubble Tea 应用
   ↓
7. 进入默认视图（概览页）
```

## 4.2 数据刷新流程

```
用户按 [R] 或定时器触发
   ↓
Data Manager 接收刷新请求
   ↓
┌─────────────────────────────────────┐
│  并发请求多个数据源（限流控制）     │
│  ┌────────────┐ ┌────────────┐      │
│  │ API Server │ │  kubelet   │      │
│  │   Client   │ │   Client   │      │
│  └────────────┘ └────────────┘      │
│  ┌────────────┐                      │
│  │  Metrics   │                      │
│  │   Server   │                      │
│  └────────────┘                      │
└─────────────────────────────────────┘
   ↓
处理响应（成功/失败/超时）
   ↓
失败？
  ├─ Yes → Fallback Manager 选择降级方案
  │         ├─ 使用缓存数据
  │         ├─ 切换备用数据源
  │         └─ 生成降级提示
  └─ No  → 更新缓存
   ↓
聚合数据（多数据源合并）
   ↓
通过 Event Bus 通知 View 更新
   ↓
View 重新渲染
```

## 4.3 视图切换流程

```
用户按快捷键（如 [1] [2] [D]）
   ↓
View Manager 接收切换请求
   ↓
当前视图 OnExit()（清理资源）
   ↓
目标视图 OnEnter()（初始化）
   ↓
Data Manager 检查是否需要加载新数据
   ↓
View 渲染
   ↓
更新状态栏（显示当前视图、快捷键提示）
```

---

# 5. 关键技术实现

## 5.1 kubelet Summary API 访问

**挑战**：kubelet 端口通常需要证书认证，且受网络策略限制。

**实现方案**：
```go
// internal/datasource/kubelet.go

// 方案 1：直接访问节点 10250 端口
func (k *KubeletClient) FetchSummaryDirect(nodeIP string) (*Summary, error) {
    url := fmt.Sprintf("https://%s:10250/stats/summary", nodeIP)
    req, _ := http.NewRequest("GET", url, nil)
    // 使用 kubeconfig 中的证书
    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: k.tlsConfig,
        },
        Timeout: 5 * time.Second,
    }
    resp, err := client.Do(req)
    // 处理响应...
}

// 方案 2：通过 API Server 代理访问
func (k *KubeletClient) FetchSummaryViaProxy(nodeName string) (*Summary, error) {
    // GET /api/v1/nodes/<node-name>/proxy/stats/summary
    proxyURL := k.clientset.CoreV1().RESTClient().Get().
        Resource("nodes").
        Name(nodeName).
        SubResource("proxy").
        Suffix("stats/summary").
        URL()

    resp, err := k.clientset.CoreV1().RESTClient().Get().
        RequestURI(proxyURL.Path).
        DoRaw(context.TODO())
    // 处理响应...
}

// 自动选择访问方式
func (k *KubeletClient) FetchSummary(node string) (*Summary, error) {
    // 先尝试直接访问，失败后切换代理
    summary, err := k.FetchSummaryDirect(node)
    if err != nil {
        k.useProxy = true
        return k.FetchSummaryViaProxy(node)
    }
    return summary, nil
}
```

## 5.2 并发控制与限流

**挑战**：避免对 K8s API Server 造成压力。

**实现方案**：
```go
// internal/datasource/concurrency.go

// 使用 worker pool 限制并发数
func (dm *DataManager) FetchNodesDataConcurrent(nodes []string, maxConcurrent int) []NodeData {
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, maxConcurrent)  // 信号量控制并发
    results := make([]NodeData, len(nodes))

    for i, node := range nodes {
        wg.Add(1)
        go func(index int, nodeName string) {
            defer wg.Done()
            semaphore <- struct{}{}  // 获取信号量
            defer func() { <-semaphore }()  // 释放信号量

            data, err := dm.kubeletClient.FetchSummary(nodeName)
            if err != nil {
                // 记录错误，使用缓存数据
                results[index] = dm.cache.Get(nodeName)
                return
            }
            results[index] = data
        }(i, node)
    }

    wg.Wait()
    return results
}

// 使用 rate limiter（client-go 内置）
import "k8s.io/client-go/util/workqueue"

rateLimiter := workqueue.NewItemExponentialFailureRateLimiter(
    100*time.Millisecond,  // 基础延迟
    10*time.Second,        // 最大延迟
)
```

## 5.3 数据降级与缓存策略

**实现方案**：
```go
// internal/datasource/fallback.go

type MetricsDataSource int
const (
    MetricsServer MetricsDataSource = iota
    KubeletSummary
    CachedData
)

func (dm *DataManager) FetchMetricsWithFallback() (*MetricsData, Notice) {
    // 优先级 1：Metrics Server
    if data, err := dm.metricsClient.FetchMetrics(); err == nil {
        dm.cache.Set("metrics", data, "metrics-server")
        return data, Notice{}
    }

    // 优先级 2：kubelet Summary API
    if data, err := dm.kubeletClient.FetchAllNodesSummary(); err == nil {
        dm.cache.Set("metrics", data, "kubelet")
        return data, Notice{
            Level: Warning,
            Message: "Metrics Server 不可达，使用 kubelet 数据（可能滞后 30-60s）",
            Suggestions: []string{
                "检查 Metrics Server 部署：kubectl get pods -n kube-system",
                "按 [M] 键查看详细诊断信息",
            },
        }
    }

    // 优先级 3：缓存数据
    if cached, ok := dm.cache.Get("metrics"); ok {
        entry := cached.(CacheEntry)
        age := time.Since(entry.Timestamp)
        return entry.Data.(*MetricsData), Notice{
            Level: Error,
            Message: fmt.Sprintf("所有数据源不可用，展示缓存数据（%s 前）", age),
            Suggestions: []string{
                "检查网络连接",
                "检查 RBAC 权限：kubectl auth can-i get nodes/stats",
                "按 [R] 键重试",
            },
        }
    }

    // 完全失败
    return nil, Notice{
        Level: Critical,
        Message: "无法获取数据，且无可用缓存",
        Suggestions: []string{
            "检查 kubeconfig 配置",
            "确认集群可访问",
        },
    }
}
```

## 5.4 错误提示 UI 实现

**实现方案**：
```go
// internal/ui/components/notice.go

type NoticeLevel int
const (
    Info NoticeLevel = iota
    Warning
    Error
    Critical
)

type Notice struct {
    Level       NoticeLevel
    Message     string
    Suggestions []string
    Actions     []Action  // 可执行的快速操作
}

type Action struct {
    Key         string  // 快捷键
    Description string
    Handler     func() tea.Cmd
}

// 渲染错误提示（使用 Lip Gloss 样式）
func (n Notice) Render() string {
    var style lipgloss.Style
    var icon string

    switch n.Level {
    case Warning:
        style = warningStyle
        icon = "⚠"
    case Error:
        style = errorStyle
        icon = "✖"
    case Critical:
        style = criticalStyle
        icon = "🚨"
    default:
        style = infoStyle
        icon = "ℹ"
    }

    var b strings.Builder
    b.WriteString(style.Render(fmt.Sprintf("%s %s", icon, n.Message)))
    b.WriteString("\n")

    if len(n.Suggestions) > 0 {
        b.WriteString("  → 诊断建议：\n")
        for i, suggestion := range n.Suggestions {
            b.WriteString(fmt.Sprintf("    %d. %s\n", i+1, suggestion))
        }
    }

    if len(n.Actions) > 0 {
        b.WriteString("  → 快速操作：")
        for _, action := range n.Actions {
            b.WriteString(fmt.Sprintf(" [%s] %s ", action.Key, action.Description))
        }
    }

    return b.String()
}
```

## 5.5 部分视图渲染（渐进式加载）

**实现方案**：
```go
// internal/ui/views/overview.go

type OverviewView struct {
    dataStates map[string]LoadState  // 跟踪各模块加载状态
}

type LoadState int
const (
    Loading LoadState = iota
    Loaded
    Failed
)

func (v *OverviewView) Render() string {
    var sections []string

    // 节点列表（来自 API Server，通常快）
    if v.dataStates["nodes"] == Loaded {
        sections = append(sections, v.renderNodeList())
    } else {
        sections = append(sections, "⏳ 加载节点列表...")
    }

    // 资源指标（来自 kubelet，可能慢）
    if v.dataStates["metrics"] == Loading {
        sections = append(sections, "⏳ 加载资源指标（3/10 节点已完成）...")
    } else if v.dataStates["metrics"] == Loaded {
        sections = append(sections, v.renderMetrics())
    } else {
        sections = append(sections, v.renderMetricsError())
    }

    return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// 数据更新时触发重新渲染
func (v *OverviewView) OnDataUpdate(module string, state LoadState) tea.Cmd {
    v.dataStates[module] = state
    return func() tea.Msg {
        return RefreshViewMsg{}
    }
}
```

## 5.6 配置文件示例

```yaml
# config/default.yaml
cluster:
  kubeconfig: ~/.kube/config
  context: ""  # 为空则使用当前 context

refresh:
  interval: 10s           # 自动刷新间隔
  timeout: 5s             # 单次请求超时
  max_concurrent: 10      # 最大并发请求数

cache:
  ttl: 60s                # 缓存过期时间
  max_entries: 1000       # 最大缓存条目数

datasource:
  priority:
    - apiserver           # 优先级 1
    - kubelet             # 优先级 2
    - metrics-server      # 优先级 3
  kubelet:
    port: 10250
    use_proxy: auto       # auto/direct/proxy
    timeout: 5s

ui:
  color_mode: auto        # auto/always/never
  default_view: overview  # overview/node/workload
  max_rows: 100           # 表格最大行数

filter:
  default_namespace: ""   # 默认过滤命名空间（空表示全部）
  exclude_namespaces:     # 排除的命名空间
    - kube-system
    - kube-public

logging:
  level: info             # debug/info/warn/error
  file: /tmp/k8s-monitor.log
  max_size: 100           # MB
  max_backups: 3
```

---

## 附录：开发规范

### A.1 代码规范

- **Go 代码风格**：遵循 `gofmt` + `golint` 标准
- **注释规范**：
  - 所有导出函数必须有文档注释
  - 复杂逻辑添加行内注释说明
- **错误处理**：
  - 使用 `errors.Wrap` 包装错误，保留调用栈
  - 关键错误记录日志（Zap）
- **命名规范**：
  - 包名：小写单词，无下划线（如 `datasource`）
  - 接口：名词或形容词（如 `DataSourceClient`）
  - 函数：动词开头（如 `FetchNodes`）

### A.2 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<type>(<scope>): <subject>

<body>

<footer>
```

**类型（type）**：
- `feat`: 新功能
- `fix`: 修复 bug
- `refactor`: 重构
- `docs`: 文档更新
- `test`: 测试相关
- `chore`: 构建/工具相关

**示例**：
```
feat(datasource): add kubelet client with proxy fallback

- Implement direct access to kubelet:10250
- Add API Server proxy fallback when direct access fails
- Add unit tests for both access methods

Closes #12
```

### A.3 测试规范

- **单元测试**：覆盖率 ≥70%
- **集成测试**：至少覆盖 3 个真实集群场景
- **性能测试**：每个版本发布前必测

**测试命令**：
```bash
# 运行所有测试
make test

# 运行单元测试
make test-unit

# 运行集成测试（需要真实集群）
make test-integration

# 性能测试
make test-perf
```

---

## 文档修订历史

### v1.0（2025-01-06）
- 初始版本，完整技术设计方案

---

**最后更新**：2025-01-06
**文档版本**：v1.0
**负责人**：开发团队
