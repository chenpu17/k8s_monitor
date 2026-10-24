package ui

// b25_npu_owner_test.go — batch 25: the Pod↔chip attribution surfaces.
//
// The ownership itself comes from NPU-Exporter's pod_name / namespace labels
// (internal/datasource/npu_exporter.go), so the reference cluster — 0/8 chips
// allocated — can only ever show the empty shape. Every allocated fixture below
// is therefore synthetic.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
)

// b25OwnedChipNode is the extended chip node of the reference geometry with two
// chips attributed to one pod. The chips keep the base fixture's metrics so the
// chip table block stays byte-identical and only the sparse rows grow.
func b25OwnedChipNode() *model.NodeData {
	node := extendedChipNode()
	node.NPUChips[0].NPUID, node.NPUChips[0].Chip = 0, 0
	node.NPUChips[1].NPUID, node.NPUChips[1].Chip = 0, 1
	node.NPUChips[0].OwnerPod, node.NPUChips[0].OwnerNamespace = "train-0", "ml"
	node.NPUChips[1].OwnerPod, node.NPUChips[1].OwnerNamespace = "train-0", "ml"
	return node
}

// TestNodeDetailChipOwnerSparseRows pins W2-1: one row per owned chip (two chips
// of the same pod stay two rows), below the table, and nothing at all for an
// idle node.
func TestNodeDetailChipOwnerSparseRows(t *testing.T) {
	out := renderNPUDetailAtWidth(t, b25OwnedChipNode(), 120)

	want := []string{
		"    chip 0-0 → ml/train-0",
		"    chip 0-1 → ml/train-0",
	}
	for _, line := range want {
		if got := stripANSI(lineContaining(out, line)); got == "" {
			t.Errorf("the node detail is missing the owner row %q:\n%s", line, out)
		}
	}

	// Both rows sit below the chip table, not inside it: the table block keeps
	// its five lines.
	block := blockAfter(out, "NPU Chip Details")
	if len(block) != 5 {
		t.Errorf("the chip table block became %d lines: %v", len(block), block)
	}

	// Sparse: an idle node renders no owner row.
	idle := renderNPUDetailAtWidth(t, extendedChipNode(), 120)
	if strings.Contains(idle, "→ ml/train-0") || strings.Contains(stripANSI(idle), "chip 0-0 →") {
		t.Errorf("an idle node rendered an owner row:\n%s", idle)
	}
}

// TestNodeDetailChipOwnerFitTerminal pins the truncation: the owner value is cut
// to the field width like the other sparse rows, so a long pod name cannot push
// the row past the terminal.
func TestNodeDetailChipOwnerFitTerminal(t *testing.T) {
	node := extendedChipNode()
	for i := range node.NPUChips {
		node.NPUChips[i].OwnerPod = strings.Repeat("p", 80)
		node.NPUChips[i].OwnerNamespace = "namespace-with-a-long-name"
	}

	for _, width := range []int{60, 80, 120} {
		out := renderNPUDetailAtWidth(t, node, width)
		line := lineContaining(out, "→ ")
		if line == "" {
			t.Fatalf("width %d: no owner row rendered:\n%s", width, out)
		}
		if w := runewidth.StringWidth(line); w > width {
			t.Errorf("width %d: the owner row is %d cells: %q", width, w, line)
		}
		if strings.Contains(line, "\n") {
			t.Errorf("width %d: the owner row wrapped: %q", width, line)
		}
	}
}

// TestNPUViewRowOccupiedSegment pins W2-2: the node row grows a "· N occupied"
// segment when the exporter attributes chips, and keeps its old bytes when it
// does not.
func TestNPUViewRowOccupiedSegment(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 200, 40)
	if strings.Contains(stripANSI(m.renderNPUView()), "occupied") {
		t.Errorf("an idle fixture rendered the occupied segment:\n%s", m.renderNPUView())
	}

	// Two of node-a's 16 chips are attributed; the row reports the chip count,
	// next to (not instead of) the grade counters.
	m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{
		{NPUID: 0, PhyID: 0, OwnerPod: "train-0", OwnerNamespace: "ml"},
		{NPUID: 0, PhyID: 1, OwnerPod: "train-0", OwnerNamespace: "ml"},
	}
	row := stripANSI(lineContaining(m.renderNPUView(), "node-a"))
	if !strings.Contains(row, "2 occupied") {
		t.Fatalf("the node row has no occupied segment: %q", row)
	}
	if !strings.Contains(row, "2 degraded • 1 fault") {
		t.Errorf("the occupied segment displaced the grade counters: %q", row)
	}
	// A node without attribution keeps no segment of its own.
	for _, line := range strings.Split(stripANSI(m.renderNPUView()), "\n") {
		if strings.Contains(line, "node-b") && strings.Contains(line, "occupied") {
			t.Errorf("a node without attribution rendered the segment: %q", line)
		}
	}
}

