package ui

// b29v_width_test.go — independent adversarial probe for batch 29's width
// guarantees: the overview NPU card's 18-cell content budget (including the zh
// labels, which are wider than the en ones), the cluster-load box's yield rule,
// and the new i18n keys' placeholder parity.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29vCardData is a chip-bearing NPU cluster whose summary carries everything
// the overview card reads.
func b29vCardData(aiCore, temp int, power float64) *model.ClusterData {
	chips := make([]model.NPUChipData, 32) // 16 per node, two nodes
	for i := range chips {
		chips[i] = model.NPUChipData{
			NPUID: (i % 16) / 2, Chip: i % 2, PhyID: i % 16, Health: "OK",
			AICore: aiCore, Temp: temp, Power: power, HBMUsed: 0, HBMTotal: 65536,
		}
	}
	nodeA := b29vChipSet("node-a", chips[:16])
	nodeB := b29vChipSet("node-b", chips[16:])
	data := &model.ClusterData{
		Nodes: []*model.NodeData{nodeA, nodeB},
		Summary: &model.ClusterSummary{
			NPUCapacity: 32, NPUAllocatable: 32, NPUAllocated: 8, NPUUtilization: 25,
			NPUNodesCount: 2, NPUChipType: "Ascend910B",
		},
	}
	return data
}

func b29vCardModel(locale string, data *model.ClusterData) *Model {
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width, m.height = 200, 90
	m.currentView = ViewOverview
	m.clusterData = data
	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		m.vizHistory = append(m.vizHistory, m.buildVizSample(data, base.Add(time.Duration(i)*2*time.Second)))
	}
	return m
}

// TestB29VCardFitsContentBudgetWorstCase repeats the budget probe at the widest
// values the fixtures allow, for both locales.
func TestB29VCardFitsContentBudgetWorstCase(t *testing.T) {
	forcedAsciiProfile(t)
	for _, locale := range []string{"en", "zh"} {
		data := b29vCardData(99, 108, 999.9) // 32 × 999.9 W = 32.0 kW
		data.Nodes[0].NPUChipHealth[3].Grade = model.NPUChipGradeDegraded
		data.Nodes[0].NPUChipHealth[7].Grade = model.NPUChipGradeFault
		m := b29vCardModel(locale, data)
		for i, line := range m.npuPanelLines(data.Summary) {
			if got := visualLength(line); got > summaryPanelContent {
				t.Errorf("%s: card line %d is %d cells wide, budget %d: %q",
					locale, i, got, summaryPanelContent, stripANSI(line))
			}
		}
	}
}

// TestB29VLoadBoxSparkYieldsOnLongLine pins the yield rule with a real
// cluster-load line longer than the box budget: the sparkline is dropped whole
// and the line keeps exactly the bytes it had before the batch.
func TestB29VLoadBoxSparkYieldsOnLongLine(t *testing.T) {
	forcedAsciiProfile(t)
	summary := &model.ClusterSummary{
		TotalNodes: 1, ReadyNodes: 1,
		CPUCapacity: 123456789, CPUAllocatable: 123456789, CPUUsed: 1234567, CPUUsageUtilization: 1.0,
		MemoryCapacity: 4 << 30, MemoryAllocatable: 4 << 30, MemoryUsed: 1 << 30, MemUsageUtilization: 25,
		KubeletMetricsAvailable: true, NodesWithMetrics: 1,
	}
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{{Name: "n", Status: "Ready", CPUAllocatable: 123456789, CPUUsage: 1234567}},
		Summary: summary,
	}
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 200, 80
	m.clusterData = data
	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		m.vizHistory = append(m.vizHistory, m.buildVizSample(data, base.Add(time.Duration(i)*2*time.Second)))
	}

	box := stripANSI(m.renderClusterLoadCompact(summary))
	lines := strings.Split(box, "\n")
	if len(lines) != 9 {
		t.Errorf("the cluster-load box has %d lines, want 9 (no row wrapped):\n%s", len(lines), box)
	}
	cpuLine := ""
	for _, line := range lines {
		if strings.Contains(line, m.T("metrics.cpu_used")) {
			cpuLine = line
		}
	}
	if cpuLine == "" {
		t.Fatalf("no CPU line in:\n%s", box)
	}
	want := fmt.Sprintf("%s %s / %s %s (%s) %s",
		m.T("metrics.cpu_used"), formatCPU(summary.CPUUsed), formatCPU(summary.CPUCapacity),
		m.T("metrics.capacity"), formatPercentage(summary.CPUUsageUtilization),
		renderProgressBar(summary.CPUUsageUtilization, 40))
	want = stripANSI(want)
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(cpuLine, "│"), "│"))
	if inner != want {
		t.Errorf("the CPU line changed shape when the sparkline did not fit:\n got %q\nwant %q", inner, want)
	}
	for _, r := range cpuLine {
		if strings.ContainsRune(sparkRamp, r) {
			t.Errorf("the CPU line kept a sparkline glyph although it does not fit: %q", cpuLine)
			break
		}
	}

	// Non-vacuousness: the short memory line beside it does carry a sparkline.
	memLine := ""
	for _, line := range lines {
		if strings.Contains(line, m.T("metrics.mem_used")) {
			memLine = line
		}
	}
	if !strings.ContainsAny(memLine, sparkRamp) {
		t.Errorf("the memory line has no sparkline although the series exists: %q", memLine)
	}
}

