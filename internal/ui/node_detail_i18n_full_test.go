package ui

import (
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
	"go.uber.org/zap"
)

// Node detail full i18n lockdown (batch 13): the remaining hardcoded English
// labels (Roles, NPU, the Link Up/Down cell values, the net-mapping ID header
// and the traffic Total label) must render byte-identically in English and
// switch in Chinese. The colour profile is forced to Ascii so the goldens are
// plain text whatever the ambient terminal detection is.

// sp returns n spaces, keeping the golden arithmetic readable.
func sp(n int) string { return strings.Repeat(" ", n) }

// renderBasicInfoAt renders renderNodeBasicInfo for a node at a fixed size.
func renderBasicInfoAt(t *testing.T, locale string, node *model.NodeData) string {
	t.Helper()
	m := NewModel(nil, zap.NewNop(), 0, locale, "test", 200)
	m.width, m.height = 120, 200
	return m.renderNodeBasicInfo(node)
}

func basicInfoNode() *model.NodeData {
	return &model.NodeData{
		Name:           "node-a",
		Status:         "Ready",
		Roles:          []string{"control-plane", "master"},
		InternalIP:     "10.0.0.1",
		ExternalIP:     "203.0.113.5",
		CPUAllocatable: 4000,
	}
}

func TestRenderNodeBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	got := renderBasicInfoAt(t, "en", basicInfoNode())
	want := " 📋 Basic Information \n" +
		"\n" +
		"  Name: node-a\n" +
		"  Roles: control-plane, master\n" +
		"  Status: Ready\n" +
		"  Internal IP: 10.0.0.1\n" +
		"  External IP: 203.0.113.5"
	if got != want {
		t.Errorf("en basic info:\n got %q\nwant %q", got, want)
	}
}

func TestRenderNodeBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	got := renderBasicInfoAt(t, "zh", basicInfoNode())
	want := " 📋 基本信息 \n" +
		"\n" +
		"  名称: node-a\n" +
		"  角色: control-plane, master\n" +
		"  状态: Ready\n" +
		"  内部 IP: 10.0.0.1\n" +
		"  外部 IP: 203.0.113.5"
	if got != want {
		t.Errorf("zh basic info:\n got %q\nwant %q", got, want)
	}
	for _, bad := range []string{
		"Roles", "Basic Information", "Name:", "Status:", "Internal IP",
		"External IP", "{{", "}}", "<no value", "%!", "detail.", "field.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

// resourceInfoNode carries CPU/memory/pods plus an NPU block without chip
// metrics, so the NPU allocatable row and the no-exporter hint both render.
func resourceInfoNode() *model.NodeData {
	return &model.NodeData{
		Name:           "node-r",
		Status:         "Ready",
		CPUAllocatable: 4000,
		MemAllocatable: 16 * 1024 * 1024 * 1024,
		PodAllocatable: 110,
		PodCount:       10,
		NPUCapacity:    8,
		NPUAllocatable: 8,
		NPUAllocated:   4,
	}
}

func renderResourceInfoAt(t *testing.T, locale string, node *model.NodeData) string {
	t.Helper()
	m := NewModel(nil, zap.NewNop(), 0, locale, "test", 200)
	m.width, m.height = 120, 200
	return m.renderNodeResourceInfo(node)
}

func TestRenderNodeResourceInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	got := renderResourceInfoAt(t, "en", resourceInfoNode())
	// CPUUsage/MemoryUsage are zero, so usage and percent show the N/A
	// placeholders (kept untranslated on purpose, like "CPU").
	want := " 📊 Resource Information \n" +
		"\n" +
		"  CPU: N/A / 4.00 (N/A)\n" +
		"  Memory: N/A / 16.0 GiB (N/A)\n" +
		"  Pods: 10 / 110 (9.1%)\n" +
		"  NPU: 4 / 8 (50.0%)\n" +
		"\n" +
		"  NPU Details\n" +
		"\n" +
		"  NPU Runtime Metrics: Not available\n" +
		"    NPU-Exporter not reachable\n" +
		"    or not deployed in kube-system namespace\n" +
		"    Use --npu-exporter flag to specify a custom endpoint\n" +
		"\n" +
		"  Chip Health  (device plugin bitmap)\n" +
		"    Not reported"
	if got != want {
		t.Errorf("en resource info:\n got %q\nwant %q", got, want)
	}
}

func TestRenderNodeResourceInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	got := renderResourceInfoAt(t, "zh", resourceInfoNode())
	for _, noun := range []string{
		"📊 资源信息", "内存", "NPU 详情", "芯片健康", "设备插件位图",
		"状态未上报", "NPU-Exporter 不可达",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh resource info %q does not contain %q", got, noun)
		}
	}
	// "N/A" (metric placeholder), "NPU" and "Pods" stay untranslated on
	// purpose (same policy as detail.field.cpu); everything else must switch.
	if !strings.Contains(got, "N/A") {
		t.Errorf("zh resource info %q must keep the N/A placeholder", got)
	}
	for _, bad := range []string{
		"Resource Information", "Memory", "NPU Runtime Metrics",
		"NPU-Exporter not reachable", "Chip Health", "Not reported",
		"{{", "}}", "<no value", "%!", "npudetail.", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh resource info %q contains %q", got, bad)
		}
	}
}

// renderNodeDetailAt renders the full node detail view at a geometry+locale.
func renderNodeDetailAt(t *testing.T, locale string, node *model.NodeData, width int) string {
	t.Helper()
	m := NewModel(nil, zap.NewNop(), 0, locale, "test", 200)
	m.width, m.height = width, 200
	m.selectedNode = node
	return stripANSI(m.renderNodeDetail())
}

// chipHeaderEn/zh are the extended chip table header rows. Every zh title fits
// its fixed column width, so the 72-cell row width is locale independent.
func chipHeaderEn() string {
	return "    " +
		"ID" + sp(1) + " " +
		"AICore" + sp(1) + " " +
		"Vector" + sp(1) + " " +
		"HBM" + sp(11) + " " +
		"Temp" + sp(2) + " " +
		"Power" + sp(1) + " " +
		"Volt" + sp(2) + " " +
		"Freq" + sp(3) + " " +
		"Link"
}

func chipHeaderZh() string {
	return "    " +
		"ID" + sp(1) + " " +
		"AI核" + sp(3) + " " +
		"向量" + sp(3) + " " +
		"HBM" + sp(11) + " " +
		"温度" + sp(2) + " " +
		"功耗" + sp(2) + " " +
		"电压" + sp(2) + " " +
		"频率" + sp(3) + " " +
		"链路"
}

func TestRenderNodeChipLinkEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	out := renderNodeDetailAt(t, "en", extendedChipNode(), 120)
	block := blockAfter(out, "NPU Chip Details")
	if len(block) != 5 {
		t.Fatalf("extended chip table has %d lines, want 5\n---\n%s", len(block), out)
	}
	if block[1] != chipHeaderEn() {
		t.Errorf("en chip header:\n got %q\nwant %q", block[1], chipHeaderEn())
	}
	// The Link cell is the last column: "Up" (LinkStatus=1) on chip 0, "Down"
	// on chip 1, each padded to the 4-cell column.
	if !strings.HasSuffix(block[3], "Up"+sp(2)) {
		t.Errorf("en chip 0 row must end with an Up cell: %q", block[3])
	}
	if !strings.HasSuffix(block[4], "Down") {
		t.Errorf("en chip 1 row must end with a Down cell: %q", block[4])
	}

	// Mid tier (width 62): the Link column is dropped with its values.
	mid := renderNodeDetailAt(t, "en", extendedChipNode(), 62)
	midBlock := blockAfter(mid, "NPU Chip Details")
	for _, line := range midBlock {
		if strings.Contains(line, "Up") || strings.Contains(line, "Down") {
			t.Errorf("mid tier must drop the Link column: %q", line)
		}
	}
}

func TestRenderNodeChipLinkZh(t *testing.T) {
	forcedAsciiProfile(t)

	out := renderNodeDetailAt(t, "zh", extendedChipNode(), 120)
	block := blockAfter(out, "NPU 芯片详情")
	if len(block) != 5 {
		t.Fatalf("extended chip table has %d lines, want 5\n---\n%s", len(block), out)
	}
	if block[1] != chipHeaderZh() {
		t.Errorf("zh chip header:\n got %q\nwant %q", block[1], chipHeaderZh())
	}
	// 在线/离线 are 4 display cells wide, exactly the Link column width, so
	// the row stays at 72 cells and no padding is appended.
	if !strings.HasSuffix(block[3], "在线") {
		t.Errorf("zh chip 0 row must end with 在线: %q", block[3])
	}
	if !strings.HasSuffix(block[4], "离线") {
		t.Errorf("zh chip 1 row must end with 离线: %q", block[4])
	}
	for i, line := range block[1:] {
		if w := runewidth.StringWidth(line); w != 72 {
			t.Errorf("zh chip table line %d is %d cells wide, want 72: %q", i+1, w, line)
		}
	}
	for _, bad := range []string{"Up", "Down", "npudetail.", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(strings.Join(block, "\n"), bad) {
			t.Errorf("zh chip table still contains %q:\n%s", bad, strings.Join(block, "\n"))
		}
	}

	// Mid tier (width 62): the Link column is dropped with its zh values, and
	// the surviving rows shrink to the 59-cell tier width.
	mid := renderNodeDetailAt(t, "zh", extendedChipNode(), 62)
	midBlock := blockAfter(mid, "NPU 芯片详情")
	if len(midBlock) != 5 {
		t.Fatalf("mid-tier chip table has %d lines, want 5\n---\n%s", len(midBlock), mid)
	}
	for _, line := range midBlock[1:] {
		if strings.Contains(line, "在线") || strings.Contains(line, "离线") {
			t.Errorf("mid tier must drop the Link column: %q", line)
		}
		if w := runewidth.StringWidth(line); w != 59 {
			t.Errorf("mid-tier line is %d cells wide, want 59: %q", w, line)
		}
	}
}

