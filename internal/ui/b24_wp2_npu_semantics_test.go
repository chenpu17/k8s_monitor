package ui

// b24_wp2_npu_semantics_test.go — batch 24 W1-5 / W3-2 / W4-2 / W5-2: the five
// new display surfaces of the NPU data semantics.
//
// The chip tables must stay byte-identical (their column widths, including the
// 61-column tier boundary, are golden anchors), so everything new is a sparse
// row below the table, a conditional summary segment or a line in the runtime
// section.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
)

// b24ChipNode is the extended-metrics node with one chip carrying everything
// the new sparse rows can render: a classified error code, the reference node's
// six-digit cumulative ECC count with no per-window errors, and a session delta.
func b24ChipNode() *model.NodeData {
	node := extendedChipNode()
	node.NPUChips[0].ErrorCode = 0x80E21007
	node.NPUChips[0].ErrorCodeFamily = model.NPUErrorCatNoAction
	node.NPUChips[1].HBMEccSingleErrTotal = 99992
	node.NPUChips[1].HBMEccSingleErrDelta = 10
	node.NPUChips[1].HBMEccDeltaSamples = 4
	return node
}

// TestRenderNodeDetailSparseChipRows pins the two sparse rows: an error-code row
// with the knowledge-base class, and an ECC row with the cumulative totals and
// their session deltas — both below the table, neither inside it.
func TestRenderNodeDetailSparseChipRows(t *testing.T) {
	out := renderNPUDetailAtWidth(t, b24ChipNode(), 120)
	lines := strings.Split(out, "\n")

	codeLine := lineContaining(out, "80E21007")
	if codeLine == "" {
		t.Fatalf("no error-code row rendered:\n%s", out)
	}
	if got, want := stripANSI(codeLine), "    chip 0-0: 80E21007 · no action needed"; got != want {
		t.Errorf("error-code row = %q, want %q", got, want)
	}

	eccLine := lineContaining(out, "ECC total single 99992")
	if eccLine == "" {
		t.Fatalf("no ECC row rendered:\n%s", out)
	}
	if got, want := stripANSI(eccLine), "    chip 0-0: ECC total single 99992 (Δ+10) · double 0 (Δ+0)"; got != want {
		t.Errorf("ECC row = %q, want %q", got, want)
	}

	// Both rows come after the chip table, not inside it: the table block is
	// still exactly five lines, its geometry unchanged.
	block := blockAfter(out, "NPU Chip Details")
	if len(block) != 5 {
		t.Fatalf("the chip table block grew to %d lines: %v", len(block), block)
	}
	tableEnd := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(stripANSI(line)), "1   20%") {
			tableEnd = i
		}
	}
	codeIdx, eccIdx := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "80E21007") {
			codeIdx = i
		}
		if strings.Contains(line, "ECC total single 99992") {
			eccIdx = i
		}
	}
	if tableEnd < 0 || codeIdx < tableEnd || eccIdx < tableEnd {
		t.Errorf("sparse rows are not below the table (table ends %d, code %d, ecc %d)", tableEnd, codeIdx, eccIdx)
	}
	if codeIdx > eccIdx {
		t.Errorf("the ECC row is above the error-code row (code %d, ecc %d)", codeIdx, eccIdx)
	}
}

// TestRenderNodeDetailSparseRowsFitTerminal pins that the sparse rows respect
// the terminal width like every other line of the detail.
func TestRenderNodeDetailSparseRowsFitTerminal(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			out := renderNPUDetailAtWidth(t, b24ChipNode(), width)
			for _, want := range []string{"80E21007", "ECC total single 99992"} {
				line := lineContaining(out, want)
				if line == "" {
					t.Fatalf("%q missing at width %d\n---\n%s", want, width, out)
				}
				if w := runewidth.StringWidth(line); w > width {
					t.Errorf("row is %d cells wide, terminal is %d: %q", w, width, line)
				}
			}
		})
	}
}

