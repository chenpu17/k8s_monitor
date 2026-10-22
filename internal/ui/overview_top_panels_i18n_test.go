package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The overview's top-row and NPU/Volcano panels (podDetailsLines,
// nodesAndPodsLines, eventSummaryLines, npuPanelLines, volcanoPanelLines) and
// the five workload/services section headers render their labels through the
// localizer, so the English output must stay byte-identical to the previously
// hardcoded strings (golden) and the Chinese output must actually switch while
// keeping the value columns aligned. forcedAsciiProfile (compact_alerts_test.go)
// makes the goldens plain text; only the StyleHeader panel titles carry layout
// padding, so every non-header line is compared byte-exact without TrimSpace.

// npuEfflowSummary builds the ClusterSummary used by the npu efficiency/sched
// fixtures; eff = NPURunningByJobs / NPUAllocated, sched = Running/(Running+Pending).
func npuEfflowSummary() *model.ClusterSummary {
	return &model.ClusterSummary{
		NPUCapacity:    64,
		NPUAllocatable: 64,
		NPUAllocated:   32,
		NPUUtilization: 50,
		NPUChipType:    "Ascend910B",
		SuperPodCount:  2,
		NPUNodesCount:  8,
	}
}

func npuEfflowClusterData() *model.ClusterData {
	return &model.ClusterData{
		VolcanoSummary: &model.VolcanoSummary{
			TotalJobs: 5, RunningJobs: 3, PendingJobs: 1, FailedJobs: 1,
			NPURequestedByJobs: 24, NPURunningByJobs: 8,
		},
	}
}

func TestOverviewTopPanelsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	// assertTopPanelGolden compares the header line with TrimSpace (StyleHeader
	// pads by one space on each side) and every other line byte-exact.
	assertTopPanelGolden := func(t *testing.T, lines, want []string) {
		t.Helper()
		if len(lines) != len(want) {
			t.Fatalf("panel returned %d lines, want %d: %q", len(lines), len(want), lines)
		}
		for i := range want {
			if i == 0 {
				if got := strings.TrimSpace(lines[i]); got != want[i] {
					t.Errorf("header line = %q, want %q", got, want[i])
				}
				continue
			}
			if lines[i] != want[i] {
				t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
			}
		}
	}

	t.Run("pods", func(t *testing.T) {
		assertTopPanelGolden(t, en.podDetailsLines(&model.ClusterSummary{
			PodAllocatable: 110, RunningPods: 80, PendingPods: 20, FailedPods: 3,
		}), []string{
			"[📦 Pods]",
			"",
			"Capacity: 110",
			"Running: 80",
			"Pending: 20",
			"Failed: 3",
		})
		assertTopPanelGolden(t, en.podDetailsLines(&model.ClusterSummary{}), []string{
			"[📦 Pods]", "", "Capacity: 0", "Running: 0", "Pending: 0", "Failed: 0",
		})
	})

	t.Run("nodes", func(t *testing.T) {
		assertTopPanelGolden(t, en.nodesAndPodsLines(&model.ClusterSummary{
			TotalNodes: 10, ReadyNodes: 8, NotReadyNodes: 2,
		}), []string{
			"[💻 Nodes]",
			"",
			"Total:    10",
			"Ready:    8",
			"NotReady: 2",
		})
	})

	t.Run("events", func(t *testing.T) {
		assertTopPanelGolden(t, en.eventSummaryLines(&model.ClusterSummary{
			TotalEvents: 100, WarningEvents: 30, ErrorEvents: 5,
		}), []string{
			"[📋 Events]",
			"",
			"Total:   100",
			"Warning: 30",
			"Error:   5",
		})
	})

	t.Run("npu", func(t *testing.T) {
		// No NPU nodes: the empty-state branch.
		en.clusterData = &model.ClusterData{}
		assertTopPanelGolden(t, en.npuPanelLines(&model.ClusterSummary{}), []string{
			"🧮 NPU", "", "No NPU nodes",
		})

		// Basic stats only: no volcano, no history, no chip, no collector hint.
		en.clusterData = &model.ClusterData{}
		en.metricHistory = nil
		assertTopPanelGolden(t, en.npuPanelLines(&model.ClusterSummary{
			NPUCapacity: 64, NPUAllocatable: 64, NPUAllocated: 30, NPUUtilization: 46.9,
		}), []string{
			"🧮 NPU",
			"",
			"Total:   64",
			"Alloc:   30",
			"Usage:   46.9%",
			"  ██████░░░░░░░░",
		})

		// Full branch set: low efficiency, sched warning, trend, chip type,
		// superpod node count, deploy-collector hint (no node chip data).
		en.clusterData = npuEfflowClusterData()
		en.metricHistory = []MetricSnapshot{
			{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 20},
			{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 25},
			{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 28},
		}
		assertTopPanelGolden(t, en.npuPanelLines(npuEfflowSummary()), []string{
			"🧮 NPU",
			"",
			"Total:   64",
			"Alloc:   32",
			"Usage:   50.0%",
			"  ███████░░░░░░░",
			"Eff:     25.0%",
			"Sched:   75% (3/4)",
			"Trend: ▁▅█            ↑",
			"",
			"Chip: Ascend910B",
			"Pods: 8 nodes",
			"",
			"Deploy collector",
			"for chip metrics",
		})

		// High efficiency, sched 100%, chip metrics available (no hint lines),
		// no trend history, no chip type, no superpod.
		en.clusterData = &model.ClusterData{
			VolcanoSummary: &model.VolcanoSummary{
				TotalJobs: 8, RunningJobs: 8,
				NPURequestedByJobs: 24, NPURunningByJobs: 24,
			},
			Nodes: []*model.NodeData{{Name: "n1", NPUChips: []model.NPUChipData{{}}}},
		}
		en.metricHistory = nil
		assertTopPanelGolden(t, en.npuPanelLines(&model.ClusterSummary{
			NPUCapacity: 64, NPUAllocatable: 64, NPUAllocated: 32, NPUUtilization: 50,
		}), []string{
			"🧮 NPU",
			"",
			"Total:   64",
			"Alloc:   32",
			"Usage:   50.0%",
			"  ███████░░░░░░░",
			"Eff:     75.0%",
			"Sched:   100% (8/8)",
		})

		// Volcano present but nothing active: no Sched line, Eff renders the
		// zero special case.
		en.clusterData = &model.ClusterData{VolcanoSummary: &model.VolcanoSummary{}}
		en.metricHistory = nil
		assertTopPanelGolden(t, en.npuPanelLines(&model.ClusterSummary{
			NPUCapacity: 64, NPUAllocatable: 64, NPUAllocated: 16, NPUUtilization: 25,
			NPUChipType: "Ascend310P",
		}), []string{
			"🧮 NPU",
			"",
			"Total:   64",
			"Alloc:   16",
			"Usage:   25.0%",
			"  ███░░░░░░░░░░░",
			"Eff:     0%",
			"",
			"Chip: Ascend310P",
		})
	})

	t.Run("volcano", func(t *testing.T) {
		// VolcanoSummary nil: the empty-state branch.
		en.clusterData = &model.ClusterData{}
		assertTopPanelGolden(t, en.volcanoPanelLines(&model.ClusterSummary{}), []string{
			"🌋 Volcano", "", "Not available",
		})

		// Everything on: failed jobs, NPU usage, queues.
		en.clusterData = &model.ClusterData{
			VolcanoSummary: &model.VolcanoSummary{
				TotalJobs: 12, RunningJobs: 5, PendingJobs: 3, FailedJobs: 2,
				TotalQueues: 4, NPURequestedByJobs: 24, NPURunningByJobs: 8,
			},
		}
		assertTopPanelGolden(t, en.volcanoPanelLines(&model.ClusterSummary{NPUCapacity: 64}), []string{
			"🌋 Volcano",
			"",
			"Jobs:    12",
			"Running: 5",
			"Pending: 3",
			"Failed:  2",
			"",
			"NPU Req: 24",
			"NPU Run: 8",
			"",
			"Queues:  4",
		})

		// Only the job trio; failed/NPU/queues lines stay absent.
		en.clusterData = &model.ClusterData{
			VolcanoSummary: &model.VolcanoSummary{TotalJobs: 2, RunningJobs: 1, PendingJobs: 1},
		}
		assertTopPanelGolden(t, en.volcanoPanelLines(&model.ClusterSummary{}), []string{
			"🌋 Volcano", "", "Jobs:    2", "Running: 1", "Pending: 1",
		})

		// Queues without jobs activity.
		en.clusterData = &model.ClusterData{
			VolcanoSummary: &model.VolcanoSummary{TotalJobs: 1, TotalQueues: 2},
		}
		assertTopPanelGolden(t, en.volcanoPanelLines(&model.ClusterSummary{}), []string{
			"🌋 Volcano", "", "Jobs:    1", "Running: 0", "Pending: 0", "", "Queues:  2",
		})
	})

	// The five workload/services section headers share the workloads.total_count
	// suffix; the two-space separator and the styled title stay outside the key.
	t.Run("workloads headers", func(t *testing.T) {
		assertHeader := func(t *testing.T, rows []string, want string) {
			t.Helper()
			if len(rows) < 1 {
				t.Fatalf("no rows rendered")
			}
			if rows[0] != want {
				t.Errorf("header row = %q, want %q", rows[0], want)
			}
		}

		en.clusterData = &model.ClusterData{}
		rows, _ := en.renderServicesList(0)
		assertHeader(t, rows, "Services  (Total: 0)")
		en.clusterData = &model.ClusterData{Services: []*model.ServiceData{
			{Name: "a", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.1"},
			{Name: "b", Namespace: "default", Type: "NodePort", ClusterIP: "10.0.0.2"},
			{Name: "c", Namespace: "kube-system", Type: "LoadBalancer", ClusterIP: "10.0.0.3"},
		}}
		rows, _ = en.renderServicesList(0)
		assertHeader(t, rows, "Services  (Total: 3)")

		en.clusterData = &model.ClusterData{}
		rows, _ = en.renderDeploymentsList(0)
		assertHeader(t, rows, "Deployments  (Total: 0)")
		en.clusterData = &model.ClusterData{Deployments: []*model.DeploymentData{
			{Name: "d1", Namespace: "default", Replicas: 2, ReadyReplicas: 2, AvailableReplicas: 2, UpdatedReplicas: 2},
			{Name: "d2", Namespace: "ops", Replicas: 3, ReadyReplicas: 1, AvailableReplicas: 1},
		}}
		rows, _ = en.renderDeploymentsList(0)
		assertHeader(t, rows, "Deployments  (Total: 2)")

		en.clusterData = &model.ClusterData{StatefulSets: []*model.StatefulSetData{
			{Name: "ss1", Namespace: "default", Replicas: 3, ReadyReplicas: 3},
		}}
		rows, _ = en.renderStatefulSetsList(0)
		assertHeader(t, rows, "StatefulSets  (Total: 1)")

		en.clusterData = &model.ClusterData{DaemonSets: []*model.DaemonSetData{
			{Name: "ds1", Namespace: "default", DesiredNumberScheduled: 4, NumberReady: 4},
			{Name: "ds2", Namespace: "ops", DesiredNumberScheduled: 2, NumberReady: 1},
			{Name: "ds3", Namespace: "ops", DesiredNumberScheduled: 2, NumberReady: 2},
			{Name: "ds4", Namespace: "kube-system", DesiredNumberScheduled: 1},
		}}
		rows, _ = en.renderDaemonSetsList(0)
		assertHeader(t, rows, "DaemonSets  (Total: 4)")

		en.clusterData = &model.ClusterData{}
		rows, _ = en.renderCronJobsList(0)
		assertHeader(t, rows, "CronJobs  (Total: 0)")
		en.clusterData = &model.ClusterData{CronJobs: []*model.CronJobData{
			{Name: "cj1", Namespace: "default", Schedule: "*/5 * * * *", Active: 2},
		}}
		rows, _ = en.renderCronJobsList(0)
		assertHeader(t, rows, "CronJobs  (Total: 1)")
	})

	// The cronjob selector switched to the shared detail.workload.* keys; the
	// en rendering must stay exactly what the detail.cronjob.* keys produced.
	t.Run("cronjob selector", func(t *testing.T) {
		empty := strings.Split(en.renderCronJobSelector(&model.CronJobData{
			Name: "cj1", Namespace: "default",
		}), "\n")
		assertTopPanelGolden(t, empty, []string{"Labels", "", "  No labels"})

		full := strings.Split(en.renderCronJobSelector(&model.CronJobData{
			Name: "cj1", Namespace: "default",
			Labels: map[string]string{"app": "web", "tier": "frontend"},
		}), "\n")
		assertTopPanelGolden(t, full, []string{"Labels", "", "  app: web", "  tier: frontend"})
	})
}

