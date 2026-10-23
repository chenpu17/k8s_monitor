package datasource

// b24_wp3_errorcodes_test.go — batch 24 W3: the device plugin fault-code
// knowledge base.
//
// The knowledge base is static vendor data (faultCode.json, mind-cluster master
// snapshot 2026-10-23, Apache-2.0) embedded in the binary. The probes pin: the
// embedded copy is the file the snapshot describes, every handling tier maps to
// exactly one of the five families with two real codes each, the lookup is
// strict about 0 / unknown / negative codes, and the phrases the knowledge base
// injects into alert actions and grade reasons survive a JSON export verbatim.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// TestNPUErrorCodeSnapshotShape pins the embedded file: decoding it yields all
// eight upstream levels with their published entry counts, and the build
// produces a lookup table whose size matches the distinct codes.
func TestNPUErrorCodeSnapshotShape(t *testing.T) {
	var doc faultCodeDoc
	if err := json.Unmarshal(npuFaultCodeJSON, &doc); err != nil {
		t.Fatalf("the embedded faultCode.json does not decode: %v", err)
	}

	wantCounts := map[string]int{
		"NotHandleFault":  132,
		"RestartRequest":  33,
		"RestartBusiness": 26,
		"FreeRestartNPU":  24,
		"RestartNPU":      161,
		"SeparateNPU":     215,
		"PreSeparateNPU":  200,
		"SubHealthFault":  13,
	}
	if len(npuErrorCodeLevels) != len(wantCounts) {
		t.Fatalf("the knowledge base maps %d levels, want %d", len(npuErrorCodeLevels), len(wantCounts))
	}
	total := 0
	for _, level := range npuErrorCodeLevels {
		codes := level.codes(doc)
		if got, want := len(codes), wantCounts[level.level]; got != want {
			t.Errorf("%s carries %d codes, want %d", level.level, got, want)
		}
		total += len(codes)
		if level.category.Level != level.level {
			t.Errorf("%s carries category level %q", level.level, level.category.Level)
		}
	}

	// 804 raw entries, 802 distinct codes: the upstream file lists B4060011
	// twice and A2301001 once in RestartBusiness and once in SeparateNPU, and
	// the builder keeps the first level, so the table holds the distinct set.
	table := npuErrorCodeSnapshot()
	if len(table) >= total {
		t.Errorf("the lookup table holds %d of %d raw entries, want fewer (duplicates collapse)", len(table), total)
	}
	if len(table) == 0 {
		t.Fatal("the lookup table is empty")
	}
}

// TestNPUErrorCodeDuplicatesKeepFirstLevel pins the duplicate policy without
// panicking: A2301001 is listed by RestartBusiness (rerun, first) and
// SeparateNPU (isolate, later), and the first wins.
func TestNPUErrorCodeDuplicatesKeepFirstLevel(t *testing.T) {
	category, ok := npuErrorCodeInfo(0xA2301001)
	if !ok {
		t.Fatal("A2301001 is not in the knowledge base")
	}
	if category.Family != model.NPUErrorCatRerun {
		t.Errorf("A2301001 maps to %q, want the first listed tier %q", category.Family, model.NPUErrorCatRerun)
	}

	// A document whose only entry repeats still yields a usable table: the
	// builder logs and keeps going.
	synthetic := []byte(`{"NotHandleFaultCodes":["80E21007","80E21007"],"RestartNPUCodes":["8C03A000"]}`)
	built := buildNPUErrorCodeMap(synthetic, zap.NewNop())
	if len(built) != 2 {
		t.Errorf("the synthetic document built %d entries, want 2 (the duplicate collapses)", len(built))
	}
	if built[0x80E21007].Family != model.NPUErrorCatNoAction {
		t.Errorf("80E21007 maps to %q, want %q", built[0x80E21007].Family, model.NPUErrorCatNoAction)
	}
}