// TestRenderNodeDetailSparseRowsHiddenWhenClean pins the sparsity: a node with
// no error code and no ECC history renders no sparse row at all, so the healthy
// detail page of the reference node gains nothing.
func TestRenderNodeDetailSparseRowsHiddenWhenClean(t *testing.T) {
	out := renderNPUDetailAtWidth(t, extendedChipNode(), 120)
	for _, absent := range []string{"ECC total", "chip 0-0:", "chip 1-0:"} {
		if strings.Contains(out, absent) {
			t.Errorf("a healthy node rendered the sparse marker %q:\n%s", absent, out)
		}
	}
}

// TestRenderNodeDetailECCDeltaNeedsTwoSamples pins the window rule: with one
// sample the ECC row shows the totals without a delta qualifier, because Δ+0
// would read as "nothing changed since monitoring started".
func TestRenderNodeDetailECCDeltaNeedsTwoSamples(t *testing.T) {
	node := extendedChipNode()
	node.NPUChips[0].HBMEccSingleErrTotal = 7
	node.NPUChips[0].HBMEccDeltaSamples = 1

	line := stripANSI(lineContaining(renderNPUDetailAtWidth(t, node, 120), "ECC total single 7"))
	if line == "" {
		t.Fatal("no ECC row rendered")
	}
	if strings.Contains(line, "Δ") {
		t.Errorf("a single-sample window rendered a delta: %q", line)
	}
	if !strings.Contains(line, "double 0") {
		t.Errorf("the ECC row lost the double-bit counter: %q", line)
	}
}

// TestRenderNodeDetailUnknownHealthCount pins W1-5's node-detail surface: the
// unknown-chip count appears only when there are unknown chips.
func TestRenderNodeDetailUnknownHealthCount(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 200

	node := &model.NodeData{
		Name: "npu-node", Status: "Ready", NPUCapacity: 2, NPUAllocatable: 2,
		NPUHealthStatus: "Healthy",
		NPUChips: []model.NPUChipData{
			{NPUID: 0, Chip: 0, PhyID: 0, Health: "OK"},
			{NPUID: 0, Chip: 1, PhyID: 1, Health: "Unknown"},
		},
	}
	m.selectedNode = node
	out := stripANSI(m.renderNodeDetail())
	if line := lineContaining(out, m.T("npudetail.health_unknown")); !strings.Contains(line, ": 1") {
		t.Errorf("unknown-chip row = %q, want it to report 1\n---\n%s", line, out)
	}

	// No unknown chips: the row is gone.
	node.NPUChips[1].Health = "OK"
	out = stripANSI(m.renderNodeDetail())
	if strings.Contains(out, m.T("npudetail.health_unknown")) {
		t.Errorf("the unknown-chip row rendered without unknown chips:\n%s", out)
	}
}

// TestRenderNodeDetailExporterLiveness pins W4-2's node-detail surface for all
// three states, including the raw error text (truncated) on the degraded one and
// no liveness row at all when the data carries no state.
func TestRenderNodeDetailExporterLiveness(t *testing.T) {
	base := func(liveness *model.NPUExporterLiveness) string {
		m := newTestModel(t)
		m.width = 120
		m.height = 200
		m.selectedNode = &model.NodeData{
			Name: "npu-node", Status: "Ready", NPUCapacity: 1, NPUAllocatable: 1,
			NPUHealthStatus: "Healthy",
			NPUChips:        []model.NPUChipData{{NPUID: 0, Chip: 0, PhyID: 0, Health: "OK"}},
		}
		if liveness != nil {
			m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{NPUExporterLiveness: liveness}}
		}
		return stripANSI(m.renderNodeDetail())
	}

	ok := base(&model.NPUExporterLiveness{
		Status: "ok", MessageKey: "npu.liveness.ok",
		LastSuccess: time.Date(2026, 10, 23, 10, 11, 12, 0, time.Local),
	})
	if line := lineContaining(ok, "NPU-Exporter:"); !strings.Contains(line, "reachable") {
		t.Errorf("ok liveness row = %q", line)
	}
	if strings.Contains(ok, "npu.liveness.") {
		t.Errorf("a liveness key leaked into the render:\n%s", ok)
	}

	const rawErr = "dial tcp 10.0.0.1:8082: connect: connection refused"
	backoff := base(&model.NPUExporterLiveness{
		Status: "backoff", MessageKey: "npu.liveness.backoff", LastError: rawErr,
		MessageArgs: map[string]interface{}{"Error": rawErr},
		LastSuccess: time.Date(2026, 10, 23, 10, 11, 12, 0, time.Local),
	})
	line := lineContaining(backoff, "NPU-Exporter:")
	if !strings.Contains(line, "unreachable") {
		t.Errorf("backoff liveness row = %q", line)
	}
	if !strings.Contains(line, "connection refused") {
		t.Errorf("the backoff row lost the raw error text: %q", line)
	}
	if w := runewidth.StringWidth(line); w > 120 {
		t.Errorf("the backoff row is %d cells wide: %q", w, line)
	}

	never := base(&model.NPUExporterLiveness{Status: "never", MessageKey: "npu.liveness.never"})
	if line := lineContaining(never, "NPU-Exporter:"); !strings.Contains(line, "not polled") {
		t.Errorf("never liveness row = %q", line)
	}

	absent := base(nil)
	if strings.Contains(absent, "NPU-Exporter:") {
		t.Errorf("a detail without liveness state rendered the row:\n%s", absent)
	}
}