func TestOverviewTopPanelsZhSwitch(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	// Capture every panel while the fixtures are in place; the empty-state
	// renders below then mutate the model state.
	en.clusterData = npuEfflowClusterData()
	zh.clusterData = npuEfflowClusterData()
	history := []MetricSnapshot{
		{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 20},
		{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 25},
		{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 28},
	}
	en.metricHistory = history
	zh.metricHistory = history
	zhSummary := npuEfflowSummary()
	nodesSummary := &model.ClusterSummary{TotalNodes: 10, ReadyNodes: 8, NotReadyNodes: 2}
	eventsSummary := &model.ClusterSummary{TotalEvents: 100, WarningEvents: 30, ErrorEvents: 5}

	enPanels := [][]string{
		en.podDetailsLines(zhSummary),
		en.nodesAndPodsLines(nodesSummary),
		en.eventSummaryLines(eventsSummary),
		en.npuPanelLines(zhSummary),
	}
	zhPanels := [][]string{
		zh.podDetailsLines(zhSummary),
		zh.nodesAndPodsLines(nodesSummary),
		zh.eventSummaryLines(eventsSummary),
		zh.npuPanelLines(zhSummary),
	}

	volcanoData := &model.ClusterData{
		VolcanoSummary: &model.VolcanoSummary{
			TotalJobs: 12, RunningJobs: 5, PendingJobs: 3, FailedJobs: 2,
			TotalQueues: 4, NPURequestedByJobs: 24, NPURunningByJobs: 8,
		},
	}
	en.clusterData = volcanoData
	zh.clusterData = volcanoData
	enPanels = append(enPanels, en.volcanoPanelLines(&model.ClusterSummary{NPUCapacity: 64}))
	zhPanels = append(zhPanels, zh.volcanoPanelLines(&model.ClusterSummary{NPUCapacity: 64}))

	// Empty states: Volcano summary absent, no NPU nodes.
	zh.clusterData = &model.ClusterData{}
	emptyStates := strings.Join(zh.volcanoPanelLines(&model.ClusterSummary{}), "\n") + "\n" +
		strings.Join(zh.npuPanelLines(&model.ClusterSummary{}), "\n")

	joined := ""
	for _, p := range zhPanels {
		joined += strings.Join(p, "\n") + "\n"
	}
	joined += emptyStates

	// The zh nouns of every new key must be on screen. The NPU and Volcano
	// headers keep their technical names in zh on purpose, so they are
	// asserted by presence (not by differing from en). Colons are full-width.
	for _, noun := range []string{
		"[📦 Pod]", "[💻 节点]", "[📋 事件]", "🧮 NPU", "🌋 Volcano",
		"总数：", "未就绪：", "警告：", "错误：",
		"已分配：", "使用率：", "效率：", "调度：", "趋势：", "芯片：",
		"Pods: 8 节点", "部署采集器", "以获取芯片指标", "无 NPU 节点",
		"任务：", "等待：", "失败：", "NPU 请求：", "NPU 运行：", "队列：",
		"暂无数据",
	} {
		if !strings.Contains(joined, noun) {
			t.Errorf("zh panels %q do not contain %q", joined, noun)
		}
	}

	// 就绪：/运行： cannot be asserted by bare Contains: the longer labels
	// 未就绪：/NPU 运行： contain them as substrings, so a zh drift of the
	// short labels passed every check (batch-17 verify mutation d). Anchor
	// them to the exact padded label plus the fixture value instead
	// (padRight widths 10 and 9 under the ascii profile).
	for _, anchor := range []string{"就绪：    8", "运行：   5"} {
		if !strings.Contains(joined, anchor) {
			t.Errorf("zh panels %q do not contain %q", joined, anchor)
		}
	}

	// The en wording must be gone everywhere (the reuse keys render their zh
	// forms: 总数：/运行：/等待：/失败： — never the en labels).
	for _, bad := range []string{
		"[📦 Pods]", "[💻 Nodes]", "[📋 Events]",
		"Total:", "(Total:", "Ready:", "NotReady:", "Warning:", "Error:",
		"Alloc:", "Usage:", "Eff:", "Sched:", "Trend:", "Chip:",
		"Jobs:", "Running:", "Pending:", "Failed:", "NPU Req:", "NPU Run:", "Queues:",
		"Not available", "No NPU nodes", "Pods: 8 nodes",
		"Deploy collector", "for chip metrics",
		"{{", "}}", "<no value", "%!", "overview.",
	} {
		if strings.Contains(joined, bad) {
			t.Errorf("zh panels %q contain %q", joined, bad)
		}
	}

	// The same zh panel must differ from its en counterpart line-for-line
	// except where zh deliberately equals en (the two panel headers).
	for p := range enPanels {
		if len(enPanels[p]) != len(zhPanels[p]) {
			t.Fatalf("panel %d: en %d lines, zh %d lines", p, len(enPanels[p]), len(zhPanels[p]))
		}
		for i := range enPanels[p] {
			if enPanels[p][i] == "" {
				continue
			}
			if p >= 3 && i == 0 {
				// NPU/Volcano headers are the same string in both locales.
				continue
			}
			if p >= 3 && !strings.ContainsFunc(enPanels[p][i], func(r rune) bool {
				return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
			}) {
				// Pure symbol lines (progress bar, sparkline) are
				// locale-invariant graphics.
				continue
			}
			if enPanels[p][i] == zhPanels[p][i] {
				t.Errorf("panel %d line %d: zh %q equals the en line — locale did not switch", p, i, zhPanels[p][i])
			}
		}
	}

	// The workload/services headers carry the zh count suffix.
	zh.clusterData = &model.ClusterData{Services: []*model.ServiceData{
		{Name: "a", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.1"},
		{Name: "b", Namespace: "default", Type: "NodePort", ClusterIP: "10.0.0.2"},
		{Name: "c", Namespace: "kube-system", Type: "LoadBalancer", ClusterIP: "10.0.0.3"},
	}}
	rows, _ := zh.renderServicesList(0)
	if !strings.Contains(rows[0], "服务  (共 3 个)") {
		t.Errorf("zh services header %q does not contain the zh count suffix", rows[0])
	}
	if strings.Contains(rows[0], "(Total:") {
		t.Errorf("zh services header %q still contains the en suffix", rows[0])
	}

	zh.clusterData = &model.ClusterData{CronJobs: []*model.CronJobData{
		{Name: "cj1", Namespace: "default", Schedule: "*/5 * * * *", Active: 2},
	}}
	rows, _ = zh.renderCronJobsList(0)
	if !strings.Contains(rows[0], "(共 1 个)") {
		t.Errorf("zh cronjobs header %q does not contain the zh count suffix", rows[0])
	}
}

// TestOverviewTopPanelsZhColumnAlignment: the zh labels go through the same
// padRight call sites as en, so each zh value must sit in the same visual
// column as its en counterpart — except the two Volcano labels that
// legitimately overflow the en width by one column (NPU 请求：/NPU 运行：,
// delta +1). The npu chip label pads to the same 6-column width, so 芯片：
// keeps delta 0 despite being wider than "Chip:".
func TestOverviewTopPanelsZhColumnAlignment(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	zh.clusterData = npuEfflowClusterData()
	en.clusterData = npuEfflowClusterData()
	en.metricHistory = []MetricSnapshot{
		{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 20},
		{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 25},
		{ClusterNPUAllocatable: 64, ClusterNPUAllocated: 28},
	}
	zh.metricHistory = en.metricHistory
	zhSummary := npuEfflowSummary()

	volcanoData := &model.ClusterData{
		VolcanoSummary: &model.VolcanoSummary{
			TotalJobs: 12, RunningJobs: 5, PendingJobs: 3, FailedJobs: 2,
			TotalQueues: 4, NPURequestedByJobs: 24, NPURunningByJobs: 8,
		},
	}
	en.clusterData = volcanoData
	zh.clusterData = volcanoData

	cases := []struct {
		name      string
		enLine    string
		zhLine    string
		wantDelta int
	}{
		// podDetailsLines rows are not listed: they predate this batch and use
		// a single-space "label value" format, not the padRight column layout.
		{"nodes total", en.nodesAndPodsLines(zhSummary)[2], zh.nodesAndPodsLines(zhSummary)[2], 0},
		{"nodes ready", en.nodesAndPodsLines(zhSummary)[3], zh.nodesAndPodsLines(zhSummary)[3], 0},
		{"nodes notready", en.nodesAndPodsLines(zhSummary)[4], zh.nodesAndPodsLines(zhSummary)[4], 0},
		{"events total", en.eventSummaryLines(zhSummary)[2], zh.eventSummaryLines(zhSummary)[2], 0},
		{"events warning", en.eventSummaryLines(zhSummary)[3], zh.eventSummaryLines(zhSummary)[3], 0},
		{"events error", en.eventSummaryLines(zhSummary)[4], zh.eventSummaryLines(zhSummary)[4], 0},
		{"npu total", en.npuPanelLines(zhSummary)[2], zh.npuPanelLines(zhSummary)[2], 0},
		{"npu alloc", en.npuPanelLines(zhSummary)[3], zh.npuPanelLines(zhSummary)[3], 0},
		{"npu usage", en.npuPanelLines(zhSummary)[4], zh.npuPanelLines(zhSummary)[4], 0},
		{"npu eff", en.npuPanelLines(zhSummary)[6], zh.npuPanelLines(zhSummary)[6], 0},
		{"npu sched", en.npuPanelLines(zhSummary)[7], zh.npuPanelLines(zhSummary)[7], 0},
		{"npu chip", en.npuPanelLines(zhSummary)[10], zh.npuPanelLines(zhSummary)[10], 0},
		{"npu superpod nodes", en.npuPanelLines(zhSummary)[11], zh.npuPanelLines(zhSummary)[11], 0},
		{"volcano jobs", en.volcanoPanelLines(volcanoSummary())[2], zh.volcanoPanelLines(volcanoSummary())[2], 0},
		{"volcano running", en.volcanoPanelLines(volcanoSummary())[3], zh.volcanoPanelLines(volcanoSummary())[3], 0},
		{"volcano pending", en.volcanoPanelLines(volcanoSummary())[4], zh.volcanoPanelLines(volcanoSummary())[4], 0},
		{"volcano failed", en.volcanoPanelLines(volcanoSummary())[5], zh.volcanoPanelLines(volcanoSummary())[5], 0},
		{"volcano npu req (zh overflows)", en.volcanoPanelLines(volcanoSummary())[7], zh.volcanoPanelLines(volcanoSummary())[7], 1},
		{"volcano npu run (zh overflows)", en.volcanoPanelLines(volcanoSummary())[8], zh.volcanoPanelLines(volcanoSummary())[8], 1},
		{"volcano queues", en.volcanoPanelLines(volcanoSummary())[10], zh.volcanoPanelLines(volcanoSummary())[10], 0},
	}

	for _, tc := range cases {
		delta := valueColumn(tc.zhLine) - valueColumn(tc.enLine)
		if delta != tc.wantDelta {
			t.Errorf("%s: zh value column offset %d (line %q), want delta %d (en %q)",
				tc.name, delta, tc.zhLine, tc.wantDelta, tc.enLine)
		}
	}
}

// volcanoSummary builds the summary for the volcano alignment fixture.
func volcanoSummary() *model.ClusterSummary {
	return &model.ClusterSummary{NPUCapacity: 64}
}

// readBundleMessages reads a go-i18n TOML message file and flattens dotted
// table headers ([a.b] other = "...") to "a.b" keys, mirroring how go-i18n
// loads these bundles (same pattern as loadTOMLMessages in cmd/k8s-monitor).
func readBundleMessages(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	pluralForms := map[string]bool{
		"zero": true, "one": true, "two": true,
		"few": true, "many": true, "other": true,
	}
	msgs := map[string]string{}
	var flatten func(prefix string, m map[string]interface{})
	flatten = func(prefix string, m map[string]interface{}) {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			sub, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			isMessage := false
			for form := range sub {
				if pluralForms[form] {
					isMessage = true
					break
				}
			}
			if isMessage {
				if other, ok := sub["other"].(string); ok {
					msgs[key] = other
				}
				continue
			}
			flatten(key, sub)
		}
	}
	flatten("", raw)
	return msgs
}