// TestNPUViewRowOccupiedNarrowTerminal pins W2-2's narrow-terminal rule: the
// segment goes through the existing per-row truncation, so no row exceeds the
// terminal width at the smoke size.
func TestNPUViewRowOccupiedNarrowTerminal(t *testing.T) {
	forcedAsciiProfile(t)

	for _, width := range []int{38, 40, 60} {
		m := npuViewModel(t, "en", width, 40)
		m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{
			{NPUID: 0, PhyID: 0, OwnerPod: "train-0", OwnerNamespace: "ml"},
		}
		for _, line := range strings.Split(stripANSI(m.renderNPUView()), "\n") {
			if w := runewidth.StringWidth(line); w > width {
				t.Errorf("width %d: a rendered line is %d cells: %q", width, w, line)
			}
		}
	}
}

// podChipFixture builds a cluster with one NPU node carrying eight chips: two
// attributed to ml/train-0, two to ml/train-1, and a third pod that requests
// NPUs without any attribution.
func podChipFixture() (*model.ClusterData, *model.PodData, *model.PodData, *model.PodData) {
	chips := make([]model.NPUChipData, 8)
	for i := range chips {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK"}
	}
	chips[0].OwnerPod, chips[0].OwnerNamespace = "train-0", "ml"
	chips[1].OwnerPod, chips[1].OwnerNamespace = "train-0", "ml"
	chips[2].OwnerPod, chips[2].OwnerNamespace = "train-1", "ml"
	chips[3].OwnerPod, chips[3].OwnerNamespace = "train-1", "ml"

	node := &model.NodeData{
		Name: "npu-node", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8,
		NPUChips: chips,
	}
	owned0 := &model.PodData{Name: "train-0", Namespace: "ml", Node: "npu-node", Phase: "Running", NPURequest: 2}
	owned1 := &model.PodData{Name: "train-1", Namespace: "ml", Node: "npu-node", Phase: "Running", NPURequest: 2}
	unattributed := &model.PodData{Name: "train-2", Namespace: "ml", Node: "npu-node", Phase: "Running", NPURequest: 2}

	return &model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Pods:    []*model.PodData{owned0, owned1, unattributed},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}, owned0, owned1, unattributed
}

// TestPodDetailChipAttributionThreeStates pins W2-3 over its three states: a pod
// with attributed chips, a pod that requests NPUs without attribution (the
// honest "not captured" wording), and a pod with neither (no row at all).
func TestPodDetailChipAttributionThreeStates(t *testing.T) {
	forcedAsciiProfile(t)

	m := NewModel(nil, nil, 0, "en", "test", 200)
	m.width, m.height = 160, 60
	data, owned0, owned1, unattributed := podChipFixture()
	m.clusterData = data

	for pod, want := range map[*model.PodData]string{
		owned0:       "Occupied NPU chips: 0-0, 0-1",
		owned1:       "Occupied NPU chips: 1-0, 1-1",
		unattributed: "not captured",
	} {
		m.selectedPod = pod
		info := stripANSI(m.renderPodBasicInfo(pod))
		if !strings.Contains(info, want) {
			t.Errorf("pod %s: the chip line %q is missing:\n%s", pod.Name, want, info)
		}
	}

	// No NPU request and no attribution: no line at all.
	plain := &model.PodData{Name: "web-0", Namespace: "default", Node: "npu-node", Phase: "Running"}
	m.selectedPod = plain
	if info := stripANSI(m.renderPodBasicInfo(plain)); strings.Contains(info, "NPU") {
		t.Errorf("a pod without NPUs rendered a chip line:\n%s", info)
	}

	// Unscheduled: no line at all — not even the "not captured" fallback, which
	// would blame the exporter for a pod that has no node to attribute to.
	unscheduled := &model.PodData{Name: "train-3", Namespace: "ml", Phase: "Pending", NPURequest: 2}
	m.selectedPod = unscheduled
	if info := stripANSI(m.renderPodBasicInfo(unscheduled)); strings.Contains(info, "NPU:") {
		t.Errorf("an unscheduled pod rendered a chip line:\n%s", info)
	}
}