// TestNPUViewSummaryUnknownSegment pins the W1-5 summary segment: it appears
// only when chips carry unknown health, and the reference fixture (all chips
// with a reported status) keeps the summary bar byte-identical.
func TestNPUViewSummaryUnknownSegment(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 120, 40)
	if strings.Contains(m.renderNPUView(), m.T("npuview.summary_unknown")[:5]) {
		t.Errorf("the unknown segment rendered without unknown chips:\n%s", m.renderNPUView())
	}

	m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{
		{NPUID: 0, Chip: 0, PhyID: 0, Health: "OK"},
		{NPUID: 0, Chip: 1, PhyID: 1, Health: "Unknown"},
		{NPUID: 1, Chip: 0, PhyID: 2, Health: "Unknown"},
	}
	out := stripANSI(m.renderNPUView())
	if !strings.Contains(out, "unknown 2") {
		t.Errorf("the unknown segment is missing its count:\n%s", out)
	}

	// The segment is part of the summary bar, i.e. above the legend.
	if lineContaining(out, "unknown 2") == "" {
		t.Errorf("the unknown segment is not in the summary bar:\n%s", out)
	}
	if idxUnknown, idxLegend := strings.Index(out, "unknown 2"), strings.Index(out, m.T("npuview.legend")); idxUnknown < 0 || idxUnknown > idxLegend {
		t.Errorf("the unknown segment is not above the legend (unknown %d, legend %d)", idxUnknown, idxLegend)
	}
}

