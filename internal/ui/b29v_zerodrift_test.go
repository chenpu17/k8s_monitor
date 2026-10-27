package ui

// b29v_zerodrift_test.go — the verifier's anchor for batch 29's hard line: in
// the health matrix mode the NPU view must be the pre-batch view plus the
// aggregate bar and nothing else.
//
// The evidence behind the frozen block: the same fixture was rendered through a
// detached HEAD worktree (git worktree at e2867f1) and through the batch tree,
// at en/zh × widths 38/60/100/160. The diff between the two dumps contained 0
// removed lines and 57 added lines, every one of them an aggregate-bar line
// (plus batch 29's card rows in the overview sections); no matrix row, summary
// segment or alert line moved. The frozen document here is the width-100 shape
// of that comparison, so a future change that moves the bar, reorders the rows,
// or drifts a summary segment fails with the exact line.
//
// Batch 30 made AI Core the default and added the always-on mode legend, so the
// health mode is now selected explicitly and the document gains exactly one
// line — the health legend — between the aggregate bar and the static glyph
// legend. Every pre-batch line keeps its bytes.

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29vZeroDriftData is two NPU nodes — one idle and healthy, one with a
// degraded and a faulted chip plus an occupied chip — and a third NPU node
// whose exporter never answered.
func b29vZeroDriftData() *model.ClusterData {
	idle := b29vHealthyNode("npu-idle", 8)
	mixed := b29vHealthyNode("npu-mixed", 8)
	mixed.NPUChipHealth[2].Grade = model.NPUChipGradeDegraded
	mixed.NPUChipHealth[5].Grade = model.NPUChipGradeFault
	mixed.NPUChips[0].OwnerPod = "train-0"
	mixed.NPUChips[1].OwnerPod = "train-0"
	noData := &model.NodeData{Name: "npu-silent", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8}
	return &model.ClusterData{
		Nodes: []*model.NodeData{idle, mixed, noData},
		Summary: &model.ClusterSummary{
			NPUCapacity: 24, NPUAllocatable: 24, NPUAllocated: 6, NPUUtilization: 25,
			NPUNodesCount: 3,
			Alerts: []model.Alert{
				{AlertType: model.AlertTypeNPUChipFault, Severity: model.AlertSeverityCritical,
					ResourceName: "npu-mixed", Message: "chip 5 fault",
					Timestamp: time.Date(2026, 10, 26, 13, 0, 5, 0, time.UTC)},
			},
		},
	}
}

func TestB29VHealthModeDocumentIsPreBatchPlusAggregateBar(t *testing.T) {
	forcedAsciiProfile(t)
	want := map[string][]string{
		"en": {
			" 🧮 NPU Overview ",
			"",
			"14/16 healthy · 1 degraded · 1 fault  •  Allocation 6/24 (25.0%)  •  Monitoring 2/3 nodes",
			"Nodes 3  •  Chips 14/16 healthy  •  Power 3040W  •  Avg temp 46°C  •  Utilization 0%",
			"Mode: Chip health (h to switch)",
			"▉ healthy  ▒ degraded  ▁ fault",
			"",
			"> npu-idle  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
			"  npu-mixed  ▉ ▉  ▒ ▉  ▉ ▁  ▉ ▉  1 degraded • 1 fault • 2 occupied",
			"  npu-silent  monitoring unavailable",
			"",
			"NPU Alerts",
			"chip fault: 1",
			"🔴 Critical  npu-mixed  chip 5 fault",
		},
		"zh": {
			" 🧮 NPU 总览 ",
			"",
			"14/16 健康 · 1 降级 · 1 故障  •  分配 6/24（25.0%）  •  监控 2/3 节点",
			"节点 3  •  芯片 14/16 健康  •  功耗 3040W  •  均温 46°C  •  利用率 0%",
			"模式：芯片健康（h 切换）",
			"▉ 健康  ▒ 降级  ▁ 故障",
			"",
			"> npu-idle  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
			"  npu-mixed  ▉ ▉  ▒ ▉  ▉ ▁  ▉ ▉  1 降级 • 1 故障 • 2 占用",
			"  npu-silent  监控不可用",
			"",
			"NPU 告警",
			"芯片故障: 1",
			"🔴 严重  npu-mixed  chip 5 fault",
		},
	}

	for _, locale := range []string{"en", "zh"} {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
		m.width, m.height = 100, 90
		m.currentView = ViewNPU
		m.clusterData = b29vZeroDriftData()
		// Batch 30: AI Core is the default; this anchor is the health mode.
		m.npuMatrixMode = npuMatrixModeHealth

		got := strings.Split(stripANSI(m.renderNPUView()), "\n")
		if len(got) != len(want[locale]) {
			t.Fatalf("%s: the document has %d lines, want the pre-batch %d + 2 (aggregate bar, mode legend):\n%q",
				locale, len(got), len(want[locale])-2, got)
		}
		for i := range want[locale] {
			if got[i] != want[locale][i] {
				t.Errorf("%s line %d drifted:\n got %q\nwant %q", locale, i, got[i], want[locale][i])
			}
		}
	}
}
