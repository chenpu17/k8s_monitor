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
| 指标路径 | `/metrics` | `fetchMetricsViaProxy` 的 `Suffix("metrics")` |
| 访问方式 | API Server 代理 | `/api/v1/namespaces/kube-system/services/npu-exporter:8082/proxy/metrics` |

所以:**改名、换命名空间、换端口都会静默关闭 NPU 指标**,客户端只会退化为「NPU-Exporter 不可达」,
不会报错。若确实部署在别处,用 `--npu-exporter http://host:port` 覆盖(该分支走直连 HTTP,不走代理)。

`deploy/k8s-monitor-npu-collector.yaml` 是另一条独立通路(用 npu-smi 写节点注解),两者不冲突,可同时存在。

## 2. 镜像

镜像**必须按你自己的授权渠道替换**,YAML 中已用醒目注释标出。仓库里能确认的两个来源:

1. **华为云 CCE 官方 npu-exporter**(本仓库 README 已记录的引用):
   `swr.cn-north-12.myhuaweicloud.com/hwofficial/npu-exporter:2.3.2`
   —— 与节点上实际运行的设备插件同属 `swr.*.myhuaweicloud.com/hwofficial/` 家族
   (设备插件实测为 `swr.cn-east-4.myhuaweicloud.com/hwofficial/huawei-npu-device-plugin:2.10.1`),
   因此同区域的 `hwofficial` 仓库是最省事的来源。**本仓库未验证该 tag 是否存在**,
   `kubectl apply` 前请先确认。
2. **MindX DL npu-exporter**(Ascend Hub 发行,等价实现):
   【需按 ascendhub 许可获取/替换】镜像受许可限制,需从你自己的 Ascend Hub 订阅拉取后替换 YAML 中的 `image`。

未联网、也未在集群中部署过 npu-exporter,因此**没有任何镜像 tag 是实测验证过的**;
上述两条只是获取途径,YAML 里的注释同时说明了该改哪里。

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

- `EnrichNodeData` 只在**单个 NPU 节点**的集群里把指标归属到节点:多个 NPU 节点时,
  ClusterIP 后面的响应无法区分来源节点,客户端会记一条 warn 并跳过 enrichment
  (`internal/datasource/npu_exporter.go`)。多 NPU 节点场景请用
  `k8s-monitor-npu-collector.yaml` 那条按节点采集的通路。
- 部署本 DaemonSet 前,节点详情页的芯片健康位图(`huawei.com/ascend-status`)与
  管理面故障信息(`os.modelarts.node/fault-info`)不依赖 npu-exporter,由设备插件/巡检任务直接写在节点上。