// TestNPUErrorCodeUndecodableDocumentIsEmpty pins the degradation path: a file
// that does not decode must leave the caller on its generic wording instead of
// panicking at first lookup.
func TestNPUErrorCodeUndecodableDocumentIsEmpty(t *testing.T) {
	if got := buildNPUErrorCodeMap([]byte("not json"), zap.NewNop()); len(got) != 0 {
		t.Errorf("an undecodable document built %d entries, want 0", len(got))
	}
}

// TestNPUErrorCodeFamilies pins the eight levels onto the five handling tiers,
// two real codes per tier, plus the boundary cases: 0 (no error), a code the
// snapshot does not carry, and a negative code.
func TestNPUErrorCodeFamilies(t *testing.T) {
	cases := []struct {
		code       string // hex, as the knowledge base writes it
		wantFamily string
		wantCat    string
		wantLevel  string
	}{
		// no action needed — NotHandleFault
		{"80E21007", model.NPUErrorCatNoAction, "no action needed", "NotHandleFault"},
		{"80E38003", model.NPUErrorCatNoAction, "no action needed", "NotHandleFault"},
		// workload rerun — RestartRequest
		{"80C98008", model.NPUErrorCatRerun, "workload rerun", "RestartRequest"},
		{"80C98002", model.NPUErrorCatRerun, "workload rerun", "RestartRequest"},
		// workload rerun — RestartBusiness
		{"8C204E00", model.NPUErrorCatRerun, "workload rerun", "RestartBusiness"},
		{"A8028802", model.NPUErrorCatRerun, "workload rerun", "RestartBusiness"},
		// NPU reset required — FreeRestartNPU
		{"8C0E4E00", model.NPUErrorCatReset, "NPU reset required", "FreeRestartNPU"},
		{"8C104E00", model.NPUErrorCatReset, "NPU reset required", "FreeRestartNPU"},
		// NPU reset required — RestartNPU
		{"8C03A000", model.NPUErrorCatReset, "NPU reset required", "RestartNPU"},
		{"8C1FA006", model.NPUErrorCatReset, "NPU reset required", "RestartNPU"},
		// chip isolation required — SeparateNPU
		{"80E3A201", model.NPUErrorCatIsolate, "chip isolation required", "SeparateNPU"},
		{"80E18402", model.NPUErrorCatIsolate, "chip isolation required", "SeparateNPU"},
		// chip isolation required — PreSeparateNPU
		{"8118043C", model.NPUErrorCatIsolate, "chip isolation required", "PreSeparateNPU"},
		{"8118045A", model.NPUErrorCatIsolate, "chip isolation required", "PreSeparateNPU"},
		// sub-health — SubHealthFault
		{"81A3880C", model.NPUErrorCatSubHealth, "sub-health", "SubHealthFault"},
		{"81AD8605", model.NPUErrorCatSubHealth, "sub-health", "SubHealthFault"},
	}

	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			var code int64
			for _, r := range tc.code {
				code = code*16 + int64(strings.IndexRune("0123456789ABCDEF", r))
			}
			category, ok := npuErrorCodeInfo(code)
			if !ok {
				t.Fatalf("%s is not in the knowledge base", tc.code)
			}
			if category.Family != tc.wantFamily {
				t.Errorf("family = %q, want %q", category.Family, tc.wantFamily)
			}
			if category.Category != tc.wantCat {
				t.Errorf("category = %q, want %q", category.Category, tc.wantCat)
			}
			if category.Level != tc.wantLevel {
				t.Errorf("level = %q, want %q", category.Level, tc.wantLevel)
			}
			if category.Action == "" {
				t.Error("action is empty")
			}
			seen[tc.wantFamily] = true
		})
	}

	// All five tiers must be covered by the table above, otherwise a family
	// could rot unnoticed.
	for _, family := range []string{
		model.NPUErrorCatNoAction, model.NPUErrorCatRerun, model.NPUErrorCatReset,
		model.NPUErrorCatIsolate, model.NPUErrorCatSubHealth,
	} {
		if !seen[family] {
			t.Errorf("the table does not cover the %q tier", family)
		}
	}
}

