package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The three bottom summary panels on the overview render their labels through
// the localizer and pad them with padRight, so the English output must stay
// byte-identical to the previously hardcoded strings (golden) and the Chinese
// output must actually switch while keeping the value columns aligned.
// forcedAsciiProfile (compact_alerts_test.go) makes the goldens plain text;
// only the StyleHeader panel titles carry layout padding, so every non-header
// line is compared byte-exact without TrimSpace.
func TestOverviewPanelsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	// assertPanelGolden compares the header line with TrimSpace (StyleHeader
	// pads by one space on each side) and every other line byte-exact.
	assertPanelGolden := func(t *testing.T, lines, want []string) {
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

	t.Run("services", func(t *testing.T) {
		summary := &model.ClusterSummary{
			TotalServices:      12,
			ClusterIPServices:  7,
			NodePortServices:   3,
			LoadBalancerSvcs:   2,
			NoEndpointServices: 1,
		}
		assertPanelGolden(t, en.servicesPanelLines(summary), []string{
			"🔌 Services",
			"",
			"Total:   12",
			"ClustIP: 7",
			"NodePt:  3",
			"LoadBal: 2",
			"",
			"NoEndpt: 1",
		})
		assertPanelGolden(t, en.servicesPanelLines(&model.ClusterSummary{}), []string{
			"🔌 Services", "", "Total:   0", "ClustIP: 0", "NodePt:  0", "LoadBal: 0",
		})
	})

	t.Run("storage", func(t *testing.T) {
		summary := &model.ClusterSummary{
			TotalPVs:            5,
			TotalStorageSize:    50 * 1024 * 1024 * 1024,
			BoundPVs:            4,
			StorageUsagePercent: 62.5,
			TotalPVCs:           8,
			BoundPVCs:           6,
			PendingPVCs:         2,
		}
		assertPanelGolden(t, en.storagePanelLines(summary), []string{
			"💾 Storage",
			"",
			"PVs:  5 (50.0Gi)",
			"Bound: 4",
			"Used: 62.5%",
			"",
			"PVCs: 8",
			"Bound: 6",
			"Pend: 2",
		})
		assertPanelGolden(t, en.storagePanelLines(&model.ClusterSummary{}), []string{
			"💾 Storage", "", "No PVs", "No PVCs",
		})
		// No usage percent and no pending PVCs: those lines stay absent.
		assertPanelGolden(t, en.storagePanelLines(&model.ClusterSummary{
			TotalPVs:         5,
			TotalStorageSize: 10 * 1024 * 1024 * 1024,
			BoundPVs:         5,
			TotalPVCs:        8,
			BoundPVCs:        8,
		}), []string{
			"💾 Storage", "", "PVs:  5 (10.0Gi)", "Bound: 5", "", "PVCs: 8", "Bound: 8",
		})
	})

	t.Run("workloads", func(t *testing.T) {
		summary := &model.ClusterSummary{
			TotalDeployments:  11,
			TotalStatefulSets: 22,
			TotalDaemonSets:   33,
			TotalJobs:         44,
			TotalCronJobs:     55,
		}
		assertPanelGolden(t, en.workloadsPanelLines(summary), []string{
			"📋 Workloads",
			"",
			"Deploy:  11",
			"StatSet: 22",
			"DaemSet: 33",
			"Jobs:    44",
			"Cron:    55",
		})
		assertPanelGolden(t, en.workloadsPanelLines(&model.ClusterSummary{}), []string{
			"📋 Workloads", "", "Limited detection", "(based on labels)",
		})
	})
}

