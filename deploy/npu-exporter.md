# NPU-Exporter 部署说明(华为 Ascend)

`npu-exporter.yaml` 部署一个 DaemonSet + Service,让 k8s-monitor 的**默认 NPU 指标通路**真正可用。
本文说明每个取值的来源,以及无法从本仓库确认的部分(镜像)该如何获取。

## 1. 为什么必须叫 kube-system/npu-exporter:8082

k8s-monitor 的 NPU-Exporter 客户端把服务名、命名空间、端口和路径都写死在代码里
(`internal/datasource/npu_exporter.go`):

| 项 | 值 | 位置 |
|---|---|---|
| Service 命名空间 | `kube-system` | `NewNPUExporterClient` 的 `serviceNS` |
| Service 名 | `npu-exporter` | `serviceName` |
| Service 端口 | `8082` | `servicePort` |
| 指标路径 | `/metrics` | `fetchMetricsViaProxy` / `fetchPodMetrics` 的 `Suffix("metrics")` |
| 逐 pod 抓取 | API Server pod 代理 | `/api/v1/namespaces/kube-system/pods/{pod}:8082/proxy/metrics` |
| pod 发现 | API Server 列表,两种标签都认:`app.kubernetes.io/name=npu-exporter`(华为官方清单)与 `app=npu-exporter`(本仓库 YAML) | `listExporterPods` 的 `npuExporterPodSelectors` |
| 回退通路 | API Server service 代理 | `/api/v1/namespaces/kube-system/services/npu-exporter:8082/proxy/metrics` |

客户端优先**逐 exporter pod 抓取**并按 pod 所在节点归属(多 NPU 节点集群因此可用);
发现不到 pod 时才回退到 Service 单份响应。两条通路的取舍见第 6 节。

所以:**改名、换命名空间、换端口都会静默关闭 NPU 指标**,客户端只会退化为「NPU-Exporter 不可达」,
不会报错。若确实部署在别处,用 `--npu-exporter http://host:port` 覆盖(该分支走直连 HTTP,不走代理,
且**没有节点身份**,只适用于单 NPU 节点集群)。

`deploy/k8s-monitor-npu-collector.yaml` 是另一条独立通路(用 npu-smi 写节点注解),两者不冲突,可同时存在。

## 2. 镜像

1. **华为云 CCE 官方 npu-exporter**(YAML 当前引用):
   `swr.cn-north-12.myhuaweicloud.com/hwofficial/npu-exporter:2.3.2`
   —— **已实测可用**(2026-10-21,华为云 CCE aarch64 节点,Huawei Cloud EulerOS 2.0):拉取成功(约 324 MB),
   配合下文参数正常采集 8×910B1 芯片指标,与节点上实测的设备插件
   (`swr.cn-east-4.myhuaweicloud.com/hwofficial/huawei-npu-device-plugin:2.10.1`)同属 `hwofficial` 家族。
2. **MindX DL npu-exporter**(Ascend Hub 发行,等价实现):
   【需按 ascendhub 许可获取/替换】镜像受许可限制,需从你自己的 Ascend Hub 订阅拉取后替换 YAML 中的 `image`。

### v2.3.2 的参数要求(实测踩坑)

- **`-ip` 必填**:不传时启动即退出,报 `the listen ip is invalid`。YAML 用 downward API 把
  `status.podIP`(hostNetwork 下即节点主网卡地址)传给它,避免绑 0.0.0.0(工具自身也不推荐多网卡宿主绑 0.0.0.0)。
- `-port` 默认即 `8082`,与客户端硬编码一致,无需显式传。
- `-containerMode` 默认 `docker`,与该 CCE 集群的实际运行时(docker 18.9.0)匹配;
  **containerd 集群需改 `containerd`** 并挂载对应 socket,否则芯片→Pod 归因退化。
- `-updateTime 2`:指标缓存刷新 2s,配合 k8s-monitor 默认 2s 自动刷新。
- 用 `docker run <image> npu-exporter -h`(或探针 pod)可查看任意 tag 的完整 flag 表。

## 3. YAML 中各项取值的来源

