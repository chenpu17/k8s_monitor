package ui

// The extended NPU chip table's HBM column adapts to its data: 14 covers the
// widest cell at the real 64 GiB per-chip geometry, and larger HBM capacities
// grow the column (and with it the two terminal-width tier thresholds) so the
// widest cell is never clipped. The tier thresholds are derived from the
// column widths:
//
//	full tier: 4 (indent) + Σ(9 columns) + 8 gaps
//	mid tier : 4 (indent) + Σ(7 surviving columns) + 9 gaps
//
// With colHBM 14 those are 72 and 62 (the historical constants, pinned by the
// existing responsive-table goldens); the 128 GB fixture below widens colHBM
// to 16 and both thresholds to 74 and 64.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
)

// wideHBMChipNode builds a node whose chips report a 128 GB HBM: the widest
// cell ("128.0/128G(100%)", 16 cells) overflows a fixed 14-wide column.
func wideHBMChipNode() *model.NodeData {
	return &model.NodeData{
		Name: "node-hbm128", Status: "Ready",
		NPUCapacity: 2, NPUAllocatable: 2,
		NPUStatusKnown:  true,
		NPUHealthStatus: "Healthy",
		NPUChips: []model.NPUChipData{
			{PhyID: 0, Health: "OK", AICore: 10, VectorUtil: 5.5, HBMUsed: 131072, HBMTotal: 131072,
				Temp: 60, Power: 150.5, Voltage: 0.85, AICoreFreq: 1800, LinkStatus: 1},
			{PhyID: 1, Health: "OK", AICore: 20, VectorUtil: 6.5, HBMUsed: 65536, HBMTotal: 131072,
				Temp: 61, Power: 151.5, Voltage: 0.86, AICoreFreq: 1900, LinkStatus: 0},
		},
	}
}

// TestExtendedTableHBMColumnAdapts locks the derived tier geometry for a
// 128 GB HBM node: colHBM 16 moves the full tier to 74 and the mid tier to 64,
// the header, separator and data rows agree on the surviving column set, and
// the widest HBM cell is never clipped.
func TestExtendedTableHBMColumnAdapts(t *testing.T) {
	cases := []struct {
		width        int
		columns      []string
		wantAbsent   []string
		blockAbsent  []string
		blockPresent []string
		rowWidth     int
	}{
		{
			// Full tier at the derived threshold 74: nine columns, row
			// 4 + 62 + 8 = 74.
			width:        74,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt", "Freq", "Link"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"128.0/128G(100%)", "64.0/128G(50%)", "1800MHz", "Up"},
			rowWidth:     74,
		},
		{
			// One below the full tier: Freq and Link drop, row
			// 4 + 51 + 6 = 61.
			width:        73,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt"},
			wantAbsent:   []string{"Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "Up", "Down"},
			blockPresent: []string{"128.0/128G(100%)", "0.85V"},
			rowWidth:     61,
		},
		{
			// The mid tier at the derived threshold 64 still carries the full
			// 16-cell HBM cell in seven columns.
			width:        64,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt"},
			wantAbsent:   []string{"Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "Up", "Down"},
			blockPresent: []string{"128.0/128G(100%)", "0.85V"},
			rowWidth:     61,
		},
		{
			// One below the mid tier: Volt drops as well, row
			// 4 + 45 + 5 = 54.
			width:        63,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power"},
			wantAbsent:   []string{"Volt", "Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "0.85V", "0.86V", "Up", "Down"},
			blockPresent: []string{"128.0/128G(100%)", "64.0/128G(50%)"},
			rowWidth:     54,
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("width_%d", tc.width), func(t *testing.T) {
			out := renderNPUDetailAtWidth(t, wideHBMChipNode(), tc.width)

			block := blockAfter(out, "NPU Chip Details")
			if len(block) != 5 { // section title + header + separator + 2 chip rows
				t.Fatalf("extended chip table has %d lines, want 5\n---\n%s", len(block), out)
			}

			header := block[1]
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

			for _, line := range block[1:] {
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