// TestPodDetailChipLookupIsNamespaced pins the triple-key lookup: a pod whose
// name matches the owner but whose namespace does not must not claim the chips.
func TestPodDetailChipLookupIsNamespaced(t *testing.T) {
	forcedAsciiProfile(t)

	m := NewModel(nil, nil, 0, "en", "test", 200)
	m.width, m.height = 160, 60
	data, _, _, _ := podChipFixture()
	m.clusterData = data

	impostor := &model.PodData{Name: "train-0", Namespace: "other", Node: "npu-node", Phase: "Running", NPURequest: 2}
	info := stripANSI(m.renderPodBasicInfo(impostor))
	if strings.Contains(info, "0-0") || strings.Contains(info, "0-1") {
		t.Errorf("a same-named pod in another namespace claimed chips:\n%s", info)
	}
	if !strings.Contains(info, "not captured") {
		t.Errorf("the impostor did not fall back to the unattributed wording:\n%s", info)
	}

	// And the node must match too: the same pod name on a different node owns
	// nothing.
	otherNode := &model.PodData{Name: "train-0", Namespace: "ml", Node: "npu-node-2", Phase: "Running", NPURequest: 2}
	info = stripANSI(m.renderPodBasicInfo(otherNode))
	if strings.Contains(info, "0-0") {
		t.Errorf("a same-named pod on another node claimed chips:\n%s", info)
	}
}

// TestPodDetailChipUnknownWordingIsNeutral pins the honesty rule for the
// "not captured" copy: it names this tool's visibility limit, so it must not
// read as a cluster fault.
func TestPodDetailChipUnknownWordingIsNeutral(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		got := i18n.NewLocalizer(locale).T("npudetail.pod_chips_unknown")
		for _, alarming := range []string{"error", "fail", "fault", "异常", "故障", "错误", "失败"} {
			if strings.Contains(strings.ToLower(got), alarming) {
				t.Errorf("%s npudetail.pod_chips_unknown reads as a fault (%q): %q", locale, alarming, got)
			}
		}
		if strings.Contains(got, "npudetail.") {
			t.Errorf("%s npudetail.pod_chips_unknown is unresolved: %q", locale, got)
		}
	}
}

// TestNPUViewZhOwnerSurfaces pins the zh render of the new surfaces: no key name
// leaks and no English is left over.
func TestNPUViewZhOwnerSurfaces(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "zh", 160, 40)
	m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{
		{NPUID: 0, PhyID: 0, OwnerPod: "train-0", OwnerNamespace: "ml"},
	}
	out := stripANSI(m.renderNPUView())
	if !strings.Contains(out, "1 占用") {
		t.Errorf("the zh node row is missing the occupied segment:\n%s", out)
	}
	for _, unwanted := range []string{"occupied", "npuview.", "{{"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the zh NPU view carries %q:\n%s", unwanted, out)
		}
	}

	// Node detail in zh.
	node := b25OwnedChipNode()
	m.selectedNode = node
	m.currentView = ViewNodeDetail
	detail := stripANSI(m.renderNodeDetail())
	if !strings.Contains(detail, "芯片 0-0 → ml/train-0") {
		t.Errorf("the zh node detail is missing the owner row:\n%s", detail)
	}
	if strings.Contains(detail, "chip 0-0 →") {
		t.Errorf("the zh node detail left the English owner row:\n%s", detail)
	}

	// Pod detail in zh.
	data, owned0, _, _ := podChipFixture()
	m.clusterData = data
	m.selectedPod = owned0
	podInfo := stripANSI(m.renderPodBasicInfo(owned0))
	if !strings.Contains(podInfo, "占用 NPU 芯片：0-0, 0-1") {
		t.Errorf("the zh pod detail is missing the chip line:\n%s", podInfo)
	}
	for _, unwanted := range []string{"Occupied NPU chips", "npudetail.", "{{"} {
		if strings.Contains(podInfo, unwanted) {
			t.Errorf("the zh pod detail carries %q:\n%s", unwanted, podInfo)
		}
	}
}

