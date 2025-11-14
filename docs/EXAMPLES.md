# k8s-monitor - Usage Examples

This document provides practical examples of using k8s-monitor to monitor your Kubernetes clusters.

## Table of Contents

- [Basic Usage](#basic-usage)
- [View Navigation](#view-navigation)
- [Filtering and Searching](#filtering-and-searching)
- [Common Scenarios](#common-scenarios)
- [Tips and Tricks](#tips-and-tricks)

## Basic Usage

### Starting the Monitor

#### Using Default Kubeconfig

```bash
# Start with default kubeconfig (~/.kube/config) and default context
k8s-monitor console
```

#### Specifying Kubeconfig Path

```bash
# Use a specific kubeconfig file
k8s-monitor console --kubeconfig /path/to/kubeconfig

# Short form
k8s-monitor console -k /path/to/kubeconfig
```

#### Using Specific Context

```bash
# Switch to a specific cluster context
k8s-monitor console --context production-cluster

# Short form
k8s-monitor console -c production-cluster
```

#### Monitoring Specific Namespace

```bash
# Filter pods by namespace on startup
k8s-monitor console --namespace kube-system

# Short form
k8s-monitor console -n kube-system
```

### Adjusting Refresh Interval

```bash
# Refresh every 5 seconds (faster updates)
k8s-monitor console --refresh 5

# Refresh every 30 seconds (reduce API calls)
k8s-monitor console -r 30
```

### Enable Debug Logging

```bash
# Enable verbose logging for troubleshooting
k8s-monitor console --verbose

# Short form
k8s-monitor console -v
```

### Using Custom Config File

```bash
# Use a specific configuration file
k8s-monitor console --config /etc/k8s-monitor/prod.yaml
```

## View Navigation

### Overview View (Default)

When you start k8s-monitor, you'll see the **Overview** view:

```
🔍 Kubernetes Monitor
Last updated: 15:42:30

┌─ Cluster Health ───────────────┐  ┌─ Nodes ─────────────────────────┐
│ Nodes: 3 / 3 online           │  │ Total Nodes: 3                  │
│ Ready: 3 / 3                  │  │ Ready: 3                        │
│                               │  │ NotReady: 0                     │
│ Status: Healthy ✓             │  │ CPU: 4800m / 12000m (40.0%)    │
└───────────────────────────────┘  │ Memory: 8.5 GB / 16.0 GB (53%)  │
                                   └─────────────────────────────────┘

┌─ Pods ─────────────────────────┐  ┌─ Recent Events ─────────────────┐
│ Running: 45                    │  │ [2m ago] Pod my-app-xyz         │
│ Pending: 2                     │  │   → CrashLoopBackOff            │
│ Failed: 1                      │  │                                 │
│ Unknown: 0                     │  │ [5m ago] Node worker-2          │
│                                │  │   → DiskPressure resolved       │
│ Total: 48                      │  │                                 │
└────────────────────────────────┘  └─────────────────────────────────┘

q: quit • r: refresh • 1/2/3: views • tab: next
```

**Quick Actions:**
- Press `1` - Stay in Overview
- Press `2` - Jump to Nodes view
- Press `3` - Jump to Pods view
- Press `Tab` - Cycle to next view
- Press `r` - Refresh data now
- Press `q` - Quit application

### Nodes View

Press `2` or `Tab` to go to **Nodes** view:

```
📋 Nodes  Total: 3

NAME                     STATUS      ROLES         CPU           MEMORY        PODS
────────────────────────────────────────────────────────────────────────────────────
control-plane-1          Ready       control-pl    1200m/4000m   2.8GB/8.0GB   15/50
worker-1                 Ready       worker        1800m/4000m   3.2GB/8.0GB   18/50
worker-2                 NotReady    worker        0m/4000m      0B/8.0GB      0/50

Ready: 2  NotReady: 1  Total: 3

q: quit • r: refresh • 1/2/3: views • tab: next • ↑/k: up • ↓/j: down • enter: detail
```

**Navigation:**
- Use `↑`/`k` or `↓`/`j` to select nodes
- Press `Enter` to view node details
- Selected row is highlighted

### Node Detail View

Press `Enter` on a selected node:

```
📋 Node: worker-1  ●● Ready

📋 Basic Information

  Name: worker-1
  Roles: worker
  Status: Ready ●●
  Internal IP: 192.168.1.101
  External IP: 203.0.113.101

📊 Resource Usage

  CPU: 1800m / 4000m (45.0%)
  Memory: 3.2 GB / 8.0 GB (40.0%)
  Pods: 18 / 50 (36.0%)

🐳 Pods on this Node (18 total)

  kube-system/coredns-xyz
  kube-system/kube-proxy-abc
  production/nginx-ingress-def
  production/my-app-v2-ghi
  ...

q: quit • r: refresh • esc: back
```

**Quick Actions:**
- Press `Esc` or `Backspace` - Return to nodes list
- Press `r` - Refresh data
- Press `q` - Quit application

### Pods View

Press `3` to go to **Pods** view:

```
📦 Pods  Total: 48

NAME                              NAMESPACE       STATUS       NODE         RESTARTS
─────────────────────────────────────────────────────────────────────────────────────
coredns-abc123                    kube-system     Running      control-1    0
kube-proxy-def456                 kube-system     Running      worker-1     0
nginx-ingress-ghi789              production      Running      worker-1     2
my-app-v2-jkl012                  production      CrashLoop    worker-2     15
database-mno345                   production      Running      worker-1     0
...

Running: 45  Pending: 2  Failed: 1  Total: 48  [1-20 of 48]

q: quit • r: refresh • 1/2/3: views • tab: next • ↑/k: up • ↓/j: down • enter: detail • f: filter
```

**Navigation:**
- Use `↑`/`↓` or `k`/`j` to scroll through pods
- Press `Enter` to view pod details
- Press `f` to open namespace filter
- If list is long, scroll indicator shows position `[1-20 of 48]`

### Pod Detail View

Press `Enter` on a selected pod:

```
📦 Pod: my-app-v2-jkl012  CrashLoopBackOff

📋 Basic Information

  Name: my-app-v2-jkl012
  Namespace: production
  Status: CrashLoopBackOff
  Node: worker-2
  Pod IP: 10.244.2.15
  Host IP: 192.168.1.102
  Restarts: 15

🐳 Containers (2 total, 1 ready)

  [1] app-container ●
      Image: myorg/myapp:v2.1.3
      Status: Waiting (restarts: 15)
      Reason: CrashLoopBackOff
      Message: Back-off 5m0s restarting failed container

  [2] sidecar-proxy ●●
      Image: envoyproxy/envoy:v1.28
      Status: Running (restarts: 0)

q: quit • r: refresh • esc: back
```

**Status Indicators:**
- `●●` Green dot = Container ready
- `●` Red dot = Container not ready

## Filtering and Searching

### Filter Pods by Namespace

In the **Pods** view, press `f` to open the filter panel:

```
📦 Pods  Total: 48

NAME                              NAMESPACE       STATUS       NODE         RESTARTS
─────────────────────────────────────────────────────────────────────────────────────
coredns-abc123                    kube-system     Running      control-1    0
...

Running: 45  Pending: 2  Failed: 1  Total: 48

🔍 Filter by Namespace

  [All namespaces]
  default
  kube-system
  production
  staging

q: quit • ↑/↓: select • enter: apply • esc: cancel
```

**Filter Controls:**
- Use `↑`/`↓` to select namespace
- Press `Enter` to apply filter
- Press `Esc` to cancel without applying
- Selection is highlighted

### After Applying Filter

```
📦 Pods  Total: 12 (filtered by: production)

NAME                              NAMESPACE       STATUS       NODE         RESTARTS
─────────────────────────────────────────────────────────────────────────────────────
nginx-ingress-ghi789              production      Running      worker-1     2
my-app-v2-jkl012                  production      CrashLoop    worker-2     15
database-mno345                   production      Running      worker-1     0
...

Running: 10  Pending: 1  Failed: 1  Total: 12

q: quit • r: refresh • 1/2/3: views • f: filter • c: clear
```

**Clear Filter:**
- Press `c` to clear the namespace filter
- Returns to showing all pods

## Common Scenarios

### Scenario 1: Quick Cluster Health Check

**Goal:** Verify cluster is healthy

```bash
# Start monitor
k8s-monitor console

# You'll immediately see:
# 1. Overview shows "Status: Healthy ✓" or issues
# 2. Node count (e.g., "3 / 3 online")
# 3. Pod status (Running/Pending/Failed counts)
# 4. Recent events (any warnings/errors)

# If healthy, you'll see:
# - All nodes Ready
# - Most pods Running
# - No recent error events

# Press q to quit
```

### Scenario 2: Investigate Failing Pods

**Goal:** Find and diagnose CrashLoopBackOff pods

```bash
# Start monitor
k8s-monitor console

# Step 1: Go to Pods view
Press 3

# Step 2: Look for red "Failed" or "CrashLoopBackOff" status
# Use ↓ or j to scroll down

# Step 3: Select the failing pod
Press Enter

# Step 4: Check container details
# Look for:
# - Restart count (high = persistent issue)
# - Reason field (e.g., "CrashLoopBackOff")
# - Message field (error details)

# Step 5: Note the pod name and namespace
# Exit to use kubectl for logs:
Press Esc, then q

# Step 6: Check logs
kubectl logs -n production my-app-v2-jkl012
```

### Scenario 3: Monitor Namespace Resources

**Goal:** Check resource usage in production namespace

```bash
# Start with namespace filter
k8s-monitor console --namespace production

# Or filter after starting:
Press 3 (Pods view)
Press f (Filter)
Use ↓ to select "production"
Press Enter

# Review:
# - Pod count in namespace
# - Running/Pending/Failed counts
# - Restart counts (high = stability issues)

# Check which nodes are running production pods:
Press Enter on a pod → Note the Node field
Press Esc → Press 2 (Nodes view)
Find that node → Press Enter
See all pods on that node
```

### Scenario 4: Pre-Deployment Check

**Goal:** Verify cluster readiness before deploying

```bash
k8s-monitor console -c production-cluster

# Checklist:
# 1. Overview: All nodes Ready?
# 2. Overview: Recent error events?
# 3. Nodes view (Press 2):
#    - Any nodes NotReady?
#    - CPU/Memory usage high (>80%)?
#    - Pods near capacity?
# 4. Pods view (Press 3):
#    - Any CrashLoopBackOff pods?
#    - High restart counts?

# If all green → Deploy
# If issues → Investigate first
```

### Scenario 5: Post-Deployment Verification

**Goal:** Verify new deployment is healthy

```bash
# Deploy your application
kubectl apply -f my-app.yaml

# Start monitor
k8s-monitor console -n production

# Watch for:
# 1. New pods appear (Press 3, look for your app name)
# 2. Status transitions: Pending → Running
# 3. Containers become ready (Enter on pod, check ●● dots)
# 4. No restart loops

# Keep monitor open:
# - Press r to refresh manually
# - Or wait for auto-refresh (default 10s)

# Verify:
# - All containers show ●● (green dots)
# - Restart count stays at 0
# - Status remains "Running"
```

### Scenario 6: Node Troubleshooting

**Goal:** Investigate a NotReady node

```bash
k8s-monitor console

# Step 1: Go to Nodes view
Press 2

# Step 2: Identify NotReady node (red status)
Use ↓/j to select the node

# Step 3: View node details
Press Enter

# Check:
# - Status conditions (DiskPressure? MemoryPressure?)
# - Resource usage (CPU/Memory at limits?)
# - Pod count (too many pods?)
# - Internal/External IP (network issue?)

# Step 4: Check pods on this node
Scroll down to "Pods on this Node" section
Note any problem pods

# Step 5: Exit and investigate
Press Esc → q
kubectl describe node <node-name>
```

## Tips and Tricks

### Fast Navigation

```bash
# Jump directly to any view:
1  → Overview
2  → Nodes
3  → Pods

# No need to Tab through views!
```

### Efficient Scrolling

```bash
# Use vim-style keys for faster navigation:
j  → Down (easier than ↓ arrow)
k  → Up (easier than ↑ arrow)

# Muscle memory from vim/less!
```

### Refresh Strategies

```bash
# Fast refresh for active monitoring (5s):
k8s-monitor console -r 5

# Slow refresh to reduce API load (30s):
k8s-monitor console -r 30

# Manual refresh only (set high interval, press r when needed):
k8s-monitor console -r 300
# Then press r to refresh on demand
```

### Multi-Cluster Workflow

```bash
# Add aliases to ~/.bashrc or ~/.zshrc:
alias k8s-dev='k8s-monitor console -c dev-cluster'
alias k8s-staging='k8s-monitor console -c staging-cluster'
alias k8s-prod='k8s-monitor console -c production-cluster'

# Usage:
k8s-dev     # Monitor dev cluster
k8s-prod    # Monitor production
```

### Quick Health Check Script

```bash
#!/bin/bash
# check-cluster.sh
# Quick cluster health check

echo "Checking cluster health..."
k8s-monitor console &
MONITOR_PID=$!

# Let it run for 10 seconds to gather data
sleep 10

# Kill the monitor
kill $MONITOR_PID

echo "Check complete. Review the output above."
```

### Combine with kubectl

```bash
# Use k8s-monitor to identify issues
# Then switch to kubectl for detailed investigation

# Example workflow:
# 1. k8s-monitor console → Find failing pod "my-app-xyz"
# 2. Exit (q)
# 3. kubectl logs -n production my-app-xyz
# 4. kubectl describe pod -n production my-app-xyz
# 5. k8s-monitor console → Verify fix
```

### Filter + Detail Workflow

```bash
# Efficient namespace investigation:

Press 3           # Pods view
Press f           # Open filter
Select namespace  # Use ↑/↓
Press Enter       # Apply filter

# Now only see pods in that namespace
# Navigate with j/k
# Press Enter on any pod for details
# Press Esc to go back to filtered list

Press c           # Clear filter when done
```

### Color vs No-Color

```bash
# Default: Auto-detect color support
k8s-monitor console

# Force disable colors (for scripts/logs):
k8s-monitor console --no-color

# Useful for:
# - Piping output
# - Terminal without color support
# - Screenshots/documentation
```

### Remote Cluster Monitoring

```bash
# Copy kubeconfig to monitoring machine:
scp ~/.kube/config monitor-server:~/.kube/

# SSH to monitoring machine:
ssh monitor-server

# Run monitor:
k8s-monitor console

# Or use SSH port forwarding for API server access:
ssh -L 6443:k8s-api-server:6443 bastion-host
k8s-monitor console --kubeconfig local-kubeconfig
```

## Keyboard Reference Card

```
┌─────────────────────────────────────────────────────────┐
│                 k8s-monitor Quick Keys                  │
├─────────────────────────────────────────────────────────┤
│ GLOBAL                                                  │
│  q / Ctrl+C    Quit application                        │
│  r             Refresh data now                        │
│  1             Jump to Overview                        │
│  2             Jump to Nodes                           │
│  3             Jump to Pods                            │
│  Tab           Cycle through views                     │
├─────────────────────────────────────────────────────────┤
│ LIST VIEWS (Nodes, Pods)                               │
│  ↑ / k         Move selection up                       │
│  ↓ / j         Move selection down                     │
│  Enter         View details                            │
├─────────────────────────────────────────────────────────┤
│ DETAIL VIEWS                                            │
│  Esc           Back to list                            │
│  Backspace     Back to list (alternative)              │
├─────────────────────────────────────────────────────────┤
│ FILTERING (Pods view only)                             │
│  f             Open filter panel                       │
│  c             Clear filter                            │
│  ↑ / ↓         Select namespace (in filter panel)      │
│  Enter         Apply filter (in filter panel)          │
│  Esc           Cancel filter (in filter panel)         │
└─────────────────────────────────────────────────────────┘
```

## Next Steps

- See [README.md](../README.md) for installation instructions
- See [CHANGELOG.md](../CHANGELOG.md) for release notes
- Check [docs/development_plan.md](development_plan.md) for upcoming features
- Report issues at [GitHub Issues](https://github.com/yourusername/k8s-monitor/issues)

---

**Happy Monitoring!** 🚀