// TestB29VLoadBoxWidthIgnoresTheTerminal records the box's actual width policy:
// it is drawn at a fixed width, so the sparkline's presence depends on the
// line's own budget and not on the terminal. Batch 29's design note speaks of
// the sparkline yielding on a narrow terminal; that is true only relative to
// the box budget, and this probe pins what actually happens.
func TestB29VLoadBoxWidthIgnoresTheTerminal(t *testing.T) {
	forcedAsciiProfile(t)
	summary := &model.ClusterSummary{
		TotalNodes: 1, ReadyNodes: 1,
		CPUCapacity: 64000, CPUAllocatable: 64000, CPUUsed: 1600, CPUUsageUtilization: 2.5,
		MemoryCapacity: 1 << 40, MemoryAllocatable: 1 << 40, MemoryUsed: 1 << 39, MemUsageUtilization: 50,
		KubeletMetricsAvailable: true, NodesWithMetrics: 1,
	}
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{{Name: "n", Status: "Ready", CPUAllocatable: 64000, CPUUsage: 1600}},
		Summary: summary,
	}
	boxes := map[int]string{}
	for _, width := range []int{60, 80, 160} {
		m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
		m.width, m.height = width, 80
		m.clusterData = data
		base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
		for i := 0; i < 6; i++ {
			m.vizHistory = append(m.vizHistory, m.buildVizSample(data, base.Add(time.Duration(i)*2*time.Second)))
		}
		boxes[width] = m.renderClusterLoadCompact(summary)
	}
	if boxes[60] != boxes[160] {
		t.Errorf("the load box depends on the terminal width:\n60 : %q\n160: %q", boxes[60], boxes[160])
	}
	for _, line := range strings.Split(stripANSI(boxes[60]), "\n") {
		if got := runewidth.StringWidth(line); got > clusterLoadBoxWidth+2 {
			t.Errorf("a box line is %d cells wide, over the box width %d: %q", got, clusterLoadBoxWidth+2, line)
		}
	}
}

// TestB29VNewI18nKeysPlaceholderParity pins the batch's 14 new keys: present in
// both bundles, and carrying the same placeholder set so one locale cannot drop
// a value the other prints.
func TestB29VNewI18nKeysPlaceholderParity(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	newKeys := []string{
		"npuview.aggregate_nodes", "npuview.aggregate_health", "npuview.aggregate_power",
		"npuview.aggregate_temp", "npuview.aggregate_hbm", "npuview.aggregate_util",
		"npuview.mode_aicore", "npuview.mode_temp", "npuview.mode_power", "npuview.heat_legend",
		"overview.npu_chip_health", "overview.npu_chip_util",
		"overview.npu_temp_power", "overview.npu_temp_only",
	}
	placeholder := regexp.MustCompile(`\{\{\.[A-Za-z]+\}\}`)
	placeholders := func(s string) []string {
		out := placeholder.FindAllString(s, -1)
		sort.Strings(out)
		return out
	}
	for _, key := range newKeys {
		enMsg, okEn := en[key]
		zhMsg, okZh := zh[key]
		if !okEn || !okZh {
			t.Errorf("%s missing (en=%v zh=%v)", key, okEn, okZh)
			continue
		}
		if strings.TrimSpace(enMsg) == "" || strings.TrimSpace(zhMsg) == "" {
			t.Errorf("%s has an empty message (%q / %q)", key, enMsg, zhMsg)
		}
		if got, want := placeholders(zhMsg), placeholders(enMsg); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s placeholder mismatch: en %v, zh %v", key, want, got)
		}
	}
}
