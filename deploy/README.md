# K8s Monitor NPU Collector

DaemonSet for collecting Huawei Ascend NPU metrics using npu-smi and writing to Kubernetes node annotations.

## Overview

This collector runs on each NPU node, periodically executes `npu-smi info` to gather NPU metrics, and stores them in the node's annotation `k8s-monitor.io/npu-metrics`.

## Requirements

- Kubernetes cluster with Huawei Ascend NPU nodes
- Nodes labeled with `accelerator/huawei-npu=ascend-snt9c`
- NPU driver installed on host nodes (`/usr/local/Ascend/driver`, `/usr/local/sbin/npu-smi`)
- Access to `docker.1ms.run/library/python:3.11-slim` image

## Collected Metrics

For each NPU chip:
| Field | Description |
|-------|-------------|
| `npu` | NPU ID (0-7) |
| `chip` | Chip number (0-1) |
| `phy_id` | Physical ID |
| `bus_id` | PCIe Bus ID |
| `health` | Health status (OK/Warning/Error) |
| `power` | Power consumption (W) |
| `temp` | Temperature (°C) |
| `aicore` | AICore utilization (%) |
| `hbm_used` | HBM memory used (MB) |
| `hbm_total` | HBM memory total (MB) |

## Installation

```bash
kubectl apply -f k8s-monitor-npu-collector.yaml
```

## Verification

Check pod status:
```bash
kubectl get pods -n k8s-monitor
```

Check logs:
```bash
kubectl logs -n k8s-monitor -l app=k8s-monitor-npu-collector --tail=10
```

Check node annotations:
```bash
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{": "}{.metadata.annotations.k8s-monitor\.io/npu-metrics}{"\n"}{end}'
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `COLLECT_INTERVAL` | 3 | Collection interval in seconds |
| `NODE_NAME` | (from downward API) | Node name |

## Resource Usage

- CPU: 10m request, 100m limit
- Memory: 32Mi request, 64Mi limit

## Uninstallation

```bash
kubectl delete -f k8s-monitor-npu-collector.yaml
```

## Notes

- Uses `privileged: true` to access NPU devices
- Uses `hostNetwork: true` for Kubernetes API access
- Uses Python's built-in `urllib` for API calls (no curl dependency)
- Image: `docker.1ms.run/library/python:3.11-slim` (glibc-based, ARM64 compatible)