// TestNPUViewSummaryNoKnownFaultSwitches pins W2-4: the wording switches only
// when unknown chips exist, and the counts never move.
func TestNPUViewSummaryNoKnownFaultSwitches(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 200, 40)
	clean := stripANSI(m.renderNPUView())
	if !strings.Contains(clean, "13/16 healthy · 2 degraded · 1 fault") {
		t.Errorf("the all-reported cluster lost the plain health wording:\n%s", clean)
	}
	if strings.Contains(clean, "no fault") {
		t.Errorf("the all-reported cluster switched to the no-fault wording:\n%s", clean)
	}

	m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{{NPUID: 0, PhyID: 0, Health: "Unknown"}}
	withUnknown := stripANSI(m.renderNPUView())
	if !strings.Contains(withUnknown, "no fault") {
		t.Errorf("a cluster with an unknown chip did not switch the wording:\n%s", withUnknown)
	}
	if !strings.Contains(withUnknown, "2 degraded • 1 fault") {
		t.Errorf("the switch lost the degraded/fault counts:\n%s", withUnknown)
	}
	// The tally is the matrix's: the one unknown chip keeps a healthy grade, so
	// the healthy count must still be 13.
	if !strings.Contains(withUnknown, "13/16 no fault") {
		t.Errorf("the switched segment changed the count:\n%s", withUnknown)
	}

	// zh switches as well.
	zh := npuViewModel(t, "zh", 200, 40)
	zh.clusterData.Nodes[0].NPUChips = []model.NPUChipData{{NPUID: 0, PhyID: 0, Health: "Unknown"}}
	if out := stripANSI(zh.renderNPUView()); !strings.Contains(out, "无故障") {
		t.Errorf("the zh summary did not switch the wording:\n%s", out)
	}
}