| 取值 | 来源 |
|---|---|
| `hostNetwork: true`、`tolerations: [{operator: Exists}]`、`privileged: true` | 实测自集群中 `kube-system/huawei-npu-device-plugin` DaemonSet(同一台 NPU 宿主机上运行,访问同一套驱动) |
| 挂载 `/usr/local/Ascend`、`/usr/local/sbin/npu-smi`、`/etc/localtime`、`/dev` | 同上,来自设备插件的 `volumeMounts`/`volumes` 实测值 |
| 挂载 `/usr/local/dcmi`、`LD_LIBRARY_PATH` 中的 dcmi 路径 | npu-exporter 依赖 dcmi 库读取芯片信息;设备插件未挂载该路径,若你的节点没有 `/usr/local/dcmi` 会自动创建空目录(需按实际驱动安装路径调整) |
| `nodeSelector: accelerator/huawei-npu: ascend-1980` | 实测节点标签(`accelerator/huawei-npu=ascend-1980`)。若你的集群用别的取值(Ascend 310/310P 等),改成对应值 |
| `port 8082` + `readinessProbe /metrics` | 见第 1 节的客户端硬编码 |
| ClusterRole 只读 `nodes`/`pods` | npu-exporter 需要把芯片归属到 Pod/节点;给的是最小只读集合 |

YAML 中**没有**任何来自本集群的私有 IP、主机名或机型名称,也没有臆造的驱动版本号。

## 4. 部署

```bash
kubectl apply -f deploy/npu-exporter.yaml
kubectl rollout status ds/npu-exporter -n kube-system
kubectl get pods -n kube-system -l app=npu-exporter -o wide
```

## 5. 验证

先确认 Service 名字和端口与客户端硬编码一致:

```bash
kubectl get svc -n kube-system npu-exporter
```

再确认指标能从 **API Server 代理**这一条真实链路取到(k8s-monitor 走的就是它):

```bash
kubectl get --raw /api/v1/namespaces/kube-system/services/npu-exporter:8082/proxy/metrics | head
```

期望看到 `machine_npu_nums <N>` 以及 `npu_chip_info_*{id="..."}` 系列指标;
`internal/datasource/npu_exporter.go` 正是按这些指标名解析的(`machine_npu_nums`、`npu_chip_info_utilization`、
`npu_chip_info_health_status` 等)。若返回 502/503,说明 Service 有端口但没有就绪的 Endpoint:

```bash
kubectl get endpoints -n kube-system npu-exporter
kubectl logs -n kube-system -l app=npu-exporter --tail=50
```

最后在 k8s-monitor 里看节点详情页的 NPU 指标块是否出现真实数值(不再是 "NPU-Exporter not reachable")。

## 6. 已知限制

- 客户端先按两种标签(`app.kubernetes.io/name=npu-exporter` 与 `app=npu-exporter`)列出
  `kube-system` 下的 exporter pod,再**逐个 pod 走 `pods/{name}:8082/proxy/metrics` 抓取**,
  按 `spec.nodeName` 把每份指标归到对应节点,因此**多 NPU 节点集群也能按节点归属**
  (每份响应只含它所在节点的芯片)。
- **权限前置条件(最容易踩的坑)**:这一步需要 k8s-monitor 自身凭证具备 `pods` 的 `list`
  与 `pods/proxy` 的 `get`。`services/proxy` 与 `pods/proxy` 是**两套独立授权**,只授过
  service 代理的旧凭证会在逐 pod 通路全数被拒。部署前自查:
  ```bash
  kubectl auth can-i list pods -n kube-system
  kubectl auth can-i get pods/proxy -n kube-system
  ```
  若要给 k8s-monitor 建一个专用最小权限身份,可直接 apply:
  ```yaml
  apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRole
  metadata:
    name: k8s-monitor-npu
  rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list"]          # 发现 exporter pod 与归属容器
  - apiGroups: [""]
    resources: ["pods/proxy"]
    verbs: ["get"]           # 逐 pod 抓取 /metrics(主通路)
  - apiGroups: [""]
    resources: ["services/proxy"]
    verbs: ["get"]           # service 回退通路
  ```
  (`deploy/k8s-monitor-npu-collector.yaml` 里的 Role 属于 npu-smi 采集器,**不是**
  k8s-monitor 主程序的;它不需要也不应共用这里的 proxy 权限。)
- **通路取舍**:发现不到 exporter pod(列表失败或为空)时回退到 Service 单份响应的老路径,
  那条通路**只有一个答案、无节点身份**,因此仅在**单个 NPU 节点**的集群里做 enrichment,
  多节点时记一条 warn 并跳过。用 `--npu-exporter <url>`(自定义端点)同样走这条单份通路。
  另有一条自动兜底:**pod 列表成功、但逐个 proxy 全部被拒时,单 NPU 节点集群会转走
  Service 通路**(该场景下答案归属同样成立),多节点集群则进退避并出
  `npu_monitor_degraded` 提示——若单节点集群"明明有 exporter 却只剩注解健康位图",
  先用上面的 `auth can-i` 自查。
- 部署本 DaemonSet 前,节点详情页的芯片健康位图(`huawei.com/ascend-status`)与
  管理面故障信息(`os.modelarts.node/fault-info`)不依赖 npu-exporter,由设备插件/巡检任务直接写在节点上。