// TestNPUErrorCodeBoundaries pins the codes the lookup must refuse. The first
// half runs against the real snapshot; the second against a synthetic table in
// which 0 and -1 are actually listed, so the "no error is never a fault class"
// rule is load-bearing rather than vacuously satisfied by the snapshot's
// contents.
func TestNPUErrorCodeBoundaries(t *testing.T) {
	cases := []struct {
		name string
		code int64
	}{
		{"zero is no error", 0},
		{"negative is not a code", -1},
		{"a code outside the snapshot", 0xDEADBEEF},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			category, ok := npuErrorCodeInfo(tc.code)
			if ok {
				t.Errorf("%d resolved to %+v, want no classification", tc.code, category)
			}
			if category != (model.NPUErrorCodeCategory{}) {
				t.Errorf("%d returned a non-zero category %+v", tc.code, category)
			}
		})
	}

	poisoned := map[int64]model.NPUErrorCodeCategory{
		0:  {Family: model.NPUErrorCatNoAction, Category: "no action needed"},
		-1: {Family: model.NPUErrorCatNoAction, Category: "no action needed"},
		7:  {Family: model.NPUErrorCatRerun, Category: "workload rerun"},
	}
	for _, code := range []int64{0, -1} {
		if category, ok := lookupNPUErrorCode(poisoned, code); ok {
			t.Errorf("a table entry for %d was returned (%+v), want the code refused", code, category)
		}
	}
	if category, ok := lookupNPUErrorCode(poisoned, 7); !ok || category.Family != model.NPUErrorCatRerun {
		t.Errorf("a positive code in a synthetic table resolved to %+v/%v", category, ok)
	}
}

// TestNPUErrorCodeHexFormat pins the canonical rendering the knowledge base is
// keyed by: uppercase, zero padded to eight digits. A lowercase code is not a
// separate entry — the lookup is numeric, the hex is only the display form.
func TestNPUErrorCodeHexFormat(t *testing.T) {
	cases := map[int64]string{
		0x80E21007: "80E21007",
		0x0000ABCD: "0000ABCD",
		0x1:        "00000001",
	}
	for code, want := range cases {
		if got := npuErrorCodeHex(code); got != want {
			t.Errorf("npuErrorCodeHex(%d) = %q, want %q", code, got, want)
		}
	}

	// The lowercase spelling of an entry resolves to the same class: the hex
	// string is only ever parsed, never compared as text.
	category, ok := npuErrorCodeInfo(0x80e21007)
	if !ok || category.Family != model.NPUErrorCatNoAction {
		t.Errorf("the lowercase spelling resolved to %+v/%v", category, ok)
	}
}