// TestB17DedupGuard: the cronjob detail switched to the byte-identical
// detail.workload.* keys, so the duplicate detail.cronjob.* tables must be
// gone from both bundles, the shared keys must still exist, and the new
// workloads.total_count key must exist in both locales with en/zh key-set
// parity intact.
func TestB17DedupGuard(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	for name, msgs := range map[string]map[string]string{"en": en, "zh": zh} {
		for _, id := range []string{"detail.cronjob.labels_header", "detail.cronjob.no_labels"} {
			if v, ok := msgs[id]; ok {
				t.Errorf("%s bundle still defines deduplicated %s = %q", name, id, v)
			}
		}
		if _, ok := msgs["workloads.total_count"]; !ok {
			t.Errorf("%s bundle is missing workloads.total_count", name)
		}
	}

	if got := en["workloads.total_count"]; got != "(Total: {{.Count}})" {
		t.Errorf("en workloads.total_count = %q", got)
	}
	if got := zh["workloads.total_count"]; got != "(共 {{.Count}} 个)" {
		t.Errorf("zh workloads.total_count = %q", got)
	}

	// The surviving shared keys the cronjob detail now uses.
	if got := en["detail.workload.labels_header"]; got != "Labels" {
		t.Errorf("en detail.workload.labels_header = %q", got)
	}
	if got := en["detail.workload.no_labels"]; got != "  No labels" {
		t.Errorf("en detail.workload.no_labels = %q", got)
	}
	if got := zh["detail.workload.labels_header"]; got != "标签" {
		t.Errorf("zh detail.workload.labels_header = %q", got)
	}
	if got := zh["detail.workload.no_labels"]; got != "  无标签" {
		t.Errorf("zh detail.workload.no_labels = %q", got)
	}

	// Key-set parity between the locales.
	var onlyEn, onlyZh []string
	for id := range en {
		if _, ok := zh[id]; !ok {
			onlyEn = append(onlyEn, id)
		}
	}
	for id := range zh {
		if _, ok := en[id]; !ok {
			onlyZh = append(onlyZh, id)
		}
	}
	if len(onlyEn) > 0 || len(onlyZh) > 0 {
		t.Errorf("en/zh key set mismatch: only-en %v, only-zh %v", onlyEn, onlyZh)
	}
}