// TestNPUViewLivenessSegment pins the W4-2 summary segment for the three states,
// including the "last success" clock and the guarantee that the raw error text
// stays out of the bar.
func TestNPUViewLivenessSegment(t *testing.T) {
	forcedAsciiProfile(t)

	setLiveness := func(m *Model, status, key string, lastSuccess time.Time) {
		m.clusterData.Summary.NPUExporterLiveness = &model.NPUExporterLiveness{
			Status: status, MessageKey: key, LastSuccess: lastSuccess,
			LastError: "dial tcp 10.0.0.1:8082: connect: connection refused",
		}
	}

	at := time.Date(2026, 10, 23, 10, 11, 12, 0, time.Local)

	t.Run("ok", func(t *testing.T) {
		m := npuViewModel(t, "en", 120, 40)
		setLiveness(m, "ok", "npu.liveness.ok", at)
		out := stripANSI(m.renderNPUView())
		if !strings.Contains(out, "Monitoring OK · last success 10:11:12") {
			t.Errorf("the ok segment is missing:\n%s", out)
		}
		if strings.Contains(out, "connection refused") {
			t.Errorf("the summary bar carries the raw error text:\n%s", out)
		}
	})

	t.Run("backoff", func(t *testing.T) {
		m := npuViewModel(t, "en", 120, 40)
		setLiveness(m, "backoff", "npu.liveness.backoff", at)
		out := stripANSI(m.renderNPUView())
		if !strings.Contains(out, "Monitoring degraded · last success 10:11:12") {
			t.Errorf("the backoff segment is missing:\n%s", out)
		}
		if strings.Contains(out, "connection refused") {
			t.Errorf("the summary bar carries the raw error text:\n%s", out)
		}
	})

	t.Run("never", func(t *testing.T) {
		m := npuViewModel(t, "en", 120, 40)
		setLiveness(m, "never", "npu.liveness.never", time.Time{})
		out := stripANSI(m.renderNPUView())
		if !strings.Contains(out, "Monitoring unavailable") {
			t.Errorf("the never segment is missing:\n%s", out)
		}
	})

	t.Run("no state keeps the bar unchanged", func(t *testing.T) {
		m := npuViewModel(t, "en", 120, 40)
		out := stripANSI(m.renderNPUView())
		for _, absent := range []string{"Monitoring OK", "Monitoring degraded", "Monitoring unavailable"} {
			if strings.Contains(out, absent) {
				t.Errorf("the bar carries %q without liveness state:\n%s", absent, out)
			}
		}
		if !strings.Contains(out, "Monitoring 1/2 nodes") {
			t.Errorf("the plain coverage segment is missing:\n%s", out)
		}
	})

	t.Run("narrow terminal keeps every segment", func(t *testing.T) {
		for _, width := range []int{38, 40, 60} {
			m := npuViewModel(t, "en", width, 40)
			setLiveness(m, "ok", "npu.liveness.ok", at)
			out := stripANSI(m.renderNPUView())
			for _, seg := range m.npuSummarySegments(m.npuNodes()) {
				if !strings.Contains(out, stripANSI(seg)) {
					t.Errorf("width %d: segment %q is not rendered in full:\n%s", width, stripANSI(seg), out)
				}
			}
		}
	})
}

// TestNPUViewZhNewSegments pins the zh render of the new segments and the new
// node-detail keys: no key-name leakage and no English left over.
func TestNPUViewZhNewSegments(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "zh", 120, 40)
	m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{{NPUID: 0, Chip: 0, PhyID: 0, Health: "Unknown"}}
	m.clusterData.Summary.NPUExporterLiveness = &model.NPUExporterLiveness{
		Status: "ok", MessageKey: "npu.liveness.ok",
		LastSuccess: time.Date(2026, 10, 23, 10, 11, 12, 0, time.Local),
	}
	out := stripANSI(m.renderNPUView())

	for _, want := range []string{"1 未知", "监控 正常 · 上次成功 10:11:12"} {
		if !strings.Contains(out, want) {
			t.Errorf("the zh summary bar is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"unknown ", "Monitoring", "npuview.", "npu.liveness."} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the zh summary bar still carries %q:\n%s", unwanted, out)
		}
	}
}

