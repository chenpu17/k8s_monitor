package ui

// The node detail pods table degrades whole columns on narrow terminals
// instead of clipping RESTARTS off the screen edge. Row widths, recomputed
// from the indent and column arithmetic (indent 2 + surviving columns + one
// separator per gap):
//
//	w >= 85 : NAME 35 + NAMESPACE 20 + STATUS 15 + RESTARTS 10 + 3 gaps = 85
//	w >= 64 : NAMESPACE dropped                     = 2 + 35+15+10 + 2 = 64
//	w >= 49 : NAME shrunk to 20 (truncate, "...")   = 2 + 20+15+10 + 2 = 49
//	w <  49 : RESTARTS dropped as well              = 2 + 20+15 + 1   = 38

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
)

func podsTierFixture() []*model.PodData {
	return []*model.PodData{
		{Name: "web-server-7f9d8b6c5-xqk2v", Namespace: "default", Node: "node-pods", Phase: "Running", RestartCount: 12},
		{Name: "beta", Namespace: "kube-system", Node: "node-pods", Phase: "Running", RestartCount: 34},
	}
}

// renderPodsTableAtWidth renders the node detail at the given width and
// returns the pods table block: header, separator and the pod rows.
func renderPodsTableAtWidth(t *testing.T, width int) []string {
	t.Helper()

	m := newTestModel(t)
	m.width = width
	m.height = 200
	pods := podsTierFixture()
	node := &model.NodeData{Name: "node-pods", Status: "Ready", PodCount: len(pods)}
	m.clusterData = &model.ClusterData{Pods: pods}
	m.selectedNode = node

	out := stripANSI(m.renderNodeDetail())
	block := blockAfter(out, "NAME")
	if block == nil {
		t.Fatalf("pods table header not found at width %d\n---\n%s", width, out)
	}
	return block
}

// TestNodePodsTableColumnTiers locks the four tiers: the surviving columns and
// their values, the dropped columns' values gone, and every rendered line
// exactly as wide as the tier's row arithmetic.
func TestNodePodsTableColumnTiers(t *testing.T) {
	// 26-cell name; at the short-NAME tier it truncates to 20 cells.
	const longName = "web-server-7f9d8b6c5-xqk2v"
	const shortName = "web-server-7f9d8b..."

	cases := []struct {
		width        int
		columns      []string
		wantAbsent   []string
		blockAbsent  []string
		blockPresent []string
		rowWidth     int
	}{
		{
			// Boundary: the four-column table is itself 85 cells.
			width:        120,
			columns:      []string{"NAME", "NAMESPACE", "STATUS", "RESTARTS"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"kube-system", "12", "34", longName},
			rowWidth:     85,
		},
		{
			width:        85,
			columns:      []string{"NAME", "NAMESPACE", "STATUS", "RESTARTS"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"kube-system", "12", "34"},
			rowWidth:     85,
		},
		{
			width:        84,
			columns:      []string{"NAME", "STATUS", "RESTARTS"},
			wantAbsent:   []string{"NAMESPACE"},
			blockAbsent:  []string{"kube-system", "default"},
			blockPresent: []string{"12", "34", longName, "Running"},
			rowWidth:     64,
		},
		{
			width:        64,
			columns:      []string{"NAME", "STATUS", "RESTARTS"},
			wantAbsent:   []string{"NAMESPACE"},
			blockAbsent:  []string{"kube-system"},
			blockPresent: []string{"12", "34", longName},
			rowWidth:     64,
		},
		{
			width:        63,
			columns:      []string{"NAME", "STATUS", "RESTARTS"},
			wantAbsent:   []string{"NAMESPACE"},
			blockAbsent:  []string{"kube-system", longName},
			blockPresent: []string{shortName, "12", "34"},
			rowWidth:     49,
		},
		{
			width:        49,
			columns:      []string{"NAME", "STATUS", "RESTARTS"},
			wantAbsent:   []string{"NAMESPACE"},
			blockAbsent:  []string{"kube-system", longName},
			blockPresent: []string{shortName, "12"},
			rowWidth:     49,
		},
		{
			width:        48,
			columns:      []string{"NAME", "STATUS"},
			wantAbsent:   []string{"NAMESPACE", "RESTARTS"},
			blockAbsent:  []string{"kube-system", longName, "12", "34"},
			blockPresent: []string{shortName, "Running"},
			rowWidth:     38,
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("width_%d", tc.width), func(t *testing.T) {
			block := renderPodsTableAtWidth(t, tc.width)
			if len(block) != 4 { // header + separator + 2 pod rows
				t.Fatalf("pods table has %d lines, want 4: %q", len(block), block)
			}

			header := block[0]
			for _, col := range tc.columns {
				if !strings.Contains(header, col) {
					t.Errorf("header is missing column %q at width %d: %q", col, tc.width, header)
				}
			}
			for _, col := range tc.wantAbsent {
				if strings.Contains(header, col) {
					t.Errorf("header kept dropped column %q at width %d: %q", col, tc.width, header)
				}
			}

			for _, line := range block {
				if w := runewidth.StringWidth(line); w != tc.rowWidth {
					t.Errorf("table line is %d cells wide, want %d at terminal width %d: %q",
						w, tc.rowWidth, tc.width, line)
				}
			}

			blockText := strings.Join(block, "\n")
			for _, gone := range tc.blockAbsent {
				if strings.Contains(blockText, gone) {
					t.Errorf("dropped-column value %q still rendered at width %d:\n%s", gone, tc.width, blockText)
				}
			}
			for _, want := range tc.blockPresent {
				if !strings.Contains(blockText, want) {
					t.Errorf("surviving-column value %q missing at width %d:\n%s", want, tc.width, blockText)
				}
			}
		})
	}
}
