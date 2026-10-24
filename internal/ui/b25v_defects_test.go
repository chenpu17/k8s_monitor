package ui

// b25v_defects_test.go — the reviewer's two RED probes against the batch as
// delivered. Both are regressions relative to HEAD=d134e53 and both are
// demonstrated by running the same file in a HEAD worktree.

import (
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
)

// b25vMixedCluster is one NPU node with 8 chips: 5 with a reported health, 2
// degraded, 1 fault, and — for the defect probe — the reported/unreported mix
// that flips the summary wording.
func b25vMixedCluster(unknownChips int) *model.ClusterData {
	chips := make([]model.NPUChipData, 8)
	grades := make([]model.NPUChipGrade, 8)
	for i := range chips {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK"}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	for i := 0; i < unknownChips; i++ {
		chips[i].Health = "Unknown"
	}
	// 1 degraded + 1 fault, so the health segment carries all three counts.
	grades[6].Grade = model.NPUChipGradeDegraded
	grades[7].Grade = model.NPUChipGradeFault
	return &model.ClusterData{
		Nodes: []*model.NodeData{{
			Name: "n", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8,
			NPUChips: chips, NPUChipHealth: grades,
		}},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8, NPUNodesCount: 1},
	}
}

// TestB25VSummaryBarKeepsFaultCountAt38Columns is the batch's narrow-terminal
// rule applied to the wording it introduced. W2-4 lengthens the health segment
// ("healthy" → "no known fault", +7 cells), and at 38 columns that segment no
// longer fits the bar: wrapSummarySegments truncates it and the trailing
// counters — the degraded and fault counts, the numbers a reader scans the bar
// for — are cut off ("6/8 no known fault · 1 degraded · 1...").
//
// At HEAD the same fixture renders "6/8 healthy · 1 degraded · 1 fault" in 34
// cells, in full. So the wording change traded a truthfulness fix for a
// information loss on the narrow terminal, without the batch's tests noticing:
// b23r's TestNPUViewSummarySegmentsSurviveNarrow only runs an all-reported
// fixture, where the new key is never selected.
func TestB25VSummaryBarKeepsFaultCountAt38Columns(t *testing.T) {
	forcedAsciiProfile(t)

	for _, width := range []int{38, 40} {
		m := NewModel(nil, nil, 0, "en", "test", 200)
		m.width, m.height = width, 60
		m.currentView = ViewNPU
		m.clusterData = b25vMixedCluster(2) // two unknown chips flip the wording

		out := stripANSI(m.renderNPUView())
		bar := lineContaining(out, "degraded")
		if bar == "" {
			t.Fatalf("width %d: no health segment rendered:\n%s", width, out)
		}
		if w := runewidth.StringWidth(bar); w > width {
			t.Errorf("width %d: the health segment is %d cells: %q", width, w, bar)
		}
		// The counts are what the segment exists for; none may be dropped.
		for _, want := range []string{"degraded", "fault"} {
			if !strings.Contains(bar, want) {
				t.Errorf("width %d: the summary bar lost the %q counter: %q", width, want, bar)
			}
		}
		if strings.Contains(bar, "...") {
			t.Errorf("width %d: the summary bar truncated the health segment: %q", width, bar)
		}
	}
}

// TestB25VNodeDetailOwnerValueNotOverTruncated pins the W2-1 truncation budget.
// npuChipSparseLines passes the *template* — "    chip {{.NPU}}-{{.Chip}} →
// {{.Owner}}" — to detailValueWidth, which measures what it is given as the
// label. A template's visual length (43 cells, placeholders included) is three
// times the length of the prefix actually printed ("    chip 0-0 → ", 15 cells),
// so the budget loses ~28 cells of the space the terminal really has.
//
// The row never overflows — it over-truncates, dropping the owner identity on a
// terminal wide enough to show it. At 80 columns an 8-cell owner gets a 31-cell
// budget instead of the 60 the fallback allows, and a name that fits the row is
// still cut to "...".
func TestB25VNodeDetailOwnerValueNotOverTruncated(t *testing.T) {
	forcedAsciiProfile(t)

	// 40 cells of owner: comfortably inside 80 columns
	// (4 indent + "chip 0-0 → " + 40 = 55 cells) and inside the 60-cell cap.
	owner := strings.Repeat("a", 20)
	node := extendedChipNode()
	node.NPUChips[0].OwnerNamespace, node.NPUChips[0].OwnerPod = owner, owner

	for _, width := range []int{80, 100, 160} {
		out := stripANSI(renderNPUDetailAtWidth(t, node, width))
		row := lineContaining(out, "chip 0-0 →")
		if row == "" {
			t.Fatalf("width %d: no owner row rendered:\n%s", width, out)
		}
		want := "chip 0-0 → " + owner + "/" + owner
		if !strings.Contains(row, want) {
			t.Errorf("width %d: the owner row dropped part of a value that fits (%d cells available, row is %d):\n got %q\nwant it to contain %q",
				width, width, runewidth.StringWidth(row), row, want)
		}
	}
}
