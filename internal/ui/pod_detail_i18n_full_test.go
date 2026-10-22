package ui

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderPodDetail and its section renderers bilingual lockdown: the English
// output must stay byte-identical to the previously hardcoded strings
// (including leading indentation), and the Chinese output must switch.
// The clear labels (Reason/Message/Usage/Resources) render through keys —
// Reason/Message are shared with the event detail view via detail.field.*
// (see detail_field_shared_keys_test.go). The compact technical lines keep
// their intentional mix: CPU:/req=/limit= stay hardcoded (CPU is a technical
// token whose Chinese value is identical; req/limit are parameter notation),
// while Mem and the network-rates line translate (detail.pod.mem_label,
// detail.pod.network_rates). forcedAsciiProfile keeps the goldens plain text.

func podDetailFixture(name string, states []model.ContainerState) *model.PodData {
	return &model.PodData{
		Name: name, Namespace: "ns1", Phase: "Running",
		Node: "node-1", PodIP: "10.0.0.5", HostIP: "192.168.1.10",
		Containers: 3, ReadyContainers: 2, RestartCount: 3,
		ContainerStates: states,
	}
}

func podDetailHistory(key string) []MetricSnapshot {
	now := time.Now()
	snap := func(rx, tx int64, ts time.Time) *PodMetric {
		return &PodMetric{CPUUsage: 100, MemoryUsage: 256 * 1024 * 1024, NetworkRxBytes: rx, NetworkTxBytes: tx, Timestamp: ts}
	}
	return []MetricSnapshot{
		{Timestamp: now.Add(-40 * time.Second), PodMetrics: map[string]*PodMetric{key: snap(1000, 2000, now.Add(-40*time.Second))}},
		{Timestamp: now.Add(-20 * time.Second), PodMetrics: map[string]*PodMetric{key: snap(3000, 6000, now.Add(-20*time.Second))}},
		{Timestamp: now, PodMetrics: map[string]*PodMetric{key: snap(7000, 12000, now)}},
	}
}

func TestRenderPodBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	pod := podDetailFixture("pod-a", nil)

	// No history yet: the collecting placeholder.
	en.metricHistory = nil
	want := " 📋 Basic Information \n\n" +
		"  Name: pod-a\n" +
		"  Namespace: ns1\n" +
		"  Status: Running\n" +
		"  Node: node-1\n" +
		"  Pod IP: 10.0.0.5\n" +
		"  Host IP: 192.168.1.10\n" +
		"  Restarts: 3\n\n" +
		"  Network Bandwidth:\n" +
		"    Collecting bandwidth data..."
	if got := en.renderPodBasicInfo(pod); got != want {
		t.Errorf("en basic info without history:\n got %q\nwant %q", got, want)
	}

	// History present but the pod is missing from the last snapshot.
	en.metricHistory = []MetricSnapshot{
		{Timestamp: time.Now().Add(-20 * time.Second), PodMetrics: map[string]*PodMetric{
			"ns1/pod-a": {CPUUsage: 100, MemoryUsage: 1, NetworkRxBytes: 1, NetworkTxBytes: 1, Timestamp: time.Now().Add(-20 * time.Second)},
		}},
		{Timestamp: time.Now(), PodMetrics: map[string]*PodMetric{
			"other/p": {CPUUsage: 1, MemoryUsage: 1, NetworkRxBytes: 1, NetworkTxBytes: 1, Timestamp: time.Now()},
		}},
	}
	got := en.renderPodBasicInfo(pod)
	if !strings.Contains(got, "    Metrics unavailable (pod not reporting kubelet stats)") {
		t.Errorf("en basic info must contain the metrics-unavailable line:\n%s", got)
	}

	// History with the pod present: rate line plus trend sparklines.
	en.metricHistory = podDetailHistory("ns1/pod-a")
	got = en.renderPodBasicInfo(pod)
	for _, frag := range []string{
		"    RX ↓: 0 B/s  •  TX ↑: 0 B/s  •  Total: 0 B/s",
		"  Historical Trends:",
		"  (Based on 3 snapshots)",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("en basic info with history must contain %q:\n%s", frag, got)
		}
	}
	if strings.Count(got, "▄▄▄") != 2 || strings.Count(got, "▁█") != 2 {
		t.Errorf("en basic info must contain the four trend sparklines:\n%s", got)
	}
}

func TestRenderPodBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	pod := podDetailFixture("pod-a", nil)
	zh.metricHistory = podDetailHistory("ns1/pod-a")

	got := zh.renderPodBasicInfo(pod)
	// RX/TX switch to the bundle-wide terms (detail.node.rx/tx/total), the
	// arrows stay, and Total translates.
	for _, noun := range []string{
		"📋 基本信息", "名称", "命名空间", "节点", "重启次数",
		"网络带宽：", "历史趋势：", "（基于 3 个快照）",
		"接收 ↓:", "发送 ↑:", "总计:",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Basic Information", "Network Bandwidth", "Historical Trends",
		"RX ↓", "TX ↑", "Total:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderPodContainerInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	// No states at all.
	got := en.renderPodContainerInfo(&model.PodData{Name: "pod-s", Namespace: "ns1", Phase: "Pending", Containers: 1})
	want := " 🐳 Containers (1 total, 0 ready) \n\n  No container information available"
	if got != want {
		t.Errorf("en container info without states:\n got %q\nwant %q", got, want)
	}

	// Reason/Message labels, usage section with limit percentages, and the
	// resources-only section with req=/limit= notation.
	pod := podDetailFixture("pod-b", []model.ContainerState{
		{Name: "c1", Image: "nginx:1.25", Ready: true, RestartCount: 0, State: "Running"},
		{Name: "c2", Image: "busybox", Ready: true, RestartCount: 2, State: "Terminated", Reason: "OOMKilled", Message: "word",
			CPUUsage: 250, CPULimit: 1000, MemoryUsage: 512 * 1024 * 1024, MemoryLimit: 1024 * 1024 * 1024},
	})
	got = en.renderPodContainerInfo(pod)
	want = " 🐳 Containers (3 total, 2 ready) \n\n" +
		"  [1] c1 ●\n" +
		"      Image: nginx:1.25\n" +
		"      State: Running (restarts: 0)\n\n" +
		"  [2] c2 ●\n" +
		"      Image: busybox\n" +
		"      State: Terminated (restarts: 2)\n" +
		"      Reason: OOMKilled\n" +
		"      Message: word\n" +
		"      Usage:\n" +
		"        CPU: 250m / 1.00 (25.0%)\n" +
		"        Mem: 512.0 MiB / 1.0 GiB (50.0%)"
	if got != want {
		t.Errorf("en container info with states:\n got %q\nwant %q", got, want)
	}

	// Usage with request-only fallback: "(req: ...)" stays notation.
	pod = podDetailFixture("pod-r", []model.ContainerState{
		{Name: "u1", Image: "app:2.0", State: "Running", CPUUsage: 100, CPURequest: 200},
		{Name: "u2", Image: "app:2.0", State: "Running", MemoryUsage: 64 * 1024 * 1024, MemoryRequest: 128 * 1024 * 1024},
	})
	got = en.renderPodContainerInfo(pod)
	if !strings.Contains(got, "        CPU: 100m (req: 200m)") ||
		!strings.Contains(got, "        Mem: 64.0 MiB (req: 128.0 MiB)") {
		t.Errorf("en usage request-only lines drifted:\n%s", got)
	}

	// No usage: the Resources section with req=/limit= notation.
	pod = podDetailFixture("pod-c", []model.ContainerState{
		{Name: "r1", Image: "app:2.0", State: "Running", CPURequest: 100, CPULimit: 500, MemoryRequest: 256 * 1024 * 1024},
		{Name: "r2", Image: "app:2.0", State: "Running", CPULimit: 250, MemoryLimit: 128 * 1024 * 1024},
	})
	got = en.renderPodContainerInfo(pod)
	want = "      Resources:\n" +
		"        CPU: req=100m, limit=500m\n" +
		"        Mem: req=256.0 MiB"
	if !strings.Contains(got, want) {
		t.Errorf("en resources-only section (r1) drifted:\n got %q\nwant substring %q", got, want)
	}
	want = "      Resources:\n" +
		"        CPU: limit=250m\n" +
		"        Mem: limit=128.0 MiB"
	if !strings.Contains(got, want) {
		t.Errorf("en resources-only section (r2) drifted:\n got %q\nwant substring %q", got, want)
	}
}

func TestRenderPodContainerInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	pod := podDetailFixture("pod-b", []model.ContainerState{
		{Name: "c1", Image: "nginx:1.25", Ready: true, RestartCount: 0, State: "Running", Reason: "CrashLoopBackOff", Message: "boom"},
		{Name: "c2", Image: "busybox", Ready: false, RestartCount: 1, State: "Waiting",
			CPUUsage: 100, CPURequest: 200, MemoryUsage: 64 * 1024 * 1024, MemoryRequest: 128 * 1024 * 1024},
		{Name: "c3", Image: "alpine", Ready: false, RestartCount: 2, State: "Running", CPURequest: 100, MemoryLimit: 128 * 1024 * 1024},
	})

	got := zh.renderPodContainerInfo(pod)
	// "内存" translates (detail.pod.mem_label) while "CPU"/req/limit stay
	// technical notation; Reason/Message switch to 原因/消息.
	for _, noun := range []string{
		"🐳 容器（共 3 个，2 个就绪）", "镜像", "原因: CrashLoopBackOff", "消息: boom",
		"用量:", "资源:", "内存: 64.0 MiB (req: 128.0 MiB)", "（重启次数：0）", "（重启次数：1）",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh container info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Containers", "No container information", "Image:", "Reason:", "Message:",
		"Usage:", "Resources:", "(restarts:", "Mem:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh container info %q contains %q", got, bad)
		}
	}
	// The CPU lines keep the technical token in both locales.
	if !strings.Contains(got, "        CPU: 100m (req: 200m)") {
		t.Errorf("zh container info must keep the CPU technical line:\n%s", got)
	}
}

func TestRenderPodDetailScrollEn(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	en.width = 120
	en.selectedPod = podDetailFixture("pod-a", nil)

	en.height = 1000
	if got := en.renderPodDetail(); strings.Contains(got, "[Lines") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", got)
	}

	en.height = 12 // maxVisible = 4
	got := en.renderPodDetail()
	// The long variant (page hint) — the same detail.scroll_indicator key the
	// node/job/volcanojob detail views render.
	if !strings.Contains(got, "[Lines 1-4 of ") ||
		!strings.Contains(got, "] (↑/↓ to scroll, PgUp/PgDn for page)") {
		t.Errorf("short render must show the long scroll indicator:\n%s", got)
	}
}