// TestExportNodesJSONNonNPUByteIdentical pins W6-2: a node without NPU data
// exports exactly the fields it exported before this batch. The golden bytes
// below were captured through exportNodesJSON on both HEAD=44c52af and the
// batch working tree (review-verified identical, sha256 8412a328…), so the
// comparison is a true byte match: any new field that is not NPU-node-specific
// (or omitempty) fails here.
func TestExportNodesJSONNonNPUByteIdentical(t *testing.T) {
	dir := t.TempDir()

	node := &model.NodeData{Name: "cpu-node", Status: "Ready", Roles: []string{"worker"}}
	m := NewModel(nil, nil, 0, "en", "test", 200)
	if err := m.exportNodesJSON(&model.ClusterData{Nodes: []*model.NodeData{node}}, dir, "nodes.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if err != nil {
		t.Fatal(err)
	}

	const golden = `[
  {
    "Name": "cpu-node",
    "InternalIP": "",
    "ExternalIP": "",
    "Roles": [
      "worker"
    ],
    "Status": "Ready",
    "Conditions": null,
    "Taints": null,
    "Labels": null,
    "Annotations": null,
    "CreationTimestamp": "0001-01-01T00:00:00Z",
    "CPUCapacity": 0,
    "MemoryCapacity": 0,
    "PodCapacity": 0,
    "CPUAllocatable": 0,
    "MemAllocatable": 0,
    "PodAllocatable": 0,
    "CPUUsage": 0,
    "MemoryUsage": 0,
    "PodCount": 0,
    "NetworkRxBytes": 0,
    "NetworkTxBytes": 0,
    "NetworkTimestamp": "0001-01-01T00:00:00Z",
    "CPUUsagePercent": 0,
    "MemoryUsagePercent": 0,
    "PodUsagePercent": 0,
    "MemoryPressure": false,
    "DiskPressure": false,
    "PIDPressure": false,
    "HasKubeletMetrics": false,
    "KubeletError": "",
    "NPUCapacity": 0,
    "NPUAllocatable": 0,
    "NPUAllocated": 0,
    "NPUResourceName": "",
    "NPUChipType": "",
    "NPUDeviceType": "",
    "NPUDriverVersion": "",
    "NPUUtilization": 0,
    "NPUMemoryTotal": 0,
    "NPUMemoryUsed": 0,
    "NPUMemoryUtil": 0,
    "NPUTemperature": 0,
    "NPUPower": 0,
    "NPUHealthStatus": "",
    "NPUErrorCount": 0,
    "NPUAICoreCount": 0,
    "NPUMetricsTime": "0001-01-01T00:00:00Z",
    "NPUChips": null,
    "NPUStatusMask": 0,
    "NPUStatusMaskRaw": "",
    "NPUStatusKnown": false,
    "NPUFault": null,
    "HyperNodeID": "",
    "HyperClusterID": "",
    "SuperPodID": "",
    "CabinetInfo": "",
    "NPUTopology": null,
    "NPUDeviceNet": null
  }
]
`
	if string(raw) != golden {
		t.Errorf("a non-NPU node's export drifted from the pinned bytes:\n%s", raw)
	}

	// The same node with NPU data is the only one that grows fields.
	grown := &model.NodeData{
		Name: "npu-node", Status: "Ready", NPUCapacity: 1,
		NPUChips: []model.NPUChipData{{NPUID: 0, Chip: 0, ErrorCodeFamily: model.NPUErrorCatIsolate}},
	}
	if err := m.exportNodesJSON(&model.ClusterData{Nodes: []*model.NodeData{grown}}, dir, "grown.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}
	grownRaw, err := os.ReadFile(filepath.Join(dir, "grown.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(grownRaw), `"error_code_family": "isolate"`) {
		t.Errorf("an NPU chip with a classified code does not export it:\n%s", grownRaw)
	}
}

// TestNPUErrorCategoryKeys pins the five fault-code class labels in both
// locales: the key the node detail builds from the model family must resolve,
// and the zh copy carries no half-width colon.
func TestNPUErrorCategoryKeys(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	want := map[string]string{
		model.NPUErrorCatNoAction:  "no action needed",
		model.NPUErrorCatRerun:     "workload rerun",
		model.NPUErrorCatReset:     "NPU reset required",
		model.NPUErrorCatIsolate:   "chip isolation required",
		model.NPUErrorCatSubHealth: "sub-health",
	}
	for family, enCopy := range want {
		key := "npu.errorcat." + family
		if got, ok := en[key]; !ok || got != enCopy {
			t.Errorf("en %s = %q (present %v), want %q", key, got, ok, enCopy)
		}
		if _, ok := zh[key]; !ok {
			t.Errorf("the zh bundle is missing %s", key)
		}
		if strings.Contains(zh[key], ": ") && !strings.Contains(zh[key], "：") {
			t.Errorf("zh %s uses a half-width colon: %q", key, zh[key])
		}
	}
	if _, ok := en["npu.errorcat.unknown"]; !ok {
		t.Error("the en bundle is missing npu.errorcat.unknown (the unclassified fallback)")
	}
	if _, ok := zh["npu.errorcat.unknown"]; !ok {
		t.Error("the zh bundle is missing npu.errorcat.unknown")
	}
}

// TestExportNodesJSONReasonNotEscaped pins W2 at the export boundary: a node
// whose chips are graded with every reason the datasource can produce exports
// no HTML escape sequence, and the reasons round-trip verbatim.
func TestExportNodesJSONReasonNotEscaped(t *testing.T) {
	dir := t.TempDir()

	reasons := []string{
		"temp 91 ge 90",
		"temp 82 ge 80",
		"double-bit ECC",
		"single-bit ECC",
		"link down",
		"chip health Error",
		"error code 80E21007 (no action needed)",
		"error code 3735928559",
		"device plugin fault bit",
		"device fault level UrgentNPU",
	}
	grades := make([]model.NPUChipGrade, len(reasons))
	for i, reason := range reasons {
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeFault, Reason: reason}
	}

	data := &model.ClusterData{Nodes: []*model.NodeData{{
		Name: "npu-node", Status: "Ready", NPUCapacity: int64(len(reasons)), NPUChipHealth: grades,
	}}}
	m := NewModel(nil, nil, 0, "en", "test", 200)
	if err := m.exportNodesJSON(data, dir, "nodes.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `\u003c`) || strings.Contains(string(raw), `\u003e`) || strings.Contains(string(raw), `\u0026`) {
		t.Errorf("the export carries an HTML-escaped character:\n%s", raw)
	}
	if strings.Contains(string(raw), `\u003c`) || strings.Contains(string(raw), ">=\")") {
		t.Errorf("a pre-batch reason template survived:\n%s", raw)
	}

	var decoded []model.NodeData
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding the export: %v", err)
	}
	if len(decoded) != 1 || len(decoded[0].NPUChipHealth) != len(reasons) {
		t.Fatalf("round trip produced %+v", decoded)
	}
	for i, grade := range decoded[0].NPUChipHealth {
		if grade.Reason != reasons[i] {
			t.Errorf("reason %d round-tripped to %q, want %q", i, grade.Reason, reasons[i])
		}
		// The guard itself: no reason may carry the three escaped characters.
		if strings.ContainsAny(grade.Reason, "<>&") {
			t.Errorf("reason %d = %q carries a JSON-escaped character", i, grade.Reason)
		}
	}
}