// netMappingHeaderEn/zh are the NPU network mapping header rows (fixed column
// widths 4/16/16/9, last column unpadded).
func netMappingHeaderEn() string {
	return "    " +
		"ID" + sp(2) + " " +
		"DEVICE IP" + sp(7) + " " +
		"TOR SWITCH" + sp(6) + " " +
		"TOR PORT"
}

func netMappingHeaderZh() string {
	return "    " +
		"ID" + sp(2) + " " +
		"设备IP" + sp(10) + " " +
		"TOR交换机" + sp(7) + " " +
		"TOR端口"
}

func netMappingNode() *model.NodeData {
	return &model.NodeData{
		Name: "node-net", Status: "Ready",
		NPUCapacity: 1, NPUAllocatable: 1,
		NPUStatusKnown: true,
		NPUDeviceNet: []model.NPUDeviceNetEntry{
			{DeviceID: 0, DeviceIP: "29.11.0.10", TORIP: "29.79.161.163", TORPort: "17:1"},
		},
	}
}

func TestRenderNodeNetMappingHeaderEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	out := renderNodeDetailAt(t, "en", netMappingNode(), 120)
	block := blockAfter(out, "NPU Network Mapping")
	if len(block) < 3 {
		t.Fatalf("network mapping block too short:\n%s", out)
	}
	if block[1] != netMappingHeaderEn() {
		t.Errorf("en net mapping header:\n got %q\nwant %q", block[1], netMappingHeaderEn())
	}
}

func TestRenderNodeNetMappingHeaderZh(t *testing.T) {
	forcedAsciiProfile(t)

	out := renderNodeDetailAt(t, "zh", netMappingNode(), 120)
	block := blockAfter(out, "NPU 网络映射")
	if len(block) < 3 {
		t.Fatalf("network mapping block too short:\n%s", out)
	}
	if block[1] != netMappingHeaderZh() {
		t.Errorf("zh net mapping header:\n got %q\nwant %q", block[1], netMappingHeaderZh())
	}
	for _, bad := range []string{"DEVICE IP", "TOR SWITCH", "TOR PORT", "npudetail.", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(strings.Join(block, "\n"), bad) {
			t.Errorf("zh net mapping block still contains %q:\n%s", bad, strings.Join(block, "\n"))
		}
	}
}

func TestRenderNodeNetworkTrafficBilingual(t *testing.T) {
	forcedAsciiProfile(t)

	node := &model.NodeData{
		Name: "node-t", Status: "Ready",
		NetworkRxBytes: 1024,
		NetworkTxBytes: 2048,
	}

	en := renderNodeDetailAt(t, "en", node, 120)
	// "Total" stays unstyled and byte-identical to the previous hardcoded
	// "Total: %s" segment.
	if !strings.Contains(en, "  RX: 1.0KB  TX: 2.0KB  Total: 3.0KB") {
		t.Errorf("en traffic line missing or changed:\n%s", en)
	}
	if !strings.Contains(en, "  Network Traffic (Cumulative):") {
		t.Errorf("en traffic header missing:\n%s", en)
	}

	zh := renderNodeDetailAt(t, "zh", node, 120)
	for _, noun := range []string{"网络流量（累计）：", "接收", "发送", "总计"} {
		if !strings.Contains(zh, noun) {
			t.Errorf("zh traffic render %q does not contain %q", zh, noun)
		}
	}
	if !strings.Contains(zh, "  接收: 1.0KB  发送: 2.0KB  总计: 3.0KB") {
		t.Errorf("zh traffic line unexpected:\n%s", zh)
	}
	for _, bad := range []string{"Total", "Network Traffic", "RX:", "TX:", "{{", "}}", "<no value", "%!", "detail."} {
		if strings.Contains(zh, bad) {
			t.Errorf("zh traffic render %q contains %q", zh, bad)
		}
	}
}