// TestExportOwnerFields pins W1-2 at the export boundary: an attributed chip
// exports its owner keys, an idle one exports none, and the non-NPU node golden
// (b24's TestExportNodesJSONNonNPUByteIdentical) is unaffected by construction.
func TestExportOwnerFields(t *testing.T) {
	dir := t.TempDir()

	data, owned0, _, _ := podChipFixture()
	_ = owned0
	m := NewModel(nil, nil, 0, "en", "test", 200)
	if err := m.exportNodesJSON(data, dir, "owned.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}
	raw := readFileString(t, dir+"/owned.json")
	for _, want := range []string{`"owner_pod": "train-0"`, `"owner_namespace": "ml"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the export is missing %s:\n%s", want, raw)
		}
	}

	// An all-idle node exports no owner key at all.
	idle := &model.NodeData{
		Name: "npu-idle", Status: "Ready", NPUCapacity: 2,
		NPUChips: []model.NPUChipData{{NPUID: 0, Chip: 0, PhyID: 0}, {NPUID: 0, Chip: 1, PhyID: 1}},
	}
	if err := m.exportNodesJSON(&model.ClusterData{Nodes: []*model.NodeData{idle}}, dir, "idle.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}
	if raw := readFileString(t, dir+"/idle.json"); strings.Contains(raw, "owner_") {
		t.Errorf("an idle node exported an owner key:\n%s", raw)
	}

	// The shape of the owner keys in context, for the delivery note.
	var decoded []model.NodeData
	rawBytes, err := os.ReadFile(dir + "/owned.json")
	if err != nil {
		t.Fatalf("read owned.json: %v", err)
	}
	if err := json.Unmarshal(rawBytes, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 1 || len(decoded[0].NPUChips) != 8 {
		t.Fatalf("round trip produced %+v", decoded)
	}
	var owned int
	for _, chip := range decoded[0].NPUChips {
		if chip.OwnerPod != "" {
			owned++
			if chip.OwnerNamespace != "ml" {
				t.Errorf("chip %d round-tripped a namespace %q", chip.PhyID, chip.OwnerNamespace)
			}
		}
	}
	if owned != 4 {
		t.Errorf("round trip kept %d attributed chips, want 4", owned)
	}
}

// TestPodDetailChipLineFitsTerminal pins that the chip row respects the
// terminal width at the narrow end, for both wordings: the chips list grows with
// every chip the pod holds, and the "not captured" explanation is the longest
// string this batch adds.
func TestPodDetailChipLineFitsTerminal(t *testing.T) {
	forcedAsciiProfile(t)

	data, owned0, _, unattributed := podChipFixture()
	// A pod holding every chip of the node, so the id list is at its widest.
	wide := &model.PodData{Name: "train-wide", Namespace: "ml", Node: "npu-node", Phase: "Running", NPURequest: 8}
	for i := range data.Nodes[0].NPUChips {
		data.Nodes[0].NPUChips[i].OwnerPod, data.Nodes[0].NPUChips[i].OwnerNamespace = wide.Name, wide.Namespace
	}

	for _, width := range []int{30, 38, 60, 80} {
		for _, pod := range []*model.PodData{owned0, unattributed, wide} {
			m := NewModel(nil, nil, 0, "en", "test", 200)
			m.width, m.height = width, 60
			m.clusterData = data
			m.selectedPod = pod

			info := stripANSI(m.renderPodBasicInfo(pod))
			line := lineContaining(info, "NPU:")
			// All three fixtures are scheduled pods (Node set) that request
			// NPUs, so the row is mandatory: owned0 lists its chips,
			// unattributed renders the "not captured" explanation and wide
			// lists every chip of the node. A missing row is a failure, not a
			// case to skip — skipping made the probe green on exactly the
			// regression it exists to catch.
			if line == "" {
				t.Fatalf("width %d, pod %s: no NPU chip row rendered", width, pod.Name)
			}
			if w := runewidth.StringWidth(line); w > width {
				t.Errorf("width %d, pod %s: the chip row is %d cells: %q", width, pod.Name, w, line)
			}
		}
	}
}

// TestOwnerKeysZhCopy pins the zh values of the five new keys: present, no key
// name leak, and the zh owner line keeps the full-width colon convention of its
// sibling sparse rows.
func TestOwnerKeysZhCopy(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	for _, key := range []string{
		"npudetail.chip_owner",
		"npudetail.pod_chips",
		"npudetail.pod_chips_unknown",
		"npuview.row_occupied",
		"npuview.summary_no_known",
	} {
		if _, ok := en[key]; !ok {
			t.Errorf("the en bundle is missing %s", key)
		}
		if _, ok := zh[key]; !ok {
			t.Errorf("the zh bundle is missing %s", key)
		}
		if strings.Contains(zh[key], ": ") && !strings.Contains(zh[key], "：") {
			t.Errorf("zh %s uses a half-width colon: %q", key, zh[key])
		}
	}

	// The two owner-row templates carry the 4-space indent the sibling sparse
	// rows use, inside the bundle value.
	if got, want := en["npudetail.chip_owner"], "    chip {{.NPU}}-{{.Chip}} → {{.Owner}}"; got != want {
		t.Errorf("en npudetail.chip_owner = %q, want %q", got, want)
	}
	if got, want := zh["npudetail.chip_owner"], "    芯片 {{.NPU}}-{{.Chip}} → {{.Owner}}"; got != want {
		t.Errorf("zh npudetail.chip_owner = %q, want %q", got, want)
	}
}

// readFileString reads a file the test just wrote, failing the test on error.
func readFileString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// TestOwnerSurfacesKeyed pins that every new user-visible string of this batch
// resolves through the bundle rather than being hardcoded: the surfaces are
// rendered here in both locales and scanned for the en text in the zh run.
func TestOwnerSurfacesKeyed(t *testing.T) {
	forcedAsciiProfile(t)

	render := func(locale string) (detail, npuView, podInfo string) {
		m := npuViewModel(t, locale, 160, 40)
		m.clusterData.Nodes[0].NPUChips = []model.NPUChipData{
			{NPUID: 0, Chip: 0, PhyID: 0, OwnerPod: "train-0", OwnerNamespace: "ml"},
		}
		npuView = stripANSI(m.renderNPUView())

		m.selectedNode = b25OwnedChipNode()
		detail = stripANSI(m.renderNodeDetail())

		data, owned0, _, _ := podChipFixture()
		m.clusterData = data
		podInfo = stripANSI(m.renderPodBasicInfo(owned0))
		return detail, npuView, podInfo
	}

	enDetail, enView, enPod := render("en")
	zhDetail, zhView, zhPod := render("zh")

	pairs := []struct {
		name       string
		en, zh     string
		enFragment string
	}{
		{"node detail owner row", enDetail, zhDetail, "chip 0-0 → ml/train-0"},
		{"npu view occupied", enView, zhView, "1 occupied"},
		{"pod detail chips", enPod, zhPod, "Occupied NPU chips: 0-0, 0-1"},
	}
	for _, p := range pairs {
		if !strings.Contains(p.en, p.enFragment) {
			t.Errorf("%s: the en render is missing %q:\n%s", p.name, p.enFragment, p.en)
		}
		if strings.Contains(p.zh, p.enFragment) {
			t.Errorf("%s: the zh render still carries the English %q:\n%s", p.name, p.enFragment, p.zh)
		}
	}
}