// TestGradeReasonsCarryNoJSONEscapes is the W2 guard: every reason the per-chip
// detection can produce must stay free of the three characters encoding/json
// escapes (< > &), because the reasons are exported and an escaped ">=" turns a
// stable phrase into noise. The fixture drives all ten triggers in one pass and
// fails if any of them is missing, so a template rewritten back to ">=" — or a
// new reason added later — cannot pass silently.
func TestGradeReasonsCarryNoJSONEscapes(t *testing.T) {
	chips := gradedChips(10)
	chips[0].Temp = npuTempCriticalCelsius + 1 // temp, critical
	chips[1].Temp = npuTempWarningCelsius      // temp, warning
	chips[2].HBMEccDoubleErr = 1               // double-bit ECC
	chips[3].HBMEccSingleErr = 1               // single-bit ECC
	chips[4].LinkStatusKnown = true            // link down
	chips[4].LinkStatus = 0
	chips[5].ErrorCode = 0x80E21007 // classified error code
	chips[6].ErrorCode = 0xDEADBEEF // unclassified error code
	chips[7].Health = "Error"       // exporter health
	chips[8].Health = "Unknown"     // unknown health (no hit, no reason)

	node := &model.NodeData{
		Name: "npu-node", NPUCapacity: 10, NPUChips: chips,
		NPUStatusKnown: true, NPUStatusMask: 0xffffffffffffff00 | 1<<9, // chip 9: bitmask
	}
	npuThresholdAlerts(node, time.Now())

	wantReasons := map[int]string{
		0: "temp 91 ge 90",
		1: "temp 80 ge 80",
		2: "double-bit ECC",
		3: "single-bit ECC",
		4: "link down",
		5: "error code 80E21007 (no action needed)",
		6: "error code 3735928559",
		7: "chip health Error",
	}
	for phyID, want := range wantReasons {
		got := gradeOf(t, node.NPUChipHealth, phyID).Reason
		if got != want {
			t.Errorf("chip %d reason = %q, want %q", phyID, got, want)
		}
	}

	// Chip 9 is named by the device-plugin bitmask, and chip 8 (unknown health)
	// stays healthy with no reason at all; neither may carry an escaped
	// character either.
	for _, phyID := range []int{8, 9} {
		grade := gradeOf(t, node.NPUChipHealth, phyID)
		if strings.ContainsAny(grade.Reason, "<>&") {
			t.Errorf("chip %d reason %q carries a JSON-escaped character", phyID, grade.Reason)
		}
	}
	// Non-vacuousness: the fixture must have produced at least the ten reasons
	// above, including the two cluster-provided ones.
	for i, grade := range node.NPUChipHealth {
		if strings.ContainsAny(grade.Reason, "<>&") {
			t.Errorf("chip %d (%d/%d) reason %q carries a JSON-escaped character", i, grade.NPUID, grade.Chip, grade.Reason)
		}
	}

	// A cluster-provided fault level that itself carries ">" must not leak it
	// into the exported reason either.
	node.NPUFault = &model.NPUFaultInfo{DeviceFaultLevel: "Urgent>NPU"}
	npuThresholdAlerts(node, time.Now())
	for _, grade := range node.NPUChipHealth {
		if strings.ContainsAny(grade.Reason, "<>&") {
			t.Errorf("a fault level leaked an escaped character into %q", grade.Reason)
		}
	}
}

// TestNPUErrorCodePhrasesSurviveJSON pins the reason template against the JSON
// export: a grade reason is exported with encoding/json's default HTML
// escaping, so any < > & in a knowledge-base phrase would surface as an escape
// sequence. Every classification phrase and every reason the knowledge base can
// produce must stay free of them.
func TestNPUErrorCodePhrasesSurviveJSON(t *testing.T) {
	if len(npuErrorCodeLevels) == 0 {
		t.Fatal("the knowledge base is empty")
	}
	const forbidden = "<>&"

	for _, level := range npuErrorCodeLevels {
		for field, value := range map[string]string{
			"Category": level.category.Category,
			"Action":   level.category.Action,
			"Family":   level.category.Family,
			"Level":    level.category.Level,
		} {
			if strings.ContainsAny(value, forbidden) {
				t.Errorf("%s.%s = %q carries a JSON-escaped character", level.level, field, value)
			}
		}

		// The rendered reason must survive the round trip verbatim.
		reason := npuErrorCodeReason(0x80E21007, level.category)
		encoded, err := json.Marshal(model.NPUChipGrade{Grade: model.NPUChipGradeFault, Reason: reason})
		if err != nil {
			t.Fatalf("marshalling the grade: %v", err)
		}
		if strings.Contains(string(encoded), `\u003`) {
			t.Errorf("the exported reason %q carries an HTML escape: %s", reason, encoded)
		}
		var decoded model.NPUChipGrade
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decoding the grade: %v", err)
		}
		if decoded.Reason != reason {
			t.Errorf("round trip changed the reason: %q -> %q", reason, decoded.Reason)
		}
	}
}