func TestOverviewPanelsZh(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	summary := &model.ClusterSummary{
		TotalServices:       12,
		ClusterIPServices:   7,
		NodePortServices:    3,
		LoadBalancerSvcs:    2,
		NoEndpointServices:  1,
		TotalPVs:            5,
		TotalStorageSize:    50 * 1024 * 1024 * 1024,
		BoundPVs:            4,
		StorageUsagePercent: 62.5,
		TotalPVCs:           8,
		BoundPVCs:           6,
		PendingPVCs:         2,
		TotalDeployments:    11,
		TotalStatefulSets:   22,
		TotalDaemonSets:     33,
		TotalJobs:           44,
		TotalCronJobs:       55,
	}

	panels := map[string][]string{
		"services":  zh.servicesPanelLines(summary),
		"storage":   zh.storagePanelLines(summary),
		"workloads": zh.workloadsPanelLines(summary),
	}
	enPanels := map[string][]string{
		"services":  en.servicesPanelLines(summary),
		"storage":   en.storagePanelLines(summary),
		"workloads": en.workloadsPanelLines(summary),
	}

	for name, lines := range panels {
		enLines := enPanels[name]
		if len(lines) != len(enLines) {
			t.Errorf("zh %s panel has %d lines, en has %d", name, len(lines), len(enLines))
			continue
		}
		for i := range lines {
			if lines[i] == "" {
				continue
			}
			if strings.TrimSpace(lines[i]) == strings.TrimSpace(enLines[i]) {
				t.Errorf("zh %s line %d %q equals the en line — locale did not switch", name, i, lines[i])
			}
		}
	}

	joined := strings.Join(panels["services"], "\n") + "\n" +
		strings.Join(panels["storage"], "\n") + "\n" +
		strings.Join(panels["workloads"], "\n")

	for _, noun := range []string{
		"工作负载", "服务", "存储",
		"部署：", "有状态：", "守护：", "任务：", "定时：",
		"总数：", "集群 IP：", "节点端口：", "负载均衡：", "无端点：",
		"PV：", "绑定：", "已用：", "PVC：", "等待：",
	} {
		if !strings.Contains(joined, noun) {
			t.Errorf("zh panels %q do not contain %q", joined, noun)
		}
	}
	for _, bad := range []string{
		"Workloads", "Services", "Storage",
		"Deploy:", "StatSet:", "DaemSet:", "Jobs:", "Cron:",
		"Total:", "ClustIP:", "NodePt:", "LoadBal:", "NoEndpt:",
		"Bound:", "Used:", "Pend:", "PVs:", "PVCs:",
		"Limited detection", "based on labels", "No PVs", "No PVCs",
		"{{", "}}", "<no value", "%!", "overview.",
	} {
		if strings.Contains(joined, bad) {
			t.Errorf("zh panels %q contain %q", joined, bad)
		}
	}

	// Zero-value branches switch too.
	zhZero := strings.Join(zh.workloadsPanelLines(&model.ClusterSummary{}), "\n") + "\n" +
		strings.Join(zh.storagePanelLines(&model.ClusterSummary{}), "\n")
	for _, noun := range []string{"检测受限", "（基于标签）", "暂无 PV", "暂无 PVC"} {
		if !strings.Contains(zhZero, noun) {
			t.Errorf("zh zero panels %q do not contain %q", zhZero, noun)
		}
	}
	for _, bad := range []string{"Limited detection", "based on labels", "No PVs", "No PVCs"} {
		if strings.Contains(zhZero, bad) {
			t.Errorf("zh zero panels %q still contain %q", zhZero, bad)
		}
	}
}

// valueColumn returns the visual column at which the first ASCII digit (the
// value) appears in the line, or -1.
func valueColumn(line string) int {
	col := 0
	for _, r := range stripANSI(line) {
		if r >= '0' && r <= '9' {
			return col
		}
		col += runewidth.RuneWidth(r)
	}
	return -1
}

// TestOverviewPanelsZhColumnAlignment: the zh labels go through the same
// padRight call sites as en, so each zh value must sit in the same visual
// column as its en counterpart — except the two labels that legitimately
// overflow the en width by one column (节点端口：/负载均衡：, delta +1).
func TestOverviewPanelsZhColumnAlignment(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	summary := &model.ClusterSummary{
		TotalServices:      12,
		ClusterIPServices:  7,
		NodePortServices:   3,
		LoadBalancerSvcs:   2,
		TotalPVs:           5,
		TotalStorageSize:   50 * 1024 * 1024 * 1024,
		BoundPVs:           4,
		TotalPVCs:          8,
		BoundPVCs:          6,
		TotalDeployments:   11,
		TotalStatefulSets:  22,
		TotalDaemonSets:    33,
		TotalJobs:          44,
		TotalCronJobs:      55,
		NoEndpointServices: 1,
	}

	cases := []struct {
		name      string
		enLine    string
		zhLine    string
		wantDelta int
	}{
		{"total", en.servicesPanelLines(summary)[2], zh.servicesPanelLines(summary)[2], 0},
		{"clustip", en.servicesPanelLines(summary)[3], zh.servicesPanelLines(summary)[3], 0},
		{"nodeport (zh overflows)", en.servicesPanelLines(summary)[4], zh.servicesPanelLines(summary)[4], 1},
		{"loadbal (zh overflows)", en.servicesPanelLines(summary)[5], zh.servicesPanelLines(summary)[5], 1},
		{"deploy", en.workloadsPanelLines(summary)[2], zh.workloadsPanelLines(summary)[2], 0},
		{"statset", en.workloadsPanelLines(summary)[3], zh.workloadsPanelLines(summary)[3], 0},
		{"daemset", en.workloadsPanelLines(summary)[4], zh.workloadsPanelLines(summary)[4], 0},
		{"jobs", en.workloadsPanelLines(summary)[5], zh.workloadsPanelLines(summary)[5], 0},
		{"cron", en.workloadsPanelLines(summary)[6], zh.workloadsPanelLines(summary)[6], 0},
		{"pvs", en.storagePanelLines(summary)[2], zh.storagePanelLines(summary)[2], 0},
		{"bound pv", en.storagePanelLines(summary)[3], zh.storagePanelLines(summary)[3], 0},
		{"pvcs", en.storagePanelLines(summary)[5], zh.storagePanelLines(summary)[5], 0},
		{"bound pvc", en.storagePanelLines(summary)[6], zh.storagePanelLines(summary)[6], 0},
		{"noendpt", en.servicesPanelLines(summary)[7], zh.servicesPanelLines(summary)[7], 0},
	}

	for _, tc := range cases {
		delta := valueColumn(tc.zhLine) - valueColumn(tc.enLine)
		if delta != tc.wantDelta {
			t.Errorf("%s: zh value column offset %d (line %q), want delta %d (en %q)",
				tc.name, delta, tc.zhLine, tc.wantDelta, tc.enLine)
		}
	}
}
