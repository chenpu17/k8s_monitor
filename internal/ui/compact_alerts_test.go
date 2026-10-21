package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The overview compact alert panel renders its eight strings through the
// localizer, so the English output must stay byte-identical to the previously
// hardcoded strings (golden) and the Chinese output must actually switch.
// The colour profile is forced to Ascii so the golden lines are plain text
// whatever the ambient terminal detection is (same pattern as color_test.go).
func forcedAsciiProfile(t *testing.T) {
	t.Helper()
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })
}

func TestCompactAlertLinesEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	summary := &model.ClusterSummary{
		NotReadyNodes:        1,
		MemoryPressureNodes:  2,
		DiskPressureNodes:    3,
		PIDPressureNodes:     4,
		CrashLoopBackOffPods: 5,
		ImagePullBackOffPods: 6,
		OOMKilledPods:        7,
		NoEndpointServices:   8,
	}
	lines := en.compactAlertLines(summary)

	// StyleHeader pads by 1 space on each side and the alert styles add
	// nothing, so TrimSpace keeps the comparison byte-exact on the panel text.
	want := []string{
		"[🔔 Alerts]",
		"",
		"[X] 1 NotReady",
		"[M] 2 MemPress",
		"[D] 3 DiskPress",
		"[P] 4 PIDPress",
		"[!] 5 CrashLoop",
		"[I] 6 ImgPull",
		"[!] 7 OOMKill",
		"[S] 8 NoEndpt",
	}
	if len(lines) != len(want) {
		t.Fatalf("compactAlertLines returned %d lines, want %d: %q", len(lines), len(want), lines)
	}
	for i := range want {
		if got := strings.TrimSpace(lines[i]); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}

	// No counted alerts: the panel must show the "no alerts" status line.
	empty := en.compactAlertLines(&model.ClusterSummary{})
	wantEmpty := []string{"[🔔 Alerts]", "", "[OK] No alerts"}
	if len(empty) != len(wantEmpty) {
		t.Fatalf("empty-summary compactAlertLines returned %d lines, want %d: %q", len(empty), len(wantEmpty), empty)
	}
	for i := range wantEmpty {
		if got := strings.TrimSpace(empty[i]); got != wantEmpty[i] {
			t.Errorf("empty-summary line %d = %q, want %q", i, got, wantEmpty[i])
		}
	}
}

func TestCompactAlertLinesZh(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	summary := &model.ClusterSummary{
		NotReadyNodes:        1,
		MemoryPressureNodes:  2,
		DiskPressureNodes:    3,
		PIDPressureNodes:     4,
		CrashLoopBackOffPods: 5,
		ImagePullBackOffPods: 6,
		OOMKilledPods:        7,
		NoEndpointServices:   8,
	}

	enLines := en.compactAlertLines(summary)
	zhLines := zh.compactAlertLines(summary)
	if len(zhLines) != len(enLines) {
		t.Fatalf("zh panel has %d lines, en has %d", len(zhLines), len(enLines))
	}
	for i := range zhLines {
		// The blank separator line is locale-neutral by construction.
		if zhLines[i] == "" {
			continue
		}
		if strings.TrimSpace(zhLines[i]) == strings.TrimSpace(enLines[i]) {
			t.Errorf("zh line %d %q equals the en line — locale did not switch", i, zhLines[i])
		}
	}

	joined := strings.Join(zhLines, "\n")
	for _, noun := range []string{"告警", "未就绪", "内存压力", "磁盘压力", "PID 压力", "崩溃循环", "镜像拉取", "OOM", "无端点"} {
		if !strings.Contains(joined, noun) {
			t.Errorf("zh panel %q does not contain %q", joined, noun)
		}
	}
	for _, bad := range []string{"overview.", "{{", "}}", "<no value>", "%!", "NotReady", "MemPress", "DiskPress", "PIDPress", "CrashLoop", "ImgPull", "OOMKill", "NoEndpt", "No alerts"} {
		if strings.Contains(joined, bad) {
			t.Errorf("zh panel %q contains %q", joined, bad)
		}
	}

	// Empty summary: the "no alerts" status line switches too.
	zhEmpty := strings.Join(zh.compactAlertLines(&model.ClusterSummary{}), "\n")
	if !strings.Contains(zhEmpty, "[OK] 暂无告警") {
		t.Errorf("zh empty panel %q does not contain 暂无告警", zhEmpty)
	}
	if strings.Contains(zhEmpty, "No alerts") {
		t.Errorf("zh empty panel %q still contains the English status line", zhEmpty)
	}
}

// TestCompactAlertLinesCoversHasAlerts: every condition hasAlerts counts must
// render its own panel line, so alertCount==0 can never coexist with
// hasAlerts==true (the "[OK] No alerts" lie found in the batch-11 review,
// reachable when PID pressure or no-endpoint services were the only signal).
func TestCompactAlertLinesCoversHasAlerts(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	cases := []struct {
		name    string
		summary model.ClusterSummary
		want    string
	}{
		{"pid pressure only", model.ClusterSummary{PIDPressureNodes: 2}, "[P] 2 PIDPress"},
		{"no-endpoint services only", model.ClusterSummary{NoEndpointServices: 3}, "[S] 3 NoEndpt"},
	}
	for _, tc := range cases {
		joined := strings.Join(en.compactAlertLines(&tc.summary), "\n")
		if !strings.Contains(joined, tc.want) {
			t.Errorf("%s: panel %q does not contain %q", tc.name, joined, tc.want)
		}
		if strings.Contains(joined, "No alerts") {
			t.Errorf("%s: panel %q still claims no alerts", tc.name, joined)
		}
	}
}
