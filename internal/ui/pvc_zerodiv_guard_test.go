package ui

// pvc_zerodiv_guard_test.go — batch 20 lockdown for the PVC detail Used line.
//
// A PVC whose capacity is unknown (0) but that reports usage made the detail
// view render a garbage percent ("+Inf%": positive usage divided by a zero
// capacity; a 0/0 would be NaN, but the UsedBytes>0 branch excludes it). The
// guard renders the usage without a percentage when the capacity is unknown,
// and the normal path keeps its percentage.

import (
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

const giB = 1024 * 1024 * 1024

// TestPVCDetailUsedLineZeroCapacityGuard is table-driven over the capacity/usage
// combinations of the Used line in the PVC detail view.
func TestPVCDetailUsedLineZeroCapacityGuard(t *testing.T) {
	cases := []struct {
		name        string
		locale      string
		capacity    int64
		usedBytes   int64
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:      "unknown capacity with usage must not render NaN percent",
			locale:    "en",
			capacity:  0,
			usedBytes: 1 * giB,
			wantContain: []string{
				"Used",
				formatMemory(1 * giB),
			},
			wantAbsent: []string{
				"NaN",
				"%",
			},
		},
		{
			name:      "known capacity keeps the percentage",
			locale:    "en",
			capacity:  5 * giB,
			usedBytes: 3 * giB,
			wantContain: []string{
				"Used",
				"(60.0%)",
			},
		},
		{
			name:        "no usage renders no Used line",
			locale:      "en",
			capacity:    5 * giB,
			usedBytes:   0,
			wantContain: []string{},
			wantAbsent: []string{
				"  Used:",
			},
		},
		{
			name:      "zh locale with unknown capacity stays free of NaN",
			locale:    "zh",
			capacity:  0,
			usedBytes: 1 * giB,
			wantContain: []string{
				"已用",
				formatMemory(1 * giB),
			},
			wantAbsent: []string{
				"NaN",
				// The pre-guard defect renders the division result as a
				// percentage; catch it regardless of its IEEE spelling.
				"%",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(nil, zap.NewNop(), time.Second, tc.locale, "test", 200)
			m.width = 120
			m.height = 40
			m.currentView = ViewPVCDetail
			m.selectedPVC = &model.PVCData{
				Name:      "data-0",
				Namespace: "default",
				Status:    "Bound",
				Capacity:  tc.capacity,
				UsedBytes: tc.usedBytes,
			}

			out := m.renderPVCDetail()

			for _, want := range tc.wantContain {
				if !strings.Contains(out, want) {
					t.Errorf("render is missing %q:\n%s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("render must not contain %q:\n%s", absent, out)
				}
			}
		})
	}
}