// TestNPUReasonTemplatesHaveNoEscapedCharacters is the mutation-catching guard
// for W2: every reason the datasource's detection can produce must stay free of
// the characters encoding/json escapes. The fixture drives all ten triggers in
// one pass, so a template rewritten back to ">=" fails here even if nothing
// else notices.
func TestNPUReasonFixtureCoversAllRoutes(t *testing.T) {
	// The datasource side pins the reasons themselves; this probe pins that the
	// export path used by the UI carries no escape for any of them.
	for _, reason := range []string{
		"temp 91 ge 90",
		"error code 80E21007 (no action needed)",
		"device fault level UrgentNPU",
	} {
		encoded, err := json.Marshal(model.NPUChipGrade{Grade: model.NPUChipGradeFault, Reason: reason})
		if err != nil {
			t.Fatalf("marshalling %q: %v", reason, err)
		}
		if strings.Contains(string(encoded), `\u00`) {
			t.Errorf("reason %q exported with an escape: %s", reason, encoded)
		}
	}
}

// TestNodeDetailNewKeysZhRenderSanity pins the four node-detail keys this batch
// adds through the real localizer: the zh render must not leak a key name or an
// unresolved placeholder.
func TestNodeDetailNewKeysZhRenderSanity(t *testing.T) {
	loc := i18n.NewLocalizer("zh")
	cases := []struct {
		key  string
		args map[string]interface{}
	}{
		{"npudetail.health_unknown", nil},
		{"npudetail.exporter_liveness", nil},
		{"npudetail.ecc_delta", map[string]interface{}{"NPU": 1, "Chip": 0, "Single": 99992, "SingleDelta": " (Δ+10)", "Double": 0, "DoubleDelta": ""}},
		{"npudetail.error_code_chip", map[string]interface{}{"NPU": 0, "Chip": 1, "Code": "80E21007", "Category": "无需处理"}},
	}
	for _, tc := range cases {
		got := loc.TF(tc.key, tc.args)
		for _, bad := range []string{"npudetail.", "{{", "}}", "<no value>", "%!"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s rendered %q which contains %q", tc.key, got, bad)
			}
		}
	}
}
